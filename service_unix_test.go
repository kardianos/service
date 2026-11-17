//go:build linux || darwin || solaris || aix || freebsd

package service

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestExecCommandIncludesStderr(t *testing.T) {
	code, out, err := execCommand("sh", true, "-c", "echo partial; echo 'Failed to connect to bus' >&2; exit 4")
	if code != 4 {
		t.Errorf("code = %d, want 4", code)
	}
	if out != "partial\n" {
		t.Errorf("out = %q, want %q", out, "partial\n")
	}
	if want := "exit status 4: Failed to connect to bus"; err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("err = %T, want it to wrap *exec.ExitError", err)
	}
}

func TestExecCommandNoStderr(t *testing.T) {
	_, _, err := execCommand("sh", false, "-c", "exit 3")
	if want := "exit status 3"; err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestExecCommandSuccessIgnoresStderr(t *testing.T) {
	code, out, err := execCommand("sh", true, "-c", "echo warning >&2; echo ok")
	if code != 0 || out != "ok\n" || err != nil {
		t.Fatalf("got (%d, %q, %v), want (0, %q, nil)", code, out, err, "ok\n")
	}
}

func TestExecCommandLargeStderr(t *testing.T) {
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		// Far more than a pipe buffer holds, written before stdout.
		_, out, err := execCommand("sh", true, "-c", "head -c 1000000 /dev/zero | tr '\\0' x >&2; echo ok; exit 1")
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		if r.out != "ok\n" {
			t.Errorf("out = %q, want %q", r.out, "ok\n")
		}
		if r.err == nil {
			t.Fatal("err = nil, want an error")
		}
		if got, want := len(r.err.Error()), len("exit status 1: ")+maxStderr; got != want {
			t.Errorf("len(err) = %d, want %d", got, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("execCommand hung on a command that fills stderr")
	}
}

func TestExecCommandStartFailure(t *testing.T) {
	_, _, err := execCommand("/nonexistent/command", true)
	if err == nil || !strings.HasPrefix(err.Error(), `"/nonexistent/command" failed: `) {
		t.Fatalf("err = %v, want a start failure", err)
	}
}
