// These tests install and remove a service named cleanup_test, so they need su

//go:build su

package service_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kardianos/service"
)

const cleanupName = "cleanup_test"

type noopProgram struct{}

func (noopProgram) Start(service.Service) error { return nil }
func (noopProgram) Stop(service.Service) error  { return nil }

func cleanupService(t *testing.T, opt service.KeyValue) service.Service {
	t.Helper()
	s, err := service.New(noopProgram{}, &service.Config{
		Name:       cleanupName,
		Executable: "/bin/sleep",
		Arguments:  []string{"1000"},
		Option:     opt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// cleanupLeftovers returns every file that a Linux backend creates for the
// service. filepath.Glob also finds dangling links.
func cleanupLeftovers(t *testing.T) []string {
	t.Helper()
	var found []string
	for _, pattern := range []string{
		"/etc/init.d/" + cleanupName,
		"/etc/rc?.d/[SK][0-9][0-9]" + cleanupName,
		"/etc/runlevels/*/" + cleanupName,
		"/etc/systemd/system/" + cleanupName + ".service",
		"/etc/systemd/system/*.wants/" + cleanupName + ".service",
	} {
		m, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		found = append(found, m...)
	}
	return found
}

// setupCleanup makes sure that each test starts and ends with no trace of the
// service, also when the backend under test does not clean up.
func setupCleanup(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the file checks are for the Linux backends")
	}
	clean := func() {
		_ = cleanupService(t, nil).Uninstall()
		for _, p := range cleanupLeftovers(t) {
			_ = os.Remove(p)
		}
	}
	clean()
	t.Cleanup(clean)
}

func TestUninstallRemovesEverything(t *testing.T) {
	setupCleanup(t)
	s := cleanupService(t, nil)
	if err := s.Install(); err != nil {
		t.Fatal("Install:", err)
	}
	if len(cleanupLeftovers(t)) == 0 {
		t.Fatal("Install created none of the expected files")
	}
	if err := s.Uninstall(); err != nil {
		t.Fatal("Uninstall:", err)
	}
	if left := cleanupLeftovers(t); len(left) > 0 {
		t.Errorf("after Uninstall, these remain: %q", left)
	}
}

func TestFailedInstall(t *testing.T) {
	setupCleanup(t)
	option, ok := map[string]string{
		"linux-systemd": "SystemdScript",
		"linux-openrc":  "OpenRCScript",
		"unix-systemv":  "SysvScript",
	}[service.Platform()]
	if !ok {
		t.Skipf("no custom script option for %s", service.Platform())
	}

	broken := cleanupService(t, service.KeyValue{option: "{{"})
	if err := broken.Install(); err == nil {
		t.Fatal("Install with an unparsable script succeeded")
	}
	if left := cleanupLeftovers(t); len(left) > 0 {
		t.Errorf("after a failed Install, these remain: %q", left)
	}

	s := cleanupService(t, nil)
	if err := s.Install(); err != nil {
		t.Fatal("Install after a failed Install:", err)
	}
	if err := s.Uninstall(); err != nil {
		t.Fatal("Uninstall:", err)
	}
}
