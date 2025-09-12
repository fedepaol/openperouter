// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	pb "github.com/openperouter/openperouter/api/grpc"
	periov1alpha1 "github.com/openperouter/openperouter/api/v1alpha1"
)

func TestConvertCRDsToUpdateAllRequest(t *testing.T) {
	tests := []struct {
		name        string
		collections crs
		want        *pb.UpdateAllRequest
		wantErr     bool
	}{
		{
			name: "complete configuration",
			collections: crs{
				Underlays: []periov1alpha1.Underlay{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "test-underlay"},
						Spec: periov1alpha1.UnderlaySpec{
							ASN:          65000,
							RouterIDCIDR: "10.0.0.0/24",
							Nics:         []string{"eth0"},
							Neighbors: []periov1alpha1.Neighbor{
								{
									ASN:     65001,
									Address: "192.168.1.1",
								},
							},
							EVPN: &periov1alpha1.EVPNConfig{
								VTEPCIDR: "192.168.100.0/24",
							},
						},
					},
				},
				L2VNIs: []periov1alpha1.L2VNI{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "test-l2vni"},
						Spec: periov1alpha1.L2VNISpec{
							VNI:       100,
							VXLanPort: 4789,
						},
					},
				},
				L3VNIs: []periov1alpha1.L3VNI{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "test-l3vni"},
						Spec: periov1alpha1.L3VNISpec{
							VNI:       200,
							VXLanPort: 4789,
						},
					},
				},
				L3Passthroughs: []periov1alpha1.L3Passthrough{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "test-l3passthrough"},
						Spec: periov1alpha1.L3PassthroughSpec{
							HostSession: periov1alpha1.HostSession{
								ASN:     65000,
								HostASN: 65100,
								LocalCIDR: periov1alpha1.LocalCIDRConfig{
									IPv4: "192.168.10.0/30",
								},
							},
						},
					},
				},
			},
			want: &pb.UpdateAllRequest{
				Underlays: []*pb.Underlay{
					{
						Asn:          65000,
						RouterIdCidr: "10.0.0.0/24",
						Nics:         []string{"eth0"},
						Neighbors: []*pb.Neighbor{
							{
								Asn:     65001,
								Address: "192.168.1.1",
							},
						},
						Evpn: &pb.EVPNConfig{
							VtepCidr: "192.168.100.0/24",
						},
					},
				},
				L2Vnis: []*pb.L2VNI{
					{
						Vrf:       "test-l2vni",
						Vni:       100,
						VxlanPort: 4789,
					},
				},
				L3Vnis: []*pb.L3VNI{
					{
						Vrf:       "test-l3vni",
						Vni:       200,
						VxlanPort: 4789,
					},
				},
				L3Passthroughs: []*pb.L3Passthrough{
					{
						HostSession: &pb.HostSession{
							Asn:     65000,
							HostAsn: 65100,
							LocalCidr: &pb.LocalCIDRConfig{
								Ipv4: "192.168.10.0/30",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:        "empty collections",
			collections: crs{},
			want: &pb.UpdateAllRequest{
				Underlays:      []*pb.Underlay{},
				L2Vnis:         []*pb.L2VNI{},
				L3Vnis:         []*pb.L3VNI{},
				L3Passthroughs: []*pb.L3Passthrough{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := crdsToUpdateAllRequest(tt.collections)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertCRDsToUpdateAllRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("ConvertCRDsToUpdateAllRequest() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConvertUnderlay(t *testing.T) {
	tests := []struct {
		name    string
		input   *periov1alpha1.Underlay
		want    *pb.Underlay
		wantErr bool
	}{
		{
			name: "complete underlay",
			input: &periov1alpha1.Underlay{
				ObjectMeta: metav1.ObjectMeta{Name: "test-underlay"},
				Spec: periov1alpha1.UnderlaySpec{
					ASN:          65000,
					RouterIDCIDR: "10.0.0.0/24",
					Nics:         []string{"eth0", "eth1"},
					Neighbors: []periov1alpha1.Neighbor{
						{
							ASN:     65001,
							Address: "192.168.1.1",
							Port:    ptr.To(uint16(179)),
						},
					},
					EVPN: &periov1alpha1.EVPNConfig{
						VTEPCIDR: "192.168.100.0/24",
					},
				},
			},
			want: &pb.Underlay{
				Asn:          65000,
				RouterIdCidr: "10.0.0.0/24",
				Nics:         []string{"eth0", "eth1"},
				Neighbors: []*pb.Neighbor{
					{
						Asn:     65001,
						Address: "192.168.1.1",
						Port:    179,
					},
				},
				Evpn: &pb.EVPNConfig{
					VtepCidr: "192.168.100.0/24",
				},
			},
			wantErr: false,
		},
		{
			name: "underlay without EVPN",
			input: &periov1alpha1.Underlay{
				ObjectMeta: metav1.ObjectMeta{Name: "test-underlay"},
				Spec: periov1alpha1.UnderlaySpec{
					ASN:          65000,
					RouterIDCIDR: "10.0.0.0/24",
					Nics:         []string{"eth0"},
					Neighbors:    []periov1alpha1.Neighbor{},
				},
			},
			want: &pb.Underlay{
				Asn:          65000,
				RouterIdCidr: "10.0.0.0/24",
				Nics:         []string{"eth0"},
				Neighbors:    []*pb.Neighbor{},
			},
			wantErr: false,
		},
		{
			name:    "nil underlay",
			input:   nil,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := underlayToAPI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertUnderlay() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("convertUnderlay() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConvertNeighbor(t *testing.T) {
	tests := []struct {
		name    string
		input   *periov1alpha1.Neighbor
		want    *pb.Neighbor
		wantErr bool
	}{
		{
			name: "complete neighbor with all fields",
			input: &periov1alpha1.Neighbor{
				ASN:            65001,
				HostASN:        ptr.To(uint32(65002)),
				Address:        "192.168.1.1",
				Port:           ptr.To(uint16(179)),
				Password:       "secret",
				PasswordSecret: "bgp-secret",
				HoldTime:       &metav1.Duration{Duration: 180 * time.Second},
				KeepaliveTime:  &metav1.Duration{Duration: 60 * time.Second},
				ConnectTime:    &metav1.Duration{Duration: 10 * time.Second},
				EBGPMultiHop:   true,
				BFD: &periov1alpha1.BFDSettings{
					ReceiveInterval:  ptr.To(uint32(300)),
					TransmitInterval: ptr.To(uint32(300)),
					DetectMultiplier: ptr.To(uint32(3)),
				},
			},
			want: &pb.Neighbor{
				Asn:            65001,
				HostAsn:        65002,
				Address:        "192.168.1.1",
				Port:           179,
				Password:       "secret",
				PasswordSecret: "bgp-secret",
				HoldTime:       180000000000, // 180 seconds in nanoseconds
				KeepaliveTime:  60000000000,  // 60 seconds in nanoseconds
				ConnectTime:    10000000000,  // 10 seconds in nanoseconds
				EbgpMultiHop:   true,
				Bfd: &pb.BFDSettings{
					ReceiveInterval:  300,
					TransmitInterval: 300,
					DetectMultiplier: 3,
				},
			},
			wantErr: false,
		},
		{
			name: "minimal neighbor",
			input: &periov1alpha1.Neighbor{
				ASN:     65001,
				Address: "192.168.1.1",
			},
			want: &pb.Neighbor{
				Asn:     65001,
				Address: "192.168.1.1",
			},
			wantErr: false,
		},
		{
			name:    "nil neighbor",
			input:   nil,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := neighborToAPI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertNeighbor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("convertNeighbor() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConvertL2VNI(t *testing.T) {
	tests := []struct {
		name    string
		input   *periov1alpha1.L2VNI
		want    *pb.L2VNI
		wantErr bool
	}{
		{
			name: "complete L2VNI",
			input: &periov1alpha1.L2VNI{
				ObjectMeta: metav1.ObjectMeta{Name: "test-l2vni"},
				Spec: periov1alpha1.L2VNISpec{
					VRF:       ptr.To("custom-vrf"),
					VNI:       100,
					VXLanPort: 4789,
					HostMaster: &periov1alpha1.HostMaster{
						Name:       "br-test",
						Type:       "bridge",
						AutoCreate: true,
					},
					L2GatewayIP: "192.168.100.1/24",
				},
			},
			want: &pb.L2VNI{
				Vrf:       "custom-vrf",
				Vni:       100,
				VxlanPort: 4789,
				HostMaster: &pb.HostMaster{
					Name:       "br-test",
					Type:       "bridge",
					AutoCreate: true,
				},
				L2GatewayIp: "192.168.100.1/24",
			},
			wantErr: false,
		},
		{
			name: "L2VNI without VRF (uses name)",
			input: &periov1alpha1.L2VNI{
				ObjectMeta: metav1.ObjectMeta{Name: "test-l2vni"},
				Spec: periov1alpha1.L2VNISpec{
					VNI:       200,
					VXLanPort: 4789,
				},
			},
			want: &pb.L2VNI{
				Vrf:       "test-l2vni",
				Vni:       200,
				VxlanPort: 4789,
			},
			wantErr: false,
		},
		{
			name:    "nil L2VNI",
			input:   nil,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := l2vniToAPI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertL2VNI() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("convertL2VNI() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConvertL3VNI(t *testing.T) {
	tests := []struct {
		name    string
		input   *periov1alpha1.L3VNI
		want    *pb.L3VNI
		wantErr bool
	}{
		{
			name: "complete L3VNI with HostSession",
			input: &periov1alpha1.L3VNI{
				ObjectMeta: metav1.ObjectMeta{Name: "test-l3vni"},
				Spec: periov1alpha1.L3VNISpec{
					VRF:       ptr.To("custom-vrf"),
					VNI:       300,
					VXLanPort: 4789,
					HostSession: &periov1alpha1.HostSession{
						ASN:     65000,
						HostASN: 65100,
						LocalCIDR: periov1alpha1.LocalCIDRConfig{
							IPv4: "192.168.10.0/30",
							IPv6: "fd00::0/64",
						},
					},
				},
			},
			want: &pb.L3VNI{
				Vrf:       "custom-vrf",
				Vni:       300,
				VxlanPort: 4789,
				HostSession: &pb.HostSession{
					Asn:     65000,
					HostAsn: 65100,
					LocalCidr: &pb.LocalCIDRConfig{
						Ipv4: "192.168.10.0/30",
						Ipv6: "fd00::0/64",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "L3VNI without HostSession",
			input: &periov1alpha1.L3VNI{
				ObjectMeta: metav1.ObjectMeta{Name: "test-l3vni"},
				Spec: periov1alpha1.L3VNISpec{
					VNI:       400,
					VXLanPort: 4789,
				},
			},
			want: &pb.L3VNI{
				Vrf:       "test-l3vni",
				Vni:       400,
				VxlanPort: 4789,
			},
			wantErr: false,
		},
		{
			name:    "nil L3VNI",
			input:   nil,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := l3vniToAPI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertL3VNI() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("convertL3VNI() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConvertL3Passthrough(t *testing.T) {
	tests := []struct {
		name    string
		input   *periov1alpha1.L3Passthrough
		want    *pb.L3Passthrough
		wantErr bool
	}{
		{
			name: "complete L3Passthrough",
			input: &periov1alpha1.L3Passthrough{
				ObjectMeta: metav1.ObjectMeta{Name: "test-l3passthrough"},
				Spec: periov1alpha1.L3PassthroughSpec{
					HostSession: periov1alpha1.HostSession{
						ASN:     65000,
						HostASN: 65100,
						LocalCIDR: periov1alpha1.LocalCIDRConfig{
							IPv4: "192.168.10.0/30",
						},
					},
				},
			},
			want: &pb.L3Passthrough{
				HostSession: &pb.HostSession{
					Asn:     65000,
					HostAsn: 65100,
					LocalCidr: &pb.LocalCIDRConfig{
						Ipv4: "192.168.10.0/30",
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "nil L3Passthrough",
			input:   nil,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := l3PassthroughToAPI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertL3Passthrough() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("convertL3Passthrough() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
