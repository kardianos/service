package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// newDarwinService builds a launchd service at the given level, with the
// user's home directory stubbed to /home/test.
func newDarwinService(t *testing.T, level ServiceLevel, opts KeyValue) *darwinLaunchdService {
	t.Helper()
	prev := userHomeDir
	userHomeDir = func() (string, error) { return "/home/test", nil }
	t.Cleanup(func() { userHomeDir = prev })

	s, err := darwinSystem{}.New(nil, &Config{Name: "svc", ServiceLevel: level, Option: opts})
	if err != nil {
		t.Fatal(err)
	}
	return s.(*darwinLaunchdService)
}

func TestDarwinLegacyUserService(t *testing.T) {
	tests := []struct {
		level       ServiceLevel
		userService bool
		want        ServiceLevel
		wantErr     bool
	}{
		{level: "", userService: true, want: ServiceLevelUser},
		{level: ServiceLevelUser, userService: true, want: ServiceLevelUser},
		{level: ServiceLevelSystem, userService: true, want: ServiceLevelSystem},
		{level: ServiceLevelAllUsers, userService: true, want: ServiceLevelAllUsers},
		{level: "", userService: false, want: ServiceLevelSystem},
	}
	for _, tt := range tests {
		c := &Config{Name: "svc", ServiceLevel: tt.level, Option: KeyValue{optionUserService: tt.userService}}
		s, err := darwinSystem{}.New(nil, c)
		if (err != nil) != tt.wantErr {
			t.Errorf("level %q, UserService %v: error = %v, wantErr %v", tt.level, tt.userService, err, tt.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if got := s.(*darwinLaunchdService).level; got != tt.want {
			t.Errorf("level %q, UserService %v: level = %q, want %q", tt.level, tt.userService, got, tt.want)
		}
	}
}

func TestDarwinAllUsersOptions(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{name: "UserName", config: Config{UserName: "bob"}, wantErr: true},
		{name: "ChRoot", config: Config{ChRoot: "/jail"}, wantErr: true},
		{name: "LogDirectory", config: Config{Option: KeyValue{optionLogDirectory: "/var/log"}}, wantErr: true},
		{name: "SessionCreate", config: Config{Option: KeyValue{optionSessionCreate: true}}, wantErr: true},
		{name: "SessionCreate false", config: Config{Option: KeyValue{optionSessionCreate: false}}},
		{name: "EnvVars", config: Config{EnvVars: map[string]string{"KEY": "value"}}},
	}
	for _, tt := range tests {
		for _, level := range []ServiceLevel{ServiceLevelAllUsers, ServiceLevelUser, ServiceLevelSystem} {
			c := tt.config
			c.Name = "svc"
			c.ServiceLevel = level
			_, err := darwinSystem{}.New(nil, &c)
			wantErr := tt.wantErr && level == ServiceLevelAllUsers
			if (err != nil) != wantErr {
				t.Errorf("%s at %s: error = %v, wantErr %v", tt.name, level, err, wantErr)
			}
		}
	}
}

func TestDarwinServiceFilePath(t *testing.T) {
	tests := []struct {
		level ServiceLevel
		want  string
	}{
		{ServiceLevelUser, "/home/test/Library/LaunchAgents/svc.plist"},
		{ServiceLevelAllUsers, "/Library/LaunchAgents/svc.plist"},
		{ServiceLevelSystem, "/Library/LaunchDaemons/svc.plist"},
	}
	for _, tt := range tests {
		got, err := newDarwinService(t, tt.level, nil).getServiceFilePath()
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("%s: path = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestDarwinLogDir(t *testing.T) {
	tests := []struct {
		level ServiceLevel
		opts  KeyValue
		want  string
	}{
		{ServiceLevelUser, nil, "/home/test"},
		{ServiceLevelAllUsers, nil, ""},
		{ServiceLevelSystem, nil, defaultDarwinLogDirectory},
		{ServiceLevelUser, KeyValue{optionLogDirectory: "/custom"}, "/custom"},
		{ServiceLevelSystem, KeyValue{optionLogDirectory: "/custom"}, "/custom"},
	}
	for _, tt := range tests {
		got, err := newDarwinService(t, tt.level, tt.opts).logDir()
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("%s %v: logDir = %q, want %q", tt.level, tt.opts, got, tt.want)
		}
	}
}

func TestDarwinLogPaths(t *testing.T) {
	tests := []struct {
		level   ServiceLevel
		opts    KeyValue
		wantOut string
		wantErr string
	}{
		{ServiceLevelUser, nil, "/home/test/svc.out.log", "/home/test/svc.err.log"},
		{ServiceLevelAllUsers, nil, "", ""},
		{ServiceLevelSystem, nil, "/var/log/svc.out.log", "/var/log/svc.err.log"},
		{ServiceLevelSystem, KeyValue{optionLogDirectory: "/custom"}, "/custom/svc.out.log", "/custom/svc.err.log"},
	}
	for _, tt := range tests {
		out, errPath, err := newDarwinService(t, tt.level, tt.opts).getLogPaths()
		if err != nil {
			t.Fatal(err)
		}
		if out != tt.wantOut || errPath != tt.wantErr {
			t.Errorf("%s %v: log paths = %q, %q, want %q, %q", tt.level, tt.opts, out, errPath, tt.wantOut, tt.wantErr)
		}
	}
}

func TestDarwinPlistOmitsEmptyLogPaths(t *testing.T) {
	out, err := launchdTemplate.render(map[string]any{
		"Name":              "svc",
		"Path":              "/bin/svc",
		"Arguments":         []string(nil),
		"ChRoot":            "",
		"UserName":          "",
		"WorkingDirectory":  "",
		"EnvVars":           []string(nil),
		"KeepAlive":         "true",
		"RunAtLoad":         "false",
		"SessionCreate":     "false",
		"StandardOutPath":   "",
		"StandardErrorPath": "",
	}, launchdFuncs)
	if err != nil {
		t.Fatal(err)
	}

	// these must not be written out in the default case. see: logDir:service_darwin.go
	for _, key := range []string{"StandardOutPath", "StandardErrorPath"} {
		if strings.Contains(out, key) {
			t.Errorf("plist contains %s:\n%s", key, out)
		}
	}
}

// stubLibraryDir points darwinLibraryDir and userHomeDir at a temp directory
// holding empty LaunchAgents and LaunchDaemons directories, and returns it.
func stubLibraryDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"LaunchAgents", "LaunchDaemons", "Library/LaunchAgents"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	prevLib, prevHome := darwinLibraryDir, userHomeDir
	darwinLibraryDir = dir
	userHomeDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { darwinLibraryDir, userHomeDir = prevLib, prevHome })
	return dir
}

func TestAllUsersInstallRequiresPrivilege(t *testing.T) {
	stubPrivileged(t, false)
	calls := recordCommands(t, nil)
	s := newDarwinService(t, ServiceLevelAllUsers, nil)
	dir := stubLibraryDir(t)

	if err := s.Install(); !errors.Is(err, ErrRequiresPrivilege) {
		t.Errorf("Install err = %v, want %v", err, ErrRequiresPrivilege)
	}
	if len(*calls) != 0 {
		t.Errorf("ran %q, want no commands", *calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "LaunchAgents", "svc.plist")); !os.IsNotExist(err) {
		t.Errorf("plist was created: %v", err)
	}
}

func TestAllUsersInstallWritesResolvedPath(t *testing.T) {
	stubPrivileged(t, true)
	recordCommands(t, nil)
	stubOwnerStat(t, func(path string) (uint32, uint32, os.FileMode, error) {
		return 0, 0, 0755, nil
	})
	target := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(target, nil, 0755); err != nil {
		t.Fatal(err)
	}
	target, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "app")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	s := newDarwinService(t, ServiceLevelAllUsers, nil)
	s.Executable = link
	dir := stubLibraryDir(t)

	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	plist, err := os.ReadFile(filepath.Join(dir, "LaunchAgents", "svc.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plist), "<string>"+target+"</string>") {
		t.Errorf("plist does not run %s:\n%s", target, plist)
	}
	if strings.Contains(string(plist), link) {
		t.Errorf("plist contains symlink %s:\n%s", link, plist)
	}
}

func TestAllUsersUninstallRequiresPrivilege(t *testing.T) {
	stubPrivileged(t, false)
	calls := recordCommands(t, nil)
	s := newDarwinService(t, ServiceLevelAllUsers, nil)
	dir := stubLibraryDir(t)
	plist := filepath.Join(dir, "LaunchAgents", "svc.plist")
	if err := os.WriteFile(plist, nil, 0644); err != nil {
		t.Fatal(err)
	}

	if err := s.Uninstall(); !errors.Is(err, ErrRequiresPrivilege) {
		t.Errorf("Uninstall err = %v, want %v", err, ErrRequiresPrivilege)
	}
	if len(*calls) != 0 {
		t.Errorf("ran %q, want no commands", *calls)
	}
	if _, err := os.Stat(plist); err != nil {
		t.Errorf("plist was removed: %v", err)
	}
}

func TestAllUsersCommandsRefuseWhenPrivileged(t *testing.T) {
	stubPrivileged(t, true)
	calls := recordCommands(t, nil)
	s := newDarwinService(t, ServiceLevelAllUsers, nil)

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

func TestDarwinAllUsersUninstallSkipsStop(t *testing.T) {
	stubPrivileged(t, true)
	gui := fmt.Sprintf("gui/%d", os.Getuid())
	tests := []struct {
		level    ServiceLevel
		plist    string
		domain   string
		wantStop bool
	}{
		{ServiceLevelAllUsers, "LaunchAgents/svc.plist", gui, false},
		{ServiceLevelSystem, "LaunchDaemons/svc.plist", "system", true},
		{ServiceLevelUser, "Library/LaunchAgents/svc.plist", gui, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			calls := recordCommands(t, nil)
			s := newDarwinService(t, tt.level, nil)
			plist := filepath.Join(stubLibraryDir(t), tt.plist)
			if err := os.WriteFile(plist, nil, 0644); err != nil {
				t.Fatal(err)
			}

			if err := s.Uninstall(); err != nil {
				t.Fatal(err)
			}
			bootout := "launchctl bootout " + tt.domain + "/svc"
			stopped := false
			for _, c := range *calls {
				if strings.Join(c, " ") == bootout {
					stopped = true
				}
			}
			if stopped != tt.wantStop {
				t.Errorf("%s ran = %v, want %v (calls %q)", bootout, stopped, tt.wantStop, *calls)
			}
			if _, err := os.Stat(plist); !os.IsNotExist(err) {
				t.Errorf("plist not removed: %v", err)
			}
		})
	}
}

func TestDarwinStop(t *testing.T) {
	gui := fmt.Sprintf("gui/%d", os.Getuid())
	domains := map[ServiceLevel]string{
		ServiceLevelSystem:   "system",
		ServiceLevelUser:     gui,
		ServiceLevelAllUsers: gui,
	}
	bootoutErr := errors.New("Boot-out failed: 5: Input/output error")
	tests := []struct {
		name    string
		code    int
		err     error
		wantErr error
	}{
		{name: "loaded"},
		{name: "not loaded", code: launchctlNoSuchProcess, err: errors.New("Boot-out failed: 3: No such process")},
		{name: "fails", code: 5, err: bootoutErr, wantErr: bootoutErr},
	}
	for level, domain := range domains {
		for _, tt := range tests {
			t.Run(string(level)+"/"+tt.name, func(t *testing.T) {
				stubPrivileged(t, level == ServiceLevelSystem)
				calls := stubCommands(t, func(argv []string) (int, string, error) {
					return tt.code, "", tt.err
				})

				err := newDarwinService(t, level, nil).Stop()
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Stop err = %v, want %v", err, tt.wantErr)
				}
				want := "launchctl bootout " + domain + "/svc"
				if len(*calls) != 1 || strings.Join((*calls)[0], " ") != want {
					t.Errorf("ran %q, want %q", *calls, want)
				}
			})
		}
	}
}

func TestAllUsersAllCommandsRequirePrivilege(t *testing.T) {
	stubPrivileged(t, false)
	calls := recordCommands(t, nil)
	s := newDarwinService(t, ServiceLevelAllUsers, nil)

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
		s := newDarwinService(t, level, nil)
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

func TestDarwinParseGUIUsers(t *testing.T) {
	const loginwindow = "/System/Library/CoreServices/loginwindow.app/Contents/MacOS/loginwindow"
	tests := []struct {
		name string
		out  string
		want []uint32
	}{
		{"empty", "", nil},
		{"no loginwindow", "  501 /usr/sbin/cfprefsd\n    0 /sbin/launchd\n", nil},
		{"root only", "    0 " + loginwindow + "\n", nil},
		{
			"two users",
			"    1 /sbin/launchd\n" +
				"  501 " + loginwindow + "\n" +
				"    0 " + loginwindow + "\n" +
				"  501 /Applications/Safari.app/Contents/MacOS/Safari\n" +
				"  502 " + loginwindow + "\n" +
				"  501 " + loginwindow + "\n" +
				"  503 /Applications/Not loginwindow.app/Contents/MacOS/helper\n",
			[]uint32{501, 502},
		},
	}
	for _, tt := range tests {
		got := parseGUIUsers(tt.out)
		if fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("%s: parseGUIUsers = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// guiUsersPS is "ps -axo uid=,comm=" output with users 501 and 502 logged in.
const guiUsersPS = "  501 /System/Library/CoreServices/loginwindow.app/Contents/MacOS/loginwindow\n" +
	"  502 /System/Library/CoreServices/loginwindow.app/Contents/MacOS/loginwindow\n"

const psGUIUsers = "ps -axo uid=,comm="

func TestDarwinStopAllCommands(t *testing.T) {
	stubPrivileged(t, true)
	// User 502 has no copy loaded, which is not an error.
	calls := stubCommands(t, func(argv []string) (int, string, error) {
		switch strings.Join(argv, " ") {
		case psGUIUsers:
			return 0, guiUsersPS, nil
		case "launchctl bootout gui/502/svc":
			return launchctlNoSuchProcess, "", errors.New("Boot-out failed: 3: No such process")
		}
		return 0, "", nil
	})

	if err := newDarwinService(t, ServiceLevelAllUsers, nil).StopAll(); err != nil {
		t.Fatal(err)
	}
	want := []string{"launchctl bootout gui/501/svc", "launchctl bootout gui/502/svc", psGUIUsers}
	if got := joinCalls(*calls); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

func TestDarwinStartAllCommands(t *testing.T) {
	stubPrivileged(t, true)
	var mu sync.Mutex
	bootstrapped := false
	calls := stubCommands(t, func(argv []string) (int, string, error) {
		mu.Lock()
		defer mu.Unlock()
		switch strings.Join(argv, " ") {
		case psGUIUsers:
			return 0, guiUsersPS, nil
		case "launchctl bootstrap gui/502 /Library/LaunchAgents/svc.plist":
			bootstrapped = true
		case "launchctl kickstart gui/502/svc":
			if !bootstrapped {
				return launchctlNotFound, "", errors.New(`Could not find service "svc"`)
			}
		}
		return 0, "", nil
	})

	if err := newDarwinService(t, ServiceLevelAllUsers, nil).StartAll(); err != nil {
		t.Fatal(err)
	}
	// not ordered
	want := []string{
		"launchctl bootstrap gui/502 /Library/LaunchAgents/svc.plist",
		"launchctl kickstart gui/501/svc",
		"launchctl kickstart gui/502/svc",
		"launchctl kickstart gui/502/svc",
		psGUIUsers,
	}
	sort.Strings(want)
	if got := joinCalls(*calls); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

func TestDarwinStart(t *testing.T) {
	gui := fmt.Sprintf("gui/%d", os.Getuid())
	domains := map[ServiceLevel]string{
		ServiceLevelSystem:   "system",
		ServiceLevelUser:     gui,
		ServiceLevelAllUsers: gui,
	}
	bootstrapErr := errors.New("Bootstrap failed: 5: Input/output error")
	for level, domain := range domains {
		s := newDarwinService(t, level, nil)
		confPath, err := s.getServiceFilePath()
		if err != nil {
			t.Fatal(err)
		}
		kickstart := "launchctl kickstart " + domain + "/svc"
		bootstrap := "launchctl bootstrap " + domain + " " + confPath

		tests := []struct {
			name      string
			loaded    bool
			bootstrap error
			want      []string
		}{
			{name: "loaded", loaded: true, want: []string{kickstart}},
			{name: "not loaded", want: []string{kickstart, bootstrap, kickstart}},
			{name: "bootstrap fails", bootstrap: bootstrapErr, want: []string{kickstart, bootstrap}},
		}
		for _, tt := range tests {
			t.Run(string(level)+"/"+tt.name, func(t *testing.T) {
				stubPrivileged(t, level == ServiceLevelSystem)
				loaded := tt.loaded
				calls := stubCommands(t, func(argv []string) (int, string, error) {
					switch strings.Join(argv, " ") {
					case bootstrap:
						if tt.bootstrap != nil {
							return 5, "", tt.bootstrap
						}
						loaded = true
					case kickstart:
						if !loaded {
							return launchctlNotFound, "", errors.New(`Could not find service "svc"`)
						}
					}
					return 0, "", nil
				})

				err := s.Start()
				if !errors.Is(err, tt.bootstrap) {
					t.Errorf("Start err = %v, want %v", err, tt.bootstrap)
				}
				var got []string
				for _, c := range *calls {
					got = append(got, strings.Join(c, " "))
				}
				if fmt.Sprint(got) != fmt.Sprint(tt.want) {
					t.Errorf("ran %q, want %q", got, tt.want)
				}
			})
		}
	}
}

func TestDarwinAllCommandsReturnErrors(t *testing.T) {
	stubPrivileged(t, true)
	stubCommands(t, func(argv []string) (int, string, error) {
		if strings.Join(argv, " ") == psGUIUsers {
			return 0, guiUsersPS, nil
		}
		return 1, "", errors.New("failed")
	})
	s := newDarwinService(t, ServiceLevelAllUsers, nil)

	for name, cmd := range map[string]func() error{"StartAll": s.StartAll, "StopAll": s.StopAll} {
		err := cmd()
		if err == nil || !strings.Contains(err.Error(), "gui/501: failed") || !strings.Contains(err.Error(), "gui/502: failed") {
			t.Errorf("%s err = %v, want both users' failures", name, err)
		}
	}
}

func TestDarwinAllCommandsRunConcurrently(t *testing.T) {
	stubPrivileged(t, true)
	var (
		mu      sync.Mutex
		started int
	)
	all := make(chan struct{})
	stubCommands(t, func(argv []string) (int, string, error) {
		if strings.Join(argv, " ") == psGUIUsers {
			return 0, guiUsersPS, nil
		}
		mu.Lock()
		ch := all
		started++
		if started == 2 {
			close(ch)
		}
		mu.Unlock()
		select {
		case <-ch:
			return 0, "", nil
		case <-time.After(5 * time.Second):
			return 1, "", errors.New("users' commands ran one at a time")
		}
	})
	s := newDarwinService(t, ServiceLevelAllUsers, nil)

	for name, cmd := range map[string]func() error{"StartAll": s.StartAll, "StopAll": s.StopAll} {
		mu.Lock()
		started = 0
		all = make(chan struct{})
		mu.Unlock()
		if err := cmd(); err != nil {
			t.Errorf("%s err = %v", name, err)
		}
	}
}
