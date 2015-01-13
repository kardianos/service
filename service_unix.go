//

// +build linux darwin

package service

import (
	"fmt"
	"log/syslog"
)

func newSysLogger(name string) (Logger, error) {
	w, err := syslog.New(syslog.LOG_INFO, name)
	if err != nil {
		return nil, err
	}
	return sysLogger{w}, nil
}

type sysLogger struct {
	*syslog.Writer
}

func (s sysLogger) Error(v ...interface{}) error {
	return s.Writer.Err(fmt.Sprint(v...))
}
func (s sysLogger) Warning(v ...interface{}) error {
	return s.Writer.Warning(fmt.Sprint(v...))
}
func (s sysLogger) Info(v ...interface{}) error {
	return s.Writer.Info(fmt.Sprint(v...))
}
func (s sysLogger) Errorf(format string, a ...interface{}) error {
	return s.Writer.Err(fmt.Sprintf(format, a...))
}
func (s sysLogger) Warningf(format string, a ...interface{}) error {
	return s.Writer.Warning(fmt.Sprintf(format, a...))
}
func (s sysLogger) Infof(format string, a ...interface{}) error {
	return s.Writer.Info(fmt.Sprintf(format, a...))
}
