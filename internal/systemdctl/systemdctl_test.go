package systemdctl

import (
	"context"
	"testing"
	"time"
)

// TestEnsureTimeout tests the ensureTimeout helper function
func TestEnsureTimeout(t *testing.T) {
	// Create a client with a custom timeout for testing
	client := &Client{
		timeout: 10 * time.Second,
	}

	tests := []struct {
		name           string
		ctx            context.Context
		expectDeadline bool
	}{
		{
			name:           "context with existing deadline",
			ctx:            func() context.Context { ctx, _ := context.WithTimeout(context.Background(), 5*time.Second); return ctx }(),
			expectDeadline: true,
		},
		{
			name:           "context without deadline",
			ctx:            context.Background(),
			expectDeadline: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := client.ensureTimeout(tt.ctx)
			defer cancel()

			_, hasDeadline := ctx.Deadline()
			if hasDeadline != tt.expectDeadline {
				t.Errorf("ensureTimeout() hasDeadline = %v, want %v", hasDeadline, tt.expectDeadline)
			}
		})
	}
}

// TestNewClient tests creating a client with a custom socket path
func TestNewClient(t *testing.T) {
	tests := []struct {
		name       string
		socketPath string
		wantErr    bool
	}{
		{
			name:       "invalid socket path",
			socketPath: "unix:path=/nonexistent/socket",
			wantErr:    true,
		},
		{
			name:       "empty socket path",
			socketPath: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.socketPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if client != nil {
				defer client.Close()
				if client.timeout != DefaultTimeout {
					t.Errorf("NewClient() timeout = %v, want %v", client.timeout, DefaultTimeout)
				}
			}
		})
	}
}

// TestNewClientWithTimeout tests creating a client with a custom timeout
func TestNewClientWithTimeout(t *testing.T) {
	customTimeout := 60 * time.Second

	tests := []struct {
		name       string
		socketPath string
		timeout    time.Duration
		wantErr    bool
	}{
		{
			name:       "custom timeout",
			socketPath: "unix:path=/nonexistent/socket",
			timeout:    customTimeout,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClientWithTimeout(tt.socketPath, tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClientWithTimeout() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if client != nil {
				defer client.Close()
				if client.timeout != tt.timeout {
					t.Errorf("NewClientWithTimeout() timeout = %v, want %v", client.timeout, tt.timeout)
				}
			}
		})
	}
}

// TestRestart is an integration test that requires systemd
// It will be skipped if systemd is not available
func TestRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Try to connect to the system D-Bus
	client, err := NewClient(HostDBusSocket)
	if err != nil {
		t.Skipf("systemd not available: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name     string
		unitName string
		wantErr  bool
	}{
		{
			name:     "nonexistent unit",
			unitName: "nonexistent-unit-12345.service",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := client.Restart(ctx, tt.unitName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Restart() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestStart is an integration test
func TestStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewClient(HostDBusSocket)
	if err != nil {
		t.Skipf("systemd not available: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name     string
		unitName string
		wantErr  bool
	}{
		{
			name:     "nonexistent unit",
			unitName: "nonexistent-unit-12345.service",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := client.Start(ctx, tt.unitName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Start() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestStop is an integration test
func TestStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewClient(HostDBusSocket)
	if err != nil {
		t.Skipf("systemd not available: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name     string
		unitName string
		wantErr  bool
	}{
		{
			name:     "nonexistent unit",
			unitName: "nonexistent-unit-12345.service",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := client.Stop(ctx, tt.unitName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Stop() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestIsActive is an integration test
func TestIsActive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewClient(HostDBusSocket)
	if err != nil {
		t.Skipf("systemd not available: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name     string
		unitName string
		wantErr  bool
	}{
		{
			name:     "nonexistent unit",
			unitName: "nonexistent-unit-12345.service",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.IsActive(tt.unitName)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsActive() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
