// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	openpeerrors "github.com/openperouter/openperouter/internal/errors"
	"github.com/openperouter/openperouter/internal/frrconfig"
)

// StaticConfigReconciler reconciles configuration from a static file.
// It's designed for host mode where Kubernetes API may not be available.
type StaticConfigReconciler struct {
	Scheme               *runtime.Scheme
	Logger               *slog.Logger
	NodeIndex            int
	LogLevel             string
	FRRConfigPath        string
	FRRReloadSocket      string
	RouterProvider       RouterProvider
	ConfigDir            string
	MyNode               string
	MyNamespace          string
	DatapathConfigurator DatapathConfigurator

	TriggerChan chan event.GenericEvent
	frrRecovery *frrRecovery
}

var staticConfigRequest = ctrl.Request{
	NamespacedName: types.NamespacedName{Name: "static-config-trigger", Namespace: "default"},
}

func (r *StaticConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.With("controller", "StaticConfigReconciler", "request", req.String())
	logger.Info("start reconcile")
	defer logger.Info("end reconcile")

	logger.Info("using config dir", "dir", r.ConfigDir)
	// Read and merge router configs from directory
	apiConfig, err := readStaticConfigs(r.ConfigDir, r.MyNode, r.MyNamespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to read static router configurations from %s: %w", r.ConfigDir, err)
	}

	logger.Info("using config",
		"nodeIndex", r.NodeIndex,
		"logLevel", r.LogLevel,
		"l3vnis", len(apiConfig.L3VNIs),
		"l2vnis", len(apiConfig.L2VNIs),
		"l3vpns", len(apiConfig.L3VPNs),
		"underlays", len(apiConfig.Underlays),
		"l3passthrough", len(apiConfig.L3Passthrough))

	router, err := r.RouterProvider.New(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get router instance: %w", err)
	}

	canReconcile, err := router.CanReconcile()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("canReconcile error: %w", err)
	}
	if !canReconcile {
		logger.Info("router not ready for reconciliation, will retry", "retryAfter", "10s")
		return ctrl.Result{Requeue: true, RequeueAfter: 2 * time.Second}, nil
	}
	frrConfigurator, requeueAfter, err := r.frrRecovery.configurator(ctx)
	if err != nil || requeueAfter > 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}
	if err := invalidateFRRConfigCacheOnRestart(ctx, router); err != nil {
		return ctrl.Result{}, err
	}

	targetNS, err := router.TargetNS(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to retrieve target namespace: %w", err)
	}

	updater := frrconfig.UpdaterForSocket(r.FRRReloadSocket, r.FRRConfigPath)

	err = Reconcile(ctx, apiConfig, r.NodeIndex, r.LogLevel, r.FRRConfigPath, targetNS, updater,
		r.DatapathConfigurator, frrConfigurator)
	for _, f := range openpeerrors.CollectFailures(err) {
		logger.Warn("resource skipped", "kind", f.Kind, "name", f.Name, "reason", f.Reason, "message", f.Message)
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to configure the host: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *StaticConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.TriggerChan = make(chan event.GenericEvent, 1)

	go func(triggerChan chan<- event.GenericEvent) {
		triggerChan <- event.GenericEvent{
			Object: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      staticConfigRequest.Name,
					Namespace: staticConfigRequest.Namespace,
				},
			},
		}
	}(r.TriggerChan)

	builder := ctrl.NewControllerManagedBy(mgr).
		Named("static-config-controller").
		WatchesRawSource(source.Channel(r.TriggerChan, &handler.EnqueueRequestForObject{}))
	if r.FRRReloadSocket != "" {
		r.frrRecovery = newFRRRecovery(r.FRRReloadSocket)
		builder = builder.WatchesRawSource(r.frrRecovery.source(staticConfigRequest))
	}
	return builder.Complete(r)
}

func (r *StaticConfigReconciler) TriggerReconcile() {
	select {
	case r.TriggerChan <- event.GenericEvent{
		Object: &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      staticConfigRequest.Name,
				Namespace: staticConfigRequest.Namespace,
			},
		},
	}:
		r.Logger.Info("triggered static config reconciliation")
	default:
		r.Logger.Debug("reconciliation already queued, skipping trigger")
	}
}
