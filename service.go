// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.package service

// Package service provides a simple way to create a system service.
// Currently supports Windows, Linux/(systemd | Upstart | SysV), and OSX/Launchd.
//
// Windows controls services by setting up callbacks that is non-trivial. This
// is very different then other systems. This package provides the same API
// despite the substantial differences.
// It also can be used to detect how a program is called, from an interactive
// terminal or from a service manager.
/*
	// Simple service that only works by printing a log message every few seconds.
	package main

	import (
		"flag"
		"log"
		"time"

		"bitbucket.org/kardianos/service2beta"
	)

	var logger service.Logger

	// Program structures.
	//  Define Start and Stop methods.
	type program struct {
		exit chan struct{}
	}

	func (p *program) Start(s service.Service) error {
		if service.Interactive() {
			logger.Info("Running in terminal.")
		} else {
			logger.Info("Running under service manager.")
		}
		p.exit = make(chan struct{})

		// Start should not block. Do the actual work async.
		go p.run()
		return nil
	}
	func (p *program) run() error {
		logger.Infof("I'm running %v.", service.Platform())
		ticker := time.NewTicker(2 * time.Second)
		for {
			select {
			case tm := <-ticker.C:
				logger.Infof("Still running at %v...", tm)
			case <-p.exit:
				ticker.Stop()
				return nil
			}
		}
		return nil
	}
	func (p *program) Stop(s service.Service) error {
		// Any work in Stop should be quick, usually a few seconds at most.
		logger.Info("I'm Stopping!")
		close(p.exit)
		return nil
	}

	// Service setup.
	//   Define service config.
	//   Create the service.
	//   Setup the logger.
	//   Handle service controls (optional).
	//   Run the service.
	func main() {
		svcFlag := flag.String("service", "", "Control the system service.")
		flag.Parse()

		svcConfig := &service.Config{
			Name:        "GoServiceTest",
			DisplayName: "Go Service Test",
			Description: "This is a test Go service.",
		}

		prg := &program{}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			log.Fatal(err)
		}
		errs := make(chan error, 5)
		logger, err = s.Logger(errs)
		if err != nil {
			log.Fatal(err)
		}

		go func() {
			for {
				err := <-errs
				if err != nil {
					log.Print(err)
				}
			}
		}()

		if len(*svcFlag) != 0 {
			err := service.Control(s, *svcFlag)
			if err != nil {
				log.Printf("Valid actions: %q\n", service.ControlAction)
				log.Fatal(err)
			}
			return
		}
		err = s.Run()
		if err != nil {
			logger.Error(err)
		}
	}
*/
package service

import (
	"errors"
	"fmt"
)

// Config provides the setup for a Service. The Name field is required.
type Config struct {
	Name        string // Required name of the service. No spaces suggested.
	DisplayName string // Display name, spaces allowed.
	Description string // Long description of service.

	UserName  string   // Run as username.
	Arguments []string // Run with arguments.

	DependsOn        []string // Other services that this depends on.
	WorkingDirectory string   // Service working directory.
	ChRoot           string
	UserService      bool // Install as a current user service.

	// System specific options.
	Option KeyValue
}

var errNameFieldRequired = errors.New("Config.Name field is required.")

// New creates a new service based on a service interface and configuration.
func New(i Interface, c *Config) (Service, error) {
	if len(c.Name) == 0 {
		return nil, errNameFieldRequired
	}
	return newService(i, c)
}

// KeyValue provides a list of platform specific options. See platform docs for
// more details.
type KeyValue map[string]interface{}

// Bool returns the value of the given name, assuming the value is a boolean.
// If the value isn't found or is not of the type, the defaultValue is returned.
func (kv KeyValue) bool(name string, defaultValue bool) bool {
	if v, found := kv[name]; found {
		if castValue, is := v.(bool); is {
			return castValue
		}
	}
	return defaultValue
}

// Int returns the value of the given name, assuming the value is an int.
// If the value isn't found or is not of the type, the defaultValue is returned.
func (kv KeyValue) int(name string, defaultValue int) int {
	if v, found := kv[name]; found {
		if castValue, is := v.(int); is {
			return castValue
		}
	}
	return defaultValue
}

// Int returns the value of the given name, assuming the value is a string.
// If the value isn't found or is not of the type, the defaultValue is returned.
func (kv KeyValue) string(name string, defaultValue string) string {
	if v, found := kv[name]; found {
		if castValue, is := v.(string); is {
			return castValue
		}
	}
	return defaultValue
}

// Int returns the value of the given name, assuming the value is a float64.
// If the value isn't found or is not of the type, the defaultValue is returned.
func (kv KeyValue) float64(name string, defaultValue float64) float64 {
	if v, found := kv[name]; found {
		if castValue, is := v.(float64); is {
			return castValue
		}
	}
	return defaultValue
}

// Platform returns a description of the OS and service platform.
func Platform() string {
	return system.String()
}

// Interactive returns false if running under the OS service manager
// and true otherwise.
func Interactive() bool {
	return system.Interactive()
}

// runningSystem represents the system and system's service being used.
type runningSystem interface {
	// String returns a description of the OS and service platform.
	String() string

	// Interactive returns false if running under the OS service manager
	// and true otherwise.
	Interactive() bool
}

// Be sure to implement each platform.
var _ runningSystem = system

// Interface represents the service interface for a program. Start runs before
// the hosting process is granted control and Stop runs when control is returned.
//
//   1. OS service manager executes user program.
//   2. User program sees it is executed from a service manager (IsInteractive is false).
//   3. User program calls Service.Run() which blocks.
//   4. Interface.Start() is called and quickly returns.
//   5. User program runs.
//   6. OS service manager signals the user program to stop.
//   7. Interface.Stop() is called and quickly returns.
//      - For a successful exit, os.Exit should not be called in Interface.Stop().
//   8. Service.Run returns.
//   9. User program should quickly exit.
type Interface interface {
	// Start provides a place to initiate the service. The service doesn't not
	// signal a completed start until after this function returns, so the
	// Start function must not take more then a few seconds at most.
	Start(s Service) error

	// Stop provides a place to clean up program execution before it is terminated.
	// It should not take more then a few seconds to execute.
	// Stop should not call os.Exit directly in the function.
	Stop(s Service) error
}

// Service represents a service that can be run or controlled.
type Service interface {
	// Run should be called shortly after the program entry point.
	// After Interface.Stop has finished running, Run will stop blocking.
	// After Run stops blocking, the program must exit shortly after.
	Run() error

	// Start signals to the OS service manager the given service should start.
	Start() error

	// Stop signals to the OS service manager the given service should stop.
	Stop() error

	// Restart signals to the OS service manager the given service should stop then start.
	Restart() error

	// Install setups up the given service in the OS service manager. This may require
	// greater rights. Will return an error if it is already installed.
	Install() error

	// Uninstall removes the given service from the OS service manager. This may require
	// greater rights. Will return an error if the service is not present.
	Uninstall() error

	// Opens and returns a system logger. If the user program is running
	// interactively rather then as a service, the returned logger will write to
	// os.Stderr. If errs is non-nil errors will be sent on errs as well as
	// returned from Logger's functions.
	Logger(errs chan<- error) (Logger, error)

	// SystemLogger opens and returns a system logger. If errs is non-nil errors
	// will be sent on errs as well as returned from Logger's functions.
	SystemLogger(errs chan<- error) (Logger, error)

	// String displays the name of the service. The display name if present,
	// otherwise the name.
	String() string
}

// ControlAction list valid string texts to use in Control.
var ControlAction = [5]string{"start", "stop", "restart", "install", "uninstall"}

// Control issues control functions to the service from a given action string.
func Control(s Service, action string) error {
	var err error
	switch action {
	case ControlAction[0]:
		err = s.Start()
	case ControlAction[1]:
		err = s.Stop()
	case ControlAction[2]:
		err = s.Restart()
	case ControlAction[3]:
		err = s.Install()
	case ControlAction[4]:
		err = s.Uninstall()
	default:
		err = fmt.Errorf("Unknown action %s", action)
	}
	if err != nil {
		return fmt.Errorf("Failed to %s %v: %v", action, s, err)
	}
	return nil
}

// Logger writes to the system log.
type Logger interface {
	Error(v ...interface{}) error
	Warning(v ...interface{}) error
	Info(v ...interface{}) error

	Errorf(format string, a ...interface{}) error
	Warningf(format string, a ...interface{}) error
	Infof(format string, a ...interface{}) error
}
