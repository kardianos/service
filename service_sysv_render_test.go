//go:build linux

package service

import (
	"strings"
	"testing"
)

// renderSysv mirrors the data map built in (*sysv).Install and renders the
// given script template through the engine, so the render can be exercised
// without touching the filesystem.
func renderSysv(script string, c *Config, path, logDir string) (string, error) {
	data := map[string]any{
		"Description":      c.Description,
		"DisplayName":      c.DisplayName,
		"Name":             c.Name,
		"Path":             path,
		"Arguments":        c.Arguments,
		"UserName":         c.UserName,
		"ChRoot":           c.ChRoot,
		"WorkingDirectory": c.WorkingDirectory,
		"LogDirectory":     logDir,
		"EnvVars":          envVars(c.EnvVars, func(k, v string) string { return "export " + k + "=" + v }),
	}
	return renderTemplate(script, data, tfs)
}

func TestSysvRenderBuiltin(t *testing.T) {
	c := &Config{
		Name:        "svc",
		DisplayName: "svc display",
		Description: "A test",
		Arguments:   []string{"-a", "with space"},
	}
	out, err := renderSysv(sysvScript, c, "/usr/bin/svc", "/var/log")
	if err != nil {
		t.Fatalf("render built-in sysv script: %v", err)
	}
	for _, want := range []string{"svc display", `"-a" "with space"`, "/usr/bin/svc", "/var/log"} {
		if !strings.Contains(out, want) {
			t.Errorf("built-in sysv script output missing %q", want)
		}
	}
}

// TestSysvRenderCustomScriptConfigKeys is a regression test for Name,
// UserName and ChRoot going missing from the sysv template data. Before the
// template engine change these were reachable through the embedded *Config,
// and custom SysvScript templates use them: a script rendering
// {{UserName|cmd}} into a start-stop-daemon --chuid clause silently loses
// its privilege drop when the key is absent.
func TestSysvRenderCustomScriptConfigKeys(t *testing.T) {
	const custom = `#!/bin/sh
NAME={{Name|cmd}}
{{if UserName}}CHUID="--chuid {{UserName|cmd}}"{{end}}
{{if ChRoot}}CHROOT="--chroot {{ChRoot|cmd}}"{{end}}
exec {{Path}} {{range Arguments}} {{.|cmd}}{{end}}
`
	c := &Config{
		Name:      "svc",
		UserName:  "svcuser",
		ChRoot:    "/jail",
		Arguments: []string{"run"},
	}
	out, err := renderSysv(custom, c, "/usr/bin/svc", "/var/log")
	if err != nil {
		t.Fatalf("render custom sysv script: %v", err)
	}
	for _, want := range []string{
		`NAME="svc"`,
		`CHUID="--chuid "svcuser""`,
		`CHROOT="--chroot "/jail""`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("custom sysv script output missing %q\ngot:\n%s", want, out)
		}
	}
}
