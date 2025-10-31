// SPDX-License-Identifier:Apache-2.0

package openperouter

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/openperouter/openperouter/e2etests/pkg/executor"
	"github.com/openperouter/openperouter/e2etests/pkg/k8s"
	corev1 "k8s.io/api/core/v1"

	clientset "k8s.io/client-go/kubernetes"
)

const (
	Namespace           = "openperouter-system"
	routerLabelSelector = "app=router"
)

type Routers interface {
}

type routerPods struct {
	pods []*corev1.Pod
}

type routerPodmans struct {
	routers []routerPodman
}

type routerPodman struct {
	nodeName string
	pid      string
}

func Get(cs clientset.Interface, hostMode bool) (Routers, error) {
	if !hostMode {
		pods, err := k8s.PodsForLabel(cs, Namespace, routerLabelSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve pods %w", err)
		}
		return routerPods{pods: pods}, nil
	}

	nodes, err := k8s.GetNodes(cs)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve nodes %w", err)
	}

	routers := []routerPodman{}
	for _, node := range nodes {
		pid, err := getPodmanRouterPID(node.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get router pod PID for node %s: %w", node.Name, err)
		}
		routers = append(routers, routerPodman{
			nodeName: node.Name,
			pid:      pid,
		})
	}

	return routerPodmans{routers: routers}, nil
}

// DaemonsetRolled checks if routers have been rolled/restarted by comparing old and new state
// For routerPods: checks if pods were deleted and recreated (names changed)
// For routerPodmans: checks if pods were restarted (PIDs changed)
func DaemonsetRolled(oldRouters Routers, newRouters Routers) error {
	// Type assert to determine which type of routers we're dealing with
	switch old := oldRouters.(type) {
	case routerPods:
		new, ok := newRouters.(routerPods)
		if !ok {
			return fmt.Errorf("old routers are routerPods but new routers are %T", newRouters)
		}
		return daemonsetPodRolled(old, new)
	case routerPodmans:
		new, ok := newRouters.(routerPodmans)
		if !ok {
			return fmt.Errorf("old routers are routerPodmans but new routers are %T", newRouters)
		}
		return podmanRolled(old, new)
	default:
		return fmt.Errorf("unknown router type: %T", oldRouters)
	}
}

func (r routerPods) Dump(writer io.Writer, name string) {
	ginkgo.GinkgoWriter.Printf("%s pods are: %s", name)
	for _, pod := range r.pods {
		ginkgo.GinkgoWriter.Printf("Pod %s/%s: %s", pod.Namespace, pod.Name, pod.Status.Phase)
		ginkgo.GinkgoWriter.Printf("  Node: %s", pod.Spec.NodeName)
		ginkgo.GinkgoWriter.Printf("  IPs: %v", pod.Status.PodIPs)
		ginkgo.GinkgoWriter.Printf("  Containers:")
		for _, c := range pod.Spec.Containers {
			ginkgo.GinkgoWriter.Printf("    - %s: %s", c.Name, c.Image)
		}
		ginkgo.GinkgoWriter.Print("\n")
	}
}

func daemonsetPodRolled(oldRouters, newRouters routerPods) error {
	oldPodsNames := []string{}
	for _, p := range oldRouters.pods {
		oldPodsNames = append(oldPodsNames, p.Name)
	}

	if len(newRouters.pods) != len(oldPodsNames) {
		return fmt.Errorf("new pods len %d different from old pods len: %d", len(newRouters.pods), len(oldPodsNames))
	}

	for _, p := range newRouters.pods {
		if slices.Contains(oldPodsNames, p.Name) {
			return fmt.Errorf("old pod %s not deleted yet", p.Name)
		}
		if !k8s.PodIsReady(p) {
			return fmt.Errorf("pod %s is not ready", p.Name)
		}
	}
	return nil
}

func podmanRolled(oldRouters, newRouters routerPodmans) error {
	// Check same number of routers
	if len(newRouters.routers) != len(oldRouters.routers) {
		return fmt.Errorf("new routers len %d different from old routers len: %d", len(newRouters.routers), len(oldRouters.routers))
	}

	oldPIDs := make(map[string]string)
	for _, router := range oldRouters.routers {
		oldPIDs[router.nodeName] = router.pid
	}

	// Check that all PIDs have changed (indicating restart)
	for _, newRouter := range newRouters.routers {
		oldPID, exists := oldPIDs[newRouter.nodeName]
		if !exists {
			return fmt.Errorf("new router found on node %s that was not in old routers", newRouter.nodeName)
		}
		if newRouter.pid == oldPID {
			return fmt.Errorf("router on node %s has same PID %s (not restarted)", newRouter.nodeName, newRouter.pid)
		}
	}

	return nil
}

func getPodmanRouterPID(nodeName string) (string, error) {
	exec := executor.ForContainer(nodeName)
	// Read the PID from the file written by the router pod
	out, err := exec.Exec("cat", "/etc/perouter/frr/frr.pid")
	if err != nil {
		return "", fmt.Errorf("failed to read PID file: %w, output: %s", err, out)
	}
	pid := strings.TrimSpace(out)
	if pid == "" || pid == "0" {
		return "", fmt.Errorf("invalid PID in file: %s", pid)
	}
	return pid, nil
}
