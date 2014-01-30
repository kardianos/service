package service

import (
	"fmt"
	"log/syslog"
	"os"
	"os/exec"
	"os/signal"
	"text/template"

	"bitbucket.org/kardianos/osext"
)

const (
	initUpstart = initFlavor(iota)
	initSystemd
)

func newService(name, displayName, description string) (Service, error) {
	flavor := initUpstart
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		flavor = initSystemd
	}
	s := &linuxService{
		flavor:      flavor,
		name:        name,
		displayName: displayName,
		description: description,
	}

	var err error
	s.logger, err = syslog.New(syslog.LOG_INFO, name)
	if err != nil {
		return nil, err
	}

	return s, nil
}

type linuxService struct {
	flavor                         initFlavor
	name, displayName, description string
	logger                         *syslog.Writer
}

type initFlavor uint8

func (f initFlavor) String() string {
	switch f {
	case initUpstart:
		return "Upstart"
	case initSystemd:
		return "systemd"
	default:
		return "unknown"
	}
}

func (f initFlavor) ConfigPath(name string) string {
	switch f {
	case initUpstart:
		return "/etc/init/" + name + ".conf"
	case initSystemd:
		return "/etc/systemd/system/" + name + ".service"
	default:
		return ""
	}
}

func (f initFlavor) GetTemplate() *template.Template {
	var templ string
	switch f {
	case initUpstart:
		templ = upstartScript
	case initSystemd:
		templ = systemdScript
	}
	return template.Must(template.New(f.String() + "Script").Parse(templ))
}

func (s *linuxService) Install() error {
	confPath := s.flavor.ConfigPath(s.name)
	_, err := os.Stat(confPath)
	if err == nil {
		return fmt.Errorf("Init already exists: %s", confPath)
	}

	f, err := os.Create(confPath)
	if err != nil {
		return err
	}
	defer f.Close()

	path, err := osext.Executable()
	if err != nil {
		return err
	}

	var to = &struct {
		Display     string
		Description string
		Path        string
	}{
		s.displayName,
		s.description,
		path,
	}

	err = s.flavor.GetTemplate().Execute(f, to)

	if err != nil {
		return err
	}

	if s.flavor == initSystemd {
		return exec.Command("systemctl", "daemon-reload").Run()
	}

	return nil
}

func (s *linuxService) Remove() error {
	if s.flavor == initSystemd {
		exec.Command("systemctl", "disable", s.name+".service").Run()
	}
	if err := os.Remove(s.flavor.ConfigPath(s.name)); err != nil {
		return err
	}
	return nil
}

func (s *linuxService) Run(onStart, onStop func() error) (err error) {
	err = onStart()
	if err != nil {
		return err
	}
	defer func() {
		err = onStop()
	}()

	sigChan := make(chan os.Signal, 3)

	signal.Notify(sigChan, os.Interrupt, os.Kill)

	<-sigChan

	return nil
}

func (s *linuxService) Start() error {
	return exec.Command("service", s.name, "start").Run()
}

func (s *linuxService) Stop() error {
	return exec.Command("service", s.name, "stop").Run()
}

func (s *linuxService) Error(format string, a ...interface{}) error {
	return s.logger.Err(fmt.Sprintf(format, a...))
}
func (s *linuxService) Warning(format string, a ...interface{}) error {
	return s.logger.Warning(fmt.Sprintf(format, a...))
}
func (s *linuxService) Info(format string, a ...interface{}) error {
	return s.logger.Info(fmt.Sprintf(format, a...))
}

const upstartScript = `# {{.Description}}

description     "{{.Display}}"

start on filesystem or runlevel [2345]
stop on runlevel [!2345]

#setuid username

kill signal INT

respawn
respawn limit 10 5
umask 022

console none

pre-start script
    test -x {{.Path}} || { stop; exit 0; }
end script

# Start
exec {{.Path}}
`

const systemdScript = `[Unit]
Description={{.Description}}
ConditionFileIsExecutable={{.Path}}

[Service]
StartLimitInterval=5
StartLimitBurst=10
ExecStart={{.Path}}

[Install]
WantedBy=multi-user.target
`
