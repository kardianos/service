// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestIsRunningFromLaunchctlOutput(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
	}{
		{
			name: "running process with pid",
			out: `{
	"LimitLoadToSessionType" = "System";
	"Label" = "com.example.service";
	"OnDemand" = false;
	"LastExitStatus" = 0;
	"PID" = 11444;
};`,
			want: true,
		},
		{
			name: "stopped process without pid",
			out: `{
	"LimitLoadToSessionType" = "Aqua";
	"Label" = "com.example.agent";
	"OnDemand" = true;
	"LastExitStatus" = 0;
};`,
			want: false,
		},
		{
			name: "empty output",
			out:  "",
			want: false,
		},
		{
			name: "minimal pid match",
			out:  `"PID" = 42;`,
			want: true,
		},
		{
			name: "pid with different key",
			out:  `"LastPID" = 42;`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRunningFromLaunchctlOutput(tt.out)
			if got != tt.want {
				t.Errorf("isRunningFromLaunchctlOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLaunchctlPermissionError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "permission denied lowercase",
			err:  errors.New("launchctl: permission denied"),
			want: true,
		},
		{
			name: "permission denied titlecase",
			err:  errors.New("Permission denied"),
			want: true,
		},
		{
			name: "not privileged error",
			err:  errors.New("Not privileged to perform operation"),
			want: true,
		},
		{
			name: "operation not permitted",
			err:  errors.New("Operation not permitted"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("Could not find service"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLaunchctlPermissionError(tt.err)
			if got != tt.want {
				t.Errorf("isLaunchctlPermissionError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLaunchctlNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "standard domain for port error",
			err:  errors.New("Could not find service \"ctrld\" in domain for port"),
			want: true,
		},
		{
			name: "standard domain for system error",
			err:  errors.New("Could not find service \"ctrld\" in domain for system"),
			want: true,
		},
		{
			name: "specified service error from launchctl code 113",
			err:  errors.New("113: Could not find specified service"),
			want: true,
		},
		{
			name: "failed with stderr wrapper",
			err:  errors.New("\"launchctl\" failed with stderr: Could not find service \"foo\" in domain for port\n"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("exec format error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLaunchctlNotFoundError(tt.err)
			if got != tt.want {
				t.Errorf("isLaunchctlNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseLaunchctlStatus(t *testing.T) {
	errStatPermission := errors.New("stat permission denied")
	errExit113Port := errors.New("\"launchctl\" failed with stderr: Could not find service \"ctrld\" in domain for port\n")
	errExit113System := errors.New("\"launchctl\" failed with stderr: Could not find service \"ctrld\" in domain for system\n")
	errSpecifiedService := errors.New("113: Could not find specified service")
	errPermDenied := errors.New("Permission denied")
	errNotPrivileged := errors.New("Not privileged to perform operation")
	errGeneric := errors.New("internal daemon failure")

	runningOutput := "{\n\t\"Label\" = \"my-service\";\n\t\"PID\" = 11444;\n};"
	stoppedOutput := "{\n\t\"Label\" = \"my-service\";\n\t\"LastExitStatus\" = 0;\n};"

	tests := []struct {
		name        string
		userService bool
		isRoot      bool
		exitCode    int
		out         string
		runErr      error
		confPathErr error
		wantStatus  Status
		wantErr     error
		wantErrSub  string
	}{
		{
			name:        "system daemon running (root query)",
			userService: false,
			isRoot:      true,
			exitCode:    0,
			out:         runningOutput,
			runErr:      nil,
			confPathErr: nil,
			wantStatus:  StatusRunning,
			wantErr:     nil,
		},
		{
			name:        "system daemon running (unprivileged query with success output)",
			userService: false,
			isRoot:      false,
			exitCode:    0,
			out:         runningOutput,
			runErr:      nil,
			confPathErr: nil,
			wantStatus:  StatusRunning,
			wantErr:     nil,
		},
		{
			name:        "user agent running",
			userService: true,
			isRoot:      false,
			exitCode:    0,
			out:         runningOutput,
			runErr:      nil,
			confPathErr: nil,
			wantStatus:  StatusRunning,
			wantErr:     nil,
		},
		{
			name:        "system daemon loaded but stopped, plist exists",
			userService: false,
			isRoot:      true,
			exitCode:    0,
			out:         stoppedOutput,
			runErr:      nil,
			confPathErr: nil,
			wantStatus:  StatusStopped,
			wantErr:     nil,
		},
		{
			name:        "system daemon loaded but stopped, plist missing",
			userService: false,
			isRoot:      true,
			exitCode:    0,
			out:         stoppedOutput,
			runErr:      nil,
			confPathErr: os.ErrNotExist,
			wantStatus:  StatusUnknown,
			wantErr:     ErrNotInstalled,
		},
		{
			name:        "system daemon loaded but stopped, stat error",
			userService: false,
			isRoot:      true,
			exitCode:    0,
			out:         stoppedOutput,
			runErr:      nil,
			confPathErr: errStatPermission,
			wantStatus:  StatusUnknown,
			wantErr:     errStatPermission,
		},
		{
			name:        "issue 400: system daemon queried unprivileged with domain for port error",
			userService: false,
			isRoot:      false,
			exitCode:    0,
			out:         "",
			runErr:      errExit113Port,
			confPathErr: nil,
			wantStatus:  StatusUnknown,
			wantErr:     errExit113Port,
		},
		{
			name:        "system daemon queried with domain for port error even if isRoot is true",
			userService: false,
			isRoot:      true,
			exitCode:    0,
			out:         "",
			runErr:      errExit113Port,
			confPathErr: nil,
			wantStatus:  StatusUnknown,
			wantErr:     errExit113Port,
		},
		{
			name:        "system daemon queried unprivileged without domain suffix",
			userService: false,
			isRoot:      false,
			exitCode:    0,
			out:         "",
			runErr:      errSpecifiedService,
			confPathErr: nil,
			wantStatus:  StatusUnknown,
			wantErr:     errSpecifiedService,
		},
		{
			name:        "system daemon stopped/unloaded (root query), plist exists",
			userService: false,
			isRoot:      true,
			exitCode:    0,
			out:         "",
			runErr:      errExit113System,
			confPathErr: nil,
			wantStatus:  StatusStopped,
			wantErr:     nil,
		},
		{
			name:        "system daemon unloaded (root query), plist missing",
			userService: false,
			isRoot:      true,
			exitCode:    0,
			out:         "",
			runErr:      errExit113System,
			confPathErr: os.ErrNotExist,
			wantStatus:  StatusUnknown,
			wantErr:     ErrNotInstalled,
		},
		{
			name:        "system daemon unloaded (root query), plist stat error",
			userService: false,
			isRoot:      true,
			exitCode:    0,
			out:         "",
			runErr:      errExit113System,
			confPathErr: errStatPermission,
			wantStatus:  StatusUnknown,
			wantErr:     errStatPermission,
		},
		{
			name:        "user agent stopped/unloaded (user query in domain for port), plist exists",
			userService: true,
			isRoot:      false,
			exitCode:    0,
			out:         "",
			runErr:      errExit113Port,
			confPathErr: nil,
			wantStatus:  StatusStopped,
			wantErr:     nil,
		},
		{
			name:        "user agent unloaded, plist missing",
			userService: true,
			isRoot:      false,
			exitCode:    0,
			out:         "",
			runErr:      errExit113Port,
			confPathErr: os.ErrNotExist,
			wantStatus:  StatusUnknown,
			wantErr:     ErrNotInstalled,
		},
		{
			name:        "permission denied error returns StatusUnknown",
			userService: false,
			isRoot:      false,
			exitCode:    1,
			out:         "",
			runErr:      errPermDenied,
			confPathErr: nil,
			wantStatus:  StatusUnknown,
			wantErr:     errPermDenied,
		},
		{
			name:        "not privileged error returns StatusUnknown",
			userService: false,
			isRoot:      false,
			exitCode:    1,
			out:         "",
			runErr:      errNotPrivileged,
			confPathErr: nil,
			wantStatus:  StatusUnknown,
			wantErr:     errNotPrivileged,
		},
		{
			name:        "generic error returns StatusUnknown",
			userService: false,
			isRoot:      true,
			exitCode:    1,
			out:         "",
			runErr:      errGeneric,
			confPathErr: nil,
			wantStatus:  StatusUnknown,
			wantErr:     errGeneric,
		},
		{
			name:        "non-zero exit code with nil runErr returns synthesized error",
			userService: false,
			isRoot:      true,
			exitCode:    5,
			out:         "",
			runErr:      nil,
			confPathErr: nil,
			wantStatus:  StatusUnknown,
			wantErrSub:  "code 5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := parseLaunchctlStatus(
				tt.userService,
				tt.isRoot,
				tt.exitCode,
				tt.out,
				tt.runErr,
				tt.confPathErr,
			)

			if status != tt.wantStatus {
				t.Errorf("parseLaunchctlStatus() status = %v, want %v", status, tt.wantStatus)
			}

			if tt.wantErr != nil {
				if err == nil || !errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error() {
					t.Errorf("parseLaunchctlStatus() err = %v, want %v", err, tt.wantErr)
				}
			} else if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("parseLaunchctlStatus() err = %v, want substring %q", err, tt.wantErrSub)
				}
			} else if err != nil {
				t.Errorf("parseLaunchctlStatus() unexpected err = %v", err)
			}
		})
	}
}

type dummyProgram struct{}

func (dummyProgram) Start(s Service) error { return nil }
func (dummyProgram) Stop(s Service) error  { return nil }

func TestDarwinStatusUnprivilegedSystemDaemon(t *testing.T) {
	// Verify that querying a system service when unprivileged does not return StatusStopped
	// when launchctl output reports "Could not find service ... in domain for port".
	cfg := &Config{
		Name: "test-system-daemon",
	}
	s, err := New(&dummyProgram{}, cfg)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	darwinSvc, ok := s.(*darwinLaunchdService)
	if !ok {
		t.Fatalf("expected *darwinLaunchdService, got %T", s)
	}

	simulatedErr := fmt.Errorf("\"launchctl\" failed with stderr: Could not find service \"%s\" in domain for port\n", cfg.Name)
	status, err := parseLaunchctlStatus(darwinSvc.userService, false, 0, "", simulatedErr, nil)
	if status == StatusStopped {
		t.Errorf("expected status != StatusStopped for unprivileged query, got %v", status)
	}
	if status != StatusUnknown {
		t.Errorf("expected status == StatusUnknown, got %v", status)
	}
	if err == nil {
		t.Errorf("expected non-nil error, got nil")
	}
}
