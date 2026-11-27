package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTargetTable(t *testing.T) {
	seen := map[string]bool{}
	for _, tg := range targets {
		if seen[tg.Name] {
			t.Errorf("target %s listed twice", tg.Name)
		}
		seen[tg.Name] = true
		if !contains(initSystems, tg.Init) {
			t.Errorf("target %s: unknown init system", tg.Name)
		}
		if len(tg.Suites) == 0 {
			t.Errorf("target %s runs no suite", tg.Name)
		}
		for _, s := range tg.Suites {
			if _, ok := suites[s]; !ok {
				t.Errorf("target %s: unknown suite %q", tg.Name, s)
			}
		}
		if _, err := os.Stat(filepath.Join("..", "targets", tg.Name, "Dockerfile")); err != nil {
			t.Errorf("target %s: %v", tg.Name, err)
		}
	}
	bins := map[string]string{}
	for name, s := range suites {
		if s.Run == "" {
			t.Errorf("suite %s needs Run", name)
		}
		if other, ok := bins[s.Binary()]; ok && suites[other].Tags != s.Tags {
			t.Errorf("suites %s and %s both build %s", name, other, s.Binary())
		}
		bins[s.Binary()] = name
	}
}

func TestSelectTargets(t *testing.T) {
	all := []Target{
		{Name: "a", Suites: []string{"allusers"}},
		{Name: "b", Suites: []string{"allusers"}},
		{Name: "c", Suites: []string{"other"}},
	}
	names := func(ts []Target) []string {
		var n []string
		for _, t := range ts {
			n = append(n, t.Name)
		}
		return n
	}
	tests := []struct {
		only    []string
		suite   string
		want    []string
		wantErr bool
	}{
		{nil, "", []string{"a", "b", "c"}, false},
		{[]string{"b", "a"}, "", []string{"a", "b"}, false},
		{nil, "allusers", []string{"a", "b"}, false},
		{[]string{"c"}, "allusers", nil, true},
		{[]string{"nope"}, "", nil, true},
		{nil, "nope", nil, true},
	}
	for _, tt := range tests {
		got, err := selectTargets(all, tt.only, tt.suite)
		if (err != nil) != tt.wantErr || !equal(names(got), tt.want) {
			t.Errorf("selectTargets(%v, %q) = %v, %v, want %v, err %v", tt.only, tt.suite, names(got), err, tt.want, tt.wantErr)
		}
		for _, tg := range got {
			if tt.suite != "" && !equal(tg.Suites, []string{tt.suite}) {
				t.Errorf("selectTargets(%v, %q): %s runs %v", tt.only, tt.suite, tg.Name, tg.Suites)
			}
		}
	}
}
