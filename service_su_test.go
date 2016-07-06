// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

// This needs to be run as root/admin hence the reason there is a build tag
// +build su

package service_test

import (
	"log"
	"os"
	"testing"

	"github.com/kardianos/service"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == runAsServiceArg {
		runService()
		return
	}
	os.Exit(m.Run())
}

func TestInstallRunRestartStopRemove(t *testing.T) {
	p := &program{}
	s, err := service.New(p, sc)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Uninstall()

	err = s.Install()
	if err != nil {
		t.Fatal("install", err)
	}
	defer s.Uninstall()

	err = s.Start()
	if err != nil {
		t.Fatal("start", err)
	}
	err = s.Restart()
	if err != nil {
		t.Fatal("restart", err)
	}
	err = s.Stop()
	if err != nil {
		t.Fatal("stop", err)
	}
	err = s.Uninstall()
	if err != nil {
		t.Fatal("uninstall", err)
	}
}

func runService() {
	p := &program{}
	s, err := service.New(p, sc)
	if err != nil {
		log.Fatal(err)
	}
	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}
