// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/openperouter/openperouter/internal/frrconfig"
)

func TestFRRRecoverySourceDelaysReconciliation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[ctrl.Request]())
		defer queue.ShutDown()
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}

		var snapshot atomic.Pointer[frrconfig.RecoveryState]
		token := frrconfig.RecoveryToken{InstanceID: "reloader-1"}
		snapshot.Store(&frrconfig.RecoveryState{Token: token, Ready: true})
		recovery := &frrRecovery{readState: func(context.Context) (frrconfig.RecoveryState, error) {
			return *snapshot.Load(), nil
		}}

		require.NoError(t, recovery.source(req).Start(ctx, queue))
		synctest.Wait()
		require.Zero(t, queue.Len(), "the initial healthy generation should not enqueue")

		token.Generation++
		snapshot.Store(&frrconfig.RecoveryState{Token: token})
		<-time.After(frrRecoveryPollInterval)
		synctest.Wait()
		require.Zero(t, queue.Len(), "the restart should enqueue with a delay")
		configure, after, err := recovery.configurator(ctx)
		require.NoError(t, err)
		require.Nil(t, configure, "an independent event must honor the restart delay")
		require.Equal(t, frrRecoveryDelay, after)

		<-time.After(frrRecoveryDelay)
		synctest.Wait()
		require.Equal(t, 1, queue.Len())
		got, shutdown := queue.Get()
		require.False(t, shutdown)
		require.Equal(t, req, got)
		queue.Done(got)

		configure, after, err = recovery.configurator(ctx)
		require.NoError(t, err)
		require.Nil(t, configure, "a reconcile should wait when a daemon is still down")
		require.Equal(t, frrRecoveryPollInterval, after)

		snapshot.Store(&frrconfig.RecoveryState{Token: token, Ready: true})
		configure, after, err = recovery.configurator(ctx)
		require.NoError(t, err)
		require.NotNil(t, configure)
		require.Zero(t, after)

		<-time.After(frrRecoveryPollInterval)
		synctest.Wait()
		require.Zero(t, queue.Len(), "a stable token should not enqueue again")
	})
}

func TestFRRRecoveryConfiguratorStartup(t *testing.T) {
	readError := errors.New("socket read failed")
	token := frrconfig.RecoveryToken{InstanceID: "reloader-1"}
	tests := []struct {
		name          string
		state         frrconfig.RecoveryState
		probeError    error
		wantConfigure bool
		wantDelay     time.Duration
		wantError     error
	}{
		{
			name:      "wait for the reloader's first healthy sample",
			state:     frrconfig.RecoveryState{Token: token},
			wantDelay: frrRecoveryPollInterval,
		},
		{
			name:          "configure immediately once the startup baseline is ready",
			state:         frrconfig.RecoveryState{Token: token, Ready: true},
			wantConfigure: true,
		},
		{
			name:          "support older reloaders without the state endpoint",
			probeError:    frrconfig.ErrRecoveryUnsupported,
			wantConfigure: true,
		},
		{
			name:       "a failed status read retries through the controller error path",
			probeError: readError,
			wantError:  readError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recovery := &frrRecovery{readState: func(context.Context) (frrconfig.RecoveryState, error) {
				return tt.state, tt.probeError
			}}
			configure, after, err := recovery.configurator(t.Context())
			require.ErrorIs(t, err, tt.wantError)
			require.Equal(t, tt.wantDelay, after)
			require.Equal(t, tt.wantConfigure, configure != nil)
		})
	}
}

func TestFRRRecoveryRestartDuringReload(t *testing.T) {
	resetFRRConfigCache(t)
	configFile := filepath.Join(t.TempDir(), "frr.conf")
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		token := frrconfig.RecoveryToken{InstanceID: "reloader-1", Generation: 1}
		recovery := &frrRecovery{readState: func(context.Context) (frrconfig.RecoveryState, error) {
			return frrconfig.RecoveryState{Token: token, Ready: true}, nil
		}}
		_, after, err := recovery.configurator(ctx)
		require.NoError(t, err)
		require.Equal(t, frrRecoveryDelay, after)
		<-time.After(after)

		configure, after, err := recovery.configurator(ctx)
		require.NoError(t, err)
		require.Zero(t, after)
		require.NotNil(t, configure)

		calls := 0
		data := frrConfigData{
			configFile: configFile,
			updater: func(_ context.Context, config string) error {
				calls++
				if calls == 1 {
					token.Generation++
				}
				return os.WriteFile(configFile, []byte(config), 0600)
			},
		}
		previousToken := token
		require.NoError(t, configure(ctx, data))
		require.Equal(t, previousToken, lastAppliedFRRRecoveryToken)

		_, after, err = recovery.configurator(ctx)
		require.NoError(t, err)
		require.Equal(t, frrRecoveryDelay, after)
		<-time.After(after)

		configure, after, err = recovery.configurator(ctx)
		require.NoError(t, err)
		require.Zero(t, after)
		require.NotNil(t, configure)
		require.NoError(t, configure(ctx, data))
		require.Equal(t, token, lastAppliedFRRRecoveryToken)
		require.Equal(t, 2, calls, "the newer generation requires another apply of the same configuration")
	})
}
