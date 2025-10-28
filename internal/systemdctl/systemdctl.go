package systemdctl

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	godbus "github.com/godbus/dbus/v5"
)

const (
	HostDBusSocket = "unix:path=/host/dbus/system_bus_socket"

	DefaultTimeout = 30 * time.Second
)

type Client struct {
	conn    *dbus.Conn
	timeout time.Duration
}

// NewClient creates a new systemd client with a custom D-Bus socket path
// and the default timeout
func NewClient(socketPath string) (*Client, error) {
	return NewClientWithTimeout(socketPath, DefaultTimeout)
}

// NewClientWithTimeout creates a new systemd client with a custom D-Bus socket path
// and a custom timeout
func NewClientWithTimeout(socketPath string, timeout time.Duration) (*Client, error) {
	// Create a context with timeout for the dial operation
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create a channel to handle the connection attempt with timeout
	type result struct {
		conn *dbus.Conn
		err  error
	}
	resultChan := make(chan result, 1)

	go func() {
		conn, err := dbus.NewConnection(func() (*godbus.Conn, error) {
			// Try to dial with the context
			return godbus.Dial(socketPath, godbus.WithContext(ctx))
		})
		resultChan <- result{conn: conn, err: err}
	}()

	// Wait for either the connection or timeout
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout connecting to D-Bus at %s: %w", socketPath, ctx.Err())
	case res := <-resultChan:
		if res.err != nil {
			return nil, fmt.Errorf("failed to connect to D-Bus at %s: %w", socketPath, res.err)
		}
		return &Client{
			conn:    res.conn,
			timeout: timeout,
		}, nil
	}
}

// Close closes the D-Bus connection
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// ensureTimeout wraps the context with the client's timeout if it doesn't have a deadline
func (c *Client) ensureTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); !ok {
		return context.WithTimeout(ctx, c.timeout)
	}
	return ctx, func() {}
}

// Restart restarts a systemd unit
func (c *Client) Restart(ctx context.Context, unitName string) error {
	ctx, cancel := c.ensureTimeout(ctx)
	defer cancel()

	responseChan := make(chan string)

	_, err := c.conn.RestartUnitContext(ctx, unitName, "replace", responseChan)
	if err != nil {
		return fmt.Errorf("failed to restart unit %s: %w", unitName, err)
	}

	// Wait for the job to complete
	select {
	case <-ctx.Done():
		return ctx.Err()
	case job := <-responseChan:
		if job == "" {
			return fmt.Errorf("restart job for %s failed", unitName)
		}
	}

	return nil
}

// Start starts a systemd unit
func (c *Client) Start(ctx context.Context, unitName string) error {
	ctx, cancel := c.ensureTimeout(ctx)
	defer cancel()

	responseChan := make(chan string)

	_, err := c.conn.StartUnitContext(ctx, unitName, "replace", responseChan)
	if err != nil {
		return fmt.Errorf("failed to start unit %s: %w", unitName, err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case job := <-responseChan:
		if job == "" {
			return fmt.Errorf("start job for %s failed", unitName)
		}
	}

	return nil
}

// Stop stops a systemd unit
func (c *Client) Stop(ctx context.Context, unitName string) error {
	ctx, cancel := c.ensureTimeout(ctx)
	defer cancel()

	responseChan := make(chan string)

	_, err := c.conn.StopUnitContext(ctx, unitName, "replace", responseChan)
	if err != nil {
		return fmt.Errorf("failed to stop unit %s: %w", unitName, err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case job := <-responseChan:
		if job == "" {
			return fmt.Errorf("stop job for %s failed", unitName)
		}
	}

	return nil
}

// IsActive checks if a systemd unit is active
func (c *Client) IsActive(unitName string) (bool, error) {
	activeState, err := c.unitProperty(unitName, "ActiveState")
	if err != nil {
		return false, err
	}

	state, ok := activeState.(string)
	if !ok {
		return false, fmt.Errorf("unexpected type for ActiveState: %T", activeState)
	}

	return state == "active", nil
}

// unitProperty gets a property of a systemd unit
func (c *Client) unitProperty(unitName, propertyName string) (interface{}, error) {
	prop, err := c.conn.GetUnitProperty(unitName, propertyName)
	if err != nil {
		return nil, fmt.Errorf("failed to get property %s for unit %s: %w", propertyName, unitName, err)
	}
	return prop.Value.Value(), nil
}
