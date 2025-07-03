// SPDX-License-Identifier:Apache-2.0

package ipam

import (
	"net"
	"testing"
)

func TestSliceCIDR(t *testing.T) {
	tests := []struct {
		name        string
		cidr        string
		index       int
		expectedIP1 string
		expectedIP2 string
		shouldFail  bool
	}{
		{
			"first",
			"192.168.1.0/24",
			0,
			"192.168.1.0/24",
			"192.168.1.1/24",
			false,
		},
		{
			"second",
			"192.168.1.0/24",
			1,
			"192.168.1.2/24",
			"192.168.1.3/24",
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, cidr, err := net.ParseCIDR(tc.cidr)
			if err != nil {
				t.Fatalf("failed to parse cidr %s", tc.cidr)
			}
			ips, err := sliceCIDR(cidr, tc.index, 2)
			if err != nil && !tc.shouldFail {
				t.Fatalf("got error %s", err)
			}
			if err == nil && tc.shouldFail {
				t.Fatalf("expected error, did not happen")
			}
			if len(ips) != 2 {
				t.Fatalf("expecting 2 ips, got %v", ips)
			}
			ip1, ip2 := ips[0], ips[1]
			if ip1.String() != tc.expectedIP1 {
				t.Fatalf("expecting %s got %s", tc.expectedIP1, ip1.String())
			}
			if ip2.String() != tc.expectedIP2 {
				t.Fatalf("expecting %s got %s", tc.expectedIP2, ip2.String())
			}

		})
	}
}

func TestVethIPs(t *testing.T) {
	tests := []struct {
		name         string
		pool         string
		index        int
		expectedPE   string
		expectedHost string
		shouldFail   bool
	}{
		{
			"first",
			"192.168.1.0/24",
			0,
			"192.168.1.0/24",
			"192.168.1.1/24",
			false,
		}, {
			"second",
			"192.168.1.0/24",
			1,
			"192.168.1.0/24",
			"192.168.1.2/24",
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := VethIPs(tc.pool, tc.index)
			if err != nil && !tc.shouldFail {
				t.Fatalf("got error %v while should not fail", err)
			}
			if err == nil && tc.shouldFail {
				t.Fatalf("was expecting error, didn't fail")
			}

			if res.HostSide.String() != tc.expectedHost {
				t.Fatalf("was expecting %s, got %s on the host", tc.expectedHost, res.HostSide.String())
			}
			if res.PeSide.String() != tc.expectedPE {
				t.Fatalf("was expecting %s, got %s on the container", tc.expectedPE, res.PeSide.String())
			}
		})
	}

}

func TestVTEPIP(t *testing.T) {
	tests := []struct {
		name           string
		pool           string
		index          int
		expectedVTEPIP string
		shouldFail     bool
	}{
		{
			"first",
			"192.168.1.0/24",
			0,
			"192.168.1.0/32",
			false,
		}, {
			"second",
			"192.168.1.0/24",
			1,
			"192.168.1.1/32",
			false,
		}, {
			"invalid",
			"hellothisisnotanip",
			0,
			"",
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := VTEPIp(tc.pool, tc.index)
			if err != nil && !tc.shouldFail {
				t.Fatalf("got error %v while should not fail", err)
			}
			if err == nil && tc.shouldFail {
				t.Fatalf("was expecting error, didn't fail")
			}

			if !tc.shouldFail && res.String() != tc.expectedVTEPIP {
				t.Fatalf("was expecting %s, got %s on the VTEPIP", tc.expectedVTEPIP, res.String())
			}
		})
	}

}
