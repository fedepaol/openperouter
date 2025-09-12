// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"fmt"

	pb "github.com/openperouter/openperouter/api/grpc"
	periov1alpha1 "github.com/openperouter/openperouter/api/v1alpha1"
)

type crs struct {
	Underlays      []periov1alpha1.Underlay
	L2VNIs         []periov1alpha1.L2VNI
	L3VNIs         []periov1alpha1.L3VNI
	L3Passthroughs []periov1alpha1.L3Passthrough
}

func crdsToUpdateAllRequest(collections crs) (*pb.UpdateAllRequest, error) {
	underlays := make([]*pb.Underlay, 0, len(collections.Underlays))
	for i := range collections.Underlays {
		underlay, err := underlayToAPI(&collections.Underlays[i])
		if err != nil {
			return nil, fmt.Errorf("failed to convert underlay %s: %w", collections.Underlays[i].Name, err)
		}
		underlays = append(underlays, underlay)
	}

	l2vnis := make([]*pb.L2VNI, 0, len(collections.L2VNIs))
	for i := range collections.L2VNIs {
		l2vni, err := l2vniToAPI(&collections.L2VNIs[i])
		if err != nil {
			return nil, fmt.Errorf("failed to convert L2VNI %s: %w", collections.L2VNIs[i].Name, err)
		}
		l2vnis = append(l2vnis, l2vni)
	}

	l3vnis := make([]*pb.L3VNI, 0, len(collections.L3VNIs))
	for i := range collections.L3VNIs {
		l3vni, err := l3vniToAPI(&collections.L3VNIs[i])
		if err != nil {
			return nil, fmt.Errorf("failed to convert L3VNI %s: %w", collections.L3VNIs[i].Name, err)
		}
		l3vnis = append(l3vnis, l3vni)
	}

	l3passthroughs := make([]*pb.L3Passthrough, 0, len(collections.L3Passthroughs))
	for i := range collections.L3Passthroughs {
		l3passthrough, err := l3PassthroughToAPI(&collections.L3Passthroughs[i])
		if err != nil {
			return nil, fmt.Errorf("failed to convert L3Passthrough %s: %w", collections.L3Passthroughs[i].Name, err)
		}
		l3passthroughs = append(l3passthroughs, l3passthrough)
	}

	return &pb.UpdateAllRequest{
		L2Vnis:         l2vnis,
		L3Vnis:         l3vnis,
		L3Passthroughs: l3passthroughs,
		Underlays:      underlays,
	}, nil
}

func underlayToAPI(k8sUnderlay *periov1alpha1.Underlay) (*pb.Underlay, error) {
	if k8sUnderlay == nil {
		return nil, fmt.Errorf("underlay cannot be nil")
	}

	neighbors := make([]*pb.Neighbor, 0, len(k8sUnderlay.Spec.Neighbors))
	for i := range k8sUnderlay.Spec.Neighbors {
		neighbor, err := neighborToAPI(&k8sUnderlay.Spec.Neighbors[i])
		if err != nil {
			return nil, fmt.Errorf("failed to convert neighbor %s: %w", k8sUnderlay.Spec.Neighbors[i].Address, err)
		}
		neighbors = append(neighbors, neighbor)
	}

	res := &pb.Underlay{
		Asn:          k8sUnderlay.Spec.ASN,
		RouterIdCidr: k8sUnderlay.Spec.RouterIDCIDR,
		Neighbors:    neighbors,
		Nics:         k8sUnderlay.Spec.Nics,
	}

	if k8sUnderlay.Spec.EVPN != nil {
		res.Evpn = &pb.EVPNConfig{
			VtepCidr: k8sUnderlay.Spec.EVPN.VTEPCIDR,
		}
	}

	return res, nil
}

func neighborToAPI(k8sNeighbor *periov1alpha1.Neighbor) (*pb.Neighbor, error) {
	if k8sNeighbor == nil {
		return nil, fmt.Errorf("neighbor cannot be nil")
	}

	neighbor := &pb.Neighbor{
		Asn:     k8sNeighbor.ASN,
		Address: k8sNeighbor.Address,
	}

	if k8sNeighbor.HostASN != nil {
		neighbor.HostAsn = *k8sNeighbor.HostASN
	}

	if k8sNeighbor.Port != nil {
		neighbor.Port = uint32(*k8sNeighbor.Port)
	}

	neighbor.Password = k8sNeighbor.Password
	neighbor.PasswordSecret = k8sNeighbor.PasswordSecret

	if k8sNeighbor.HoldTime != nil {
		neighbor.HoldTime = k8sNeighbor.HoldTime.Duration.Nanoseconds()
	}

	if k8sNeighbor.KeepaliveTime != nil {
		neighbor.KeepaliveTime = k8sNeighbor.KeepaliveTime.Duration.Nanoseconds()
	}

	if k8sNeighbor.ConnectTime != nil {
		neighbor.ConnectTime = k8sNeighbor.ConnectTime.Duration.Nanoseconds()
	}

	neighbor.EbgpMultiHop = k8sNeighbor.EBGPMultiHop

	if k8sNeighbor.BFD != nil {
		neighbor.Bfd = bfdSettingsToAPI(k8sNeighbor.BFD)
	}

	return neighbor, nil
}

func bfdSettingsToAPI(k8sBFD *periov1alpha1.BFDSettings) *pb.BFDSettings {
	if k8sBFD == nil {
		return nil
	}

	bfd := &pb.BFDSettings{}

	if k8sBFD.ReceiveInterval != nil {
		bfd.ReceiveInterval = *k8sBFD.ReceiveInterval
	}

	if k8sBFD.TransmitInterval != nil {
		bfd.TransmitInterval = *k8sBFD.TransmitInterval
	}

	if k8sBFD.DetectMultiplier != nil {
		bfd.DetectMultiplier = *k8sBFD.DetectMultiplier
	}

	if k8sBFD.EchoInterval != nil {
		bfd.EchoInterval = *k8sBFD.EchoInterval
	}

	if k8sBFD.EchoMode != nil {
		bfd.EchoMode = *k8sBFD.EchoMode
	}

	if k8sBFD.PassiveMode != nil {
		bfd.PassiveMode = *k8sBFD.PassiveMode
	}

	if k8sBFD.MinimumTTL != nil {
		bfd.MinimumTtl = *k8sBFD.MinimumTTL
	}

	return bfd
}

func l2vniToAPI(k8sL2VNI *periov1alpha1.L2VNI) (*pb.L2VNI, error) {
	if k8sL2VNI == nil {
		return nil, fmt.Errorf("L2VNI cannot be nil")
	}

	l2vni := &pb.L2VNI{
		Vrf:         k8sL2VNI.VRFName(),
		Vni:         k8sL2VNI.Spec.VNI,
		VxlanPort:   k8sL2VNI.Spec.VXLanPort,
		L2GatewayIp: k8sL2VNI.Spec.L2GatewayIP,
	}

	if k8sL2VNI.Spec.HostMaster != nil {
		l2vni.HostMaster = &pb.HostMaster{
			Name:       k8sL2VNI.Spec.HostMaster.Name,
			Type:       k8sL2VNI.Spec.HostMaster.Type,
			AutoCreate: k8sL2VNI.Spec.HostMaster.AutoCreate,
		}
	}

	return l2vni, nil
}

func l3vniToAPI(k8sL3VNI *periov1alpha1.L3VNI) (*pb.L3VNI, error) {
	if k8sL3VNI == nil {
		return nil, fmt.Errorf("L3VNI cannot be nil")
	}

	l3vni := &pb.L3VNI{
		Vrf:       k8sL3VNI.VRFName(),
		Vni:       k8sL3VNI.Spec.VNI,
		VxlanPort: k8sL3VNI.Spec.VXLanPort,
	}

	if k8sL3VNI.Spec.HostSession != nil {
		hostSession, err := hostSessionToAPI(k8sL3VNI.Spec.HostSession)
		if err != nil {
			return nil, fmt.Errorf("failed to convert host session: %w", err)
		}
		l3vni.HostSession = hostSession
	}

	return l3vni, nil
}

func l3PassthroughToAPI(k8sL3Passthrough *periov1alpha1.L3Passthrough) (*pb.L3Passthrough, error) {
	if k8sL3Passthrough == nil {
		return nil, fmt.Errorf("L3Passthrough cannot be nil")
	}

	hostSession, err := hostSessionToAPI(&k8sL3Passthrough.Spec.HostSession)
	if err != nil {
		return nil, fmt.Errorf("failed to convert host session: %w", err)
	}

	return &pb.L3Passthrough{
		HostSession: hostSession,
	}, nil
}

func hostSessionToAPI(k8sHostSession *periov1alpha1.HostSession) (*pb.HostSession, error) {
	if k8sHostSession == nil {
		return nil, fmt.Errorf("host session cannot be nil")
	}

	return &pb.HostSession{
		Asn:     k8sHostSession.ASN,
		HostAsn: k8sHostSession.HostASN,
		LocalCidr: &pb.LocalCIDRConfig{
			Ipv4: k8sHostSession.LocalCIDR.IPv4,
			Ipv6: k8sHostSession.LocalCIDR.IPv6,
		},
	}, nil
}
