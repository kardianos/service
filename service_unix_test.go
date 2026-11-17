//go:build linux || darwin || solaris || aix || freebsd

package service

import (
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestExecCommand(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		args        []string
		wantCode    int
		wantOut     string
		wantErr     string
		wantExitErr bool
	}{
		{
			name:        "stderr in error",
			command:     "sh",
			args:        []string{"-c", "echo partial; echo 'Failed to connect to bus' >&2; exit 4"},
			wantCode:    4,
			wantOut:     "partial\n",
			wantErr:     "exit status 4: Failed to connect to bus",
			wantExitErr: true,
		},
		{
			name:        "no stderr",
			command:     "sh",
			args:        []string{"-c", "exit 3"},
			wantCode:    3,
			wantErr:     "exit status 3",
			wantExitErr: true,
		},
		{
			name:    "success ignores stderr",
			command: "sh",
			args:    []string{"-c", "echo warning >&2; echo ok"},
			wantOut: "ok\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, err := execCommand(tt.command, true, tt.args...)
			if code != tt.wantCode || out != tt.wantOut {
				t.Errorf("got (%d, %q), want (%d, %q)", code, out, tt.wantCode, tt.wantOut)
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) != tt.wantExitErr {
				t.Errorf("err wraps *exec.ExitError = %v, want %v", !tt.wantExitErr, tt.wantExitErr)
			}
		})
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
