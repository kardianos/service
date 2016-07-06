// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

package service_test

import (
	"log"
	"testing"
	"time"

	"github.com/kardianos/service"
)

const runAsServiceArg = "RunThisAsService"

var sc = &service.Config{
	Name:      "go_service_test",
	Arguments: []string{runAsServiceArg},
}

func TestRunInterrupt(t *testing.T) {
	p := &program{}
	s, err := service.New(p, sc)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		<-time.After(1 * time.Second)
		interruptProcess(t)
	}()

	go func() {
		<-time.After(3 * time.Second)
		if !p.hasStopped {
			panic("Run() hasn't been stopped")
		}
	}()

	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}

type program struct {
	hasRun     bool
	hasStopped bool
}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}
func (p *program) run() {
	p.hasRun = true
	// Do work here
}
func (p *program) Stop(s service.Service) error {
	p.hasStopped = true
	return nil
}
