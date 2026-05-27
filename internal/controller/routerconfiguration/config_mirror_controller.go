// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/internal/conversion"
	"github.com/openperouter/openperouter/internal/staticconfiguration"
)

const (
	LabelMirrored      = "openperouter.io/mirrored"
	LabelSourceNode    = "openperouter.io/source-node"
	LabelValueMirrored = "true"
)

// ConfigMirrorController mirrors static file-based configs to Kubernetes CRs.
type ConfigMirrorController struct {
	client.Client
	Scheme      *runtime.Scheme
	Logger      *slog.Logger
	NodeName    string
	Namespace   string
	ConfigDir   string
	TriggerChan chan event.GenericEvent
}

func (r *ConfigMirrorController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.With("controller", "ConfigMirrorController", "request", req.String())
	logger.Info("start reconcile")
	defer logger.Info("end reconcile")

	routerConfigs, err := staticconfiguration.ReadRouterConfigs(r.ConfigDir)
	if err != nil {
		var noConfigErr *staticconfiguration.NoConfigAvailable
		if errors.As(err, &noConfigErr) {
			logger.Info("no config files available, cleaning up mirrored CRs")
			return ctrl.Result{}, r.cleanupAllMirroredCRs(ctx, logger)
		}
		return ctrl.Result{}, fmt.Errorf("failed to read router configs: %w", err)
	}

	desired := make(map[string]client.Object)
	for _, rc := range routerConfigs {
		apiConfig := staticConfigToAPIConfig(rc)
		r.addDesiredCRs(apiConfig, desired)
	}

	for name, obj := range desired {
		if err := r.createOrUpdate(ctx, obj, logger); err != nil {
			logger.Error("failed to create or update CR", "name", name, "error", err)
			return ctrl.Result{}, fmt.Errorf("failed to reconcile CR %s: %w", name, err)
		}
	}

	if err := r.deleteStale(ctx, desired, logger); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete stale CRs: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *ConfigMirrorController) SetupWithManager(mgr ctrl.Manager) error {
	if r.TriggerChan == nil {
		r.TriggerChan = make(chan event.GenericEvent, 1)
	}

	go func(triggerChan chan<- event.GenericEvent) {
		select {
		case triggerChan <- event.GenericEvent{
			Object: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-mirror-trigger",
					Namespace: "default",
				},
			},
		}:
		default:
		}
	}(r.TriggerChan)

	return ctrl.NewControllerManagedBy(mgr).
		Named("config-mirror-controller").
		WatchesRawSource(source.Channel(r.TriggerChan, &handler.EnqueueRequestForObject{})).
		Complete(r)
}

func (r *ConfigMirrorController) addDesiredCRs(config conversion.APIConfigData, desired map[string]client.Object) {

	nodeSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"kubernetes.io/hostname": r.NodeName,
		},
	}

	for i, underlay := range config.Underlays {
		name := fmt.Sprintf("%s-static-underlay-%d", r.NodeName, i)
		obj := underlay.DeepCopy()
		obj.Name = name
		obj.Namespace = r.Namespace
		if obj.Labels == nil {
			obj.Labels = make(map[string]string)
		}
		obj.Labels[LabelMirrored] = LabelValueMirrored
		obj.Labels[LabelSourceNode] = r.NodeName
		obj.Spec.NodeSelector = nodeSelector
		desired[name] = obj
	}

	for i, l3vni := range config.L3VNIs {
		name := fmt.Sprintf("%s-static-l3vni-%d", r.NodeName, i)
		obj := l3vni.DeepCopy()
		obj.Name = name
		obj.Namespace = r.Namespace
		if obj.Labels == nil {
			obj.Labels = make(map[string]string)
		}
		obj.Labels[LabelMirrored] = LabelValueMirrored
		obj.Labels[LabelSourceNode] = r.NodeName
		obj.Spec.NodeSelector = nodeSelector
		desired[name] = obj
	}

	for i, l2vni := range config.L2VNIs {
		name := fmt.Sprintf("%s-static-l2vni-%d", r.NodeName, i)
		obj := l2vni.DeepCopy()
		obj.Name = name
		obj.Namespace = r.Namespace
		if obj.Labels == nil {
			obj.Labels = make(map[string]string)
		}
		obj.Labels[LabelMirrored] = LabelValueMirrored
		obj.Labels[LabelSourceNode] = r.NodeName
		obj.Spec.NodeSelector = nodeSelector
		desired[name] = obj
	}

	for i, l3pass := range config.L3Passthrough {
		name := fmt.Sprintf("%s-static-l3passthrough-%d", r.NodeName, i)
		obj := l3pass.DeepCopy()
		obj.Name = name
		obj.Namespace = r.Namespace
		if obj.Labels == nil {
			obj.Labels = make(map[string]string)
		}
		obj.Labels[LabelMirrored] = LabelValueMirrored
		obj.Labels[LabelSourceNode] = r.NodeName
		obj.Spec.NodeSelector = nodeSelector
		desired[name] = obj
	}

	for i, rawfrr := range config.RawFRRConfigs {
		name := fmt.Sprintf("%s-static-rawfrrconfig-%d", r.NodeName, i)
		obj := rawfrr.DeepCopy()
		obj.Name = name
		obj.Namespace = r.Namespace
		if obj.Labels == nil {
			obj.Labels = make(map[string]string)
		}
		obj.Labels[LabelMirrored] = LabelValueMirrored
		obj.Labels[LabelSourceNode] = r.NodeName
		obj.Spec.NodeSelector = nodeSelector
		desired[name] = obj
	}
}

func (r *ConfigMirrorController) createOrUpdate(ctx context.Context, desired client.Object, logger *slog.Logger) error {
	var result controllerutil.OperationResult
	var err error

	switch d := desired.(type) {
	case *v1alpha1.Underlay:
		existing := &v1alpha1.Underlay{}
		existing.Name = d.Name
		existing.Namespace = d.Namespace
		result, err = controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
			existing.Spec = d.Spec
			existing.Labels = d.Labels
			return nil
		})
	case *v1alpha1.L3VNI:
		existing := &v1alpha1.L3VNI{}
		existing.Name = d.Name
		existing.Namespace = d.Namespace
		result, err = controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
			existing.Spec = d.Spec
			existing.Labels = d.Labels
			return nil
		})
	case *v1alpha1.L2VNI:
		existing := &v1alpha1.L2VNI{}
		existing.Name = d.Name
		existing.Namespace = d.Namespace
		result, err = controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
			existing.Spec = d.Spec
			existing.Labels = d.Labels
			return nil
		})
	case *v1alpha1.L3Passthrough:
		existing := &v1alpha1.L3Passthrough{}
		existing.Name = d.Name
		existing.Namespace = d.Namespace
		result, err = controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
			existing.Spec = d.Spec
			existing.Labels = d.Labels
			return nil
		})
	case *v1alpha1.RawFRRConfig:
		existing := &v1alpha1.RawFRRConfig{}
		existing.Name = d.Name
		existing.Namespace = d.Namespace
		result, err = controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
			existing.Spec = d.Spec
			existing.Labels = d.Labels
			return nil
		})
	default:
		return fmt.Errorf("unknown object type: %T", desired)
	}

	if err != nil {
		return err
	}

	switch result {
	case controllerutil.OperationResultCreated:
		logger.Info("created mirrored CR", "name", desired.GetName(), "kind", desired.GetObjectKind().GroupVersionKind().Kind)
	case controllerutil.OperationResultUpdated:
		logger.Info("updated mirrored CR", "name", desired.GetName(), "kind", desired.GetObjectKind().GroupVersionKind().Kind)
	}

	return nil
}

func (r *ConfigMirrorController) deleteStale(ctx context.Context, desired map[string]client.Object, logger *slog.Logger) error {
	listOpts := []client.ListOption{
		client.InNamespace(r.Namespace),
		client.MatchingLabels{
			LabelMirrored:   LabelValueMirrored,
			LabelSourceNode: r.NodeName,
		},
	}

	var underlays v1alpha1.UnderlayList
	if err := r.List(ctx, &underlays, listOpts...); err != nil {
		return fmt.Errorf("failed to list underlays: %w", err)
	}
	if err := r.deleteStaleFromList(ctx, desired, ptrsToClientObjects(underlays.Items), "Underlay", logger); err != nil {
		return err
	}

	var l3vnis v1alpha1.L3VNIList
	if err := r.List(ctx, &l3vnis, listOpts...); err != nil {
		return fmt.Errorf("failed to list l3vnis: %w", err)
	}
	if err := r.deleteStaleFromList(ctx, desired, ptrsToClientObjects(l3vnis.Items), "L3VNI", logger); err != nil {
		return err
	}

	var l2vnis v1alpha1.L2VNIList
	if err := r.List(ctx, &l2vnis, listOpts...); err != nil {
		return fmt.Errorf("failed to list l2vnis: %w", err)
	}
	if err := r.deleteStaleFromList(ctx, desired, ptrsToClientObjects(l2vnis.Items), "L2VNI", logger); err != nil {
		return err
	}

	var l3passthroughs v1alpha1.L3PassthroughList
	if err := r.List(ctx, &l3passthroughs, listOpts...); err != nil {
		return fmt.Errorf("failed to list l3passthroughs: %w", err)
	}
	if err := r.deleteStaleFromList(ctx, desired, ptrsToClientObjects(l3passthroughs.Items), "L3Passthrough", logger); err != nil {
		return err
	}

	var rawfrrconfigs v1alpha1.RawFRRConfigList
	if err := r.List(ctx, &rawfrrconfigs, listOpts...); err != nil {
		return fmt.Errorf("failed to list rawfrrconfigs: %w", err)
	}
	return r.deleteStaleFromList(ctx, desired, ptrsToClientObjects(rawfrrconfigs.Items), "RawFRRConfig", logger)
}

func (r *ConfigMirrorController) cleanupAllMirroredCRs(ctx context.Context, logger *slog.Logger) error {
	return r.deleteStale(ctx, make(map[string]client.Object), logger)
}

func (r *ConfigMirrorController) deleteStaleFromList(ctx context.Context, desired map[string]client.Object, objects []client.Object, kind string, logger *slog.Logger) error {
	for _, obj := range objects {
		if _, exists := desired[obj.GetName()]; exists {
			continue
		}
		if err := r.Delete(ctx, obj); err != nil {
			return fmt.Errorf("failed to delete %s %s: %w", kind, obj.GetName(), err)
		}
		logger.Info("deleted stale mirrored CR", "name", obj.GetName(), "kind", kind)
	}
	return nil
}

func ptrsToClientObjects[T any, PT interface {
	*T
	client.Object
}](items []T) []client.Object {
	result := make([]client.Object, len(items))
	for i := range items {
		result[i] = PT(&items[i])
	}
	return result
}
