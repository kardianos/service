// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

package service

import (
	"io/ioutil"
	"testing"
)

func Test_isInContainer(t *testing.T) {

	// setup
	hDockerGrp, err := ioutil.TempFile("", "*")
	if err != nil {
		t.Fatal("docker tempfile create failed")
	}
	defer func() {
		_=hDockerGrp.Close()
		_=os.Remove(hDockerGrp.Name())

	}
	_,err:=hDockerGrp.Write([]byte(dockerCgroup))
	if err!=nil{
		t.Fatal("docker tempfile write failed")
	}

	hNormalGrp, err := ioutil.TempFile("", "*")
	if err != nil {
		t.Fatal("\"normal\" tempfile  create failed")
	}
	defer func() {
		_=hNormalGrp.Close()
		_=os.Remove(hNormalGrp.Name())

	}
	_,err:=hDockerGrp.Write([]byte(dockerCgroup))
	if err!=nil{
		t.Fatal("\"normal\" tempfile write failed")
	}

	type args struct {
		cgroupPath string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{"docker", args{hDockerGrp.Name()}, true, false},
		{"normal", args{hNormalGrp.Name()}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isInContainer(tt.args.cgroupPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("isInContainer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isInContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}

const (
	dockerCgroup = `13:name=systemd:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
12:pids:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
11:hugetlb:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
10:net_prio:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
9:perf_event:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
8:net_cls:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
7:freezer:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
6:devices:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
5:memory:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
4:blkio:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
3:cpuacct:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
2:cpu:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee
1:cpuset:/docker/bc9f0894926991e3064b731c26d86af6df7390c0e6453e6027f9545aba5809ee`

	linuxCgroup = `11:cpuset:/
10:pids:/init.scope
9:perf_event:/
8:memory:/init.scope
7:blkio:/
6:devices:/init.scope
5:rdma:/
4:net_cls,net_prio:/
3:freezer:/
2:cpu,cpuacct:/
1:name=systemd:/init.scope
0::/init.scope`
)
