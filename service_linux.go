// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.package service

package service

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type newServiceFunc func(i Interface, c *Config) (Service, error)

type linuxSystem struct {
	interactive  bool
	selectedName string
	selectedNew  newServiceFunc
}

func (ls linuxSystem) String() string {
	return fmt.Sprintf("Linux %s", ls.selectedName)
}

func (ls linuxSystem) Interactive() bool {
	return ls.interactive
}

type systemChoice interface {
	Name() string
	Detect() bool
	Interactive() bool
	New(i Interface, c *Config) (Service, error)
}

type linuxSystemChoice struct {
	name        string
	detect      func() bool
	interactive func() bool
	new         func(i Interface, c *Config) (Service, error)
}

func (sc linuxSystemChoice) Name() string {
	return sc.name
}
func (sc linuxSystemChoice) Detect() bool {
	return sc.detect()
}
func (sc linuxSystemChoice) Interactive() bool {
	return sc.interactive()
}
func (sc linuxSystemChoice) New(i Interface, c *Config) (Service, error) {
	return sc.new(i, c)
}

var systemRegistry = []systemChoice{
	linuxSystemChoice{
		name:   "systemd",
		detect: isSystemd,
		interactive: func() bool {
			is, _ := isInteractive()
			return is
		},
		new: newSystemdService,
	},
	linuxSystemChoice{
		name:   "Upstart",
		detect: isUpstart,
		interactive: func() bool {
			is, _ := isInteractive()
			return is
		},
		new: newUpstartService,
	},
	linuxSystemChoice{
		name:   "System-V",
		detect: func() bool { return true },
		interactive: func() bool {
			is, _ := isInteractive()
			return is
		},
		new: newSystemVService,
	},
}

func newLinuxSystem() linuxSystem {
	for _, choice := range systemRegistry {
		if choice.Detect() == false {
			continue
		}
		return linuxSystem{
			interactive:  choice.Interactive(),
			selectedName: choice.Name(),
			selectedNew:  choice.New,
		}
	}
	return linuxSystem{}
}

var system = newLinuxSystem()

var errNoServiceSystemDetected = errors.New("No service system detected.")

func newService(i Interface, c *Config) (Service, error) {
	if system.selectedNew == nil {
		return nil, errNoServiceSystemDetected
	}
	return system.selectedNew(i, c)
}

func isInteractive() (bool, error) {
	// TODO: This is not true for user services.
	return os.Getppid() != 1, nil
}

var tf = map[string]interface{}{
	"cmd": func(s string) string {
		return `"` + strings.Replace(s, `"`, `\"`, -1) + `"`
	},
}
