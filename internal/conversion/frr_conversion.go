// SPDX-License-Identifier:Apache-2.0

package conversion

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/openperouter/openperouter/api/grpc"
	"github.com/openperouter/openperouter/internal/frr"
	"github.com/openperouter/openperouter/internal/ipam"
	"github.com/openperouter/openperouter/internal/ipfamily"
	"k8s.io/utils/ptr"
)

const defaultRouterIDCidr = "10.0.0.0/24"

type FRREmptyConfigError string

func (e FRREmptyConfigError) Error() string {
	return string(e)
}

func APItoFRR(config ApiConfigData) (frr.Config, error) {
	if len(config.Underlays) > 1 {
		return frr.Config{}, errors.New("multiple underlays defined")
	}
	if len(config.Underlays) == 0 {
		return frr.Config{}, FRREmptyConfigError("no underlays provided")
	}
	if len(config.L3Passthrough) > 1 {
		return frr.Config{}, errors.New("multiple passthrough defined, can have only one")
	}

	underlay := config.Underlays[0]

	underlayNeighbors := []frr.NeighborConfig{}
	bfdProfiles := []frr.BFDProfile{}
	for _, n := range underlay.Neighbors {
		frrNeigh, err := neighborToFRR(n)
		if err != nil {
			return frr.Config{}, fmt.Errorf("failed to translate underlay neighbor %s to frr, err: %w", neighborName(n), err)
		}

		bfdProfile := bfdProfileForNeighbor(n)
		underlayNeighbors = append(underlayNeighbors, *frrNeigh)
		if bfdProfile != nil {
			bfdProfiles = append(bfdProfiles, *bfdProfile)
		}
	}

	routerID, err := routerIDFromUnderlay(underlay, config.NodeIndex)
	if err != nil {
		return frr.Config{}, fmt.Errorf("failed to get routerID: %w", err)
	}

	underlayConfig := frr.UnderlayConfig{
		MyASN:     underlay.Asn,
		RouterID:  routerID,
		Neighbors: underlayNeighbors,
	}

	if len(config.L3Passthrough) > 0 {
		passthrough, err := passthroughToFRR(&config.L3Passthrough[0], config.NodeIndex)
		if err != nil {
			return frr.Config{}, fmt.Errorf("failed to translate passthrough to frr: %w", err)
		}
		underlayConfig.Passthrough = passthrough
	}

	if len(config.L3VNIs) > 0 && underlay.Evpn == nil {
		return frr.Config{}, fmt.Errorf("EVPN configuration is required when L3 VNIs are defined")
	}
	if underlay.Evpn == nil {
		return frr.Config{
			Underlay:    underlayConfig,
			BFDProfiles: bfdProfiles,
			Loglevel:    config.LogLevel,
			VNIs:        []frr.L3VNIConfig{},
		}, nil
	}

	vtepIP, err := ipam.VTEPIp(underlay.Evpn.VtepCidr, config.NodeIndex)
	if err != nil {
		return frr.Config{}, fmt.Errorf("failed to get vtep ip, cidr %s, nodeIntex %d", underlay.Evpn.VtepCidr, config.NodeIndex)
	}
	underlayConfig.EVPN = &frr.UnderlayEvpn{
		VTEP: vtepIP.String(),
	}

	vniConfigs := []frr.L3VNIConfig{}
	for i := range config.L3VNIs {
		vni := &config.L3VNIs[i]
		frrVNI, err := l3vniToFRR(vni, routerID, underlay.Asn, config.NodeIndex)
		if err != nil {
			return frr.Config{}, fmt.Errorf("failed to translate vni to frr: %w, vni %v", err, vni)
		}
		vniConfigs = append(vniConfigs, frrVNI...)
	}

	return frr.Config{
		Underlay:    underlayConfig,
		VNIs:        vniConfigs,
		BFDProfiles: bfdProfiles,
		Loglevel:    config.LogLevel,
	}, nil
}

func passthroughToFRR(passthrough *grpc.L3Passthrough, nodeIndex int) (*frr.PassthroughConfig, error) {
	vethIPs, err := ipam.VethIPsFromPool(passthrough.HostSession.LocalCidr.Ipv4, passthrough.HostSession.LocalCidr.Ipv6, nodeIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to get veth ips, cidr %v, nodeIndex %d", passthrough.HostSession.LocalCidr, nodeIndex)
	}

	res := &frr.PassthroughConfig{
		ToAdvertiseIPv4: []string{},
		ToAdvertiseIPv6: []string{},
	}

	if vethIPs.Ipv4.HostSide.IP != nil {
		res.LocalNeighborV4 = &frr.NeighborConfig{
			ASN:  passthrough.HostSession.HostAsn,
			Addr: vethIPs.Ipv4.HostSide.IP.String(),
		}
		ipnet := net.IPNet{
			IP:   vethIPs.Ipv4.HostSide.IP,
			Mask: net.CIDRMask(32, 32),
		}

		res.ToAdvertiseIPv4 = append(res.ToAdvertiseIPv4, ipnet.String())
	}
	if vethIPs.Ipv6.HostSide.IP != nil {
		res.LocalNeighborV6 = &frr.NeighborConfig{
			ASN:  passthrough.HostSession.HostAsn,
			Addr: vethIPs.Ipv6.HostSide.IP.String(),
		}

		ipnet := net.IPNet{
			IP:   vethIPs.Ipv6.HostSide.IP,
			Mask: net.CIDRMask(128, 128),
		}
		res.ToAdvertiseIPv6 = append(res.ToAdvertiseIPv6, ipnet.String())
	}

	return res, nil
}

func l3vniToFRR(vni *grpc.L3VNI, routerID string, underlayASN uint32, nodeIndex int) ([]frr.L3VNIConfig, error) {
	if vni.HostSession == nil { // no neighbor, just the vni / vrf
		return []frr.L3VNIConfig{
			{
				VNI:      int(vni.Vni),
				VRF:      vni.VRFName(),
				ASN:      underlayASN, // Since there is no session, the ASN is arbitrary
				RouterID: routerID,
			},
		}, nil
	}

	veths, err := ipam.VethIPsFromPool(vni.HostSession.LocalCidr.Ipv4, vni.HostSession.LocalCidr.Ipv6, nodeIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to get veths ips for vni: %w", err)
	}

	var configs []frr.L3VNIConfig

	// Create IPv4 neighbor if IPv4 IP is available
	if veths.Ipv4.HostSide.IP != nil {
		config := createVNIConfig(vni, veths.Ipv4.HostSide.IP, net.CIDRMask(32, 32), routerID)
		configs = append(configs, config)
	}

	// Create IPv6 neighbor if IPv6 IP is available
	if veths.Ipv6.HostSide.IP != nil {
		config := createVNIConfig(vni, veths.Ipv6.HostSide.IP, net.CIDRMask(128, 128), routerID)
		configs = append(configs, config)
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no valid host side IP found for vni %s", vni.Key())
	}

	return configs, nil
}

// createVNIConfig creates a VNI configuration for a specific IP family
func createVNIConfig(vni *grpc.L3VNI, hostIP net.IP, mask net.IPMask, routerID string) frr.L3VNIConfig {
	vniNeighbor := &frr.NeighborConfig{
		Addr: hostIP.String(),
	}
	vniNeighbor.ASN = vni.HostSession.Asn
	if vni.HostSession.HostAsn != 0 {
		vniNeighbor.ASN = vni.HostSession.HostAsn
	}

	ipnet := net.IPNet{
		IP:   hostIP,
		Mask: mask,
	}

	config := frr.L3VNIConfig{
		ASN:           vni.HostSession.Asn,
		VNI:           int(vni.Vni),
		VRF:           vni.VRFName(),
		RouterID:      routerID,
		LocalNeighbor: vniNeighbor,
	}

	ipFamily := ipfamily.ForAddress(hostIP)
	if ipFamily == ipfamily.IPv4 {
		config.ToAdvertiseIPv4 = []string{ipnet.String()}
		config.ToAdvertiseIPv6 = []string{}
		return config
	}

	// Else ipv6

	config.ToAdvertiseIPv4 = []string{}
	config.ToAdvertiseIPv6 = []string{ipnet.String()}
	return config
}

func neighborToFRR(n *grpc.Neighbor) (*frr.NeighborConfig, error) {
	neighborFamily, err := ipfamily.ForAddresses(n.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to find ipfamily for %s, %w", n.Address, err)
	}

	if n.Asn == 0 {
		return nil, fmt.Errorf("neighbor %s does not have ASN", n.Address)
	}

	res := &frr.NeighborConfig{
		Name:         neighborName(n),
		ASN:          n.Asn,
		Addr:         n.Address,
		IPFamily:     neighborFamily,
		EBGPMultiHop: n.EbgpMultiHop,
	}
	if n.Port != 0 {
		res.Port = ptr.To(n.Port)
	}
	res.HoldTime, res.KeepaliveTime, err = parseTimers(n.HoldTime, n.KeepaliveTime)
	if err != nil {
		return nil, fmt.Errorf("invalid timers for neighbor %s, err: %w", neighborName(n), err)
	}

	if n.ConnectTime != 0 {
		connectSecond, err := durationToUint64(time.Duration(n.ConnectTime) * time.Second)
		if err != nil {
			return nil, fmt.Errorf("invalid connecttime %v: %w", time.Duration(n.ConnectTime)*time.Second, err)
		}
		res.ConnectTime = ptr.To(connectSecond)
	}

	if n.Bfd == nil {
		return res, nil
	}

	res.BFDEnabled = true
	if n.Bfd.IsEmpty() {
		return res, nil
	}
	res.BFDProfile = bfdProfileNameForNeighbor(n)

	return res, nil
}

func bfdProfileForNeighbor(n *grpc.Neighbor) *frr.BFDProfile {
	if n.Bfd == nil {
		return nil
	}

	if n.Bfd.IsEmpty() {
		return nil
	}

	profileName := bfdProfileNameForNeighbor(n)
	bfdProfile := &frr.BFDProfile{
		Name:             profileName,
		ReceiveInterval:  &n.Bfd.ReceiveInterval,
		TransmitInterval: &n.Bfd.TransmitInterval,
		DetectMultiplier: &n.Bfd.DetectMultiplier,
		EchoMode:         n.Bfd.EchoMode,
		PassiveMode:      n.Bfd.PassiveMode,
	}

	if n.Bfd.EchoInterval != 0 {
		bfdProfile.EchoInterval = &n.Bfd.EchoInterval
	}
	if n.Bfd.MinimumTtl != 0 {
		bfdProfile.MinimumTTL = &n.Bfd.MinimumTtl
	}

	return bfdProfile
}

func bfdProfileNameForNeighbor(n *grpc.Neighbor) string {
	return fmt.Sprintf("neighbor-%s", n.Address)
}

func neighborName(n *grpc.Neighbor) string {
	return fmt.Sprintf("%d@%s", n.Asn, n.Address)
}

func parseTimers(ht, ka int64) (*uint64, *uint64, error) {
	if ht == 0 && ka != 0 || ht != 0 && ka == 0 {
		return nil, nil, fmt.Errorf("one of KeepaliveTime/HoldTime specified, both must be set or none")
	}

	if ht == 0 && ka == 0 {
		return nil, nil, nil
	}

	holdTime := time.Duration(ht) * time.Second
	keepaliveTime := time.Duration(ka) * time.Second

	if holdTime != 0 && holdTime < 3*time.Second {
		return nil, nil, fmt.Errorf("invalid hold time %v: must be 0 or >=3s", holdTime)
	}

	if keepaliveTime > holdTime {
		return nil, nil, fmt.Errorf("invalid keepaliveTime %v, must be lower than holdTime %v", keepaliveTime, holdTime)
	}

	htSeconds, err := durationToUint64(holdTime / time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid hold time %v: %w", holdTime, err)
	}
	kaSeconds, err := durationToUint64(keepaliveTime / time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid keepalive time %v: %w", keepaliveTime, err)
	}

	return &htSeconds, &kaSeconds, nil
}

func durationToUint64(value time.Duration) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("cannot convert negative value to uint64: %d", value)
	}
	return uint64(value), nil // #nosec G115
}

func routerIDFromUnderlay(underlay *grpc.Underlay, nodeIndex int) (string, error) {
	routerIDCidr := underlay.RouterIdCidr
	if underlay.RouterIdCidr == "" {
		routerIDCidr = defaultRouterIDCidr
		slog.Info("empty routerid cidr, using the default one", "underlay", underlay.Key(), "default cidr", defaultRouterIDCidr)
	}
	routerID, err := ipam.RouterID(routerIDCidr, nodeIndex)
	if err != nil {
		return "", fmt.Errorf("failed to get router id, cidr %s, nodeIndex %d: %w", underlay.RouterIdCidr, nodeIndex, err)
	}
	return routerID, nil
}
