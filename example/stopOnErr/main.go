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
	"context"
	"os/signal"
	"syscall"
)

var logger service.Logger

type program struct{
	svc service.Service
	logger service.Logger
	stopFunc context.CancelFunc
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
		p.stopFunc()
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

func (p *program) RunWaitFunc(ctx context.Context) func() {
	ctx, cancel := context.WithCancel(context.Background())
	p.stopFunc = cancel

	var sigChan = make(chan os.Signal, 3)
	signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)

	return func () {
		select {
		case <-ctx.Done():
			log.Println("Terminated with context")
			return
		case <-sigChan:
			log.Println("Terminated with signal")
			return
		}
	}
}

func main() {
	prg := &program{}
	kv := service.KeyValue{"RunWait": prg.RunWaitFunc(context.Background())}

	svcConfig := &service.Config{
		Name:        "GoServiceExampleStopOnErr",
		DisplayName: "Go Service Example: Stop On Error",
		Description: "This is an example Go service that stops on error.",
		Option: kv,
	}


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
