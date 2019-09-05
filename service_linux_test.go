// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"
)

// createTestCgroupFiles creates mock files for tests
func createTestCgroupFiles() (*os.File, *os.File, error) {
	// docker cgroup setup
	hDockerGrp, err := ioutil.TempFile("", "*")
	if err != nil {
		return nil, nil, errors.New("docker tempfile create failed")
	}
	_, err = hDockerGrp.Write([]byte(dockerCgroup))
	if err != nil {
		return nil, nil, errors.New("docker tempfile write failed")
	}

	// linux cgroup setup
	hLinuxGrp, err := ioutil.TempFile("", "*")
	if err != nil {
		return nil, nil, errors.New("\"normal\" tempfile  create failed")
	}
	_, err = hLinuxGrp.Write([]byte(linuxCgroup))
	if err != nil {
		return nil, nil, errors.New("\"normal\" tempfile write failed")
	}

	return hDockerGrp, hLinuxGrp, nil
}

// removeTestFile closes and removes the provided file
func removeTestFile(hFile *os.File) {
	hFile.Close()
	os.Remove(hFile.Name())
}

func Test_isInContainer(t *testing.T) {

	// setup
	hDockerGrp, hLinuxGrp, err := createTestCgroupFiles()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// tear down
		removeTestFile(hDockerGrp)
		removeTestFile(hLinuxGrp)
	}()

	// TEST
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
		{"linux", args{hLinuxGrp.Name()}, false, false},
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

func Test_isInteractive(t *testing.T) {

	// setup
	hDockerGrp, hLinuxGrp, err := createTestCgroupFiles()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// tear down
		removeTestFile(hDockerGrp)
		removeTestFile(hLinuxGrp)
	}()

	// stack emulation for before() and after() for storing global values
	strStack := make(chan string, 4)

	// TEST
	tests := []struct {
		name    string
		before  func()
		after   func()
		want    bool
		wantErr bool
	}{
		{"docker",
			func() {
				strStack <- cgroupFile
				cgroupFile = hDockerGrp.Name()
			},
			func() {
				cgroupFile = <-strStack
			},
			true, false,
		},
		{"linux",
			func() {
				strStack <- cgroupFile
				cgroupFile = hLinuxGrp.Name()
			},
			func() {
				cgroupFile = <-strStack
			},
			true, false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.before()
			got, err := isInteractive()
			tt.after()
			if (err != nil) != tt.wantErr {
				t.Errorf("isInteractive() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isInteractive() = %v, want %v", got, tt.want)
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
