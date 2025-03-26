package openperouter

import (
	"fmt"
	"net"
	"strconv"

	gocidr "github.com/apparentlymart/go-cidr/cidr"
	corev1 "k8s.io/api/core/v1"
)

const nodeIndexAnnotation = "openpe.io/nodeindex"

func VtepIPForNode(cidr string, node *corev1.Node) (string, error) {
	if node.Annotations == nil ||
		node.Annotations[nodeIndexAnnotation] == "" {
		return "", fmt.Errorf("no index for node %s", node.Name)
	}
	nodeIndex, err := strconv.Atoi(node.Annotations[nodeIndexAnnotation])
	_, vtepCIDR, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("failed to parse cidr %s: %w", cidr, err)
	}

	ip, err := gocidr.Host(vtepCIDR, nodeIndex)
	if err != nil {
		return "", fmt.Errorf("failed to get index %d from cidr %s", nodeIndex, vtepCIDR)
	}
	return ip.String(), nil

}
