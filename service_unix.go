// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

// +build linux darwin solaris aix freebsd

package service

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log/syslog"
	"os/exec"
	"strings"
	"syscall"
)

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

func runCommand(command string, readStdout bool, arguments ...string) (exitStatus int, stdout string, err error) {
	var (
		cmd                    = exec.Command(command, arguments...)
		cmdErr                 = fmt.Errorf("exec `%s` failed", strings.Join(cmd.Args, " "))
		stdoutPipe, stderrPipe io.ReadCloser
	)
	if stderrPipe, err = cmd.StderrPipe(); err != nil {
		err = fmt.Errorf("%s to connect stderr pipe: %w", cmdErr, err)
		return
	}
	if readStdout {
		if stdoutPipe, err = cmd.StdoutPipe(); err != nil {
			err = fmt.Errorf("%s to connect stdout pipe: %w", cmdErr, err)
			return
		}
	}

	// Execute the command.
	if err = cmd.Start(); err != nil {
		err = fmt.Errorf("%s: %w", cmdErr, err)
		return
	}

	// Process command outputs.
	var (
		pipeErr   = fmt.Errorf("%s while attempting to read", cmdErr)
		stdoutErr = fmt.Errorf("%s from stdout", pipeErr)
		stderrErr = fmt.Errorf("%s from stderr", pipeErr)

		errBuffer, readErr = ioutil.ReadAll(stderrPipe)
		stderr             = strings.TrimSuffix(string(errBuffer), "\n")

		haveStdErr = len(stderr) != 0
	)

	// Always read stderr.
	if readErr != nil {
		err = fmt.Errorf("%s: %w", stderrErr, readErr)
		return
	}

	// Maybe read stdout.
	if readStdout {
		outBuffer, readErr := ioutil.ReadAll(stdoutPipe)
		if readErr != nil {
			err = fmt.Errorf("%s: %w", stdoutErr, readErr)
			return
		}
		stdout = string(outBuffer)
	}

	// Wait for command to finish.
	if runErr := cmd.Wait(); runErr != nil {
		var execErr *exec.ExitError
		if errors.As(runErr, &execErr) {
			if status, ok := execErr.Sys().(syscall.WaitStatus); ok {
				exitStatus = status.ExitStatus()
			}
		}
		err = fmt.Errorf("%w: %s", cmdErr, runErr)
		if haveStdErr {
			err = fmt.Errorf("%w with stderr: %s", err, stderr)
		}
		return
	}

	// Darwin: launchctl can fail with a zero exit status,
	// so stderr must be inspected.
	systemIsDarwin := command == "launchctl"
	if systemIsDarwin &&
		haveStdErr &&
		!strings.HasSuffix(stderr, "Operation now in progress") {
		err = fmt.Errorf("%w with stderr: %s", cmdErr, stderr)
	}

	return
}
