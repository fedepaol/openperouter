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
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name           string
		socketPath     string
		expectedSocket string
	}{
		{
			name:           "default socket path",
			socketPath:     "",
			expectedSocket: defaultSocketPath,
		},
		{
			name:           "custom socket path",
			socketPath:     "/custom/path/podman.sock",
			expectedSocket: "/custom/path/podman.sock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.socketPath)
			if client.socketPath != tt.expectedSocket {
				t.Errorf("expected socket path %s, got %s", tt.expectedSocket, client.socketPath)
			}
			if client.httpClient == nil {
				t.Error("http client should not be nil")
			}
		})
	}
}

func TestStopPod(t *testing.T) {
	tests := []struct {
		name           string
		podName        string
		statusCode     int
		expectedError  bool
		errorContains  string
	}{
		{
			name:          "successful stop - 200 OK",
			podName:       "testpod",
			statusCode:    http.StatusOK,
			expectedError: false,
		},
		{
			name:          "successful stop - 204 No Content",
			podName:       "testpod",
			statusCode:    http.StatusNoContent,
			expectedError: false,
		},
		{
			name:          "pod not found",
			podName:       "nonexistent",
			statusCode:    http.StatusNotFound,
			expectedError: true,
			errorContains: "unexpected status code",
		},
		{
			name:          "server error",
			podName:       "testpod",
			statusCode:    http.StatusInternalServerError,
			expectedError: true,
			errorContains: "unexpected status code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for the Unix socket
			tmpDir := t.TempDir()
			socketPath := filepath.Join(tmpDir, "test.sock")

			// Create a test HTTP server
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := fmt.Sprintf("/%s/libpod/pods/%s/stop", apiVersion, tt.podName)
				if r.URL.Path != expectedPath {
					t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("expected POST method, got %s", r.Method)
				}
				w.WriteHeader(tt.statusCode)
			})

			// Start Unix socket server
			server := httptest.NewUnstartedServer(handler)
			listener, err := net.Listen("unix", socketPath)
			if err != nil {
				t.Fatalf("failed to create unix listener: %v", err)
			}
			defer os.Remove(socketPath)

			server.Listener = listener
			server.Start()
			defer server.Close()

			// Create client and test StopPod
			client := NewClient(socketPath)
			err = client.StopPod(context.Background(), tt.podName)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRestartRouterPod(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := fmt.Sprintf("/%s/libpod/pods/%s/stop", apiVersion, defaultPodName)
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewUnstartedServer(handler)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to create unix listener: %v", err)
	}
	defer os.Remove(socketPath)

	server.Listener = listener
	server.Start()
	defer server.Close()

	client := NewClient(socketPath)
	err = client.RestartRouterPod(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
