// SPDX-License-Identifier:Apache-2.0

package conversion

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"

	"github.com/openperouter/openperouter/api/grpc"
	"github.com/openperouter/openperouter/internal/frr"
	"github.com/openperouter/openperouter/internal/ipfamily"
)

func TestAPItoFRR(t *testing.T) {
	tests := []struct {
		name          string
		nodeIndex     int
		underlays     []grpc.Underlay
		vnis          []grpc.L3VNI
		l3Passthrough []grpc.L3Passthrough
		logLevel      string
		want          frr.Config
		wantErr       bool
	}{
		{
			name:          "no underlays",
			nodeIndex:     0,
			underlays:     []grpc.Underlay{},
			vnis:          []grpc.L3VNI{{}},
			l3Passthrough: []grpc.L3Passthrough{},
			wantErr:       true,
		},
		{
			name:      "no vnis",
			nodeIndex: 0,
			underlays: []grpc.Underlay{
				{
					Asn: 65000,
					Evpn: &grpc.EVPNConfig{
						VtepCidr: "192.168.1.0/24",
					},
					RouterIdCidr: "10.0.0.0/24",
					Neighbors:    []*grpc.Neighbor{{Address: "192.168.1.1", Asn: 65001}},
				},
			},
			vnis:          []grpc.L3VNI{},
			l3Passthrough: []grpc.L3Passthrough{},
			logLevel:      "debug",
			want: frr.Config{
				Underlay: frr.UnderlayConfig{
					MyASN: 65000,
					EVPN: &frr.UnderlayEvpn{
						VTEP: "192.168.1.0/32",
					},
					RouterID: "10.0.0.1",
					Neighbors: []frr.NeighborConfig{
						{
							Name:         "65001@192.168.1.1",
							ASN:          65001,
							Addr:         "192.168.1.1",
							IPFamily:     ipfamily.IPv4,
							EBGPMultiHop: false,
						},
					},
				},
				VNIs:        []frr.L3VNIConfig{},
				BFDProfiles: []frr.BFDProfile{},
				Loglevel:    "debug",
			},
			wantErr: false,
		},
		{
			name:      "ipv4 only",
			nodeIndex: 0,
			underlays: []grpc.Underlay{
				{
					Asn: 65000,
					Evpn: &grpc.EVPNConfig{
						VtepCidr: "192.168.1.0/24",
					},
					RouterIdCidr: "10.0.0.0/24",
					Neighbors:    []*grpc.Neighbor{{Address: "192.168.1.1", Asn: 65001}},
				},
			},
			vnis: []grpc.L3VNI{
				{
					HostSession: &grpc.HostSession{
						Asn: 65000,
						LocalCidr: &grpc.LocalCIDRConfig{
							Ipv4: "192.168.2.0/24",
						},
						HostAsn: 65001,
					},
					Vrf: "vrf1",
					Vni: 200,
				},
			},
			l3Passthrough: []grpc.L3Passthrough{},
			logLevel:      "debug",
			want: frr.Config{
				Underlay: frr.UnderlayConfig{
					MyASN: 65000,
					EVPN: &frr.UnderlayEvpn{
						VTEP: "192.168.1.0/32",
					},
					RouterID: "10.0.0.1",
					Neighbors: []frr.NeighborConfig{
						{
							Name:         "65001@192.168.1.1",
							ASN:          65001,
							Addr:         "192.168.1.1",
							IPFamily:     ipfamily.IPv4,
							EBGPMultiHop: false,
						},
					},
				},
				VNIs: []frr.L3VNIConfig{
					{
						ASN:      65000,
						VNI:      200,
						VRF:      "vrf1",
						RouterID: "10.0.0.1",
						LocalNeighbor: &frr.NeighborConfig{
							Addr: "192.168.2.2",
							ASN:  65001,
						},
						ToAdvertiseIPv4: []string{"192.168.2.2/32"},
						ToAdvertiseIPv6: []string{},
					},
				},
				BFDProfiles: []frr.BFDProfile{},
				Loglevel:    "debug",
			},
			wantErr: false,
		},
		{
			name:      "ipv6 only",
			nodeIndex: 0,
			underlays: []grpc.Underlay{
				{
					Asn: 65000,
					Evpn: &grpc.EVPNConfig{
						VtepCidr: "192.168.1.0/24",
					},
					RouterIdCidr: "10.0.0.0/24",
					Neighbors:    []*grpc.Neighbor{{Address: "192.168.1.1", Asn: 65001}},
				},
			},
			vnis: []grpc.L3VNI{
				{
					HostSession: &grpc.HostSession{
						Asn: 65000,
						LocalCidr: &grpc.LocalCIDRConfig{
							Ipv6: "2001:db8::/64",
						},
						HostAsn: 65001,
					},
					Vrf: "vrf1",
					Vni: 200,
				},
			},
			l3Passthrough: []grpc.L3Passthrough{},
			logLevel:      "debug",
			want: frr.Config{
				Underlay: frr.UnderlayConfig{
					MyASN: 65000,
					EVPN: &frr.UnderlayEvpn{
						VTEP: "192.168.1.0/32",
					},
					RouterID: "10.0.0.1",
					Neighbors: []frr.NeighborConfig{
						{
							Name:         "65001@192.168.1.1",
							ASN:          65001,
							Addr:         "192.168.1.1",
							IPFamily:     ipfamily.IPv4,
							EBGPMultiHop: false,
						},
					},
				},
				VNIs: []frr.L3VNIConfig{
					{
						ASN:      65000,
						VNI:      200,
						VRF:      "vrf1",
						RouterID: "10.0.0.1",
						LocalNeighbor: &frr.NeighborConfig{
							Addr: "2001:db8::2",
							ASN:  65001,
						},
						ToAdvertiseIPv4: []string{},
						ToAdvertiseIPv6: []string{"2001:db8::2/128"},
					},
				},
				BFDProfiles: []frr.BFDProfile{},
				Loglevel:    "debug",
			},
			wantErr: false,
		},
		{
			name:      "dual stack",
			nodeIndex: 0,
			underlays: []grpc.Underlay{
				{
					Asn: 65000,
					Evpn: &grpc.EVPNConfig{
						VtepCidr: "192.168.1.0/24",
					},
					RouterIdCidr: "10.0.0.0/24",
					Neighbors:    []*grpc.Neighbor{{Address: "192.168.1.1", Asn: 65001}},
				},
			},
			vnis: []grpc.L3VNI{
				{
					HostSession: &grpc.HostSession{
						Asn: 65000,
						LocalCidr: &grpc.LocalCIDRConfig{
							Ipv4: "192.168.2.0/24",
							Ipv6: "2001:db8::/64",
						},
						HostAsn: 65001,
					},
					Vrf: "vrf1",
					Vni: 200,
				},
			},
			l3Passthrough: []grpc.L3Passthrough{},
			logLevel:      "debug",
			want: frr.Config{
				Underlay: frr.UnderlayConfig{
					MyASN: 65000,
					EVPN: &frr.UnderlayEvpn{
						VTEP: "192.168.1.0/32",
					},
					RouterID: "10.0.0.1",
					Neighbors: []frr.NeighborConfig{
						{
							Name:         "65001@192.168.1.1",
							ASN:          65001,
							Addr:         "192.168.1.1",
							IPFamily:     ipfamily.IPv4,
							EBGPMultiHop: false,
						},
					},
				},
				VNIs: []frr.L3VNIConfig{
					{
						ASN:      65000,
						VNI:      200,
						VRF:      "vrf1",
						RouterID: "10.0.0.1",
						LocalNeighbor: &frr.NeighborConfig{
							Addr: "192.168.2.2",
							ASN:  65001,
						},
						ToAdvertiseIPv4: []string{"192.168.2.2/32"},
						ToAdvertiseIPv6: []string{},
					},
					{
						ASN:      65000,
						VNI:      200,
						VRF:      "vrf1",
						RouterID: "10.0.0.1",
						LocalNeighbor: &frr.NeighborConfig{
							Addr: "2001:db8::2",
							ASN:  65001,
						},
						ToAdvertiseIPv4: []string{},
						ToAdvertiseIPv6: []string{"2001:db8::2/128"},
					},
				},
				BFDProfiles: []frr.BFDProfile{},
				Loglevel:    "debug",
			},
			wantErr: false,
		},
		{
			name:      "BFD with custom settings",
			nodeIndex: 0,
			underlays: []grpc.Underlay{
				{
					Asn: 65000,
					Evpn: &grpc.EVPNConfig{
						VtepCidr: "192.168.1.0/24",
					},
					RouterIdCidr: "10.0.0.0/24",
					Neighbors: []*grpc.Neighbor{
						{
							Address: "192.168.1.100",
							Asn:     65001,
							Bfd: &grpc.BFDSettings{
								ReceiveInterval:  300,
								TransmitInterval: 300,
								DetectMultiplier: 3,
								EchoMode:         false,
								PassiveMode:      false,
							},
						},
					},
				},
			},
			vnis:          []grpc.L3VNI{},
			l3Passthrough: []grpc.L3Passthrough{},
			logLevel:      "debug",
			want: frr.Config{
				Underlay: frr.UnderlayConfig{
					MyASN: 65000,
					EVPN: &frr.UnderlayEvpn{
						VTEP: "192.168.1.0/32",
					},
					RouterID: "10.0.0.1",
					Neighbors: []frr.NeighborConfig{
						{
							Name:         "65001@192.168.1.100",
							ASN:          65001,
							Addr:         "192.168.1.100",
							IPFamily:     ipfamily.IPv4,
							EBGPMultiHop: false,
							BFDEnabled:   true,
							BFDProfile:   "neighbor-192.168.1.100",
						},
					},
				},
				VNIs: []frr.L3VNIConfig{},
				BFDProfiles: []frr.BFDProfile{
					{
						Name:             "neighbor-192.168.1.100",
						ReceiveInterval:  ptr.To(uint32(300)),
						TransmitInterval: ptr.To(uint32(300)),
						DetectMultiplier: ptr.To(uint32(3)),
					},
				},
				Loglevel: "debug",
			},
			wantErr: false,
		},
		{
			name:      "BFD enabled without settings",
			nodeIndex: 0,
			underlays: []grpc.Underlay{
				{
					Asn: 65000,
					Evpn: &grpc.EVPNConfig{
						VtepCidr: "192.168.1.0/24",
					},
					RouterIdCidr: "10.0.0.0/24",
					Neighbors: []*grpc.Neighbor{
						{
							Address: "192.168.1.100",
							Asn:     65001,
							Bfd:     &grpc.BFDSettings{},
						},
					},
				},
			},
			vnis:          []grpc.L3VNI{},
			l3Passthrough: []grpc.L3Passthrough{},
			logLevel:      "debug",
			want: frr.Config{
				Underlay: frr.UnderlayConfig{
					MyASN: 65000,
					EVPN: &frr.UnderlayEvpn{
						VTEP: "192.168.1.0/32",
					},
					RouterID: "10.0.0.1",
					Neighbors: []frr.NeighborConfig{
						{
							Name:         "65001@192.168.1.100",
							ASN:          65001,
							Addr:         "192.168.1.100",
							IPFamily:     ipfamily.IPv4,
							EBGPMultiHop: false,
							BFDEnabled:   true,
							BFDProfile:   "",
						},
					},
				},
				VNIs:        []frr.L3VNIConfig{},
				BFDProfiles: []frr.BFDProfile{},
				Loglevel:    "debug",
			},
			wantErr: false,
		},
		{
			name:      "vni without host session",
			nodeIndex: 0,
			underlays: []grpc.Underlay{
				{
					Asn: 65000,
					Evpn: &grpc.EVPNConfig{
						VtepCidr: "192.168.1.0/24",
					},
					RouterIdCidr: "10.0.0.0/24",
					Neighbors:    []*grpc.Neighbor{{Address: "192.168.1.1", Asn: 65001}},
				},
			},
			vnis: []grpc.L3VNI{
				{
					Vrf: "vrf1",
					Vni: 200,
				},
			},
			l3Passthrough: []grpc.L3Passthrough{},
			logLevel:      "debug",
			want: frr.Config{
				Underlay: frr.UnderlayConfig{
					MyASN: 65000,
					EVPN: &frr.UnderlayEvpn{
						VTEP: "192.168.1.0/32",
					},
					RouterID: "10.0.0.1",
					Neighbors: []frr.NeighborConfig{
						{
							Name:         "65001@192.168.1.1",
							ASN:          65001,
							Addr:         "192.168.1.1",
							IPFamily:     ipfamily.IPv4,
							EBGPMultiHop: false,
						},
					},
				},
				VNIs: []frr.L3VNIConfig{
					{
						ASN:      65000,
						VNI:      200,
						VRF:      "vrf1",
						RouterID: "10.0.0.1",
					},
				},
				BFDProfiles: []frr.BFDProfile{},
				Loglevel:    "debug",
			},
			wantErr: false,
		},
		{
			name:      "empty routeridcidr uses default",
			nodeIndex: 0,
			underlays: []grpc.Underlay{
				{
					Asn: 65000,
					Evpn: &grpc.EVPNConfig{
						VtepCidr: "192.168.1.0/24",
					},
					RouterIdCidr: "",
					Neighbors:    []*grpc.Neighbor{{Address: "192.168.1.1", Asn: 65001}},
				},
			},
			vnis: []grpc.L3VNI{
				{
					HostSession: &grpc.HostSession{
						Asn: 65000,
						LocalCidr: &grpc.LocalCIDRConfig{
							Ipv4: "192.168.2.0/24",
						},
						HostAsn: 65001,
					},
					Vni: 200,
				},
			},
			l3Passthrough: []grpc.L3Passthrough{},
			logLevel:      "debug",
			want: frr.Config{
				Underlay: frr.UnderlayConfig{
					MyASN: 65000,
					EVPN: &frr.UnderlayEvpn{
						VTEP: "192.168.1.0/32",
					},
					RouterID: "10.0.0.1",
					Neighbors: []frr.NeighborConfig{
						{
							Name:         "65001@192.168.1.1",
							ASN:          65001,
							Addr:         "192.168.1.1",
							IPFamily:     ipfamily.IPv4,
							EBGPMultiHop: false,
						},
					},
				},
				VNIs: []frr.L3VNIConfig{
					{
						ASN:      65000,
						VNI:      200,
						VRF:      "l3vni-200",
						RouterID: "10.0.0.1",
						LocalNeighbor: &frr.NeighborConfig{
							Addr: "192.168.2.2",
							ASN:  65001,
						},
						ToAdvertiseIPv4: []string{"192.168.2.2/32"},
						ToAdvertiseIPv6: []string{},
					},
				},
				BFDProfiles: []frr.BFDProfile{},
				Loglevel:    "debug",
			},
			wantErr: false,
		},
		{
			name:      "missing EVPN parameter",
			nodeIndex: 0,
			underlays: []grpc.Underlay{
				{
					Asn:          65000,
					RouterIdCidr: "10.0.0.0/24",
					Neighbors:    []*grpc.Neighbor{{Address: "192.168.1.1", Asn: 65001}},
				},
			},
			vnis:          []grpc.L3VNI{},
			l3Passthrough: []grpc.L3Passthrough{},
			logLevel:      "debug",
			want: frr.Config{
				Underlay: frr.UnderlayConfig{
					MyASN:    65000,
					RouterID: "10.0.0.1",
					Neighbors: []frr.NeighborConfig{
						{
							Name:         "65001@192.168.1.1",
							ASN:          65001,
							Addr:         "192.168.1.1",
							IPFamily:     ipfamily.IPv4,
							EBGPMultiHop: false,
						},
					},
				},
				VNIs:        []frr.L3VNIConfig{},
				BFDProfiles: []frr.BFDProfile{},
				Loglevel:    "debug",
			},
			wantErr: false,
		},
		{
			name:      "L3 passthrough",
			nodeIndex: 0,
			underlays: []grpc.Underlay{
				{
					Asn: 65000,
					Evpn: &grpc.EVPNConfig{
						VtepCidr: "192.168.1.0/24",
					},
					RouterIdCidr: "10.0.0.0/24",
					Neighbors:    []*grpc.Neighbor{{Address: "192.168.1.1", Asn: 65001}},
				},
			},
			vnis: []grpc.L3VNI{},
			l3Passthrough: []grpc.L3Passthrough{
				{
					HostSession: &grpc.HostSession{
						HostAsn: 65001,
						Asn:     65000,
						LocalCidr: &grpc.LocalCIDRConfig{
							Ipv4: "192.168.2.0/24",
							Ipv6: "2001:db8::/64",
						},
					},
				},
			},
			logLevel: "debug",
			want: frr.Config{
				Underlay: frr.UnderlayConfig{
					MyASN: 65000,
					EVPN: &frr.UnderlayEvpn{
						VTEP: "192.168.1.0/32",
					},
					RouterID: "10.0.0.1",
					Neighbors: []frr.NeighborConfig{
						{
							Name:         "65001@192.168.1.1",
							ASN:          65001,
							Addr:         "192.168.1.1",
							IPFamily:     ipfamily.IPv4,
							EBGPMultiHop: false,
						},
					},
					Passthrough: &frr.PassthroughConfig{
						LocalNeighborV4: &frr.NeighborConfig{
							ASN:  65001,
							Addr: "192.168.2.2",
						},
						LocalNeighborV6: &frr.NeighborConfig{
							ASN:  65001,
							Addr: "2001:db8::2",
						},
						ToAdvertiseIPv4: []string{"192.168.2.2/32"},
						ToAdvertiseIPv6: []string{"2001:db8::2/128"},
					},
				},
				VNIs:        []frr.L3VNIConfig{},
				BFDProfiles: []frr.BFDProfile{},
				Loglevel:    "debug",
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
				L3Passthrough: tt.l3Passthrough,
				LogLevel:      tt.logLevel,
			}
			got, err := APItoFRR(apiConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("APItoFRR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !cmp.Equal(got, tt.want) {
				t.Errorf("APItoFRR() = %v, diff %s", got, cmp.Diff(got, tt.want))
			}
		})
	}
}
