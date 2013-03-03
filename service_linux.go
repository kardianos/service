package service

import (
	"bitbucket.org/kardianos/osext"
	"fmt"
	"log/syslog"
	"os"
	"os/exec"
	"os/signal"
	"text/template"
)

func newService(name, displayName, description string) (s *linuxUpstartService, err error) {
	s = &linuxUpstartService{
		name:        name,
		displayName: displayName,
		description: description,
	}

	s.logger, err = syslog.New(syslog.LOG_INFO, name)
	if err != nil {
		return nil, err
	}

	return s, nil
}

type linuxUpstartService struct {
	name, displayName, description string
	logger                         *syslog.Writer
}

func (s *linuxUpstartService) Install() error {
	var confPath = "/etc/init/" + s.name + ".conf"
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

	t := template.Must(template.New("upstartScript").Parse(upstartScript))
	err = t.Execute(f, to)

	if err != nil {
		return err
	}

	return nil
}

func (s *linuxUpstartService) Remove() error {
	return os.Remove("/etc/init/" + s.name + ".conf")
}

func (s *linuxUpstartService) Run(onStart, onStop func() error) error {
	var err error

	err = onStart()
	if err != nil {
		return err
	}

	var sigChan = make(chan os.Signal, 3)

	signal.Notify(sigChan, os.Interrupt, os.Kill)

	<-sigChan

	return onStop()
}

func (s *linuxUpstartService) Start() error {
	cmd := exec.Command("start", s.name)
	return cmd.Run()
}

func (s *linuxUpstartService) Stop() error {
	cmd := exec.Command("stop", s.name)
	return cmd.Run()
}

func (s *linuxUpstartService) Error(format string, a ...interface{}) error {
	return s.logger.Err(fmt.Sprintf(format, a...))
}
func (s *linuxUpstartService) Warning(format string, a ...interface{}) error {
	return s.logger.Warning(fmt.Sprintf(format, a...))
}
func (s *linuxUpstartService) Info(format string, a ...interface{}) error {
	return s.logger.Info(fmt.Sprintf(format, a...))
}

var upstartScript = `# {{.Description}}

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
