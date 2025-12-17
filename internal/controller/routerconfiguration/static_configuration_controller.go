// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openperouter/openperouter/internal/frrconfig"
)

// StaticConfigReconciler reconciles configuration from a static file.
// It's designed for host mode where Kubernetes API may not be available.
type StaticConfigReconciler struct {
	Scheme          *runtime.Scheme
	Logger          *slog.Logger
	LogLevel        string
	FRRConfigPath   string
	FRRReloadSocket string
	RouterProvider  RouterProvider
	ConfigFilePath  string

	TriggerChan chan event.GenericEvent
}

func (r *StaticConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.With("controller", "StaticConfigReconciler", "request", req.String())
	logger.Info("start reconcile")
	defer logger.Info("end reconcile")

	apiConfig, err := readStaticConfig(r.ConfigFilePath)
	if err != nil {
		logger.Error("failed to read static configuration", "error", err, "path", r.ConfigFilePath)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	logger.Info("using config",
		"l3vnis", apiConfig.L3VNIs,
		"l2vnis", apiConfig.L2VNIs,
		"underlays", apiConfig.Underlays,
		"l3passthrough", apiConfig.L3Passthrough)

	router, err := r.RouterProvider.New(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get router instance: %w", err)
	}

	targetNS, err := router.TargetNS(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to retrieve target namespace: %w", err)
	}

	updater := frrconfig.UpdaterForSocket(r.FRRReloadSocket, r.FRRConfigPath)

	err = Reconcile(ctx, apiConfig, r.FRRConfigPath, targetNS, updater)
	if nonRecoverableHostError(err) {
		if err := router.HandleNonRecoverableError(ctx); err != nil {
			logger.Error("failed to handle non recoverable error", "error", err)
			return ctrl.Result{}, err
		}
	}
	if err != nil {
		logger.Error("failed to configure the host", "error", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *StaticConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.TriggerChan = make(chan event.GenericEvent, 1)

	go func() {
		time.Sleep(1 * time.Second)
		r.TriggerChan <- event.GenericEvent{}
	}()

	return ctrl.NewControllerManagedBy(mgr).
		Named("static-config-controller").
		WatchesRawSource(source.Channel(r.TriggerChan, &handler.EnqueueRequestForObject{})).
		Complete(r)
}

func (r *StaticConfigReconciler) TriggerReconcile() {
	select {
	case r.TriggerChan <- event.GenericEvent{}:
		r.Logger.Info("triggered static config reconciliation")
	default:
		r.Logger.Debug("reconciliation already queued, skipping trigger")
	}
}
