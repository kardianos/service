//go:build linux

package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
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

func TestAllUsersCommandsRefuseWhenPrivileged(t *testing.T) {
	stubPrivileged(t, true)
	calls := recordCommands(t, nil)
	s := newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers})

	commands := map[string]func() error{
		"Start":   s.Start,
		"Stop":    s.Stop,
		"Restart": s.Restart,
		"Status":  func() error { _, err := s.Status(); return err },
	}
	for name, cmd := range commands {
		if err := cmd(); !errors.Is(err, ErrUserContextRequired) {
			t.Errorf("%s err = %v, want %v", name, err, ErrUserContextRequired)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("ran %q, want no commands", *calls)
	}
}

func TestSystemdCommandScope(t *testing.T) {
	stubPrivileged(t, false)
	tests := []struct {
		level ServiceLevel
		scope []string
	}{
		{ServiceLevelSystem, nil},
		{ServiceLevelUser, []string{"--user"}},
		{ServiceLevelAllUsers, []string{"--user"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			argv := func(args ...string) []string {
				return append(append([]string{"systemctl"}, tt.scope...), args...)
			}
			calls := recordCommands(t, map[string]string{
				strings.Join(argv("is-active", "svc.service"), " "): "inactive\n",
			})
			s := newSystemd(t, &Config{ServiceLevel: tt.level})

			if err := s.Start(); err != nil {
				t.Fatal(err)
			}
			if err := s.Stop(); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Status(); !errors.Is(err, ErrNotInstalled) {
				t.Fatalf("Status err = %v, want %v", err, ErrNotInstalled)
			}
			want := [][]string{
				argv("start", "svc.service"),
				argv("stop", "svc.service"),
				argv("is-active", "svc.service"),
				argv("list-unit-files", "-t", "service", "svc.service"),
			}
			if !reflect.DeepEqual(*calls, want) {
				t.Errorf("ran %q, want %q", *calls, want)
			}
		})
	}
}

var _ AllUsersController = (*systemd)(nil)

func TestAllUsersAllCommandsRequirePrivilege(t *testing.T) {
	stubPrivileged(t, false)
	calls := recordCommands(t, nil)
	s := newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers})

	for name, cmd := range map[string]func() error{"StartAll": s.StartAll, "StopAll": s.StopAll} {
		if err := cmd(); !errors.Is(err, ErrRequiresPrivilege) {
			t.Errorf("%s err = %v, want %v", name, err, ErrRequiresPrivilege)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("ran %q, want no commands", *calls)
	}
}

func TestAllUsersAllCommandsOtherLevels(t *testing.T) {
	stubPrivileged(t, true)
	calls := recordCommands(t, nil)
	for _, level := range []ServiceLevel{ServiceLevelSystem, ServiceLevelUser} {
		s := newSystemd(t, &Config{ServiceLevel: level})
		for name, cmd := range map[string]func() error{"StartAll": s.StartAll, "StopAll": s.StopAll} {
			if err := cmd(); !errors.Is(err, ErrUnsupportedOperation) {
				t.Errorf("%s %s err = %v, want %v", level, name, err, ErrUnsupportedOperation)
			}
		}
	}
	if len(*calls) != 0 {
		t.Errorf("ran %q, want no commands", *calls)
	}
}

func TestSystemdParseLoginctlUsers(t *testing.T) {
	alice := loginUser{uid: 1000, name: "alice"}
	bob := loginUser{uid: 1001, name: "bob"}
	tests := []struct {
		name string
		out  string
		want []loginUser
	}{
		{"empty", "", nil},
		{"two users", "1000 alice\n1001 bob\n", []loginUser{alice, bob}},
		{"root", "   0 root\n1000 alice\n", []loginUser{alice}},
		{"duplicate", "1000 alice\n1000 alice\n", []loginUser{alice}},
		{"whitespace", "\n      1000 alice   \n\t1001 bob\n\n", []loginUser{alice, bob}},
		{"old format", "      1000 alice\n       120 gdm\n", []loginUser{alice, {uid: 120, name: "gdm"}}},
		{"linger and state", "1000 alice no  active\n1001 bob   yes lingering\n", []loginUser{alice, bob}},
		{"malformed", "alice 1000\n1000\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLoginctlUsers(tt.out); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseLoginctlUsers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSystemdUserSystemctl(t *testing.T) {
	u := loginUser{uid: 1000, name: "alice"}
	machine := []string{"systemctl", "--user", "-M", "alice@", "start", "svc.service"}
	runuser := []string{
		"runuser", "-u", "alice", "--", "env", "XDG_RUNTIME_DIR=/run/user/1000",
		"systemctl", "--user", "start", "svc.service",
	}
	tests := []struct {
		version int64
		want    []string
	}{
		{260, machine},
		{248, machine},
		{247, runuser},
		{239, runuser},
		{-1, runuser},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.version), func(t *testing.T) {
			if got := systemctlAsUser(tt.version, u, "start", "svc.service"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("userSystemctl() = %q, want %q", got, tt.want)
			}
		})
	}
}

const (
	loginctlUsers  = "loginctl list-users --no-legend"
	loginctlOutput = "1000 alice no active\n1001 bob no active\n1002 carol no closing\n"
	systemctlVer   = "systemctl --version"
)

var managerChecks = map[string]string{
	"systemctl is-active user@1000.service": "active\n",
	"systemctl is-active user@1001.service": "active\n",
	"systemctl is-active user@1002.service": "inactive\n",
}

// withManagerChecks adds the manager checks to calls, sorted.
func withManagerChecks(calls ...string) []string {
	for c := range managerChecks {
		calls = append(calls, c)
	}
	sort.Strings(calls)
	return calls
}

// allUsersVersions are the systemd versions the StartAll and StopAll tests run
// against, one for each way of reaching a user's manager.
var allUsersVersions = []struct {
	version int64
	prefix  func(u loginUser) string
}{
	{239, func(u loginUser) string {
		return fmt.Sprintf("runuser -u %s -- env XDG_RUNTIME_DIR=/run/user/%d systemctl --user", u.name, u.uid)
	}},
	{252, func(u loginUser) string { return "systemctl --user -M " + u.name + "@" }},
}

var (
	testAlice = loginUser{uid: 1000, name: "alice"}
	testBob   = loginUser{uid: 1001, name: "bob"}
)

func TestSystemdStopAllCommands(t *testing.T) {
	stubPrivileged(t, true)
	for _, v := range allUsersVersions {
		t.Run(fmt.Sprint(v.version), func(t *testing.T) {
			// bob has no copy loaded, which is not an error.
			bobStop := v.prefix(testBob) + " stop svc.service"
			calls := stubCommands(t, func(argv []string) (int, string, error) {
				cmd := strings.Join(argv, " ")
				if out, ok := managerChecks[cmd]; ok {
					return 0, out, nil
				}
				switch cmd {
				case loginctlUsers:
					return 0, loginctlOutput, nil
				case systemctlVer:
					return 0, fmt.Sprintf("systemd %d (%d)\n", v.version, v.version), nil
				case bobStop:
					return systemctlNotLoaded, "", errors.New("Unit svc.service not loaded.")
				}
				return 0, "", nil
			})

			if err := newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers}).StopAll(); err != nil {
				t.Fatal(err)
			}
			want := withManagerChecks(
				loginctlUsers,
				systemctlVer,
				v.prefix(testAlice)+" stop svc.service",
				bobStop,
			)
			if got := joinCalls(*calls); !reflect.DeepEqual(got, want) {
				t.Errorf("ran %q, want %q", got, want)
			}
		})
	}
}

func TestSystemdStartAllCommands(t *testing.T) {
	stubPrivileged(t, true)
	for _, v := range allUsersVersions {
		t.Run(fmt.Sprint(v.version), func(t *testing.T) {
			outputs := map[string]string{
				loginctlUsers: loginctlOutput,
				systemctlVer:  fmt.Sprintf("systemd %d (%d)\n", v.version, v.version),
			}
			for c, out := range managerChecks {
				outputs[c] = out
			}
			calls := recordCommands(t, outputs)

			if err := newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers}).StartAll(); err != nil {
				t.Fatal(err)
			}
			want := []string{loginctlUsers, systemctlVer}
			for _, u := range []loginUser{testAlice, testBob} {
				want = append(want,
					v.prefix(u)+" daemon-reload",
					v.prefix(u)+" start svc.service")
			}
			want = withManagerChecks(want...)
			if got := joinCalls(*calls); !reflect.DeepEqual(got, want) {
				t.Errorf("ran %q, want %q", got, want)
			}
			// Each user's reload comes before their start.
			for _, u := range []loginUser{testAlice, testBob} {
				reload, start := -1, -1
				for i, c := range *calls {
					switch strings.Join(c, " ") {
					case v.prefix(u) + " daemon-reload":
						reload = i
					case v.prefix(u) + " start svc.service":
						start = i
					}
				}
				if reload > start {
					t.Errorf("%s: start ran before daemon-reload", u.name)
				}
			}
		})
	}
}

func TestSystemdAllCommandsReturnErrors(t *testing.T) {
	stubPrivileged(t, true)
	stubCommands(t, func(argv []string) (int, string, error) {
		cmd := strings.Join(argv, " ")
		switch {
		case cmd == loginctlUsers:
			return 0, loginctlOutput, nil
		case cmd == systemctlVer:
			return 0, "systemd 252 (252.5-2)\n", nil
		case managerChecks[cmd] != "":
			return 0, managerChecks[cmd], nil
		case strings.Contains(cmd, " stop ") || strings.Contains(cmd, " start "):
			return 1, "", errors.New("Failed to connect to bus")
		}
		return 0, "", nil
	})
	s := newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers})

	for name, cmd := range map[string]func() error{"StartAll": s.StartAll, "StopAll": s.StopAll} {
		err := cmd()
		if err == nil {
			t.Fatalf("%s err = nil, want an error", name)
		}
		want := "alice: Failed to connect to bus\nbob: Failed to connect to bus"
		if err.Error() != want {
			t.Errorf("%s err = %q, want %q", name, err, want)
		}
	}
}
