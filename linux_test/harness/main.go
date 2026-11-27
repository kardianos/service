// Command harness starts init systems in temporary containers and runs the
// end-to-end tests in them.
//
//	go run -C linux_test/harness . [flags]
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const rootModule = "github.com/kardianos/service"

func main() { os.Exit(run()) }

func run() int {
	var (
		only      = flag.String("only", "", "comma-separated targets to run (default: all)")
		suiteFlag = flag.String("suite", "", "run only this suite")
		runFlag   = flag.String("run", "", "regexp overriding each suite's test pattern")
		timeout   = flag.Duration("timeout", 10*time.Minute, "per-suite test timeout")
		keep      = flag.Bool("keep", false, "leave failed containers running")
		pull      = flag.Bool("pull", false, "pull base images and rebuild")
		cleanup   = flag.Bool("cleanup", false, "remove containers left by earlier runs, then exit")
		list      = flag.Bool("list", false, "print the targets and suites, then exit")
	)
	flag.Parse()

	if *list {
		printList()
		return 0
	}
	if _, err := exec.LookPath("podman"); err != nil {
		return usage(errors.New("podman is not installed"))
	}
	var rt Runtime = podman{}
	if *cleanup {
		n, err := rt.RemoveLabeled()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("removed %d container(s)\n", n)
		return 0
	}

	sel, err := selectTargets(targets, splitList(*only), *suiteFlag)
	if err != nil {
		return usage(err)
	}
	if _, err := regexp.Compile(*runFlag); err != nil {
		return usage(fmt.Errorf("-run: %w", err))
	}
	wd, err := os.Getwd()
	if err != nil {
		return usage(err)
	}
	root, err := findRoot(wd)
	if err != nil {
		return usage(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	info, err := rt.Info(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "podman info: %v\n", err)
		return 1
	}

	outDir := filepath.Join(root, "linux_test", "out")
	binDir := filepath.Join(outDir, "bin")
	fmt.Printf("podman, linux/%s, cgroup v%d\n", info.GOARCH, info.Cgroups)
	if err := buildBinaries(ctx, root, binDir, info.GOARCH, sel); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	o := options{
		runPattern:  *runFlag,
		timeout:     *timeout,
		bootTimeout: 60 * time.Second,
		poll:        time.Second,
		keep:        *keep,
		pull:        *pull,
		cgroups:     info.Cgroups,
		outDir:      outDir,
		targetsDir:  filepath.Join(root, "linux_test", "targets"),
		binDir:      binDir,
		runID:       strconv.Itoa(os.Getpid()),
		logf: func(format string, args ...any) {
			fmt.Printf("== "+format+"\n", args...)
		},
	}
	var results []Result
	for _, t := range sel {
		results = append(results, runTarget(ctx, rt, t, o)...)
	}
	return summarize(os.Stdout, results, root)
}

func usage(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return 2
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func printList() {
	fmt.Println("Targets:")
	for _, t := range targets {
		var notes []string
		if t.BestEffort {
			notes = append(notes, "best effort")
		}
		if t.RequiresCgroupV1 {
			notes = append(notes, "cgroup v1 only")
		}
		line := fmt.Sprintf("  %-20s %-8s %s", t.Name, t.Init.Name, strings.Join(t.Suites, ","))
		if len(notes) > 0 {
			line += "  (" + strings.Join(notes, ", ") + ")"
		}
		fmt.Println(line)
	}
	fmt.Println("Suites:")
	names := maps.Keys(suites)
	slices.Sort(names)
	for _, name := range names {
		s := suites[name]
		tags := s.Tags
		if tags == "" {
			tags = "none"
		}
		fmt.Printf("  %-10s tags %s, run %s\n", name, tags, s.Run)
	}
}

// The harness is a nested module, but the tests build from the root module.
func findRoot(dir string) (string, error) {
	for {
		if moduleName(filepath.Join(dir, "go.mod")) == rootModule {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod for %s at or above the working directory", rootModule)
		}
		dir = parent
	}
}

func moduleName(gomod string) string {
	f, err := os.Open(gomod)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if name, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(name), `"`)
		}
	}
	return ""
}

func buildBinaries(ctx context.Context, root, binDir, goarch string, sel []Target) error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	builds := map[string][]string{}
	for _, t := range sel {
		for _, name := range t.Suites {
			s := suites[name]
			builds[s.Binary()] = []string{"test", "-c", "-tags", s.Tags, "-o", filepath.Join(binDir, s.Binary()), "."}
			if s.Testsvc {
				builds["testsvc"] = []string{"build", "-o", filepath.Join(binDir, "testsvc"), "./linux_test/testsvc"}
			}
		}
	}
	for bin, args := range builds {
		fmt.Printf("== building %s\n", bin)
		c := exec.CommandContext(ctx, "go", args...)
		c.Dir = root
		// Static binaries do not depend on the libc of the target.
		c.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+goarch)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("building %s: %w", bin, err)
		}
	}
	return nil
}
