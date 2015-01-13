package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"text/template"
	"time"

	"bitbucket.org/kardianos/osext"
)

const maxPathSize = 32 * 1024

const version = "Darwin Launchd"

type darwinSystem struct{}

func (ls darwinSystem) String() string {
	return version
}

var system = darwinSystem{}

func isInteractive() (bool, error) {
	// TODO: The PPID of Launchd is 1. The PPid of a service process should match launchd's PID.
	return os.Getppid() != 1, nil
}

func newService(i Interface, c *Config) (*darwinLaunchdService, error) {
	s := &darwinLaunchdService{
		i:      i,
		Config: c,
	}

	var err error
	s.interactive, err = isInteractive()

	return s, err
}

type darwinLaunchdService struct {
	i Interface
	*Config

	interactive bool
}

func (s *darwinLaunchdService) String() string {
	if len(s.DisplayName) > 0 {
		return s.DisplayName
	}
	return s.Name
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

func (s *darwinLaunchdService) Interactive() bool {
	return s.interactive
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
func (s *darwinLaunchdService) Restart() error {
	err := s.Stop()
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return s.Start()
}

func (s *darwinLaunchdService) Run() error {
	var err error

	err = s.i.Start(s)
	if err != nil {
		return err
	}

	var sigChan = make(chan os.Signal, 3)

	signal.Notify(sigChan, os.Interrupt, os.Kill)

	<-sigChan

	return s.i.Stop(s)
}

func (s *darwinLaunchdService) Logger() (Logger, error) {
	if s.interactive {
		return ConsoleLogger, nil
	}
	return s.SystemLogger()
}
func (s *darwinLaunchdService) SystemLogger() (Logger, error) {
	return newSysLogger(s.Name)
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
