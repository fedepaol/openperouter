// SPDX-License-Identifier:Apache-2.0

package frrconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const RecoveryPath = "/recovery"

// RecoveryToken identifies the most recent FRR daemon restart observed by a reloader instance.
type RecoveryToken struct {
	InstanceID string `json:"instanceID"`
	Generation uint64 `json:"generation"`
}

// RecoveryState is the latest daemon state sampled by the reloader.
type RecoveryState struct {
	Token RecoveryToken `json:"token"`
	Ready bool          `json:"ready"`
}

// ErrRecoveryUnsupported indicates that the reloader does not expose recovery state.
var ErrRecoveryUnsupported = errors.New("reloader does not expose FRR recovery state")

// ReadRecoveryState reads the latest recovery state over the reload socket.
func ReadRecoveryState(ctx context.Context, socketPath string) (RecoveryState, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix"+RecoveryPath, nil)
	if err != nil {
		return RecoveryState{}, fmt.Errorf("creating FRR recovery state request: %w", err)
	}

	client := &http.Client{Transport: unixSocketTransport(socketPath)}
	res, err := client.Do(req)
	if err != nil {
		return RecoveryState{}, fmt.Errorf("reading FRR recovery state from %s: %w", socketPath, err)
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			slog.ErrorContext(ctx, "failed to close FRR recovery response", "error", err)
		}
	}()

	switch res.StatusCode {
	case http.StatusBadRequest, http.StatusNotFound:
		// Older reloaders route this GET to the POST-only reload handler.
		return RecoveryState{}, ErrRecoveryUnsupported
	case http.StatusOK:
	default:
		return RecoveryState{}, fmt.Errorf("reading FRR recovery state from %s: status %d", socketPath, res.StatusCode)
	}

	var state RecoveryState
	if err := json.NewDecoder(res.Body).Decode(&state); err != nil {
		return RecoveryState{}, fmt.Errorf("decoding FRR recovery state from %s: %w", socketPath, err)
	}
	if state.Token.InstanceID == "" {
		return RecoveryState{}, fmt.Errorf("FRR recovery state from %s has no reloader instance ID", socketPath)
	}
	return state, nil
}
