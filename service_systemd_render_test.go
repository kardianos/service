//go:build linux

package service

import (
	"strings"
	"testing"
)

// renderSystemd renders the built-in template with the data Install would use
// for c, on a systemd with file output support.
func renderSystemd(t *testing.T, c *Config, path string) string {
	t.Helper()
	recordCommands(t, map[string]string{"systemctl --version": "systemd 252 (252.5-2)"})
	svc, err := newSystemdService(nil, "linux-systemd", c)
	if err != nil {
		t.Fatal(err)
	}
	out, err := systemdTemplate.render(svc.(*systemd).templateData(path), tfs)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

const systemdGoldenMinimal = `[Unit]
Description=A test
ConditionFileIsExecutable=/usr/bin/svc\x20app

[Service]
StartLimitInterval=5
StartLimitBurst=10
ExecStart=/usr/bin/svc\x20app
ExecReload=/bin/kill -SIGUSR1 "$MAINPID"
PIDFile="/var/run/svc.pid"
StandardOutput=file:/var/log/svc.out
StandardError=file:/var/log/svc.err
LimitNOFILE=4096
Restart=always
SuccessExitStatus=1 2
RestartSec=120
EnvironmentFile=-/etc/sysconfig/svc

[Install]
WantedBy=multi-user.target
`

const systemdGoldenFull = `[Unit]
Description=A test service
ConditionFileIsExecutable=/usr/bin/svc\x20app
After=network.target
Requires=foo.service

[Service]
StartLimitInterval=5
StartLimitBurst=10
ExecStart=/usr/bin/svc\x20app "-a" "with space"
RootDirectory="/jail"
WorkingDirectory=/opt/my\x20app
User=bob
ExecReload=/bin/kill -SIGUSR1 "$MAINPID"
PIDFile="/var/run/svc.pid"
StandardOutput=file:/var/log/svc.out
StandardError=file:/var/log/svc.err
LimitNOFILE=4096
Restart=always
SuccessExitStatus=1 2
RestartSec=120
EnvironmentFile=-/etc/sysconfig/svc

Environment=A=1
Environment=B=2
[Install]
WantedBy=multi-user.target
`

func TestSystemdRender(t *testing.T) {
	options := KeyValue{
		optionReloadSignal:      "SIGUSR1",
		optionPIDFile:           "/var/run/svc.pid",
		optionLimitNOFILE:       4096,
		optionRestart:           "always",
		optionSuccessExitStatus: "1 2",
		optionLogOutput:         true,
	}
	cases := []struct {
		name string
		c    *Config
		want string
	}{
		{"minimal", &Config{Name: "svc", Description: "A test", Option: options}, systemdGoldenMinimal},
		{"full", &Config{
			Name:             "svc",
			Description:      "A test service",
			UserName:         "bob",
			Arguments:        []string{"-a", "with space"},
			Dependencies:     []string{"After=network.target", "Requires=foo.service"},
			WorkingDirectory: "/opt/my app",
			ChRoot:           "/jail",
			EnvVars:          map[string]string{"B": "2", "A": "1"},
			Option:           options,
		}, systemdGoldenFull},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderSystemd(t, tc.c, "/usr/bin/svc app")
			if got != tc.want {
				t.Errorf("output mismatch\n--- got ---\n%q\n--- want ---\n%q", got, tc.want)
			}
		})
	}
}

// unitSection returns the body of section name in a rendered unit.
func unitSection(unit, name string) string {
	_, after, ok := strings.Cut(unit, "["+name+"]\n")
	if !ok {
		return ""
	}
	body, _, _ := strings.Cut(after, "\n[")
	return "\n" + body
}

func TestSystemdRenderByLevel(t *testing.T) {
	cases := []struct {
		level     ServiceLevel
		want      []string
		notWant   []string
		inUnit    []string
		inInstall []string
	}{
		{
			level:     ServiceLevelSystem,
			want:      []string{"User=bob\n", "StandardOutput=file:"},
			notWant:   []string{"ConditionUser=", "graphical-session.target"},
			inInstall: []string{"\nWantedBy=multi-user.target\n"},
		},
		{
			level:     ServiceLevelUser,
			want:      []string{"StandardOutput=file:"},
			notWant:   []string{"User=", "ConditionUser=", "multi-user.target", "graphical-session.target"},
			inInstall: []string{"\nWantedBy=default.target\n"},
		},
		{
			level: ServiceLevelAllUsers,
			notWant: []string{
				"User=bob", "StandardOutput=", "StandardError=",
				"multi-user.target", "default.target", "Requisite=",
			},
			inUnit: []string{
				"\nConditionUser=!@system\n",
				"\nPartOf=graphical-session.target\n",
				"\nAfter=graphical-session.target\n",
			},
			inInstall: []string{"\nWantedBy=graphical-session.target\n"},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.level), func(t *testing.T) {
			c := &Config{Name: "svc", ServiceLevel: tc.level, Option: KeyValue{optionLogOutput: true}}
			if tc.level != ServiceLevelAllUsers {
				c.UserName = "bob"
			}
			got := renderSystemd(t, c, "/usr/bin/svc")
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in\n%s", w, got)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q in\n%s", w, got)
				}
			}
			for section, wants := range map[string][]string{"Unit": tc.inUnit, "Install": tc.inInstall} {
				body := unitSection(got, section)
				for _, w := range wants {
					if !strings.Contains(body, w) {
						t.Errorf("missing %q in [%s] of\n%s", w, section, got)
					}
				}
			}
		})
	}
}
