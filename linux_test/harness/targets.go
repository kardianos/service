package main

import (
	"fmt"

	"golang.org/x/exp/slices"
)

type Suite struct {
	Tags    string
	Run     string
	Testsvc bool
}

func (s Suite) Binary() string {
	if s.Tags == "" {
		return "service.test"
	}
	return s.Tags + ".test"
}

type Target struct {
	// Name is also the directory of the Dockerfile in linux_test/targets.
	Name   string
	Init   *InitSystem
	Suites []string
	// Some images cannot boot on all hosts. That is not a defect in the
	// service, so a failure to boot gives SKIP.
	BestEffort bool
	// Old systemd cannot boot on cgroup v2. Skip before the build to save the
	// build and the boot timeout.
	RequiresCgroupV1 bool
}

var suites = map[string]Suite{
	"unit":      {Run: "."},
	"lifecycle": {Tags: "su", Run: "^(TestInstallRunRestartStopRemove|TestExpectedPlatform|TestUninstallRemovesEverything|TestFailedInstall)$"},
	"allusers":  {Tags: "systemd_container", Run: "^TestContainer", Testsvc: true},
}

var targets = []Target{
	{Name: "systemd-debian12", Init: systemd, Suites: []string{"unit", "lifecycle", "allusers"}},
	{Name: "systemd-fedora", Init: systemd, Suites: []string{"unit", "lifecycle", "allusers"}},
	{Name: "systemd-ubuntu2004", Init: systemd, Suites: []string{"unit", "lifecycle", "allusers"}},
	{Name: "systemd-rocky8", Init: systemd, Suites: []string{"unit", "lifecycle", "allusers"}, BestEffort: true},
	{Name: "systemd-centos7", Init: systemd, Suites: []string{"unit", "allusers"}, BestEffort: true, RequiresCgroupV1: true},
	{Name: "openrc-alpine", Init: openrc, Suites: []string{"unit", "lifecycle"}},
	{Name: "sysv-debian12", Init: sysv, Suites: []string{"unit", "lifecycle"}, BestEffort: true},
}

func selectTargets(all []Target, only []string, suite string) ([]Target, error) {
	if suite != "" {
		if _, ok := suites[suite]; !ok {
			return nil, fmt.Errorf("unknown suite %q", suite)
		}
	}
	for _, name := range only {
		if !slices.ContainsFunc(all, func(t Target) bool { return t.Name == name }) {
			return nil, fmt.Errorf("unknown target %q", name)
		}
	}
	var sel []Target
	for _, t := range all {
		if len(only) > 0 && !slices.Contains(only, t.Name) {
			continue
		}
		if suite != "" {
			if !slices.Contains(t.Suites, suite) {
				continue
			}
			t.Suites = []string{suite}
		}
		sel = append(sel, t)
	}
	if len(sel) == 0 {
		return nil, fmt.Errorf("no target runs suite %q", suite)
	}
	return sel, nil
}
