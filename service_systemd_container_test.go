// These end-to-end tests change the users and the systemd units of the host.
// Thus they run only in the temporary containers of linux_test/harness.

//go:build linux && systemd_container

package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	ctrName     = "testsvc"
	ctrExe      = "/usr/local/bin/testsvc"
	ctrUnit     = "/etc/systemd/user/testsvc.service"
	ctrWantsDir = "/etc/systemd/user/default.target.wants"
	ctrTestsvc  = "/opt/test/testsvc"
	// The harness writes this file after boot. The guard does not use
	// /proc/1/environ, because the helper runs as alice and cannot read it.
	ctrMarker = "/run/kardianos-service-test"

	// ctrWait is how long to wait for something asynchronous, such as a user
	// manager starting. ctrQuiet is how long something must not happen.
	ctrWait  = 10 * time.Second
	ctrQuiet = 3 * time.Second
	// ctrResetWait allows for logind keeping a user for UserStopDelaySec
	// (10 s by default) after their manager stops.
	ctrResetWait = 30 * time.Second

	// helperEnv makes TestContainerHelperUserCommands run, as the user.
	helperEnv = "SYSTEMD_CONTAINER_HELPER"
)

var ctrUsers = []string{"alice", "bob", "greeter"}

func requireContainer(t *testing.T) {
	t.Helper()
	comm, _ := os.ReadFile("/proc/1/comm")
	if !exists(ctrMarker) || strings.TrimSpace(string(comm)) != "systemd" {
		t.Skip("not in a container booted by linux_test/harness")
	}
}

// setupContainer resets the container to no unit, no lingering users and no
// markers, before and after the test, and returns an AllUsers service for
// testsvc. It skips the test on systemd too old for AllUsers.
func setupContainer(t *testing.T) *systemd {
	t.Helper()
	requireContainer(t)
	if os.Geteuid() != 0 {
		t.Fatal("container tests run as root")
	}
	ensureUsers(t)
	installTestsvc(t, ctrExe, "root")
	s := ctrService(t, ctrExe)
	if v := s.getSystemdVersion(); v != -1 && v < minAllUsersVersion {
		t.Skipf("systemd %d is older than %d", v, minAllUsersVersion)
	}
	resetContainer(t, s)
	t.Cleanup(func() { resetContainer(t, s) })
	return s
}

func ctrService(t *testing.T, exe string) *systemd {
	t.Helper()
	svc, err := newSystemdService(nil, "linux-systemd", &Config{
		Name:         ctrName,
		Executable:   exe,
		ServiceLevel: ServiceLevelAllUsers,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc.(*systemd)
}

func ensureUsers(t *testing.T) {
	t.Helper()
	for _, name := range ctrUsers {
		if _, err := user.Lookup(name); err == nil {
			continue
		}
		args := []string{"-m", name}
		if name == "greeter" {
			// A system user, standing in for gdm or lightdm.
			args = []string{"-r", "-m", name}
		}
		mustRun(t, "useradd", args...)
	}
}

// installTestsvc copies the testsvc binary to path, owned by owner.
func installTestsvc(t *testing.T, path, owner string) {
	t.Helper()
	data, err := os.ReadFile(ctrTestsvc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0755); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "chown", owner+":", path)
}

func resetContainer(t *testing.T, s *systemd) {
	t.Helper()
	if _, err := os.Stat(ctrUnit); err == nil {
		if err := s.Uninstall(); err != nil {
			t.Logf("reset: Uninstall: %v", err)
			os.Remove(ctrUnit)
			os.Remove(filepath.Join(ctrWantsDir, ctrName+".service"))
		}
	}
	for _, name := range ctrUsers {
		u := lookupUser(t, name)
		tryRun("loginctl", "disable-linger", name)
		tryRun("systemctl", "stop", userManager(u))
	}
	// Until logind lets go of a user, enable-linger doesn't start a new
	// manager for them.
	waitForWithin(t, ctrResetWait, "every user to leave logind", func() bool {
		for _, name := range ctrUsers {
			if _, err := tryRun("loginctl", "show-user", name); err == nil {
				return false
			}
		}
		return true
	})
	markers, _ := filepath.Glob("/tmp/testsvc-*")
	for _, m := range markers {
		os.Remove(m)
	}
	os.Remove("/home/alice/testsvc")
}

func lookupUser(t *testing.T, name string) loginUser {
	t.Helper()
	u, err := user.Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	return loginUser{uid: uint32(uid), name: name}
}

func userManager(u loginUser) string {
	return fmt.Sprintf("user@%d.service", u.uid)
}

func managerActive(u loginUser) bool {
	out, _ := tryRun("systemctl", "is-active", userManager(u))
	return out == "active"
}

// linger starts u's user manager, as a login would, and waits until both
// ways of reaching it are ready.
func linger(t *testing.T, s *systemd, u loginUser) {
	t.Helper()
	mustRun(t, "loginctl", "enable-linger", u.name)
	runtime := fmt.Sprintf("/run/user/%d", u.uid)
	needBus := s.getSystemdVersion() >= minMachineUserVersion
	waitFor(t, u.name+"'s user manager", func() bool {
		if !managerActive(u) || !exists(runtime+"/systemd/private") {
			return false
		}
		return !needBus || exists(runtime+"/bus")
	})
}

// userCtl runs "systemctl --user args" in u's manager, the way StartAll does.
func userCtl(s *systemd, u loginUser, args ...string) (string, error) {
	argv := systemctlAsUser(s.getSystemdVersion(), u, args...)
	return tryRun(argv[0], argv[1:]...)
}

func markerPath(name, event string) string {
	return "/tmp/testsvc-" + name + "." + event
}

// markerPID returns the PID in a marker, or 0 if there is none.
func markerPID(name, event string) int {
	data, err := os.ReadFile(markerPath(name, event))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(string(data))
	return pid
}

func alive(pid int) bool {
	return pid > 0 && exists(fmt.Sprintf("/proc/%d", pid))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// running waits for name's copy to start, and returns its PID.
func running(t *testing.T, name string) int {
	t.Helper()
	var pid int
	waitFor(t, name+"'s copy to start", func() bool {
		pid = markerPID(name, "started")
		return alive(pid)
	})
	return pid
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	waitForWithin(t, ctrWait, what, cond)
}

func waitForWithin(t *testing.T, limit time.Duration, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(limit); ; {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", limit, what)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func never(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(ctrQuiet); time.Now().Before(deadline); {
		if cond() {
			t.Fatalf("%s", what)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func mustRun(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := tryRun(name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v: %s", name, strings.Join(args, " "), err, out)
	}
	return out
}

func tryRun(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func TestContainerAllUsersInstall(t *testing.T) {
	s := setupContainer(t)
	alice, greeter := lookupUser(t, "alice"), lookupUser(t, "greeter")

	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(ctrUnit)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0644 {
		t.Errorf("unit mode = %v, want 0644", fi.Mode().Perm())
	}
	if _, err := os.Lstat(filepath.Join(ctrWantsDir, ctrName+".service")); err != nil {
		t.Errorf("not enabled: %v", err)
	}

	linger(t, s, alice)
	running(t, "alice")

	linger(t, s, greeter)
	never(t, "greeter got a copy", func() bool { return exists(markerPath("greeter", "started")) })
}

func TestContainerAllUsersOwnership(t *testing.T) {
	setupContainer(t)
	const exe = "/home/alice/testsvc"
	installTestsvc(t, exe, "alice")

	err := ctrService(t, exe).Install()
	if err == nil || !strings.Contains(err.Error(), exe) {
		t.Errorf("Install err = %v, want one naming %s", err, exe)
	}
	if exists(ctrUnit) {
		t.Error("unit file was written")
	}
}

func TestContainerAllUsersStartAll(t *testing.T) {
	s := setupContainer(t)
	alice, bob := lookupUser(t, "alice"), lookupUser(t, "bob")

	// alice's manager starts before the unit exists, so it doesn't start her
	// copy; bob's starts after, so it does.
	linger(t, s, alice)
	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	linger(t, s, bob)
	bobPID := running(t, "bob")
	never(t, "alice got a copy before StartAll", func() bool { return exists(markerPath("alice", "started")) })

	if err := s.StartAll(); err != nil {
		t.Fatal(err)
	}
	running(t, "alice")
	if got := markerPID("bob", "started"); got != bobPID || !alive(bobPID) {
		t.Errorf("bob's copy was restarted: PID %d, then %d", bobPID, got)
	}
}

func TestContainerAllUsersStopAll(t *testing.T) {
	s := setupContainer(t)
	users := []loginUser{lookupUser(t, "alice"), lookupUser(t, "bob")}

	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	pids := map[string]int{}
	for _, u := range users {
		linger(t, s, u)
		pids[u.name] = running(t, u.name)
	}

	if err := s.StopAll(); err != nil {
		t.Fatal(err)
	}
	// No waiting: StopAll returns once every copy has exited.
	for _, u := range users {
		if alive(pids[u.name]) {
			t.Errorf("%s's copy is still running after StopAll", u.name)
		}
		if markerPID(u.name, "stopped") != pids[u.name] {
			t.Errorf("%s's copy exited without running Stop", u.name)
		}
		// Restart=always would show as "activating" until RestartSec passes.
		if out, _ := userCtl(s, u, "is-active", s.unitName()); out != "inactive" {
			t.Errorf("%s's unit is %q after StopAll, want inactive", u.name, out)
		}
	}
}

func TestContainerAllUsersUninstall(t *testing.T) {
	s := setupContainer(t)
	alice, bob := lookupUser(t, "alice"), lookupUser(t, "bob")

	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	linger(t, s, alice)
	pid := running(t, "alice")

	if err := s.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if exists(ctrUnit) {
		t.Error("unit file is still there")
	}
	never(t, "Uninstall stopped alice's copy", func() bool { return !alive(pid) })

	// bob's manager starts after Uninstall, so it never loads the unit, and
	// stopping it there fails with "not loaded".
	linger(t, s, bob)
	if out, err := userCtl(s, bob, "stop", s.unitName()); err == nil {
		t.Errorf("stop in bob's manager succeeded, want the not-loaded error: %s", out)
	} else if code, _ := isExitError(err); code != systemctlNotLoaded {
		t.Errorf("stop in bob's manager exited %d, want %d: %s", code, systemctlNotLoaded, out)
	}

	if err := s.StopAll(); err != nil {
		t.Fatal(err)
	}
	if alive(pid) {
		t.Error("alice's copy is still running after StopAll")
	}
}

func TestContainerAllUsersUserCommands(t *testing.T) {
	s := setupContainer(t)
	alice := lookupUser(t, "alice")

	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	linger(t, s, alice)
	running(t, "alice")

	for name, cmd := range map[string]func() error{
		"Start":   s.Start,
		"Stop":    s.Stop,
		"Restart": s.Restart,
		"Status":  func() error { _, err := s.Status(); return err },
	} {
		if err := cmd(); !errors.Is(err, ErrUserContextRequired) {
			t.Errorf("as root, %s err = %v, want %v", name, err, ErrUserContextRequired)
		}
	}

	testBin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out, err := tryRun("runuser", "-u", alice.name, "--", "env",
		fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", alice.uid), helperEnv+"=1",
		testBin, "-test.v", "-test.run", "^TestContainerHelperUserCommands$")
	t.Logf("as alice:\n%s", out)
	if err != nil || !strings.Contains(out, "--- PASS: TestContainerHelperUserCommands") {
		t.Errorf("user commands failed as alice: %v", err)
	}
}

// TestContainerHelperUserCommands runs as alice, started by
// TestContainerAllUsersUserCommands, with her copy running.
func TestContainerHelperUserCommands(t *testing.T) {
	if os.Getenv(helperEnv) == "" {
		t.Skip("run by TestContainerAllUsersUserCommands")
	}
	requireContainer(t)
	s := ctrService(t, ctrExe)

	status := func(want Status) {
		t.Helper()
		if got, err := s.Status(); err != nil || got != want {
			t.Fatalf("Status() = %v, %v, want %v", got, err, want)
		}
	}
	status(StatusRunning)
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	status(StatusStopped)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	status(StatusRunning)
	if err := s.Restart(); err != nil {
		t.Fatal(err)
	}
	status(StatusRunning)
}

func TestContainerAllUsersPath(t *testing.T) {
	s := setupContainer(t)
	alice := lookupUser(t, "alice")

	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	linger(t, s, alice)

	var (
		mu    sync.Mutex
		calls [][]string
	)
	prev := runCommand
	runCommand = func(command string, readStdout bool, arguments ...string) (int, string, error) {
		mu.Lock()
		calls = append(calls, append([]string{command}, arguments...))
		mu.Unlock()
		return prev(command, readStdout, arguments...)
	}
	t.Cleanup(func() { runCommand = prev })

	if err := s.StartAll(); err != nil {
		t.Fatal(err)
	}
	running(t, "alice")

	version := s.getSystemdVersion()
	wantMachine := version >= minMachineUserVersion
	for _, c := range calls {
		if !contains(c, "start") {
			continue
		}
		usedMachine := c[0] == "systemctl" && contains(c, "-M")
		t.Logf("systemd %d: StartAll ran %q", version, c)
		if usedMachine != wantMachine {
			t.Errorf("systemd %d: StartAll used -M = %v, want %v", version, usedMachine, wantMachine)
		}
	}

	// An unknown version falls back to runuser, so it must work everywhere.
	argv := systemctlAsUser(-1, alice, "is-active", s.unitName())
	if out, err := tryRun(argv[0], argv[1:]...); out != "active" {
		t.Errorf("runuser form: is-active = %q, %v, want active", out, err)
	}
}

func TestContainerAllUsersMinVersion(t *testing.T) {
	requireContainer(t)
	s := ctrService(t, ctrExe)
	v := s.getSystemdVersion()
	if v == -1 || v >= minAllUsersVersion {
		t.Skipf("systemd %d supports AllUsers", v)
	}
	installTestsvc(t, ctrExe, "root")

	err := s.Install()
	if err == nil || !strings.Contains(err.Error(), strconv.Itoa(minAllUsersVersion)) {
		t.Errorf("Install err = %v, want the version error", err)
	}
	if exists(ctrUnit) {
		t.Error("unit file was written")
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
