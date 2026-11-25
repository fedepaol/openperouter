// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openperouter/openperouter/internal/frrconfig"
)

const (
	frrRecoveryPollInterval = 2 * time.Second
	frrRecoveryDelay        = 2 * time.Second
)

type frrRecovery struct {
	readState func(context.Context) (frrconfig.RecoveryState, error)
	mu        sync.Mutex
	token     frrconfig.RecoveryToken
	notBefore time.Time
}

func newFRRRecovery(socketPath string) *frrRecovery {
	return &frrRecovery{readState: func(ctx context.Context) (frrconfig.RecoveryState, error) {
		return frrconfig.ReadRecoveryState(ctx, socketPath)
	}}
}

func (r *frrRecovery) source(req ctrl.Request) source.Source {
	return source.TypedFunc[ctrl.Request](func(ctx context.Context, queue workqueue.TypedRateLimitingInterface[ctrl.Request]) error {
		go r.watch(ctx, queue, req)
		return nil
	})
}

func (r *frrRecovery) configurator(ctx context.Context) (frrConfiguratorType, time.Duration, error) {
	if r == nil {
		return configureFRR, 0, nil
	}
	state, notBefore, err := r.observe(ctx)
	if errors.Is(err, frrconfig.ErrRecoveryUnsupported) {
		return configureFRR, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}

	// API, file, and pod events may enqueue the same request before the
	// recovery event's delay has elapsed.
	if remaining := time.Until(notBefore); remaining > 0 {
		return nil, remaining, nil
	}
	if !state.Ready {
		return nil, frrRecoveryPollInterval, nil
	}

	return func(ctx context.Context, data frrConfigData) error {
		data.recoveryToken = state.Token
		return configureFRR(ctx, data)
	}, 0, nil
}

func (r *frrRecovery) watch(ctx context.Context, queue workqueue.TypedRateLimitingInterface[ctrl.Request], req ctrl.Request) {
	ticker := time.NewTicker(frrRecoveryPollInterval)
	defer ticker.Stop()

	var previous frrconfig.RecoveryToken
	for ctx.Err() == nil {
		state, notBefore, err := r.observe(ctx)
		switch {
		case err == nil:
			if state.Token != previous && !notBefore.IsZero() {
				queue.AddAfter(req, time.Until(notBefore))
			}
			previous = state.Token
		case !errors.Is(err, frrconfig.ErrRecoveryUnsupported) && ctx.Err() == nil:
			slog.DebugContext(ctx, "failed to watch FRR recovery state", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *frrRecovery) observe(ctx context.Context) (frrconfig.RecoveryState, time.Time, error) {
	// Keep reads ordered so a reconcile cannot replace a newer snapshot that
	// the event source has already seen with an older snapshot.
	r.mu.Lock()
	defer r.mu.Unlock()

	state, err := r.readState(ctx)
	if err != nil {
		return frrconfig.RecoveryState{}, time.Time{}, err
	}
	if state.Token != r.token {
		if r.token.InstanceID != "" || state.Token.Generation != 0 {
			r.notBefore = time.Now().Add(frrRecoveryDelay)
		}
		r.token = state.Token
	}
	return state, r.notBefore, nil
}
