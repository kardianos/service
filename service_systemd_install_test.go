//go:build linux

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdInstallRenderFailureCanBeRetried(t *testing.T) {
	dir := t.TempDir()
	// Keep every systemctl invocation inside the fixture.
	command := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + filepath.Join(dir, "calls") + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(command), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	s := &systemd{Config: &Config{Name: "fixture", Option: KeyValue{
		optionSystemdScript: "{{MissingKey}}",
	}}}
	path := filepath.Join(dir, "fixture.service")
	if err := s.install(path); err == nil || !strings.Contains(err.Error(), "MissingKey") {
		t.Fatalf("render error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed rendering left a unit file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "calls")); !os.IsNotExist(err) {
		t.Fatalf("systemctl ran after failed rendering: %v", err)
	}

	const unit = "[Service]\nExecStart=/usr/bin/fixture\n"
	s.Option[optionSystemdScript] = unit
	if err := s.install(path); err != nil {
		t.Fatalf("retry: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != unit {
		t.Fatalf("unit: %q, %v", got, err)
	}
	calls, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil || string(calls) != "enable fixture.service\ndaemon-reload\n" {
		t.Fatalf("systemctl calls: %q, %v", calls, err)
	}
	files, err := filepath.Glob(filepath.Join(dir, ".fixture.service-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary unit files left behind: %v, %v", files, err)
	}
}

func TestSystemdInstallPreservesExistingUnit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.service")
	const unit = "existing unit\n"
	if err := os.WriteFile(path, []byte(unit), 0600); err != nil {
		t.Fatal(err)
	}
	s := &systemd{Config: &Config{Name: "fixture"}}
	if err := s.install(path); err == nil {
		t.Fatal("install over an existing unit succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != unit {
		t.Fatalf("existing unit changed: %q, %v", got, err)
	}
}
