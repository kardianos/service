package service

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/template"
)

func isProcd() bool {
	if _, err := os.Stat("/etc/rc.common"); err == nil {
		return true
	}
	return false
}

type procd struct {
	i        Interface
	platform string
	*Config
}

func newProcdService(i Interface, platform string, c *Config) (Service, error) {
	s := &procd{
		i:        i,
		platform: platform,
		Config:   c,
	}

	return s, nil
}

func (s *procd) String() string {
	if len(s.DisplayName) > 0 {
		return s.DisplayName
	}
	return s.Name
}

func (s *procd) Platform() string {
	return s.platform
}

var errNoUserServiceProcd = errors.New("User services are not supported on Procd.")

func (s *procd) configPath() (cp string, err error) {
	if s.Option.bool(optionUserService, optionUserServiceDefault) {
		err = errNoUserServiceProcd
		return
	}
	cp = "/etc/init.d/" + s.Config.Name
	return
}

func (s *procd) template() *template.Template {
	customScript := s.Option.string(optionSysvScript, "")

	if customScript != "" {
		return template.Must(template.New("").Funcs(tf).Parse(customScript))
	} else {
		return template.Must(template.New("").Funcs(tf).Parse(procdScript))
	}
}

func (s *procd) Install() error {
	confPath, err := s.configPath()
	if err != nil {
		return err
	}
	_, err = os.Stat(confPath)
	if err == nil {
		return fmt.Errorf("Init already exists: %s", confPath)
	}

	f, err := os.Create(confPath)
	if err != nil {
		return err
	}
	defer f.Close()

	path, err := s.execPath()
	if err != nil {
		return err
	}

	var to = &struct {
		*Config
		Path string
	}{
		s.Config,
		path,
	}

	err = s.template().Execute(f, to)
	if err != nil {
		return err
	}

	if err = os.Chmod(confPath, 0755); err != nil {
		return err
	}
	os.Symlink(confPath, "/etc/rc.d/S50"+s.Name)

	return nil
}

func (s *procd) Uninstall() error {
	cp, err := s.configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(cp); err != nil {
		return err
	}
	return nil
}

func (s *procd) Logger(errs chan<- error) (Logger, error) {
	if system.Interactive() {
		return ConsoleLogger, nil
	}
	return s.SystemLogger(errs)
}
func (s *procd) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(s.Name, errs)
}

func (s *procd) Run() (err error) {
	err = s.i.Start(s)
	if err != nil {
		return err
	}

	s.Option.funcSingle(optionRunWait, func() {
		var sigChan = make(chan os.Signal, 3)
		signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)
		<-sigChan
	})()

	return s.i.Stop(s)
}

func (s *procd) Status() (Status, error) {
	return StatusUnknown, nil
}

func (s *procd) Start() error {
	cp, err := s.configPath()
	if err != nil {
		return err
	}
	return run(cp, "start")
}

func (s *procd) Stop() error {
	cp, err := s.configPath()
	if err != nil {
		return err
	}
	return run(cp, "stop")
}

func (s *procd) Restart() error {
	cp, err := s.configPath()
	if err != nil {
		return err
	}
	return run(cp, "restart")
}

const procdScript = `#!/bin/sh /etc/rc.common
# Copyright (C) 2008 OpenWrt.org

USE_PROCD=1
START=90

cmd="{{.Path}}{{range $key,$value :=.Arguments}}{{" "}}{{$value}}{{end}}"

name=$(basename $initscript)
pid_file="/var/run/$name.pid"
stdout_log="/var/log/$name.log"
stderr_log="/var/log/$name.err"

start_service() {
	procd_open_instance
	procd_set_param command $cmd
	procd_close_instance
}
stop_service() {
    rm -f $pid_file
}
restart() {
	stop
	start
}
`
