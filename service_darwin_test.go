// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

//go:build darwin
// +build darwin

package service

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/google/go-cmp/cmp"
)

func TestDarwinLaunchdRender(t *testing.T) {

	base := Config{
		Name:             "ServiceName",
		DisplayName:      "ServiceDisplayName",
		Description:      "ServiceDescription",
		Executable:       "/usr/local/bin/executable",
		WorkingDirectory: "/usr/local/bin",
	}

	makeConfig := func(option KeyValue) *Config {
		cfg := base // copy base
		cfg.Option = option
		return &cfg
	}

	makePlist := func(cfg *Config, m map[string]interface{}) string {
		functions := template.FuncMap{
			"bool": func(v bool) string {
				if v {
					return "true"
				}
				return "false"
			},
		}
		cm := make(map[string]interface{})
		for k, v := range m {
			cm[k] = v
		}
		cm[optionSessionCreate] = optionSessionCreateDefault
		cm[optionKeepAlive] = optionKeepAliveDefault
		cm[optionRunAtLoad] = optionRunAtLoadDefault
		cm["Name"] = cfg.Name
		cm["Path"] = cfg.Executable
		cm["WorkingDirectory"] = cfg.WorkingDirectory
		cm["StandardOutPath"] = "/var/log/" + cfg.Name + ".out.log"
		cm["StandardErrorPath"] = "/var/log/" + cfg.Name + ".err.log"

		tmpl := template.Must(template.New("").Funcs(functions).Parse(launchdConfig))
		var buf bytes.Buffer
		err := tmpl.Execute(&buf, cm)
		if err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	tests := []struct {
		name  string
		cfg   *Config
		plist string
	}{
		{
			name:  "nil options",
			cfg:   makeConfig(nil),
			plist: makePlist(makeConfig(nil), nil),
		},
		{
			name:  "empty options",
			cfg:   makeConfig(map[string]interface{}{}),
			plist: makePlist(makeConfig(nil), nil),
		},
		{
			name: "with ExitTimeOut",
			cfg: makeConfig(map[string]interface{}{
				"ExitTimeOut": 30,
			}),
			plist: makePlist(makeConfig(nil), map[string]interface{}{
				"ExitTimeOut": 30,
			}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := New(nil, tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			dsvc := svc.(*darwinLaunchdService)

			var buf bytes.Buffer
			err = dsvc.render(&buf)
			if err != nil {
				t.Fatal(err)
			}
			plist := buf.String()
			diff := cmp.Diff(tc.plist, plist)
			if diff != "" {
				t.Error(diff)
			}
		})
	}

}
