// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openperouter/openperouter/api/static"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/internal/conversion"
	"github.com/openperouter/openperouter/internal/staticconfiguration"
)

// StaticConfigReconciler reconciles configuration from a static file.
// It's designed for host mode where Kubernetes API may not be available.
type StaticConfigReconciler struct {
	Scheme        *runtime.Scheme
	Logger        *slog.Logger
	LogLevel      string
	FRRConfigPath string

	// Host mode specific configuration
	FRRReloadSocket   string
	RouterPidFilePath string
	SystemdSocketPath string
	ConfigFilePath    string

	TriggerChan chan event.GenericEvent
}

func (r *StaticConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.With("controller", "StaticConfigReconciler", "request", req.String())
	logger.Info("start reconcile")
	defer logger.Info("end reconcile")

	staticConfig, err := staticconfiguration.ReadFromFile(r.ConfigFilePath)
	if err != nil {
		logger.Error("failed to read static configuration", "error", err, "path", r.ConfigFilePath)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, fmt.Errorf("failed to read static config: %w", err)
	}

	logger.Debug("loaded static configuration", "config", staticConfig)

	apiConfig := r.staticConfigToAPIConfig(staticConfig)

	logger.Debug("using config",
		"l3vnis", apiConfig.L3VNIs,
		"l2vnis", apiConfig.L2VNIs,
		"underlays", apiConfig.Underlays,
		"l3passthrough", apiConfig.L3Passthrough)

	routerProvider := &RouterHostProvider{
		FRRConfigPath:     r.FRRConfigPath,
		FRRReloadSocket:   r.FRRReloadSocket,
		RouterPidFilePath: r.RouterPidFilePath,
		CurrentNodeIndex:  staticConfig.NodeIndex,
		SystemdSocketPath: r.SystemdSocketPath,
	}

	router, err := routerProvider.New(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get router instance: %w", err)
	}

	targetNS, err := router.TargetNS(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to retrieve target namespace: %w", err)
	}

	updater, err := router.Updater(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to retrieve updater: %w", err)
	}

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

func (r *StaticConfigReconciler) staticConfigToAPIConfig(staticConfig *static.PERouterConfig) conversion.ApiConfigData {
	underlays := make([]v1alpha1.Underlay, len(staticConfig.Underlays))
	for i, spec := range staticConfig.Underlays {
		underlays[i] = v1alpha1.Underlay{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Underlay",
				APIVersion: "openpe.openperouter.github.io/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("static-underlay-%d", i),
			},
			Spec: spec,
		}
	}

	l3vnis := make([]v1alpha1.L3VNI, len(staticConfig.L3VNIs))
	for i, spec := range staticConfig.L3VNIs {
		l3vnis[i] = v1alpha1.L3VNI{
			TypeMeta: metav1.TypeMeta{
				Kind:       "L3VNI",
				APIVersion: "openpe.openperouter.github.io/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("static-l3vni-%d", i),
			},
			Spec: spec,
		}
	}

	l2vnis := make([]v1alpha1.L2VNI, len(staticConfig.L2VNIs))
	for i, spec := range staticConfig.L2VNIs {
		l2vnis[i] = v1alpha1.L2VNI{
			TypeMeta: metav1.TypeMeta{
				Kind:       "L2VNI",
				APIVersion: "openpe.openperouter.github.io/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("static-l2vni-%d", i),
			},
			Spec: spec,
		}
	}

	var l3passthrough []v1alpha1.L3Passthrough
	if staticConfig.BGPPassthrough.HostSession.ASN > 0 {
		l3passthrough = []v1alpha1.L3Passthrough{
			{
				TypeMeta: metav1.TypeMeta{
					Kind:       "L3Passthrough",
					APIVersion: "openpe.openperouter.github.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "static-l3passthrough",
				},
				Spec: staticConfig.BGPPassthrough,
			},
		}
	}

	return conversion.ApiConfigData{
		NodeIndex:     staticConfig.NodeIndex,
		Underlays:     underlays,
		L3VNIs:        l3vnis,
		L2VNIs:        l2vnis,
		L3Passthrough: l3passthrough,
		LogLevel:      r.LogLevel,
	}
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
