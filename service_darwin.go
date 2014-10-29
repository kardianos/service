package service

import (
	"fmt"
	"log/syslog"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"text/template"

	"bitbucket.org/kardianos/osext"
)

const maxPathSize = 32 * 1024

func newService(c *Config) (s *darwinLaunchdService, err error) {
	s = &darwinLaunchdService{
		Config: c,
	}

	s.logger, err = syslog.New(syslog.LOG_INFO, c.Name)
	if err != nil {
		return nil, err
	}

	return s, nil
}

type darwinLaunchdService struct {
	*Config
	logger *syslog.Writer
}

const version = "Darwin Launchd"

func (s *darwinLaunchdService) String() string {
	return version
}

func (s *darwinLaunchdService) getServiceFilePath() (string, error) {
	if s.UserService {
		u, err := user.Current()
		if err != nil {
			return "", err
		}
		return u.HomeDir + "/Library/LaunchAgents/" + s.Name + ".plist", nil
	}
	return "/Library/LaunchDaemons/" + s.Name + ".plist", nil
}

func (s *darwinLaunchdService) Install() error {
	confPath, err := s.getServiceFilePath()
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

	path, err := osext.Executable()
	if err != nil {
		return err
	}

	var to = &struct {
		*Config
		Path string

		KeepAlive, RunAtLoad bool
	}{
		Config:    s.Config,
		Path:      path,
		KeepAlive: s.KV.bool("KeepAlive", true),
		RunAtLoad: s.KV.bool("RunAtLoad", false),
	}

	functions := template.FuncMap{
		"bool": func(v bool) string {
			if v {
				return "true"
			}
			return "false"
		},
	}
	t := template.Must(template.New("launchdConfig").Funcs(functions).Parse(launchdConfig))
	return t.Execute(f, to)
}

func (s *darwinLaunchdService) Remove() error {
	s.Stop()

	confPath, err := s.getServiceFilePath()
	if err != nil {
		return err
	}
	return os.Remove(confPath)
}

func (s *darwinLaunchdService) Start() error {
	confPath, err := s.getServiceFilePath()
	if err != nil {
		return err
	}
	cmd := exec.Command("launchctl", "load", confPath)
	return cmd.Run()
}
func (s *darwinLaunchdService) Stop() error {
	confPath, err := s.getServiceFilePath()
	if err != nil {
		return err
	}
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
	// On Darwin syslog.log defaults to loggint >= Notice (see /etc/asl.conf).
	return s.logger.Notice(fmt.Sprintf(format, a...))
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
<key>KeepAlive</key><{{bool .KeepAlive}}/>
<key>RunAtLoad</key><{{bool .RunAtLoad}}/>
<key>Disabled</key><false/>
</dict>
</plist>
`
