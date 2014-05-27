/*
Many services that run on different platforms cannot rely
on flags to be passed for configuration. Some platforms
require explicit install commands. This package handles the common
boilerplate code. The following command may be passed to the
executable as the first argument:
	install | remove | run | start | stop

These commands will do the following actions:
	install - Install the running executable as a service on the system.
	remove - Remove the running executable as a service on the system.
	run - Run the service as a command line application, output log to prompt.
	start - Starts the service via system controls.
	stop - Stops the service via system controls.
*/
package stdservice

import (
	"fmt"
	"os"

	"bitbucket.org/kardianos/service"
)

// Standard service configuration. Start MUST block.
// Stop MUST NOT block for more then a second or two.
type Config struct {
	// Used to register the service with the operating system.
	Name, DisplayName, LongDescription string

	// Called when the service starts or stops.
	// Stop may be nil.
	Start, Stop func(c *Config)

	// Called after logging may be setup but before the service is started.
	// Init is optional and may be nil.
	// If Init returns an error, that error is logged to the logger
	// and the service start is aborted.
	// Init should not block.
	Init func(c *Config) error

	s service.Service
	l service.Logger
}

// Get service after Run() has been called.
func (c *Config) Service() service.Service {
	return c.s
}

// Get logger after Run() has been called.
func (c *Config) Logger() service.Logger {
	return c.l
}

// Fill in configuration, then call Run() to setup basic handling.
// Blocks until program completes. Is intended to handle the standard
// simple cases for running a service.
func (c *Config) Run() {
	run(c)
}

// Depreciated. Same as *Config.Run().
func Run(c *Config) {
	run(c)
}

func run(c *Config) {
	var s, err = service.NewService(c.Name, c.DisplayName, c.LongDescription)
	c.s = s
	c.l = s

	if err != nil {
		fmt.Printf("%s unable to start: %s", c.DisplayName, err)
		return
	}

	if len(os.Args) > 1 {
		var err error
		verb := os.Args[1]
		switch verb {
		case "install":
			err = s.Install()
			if err != nil {
				fmt.Printf("Failed to install: %s\n", err)
				return
			}
			fmt.Printf("Service \"%s\" installed.\n", c.DisplayName)
		case "remove":
			err = s.Remove()
			if err != nil {
				fmt.Printf("Failed to remove: %s\n", err)
				return
			}
			fmt.Printf("Service \"%s\" removed.\n", c.DisplayName)
		case "run":
			c.l = ConsoleLogger{}
			defer func() {
				if c.Stop != nil {
					c.Stop(c)
				}
			}()
			if c.Init != nil {
				err := c.Init(c)
				if err != nil {
					c.l.Error(err.Error())
					return
				}
			}
			c.Start(c)
		case "start":
			err = s.Start()
			if err != nil {
				fmt.Printf("Failed to start: %s\n", err)
				return
			}
			fmt.Printf("Service \"%s\" started.\n", c.DisplayName)
		case "stop":
			err = s.Stop()
			if err != nil {
				fmt.Printf("Failed to stop: %s\n", err)
				return
			}
			fmt.Printf("Service \"%s\" stopped.\n", c.DisplayName)
		default:
			fmt.Printf("Options for \"%s\": (install | remove | run | start | stop)\n", os.Args[0])
		}
		return
	}

	if c.Init != nil {
		err := c.Init(c)
		if err != nil {
			c.l.Error(err.Error())
			return
		}
	}

	err = s.Run(func() error {
		// start
		go c.Start(c)
		return nil
	}, func() error {
		// stop
		if c.Stop != nil {
			c.Stop(c)
		}
		return nil
	})
	if err != nil {
		c.l.Error(err.Error())
	}
}
