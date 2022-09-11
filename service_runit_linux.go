package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"text/template"
	"time"
)

func isRunit() bool {
	if _, err := exec.LookPath("runit"); err == nil {
		return true
	}
	return false
}

type runit struct {
	i        Interface
	platform string
	*Config
}

func (r *runit) String() string {
	if len(r.DisplayName) > 0 {
		return r.DisplayName
	}
	return r.Name
}

func (r *runit) Platform() string {
	return r.platform
}

func (r *runit) template() *template.Template {
	customScript := r.Option.string(optionRunitScript, "")

	if customScript != "" {
		return template.Must(template.New("").Funcs(tf).Parse(customScript))
	}
	return template.Must(template.New("").Funcs(tf).Parse(runitScript))
}

func newRunitService(i Interface, platform string, c *Config) (Service, error) {
	s := &runit{
		i:        i,
		platform: platform,
		Config:   c,
	}
	return s, nil
}

var errNoUserServiceRunit = errors.New("user services are not supported on runit")

func (r *runit) configPath() (cp string, err error) {
	if r.Option.bool(optionUserService, optionUserServiceDefault) {
		err = errNoUserServiceRunit
		return
	}
	cp = "/etc/sv/" + r.Config.Name
	return
}

func (r *runit) Install() error {
	confPath, err := r.configPath()
	if err != nil {
		return err
	}
	_, err = os.Stat(confPath)
	if err == nil {
		return fmt.Errorf("Init already exists: %s", confPath)
	}

	if err := os.MkdirAll(confPath, 0755); err != nil {
		return err
	}

	logpath := path.Join(confPath, "log")

	if err := os.MkdirAll(logpath, 0755); err != nil {
		return err
	}

	if err := os.Symlink(path.Join(logpath, "run"), "/usr/bin/vlogger"); err != nil {
		return err
	}

	f, err := os.Create(path.Join(confPath, "run"))
	if err != nil {
		return err
	}
	defer f.Close()

	execPath, err := r.execPath()
	if err != nil {
		return err
	}

	var to = &struct {
		Path string
	}{
		Path: execPath,
	}

	err = r.template().Execute(f, to)
	if err != nil {
		return err
	}

	return nil
}

func (r *runit) Uninstall() error {
	confPath, err := r.configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(confPath); err != nil {
		return err
	}
	return r.runAction("delete")
}

func (r *runit) Logger(errs chan<- error) (Logger, error) {
	if system.Interactive() {
		return ConsoleLogger, nil
	}
	return r.SystemLogger(errs)
}

func (r *runit) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(r.Name, errs)
}

func (r *runit) Run() (err error) {
	err = r.i.Start(r)
	if err != nil {
		return err
	}

	r.Option.funcSingle(optionRunWait, func() {
		var sigChan = make(chan os.Signal, 3)
		signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)
		<-sigChan
	})()

	return r.i.Stop(r)
}

var errStatusUndefinedRunit = errors.New("undefined status of the service")

func (r *runit) Status() (Status, error) {
	_, out, err := runWithOutput("sv", "status", r.Name)
	if err != nil {
		return StatusUnknown, err
	}

	if strings.HasPrefix(out, "run:") {
		return StatusRunning, nil
	} else if strings.HasPrefix(out, "down:") {
		return StatusStopped, nil
	} else {
		return StatusUnknown, errStatusUndefinedRunit
	}
}

func (r *runit) ensureLinked() error {
	confPath, err := r.configPath()
	if err != nil {
		return err
	}

	_, err = os.Stat(confPath)
	if err != nil {
		if err := os.Symlink(confPath, path.Join("/etc/service", r.Name)); err != nil {
			return err
		}
	}

	return nil
}

func (r *runit) Start() error {
	if err := r.ensureLinked(); err != nil {
		return nil
	}

	return run("sv", "up", r.Name)
}

func (r *runit) Stop() error {
	return run("sv", "down", r.Name)
}

func (r *runit) Restart() error {
	err := r.Stop()
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return r.Start()
}

func (r *runit) run(action string, args ...string) error {
	return run("sv", append([]string{action}, args...)...)
}

func (r *runit) runAction(action string) error {
	return r.run(action, r.Name)
}

const runitScript = `#!/bin/sh

exec {{.Path}} 2>&1
`
