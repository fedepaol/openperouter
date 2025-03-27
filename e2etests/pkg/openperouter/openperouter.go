package openperouter

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/openperouter/openperouter/e2etests/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clientset "k8s.io/client-go/kubernetes"
)

const (
	Namespace           = "openperouter-system"
	routerLabelSelector = "app=router"
)

func RouterPods(cs clientset.Interface) ([]*corev1.Pod, error) {
	routers, err := cs.CoreV1().Pods(Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: routerLabelSelector,
	})
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to fetch router pods"))
	}
	if len(routers.Items) == 0 {
		return nil, errors.New("no router pods found")
	}
	routerPods := make([]*corev1.Pod, 0)
	for _, item := range routers.Items {
		i := item
		routerPods = append(routerPods, &i)
	}
	return routerPods, nil
}

func DaemonsetRolled(cs clientset.Interface, oldRouterPods []*corev1.Pod) error {
	oldPodsNames := []string{}
	for _, p := range oldRouterPods {
		oldPodsNames = append(oldPodsNames, p.Name)
	}
	routerPods, err := RouterPods(cs)
	if err != nil {
		return err
	}
	if len(routerPods) != len(oldPodsNames) {
		return fmt.Errorf("new pods len %d different from old pods len: %d", len(routerPods), len(oldPodsNames))
	}

	for _, p := range routerPods {
		if slices.Contains(oldPodsNames, p.Name) {
			return fmt.Errorf("old pod %s not deleted yet", p.Name)
		}
		if !k8s.PodIsReady(p) {
			return fmt.Errorf("pod %s is not ready", p.Name)
		}
	}
	return nil
}
