// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/openperouter/openperouter/internal/frr"
	"github.com/openperouter/openperouter/internal/frrconfig"
	"github.com/vishvananda/netns"
)

type RouterHostManager struct {
	FRRConfigPath     string
	FRRReloadSocket   string
	RouterPidFilePath string
	CurrentNodeIndex  int
}

var _ RouterManager = (*RouterHostManager)(nil)

type RouterHostContainer struct {
	manager *RouterHostManager
}

var _ Router = (*RouterHostContainer)(nil)

func (r *RouterHostManager) New(ctx context.Context) (Router, error) {
	return &RouterHostContainer{
		manager: r,
	}, nil
}

func (r *RouterHostManager) NodeIndex(ctx context.Context) (int, error) {
	return r.CurrentNodeIndex, nil
}

func (r *RouterHostContainer) TargetNS(ctx context.Context) (string, error) {
	pidData, err := os.ReadFile(r.manager.RouterPidFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read PID file %s: %w", r.manager.RouterPidFilePath, err)
	}

	pidStr := strings.TrimSpace(string(pidData))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse PID from file %s: %w", r.manager.RouterPidFilePath, err)
	}

	nsHandle, err := netns.GetFromPid(pid)
	if err != nil {
		return "", fmt.Errorf("failed to get network namespace for PID %d: %w", pid, err)
	}
	defer nsHandle.Close()

	return nsHandle.String(), nil
}

func (r *RouterHostContainer) HandleNonRecoverableError(ctx context.Context) error {
	// TODO
	return nil
}

func (r *RouterHostContainer) Updater(ctx context.Context) (frr.Updater, error) {
	updater := frrconfig.UpdaterForSocket(r.manager.FRRReloadSocket, r.manager.FRRConfigPath)
	return updater, nil
}

func (r *RouterHostContainer) CanReconcile(ctx context.Context) (bool, error) {
	// TODO check if the container is up
	return true, nil
}
