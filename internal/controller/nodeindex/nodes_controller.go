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

package nodeindex

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "k8s.io/api/core/v1"
)

type requestKey string

type NodesReconciler struct {
	client.Client
	clientset     *kubernetes.Clientset
	Scheme        *runtime.Scheme
	MyNamespace   string
	PodName       string
	LogLevel      string
	Logger        *slog.Logger
	ReconcileChan chan event.GenericEvent
	iAmLeader     bool
	sync.Mutex
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update

func (r *NodesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.With("request", req.NamespacedName.String())
	logger.Info("controller", "NodeReconciler", "start reconcile")
	defer logger.Info("controller", "NodeReconciler", "end reconcile")

	r.Lock()
	iAmLeader := r.iAmLeader
	r.Unlock()

	if !iAmLeader {
		logger.Debug("skipping reconcile", "reason", "i am not leader")
		return ctrl.Result{}, nil
	}

	ctx = context.WithValue(ctx, requestKey("request"), req.NamespacedName.String())

	var nodes v1.NodeList
	if err := r.Client.List(ctx, &nodes); err != nil {
		slog.Error("failed to list nodes", "error", err)
		return ctrl.Result{}, err
	}

	nodesToAnnotate := nodesToAnnotate(nodes.Items)
	for _, n := range nodesToAnnotate {
		if err := r.Client.Update(ctx, &n); err != nil {
			slog.Error("failed to update node", "node", n.Name, "error", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

const perouterLeaderLease = "perouter-leader"

func (r *NodesReconciler) monitorLeader(ctx context.Context, config *rest.Config, myName, namespace string) error {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %s", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{Name: perouterLeaderLease, Namespace: namespace},
		Client:    clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: myName,
		},
	}

	go leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				r.onLeaderChanged(true)
			},
			OnStoppedLeading: func() {
				r.onLeaderChanged(false)
			},
		},
	})
	return nil
}

func (r *NodesReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.monitorLeader(ctx, mgr.GetConfig(), r.PodName, r.MyNamespace)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Node{}).
		Named("nodecontroller").
		Complete(r)
}

func (r *NodesReconciler) onLeaderChanged(iAmLeader bool) {
	r.Lock()
	defer r.Unlock()
	if r.iAmLeader == iAmLeader {
		return
	}
	slog.Info("node leader changed", "i am leader", iAmLeader)
	r.iAmLeader = iAmLeader
	r.ReconcileChan <- newLeaderEvent("leader", "leader")
}

type leaderEvent struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (evt *leaderEvent) DeepCopyObject() runtime.Object {
	res := new(leaderEvent)
	res.Name = evt.Name
	res.Namespace = evt.Namespace
	return res
}

func newLeaderEvent(namespace, name string) event.GenericEvent {
	evt := leaderEvent{}
	evt.Name = name
	evt.Namespace = namespace
	return event.GenericEvent{Object: &evt}
}
