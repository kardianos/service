package service

import (
	"fmt"
	"os"
	"os/signal"
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

	interactive bool
}

// WindowsLogger allows using windows specific logging methods.
type WindowsLogger struct {
	ev *eventlog.Log
}

type windowsSystem struct{}

func (windowsSystem) String() string {
	return version
}

var system = windowsSystem{}

func (l WindowsLogger) Error(v ...interface{}) error {
	return l.ev.Error(3, fmt.Sprint(v...))
}
func (l WindowsLogger) Warning(v ...interface{}) error {
	return l.ev.Warning(2, fmt.Sprint(v...))
}
func (l WindowsLogger) Info(v ...interface{}) error {
	return l.ev.Info(1, fmt.Sprint(v...))
}
func (l WindowsLogger) Errorf(format string, a ...interface{}) error {
	return l.ev.Error(3, fmt.Sprintf(format, a...))
}
func (l WindowsLogger) Warningf(format string, a ...interface{}) error {
	return l.ev.Warning(2, fmt.Sprintf(format, a...))
}
func (l WindowsLogger) Infof(format string, a ...interface{}) error {
	return l.ev.Info(1, fmt.Sprintf(format, a...))
}

func (l WindowsLogger) NError(eventId uint32, v ...interface{}) error {
	return l.ev.Error(eventId, fmt.Sprint(v...))
}
func (l WindowsLogger) NWarning(eventId uint32, v ...interface{}) error {
	return l.ev.Warning(eventId, fmt.Sprint(v...))
}
func (l WindowsLogger) NInfo(eventId uint32, v ...interface{}) error {
	return l.ev.Info(eventId, fmt.Sprint(v...))
}
func (l WindowsLogger) NErrorf(eventId uint32, format string, a ...interface{}) error {
	return l.ev.Error(eventId, fmt.Sprintf(format, a...))
}
func (l WindowsLogger) NWarningf(eventId uint32, format string, a ...interface{}) error {
	return l.ev.Warning(eventId, fmt.Sprintf(format, a...))
}
func (l WindowsLogger) NInfof(eventId uint32, format string, a ...interface{}) error {
	return l.ev.Info(eventId, fmt.Sprintf(format, a...))
}

func isInteractive() (bool, error) {
	return svc.IsAnInteractiveSession()
}

func newService(i Interface, c *Config) (*windowsService, error) {
	ws := &windowsService{
		i:      i,
		Config: c,
	}
	var err error
	ws.interactive, err = isInteractive()
	return ws, err
}

func (ws *windowsService) String() string {
	if len(ws.DisplayName) > 0 {
		return ws.DisplayName
	}
	return ws.Name
}

func (ws *windowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	if err := ws.i.Start(ws); err != nil {
		// TODO: log error.
		// ws.Error(err.Error())
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
				// TODO: Log error.
				// ws.Error(err.Error())
				return true, 2
			}
			break loop
		default:
			continue loop
		}
	}

	return false, 0
}

func (ws *windowsService) Interactive() bool {
	return ws.interactive
}

func (ws *windowsService) Install() error {
	exepath, err := osext.Executable()
	if err != nil {
		return err
	}
	// Used if path contains a space.
	exepath = `"` + exepath + `"`
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
	s, err = m.CreateService(ws.Name, exepath, mgr.Config{
		DisplayName: ws.DisplayName,
		Description: ws.Description,
		StartType:   mgr.StartAutomatic,
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

func (ws *windowsService) Remove() error {
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
	if !ws.interactive {
		return svc.Run(ws.Name, ws)
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
func (ws *windowsService) Logger() (Logger, error) {
	if ws.interactive {
		return ConsoleLogger, nil
	}
	return ws.SystemLogger()
}
func (ws *windowsService) SystemLogger() (Logger, error) {
	el, err := eventlog.Open(ws.Name)
	if err != nil {
		return nil, err
	}
	return WindowsLogger{el}, nil
}
