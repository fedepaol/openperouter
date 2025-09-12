// SPDX-License-Identifier:Apache-2.0

package conversion

import (
	"fmt"
	"net"

	"github.com/openperouter/openperouter/internal/hostnetwork"
	"github.com/openperouter/openperouter/internal/ipam"
)

func APItoHostConfig(nodeIndex int, targetNS string, apiConfig ApiConfigData) (HostConfigData, error) {
	res := HostConfigData{
		L3VNIs: []hostnetwork.L3VNIParams{},
		L2VNIs: []hostnetwork.L2VNIParams{},
	}
	if len(apiConfig.Underlays) > 1 {
		return res, fmt.Errorf("can't have more than one underlay")
	}
	if len(apiConfig.L3Passthrough) > 1 {
		return res, fmt.Errorf("can't have more than one passthrough")
	}
	if len(apiConfig.Underlays) == 0 {
		return res, nil
	}

	underlay := apiConfig.Underlays[0]

	res.Underlay = hostnetwork.UnderlayParams{
		UnderlayInterface: underlay.Nics[0],
		TargetNS:          targetNS,
	}

	if len(apiConfig.L3Passthrough) == 1 {
		vethIPs, err := ipam.VethIPsFromPool(apiConfig.L3Passthrough[0].HostSession.LocalCidr.Ipv4, apiConfig.L3Passthrough[0].HostSession.LocalCidr.Ipv6, nodeIndex)
		if err != nil {
			return res, fmt.Errorf("failed to get veth ips, cidr %v, nodeIndex %d", apiConfig.L3Passthrough[0].HostSession.LocalCidr, nodeIndex)
		}

		res.L3Passthrough = &hostnetwork.PassthroughParams{
			TargetNS: targetNS,
			HostVeth: hostnetwork.Veth{
				HostIPv4: ipNetToString(vethIPs.Ipv4.HostSide),
				NSIPv4:   ipNetToString(vethIPs.Ipv4.PeSide),
				HostIPv6: ipNetToString(vethIPs.Ipv6.HostSide),
				NSIPv6:   ipNetToString(vethIPs.Ipv6.PeSide),
			},
		}
	}

	if underlay.Evpn == nil && (len(apiConfig.L3VNIs) > 0 || len(apiConfig.L2VNIs) > 0) {
		return res, fmt.Errorf("underlay EVPN configuration is required when L3 or L2 VNIs are defined")
	}

	if underlay.Evpn == nil {
		return res, nil
	}

	vtepIP, err := ipam.VTEPIp(underlay.Evpn.VtepCidr, nodeIndex)
	if err != nil {
		return res, fmt.Errorf("failed to get vtep ip, cidr %s, nodeIntex %d", underlay.Evpn.VtepCidr, nodeIndex)
	}
	res.Underlay.EVPN = &hostnetwork.UnderlayEVPNParams{
		VtepIP: vtepIP.String(),
	}

	for i := range apiConfig.L3VNIs {
		vni := apiConfig.L3VNIs[i]
		v := hostnetwork.L3VNIParams{
			VNIParams: hostnetwork.VNIParams{
				VRF:       vni.VRFName(),
				TargetNS:  targetNS,
				VTEPIP:    vtepIP.String(),
				VNI:       int(vni.Vni),
				VXLanPort: int(vni.VxlanPort),
			},
		}
		if vni.HostSession == nil {
			res.L3VNIs = append(res.L3VNIs, v)
			continue
		}

		vethIPs, err := ipam.VethIPsFromPool(vni.HostSession.LocalCidr.Ipv4, vni.HostSession.LocalCidr.Ipv6, nodeIndex)
		if err != nil {
			return res, fmt.Errorf("failed to get veth ips, cidr %v, nodeIndex %d", vni.HostSession.LocalCidr, nodeIndex)
		}

		v.HostVeth = &hostnetwork.Veth{
			HostIPv4: ipNetToString(vethIPs.Ipv4.HostSide),
			NSIPv4:   ipNetToString(vethIPs.Ipv4.PeSide),
			HostIPv6: ipNetToString(vethIPs.Ipv6.HostSide),
			NSIPv6:   ipNetToString(vethIPs.Ipv6.PeSide),
		}

		res.L3VNIs = append(res.L3VNIs, v)
	}

	res.L2VNIs = []hostnetwork.L2VNIParams{}
	for i := range apiConfig.L2VNIs {
		l2vni := apiConfig.L2VNIs[i]
		vni := hostnetwork.L2VNIParams{
			VNIParams: hostnetwork.VNIParams{
				VRF:       l2vni.VRFName(),
				TargetNS:  targetNS,
				VTEPIP:    vtepIP.String(),
				VNI:       int(l2vni.Vni),
				VXLanPort: int(l2vni.VxlanPort),
			},
		}
		if l2vni.L2GatewayIp != "" {
			vni.L2GatewayIP = &l2vni.L2GatewayIp
		}
		if l2vni.HostMaster != nil {
			vni.HostMaster = &hostnetwork.HostMaster{
				Name:       l2vni.HostMaster.Name,
				AutoCreate: l2vni.HostMaster.AutoCreate,
			}
		}

		res.L2VNIs = append(res.L2VNIs, vni)
	}

	return res, nil
}

// ipNetToString returns the string representation of the IPNet, or empty string if IP is nil
func ipNetToString(ipNet net.IPNet) string {
	if ipNet.IP == nil {
		return ""
	}
	return ipNet.String()
}
