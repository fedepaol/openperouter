// SPDX-License-Identifier:Apache-2.0

package conversion

import (
	"reflect"
	"testing"

	"github.com/openperouter/openperouter/api/grpc"
	"github.com/openperouter/openperouter/internal/hostnetwork"
)

func TestAPItoHostConfig(t *testing.T) {
	tests := []struct {
		name            string
		nodeIndex       int
		targetNS        string
		underlays       []grpc.Underlay
		vnis            []grpc.L3VNI
		l2vnis          []grpc.L2VNI
		l3Passthrough   []grpc.L3Passthrough
		wantUnderlay    hostnetwork.UnderlayParams
		wantL2VNIParams []hostnetwork.L2VNIParams
		wantL3VNIParams []hostnetwork.L3VNIParams
		wantPassthrough *hostnetwork.PassthroughParams
		wantErr         bool
	}{
		{
			name:            "no underlays",
			nodeIndex:       0,
			targetNS:        "namespace",
			underlays:       []grpc.Underlay{},
			vnis:            []grpc.L3VNI{},
			l2vnis:          []grpc.L2VNI{},
			l3Passthrough:   []grpc.L3Passthrough{},
			wantUnderlay:    hostnetwork.UnderlayParams{},
			wantL3VNIParams: []hostnetwork.L3VNIParams{},
			wantL2VNIParams: []hostnetwork.L2VNIParams{},
			wantPassthrough: nil,
			wantErr:         false,
		},
		{
			name:      "multiple underlays",
			nodeIndex: 0,
			targetNS:  "namespace",
			underlays: []grpc.Underlay{
				{Nics: []string{"eth0"}, Evpn: &grpc.EVPNConfig{VtepCidr: "10.0.0.0/24"}},
				{Nics: []string{"eth1"}, Evpn: &grpc.EVPNConfig{VtepCidr: "10.0.1.0/24"}},
			},
			vnis:            []grpc.L3VNI{},
			l2vnis:          []grpc.L2VNI{},
			l3Passthrough:   []grpc.L3Passthrough{},
			wantUnderlay:    hostnetwork.UnderlayParams{},
			wantL3VNIParams: []hostnetwork.L3VNIParams{},
			wantL2VNIParams: []hostnetwork.L2VNIParams{},
			wantPassthrough: nil,
			wantErr:         true,
		},
		{
			name:      "ipv4 only",
			nodeIndex: 0,
			targetNS:  "namespace",
			underlays: []grpc.Underlay{
				{Nics: []string{"eth0"}, Evpn: &grpc.EVPNConfig{VtepCidr: "10.0.0.0/24"}},
			},
			vnis: []grpc.L3VNI{
				{Vrf: "red", HostSession: &grpc.HostSession{LocalCidr: &grpc.LocalCIDRConfig{Ipv4: "10.1.0.0/24"}}, Vni: 100, VxlanPort: 4789},
			},
			l2vnis:        []grpc.L2VNI{},
			l3Passthrough: []grpc.L3Passthrough{},
			wantUnderlay: hostnetwork.UnderlayParams{
				UnderlayInterface: "eth0",
				TargetNS:          "namespace",
				EVPN: &hostnetwork.UnderlayEVPNParams{
					VtepIP: "10.0.0.0/32",
				},
			},
			wantL3VNIParams: []hostnetwork.L3VNIParams{
				{
					VNIParams: hostnetwork.VNIParams{
						VRF:       "red",
						TargetNS:  "namespace",
						VTEPIP:    "10.0.0.0/32",
						VNI:       100,
						VXLanPort: 4789,
					},
					HostVeth: &hostnetwork.Veth{
						HostIPv4: "10.1.0.2/24",
						NSIPv4:   "10.1.0.1/24",
					},
				},
			},
			wantL2VNIParams: []hostnetwork.L2VNIParams{},
			wantPassthrough: nil,
			wantErr:         false,
		},
		{
			name:      "ipv6 only",
			nodeIndex: 0,
			targetNS:  "namespace",
			underlays: []grpc.Underlay{
				{Nics: []string{"eth0"}, Evpn: &grpc.EVPNConfig{VtepCidr: "10.0.0.0/24"}},
			},
			vnis: []grpc.L3VNI{
				{Vrf: "red", HostSession: &grpc.HostSession{LocalCidr: &grpc.LocalCIDRConfig{Ipv6: "2001:db8::/64"}}, Vni: 100, VxlanPort: 4789},
			},
			l2vnis:        []grpc.L2VNI{},
			l3Passthrough: []grpc.L3Passthrough{},
			wantUnderlay: hostnetwork.UnderlayParams{
				UnderlayInterface: "eth0",
				TargetNS:          "namespace",
				EVPN: &hostnetwork.UnderlayEVPNParams{
					VtepIP: "10.0.0.0/32",
				},
			},
			wantL3VNIParams: []hostnetwork.L3VNIParams{
				{
					VNIParams: hostnetwork.VNIParams{
						VRF:       "red",
						TargetNS:  "namespace",
						VTEPIP:    "10.0.0.0/32",
						VNI:       100,
						VXLanPort: 4789,
					},
					HostVeth: &hostnetwork.Veth{
						HostIPv6: "2001:db8::2/64",
						NSIPv6:   "2001:db8::1/64",
					},
				},
			},
			wantL2VNIParams: []hostnetwork.L2VNIParams{},
			wantPassthrough: nil,
			wantErr:         false,
		},
		{
			name:      "dual stack",
			nodeIndex: 0,
			targetNS:  "namespace",
			underlays: []grpc.Underlay{
				{Nics: []string{"eth0"}, Evpn: &grpc.EVPNConfig{VtepCidr: "10.0.0.0/24"}},
			},
			vnis: []grpc.L3VNI{
				{Vrf: "red", HostSession: &grpc.HostSession{LocalCidr: &grpc.LocalCIDRConfig{Ipv4: "10.1.0.0/24", Ipv6: "2001:db8::/64"}}, Vni: 100, VxlanPort: 4789},
			},
			l2vnis:        []grpc.L2VNI{},
			l3Passthrough: []grpc.L3Passthrough{},
			wantUnderlay: hostnetwork.UnderlayParams{
				UnderlayInterface: "eth0",
				TargetNS:          "namespace",
				EVPN: &hostnetwork.UnderlayEVPNParams{
					VtepIP: "10.0.0.0/32",
				},
			},
			wantL3VNIParams: []hostnetwork.L3VNIParams{
				{
					VNIParams: hostnetwork.VNIParams{
						VRF:       "red",
						TargetNS:  "namespace",
						VTEPIP:    "10.0.0.0/32",
						VNI:       100,
						VXLanPort: 4789,
					},
					HostVeth: &hostnetwork.Veth{
						HostIPv4: "10.1.0.2/24",
						NSIPv4:   "10.1.0.1/24",
						HostIPv6: "2001:db8::2/64",
						NSIPv6:   "2001:db8::1/64",
					},
				},
			},
			wantL2VNIParams: []hostnetwork.L2VNIParams{},
			wantPassthrough: nil,
			wantErr:         false,
		},
		{
			name:      "l2 vni input",
			nodeIndex: 0,
			targetNS:  "namespace",
			underlays: []grpc.Underlay{
				{Nics: []string{"eth0"}, Evpn: &grpc.EVPNConfig{VtepCidr: "10.0.0.0/24"}},
			},
			vnis: []grpc.L3VNI{},
			l2vnis: []grpc.L2VNI{
				{Vni: 200, VxlanPort: 4789},
			},
			l3Passthrough: []grpc.L3Passthrough{},
			wantUnderlay: hostnetwork.UnderlayParams{
				UnderlayInterface: "eth0",
				TargetNS:          "namespace",
				EVPN: &hostnetwork.UnderlayEVPNParams{
					VtepIP: "10.0.0.0/32",
				},
			},
			wantL3VNIParams: []hostnetwork.L3VNIParams{},
			wantL2VNIParams: []hostnetwork.L2VNIParams{
				{
					VNIParams: hostnetwork.VNIParams{
						VRF:       "l2vni-200",
						TargetNS:  "namespace",
						VTEPIP:    "10.0.0.0/32",
						VNI:       200,
						VXLanPort: 4789,
					},
					L2GatewayIP: nil,
					HostMaster:  nil,
				},
			},
			wantPassthrough: nil,
			wantErr:         false,
		},
		{
			name:      "l2 vni with hostmaster and l2gatewayip",
			nodeIndex: 0,
			targetNS:  "namespace",
			underlays: []grpc.Underlay{
				{Nics: []string{"eth0"}, Evpn: &grpc.EVPNConfig{VtepCidr: "10.0.0.0/24"}},
			},
			vnis: []grpc.L3VNI{},
			l2vnis: []grpc.L2VNI{
				{Vni: 201, VxlanPort: 4789, HostMaster: &grpc.HostMaster{Name: "br0"}, L2GatewayIp: "192.168.100.1/24"},
			},
			l3Passthrough: []grpc.L3Passthrough{},
			wantUnderlay: hostnetwork.UnderlayParams{
				UnderlayInterface: "eth0",
				TargetNS:          "namespace",
				EVPN: &hostnetwork.UnderlayEVPNParams{
					VtepIP: "10.0.0.0/32",
				},
			},
			wantL3VNIParams: []hostnetwork.L3VNIParams{},
			wantL2VNIParams: []hostnetwork.L2VNIParams{
				{
					VNIParams: hostnetwork.VNIParams{
						VRF:       "l2vni-201",
						TargetNS:  "namespace",
						VTEPIP:    "10.0.0.0/32",
						VNI:       201,
						VXLanPort: 4789,
					},
					L2GatewayIP: func() *string { s := "192.168.100.1/24"; return &s }(),
					HostMaster:  &hostnetwork.HostMaster{Name: "br0"},
				},
			},
			wantPassthrough: nil,
			wantErr:         false,
		},
		{
			name:      "l3 vni without hostsession",
			nodeIndex: 0,
			targetNS:  "namespace",
			underlays: []grpc.Underlay{
				{Nics: []string{"eth0"}, Evpn: &grpc.EVPNConfig{VtepCidr: "10.0.0.0/24"}},
			},
			vnis: []grpc.L3VNI{
				{Vrf: "red", Vni: 100, VxlanPort: 4789},
			},
			l2vnis:        []grpc.L2VNI{},
			l3Passthrough: []grpc.L3Passthrough{},
			wantUnderlay: hostnetwork.UnderlayParams{
				UnderlayInterface: "eth0",
				TargetNS:          "namespace",
				EVPN: &hostnetwork.UnderlayEVPNParams{
					VtepIP: "10.0.0.0/32",
				},
			},
			wantL3VNIParams: []hostnetwork.L3VNIParams{
				{
					VNIParams: hostnetwork.VNIParams{
						VRF:       "red",
						TargetNS:  "namespace",
						VTEPIP:    "10.0.0.0/32",
						VNI:       100,
						VXLanPort: 4789,
					},
					HostVeth: nil,
				},
			},
			wantL2VNIParams: []hostnetwork.L2VNIParams{},
			wantPassthrough: nil,
			wantErr:         false,
		},
		{
			name:      "underlay without evpn",
			nodeIndex: 0,
			targetNS:  "namespace",
			underlays: []grpc.Underlay{
				{Nics: []string{"eth0"}},
			},
			vnis:          []grpc.L3VNI{},
			l2vnis:        []grpc.L2VNI{},
			l3Passthrough: []grpc.L3Passthrough{},
			wantUnderlay: hostnetwork.UnderlayParams{
				UnderlayInterface: "eth0",
				TargetNS:          "namespace",
			},
			wantL3VNIParams: []hostnetwork.L3VNIParams{},
			wantL2VNIParams: []hostnetwork.L2VNIParams{},
			wantPassthrough: nil,
			wantErr:         false,
		},
		{
			name:      "L3 passthrough dual stack",
			nodeIndex: 0,
			targetNS:  "namespace",
			underlays: []grpc.Underlay{
				{Nics: []string{"eth0"}},
			},
			vnis:   []grpc.L3VNI{},
			l2vnis: []grpc.L2VNI{},
			l3Passthrough: []grpc.L3Passthrough{
				{
					HostSession: &grpc.HostSession{
						Asn: 65000,
						LocalCidr: &grpc.LocalCIDRConfig{
							Ipv4: "192.168.2.0/24",
							Ipv6: "2001:db8::/64",
						},
					},
				},
			},
			wantUnderlay: hostnetwork.UnderlayParams{
				UnderlayInterface: "eth0",
				TargetNS:          "namespace",
			},
			wantL3VNIParams: []hostnetwork.L3VNIParams{},
			wantL2VNIParams: []hostnetwork.L2VNIParams{},
			wantPassthrough: &hostnetwork.PassthroughParams{
				TargetNS: "namespace",
				HostVeth: hostnetwork.Veth{
					HostIPv4: "192.168.2.2/24",
					NSIPv4:   "192.168.2.1/24",
					HostIPv6: "2001:db8::2/64",
					NSIPv6:   "2001:db8::1/64",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiConfig := ApiConfigData{
				NodeIndex:     tt.nodeIndex,
				Underlays:     tt.underlays,
				L3VNIs:        tt.vnis,
				L2VNIs:        tt.l2vnis,
				L3Passthrough: tt.l3Passthrough,
			}

			gotHostConfig, err := APItoHostConfig(tt.nodeIndex, tt.targetNS, apiConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("APItoHostConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotHostConfig.Underlay, tt.wantUnderlay) {
				t.Errorf("APItoHostConfig() gotUnderlay = %v, want %v", gotHostConfig.Underlay, tt.wantUnderlay)
			}
			if !reflect.DeepEqual(gotHostConfig.L3VNIs, tt.wantL3VNIParams) {
				t.Errorf("APItoHostConfig() gotL3VNIParams = %v, want %v", gotHostConfig.L3VNIs, tt.wantL3VNIParams)
			}
			if !reflect.DeepEqual(gotHostConfig.L2VNIs, tt.wantL2VNIParams) {
				t.Errorf("APItoHostConfig() gotL2VNIParams = %v, want %v", gotHostConfig.L2VNIs, tt.wantL2VNIParams)
			}
			if !reflect.DeepEqual(gotHostConfig.L3Passthrough, tt.wantPassthrough) {
				t.Errorf("APItoHostConfig() gotPassthrough = %v, want %v", gotHostConfig.L3Passthrough, tt.wantPassthrough)
			}
		})
	}
}
