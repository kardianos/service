//go:build linux

package service

import (
	"errors"
	"fmt"
	"os"
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

// allUsersCommands lets a test run the same checks against StartAll and StopAll.
func allUsersCommands(s *systemd) map[string]func() error {
	return map[string]func() error{"StartAll": s.StartAll, "StopAll": s.StopAll}
}

func TestAllUsersAllCommandsRefused(t *testing.T) {
	tests := []struct {
		name       string
		level      ServiceLevel
		privileged bool
		want       error
	}{
		{"unprivileged", ServiceLevelAllUsers, false, ErrRequiresPrivilege},
		{"system level", ServiceLevelSystem, true, ErrUnsupportedOperation},
		{"user level", ServiceLevelUser, true, ErrUnsupportedOperation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubPrivileged(t, tt.privileged)
			calls := recordCommands(t, nil)
			for name, cmd := range allUsersCommands(newSystemd(t, &Config{ServiceLevel: tt.level})) {
				if err := cmd(); !errors.Is(err, tt.want) {
					t.Errorf("%s err = %v, want %v", name, err, tt.want)
				}
			}
			if len(*calls) != 0 {
				t.Errorf("ran %q, want no commands", *calls)
			}
		})
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

// The StartAll and StopAll tests use one version, since
// TestSystemdUserSystemctl covers how each version reaches a user's manager.
const allUsersVersion = 252

func userPrefix(u loginUser) string {
	return "systemctl --user -M " + u.name + "@"
}

var (
	testAlice = loginUser{uid: 1000, name: "alice"}
	testBob   = loginUser{uid: 1001, name: "bob"}
)

type cmdResult struct {
	code int
	out  string
	err  error
}

// stubAllUsers answers the commands StartAll and StopAll run with
// loginctlOutput, managerChecks and the given systemd version. override can
// replace the result of any command by returning non-nil.
func stubAllUsers(t *testing.T, version int64, override func(cmd string) *cmdResult) *[][]string {
	t.Helper()
	stubPrivileged(t, true)
	return stubCommands(t, func(argv []string) (int, string, error) {
		cmd := strings.Join(argv, " ")
		if override != nil {
			if r := override(cmd); r != nil {
				return r.code, r.out, r.err
			}
		}
		if out, ok := managerChecks[cmd]; ok {
			return 0, out, nil
		}
		switch cmd {
		case loginctlUsers:
			return 0, loginctlOutput, nil
		case systemctlVer:
			return 0, fmt.Sprintf("systemd %d (%d)\n", version, version), nil
		}
		return 0, "", nil
	})
}

func TestSystemdStopAllCommands(t *testing.T) {
	// bob has no copy loaded, which is not an error.
	bobStop := userPrefix(testBob) + " stop svc.service"
	calls := stubAllUsers(t, allUsersVersion, func(cmd string) *cmdResult {
		if cmd == bobStop {
			return &cmdResult{systemctlNotLoaded, "", errors.New("exit status 5: Unit svc.service not loaded.")}
		}
		return nil
	})

	if err := newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers}).StopAll(); err != nil {
		t.Fatal(err)
	}
	want := withManagerChecks(
		loginctlUsers,
		systemctlVer,
		userPrefix(testAlice)+" stop svc.service",
		bobStop,
	)
	if got := joinCalls(*calls); !reflect.DeepEqual(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

func TestSystemdStartAllCommands(t *testing.T) {
	calls := stubAllUsers(t, allUsersVersion, nil)

	if err := newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers}).StartAll(); err != nil {
		t.Fatal(err)
	}
	want := []string{loginctlUsers, systemctlVer}
	for _, u := range []loginUser{testAlice, testBob} {
		want = append(want,
			userPrefix(u)+" daemon-reload",
			userPrefix(u)+" start svc.service")
	}
	want = withManagerChecks(want...)
	if got := joinCalls(*calls); !reflect.DeepEqual(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
	// Each user's reload comes before their start.
	for _, u := range []loginUser{testAlice, testBob} {
		reload := callIndex(*calls, userPrefix(u)+" daemon-reload")
		start := callIndex(*calls, userPrefix(u)+" start svc.service")
		if reload > start {
			t.Errorf("%s: start ran before daemon-reload", u.name)
		}
	}
}

func TestSystemdAllCommandsReturnErrors(t *testing.T) {
	errBus := errors.New("exit status 1: Failed to connect to bus")
	tests := []struct {
		name          string
		fails         func(cmd string) bool
		wantUsers     []string
		wantUserCalls bool
	}{
		{
			name:      "manager check",
			fails:     func(cmd string) bool { return strings.HasPrefix(cmd, "systemctl is-active ") },
			wantUsers: []string{"alice", "bob", "carol"},
		},
		{
			name: "start or stop",
			fails: func(cmd string) bool {
				return strings.Contains(cmd, " stop ") || strings.Contains(cmd, " start ")
			},
			wantUsers:     []string{"alice", "bob"},
			wantUserCalls: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := stubAllUsers(t, allUsersVersion, func(cmd string) *cmdResult {
				if tt.fails(cmd) {
					return &cmdResult{1, "", errBus}
				}
				return nil
			})
			for name, cmd := range allUsersCommands(newSystemd(t, &Config{ServiceLevel: ServiceLevelAllUsers})) {
				*calls = nil
				err := cmd()
				if !errors.Is(err, errBus) {
					t.Fatalf("%s err = %v, want it to wrap %v", name, err, errBus)
				}
				var gotUsers []string
				for _, line := range strings.Split(err.Error(), "\n") {
					user, _, _ := strings.Cut(line, ": ")
					gotUsers = append(gotUsers, user)
				}
				if !reflect.DeepEqual(gotUsers, tt.wantUsers) {
					t.Errorf("%s failed for %q, want %q", name, gotUsers, tt.wantUsers)
				}
				if tt.wantUserCalls {
					continue
				}
				for _, c := range joinCalls(*calls) {
					if strings.Contains(c, "--user") {
						t.Errorf("%s ran %q after the manager check failed", name, c)
					}
				}
			}
		})
	}
}

func TestManagerRunning(t *testing.T) {
	// is-active exits 3 for every state except active, reloading and (v257+) refreshing.
	errExit3 := errors.New("exit status 3")
	errBus := errors.New("exit status 1: Failed to connect to bus")
	tests := []struct {
		name    string
		out     string
		err     error
		want    bool
		wantErr bool
	}{
		{"active", "active\n", nil, true, false},
		{"reloading", "reloading\n", nil, true, false},
		{"refreshing", "refreshing\n", nil, true, false},
		{"activating", "activating\n", errExit3, true, false},
		{"inactive", "inactive\n", errExit3, false, false},
		{"failed", "failed\n", errExit3, false, false},
		{"deactivating", "deactivating\n", errExit3, false, false},
		{"maintenance", "maintenance\n", errExit3, false, false},
		{"query failed", "", errBus, false, true},
		{"no output", "", nil, false, true},
		{"unknown state", "bogus\n", errExit3, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubCommands(t, func([]string) (int, string, error) {
				return 0, tt.out, tt.err
			})
			got, err := managerRunning(testAlice)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.err == errBus && !errors.Is(err, errBus) {
				t.Errorf("err = %v, want it to wrap %v", err, errBus)
			}
			if err != nil && !strings.Contains(err.Error(), "user@1000.service") {
				t.Errorf("err = %q, want it to name the unit", err)
			}
			if got != tt.want {
				t.Errorf("running = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSystemdStatus(t *testing.T) {
	stubPrivileged(t, false)
	errExit3 := errors.New("exit status 3")
	errBus := errors.New("exit status 1: Failed to connect to bus")
	// errAny matches any error except ErrNotInstalled
	errAny := errors.New("any error")
	const (
		listed   = "UNIT FILE   STATE    PRESET\nsvc.service disabled enabled\n\n1 unit files listed.\n"
		unlisted = "UNIT FILE STATE PRESET\n\n0 unit files listed.\n"
	)
	type result struct {
		out string
		err error
	}
	tests := []struct {
		name      string
		isActive  result
		unitFiles result
		want      Status
		wantErr   error
	}{
		{"active", result{"active\n", nil}, result{}, StatusRunning, nil},
		{"reloading", result{"reloading\n", nil}, result{}, StatusRunning, nil},
		{"refreshing", result{"refreshing\n", nil}, result{}, StatusRunning, nil},
		{"activating", result{"activating\n", errExit3}, result{}, StatusRunning, nil},
		{"deactivating", result{"deactivating\n", errExit3}, result{}, StatusStopped, nil},
		{"maintenance", result{"maintenance\n", errExit3}, result{}, StatusStopped, nil},
		{"installed", result{"inactive\n", errExit3}, result{listed, nil}, StatusStopped, nil},
		{"not installed", result{"inactive\n", errExit3}, result{unlisted, nil}, StatusUnknown, ErrNotInstalled},
		{"unknown installed", result{"unknown\n", errExit3}, result{listed, nil}, StatusStopped, nil},
		{"unknown not installed", result{"unknown\n", errExit3}, result{unlisted, nil}, StatusUnknown, ErrNotInstalled},
		{"failed", result{"failed\n", errExit3}, result{}, StatusUnknown, errAny},
		{"unexpected state", result{"bogus\n", errExit3}, result{}, StatusUnknown, errAny},
		{"query failed", result{"", errBus}, result{}, StatusUnknown, errBus},
		{"unit file query failed", result{"inactive\n", errExit3}, result{"", errBus}, StatusUnknown, errBus},
		{"no unit files, non-zero exit", result{"inactive\n", errExit3}, result{unlisted, errors.New("exit status 1")}, StatusUnknown, ErrNotInstalled},
	}
	for _, level := range []ServiceLevel{ServiceLevelSystem, ServiceLevelUser} {
		for _, tt := range tests {
			t.Run(string(level)+"/"+tt.name, func(t *testing.T) {
				stubCommands(t, func(argv []string) (int, string, error) {
					r := tt.isActive
					if strings.Contains(strings.Join(argv, " "), " list-unit-files ") {
						r = tt.unitFiles
					}
					if r.err != nil {
						return 1, r.out, r.err
					}
					return 0, r.out, nil
				})
				got, err := newSystemd(t, &Config{ServiceLevel: level}).Status()
				if tt.wantErr == errAny {
					if err == nil || errors.Is(err, ErrNotInstalled) {
						t.Fatalf("err = %v, want an error other than %v", err, ErrNotInstalled)
					}
				} else if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if got != tt.want {
					t.Errorf("status = %v, want %v", got, tt.want)
				}
			})
		}
	}
}

func TestSystemdStatusNameInListing(t *testing.T) {
	stubPrivileged(t, false)
	stubCommands(t, func(argv []string) (int, string, error) {
		if strings.Contains(strings.Join(argv, " "), " list-unit-files ") {
			return 0, "UNIT FILE STATE PRESET\n\n0 unit files listed.\n", nil
		}
		return 0, "inactive\n", errors.New("exit status 3")
	})
	_, err := newSystemd(t, &Config{Name: "unit"}).Status()
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want %v", err, ErrNotInstalled)
	}
}

// installName keeps the units Install writes apart from real ones.
const installName = "kardianos-install-test"

// newInstallSystemd returns a service named installName at level, whose unit
// file Install can write. The unit file is removed when the test ends.
func newInstallSystemd(t *testing.T, level ServiceLevel) (*systemd, string) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("Install writes under /etc/systemd; run as root in a container")
	}
	t.Setenv("HOME", t.TempDir())
	s := newSystemd(t, &Config{Name: installName, Executable: "/usr/bin/env", ServiceLevel: level})
	path, err := s.configPath()
	if err != nil {
		t.Fatal(err)
	}
	if exists(path) {
		t.Fatalf("%s already exists", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })
	return s, path
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// withoutVersion drops the version queries, which Install may repeat.
func withoutVersion(calls []string) []string {
	var out []string
	for _, c := range calls {
		if c != systemctlVer {
			out = append(out, c)
		}
	}
	return out
}

func TestSystemdInstallRunsCommands(t *testing.T) {
	tests := []struct {
		level      ServiceLevel
		privileged bool
		want       []string
	}{
		{ServiceLevelSystem, true, []string{
			"systemctl enable " + installName + ".service",
			"systemctl daemon-reload",
		}},
		{ServiceLevelUser, false, []string{
			"systemctl --user enable " + installName + ".service",
			"systemctl --user daemon-reload",
		}},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			s, path := newInstallSystemd(t, tt.level)
			stubPrivileged(t, tt.privileged)
			calls := recordCommands(t, map[string]string{systemctlVer: "systemd 252 (252.5-2)\n"})

			if err := s.Install(); err != nil {
				t.Fatal(err)
			}
			if !exists(path) {
				t.Errorf("%s was not written", path)
			}
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if got := withoutVersion(joinCalls(*calls)); !reflect.DeepEqual(got, want) {
				t.Errorf("ran %q, want %q", got, want)
			}
		})
	}
}

func TestAllUsersInstallReloadsManagers(t *testing.T) {
	withRoot := "0 root no active\n" + loginctlOutput
	tests := []struct {
		name     string
		version  int64
		loginctl string
	}{
		{"252", allUsersVersion, loginctlOutput},
		{"247", 247, loginctlOutput},
		{"252 root logged in", allUsersVersion, withRoot},
		{"247 root logged in", 247, withRoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, path := newInstallSystemd(t, ServiceLevelAllUsers)
			unitAtLogin := true
			calls := stubAllUsers(t, tt.version, func(cmd string) *cmdResult {
				switch cmd {
				case loginctlUsers:
					unitAtLogin = exists(path)
					return &cmdResult{0, tt.loginctl, nil}
				case "systemctl is-active user@0.service":
					return &cmdResult{0, "active\n", nil}
				}
				return nil
			})

			if err := s.Install(); err != nil {
				t.Fatal(err)
			}
			if !exists(path) {
				t.Errorf("%s was not written", path)
			}
			if !unitAtLogin {
				t.Error("loginctl ran before the unit was written")
			}
			enable := "systemctl --global enable " + installName + ".service"
			want := []string{loginctlUsers, enable}
			for _, u := range []loginUser{testAlice, testBob} {
				want = append(want, strings.Join(systemctlAsUser(tt.version, u, "daemon-reload"), " "))
			}
			want = withManagerChecks(want...)
			if got := withoutVersion(joinCalls(*calls)); !reflect.DeepEqual(got, want) {
				t.Errorf("ran %q, want %q", got, want)
			}
			loginctlAt := callIndex(*calls, loginctlUsers)
			if loginctlAt >= 0 && callIndex(*calls, enable) > loginctlAt {
				t.Error("loginctl ran before enable")
			}
		})
	}
}

func TestAllUsersInstallReloadErrors(t *testing.T) {
	errBus := errors.New("exit status 1: Failed to connect to bus")
	tests := []struct {
		name      string
		failUsers []loginUser
		wantUsers []string
	}{
		{"one user", []loginUser{testAlice}, []string{"alice"}},
		{"every user", []loginUser{testAlice, testBob}, []string{"alice", "bob"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, path := newInstallSystemd(t, ServiceLevelAllUsers)
			fails := map[string]bool{}
			for _, u := range tt.failUsers {
				fails[userPrefix(u)+" daemon-reload"] = true
			}
			calls := stubAllUsers(t, allUsersVersion, func(cmd string) *cmdResult {
				if fails[cmd] {
					return &cmdResult{1, "", errBus}
				}
				return nil
			})

			err := s.Install()
			if !errors.Is(err, errBus) {
				t.Fatalf("Install err = %v, want it to wrap %v", err, errBus)
			}
			var gotUsers []string
			for _, line := range strings.Split(err.Error(), "\n") {
				user, _, _ := strings.Cut(line, ": ")
				gotUsers = append(gotUsers, user)
			}
			if !reflect.DeepEqual(gotUsers, tt.wantUsers) {
				t.Errorf("Install failed for %q, want %q", gotUsers, tt.wantUsers)
			}
			got := joinCalls(*calls)
			for _, u := range []loginUser{testAlice, testBob} {
				if !contains(got, userPrefix(u)+" daemon-reload") {
					t.Errorf("%s's manager was not reloaded", u.name)
				}
			}
			assertStillInstalled(t, path, got)
		})
	}
}

func TestAllUsersInstallLoginctlError(t *testing.T) {
	s, path := newInstallSystemd(t, ServiceLevelAllUsers)
	errLogind := errors.New("exit status 1: Failed to list users")
	calls := stubAllUsers(t, allUsersVersion, func(cmd string) *cmdResult {
		if cmd == loginctlUsers {
			return &cmdResult{1, "", errLogind}
		}
		return nil
	})

	if err := s.Install(); !errors.Is(err, errLogind) {
		t.Fatalf("Install err = %v, want it to wrap %v", err, errLogind)
	}
	got := joinCalls(*calls)
	if !contains(got, "systemctl --global enable "+installName+".service") {
		t.Errorf("ran %q, want the unit enabled", got)
	}
	for _, c := range got {
		if strings.Contains(c, "daemon-reload") {
			t.Errorf("ran %q after loginctl failed", c)
		}
	}
	assertStillInstalled(t, path, got)
}

// assertStillInstalled checks that a failed Install left the unit file and
// did not disable it.
func assertStillInstalled(t *testing.T, path string, calls []string) {
	t.Helper()
	if !exists(path) {
		t.Errorf("%s was removed", path)
	}
	for _, c := range calls {
		if strings.Contains(c, "disable") {
			t.Errorf("ran %q", c)
		}
	}
}

func TestAllUsersInstallExistingUnit(t *testing.T) {
	s, path := newInstallSystemd(t, ServiceLevelAllUsers)
	const old = "[Unit]\nDescription=old\n"
	if err := os.WriteFile(path, []byte(old), 0644); err != nil {
		t.Fatal(err)
	}
	calls := stubAllUsers(t, allUsersVersion, nil)

	if err := s.Install(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Install err = %v, want the already-exists error", err)
	}
	if data, _ := os.ReadFile(path); string(data) != old {
		t.Errorf("unit file was rewritten:\n%s", data)
	}
	if got := withoutVersion(joinCalls(*calls)); len(got) != 0 {
		t.Errorf("ran %q, want no commands", got)
	}
}

// callIndex returns the index of the first call that ran cmd, or -1.
func callIndex(calls [][]string, cmd string) int {
	for i, c := range calls {
		if strings.Join(c, " ") == cmd {
			return i
		}
	}
	return -1
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
