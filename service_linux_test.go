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

// createTestMountInfoFiles creates mock files for tests
func createTestMountInfoFiles() (*os.File, *os.File, error) {
	// docker cgroup setup
	hDockerGrp, err := os.CreateTemp("", "*")
	if err != nil {
		return nil, nil, errors.New("docker tempfile create failed")
	}
	_, err = hDockerGrp.Write([]byte(dockerMountInfo))
	if err != nil {
		return nil, nil, errors.New("docker tempfile write failed")
	}

	// linux cgroup setup
	hLinuxGrp, err := os.CreateTemp("", "*")
	if err != nil {
		return nil, nil, errors.New("\"normal\" tempfile  create failed")
	}
	_, err = hLinuxGrp.Write([]byte(linuxMountInfo))
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

func Test_isInContainerCGroup(t *testing.T) {

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
		filePath string
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
			got, err := isInContainerCGroup(tt.args.filePath)
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
func Test_isInContainerMountInfo(t *testing.T) {

	// setup
	hDockerGrp, hLinuxGrp, err := createTestMountInfoFiles()
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
		filePath string
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
			got, err := isInContainerMountInfo(tt.args.filePath)
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

func Test_isInContainerDockerEnv(t *testing.T) {

	// TEST
	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{"docker", args{os.TempDir()}, true, false},
		{"linux", args{"/non_existent_file"}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isInContainerDockerEnv(tt.args.filePath)
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

	dockerMountInfo = `3860 3859 0:159 / /dev/pts rw,nosuid,noexec,relatime - devpts devpts rw,gid=5,mode=620,ptmxmode=666
3861 3857 0:160 / /sys ro,nosuid,nodev,noexec,relatime - sysfs sysfs ro
3862 3861 0:29 / /sys/fs/cgroup ro,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw
3863 3859 0:156 / /dev/mqueue rw,nosuid,nodev,noexec,relatime - mqueue mqueue rw
3864 3859 0:161 / /dev/shm rw,nosuid,nodev,noexec,relatime - tmpfs shm rw,size=65536k,inode64
3865 3857 259:4 /var/lib/docker/volumes/345b0c4550daa5dbc7f4fa9fbd28717844e69e235f2aa014c7d24114320dd9a1/_data /opt/data rw,relatime master:1 - ext4 /dev/nvme0n1p4 rw
3866 3857 259:4 /var/lib/docker/containers/ea4d56df6742a4940bfa0b31a4481707511f2da7b7c0708ffe901b46f461eb89/resolv.conf /etc/resolv.conf rw,relatime - ext4 /dev/nvme0n1p4 rw
3867 3857 259:4 /var/lib/docker/containers/ea4d56df6742a4940bfa0b31a4481707511f2da7b7c0708ffe901b46f461eb89/hostname /etc/hostname rw,relatime - ext4 /dev/nvme0n1p4 rw
3868 3857 259:4 /var/lib/docker/containers/ea4d56df6742a4940bfa0b31a4481707511f2da7b7c0708ffe901b46f461eb89/hosts /etc/hosts rw,relatime - ext4 /dev/nvme0n1p4 rw
3776 3859 0:159 /0 /dev/console rw,nosuid,noexec,relatime - devpts devpts rw,gid=5,mode=620,ptmxmode=666
3777 3858 0:157 /bus /proc/bus ro,nosuid,nodev,noexec,relatime - proc proc rw
3778 3858 0:157 /fs /proc/fs ro,nosuid,nodev,noexec,relatime - proc proc rw`

	linuxMountInfo = `183 28 0:44 / /run/credentials/systemd-tmpfiles-setup.service ro,nosuid,nodev,noexec,relatime,nosymfollow shared:103 - tmpfs tmpfs rw,size=1024k,nr_inodes=1024,mode=700,inode64,noswap
129 36 0:45 / /proc/sys/fs/binfmt_misc rw,nosuid,nodev,noexec,relatime shared:105 - binfmt_misc binfmt_misc rw
407 28 0:48 / /run/credentials/systemd-resolved.service ro,nosuid,nodev,noexec,relatime,nosymfollow shared:107 - tmpfs tmpfs rw,size=1024k,nr_inodes=1024,mode=700,inode64,noswap
166 30 0:60 / /var/lib/lxcfs rw,nosuid,nodev,relatime shared:329 - fuse.lxcfs lxcfs rw,user_id=0,group_id=0,allow_other
217 28 0:67 / /run/rpc_pipefs rw,relatime shared:911 - rpc_pipefs sunrpc rw
298 28 0:26 /snapd/ns /run/snapd/ns rw,nosuid,nodev,noexec,relatime - tmpfs tmpfs rw,size=4066576k,mode=755,inode64
335 298 0:4 mnt:[4026533124] /run/snapd/ns/cups.mnt rw - nsfs nsfs rw
1821 298 0:4 mnt:[4026533318] /run/snapd/ns/snapd-desktop-integration.mnt rw - nsfs nsfs rw
169 28 0:64 / /run/user/1000 rw,nosuid,nodev,relatime shared:700 - tmpfs tmpfs rw,size=4066576k,nr_inodes=1016644,mode=700,uid=1000,gid=1000,inode64
924 169 0:65 / /run/user/1000/doc rw,nosuid,nodev,relatime shared:794 - fuse.portal portal rw,user_id=1000,group_id=1000
2388 169 0:71 / /run/user/1000/gvfs rw,nosuid,nodev,relatime shared:814 - fuse.gvfsd-fuse gvfsd-fuse rw,user_id=1000,group_id=1000
2948 298 0:4 mnt:[4026534753] /run/snapd/ns/firmware-updater.mnt rw - nsfs nsfs rw
3517 30 0:129 / /var/lib/docker/overlay2/79490de289b65b7a63e86a6ae48a8607cf251030eec80d554f99eff740ba425e/merged rw,relatime shared:834 - overlay overlay rw,lowerdir=/var/lib/docker/overlay2/l/FCSCRZU3XZN6KVYV6UA4IU6CZM:/var/lib/docker/overlay2/l/I7GRZ2AVB7NO3MPA7VF75DF25N:/var/lib/docker/overlay2/l/3PHQOMHS6SD5HZFMUUENIQXENT:/var/lib/docker/overlay2/l/YOR22M55ADQZ4GRJIW3NR4EKF3:/var/lib/docker/overlay2/l/SLIJGALM6YROVNIV62WO7HLAXT,upperdir=/var/lib/docker/overlay2/79490de289b65b7a63e86a6ae48a8607cf251030eec80d554f99eff740ba425e/diff,workdir=/var/lib/docker/overlay2/79490de289b65b7a63e86a6ae48a8607cf251030eec80d554f99eff740ba425e/work,nouserxattr
3623 28 0:4 net:[4026537044] /run/docker/netns/e63556d83137 rw shared:1054 - nsfs nsfs rw
3548 30 0:137 / /var/lib/docker/overlay2/5f0fd269ad76199040b9b3ca1fa13ce36f9ab6799cd4b0b5406732c2c8407ff6/merged rw,relatime shared:1074 - overlay overlay rw,lowerdir=/var/lib/docker/overlay2/l/SXPONMLN4JW3QBFQWXCZ3RHVST:/var/lib/docker/overlay2/l/LA6JENXEZAPXZZF2FP4ON5WDEA:/var/lib/docker/overlay2/l/XEGVGERQJ7L72RWT3VIXEWGBL4:/var/lib/docker/overlay2/l/BPXXR3DHMVCSQWGSXG5NFNBE5W,upperdir=/var/lib/docker/overlay2/5f0fd269ad76199040b9b3ca1fa13ce36f9ab6799cd4b0b5406732c2c8407ff6/diff,workdir=/var/lib/docker/overlay2/5f0fd269ad76199040b9b3ca1fa13ce36f9ab6799cd4b0b5406732c2c8407ff6/work,nouserxattr
3700 28 0:4 net:[4026537144] /run/docker/netns/0b489b9c590d rw shared:1094 - nsfs nsfs rw`
)
