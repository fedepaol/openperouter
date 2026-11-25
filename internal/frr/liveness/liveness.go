// SPDX-License-Identifier:Apache-2.0

package liveness

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/openperouter/openperouter/internal/frr/vtysh"
)

// ProcessAlive reports whether the process identified by pid exists.
func ProcessAlive(pid int) error {
	return syscall.Kill(pid, 0)
}

type CheckFn func() error

// DaemonState is the state reported by watchfrr for a supervised daemon.
type DaemonState struct {
	Name  string
	State string
}

// WatchFRR reports whether watchfrr is running and responding on its VTY.
func WatchFRR(pidFilePath string, watchFRRCli vtysh.Cli, processAlive func(int) error) error {
	_, err := watchFRROutput(pidFilePath, watchFRRCli, processAlive)
	return err
}

// WatchFRRReady reports whether every daemon supervised by watchfrr is Up.
func WatchFRRReady(pidFilePath string, watchFRRCli vtysh.Cli, processAlive func(int) error) error {
	daemons, err := WatchFRRDaemonStates(pidFilePath, watchFRRCli, processAlive)
	if err != nil {
		return err
	}

	unhealthyDaemons := []string{}
	for _, daemon := range daemons {
		if daemon.State != "Up" {
			unhealthyDaemons = append(unhealthyDaemons, fmt.Sprintf("%s is %s", daemon.Name, daemon.State))
		}
	}

	if len(unhealthyDaemons) > 0 {
		return fmt.Errorf("watchfrr reports unhealthy daemons: %s", strings.Join(unhealthyDaemons, ", "))
	}
	return nil
}

// WatchFRRDaemonStates fetches the states of all daemons supervised by watchfrr.
func WatchFRRDaemonStates(pidFilePath string, watchFRRCli vtysh.Cli, processAlive func(int) error) ([]DaemonState, error) {
	output, err := watchFRROutput(pidFilePath, watchFRRCli, processAlive)
	if err != nil {
		return nil, err
	}

	var daemons []DaemonState
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			daemons = append(daemons, DaemonState{Name: fields[0], State: fields[1]})
		}
	}
	if len(daemons) == 0 {
		return nil, errors.New("watchfrr did not report any daemon states")
	}
	return daemons, nil
}

func watchFRROutput(pidFilePath string, watchFRRCli vtysh.Cli, processAlive func(int) error) (string, error) {
	content, err := os.ReadFile(pidFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read the watchfrr pid file %s: %w", pidFilePath, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return "", fmt.Errorf("invalid pid in the watchfrr pid file %s: %w", pidFilePath, err)
	}
	if err := processAlive(pid); err != nil {
		return "", fmt.Errorf("watchfrr %d is not running: %w", pid, err)
	}
	monitorOutput, err := watchFRRCli("show watchfrr")
	if err != nil {
		return "", fmt.Errorf("watchfrr %d does not answer on its vty: %w", pid, err)
	}
	return monitorOutput, nil
}
