// SPDX-License-Identifier:Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openperouter/openperouter/internal/frr/liveness"
	"github.com/openperouter/openperouter/internal/frrconfig"
)

func TestDaemonMonitor(t *testing.T) {
	zebra := processIdentity{pid: 100, startTime: 1000}
	bgp := processIdentity{pid: 101, startTime: 1001}
	restartedZebra := processIdentity{pid: 200, startTime: 2000}
	restartedBGP := processIdentity{pid: bgp.pid, startTime: 2001}
	type sample struct {
		processes  map[string]processIdentity
		ready      bool
		generation uint64
	}
	tests := []struct {
		name    string
		samples []sample
	}{
		{
			name: "startup waits for a healthy baseline",
			samples: []sample{
				{processes: map[string]processIdentity{"zebra": zebra}},
				{processes: map[string]processIdentity{"zebra": restartedZebra}, ready: true},
				{processes: map[string]processIdentity{"zebra": restartedZebra}, ready: true},
			},
		},
		{
			name: "a replacement between ready samples advances the generation even if the PID is reused",
			samples: []sample{
				{processes: map[string]processIdentity{"bgpd": bgp}, ready: true},
				{processes: map[string]processIdentity{"bgpd": restartedBGP}, ready: true, generation: 1},
				{processes: map[string]processIdentity{"bgpd": restartedBGP}, ready: true, generation: 1},
			},
		},
		{
			name: "an exit is counted once and the daemon can crash again after recovery",
			samples: []sample{
				{processes: map[string]processIdentity{"bgpd": bgp}, ready: true},
				{processes: map[string]processIdentity{"bgpd": {}}, generation: 1},
				{processes: map[string]processIdentity{"bgpd": {}}, generation: 1},
				{processes: map[string]processIdentity{"bgpd": restartedBGP}, ready: true, generation: 1},
				{processes: map[string]processIdentity{"bgpd": {}}, generation: 2},
			},
		},
		{
			name: "a failed observation preserves the baseline and simultaneous restarts coalesce",
			samples: []sample{
				{processes: map[string]processIdentity{"zebra": zebra, "bgpd": bgp}, ready: true},
				{},
				{
					processes:  map[string]processIdentity{"zebra": restartedZebra, "bgpd": restartedBGP},
					ready:      true,
					generation: 1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := newDaemonMonitor()
			instanceID := monitor.state.Load().Token.InstanceID
			require.NotEmpty(t, instanceID)
			for i, sample := range tt.samples {
				monitor.observe(sample.processes, sample.ready)
				want := frrconfig.RecoveryState{
					Token: frrconfig.RecoveryToken{InstanceID: instanceID, Generation: sample.generation},
					Ready: sample.ready,
				}
				require.Equal(t, want, *monitor.state.Load(), "sample %d", i)
			}
		})
	}
}

func TestReadDaemonIdentity(t *testing.T) {
	const pidDir = "/var/run/frr"
	pidPath := filepath.Join(pidDir, "bgpd.pid")
	statPath := "/proc/123/stat"
	procStat := "123 (bgpd) S" + strings.Repeat(" 0", 18) + " 456"
	tests := []struct {
		name        string
		daemonState string
		pids        []string
		stat        string
		want        processIdentity
		wantError   string
	}{
		{
			name:        "reads the PID and process start time",
			daemonState: "Up",
			pids:        []string{"123\n", "123\n"},
			stat:        procStat,
			want:        processIdentity{pid: 123, startTime: 456},
		},
		{
			name:        "a missing PID file while down means the daemon is gone",
			daemonState: "Down",
		},
		{
			name:        "a missing PID file while up is an incomplete sample",
			daemonState: "Up",
			wantError:   "reading daemon PID file: file does not exist",
		},
		{
			name:        "a stale PID file points to an exited process",
			daemonState: "Up",
			pids:        []string{"123", "123"},
		},
		{
			name:        "a zombie is an exited process",
			daemonState: "Up",
			pids:        []string{"123", "123"},
			stat:        strings.Replace(procStat, ") S", ") Z", 1),
		},
		{
			name:        "a reused PID for another command is an exited process",
			daemonState: "Up",
			pids:        []string{"123", "123"},
			stat:        strings.Replace(procStat, "(bgpd)", "(other ) process)", 1),
		},
		{
			name:        "a PID file changing during the probe is an incomplete sample",
			daemonState: "Up",
			pids:        []string{"123", "789"},
			stat:        procStat,
			wantError:   "daemon PID changed during sample",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reads := 0
			readFile := func(path string) ([]byte, error) {
				switch path {
				case pidPath:
					if reads >= len(tt.pids) {
						return nil, os.ErrNotExist
					}
					pid := tt.pids[reads]
					reads++
					return []byte(pid), nil
				case statPath:
					if tt.stat == "" {
						return nil, os.ErrNotExist
					}
					return []byte(tt.stat), nil
				}
				return nil, fmt.Errorf("unexpected path %q", path)
			}
			identity, err := readDaemonIdentity(pidDir, liveness.DaemonState{Name: "bgpd", State: tt.daemonState}, readFile)
			require.Equal(t, tt.want, identity)
			if tt.wantError != "" {
				require.EqualError(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
		})
	}
}
