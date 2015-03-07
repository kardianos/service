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
	"strings"
	"text/template"
	"time"

	"github.com/kardianos/osext"
)

const (
	initSystemV = initFlavor(iota)
	initUpstart
	initSystemd
	initNotAvailable
)

func getFlavor() initFlavor {
	flavor := initNotAvailable
	switch {
	// XXX: enable when systemd bug is fixed
	//case isSystemd():
	//	flavor = initSystemd
	case isUpstart():
		flavor = initUpstart
	}
	return flavor
}

func isUpstart() bool {
	if _, err := os.Stat("/sbin/upstart-udev-bridge"); err == nil {
		return true
	}
	return false
}

func isSystemd() bool {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}
	return false
}

type linuxService struct {
	i Interface
	*Config

	interactive bool
}

var flavor = getFlavor()

type linuxSystem struct{}

func (ls linuxSystem) String() string {
	return fmt.Sprintf("Linux %s", flavor.String())
}

func (ls linuxSystem) Interactive() bool {
	return interactive
}

var system = linuxSystem{}

func newService(i Interface, c *Config) (Service, error) {
	s := &linuxService{
		i:      i,
		Config: c,
	}
	var err error
	s.interactive, err = isInteractive()

	return s, err
}

func (s *linuxService) String() string {
	if len(s.DisplayName) > 0 {
		return s.DisplayName
	}
	return s.Name
}

type initFlavor uint8

func (f initFlavor) String() string {
	switch f {
	case initSystemV:
		return "System-V"
	case initUpstart:
		return "Upstart"
	case initSystemd:
		return "systemd"
	default:
		panic("Invalid flavor")
	}
}

// Systemd services should be supported, but are not currently.
var errNoUserServiceSystemd = errors.New("User services are not supported on systemd.")

var errNoUserServiceSystemV = errors.New("User services are not supported on SystemV.")

// Upstart has some support for user services in graphical sessions.
// Due to the mix of actual support for user services over versions, just don't bother.
// Upstart will be replaced by systemd in most cases anyway.
var errNoUserServiceUpstart = errors.New("User services are not supported on Upstart.")

func (f initFlavor) ConfigPath(name string, c *Config) (cp string, err error) {
	if c.UserService {
		switch f {
		case initSystemd:
			err = errNoUserServiceSystemd
		case initSystemV:
			err = errNoUserServiceSystemV
		case initUpstart:
			err = errNoUserServiceUpstart
		default:
			panic("Invalid flavor")
		}
		return
	}
	switch f {
	case initSystemd:
		cp = "/etc/systemd/system/" + name + ".service"
	case initSystemV:
		cp = "/etc/init.d/" + name
	case initUpstart:
		cp = "/etc/init/" + name + ".conf"
	default:
		panic("Invalid flavor")
	}
	return
}

func (f initFlavor) Template() *template.Template {
	var templ string
	switch f {
	case initSystemd:
		templ = systemdScript
	case initSystemV:
		templ = systemVScript
	case initUpstart:
		templ = upstartScript
	}
	return template.Must(template.New(f.String() + "Script").Funcs(tf).Parse(templ))
}

var interactive = false

func init() {
	var err error
	interactive, err = isInteractive()
	if err != nil {
		panic(err)
	}
}

func isInteractive() (bool, error) {
	// TODO: This is not true for user services.
	return os.Getppid() != 1, nil
}

func (s *linuxService) Install() error {
	if ok, err := notSupported(); ok {
		return err
	}

	confPath, err := flavor.ConfigPath(s.Name, s.Config)
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
		Display     string
		Description string
		Path        string
		Arguments   []string
	}{
		s.DisplayName,
		s.Description,
		path,
		s.Config.Arguments,
	}

	err = flavor.Template().Execute(f, to)
	if err != nil {
		return err
	}

	if flavor == initSystemV {
		if err = os.Chmod(confPath, 0755); err != nil {
			return err
		}
		for _, i := range [...]string{"2", "3", "4", "5"} {
			if err = os.Symlink(confPath, "/etc/rc"+i+".d/S50"+s.Name); err != nil {
				continue
			}
		}
		for _, i := range [...]string{"0", "1", "6"} {
			if err = os.Symlink(confPath, "/etc/rc"+i+".d/K02"+s.Name); err != nil {
				continue
			}
		}
	}

	if flavor == initSystemd {
		err = exec.Command("systemctl", "enable", s.Name+".service").Run()
		if err != nil {
			return err
		}
		return exec.Command("systemctl", "daemon-reload").Run()
	}

	return nil
}

func (s *linuxService) Uninstall() error {
	if ok, err := notSupported(); ok {
		return err
	}

	if flavor == initSystemd {
		exec.Command("systemctl", "disable", s.Name+".service").Run()
	}
	cp, err := flavor.ConfigPath(s.Name, s.Config)
	if err != nil {
		return err
	}
	if err := os.Remove(cp); err != nil {
		return err
	}
	return nil
}

func (s *linuxService) Logger(errs chan<- error) (Logger, error) {
	if s.interactive {
		return ConsoleLogger, nil
	}
	return s.SystemLogger(errs)
}
func (s *linuxService) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(s.Name, errs)
}

func (s *linuxService) Run() (err error) {
	err = s.i.Start(s)
	if err != nil {
		return err
	}

	sigChan := make(chan os.Signal, 3)

	signal.Notify(sigChan, os.Interrupt, os.Kill)

	<-sigChan

	return s.i.Stop(s)
}

func (s *linuxService) Start() error {
	if ok, err := notSupported(); ok {
		return err
	}
	switch flavor {
	case initSystemd:
		return exec.Command("systemctl", "start", s.Name+".service").Run()
	case initUpstart:
		return exec.Command("initctl", "start", s.Name).Run()
	default:
		return exec.Command("service", s.Name, "start").Run()
	}
}

func (s *linuxService) Stop() error {
	if ok, err := notSupported(); ok {
		return err
	}
	switch flavor {
	case initSystemd:
		return exec.Command("systemctl", "stop", s.Name+".service").Run()
	case initUpstart:
		return exec.Command("initctl", "stop", s.Name).Run()
	default:
		return exec.Command("service", s.Name, "stop").Run()
	}
}

func (s *linuxService) Restart() error {
	err := s.Stop()
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return s.Start()
}

func notSupported() (bool, error) {
	if flavor == initNotAvailable {
		return true, errors.New("No supported init system. Supported: upstart, systemd.")
	}
	return false, nil
}

var tf = map[string]interface{}{
	"cmd": func(s string) string {
		return `"` + strings.Replace(s, `"`, `\"`, -1) + `"`
	},
}

const systemVScript = `#!/bin/sh
# For RedHat and cousins:
# chkconfig: - 99 01
# description: {{.Description}}
# processname: {{.Path}}

### BEGIN INIT INFO
# Provides:          {{.Path}}
# Required-Start:
# Required-Stop:
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: {{.Display}}
# Description:       {{.Description}}
### END INIT INFO

cmd="{{.Path}}{{range .Arguments}} {{.|cmd}}{{end}}"

name=$(basename $0)
pid_file="/var/run/$name.pid"
stdout_log="/var/log/$name.log"
stderr_log="/var/log/$name.err"

get_pid() {
    cat "$pid_file"
}

is_running() {
    [ -f "$pid_file" ] && ps $(get_pid) > /dev/null 2>&1
}

case "$1" in
    start)
        if is_running; then
            echo "Already started"
        else
            echo "Starting $name"
            $cmd >> "$stdout_log" 2>> "$stderr_log" &
            echo $! > "$pid_file"
            if ! is_running; then
                echo "Unable to start, see $stdout_log and $stderr_log"
                exit 1
            fi
        fi
    ;;
    stop)
        if is_running; then
            echo -n "Stopping $name.."
            kill $(get_pid)
            for i in {1..10}
            do
                if ! is_running; then
                    break
                fi
                echo -n "."
                sleep 1
            done
            echo
            if is_running; then
                echo "Not stopped; may still be shutting down or shutdown may have failed"
                exit 1
            else
                echo "Stopped"
                if [ -f "$pid_file" ]; then
                    rm "$pid_file"
                fi
            fi
        else
            echo "Not running"
        fi
    ;;
    restart)
        $0 stop
        if is_running; then
            echo "Unable to stop, will not attempt to start"
            exit 1
        fi
        $0 start
    ;;
    status)
        if is_running; then
            echo "Running"
        else
            echo "Stopped"
            exit 1
        fi
    ;;
    *)
    echo "Usage: $0 {start|stop|restart|status}"
    exit 1
    ;;
esac
exit 0`

// The upstart script should stop with an INT or the Go runtime will terminate
// the program before the Stop handler can run.
const upstartScript = `# {{.Description}}

description     "{{.Display}}"

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

const systemdScript = `[Unit]
Description={{.Description}}
ConditionFileIsExecutable={{.Path|cmd}}

[Service]
StartLimitInterval=5
StartLimitBurst=10
ExecStart={{.Path|cmd}}{{range .Arguments}} {{.|cmd}}{{end}}
{{if .ChRoot}}RootDirectory={{.ChRoot|cmd}}{{end}}
{{if .WorkingDirectory}}WorkingDirectory={{.WorkingDirectory|cmd}}{{end}}
{{if .UserName}}User={{.UserName}}{{end}}
Restart=always
RestartSec=120

[Install]
WantedBy=multi-user.target
`
