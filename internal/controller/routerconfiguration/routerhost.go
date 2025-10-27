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
	"github.com/openperouter/openperouter/internal/systemdctl"
)

type RouterHostManager struct {
	FRRConfigPath     string
	FRRReloadSocket   string
	RouterPidFilePath string
	CurrentNodeIndex  int
	SystemdSocketPath string
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

	res := fmt.Sprintf("/hostproc/%d/ns/net", pid)
	return res, nil
}

func (r *RouterHostContainer) HandleNonRecoverableError(ctx context.Context) error {
	client, err := systemdctl.NewClient(r.manager.SystemdSocketPath)
	if err != nil {
		return fmt.Errorf("failed to create systemd client %w", err)
	}
	defer client.Close()
	if err := client.Restart(ctx, "pod-routerpod.service"); err != nil {
		return fmt.Errorf("failed to restart routerpod service")
	}

	return nil
}

func (r *RouterHostContainer) Updater(ctx context.Context) (frr.Updater, error) {
	updater := frrconfig.UpdaterForSocket(r.manager.FRRReloadSocket, r.manager.FRRConfigPath)
	return updater, nil
}

func (r *RouterHostContainer) CanReconcile(ctx context.Context) (bool, error) {
	client, err := systemdctl.NewClient(r.manager.SystemdSocketPath)
	if err != nil {
		return false, fmt.Errorf("failed to create systemd client %w", err)
	}
	defer client.Close()
	res, err := client.IsActive("pod-routerpod.service")
	if err != nil {
		return false, fmt.Errorf("failed to restart routerpod service")
	}

	return res, nil
}
