// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.package service

package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"text/template"
	"time"
)

func isUpstart() bool {
	if _, err := os.Stat("/sbin/upstart-udev-bridge"); err == nil {
		return true
	}
	return false
}

type upstart struct {
	i Interface
	*Config
}

func newUpstartService(i Interface, c *Config) (Service, error) {
	s := &upstart{
		i:      i,
		Config: c,
	}

	return s, nil
}

func (s *upstart) String() string {
	if len(s.DisplayName) > 0 {
		return s.DisplayName
	}
	return s.Name
}

// Upstart has some support for user services in graphical sessions.
// Due to the mix of actual support for user services over versions, just don't bother.
// Upstart will be replaced by systemd in most cases anyway.
var errNoUserServiceUpstart = errors.New("User services are not supported on Upstart.")

func (s *upstart) configPath() (cp string, err error) {
	if s.Config.UserService {
		err = errNoUserServiceUpstart
		return
	}
	cp = "/etc/init/" + s.Config.Name + ".conf"
	return
}
func (s *upstart) template() *template.Template {
	return template.Must(template.New("").Funcs(tf).Parse(upstartScript))
}

func (s *upstart) Install() error {
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

	return s.template().Execute(f, to)
}

func (s *upstart) Uninstall() error {
	cp, err := s.configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(cp); err != nil {
		return err
	}
	return nil
}

func (s *upstart) Logger(errs chan<- error) (Logger, error) {
	if system.Interactive() {
		return ConsoleLogger, nil
	}
	return s.SystemLogger(errs)
}
func (s *upstart) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(s.Name, errs)
}

func (s *upstart) Run() (err error) {
	err = s.i.Start(s)
	if err != nil {
		return err
	}

	sigChan := make(chan os.Signal, 3)

	signal.Notify(sigChan, os.Interrupt, os.Kill)

	<-sigChan

	return s.i.Stop(s)
}

func (s *upstart) Start() error {
	return exec.Command("initctl", "start", s.Name).Run()
}

func (s *upstart) Stop() error {
	return exec.Command("initctl", "stop", s.Name).Run()
}

func (s *upstart) Restart() error {
	err := s.Stop()
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return s.Start()
}

// The upstart script should stop with an INT or the Go runtime will terminate
// the program before the Stop handler can run.
const upstartScript = `# {{.Description}}

 {{if .DisplayName}}description    "{{.DisplayName}}"{{end}}

kill signal INT
start on filesystem or runlevel [2345]
stop on runlevel [!2345]

#setuid username

respawn
respawn limit 10 5
umask 022

console none

pre-start script
    test -x {{.Path}} || { stop; exit 0; }
end script

# Start
exec {{.Path}}{{range .Arguments}} {{.|cmd}}{{end}}
`
