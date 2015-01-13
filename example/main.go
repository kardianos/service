package main

import (
	"log"
	"os"
	"time"

	"bitbucket.org/kardianos/service2beta"
)

var logger service.Logger

type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	if s.Interactive() {
		logger.Info("Running in terminal.")
	} else {
		logger.Info("Running under service manager.")
	}
	p.exit = make(chan struct{})
	go p.run()
	return nil
}
func (p *program) run() error {
	logger.Infof("I'm running %v.", service.LocalSystem())
	ticker := time.NewTicker(2 * time.Second)
	for {
		select {
		case tm := <-ticker.C:
			err := logger.Infof("Still running at %v...", tm)
			if err != nil {
				panic(err)
			}
		case <-p.exit:
			ticker.Stop()
			return nil
		}
	}
	return nil
}
func (p *program) Stop(s service.Service) error {
	err := logger.Info("I'm Stopping!")
	if err != nil {
		panic(err)
	}
	close(p.exit)
	return nil
}

func main() {
	svcConfig := &service.Config{
		Name:        "GoServiceTest",
		DisplayName: "Go Service Test",
		Description: "This is a test Go service.",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		panic(err)
	}
	logger, err = s.SystemLogger()
	if err != nil {
		panic(err)
	}

	if len(os.Args) > 1 {
		err := service.Control(s, os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}
