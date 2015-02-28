// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.package service

package service

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"bitbucket.org/kardianos/osext"
	"code.google.com/p/winsvc/eventlog"
	"code.google.com/p/winsvc/mgr"
	"code.google.com/p/winsvc/svc"
)

const version = "Windows Service"

type windowsService struct {
	i Interface
	*Config

	errSync      sync.Mutex
	stopStartErr error
}

// WindowsLogger allows using windows specific logging methods.
type WindowsLogger struct {
	ev   *eventlog.Log
	errs chan<- error
}

type windowsSystem struct{}

func (windowsSystem) String() string {
	return version
}
func (windowsSystem) Interactive() bool {
	return interactive
}

var system = windowsSystem{}

func (l WindowsLogger) send(err error) error {
	if err == nil {
		return nil
	}
	if l.errs != nil {
		l.errs <- err
	}
	return err
}
func (l WindowsLogger) Error(v ...interface{}) error {
	return l.send(l.ev.Error(3, fmt.Sprint(v...)))
}
func (l WindowsLogger) Warning(v ...interface{}) error {
	return l.send(l.ev.Warning(2, fmt.Sprint(v...)))
}
func (l WindowsLogger) Info(v ...interface{}) error {
	return l.send(l.ev.Info(1, fmt.Sprint(v...)))
}
func (l WindowsLogger) Errorf(format string, a ...interface{}) error {
	return l.send(l.ev.Error(3, fmt.Sprintf(format, a...)))
}
func (l WindowsLogger) Warningf(format string, a ...interface{}) error {
	return l.send(l.ev.Warning(2, fmt.Sprintf(format, a...)))
}
func (l WindowsLogger) Infof(format string, a ...interface{}) error {
	return l.send(l.ev.Info(1, fmt.Sprintf(format, a...)))
}

func (l WindowsLogger) NError(eventId uint32, v ...interface{}) error {
	return l.send(l.ev.Error(eventId, fmt.Sprint(v...)))
}
func (l WindowsLogger) NWarning(eventId uint32, v ...interface{}) error {
	return l.send(l.ev.Warning(eventId, fmt.Sprint(v...)))
}
func (l WindowsLogger) NInfo(eventId uint32, v ...interface{}) error {
	return l.send(l.ev.Info(eventId, fmt.Sprint(v...)))
}
func (l WindowsLogger) NErrorf(eventId uint32, format string, a ...interface{}) error {
	return l.send(l.ev.Error(eventId, fmt.Sprintf(format, a...)))
}
func (l WindowsLogger) NWarningf(eventId uint32, format string, a ...interface{}) error {
	return l.send(l.ev.Warning(eventId, fmt.Sprintf(format, a...)))
}
func (l WindowsLogger) NInfof(eventId uint32, format string, a ...interface{}) error {
	return l.send(l.ev.Info(eventId, fmt.Sprintf(format, a...)))
}

var interactive = false

func init() {
	var err error
	interactive, err = svc.IsAnInteractiveSession()
	if err != nil {
		panic(err)
	}
}

func newService(i Interface, c *Config) (*windowsService, error) {
	ws := &windowsService{
		i:      i,
		Config: c,
	}
	return ws, nil
}

func (ws *windowsService) String() string {
	if len(ws.DisplayName) > 0 {
		return ws.DisplayName
	}
	return ws.Name
}

func (ws *windowsService) setError(err error) {
	ws.errSync.Lock()
	defer ws.errSync.Unlock()
	ws.stopStartErr = err
}
func (ws *windowsService) getError() error {
	ws.errSync.Lock()
	defer ws.errSync.Unlock()
	return ws.stopStartErr
}

func (ws *windowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	if err := ws.i.Start(ws); err != nil {
		ws.setError(err)
		return true, 1
	}

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
loop:
	for {
		c := <-r
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			if err := ws.i.Stop(ws); err != nil {
				ws.setError(err)
				return true, 2
			}
			break loop
		default:
			continue loop
		}
	}

	return false, 0
}

func (ws *windowsService) Install() error {
	exepath, err := osext.Executable()
	if err != nil {
		return err
	}
	binPath := &bytes.Buffer{}
	// Quote exe path in case it contains a string.
	binPath.WriteRune('"')
	binPath.WriteString(exepath)
	binPath.WriteRune('"')

	// Arguments are encoded with the binary path to service.
	// Enclose arguments in quotes. Escape quotes with a backslash.
	for _, arg := range ws.Arguments {
		binPath.WriteRune(' ')
		binPath.WriteString(`"`)
		binPath.WriteString(strings.Replace(arg, `"`, `\"`, -1))
		binPath.WriteString(`"`)
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ws.Name)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", ws.Name)
	}
	s, err = m.CreateService(ws.Name, binPath.String(), mgr.Config{
		DisplayName:      ws.DisplayName,
		Description:      ws.Description,
		StartType:        mgr.StartAutomatic,
		ServiceStartName: ws.UserName,
		Password:         ws.Option.string("Password", ""),
		Dependencies:     ws.Dependencies,
	})
	if err != nil {
		return err
	}
	defer s.Close()
	err = eventlog.InstallAsEventCreate(ws.Name, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		s.Delete()
		return fmt.Errorf("InstallAsEventCreate() failed: %s", err)
	}
	return nil
}

func (ws *windowsService) Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ws.Name)
	if err != nil {
		return fmt.Errorf("service %s is not installed", ws.Name)
	}
	defer s.Close()
	err = s.Delete()
	if err != nil {
		return err
	}
	err = eventlog.Remove(ws.Name)
	if err != nil {
		return fmt.Errorf("RemoveEventLogSource() failed: %s", err)
	}
	return nil
}

func (ws *windowsService) Run() error {
	ws.setError(nil)
	if !interactive {
		// Return error messages from start and stop routines
		// that get executed in the Execute method.
		// Guarded with a mutex as it may run a different thread
		// (callback from windows).
		runErr := svc.Run(ws.Name, ws)
		startStopErr := ws.getError()
		if startStopErr != nil {
			return startStopErr
		}
		if runErr != nil {
			return runErr
		}
		return nil
	}
	err := ws.i.Start(ws)
	if err != nil {
		return err
	}

	sigChan := make(chan os.Signal)

	signal.Notify(sigChan, os.Interrupt, os.Kill)

	<-sigChan

	return ws.i.Stop(ws)
}

func (ws *windowsService) Start() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ws.Name)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Start([]string{})
}

func (ws *windowsService) Stop() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ws.Name)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Control(svc.Stop)
	return err
}

func (ws *windowsService) Restart() error {
	err := ws.Stop()
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return ws.Start()
}
func (ws *windowsService) Logger(errs chan<- error) (Logger, error) {
	if interactive {
		return ConsoleLogger, nil
	}
	return ws.SystemLogger(errs)
}
func (ws *windowsService) SystemLogger(errs chan<- error) (Logger, error) {
	el, err := eventlog.Open(ws.Name)
	if err != nil {
		return nil, err
	}
	return WindowsLogger{el, errs}, nil
}
