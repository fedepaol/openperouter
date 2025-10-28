package systemdctl

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ExecClient is a systemd client that uses systemctl commands via nsenter
// to avoid D-Bus authentication issues when running in containers
type ExecClient struct {
	timeout time.Duration
	hostPID string // PID 1 on the host (usually "1")
}

// NewExecClient creates a new systemd client that uses systemctl commands
func NewExecClient() (*ExecClient, error) {
	return NewExecClientWithTimeout(DefaultTimeout)
}

// NewExecClientWithTimeout creates a new systemd client with a custom timeout
func NewExecClientWithTimeout(timeout time.Duration) (*ExecClient, error) {
	return &ExecClient{
		timeout: timeout,
		hostPID: "1",
	}, nil
}

// Close is a no-op for exec client
func (c *ExecClient) Close() {
	// Nothing to close
}

// runSystemctl executes a systemctl command in the host's namespaces
func (c *ExecClient) runSystemctl(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Use nsenter to run systemctl in the host's mount, UTS, IPC, and net namespaces
	// Since we're already running with --pid=host, we can use PID 1
	cmdArgs := []string{
		"-t", c.hostPID,
		"-m", "-u", "-i", "-n",
		"systemctl",
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, "nsenter", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s failed: %w: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

// Restart restarts a systemd unit
func (c *ExecClient) Restart(ctx context.Context, unitName string) error {
	return c.runSystemctl(ctx, "restart", unitName)
}

// Start starts a systemd unit
func (c *ExecClient) Start(ctx context.Context, unitName string) error {
	return c.runSystemctl(ctx, "start", unitName)
}

// Stop stops a systemd unit
func (c *ExecClient) Stop(ctx context.Context, unitName string) error {
	return c.runSystemctl(ctx, "stop", unitName)
}

// IsActive checks if a systemd unit is active
func (c *ExecClient) IsActive(unitName string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmdArgs := []string{
		"-t", c.hostPID,
		"-m", "-u", "-i", "-n",
		"systemctl",
		"is-active",
		unitName,
	}

	cmd := exec.CommandContext(ctx, "nsenter", cmdArgs...)
	output, err := cmd.CombinedOutput()

	// systemctl is-active returns 0 if active, non-zero otherwise
	if err != nil {
		// Check if it's just because the unit is not active
		if strings.TrimSpace(string(output)) == "inactive" ||
		   strings.TrimSpace(string(output)) == "failed" {
			return false, nil
		}
		return false, fmt.Errorf("systemctl is-active %s failed: %w: %s", unitName, err, string(output))
	}

	return strings.TrimSpace(string(output)) == "active", nil
}
