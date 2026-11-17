// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

//go:build linux || darwin || solaris || aix || freebsd
// +build linux darwin solaris aix freebsd

package service

import (
	"bytes"
	"errors"
	"fmt"
	"log/syslog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

const defaultLogDirectory = "/var/log"

func newSysLogger(name string, errs chan<- error) (Logger, error) {
	w, err := syslog.New(syslog.LOG_INFO, name)
	if err != nil {
		return nil, err
	}
	return sysLogger{w, errs}, nil
}

type sysLogger struct {
	*syslog.Writer
	errs chan<- error
}

func (s sysLogger) send(err error) error {
	if err != nil && s.errs != nil {
		s.errs <- err
	}
	return err
}

func (s sysLogger) Error(v ...interface{}) error {
	return s.send(s.Writer.Err(fmt.Sprint(v...)))
}
func (s sysLogger) Warning(v ...interface{}) error {
	return s.send(s.Writer.Warning(fmt.Sprint(v...)))
}
func (s sysLogger) Info(v ...interface{}) error {
	return s.send(s.Writer.Info(fmt.Sprint(v...)))
}
func (s sysLogger) Errorf(format string, a ...interface{}) error {
	return s.send(s.Writer.Err(fmt.Sprintf(format, a...)))
}
func (s sysLogger) Warningf(format string, a ...interface{}) error {
	return s.send(s.Writer.Warning(fmt.Sprintf(format, a...)))
}
func (s sysLogger) Infof(format string, a ...interface{}) error {
	return s.send(s.Writer.Info(fmt.Sprintf(format, a...)))
}

func run(command string, arguments ...string) error {
	_, _, err := runCommand(command, false, arguments...)
	return err
}

func runWithOutput(command string, arguments ...string) (int, string, error) {
	return runCommand(command, true, arguments...)
}

// runCommand runs an external command. Replaced in tests.
var runCommand = execCommand

// isPrivileged reports whether the process runs as root. Replaced in tests.
var isPrivileged = func() bool { return os.Geteuid() == 0 } //nolint:unused // Unused on freebsd, solaris and aix.

func (c *Config) userServiceLevel() (ServiceLevel, error) {
	// fallback to the deprecated UserService level if ServiceLevel isn't defined
	if c.ServiceLevel == "" && c.Option.bool(optionUserService, optionUserServiceDefault) {
		return ServiceLevelUser, nil
	}
	return c.serviceLevel()
}

func (c *Config) requireSystemLevel() error {
	level, err := c.serviceLevel() // defaults to system if unset
	if err != nil {
		return err
	}
	if level != ServiceLevelSystem {
		return ErrUnsupportedServiceLevel
	}
	return nil
}

// forEachUser runs f for every user concurrently
func forEachUser[U any](users []U, label func(U) string, f func(U) error) error { //nolint:unused // Unused on freebsd, solaris and aix.
	errs := make([]error, len(users))
	var wg sync.WaitGroup
	for i, u := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(u); err != nil {
				errs[i] = fmt.Errorf("%s: %w", label(u), err)
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func execCommand(command string, readStdout bool, arguments ...string) (int, string, error) {
	cmd := exec.Command(command, arguments...)

	var stdout bytes.Buffer
	if readStdout {
		cmd.Stdout = &stdout
	}

	stderr := &cappedBuffer{max: maxStderr}
	cmd.Stderr = stderr

	err := cmd.Run()
	slurp := stderr.buf.Bytes()

	// launchctl can fail with a zero exit status, so non-empty stderr is the
	// only sign of failure.
	if command == "launchctl" && len(slurp) > 0 && !bytes.HasSuffix(slurp, []byte("Operation now in progress\n")) {
		exitStatus, _ := isExitError(err)
		return exitStatus, "", fmt.Errorf("%q failed with stderr: %s", command, slurp)
	}

	output := stdout.String()
	if err == nil {
		return 0, output, nil
	}
	exitStatus, ok := isExitError(err)
	if !ok {
		return 0, output, fmt.Errorf("%q failed: %v", command, err)
	}
	if msg := strings.TrimSpace(string(slurp)); msg != "" {
		err = fmt.Errorf("%w: %s", err, msg)
	}
	return exitStatus, output, err
}

const maxStderr = 16 << 10

// cappedBuffer drops writes past max but reports them as written, so the
// command never sees a short write.
type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func isExitError(err error) (int, bool) {
	if exiterr, ok := err.(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), true
		}
	}

	return 0, false
}
