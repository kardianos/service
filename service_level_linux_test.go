package service

import (
	"errors"
	"testing"
)

func TestUnixBackendsRejectLevels(t *testing.T) {
	backends := []struct {
		name      string
		new       func(i Interface, platform string, c *Config) (Service, error)
		allowUser bool
		allowAll  bool
	}{
		{name: "systemd", new: newSystemdService, allowUser: true, allowAll: true},
		{name: "sysv", new: newSystemVService},
		{name: "upstart", new: newUpstartService},
		{name: "openrc", new: newOpenRCService},
		{name: "rcs", new: newRCSService},
		{name: "procd", new: newProcdService},
	}
	levels := []struct {
		level   ServiceLevel
		wantErr error
	}{
		{level: ""},
		{level: ServiceLevelSystem},
		{level: ServiceLevelUser, wantErr: ErrUnsupportedServiceLevel},
		{level: ServiceLevelAllUsers, wantErr: ErrUnsupportedServiceLevel},
		{level: "bogus"},
	}
	for _, b := range backends {
		for _, l := range levels {
			t.Run(b.name+"/"+string(l.level), func(t *testing.T) {
				_, err := b.new(nil, b.name, &Config{Name: "svc", ServiceLevel: l.level})
				switch {
				case l.level == "bogus":
					if err == nil {
						t.Fatal("err = nil, want an error")
					}
				case l.level == ServiceLevelUser && b.allowUser,
					l.level == ServiceLevelAllUsers && b.allowAll:
					if err != nil {
						t.Fatalf("err = %v, want nil", err)
					}
				case !errors.Is(err, l.wantErr):
					t.Fatalf("err = %v, want %v", err, l.wantErr)
				}
			})
		}
	}
}

func TestSystemdLegacyUserService(t *testing.T) {
	tests := []struct {
		name  string
		level ServiceLevel
		want  ServiceLevel
	}{
		{name: "legacy", want: ServiceLevelUser},
		{name: "system wins", level: ServiceLevelSystem, want: ServiceLevelSystem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := newSystemdService(nil, "linux-systemd", &Config{
				Name:         "svc",
				ServiceLevel: tt.level,
				Option:       KeyValue{optionUserService: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			s := svc.(*systemd)
			if s.level != tt.want {
				t.Errorf("level = %q, want %q", s.level, tt.want)
			}
			if got, want := s.isUserService(), tt.want == ServiceLevelUser; got != want {
				t.Errorf("isUserService() = %v, want %v", got, want)
			}
		})
	}
}
