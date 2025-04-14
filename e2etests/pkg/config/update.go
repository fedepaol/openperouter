// SPDX-License-Identifier:Apache-2.0

package config

import (
	"context"

	frrk8sv1beta1 "github.com/metallb/frr-k8s/api/v1beta1"
	"github.com/openperouter/openperouter/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Resources struct {
	Underlays []v1alpha1.Underlay `json:"underlays"`
	VNIs      []v1alpha1.VNI      `json:"vnis"`
}

type Updater interface {
	Update(r Resources) error
	CleanAll() error
	CleanButUnderlay() error
	Client() client.Client
	Namespace() string
}

type beta1Updater struct {
	cli       client.Client
	namespace string
}

func UpdaterForCRs(r *rest.Config, ns string) (Updater, error) {
	myScheme := runtime.NewScheme()

	if err := v1alpha1.AddToScheme(myScheme); err != nil {
		return nil, err
	}

	if err := corev1.AddToScheme(myScheme); err != nil {
		return nil, err
	}

	if err := frrk8sv1beta1.AddToScheme(myScheme); err != nil {
		return nil, err
	}

	cl, err := client.New(r, client.Options{
		Scheme: myScheme,
	})

	if err != nil {
		return nil, err
	}

	return &beta1Updater{
		cli:       cl,
		namespace: ns,
	}, nil
}

func (o beta1Updater) Update(r Resources) error {
	// we fill a map of objects to keep the order we add the resources random, as
	// it would happen by throwing a set of manifests against a cluster, hoping to
	// find corner cases that we would not find by adding them always in the same
	// order.
	objects := map[int]client.Object{}
	oldValues := map[int]client.Object{}
	key := 0
	for _, underlay := range r.Underlays {
		objects[key] = underlay.DeepCopy()
		oldValues[key] = underlay.DeepCopy()
		key++
	}
	for _, vni := range r.VNIs {
		objects[key] = vni.DeepCopy()
		oldValues[key] = vni.DeepCopy()
		key++
	}

	// Iterating over the map will return the items in a random order.
	for i, obj := range objects {
		obj.SetNamespace(o.namespace)
		_, err := controllerutil.CreateOrUpdate(context.Background(), o.cli, obj, func() error {
			// the mutate function is expected to change the object when updating.
			// we always override with the old version, and we change only the spec part.
			switch toChange := obj.(type) {
			case *v1alpha1.Underlay:
				old := oldValues[i].(*v1alpha1.Underlay)
				toChange.Spec = *old.Spec.DeepCopy()
			case *v1alpha1.VNI:
				old := oldValues[i].(*v1alpha1.VNI)
				toChange.Spec = *old.Spec.DeepCopy()
			}

			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (o beta1Updater) CleanAll() error {
	if err := o.cli.DeleteAllOf(context.Background(), &v1alpha1.Underlay{}, client.InNamespace(o.namespace)); err != nil {
		return err
	}
	if err := o.CleanButUnderlay(); err != nil {
		return err
	}
	return nil
}

func (o beta1Updater) CleanButUnderlay() error {
	err := o.cli.DeleteAllOf(context.Background(), &v1alpha1.VNI{}, client.InNamespace(o.namespace))
	if err != nil {
		return err
	}
	return nil
}

func (o beta1Updater) Client() client.Client {
	return o.cli
}

func (o beta1Updater) Namespace() string {
	return o.namespace
}
