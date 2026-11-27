package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Prevents tests from accidentally running on a live host
const marker = "/run/kardianos-service-test"

type Status string

const (
	Pass Status = "PASS"
	Fail Status = "FAIL"
	Skip Status = "SKIP"
)

type Result struct {
	Target, Suite string
	Status        Status
	Reason        string
	Log           string
	// A test binary that runs no tests exits 0. Do not show this as a PASS.
	NoTests bool
}

type options struct {
	runPattern  string
	timeout     time.Duration
	bootTimeout time.Duration
	poll        time.Duration
	keep        bool
	pull        bool
	cgroups     int
	outDir      string
	targetsDir  string
	binDir      string
	runID       string
	logf        func(format string, args ...any)
}

func imageTag(t Target) string { return "localhost/" + label + ":" + t.Name }

func runTarget(ctx context.Context, rt Runtime, t Target, o options) []Result {
	all := func(st Status, reason, log string) []Result {
		var rs []Result
		for _, s := range t.Suites {
			rs = append(rs, Result{Target: t.Name, Suite: s, Status: st, Reason: reason, Log: log})
		}
		return rs
	}
	if t.RequiresCgroupV1 && o.cgroups != 1 {
		return all(Skip, fmt.Sprintf("needs cgroup v1, the host has v%d", o.cgroups), "")
	}
	dir := filepath.Join(o.outDir, t.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return all(Fail, err.Error(), "")
	}
	buildLog := filepath.Join(dir, "build.log")
	f, err := os.Create(buildLog)
	if err != nil {
		return all(Fail, err.Error(), "")
	}
	o.logf("%s: building image", t.Name)
	err = rt.Build(ctx, imageTag(t), filepath.Join(o.targetsDir, t.Name), o.pull, f)
	f.Close()
	if err != nil {
		return all(Fail, "image build failed: "+err.Error(), buildLog)
	}
	var rs []Result
	for _, s := range t.Suites {
		if ctx.Err() != nil {
			rs = append(rs, Result{Target: t.Name, Suite: s, Status: Fail, Reason: "interrupted"})
			continue
		}
		rs = append(rs, runSuite(ctx, rt, t, s, o))
	}
	return rs
}

func runSuite(ctx context.Context, rt Runtime, t Target, suiteName string, o options) (r Result) {
	s := suites[suiteName]
	r = Result{Target: t.Name, Suite: suiteName, Log: filepath.Join(o.outDir, t.Name, suiteName+".log")}
	fail := func(format string, args ...any) Result {
		r.Status, r.Reason = Fail, fmt.Sprintf(format, args...)
		if ctx.Err() != nil {
			r.Reason = "interrupted"
		}
		return r
	}
	logFile, err := os.Create(r.Log)
	if err != nil {
		return fail("%v", err)
	}
	defer logFile.Close()
	var out io.Writer = logFile

	name := fmt.Sprintf("%s-%s-%s-%s", label, t.Name, suiteName, o.runID)
	o.logf("%s/%s: booting %s", t.Name, suiteName, t.Init.Name)
	if err := rt.Start(ctx, name, imageTag(t), t.Init); err != nil {
		return fail("container start failed: %v", err)
	}
	defer func() {
		if o.keep && r.Status == Fail && ctx.Err() == nil {
			r.Reason += "; kept, enter with: podman exec -it " + name + " sh"
			return
		}
		if err := rt.Remove(name); err != nil {
			fmt.Fprintf(os.Stderr, "removing %s: %v\n", name, err)
		}
	}()

	ready, state := waitReady(ctx, rt, name, t.Init, o.bootTimeout, o.poll)
	if !ready {
		fmt.Fprintf(out, "--- %s not ready (state: %s); container logs ---\n", t.Init.Name, state)
		_ = rt.Logs(ctx, name, out)
		r = fail("%s not ready after %s (state: %s)", t.Init.Name, o.bootTimeout, state)
		if t.BestEffort && ctx.Err() == nil {
			r.Status = Skip
		}
		return r
	}
	version := initVersion(ctx, rt, name, t.Init)

	setup := [][]string{{"touch", marker}, {"mkdir", "-p", "/opt/test"}}
	for _, argv := range setup {
		if code, err := rt.Exec(ctx, name, nil, out, argv...); err != nil || code != 0 {
			return fail("%s failed: exit %d, %v", strings.Join(argv, " "), code, err)
		}
	}
	bins := []string{s.Binary()}
	if s.Testsvc {
		bins = append(bins, "testsvc")
	}
	for _, b := range bins {
		if err := rt.CopyIn(ctx, name, filepath.Join(o.binDir, b), "/opt/test/"+b); err != nil {
			return fail("copying %s failed: %v", b, err)
		}
	}

	pattern := s.Run
	if o.runPattern != "" {
		pattern = o.runPattern
	}
	bin := "/opt/test/" + s.Binary()
	var list bytes.Buffer
	if code, err := rt.Exec(ctx, name, nil, &list, bin, "-test.list", pattern); err != nil || code != 0 {
		out.Write(list.Bytes())
		return fail("listing tests failed: exit %d, %v", code, err)
	}
	if countTests(list.String()) == 0 {
		fmt.Fprintf(out, "no test matches %q\n", pattern)
		r.NoTests = true
		return r
	}

	o.logf("%s/%s: testing (%s)", t.Name, suiteName, version)
	runCtx, cancel := context.WithTimeout(ctx, o.timeout+time.Minute)
	code, err := rt.Exec(runCtx, name, []string{"EXPECT_PLATFORM=" + t.Init.Platform}, out,
		bin, "-test.v", "-test.run", pattern, "-test.timeout", o.timeout.String())
	cancel()
	switch {
	case err != nil:
		r = fail("runtime failed while testing: %v", err)
	case code != 0:
		r = fail("%s, tests failed", version)
	default:
		r.Status, r.Reason = Pass, version
		return r
	}
	if ctx.Err() == nil {
		for _, argv := range t.Init.FailureLogs {
			fmt.Fprintf(out, "--- %s ---\n", strings.Join(argv, " "))
			_, _ = rt.Exec(ctx, name, nil, out, argv...)
		}
	}
	return r
}

func countTests(list string) int {
	n := 0
	for _, line := range strings.Split(list, "\n") {
		if strings.HasPrefix(line, "Test") {
			n++
		}
	}
	return n
}

// A run that tests nothing fails. Thus a wrong -run pattern cannot look like a
// pass.
func summarize(w io.Writer, rs []Result, root string) int {
	code := 0
	shown := 0
	fmt.Fprintln(w, "\nSummary:")
	for _, r := range rs {
		if r.NoTests {
			continue
		}
		shown++
		if r.Status == Fail {
			code = 1
		}
		line := fmt.Sprintf("  %s  %s  %s", r.Status, r.Target, r.Suite)
		if r.Reason != "" {
			line += "  " + r.Reason
		}
		if r.Log != "" {
			log := r.Log
			if rel, err := filepath.Rel(root, log); err == nil {
				log = rel
			}
			line += "  " + log
		}
		fmt.Fprintln(w, line)
	}
	if shown == 0 && len(rs) > 0 {
		fmt.Fprintln(w, "  -run matched no test in any selected suite")
		code = 1
	}
	return code
}
