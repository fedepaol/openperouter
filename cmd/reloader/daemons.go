// SPDX-License-Identifier:Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/openperouter/openperouter/internal/frr/liveness"
	"github.com/openperouter/openperouter/internal/frr/vtysh"
	"github.com/openperouter/openperouter/internal/frrconfig"
)

const daemonSampleInterval = 2 * time.Second

type daemonMonitor struct {
	state atomic.Pointer[frrconfig.RecoveryState]
	// A zero identity means we have already observed and counted this process exiting.
	processes map[string]processIdentity
}

type processIdentity struct {
	pid       int
	startTime uint64
}

func newDaemonMonitor() *daemonMonitor {
	monitor := &daemonMonitor{}
	monitor.state.Store(&frrconfig.RecoveryState{
		Token: frrconfig.RecoveryToken{InstanceID: rand.Text()},
	})
	return monitor
}

func (m *daemonMonitor) run(ctx context.Context, watchfrrPIDFile string, cli vtysh.Cli) {
	ticker := time.NewTicker(daemonSampleInterval)
	defer ticker.Stop()

	for ctx.Err() == nil {
		processes, ready := sampleDaemons(ctx, watchfrrPIDFile, cli)
		m.observe(processes, ready)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *daemonMonitor) serveState(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(m.state.Load()); err != nil {
		slog.DebugContext(req.Context(), "failed to write FRR recovery state", "error", err)
	}
}

func (m *daemonMonitor) observe(processes map[string]processIdentity, ready bool) {
	state := *m.state.Load()
	state.Ready = ready
	defer m.state.Store(&state)

	if m.processes == nil {
		if ready {
			m.processes = processes
		}
		return
	}

	restarted := false
	for name, identity := range processes {
		previous := m.processes[name]
		if previous.pid != 0 && previous != identity {
			restarted = true
		}
		m.processes[name] = identity
	}
	if restarted {
		state.Token.Generation++
	}
}

func sampleDaemons(ctx context.Context, watchfrrPIDFile string, cli vtysh.Cli) (map[string]processIdentity, bool) {
	daemons, err := liveness.WatchFRRDaemonStates(watchfrrPIDFile, cli, liveness.ProcessAlive)
	if err != nil {
		slog.DebugContext(ctx, "failed to sample FRR daemons", "error", err)
		return nil, false
	}

	processes := make(map[string]processIdentity, len(daemons))
	pidDir := filepath.Dir(watchfrrPIDFile)
	ready := true
	for _, daemon := range daemons {
		identity, err := readDaemonIdentity(pidDir, daemon, os.ReadFile)
		if err != nil {
			slog.DebugContext(ctx, "failed to identify FRR daemon", "daemon", daemon.Name, "error", err)
			ready = false
			continue
		}
		processes[daemon.Name] = identity
		ready = ready && daemon.State == "Up" && identity.pid != 0
	}
	return processes, ready
}

func readDaemonIdentity(
	pidDir string, daemon liveness.DaemonState, readFile func(string) ([]byte, error),
) (processIdentity, error) {
	if filepath.Base(daemon.Name) != daemon.Name {
		return processIdentity{}, fmt.Errorf("invalid FRR daemon name %q", daemon.Name)
	}

	pidFile := filepath.Join(pidDir, daemon.Name+".pid")
	pidData, err := readFile(pidFile)
	if errors.Is(err, os.ErrNotExist) && daemon.State != "Up" {
		return processIdentity{}, nil
	}
	if err != nil {
		return processIdentity{}, fmt.Errorf("reading daemon PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return processIdentity{}, fmt.Errorf("parsing daemon PID: %w", err)
	}
	if pid <= 0 {
		return processIdentity{}, fmt.Errorf("invalid daemon PID %d", pid)
	}

	stat, statErr := readFile(fmt.Sprintf("/proc/%d/stat", pid))
	// A restart can replace the PID file between reading it and opening procfs.
	currentPID, err := readFile(pidFile)
	if err != nil {
		return processIdentity{}, fmt.Errorf("rechecking daemon PID file: %w", err)
	}
	if strings.TrimSpace(string(currentPID)) != strconv.Itoa(pid) {
		return processIdentity{}, errors.New("daemon PID changed during sample")
	}
	if errors.Is(statErr, os.ErrNotExist) {
		return processIdentity{}, nil
	}
	if statErr != nil {
		return processIdentity{}, fmt.Errorf("reading daemon process stat: %w", statErr)
	}

	return parseProcessStat(pid, daemon.Name, string(stat))
}

func parseProcessStat(pid int, daemon string, stat string) (processIdentity, error) {
	// The command field is parenthesized and can contain spaces or parentheses.
	commandStart := strings.IndexByte(stat, '(')
	commandEnd := strings.LastIndexByte(stat, ')')
	if commandStart == -1 || commandEnd <= commandStart {
		return processIdentity{}, errors.New("invalid daemon process stat")
	}
	fields := strings.Fields(stat[commandEnd+1:])
	const startTimeIndex = 19 // /proc/<pid>/stat field 22, counting from field 3.
	if len(fields) <= startTimeIndex {
		return processIdentity{}, errors.New("daemon process stat has no start time")
	}
	if stat[commandStart+1:commandEnd] != daemon || fields[0] == "Z" || fields[0] == "X" {
		return processIdentity{}, nil
	}
	startTime, err := strconv.ParseUint(fields[startTimeIndex], 10, 64)
	if err != nil {
		return processIdentity{}, fmt.Errorf("parsing daemon process start time: %w", err)
	}
	return processIdentity{pid: pid, startTime: startTime}, nil
}
