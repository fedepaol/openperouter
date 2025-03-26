package openperouter

import (
	"context"
	"errors"

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
