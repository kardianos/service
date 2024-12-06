// Copyright 2016 Lawrence Woodman <lwoodman@vlifesystems.com>
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

//go:build darwin || dragonfly || freebsd || linux || nacl || netbsd || openbsd || solaris || aix || ppc64
// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris aix ppc64

package service_test

import (
	"log"
	"os"
)

func interruptProcess() {
	pid := os.Getpid()
	p, err := os.FindProcess(pid)
	if err != nil {
		log.Fatalf("FindProcess: %s", err)
	}
	if err := p.Signal(os.Interrupt); err != nil {
		log.Fatalf("Signal: %s", err)
	}
}
