package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// -cleanup uses this label to find the containers of a run that crashed.
const label = "kardianos-service-test"

type HostInfo struct {
	GOARCH  string
	Cgroups int
}

// Runtime exists so that the unit tests can replace podman.
type Runtime interface {
	Info(ctx context.Context) (HostInfo, error)
	Build(ctx context.Context, tag, dir string, pull bool, out io.Writer) error
	Start(ctx context.Context, name, tag string, init *InitSystem) error
	// The error is only for a failure of the runtime. Thus a broken runtime
	// does not show as a failed test.
	Exec(ctx context.Context, name string, env []string, out io.Writer, argv ...string) (int, error)
	CopyIn(ctx context.Context, name, src, dst string) error
	Logs(ctx context.Context, name string, out io.Writer) error
	// Remove uses its own context. It must also work after an interrupt
	// cancels the run.
	Remove(name string) error
	RemoveLabeled() (int, error)
}

type podman struct{}

func (podman) buildArgs(tag, dir string, pull bool) []string {
	args := []string{"build", "-t", tag}
	if pull {
		args = append(args, "--pull=always")
	}
	return append(args, dir)
}

// --systemd=always also sets up /run, /tmp and the cgroups for the other init
// systems.
func (podman) startArgs(name, tag string, init *InitSystem) []string {
	args := []string{"run", "-d", "--name", name, "--label", label + "=1", "--systemd=always", tag}
	return append(args, init.Boot...)
}

func (podman) execArgs(name string, env []string, argv []string) []string {
	args := []string{"exec"}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, name)
	return append(args, argv...)
}

func (podman) copyArgs(name, src, dst string) []string {
	return []string{"cp", src, name + ":" + dst}
}

func (podman) run(ctx context.Context, out io.Writer, args ...string) (int, error) {
	c := exec.CommandContext(ctx, "podman", args...)
	c.Stdout, c.Stderr = out, out
	err := c.Run()
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

func (r podman) must(ctx context.Context, out io.Writer, args ...string) error {
	var buf bytes.Buffer
	w := out
	if w == nil {
		w = &buf
	}
	code, err := r.run(ctx, w, args...)
	if err != nil {
		return err
	}
	if code != 0 {
		msg := strings.TrimSpace(buf.String())
		if out != nil {
			msg = "see log"
		}
		return fmt.Errorf("podman %s: exit %d: %s", args[0], code, msg)
	}
	return nil
}

func (r podman) Info(ctx context.Context) (HostInfo, error) {
	var buf bytes.Buffer
	if err := r.must(ctx, &buf, "info", "--format", "{{.Host.Arch}} {{.Host.CgroupsVersion}}"); err != nil {
		return HostInfo{}, fmt.Errorf("%w: %s", err, strings.TrimSpace(buf.String()))
	}
	return parseInfo(buf.String())
}

func parseInfo(s string) (HostInfo, error) {
	f := strings.Fields(s)
	if len(f) != 2 {
		return HostInfo{}, fmt.Errorf("unexpected podman info %q", s)
	}
	cg, err := strconv.Atoi(strings.TrimPrefix(f[1], "v"))
	if err != nil {
		return HostInfo{}, fmt.Errorf("unexpected cgroup version in podman info %q", s)
	}
	return HostInfo{GOARCH: f[0], Cgroups: cg}, nil
}

func (r podman) Build(ctx context.Context, tag, dir string, pull bool, out io.Writer) error {
	return r.must(ctx, out, r.buildArgs(tag, dir, pull)...)
}

func (r podman) Start(ctx context.Context, name, tag string, init *InitSystem) error {
	return r.must(ctx, nil, r.startArgs(name, tag, init)...)
}

// podman exec uses 125 for its own failure. It uses 126 and 127 when it cannot
// start the command.
func isRuntimeFailure(code int) bool { return code >= 125 && code <= 127 }

func (r podman) Exec(ctx context.Context, name string, env []string, out io.Writer, argv ...string) (int, error) {
	if out == nil {
		out = io.Discard
	}
	code, err := r.run(ctx, out, r.execArgs(name, env, argv)...)
	if err != nil {
		return code, err
	}
	if isRuntimeFailure(code) {
		return code, fmt.Errorf("podman exec %s: exit %d", argv[0], code)
	}
	return code, nil
}

func (r podman) CopyIn(ctx context.Context, name, src, dst string) error {
	return r.must(ctx, nil, r.copyArgs(name, src, dst)...)
}

func (r podman) Logs(ctx context.Context, name string, out io.Writer) error {
	return r.must(ctx, out, "logs", name)
}

func (r podman) Remove(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return r.must(ctx, nil, "rm", "-f", name)
}

func (r podman) RemoveLabeled() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var buf bytes.Buffer
	if err := r.must(ctx, &buf, "ps", "-aq", "--filter", "label="+label); err != nil {
		return 0, err
	}
	ids := strings.Fields(buf.String())
	if len(ids) == 0 {
		return 0, nil
	}
	return len(ids), r.must(ctx, nil, append([]string{"rm", "-f"}, ids...)...)
}
