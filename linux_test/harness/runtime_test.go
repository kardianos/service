package main

import (
	"testing"

	"golang.org/x/exp/slices"
)

func TestPodmanArgs(t *testing.T) {
	var p podman
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{"start", p.startArgs("c", "img", &InitSystem{Boot: []string{"/sbin/init"}}),
			[]string{"run", "-d", "--name", "c", "--label", label + "=1", "--systemd=always", "img", "/sbin/init"}},
		{"build", p.buildArgs("img", "dir", false), []string{"build", "-t", "img", "dir"}},
		{"build -pull", p.buildArgs("img", "dir", true), []string{"build", "-t", "img", "--pull=always", "dir"}},
		{"exec", p.execArgs("c", []string{"A=1", "B=2"}, []string{"/bin/t", "-x"}),
			[]string{"exec", "-e", "A=1", "-e", "B=2", "c", "/bin/t", "-x"}},
		{"cp", p.copyArgs("c", "/h/f", "/opt/f"), []string{"cp", "/h/f", "c:/opt/f"}},
	}
	for _, tt := range tests {
		if !slices.Equal(tt.got, tt.want) {
			t.Errorf("%s args = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestParseInfo(t *testing.T) {
	tests := []struct {
		in      string
		want    HostInfo
		wantErr bool
	}{
		{"arm64 v2\n", HostInfo{"arm64", 2}, false},
		{"amd64 v1", HostInfo{"amd64", 1}, false},
		{"arm64", HostInfo{}, true},
		{"arm64 vX", HostInfo{}, true},
	}
	for _, tt := range tests {
		got, err := parseInfo(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("parseInfo(%q) = %v, %v, want %v, err %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestIsRuntimeFailure(t *testing.T) {
	for code, want := range map[int]bool{0: false, 1: false, 2: false, 124: false, 125: true, 126: true, 127: true, 128: false} {
		if got := isRuntimeFailure(code); got != want {
			t.Errorf("isRuntimeFailure(%d) = %v, want %v", code, got, want)
		}
	}
}
