package service

/*
#include <mach-o/dyld.h>
#include <string.h>

int
GetExecPath(char* path) {
	uint32_t size = 32*1024;
	if (_NSGetExecutablePath(path, &size) == 0) {
		// Despite Apple docs, size does NOT get set in call.
		return strlen(path);
	} else {
		return 0;
	}
}
*/
import "C"

import (
	"unsafe"
	"errors"
	"fmt"
	"log/syslog"
	"os"
	"os/exec"
	"os/signal"
	"text/template"
)

// BUG(kardia): I have not confirmed this service works as intended on launchd.
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

	path, err := getExePath()
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

	t := template.Must(template.New("upstartScript").Parse(upstartScript))
	err = t.Execute(f, to)

	if err != nil {
		return err
	}

	cmd := exec.Command("launchctl", "load", confPath)
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func (s *darwinLaunchdService) Remove() error {
	confPath := s.getServiceFilePath()
	cmd := exec.Command("launchctl", "unload", confPath)
	err := cmd.Run()

	// Don't worry about his error if not found.
	os.Remove(confPath)
	return err
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

func (s *darwinLaunchdService) LogError(format string, a ...interface{}) error {
	return s.logger.Err(fmt.Sprintf(format, a...))
}
func (s *darwinLaunchdService) LogWarning(format string, a ...interface{}) error {
	return s.logger.Warning(fmt.Sprintf(format, a...))
}
func (s *darwinLaunchdService) LogInfo(format string, a ...interface{}) error {
	return s.logger.Info(fmt.Sprintf(format, a...))
}

func getExePath() (exePath string, err error) {
	buffer := make([]byte, 32*1024)
	size := C.GetExecPath((*C.char)(unsafe.Pointer(&buffer[0])))
	if size == 0 {
		return "", errors.New("Unable to get exec path.")
	}
	buffer = buffer[:size]
	ret := string(buffer)
	return ret, nil
}

var upstartScript = `<?xml version='1.0' encoding='UTF-8'?>
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
