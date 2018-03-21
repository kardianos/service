// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

// simple does nothing except block while running the service.
package main

import (
	"log"
	"os"
	"time"

	"github.com/kardianos/service"
	"github.com/pkg/errors"
)

var logger service.Logger

type program struct{
	svc service.Service
	logger service.Logger
}

func (p *program) Start(s service.Service) error {
	// Start should not block. Do the actual work async.
	go p.run()
	p.logger.Info("Started")
	return nil
}
func (p *program) run() {
	// Do work here
	err := doSomethingBlockingWithErr()
	if err != nil {
		p.logger.Errorf("Err while doing something: %s", err)
		p.svc.Stop()
	}
}

func doSomethingBlockingWithErr() error {
	time.Sleep(time.Second*5)
	return errors.New("Err doing something")
}

func (p *program) Stop(s service.Service) error {
	// Stop should not block.
	p.logger.Info("Stopping")
	return nil
}

func main() {
	svcConfig := &service.Config{
		Name:        "GoServiceExampleStopOnErr",
		DisplayName: "Go Service Example: Stop On Error",
		Description: "This is an example Go service that stops on error.",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		err = service.Control(s, os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	logger, err = s.Logger(nil)
	if err != nil {
		log.Fatal(err)
	}

	// Set instance of service and logger
	prg.svc = s
	prg.logger = logger

	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}
