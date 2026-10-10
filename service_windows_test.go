// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

package service

import (
	"testing"
	"time"

	"golang.org/x/sys/windows/svc/mgr"
)

func TestTimeout(t *testing.T) {
	stopSpan := getStopTimeout()
	t.Log("Max Stop Duration", stopSpan)
}

func TestRecoveryActionsDefaults(t *testing.T) {
	tests := []struct {
		name            string
		option          KeyValue
		wantActionType  int
		wantDelay       time.Duration
		wantResetPeriod uint32
	}{
		{
			name:            "default options when option map is nil",
			option:          nil,
			wantActionType:  mgr.ServiceRestart,
			wantDelay:       1 * time.Second,
			wantResetPeriod: 10,
		},
		{
			name:            "default options when option map is empty",
			option:          KeyValue{},
			wantActionType:  mgr.ServiceRestart,
			wantDelay:       1 * time.Second,
			wantResetPeriod: 10,
		},
		{
			name: "default options when OnFailure is empty string",
			option: KeyValue{
				OnFailure: "",
			},
			wantActionType:  mgr.ServiceRestart,
			wantDelay:       1 * time.Second,
			wantResetPeriod: 10,
		},
		{
			name: "explicit OnFailure restart",
			option: KeyValue{
				OnFailure: OnFailureRestart,
			},
			wantActionType:  mgr.ServiceRestart,
			wantDelay:       1 * time.Second,
			wantResetPeriod: 10,
		},
		{
			name: "explicit OnFailure reboot",
			option: KeyValue{
				OnFailure: OnFailureReboot,
			},
			wantActionType:  mgr.ComputerReboot,
			wantDelay:       1 * time.Second,
			wantResetPeriod: 10,
		},
		{
			name: "explicit OnFailure noaction",
			option: KeyValue{
				OnFailure: OnFailureNoAction,
			},
			wantActionType:  mgr.NoAction,
			wantDelay:       1 * time.Second,
			wantResetPeriod: 10,
		},
		{
			name: "unrecognized OnFailure falls back to restart",
			option: KeyValue{
				OnFailure: "unknown-action",
			},
			wantActionType:  mgr.ServiceRestart,
			wantDelay:       1 * time.Second,
			wantResetPeriod: 10,
		},
		{
			name: "custom delay and reset period without explicit OnFailure",
			option: KeyValue{
				OnFailureDelayDuration: "5s",
				OnFailureResetPeriod:   60,
			},
			wantActionType:  mgr.ServiceRestart,
			wantDelay:       5 * time.Second,
			wantResetPeriod: 60,
		},
		{
			name: "invalid delay falls back to default 1s",
			option: KeyValue{
				OnFailureDelayDuration: "invalid-duration",
			},
			wantActionType:  mgr.ServiceRestart,
			wantDelay:       1 * time.Second,
			wantResetPeriod: 10,
		},
		{
			name: "non-string OnFailure value defaults to restart",
			option: KeyValue{
				OnFailure: 12345,
			},
			wantActionType:  mgr.ServiceRestart,
			wantDelay:       1 * time.Second,
			wantResetPeriod: 10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := &windowsService{
				Config: &Config{
					Option: tc.option,
				},
			}
			actions, resetPeriod := ws.recoveryActions()
			if len(actions) != 1 {
				t.Fatalf("expected 1 recovery action, got %d", len(actions))
			}
			if actions[0].Type != tc.wantActionType {
				t.Errorf("action Type = %v, want %v", actions[0].Type, tc.wantActionType)
			}
			if actions[0].Delay != tc.wantDelay {
				t.Errorf("action Delay = %v, want %v", actions[0].Delay, tc.wantDelay)
			}
			if resetPeriod != tc.wantResetPeriod {
				t.Errorf("resetPeriod = %v, want %v", resetPeriod, tc.wantResetPeriod)
			}
		})
	}
}
