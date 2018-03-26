// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

package service_test

import (
	"testing"
	"time"

	"github.com/kardianos/service"
	"errors"
	"context"
)

func TestRunInterrupt(t *testing.T) {
	p := &program{}
	sc := &service.Config{
		Name: "go_service_test",
	}
	s, err := service.New(p, sc)
	if err != nil {
		t.Fatalf("New err: %s", err)
	}

	go func() {
		<-time.After(1 * time.Second)
		interruptProcess(t)
	}()

	go func() {
		for i := 0; i < 25 && p.numStopped == 0; i++ {
			<-time.After(200 * time.Millisecond)
		}
		if p.numStopped == 0 {
			t.Fatal("Run() hasn't been stopped")
		}
	}()

	if err = s.Run(); err != nil {
		t.Fatalf("Run() err: %s", err)
	}
}

type program struct {
	numStopped int
}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}
func (p *program) run() {
	// Do work here
}
func (p *program) Stop(s service.Service) error {
	p.numStopped++
	return nil
}

func TestRunSelfStop(t *testing.T) {
	p := &selfStoppingProgram{}
	kv := service.KeyValue{"RunWait": p.RunWaitFunc(context.Background())}

	sc := &service.Config{
		Name: "go_service_test",
		Option: kv,
	}
	s, err := service.New(p, sc)
	if err != nil {
		t.Fatalf("New err: %s", err)
	}

	errForTest := errors.New("TestErr")
	p.svc = s
	p.errOnStop = errForTest


	go func() {
		for i := 0; i < 25 && p.numStopped == 0; i++ {
			<-time.After(200 * time.Millisecond)
		}

		if p.numStopped == 0 {
			t.Fatal("Run() hasn't been stopped")
		}
	}()
	time.Sleep(time.Millisecond)

	select {
	case err := <-RunServiceAsync(s):
		if err != nil && err != errForTest{
			t.Fatalf("Run() err: %s, expected: %s", err, errForTest)
		}

	case <-time.After(time.Second*2):
		t.Fatalf("Service process hasn't been stopped")
	}
}

func RunServiceAsync(svc service.Service) chan error {
	ch := make(chan error)
	go func () {
		ch <- svc.Run()
	}()
	return ch
}

type selfStoppingProgram struct {
	numStopped int
	svc service.Service
	errOnStop error
	stopFunc context.CancelFunc
}

func (p *selfStoppingProgram) Start(s service.Service) error {
	go p.run()
	return nil
}
func (p *selfStoppingProgram) run() {
	// Do work here
	time.Sleep(1*time.Second)
	p.stopFunc()
}

func (p *selfStoppingProgram) RunWaitFunc(ctx context.Context) func() {
	ctx, cancel := context.WithCancel(context.Background())
	p.stopFunc = cancel

	return func () {
		select {
		case <-ctx.Done():
			return
		}
	}
}

func (p *selfStoppingProgram) Stop(s service.Service) error {
	p.numStopped++
	return p.errOnStop
}
