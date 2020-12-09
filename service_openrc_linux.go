package service

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"text/template"
	"time"
)

func isOpenRC() bool {
	if _, err := exec.LookPath("openrc-init"); err == nil {
		return true
	}
	if _, err := os.Stat("/etc/inittab"); err == nil {
		filerc, err := os.Open("/etc/inittab")
		if err != nil {
			return false
		}
		defer filerc.Close()

		buf := new(bytes.Buffer)
		buf.ReadFrom(filerc)
		contents := buf.String()

		re := regexp.MustCompile(`::sysinit:.*openrc.*sysinit`)
		matches := re.FindStringSubmatch(contents)
		if len(matches) > 0 {
			return true
		}
		return false
	}
	return false
}

type openrc struct {
	i        Interface
	platform string
	*Config
}

func (s *openrc) String() string {
	if len(s.DisplayName) > 0 {
		return s.DisplayName
	}
	return s.Name
}

func (s *openrc) Platform() string {
	return s.platform
}

func (s *openrc) template() *template.Template {
	customScript := s.Option.string(optionOpenRCScript, "")

	if customScript != "" {
		return template.Must(template.New("").Funcs(tf).Parse(customScript))
	} else {
		return template.Must(template.New("").Funcs(tf).Parse(openRCScript))
	}
}

func newOpenRCService(i Interface, platform string, c *Config) (Service, error) {
	s := &openrc{
		i:        i,
		platform: platform,
		Config:   c,
	}
	return s, nil
}

var errNoUserServiceOpenRC = errors.New("User services are not supported on OpenRC.")

func (s *openrc) configPath() (cp string, err error) {
	if s.Option.bool(optionUserService, optionUserServiceDefault) {
		err = errNoUserServiceOpenRC
		return
	}
	cp = "/etc/init.d/" + s.Config.Name
	return
}

func (s *openrc) Install() error {
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
	// run rc-update
	return s.runAction("add")
}

func (s *openrc) Uninstall() error {
	confPath, err := s.configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(confPath); err != nil {
		return err
	}
	return s.runAction("delete")
}

func (s *openrc) Logger(errs chan<- error) (Logger, error) {
	if system.Interactive() {
		return ConsoleLogger, nil
	}
	return s.SystemLogger(errs)
}

func (s *openrc) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(s.Name, errs)
}

func (s *openrc) Run() (err error) {
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

func (s *openrc) Status() (Status, error) {
	_, out, err := runWithOutput("service", s.Name, "status")
	if err != nil {
		return StatusUnknown, err
	}

	switch {
	case strings.HasPrefix(out, "Running"):
		return StatusRunning, nil
	case strings.HasPrefix(out, "Stopped"):
		return StatusStopped, nil
	default:
		return StatusUnknown, ErrNotInstalled
	}
}

func (s *openrc) Start() error {
	return run("service", s.Name, "start")
}

func (s *openrc) Stop() error {
	return run("service", s.Name, "stop")
}

func (s *openrc) Restart() error {
	err := s.Stop()
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return s.Start()
}

func (s *openrc) runAction(action string) error {
	return s.run(action, s.Name)
}

/*
func (s *openrc) getOpenRCVersion() int64 {
	_, out, err := runWithOutput("systemctl", "--version")
	if err != nil {
		return -1
	}

	re := regexp.MustCompile(`systemd ([0-9]+)`)
	matches := re.FindStringSubmatch(out)
	if len(matches) != 2 {
		return -1
	}

	v, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return -1
	}

	return v
} */

func (s *openrc) run(action string, args ...string) error {
	return run("rc-update", append([]string{action}, args...)...)
}

const openRCScript = `#!/sbin/openrc-run
supervisor=supervise-daemon
name={{.DisplayName}}
description={{.Description}}
command={{.Path|cmdEscape}}{{range .Arguments}} {{.|cmd}}{{end}}
command_args={{.Path|cmdEscape}}{{range .Arguments}} {{.|cmd}}{{end}}

depend() {
#  (Dependency information)
{{range $i, $dep := .Dependencies}} 
{{$dep}} {{end}}
}
  
start() {
#  (Commands necessary to start the service)
}
  
stop() {
#  (Commands necessary to stop the service)
}
`