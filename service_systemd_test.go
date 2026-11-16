//go:build linux

package service

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func newSystemd(t *testing.T, c *Config) *systemd {
	t.Helper()
	if c.Name == "" {
		c.Name = "svc"
	}
	svc, err := newSystemdService(nil, "linux-systemd", c)
	if err != nil {
		t.Fatal(err)
	}
	return svc.(*systemd)
}

func TestSystemdConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct {
		level ServiceLevel
		want  string
	}{
		{ServiceLevelSystem, "/etc/systemd/system/svc.service"},
		{ServiceLevelUser, filepath.Join(home, ".config/systemd/user/svc.service")},
		{ServiceLevelAllUsers, "/etc/systemd/user/svc.service"},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			got, err := newSystemd(t, &Config{ServiceLevel: tt.level}).configPath()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("configPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSystemdInstallCommands(t *testing.T) {
	tests := []struct {
		level ServiceLevel
		want  [][]string
	}{
		{ServiceLevelSystem, [][]string{{"enable", "svc.service"}, {"daemon-reload"}}},
		{ServiceLevelUser, [][]string{{"--user", "enable", "svc.service"}, {"--user", "daemon-reload"}}},
		{ServiceLevelAllUsers, [][]string{{"--global", "enable", "svc.service"}}},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			got := newSystemd(t, &Config{ServiceLevel: tt.level}).installCommands()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("installCommands() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSystemdUninstallCommands(t *testing.T) {
	tests := []struct {
		level ServiceLevel
		want  [][]string
	}{
		{ServiceLevelSystem, [][]string{{"disable", "svc.service"}, {"daemon-reload"}}},
		{ServiceLevelUser, [][]string{{"--user", "disable", "svc.service"}, {"--user", "daemon-reload"}}},
		{ServiceLevelAllUsers, [][]string{{"--global", "disable", "svc.service"}}},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			got := newSystemd(t, &Config{ServiceLevel: tt.level}).uninstallCommands()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("uninstallCommands() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSystemdAllUsersOptions(t *testing.T) {
	tests := []struct {
		name string
		c    Config
	}{
		{"UserName", Config{UserName: "bob"}},
		{"ChRoot", Config{ChRoot: "/jail"}},
		{"LogDirectory", Config{Option: KeyValue{optionLogDirectory: "/var/log/svc"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.c
			c.Name = "svc"
			c.ServiceLevel = ServiceLevelAllUsers
			if _, err := newSystemdService(nil, "linux-systemd", &c); err == nil {
				t.Error("AllUsers: err = nil, want an error")
			}
			c.ServiceLevel = ServiceLevelSystem
			if _, err := newSystemdService(nil, "linux-systemd", &c); err != nil {
				t.Errorf("System: err = %v, want nil", err)
			}
		})
	}
}

func TestAllUsersInstallRequiresPrivilege(t *testing.T) {
	stubPrivileged(t, false)
	calls := recordCommands(t, nil)
	s := newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers})

	if err := s.Install(); !errors.Is(err, ErrRequiresPrivilege) {
		t.Errorf("Install err = %v, want %v", err, ErrRequiresPrivilege)
	}
	if len(*calls) != 0 {
		t.Errorf("ran %q, want no commands", *calls)
	}
}

func TestAllUsersUninstallRequiresPrivilege(t *testing.T) {
	stubPrivileged(t, false)
	calls := recordCommands(t, nil)
	s := newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers})

	if err := s.Uninstall(); !errors.Is(err, ErrRequiresPrivilege) {
		t.Errorf("Uninstall err = %v, want %v", err, ErrRequiresPrivilege)
	}
	if len(*calls) != 0 {
		t.Errorf("ran %q, want no commands", *calls)
	}
}

func TestAllUsersInstallMinVersion(t *testing.T) {
	stubPrivileged(t, true)
	calls := recordCommands(t, map[string]string{"systemctl --version": "systemd 219\n+PAM +AUDIT"})
	s := newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers})

	err := s.Install()
	if err == nil || !strings.Contains(err.Error(), "235") || !strings.Contains(err.Error(), "219") {
		t.Errorf("Install err = %v, want the version error", err)
	}
	if want := [][]string{{"systemctl", "--version"}}; !reflect.DeepEqual(*calls, want) {
		t.Errorf("ran %q, want %q", *calls, want)
	}
}
