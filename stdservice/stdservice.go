package stdservice

import (
	"bitbucket.org/kardianos/service"
	"fmt"
	"os"
)

// Standard service configuration. Start() can block as long as desired,
// Stop() should not block for more then a second or two.
type Config struct {
	// Used to register the service with the operating system.
	Name, DisplayName, LongDescription string

	// Called when the service starts or stops.
	// Stop() may be nil.
	Start, Stop func(c *Config)

	// Called after logging may be setup but before the service is started.
	// Init() is optional and may be nil.
	Init func(c *Config)

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
func Run(c *Config) {
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
		}
		return
	}

	if c.Init != nil {
		c.Init(c)
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
