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

const maxPathSize = 32 * 1024

func newService(name, displayName, description string) (s *darwinLaunchdService, err error) {
	s = &darwinLaunchdService{
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

type darwinLaunchdService struct {
	name, displayName, description string
	logger                         *syslog.Writer
}

func (s *darwinLaunchdService) getServiceFilePath() string {
	return "/Library/LaunchDaemons/" + s.name + ".plist"
}

func (s *darwinLaunchdService) Install() error {
	var confPath = s.getServiceFilePath()
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
		Name        string
		Display     string
		Description string
		Path        string
	}{
		s.name,
		s.displayName,
		s.description,
		path,
	}

	t := template.Must(template.New("launchdConfig").Parse(launchdConfig))
	return t.Execute(f, to)
}

func (s *darwinLaunchdService) Remove() error {
	s.Stop()

	confPath := s.getServiceFilePath()
	return os.Remove(confPath)
}

func (s *darwinLaunchdService) Start() error {
	confPath := s.getServiceFilePath()
	cmd := exec.Command("launchctl", "load", confPath)
	return cmd.Run()
}
func (s *darwinLaunchdService) Stop() error {
	confPath := s.getServiceFilePath()
	cmd := exec.Command("launchctl", "unload", confPath)
	return cmd.Run()
}

func (s *darwinLaunchdService) Run(onStart, onStop func() error) error {
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

func (s *darwinLaunchdService) Error(format string, a ...interface{}) error {
	return s.logger.Err(fmt.Sprintf(format, a...))
}
func (s *darwinLaunchdService) Warning(format string, a ...interface{}) error {
	return s.logger.Warning(fmt.Sprintf(format, a...))
}
func (s *darwinLaunchdService) Info(format string, a ...interface{}) error {
	return s.logger.Info(fmt.Sprintf(format, a...))
}

var launchdConfig = `<?xml version='1.0' encoding='UTF-8'?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN"
"http://www.apple.com/DTDs/PropertyList-1.0.dtd" >
<plist version='1.0'>
<dict>
<key>Label</key><string>{{.Name}}</string>
<key>ProgramArguments</key>
<array>
        <string>{{.Path}}</string>
</array>
<key>KeepAlive</key><true/>
<key>Disabled</key><false/>
</dict>
</plist>
`
