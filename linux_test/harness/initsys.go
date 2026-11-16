package main

import (
	"bytes"
	"context"
	"strings"
	"time"
)

type InitSystem struct {
	Name        string
	Boot        []string
	Platform    string
	Ready       func(ctx context.Context, rt Runtime, name string) (bool, string)
	Version     []string
	FailureLogs [][]string
}

var systemd = &InitSystem{
	Name:     "systemd",
	Boot:     []string{"/sbin/init"},
	Platform: "linux-systemd",
	Ready: func(ctx context.Context, rt Runtime, name string) (bool, string) {
		var out bytes.Buffer
		if _, err := rt.Exec(ctx, name, nil, &out, "systemctl", "is-system-running"); err != nil {
			return false, err.Error()
		}
		state := strings.TrimSpace(out.String())
		// Some units always fail in a container, so "degraded" is also ready.
		return state == "running" || state == "degraded", state
	},
	Version:     []string{"systemctl", "--version"},
	FailureLogs: [][]string{{"journalctl", "--no-pager", "-n", "300"}},
}

var initSystems = []*InitSystem{systemd}

func waitReady(ctx context.Context, rt Runtime, name string, init *InitSystem, timeout, poll time.Duration) (bool, string) {
	deadline := time.Now().Add(timeout)
	state := "none"
	for {
		pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		ready, s := init.Ready(pollCtx, rt, name)
		cancel()
		if s != "" {
			state = s
		}
		if ready {
			return true, state
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			return false, state
		}
		select {
		case <-ctx.Done():
			return false, state
		case <-time.After(poll):
		}
	}
}

func initVersion(ctx context.Context, rt Runtime, name string, init *InitSystem) string {
	if len(init.Version) == 0 {
		return ""
	}
	var out bytes.Buffer
	if code, err := rt.Exec(ctx, name, nil, &out, init.Version...); err != nil || code != 0 {
		return ""
	}
	line, _, _ := strings.Cut(out.String(), "\n")
	return strings.TrimSpace(line)
}
