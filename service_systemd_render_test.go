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

func TestSystemdRenderByLevel(t *testing.T) {
	cases := []struct {
		level   ServiceLevel
		want    []string
		notWant []string
	}{
		{
			level:   ServiceLevelSystem,
			want:    []string{"WantedBy=multi-user.target\n", "User=bob\n", "StandardOutput=file:"},
			notWant: []string{"ConditionUser="},
		},
		{
			level:   ServiceLevelUser,
			want:    []string{"WantedBy=default.target\n", "StandardOutput=file:"},
			notWant: []string{"User=", "ConditionUser=", "multi-user.target"},
		},
		{
			level:   ServiceLevelAllUsers,
			want:    []string{"WantedBy=default.target\n", "\nConditionUser=!@system\n"},
			notWant: []string{"User=bob", "StandardOutput=", "StandardError=", "multi-user.target"},
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
		})
	}
}
