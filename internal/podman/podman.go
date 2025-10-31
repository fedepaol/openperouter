/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package podman

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Client wraps the podman socket connection
type Client struct {
	httpClient *http.Client
	socketPath string
	apiVersion string
}

// NewClient creates a new podman client
func NewClient(socketPath, apiVersion string) *Client {
	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
				},
			},
			Timeout: 30 * time.Second,
		},
	}
}

// StopPod stops a pod by name
// The pod will be automatically restarted by systemd if Restart=always is configured
func (c *Client) StopPod(ctx context.Context, podName string) error {
	url := fmt.Sprintf("http://unix/%s/libpod/pods/%s/stop", c.apiVersion, podName)

	slog.Info("stopping pod via podman socket", "pod", podName, "socket", c.socketPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to stop pod %s: %w", podName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code stopping pod %s: %d", podName, resp.StatusCode)
	}

	slog.Info("pod stopped successfully", "pod", podName)
	return nil
}
