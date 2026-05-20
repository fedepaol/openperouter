// SPDX-License-Identifier:Apache-2.0

//go:build runasroot

package staticconfiguration

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/openperouter/openperouter/api/static"
	"github.com/vishvananda/netlink"
)

const (
	testIPv4 = "ipv4"
	testIPv6 = "ipv6"
)

func createDummyInterface(t *testing.T, name string, addrs ...string) {
	t.Helper()

	link := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: name}}
	if err := netlink.LinkAdd(link); err != nil {
		t.Fatalf("failed to create dummy interface %s: %v", name, err)
	}
	t.Cleanup(func() {
		netlink.LinkDel(link)
	})

	if err := netlink.LinkSetUp(link); err != nil {
		t.Fatalf("failed to bring up dummy interface %s: %v", name, err)
	}

	for _, addrStr := range addrs {
		addr, err := netlink.ParseAddr(addrStr)
		if err != nil {
			t.Fatalf("failed to parse address %s: %v", addrStr, err)
		}
		if err := netlink.AddrAdd(link, addr); err != nil {
			t.Fatalf("failed to add address %s to %s: %v", addrStr, name, err)
		}
	}
}

func TestDeriveNodeIndex_IPv4_Slash24(t *testing.T) {
	createDummyInterface(t, "testidx0", "192.168.1.42/24")

	config := &static.NodeConfig{
		NodeIndexFromInterface: &static.NodeIndexFromInterface{
			Interface: "testidx0",
			Len:       24,
			Family:    testIPv4,
		},
	}
	idx, err := ResolveNodeIndex(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 42 {
		t.Errorf("expected node index 42, got %d", idx)
	}
}

func TestDeriveNodeIndex_IPv4_Slash16(t *testing.T) {
	createDummyInterface(t, "testidx1", "10.1.2.42/16")

	config := &static.NodeConfig{
		NodeIndexFromInterface: &static.NodeIndexFromInterface{
			Interface: "testidx1",
			Len:       16,
			Family:    testIPv4,
		},
	}
	idx, err := ResolveNodeIndex(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 2*256 + 42 // 554
	if idx != expected {
		t.Errorf("expected node index %d, got %d", expected, idx)
	}
}

func TestDeriveNodeIndex_DifferentIPs_SameConfig(t *testing.T) {
	createDummyInterface(t, "testidx2", "192.168.1.10/24")
	createDummyInterface(t, "testidx3", "192.168.1.20/24")

	cfg := func(iface string) *static.NodeConfig {
		return &static.NodeConfig{
			NodeIndexFromInterface: &static.NodeIndexFromInterface{
				Interface: iface,
				Len:       24,
				Family:    testIPv4,
			},
		}
	}

	idx1, err := ResolveNodeIndex(cfg("testidx2"))
	if err != nil {
		t.Fatalf("unexpected error for testidx2: %v", err)
	}
	idx2, err := ResolveNodeIndex(cfg("testidx3"))
	if err != nil {
		t.Fatalf("unexpected error for testidx3: %v", err)
	}

	if idx1 != 10 {
		t.Errorf("expected node index 10 for testidx2, got %d", idx1)
	}
	if idx2 != 20 {
		t.Errorf("expected node index 20 for testidx3, got %d", idx2)
	}
	if idx1 == idx2 {
		t.Errorf("node indexes should differ: both got %d", idx1)
	}
}

func TestDeriveNodeIndex_IPv6(t *testing.T) {
	createDummyInterface(t, "testidx4", "fd00:1:2:3::42/64")

	config := &static.NodeConfig{
		NodeIndexFromInterface: &static.NodeIndexFromInterface{
			Interface: "testidx4",
			Len:       64,
			Family:    testIPv6,
		},
	}
	idx, err := ResolveNodeIndex(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 0x42 {
		t.Errorf("expected node index 0x42 (66), got %d (0x%x)", idx, idx)
	}
}

func TestDeriveNodeIndex_NetworkFilter(t *testing.T) {
	createDummyInterface(t, "testidx5", "192.168.1.42/24", "10.0.0.99/24")

	config := &static.NodeConfig{
		NodeIndexFromInterface: &static.NodeIndexFromInterface{
			Interface: "testidx5",
			Len:       24,
			Family:    testIPv4,
			Network:   "10.0.0.0/24",
		},
	}
	idx, err := ResolveNodeIndex(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 99 {
		t.Errorf("expected node index 99 (from 10.0.0.99), got %d", idx)
	}
}

func TestDeriveNodeIndex_NetworkFilter_NoMatch(t *testing.T) {
	createDummyInterface(t, "testidx6", "192.168.1.42/24")

	config := &static.NodeConfig{
		NodeIndexFromInterface: &static.NodeIndexFromInterface{
			Interface: "testidx6",
			Len:       24,
			Family:    testIPv4,
			Network:   "10.0.0.0/24",
		},
	}
	_, err := ResolveNodeIndex(config)
	if err == nil {
		t.Fatal("expected error when network filter matches nothing")
	}
	if !strings.Contains(err.Error(), "10.0.0.0/24") {
		t.Errorf("error should mention the network filter, got: %v", err)
	}
}

func TestDeriveNodeIndex_WrongFamily(t *testing.T) {
	// Dummy interfaces get an auto-assigned link-local IPv6, so use network
	// filter to test "no matching address" with wrong family instead.
	createDummyInterface(t, "testidx7", "192.168.1.42/24")

	config := &static.NodeConfig{
		NodeIndexFromInterface: &static.NodeIndexFromInterface{
			Interface: "testidx7",
			Len:       24,
			Family:    testIPv4,
			Network:   "10.0.0.0/8",
		},
	}
	_, err := ResolveNodeIndex(config)
	if err == nil {
		t.Fatal("expected error when no address matches network filter")
	}
	if !strings.Contains(err.Error(), "10.0.0.0/8") {
		t.Errorf("error should mention the network filter, got: %v", err)
	}
}

func TestDeriveNodeIndex_ZeroIndex(t *testing.T) {
	createDummyInterface(t, "testidx8", "192.168.1.0/24")

	config := &static.NodeConfig{
		NodeIndexFromInterface: &static.NodeIndexFromInterface{
			Interface: "testidx8",
			Len:       24,
			Family:    testIPv4,
		},
	}
	idx, err := ResolveNodeIndex(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 0 {
		t.Errorf("expected node index 0, got %d", idx)
	}
}

func TestHostIndex_IPv4_Various(t *testing.T) {
	tests := []struct {
		ip       string
		len      int
		expected int
	}{
		{"192.168.1.42", 24, 42},
		{"10.1.2.42", 16, 554},
		{"10.1.2.42", 8, 1*65536 + 2*256 + 42},
		{"192.168.1.0", 24, 0},
		{"192.168.1.255", 24, 255},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%d", tt.ip, tt.len), func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			idx := hostIndex(ip, tt.len, testIPv4)
			if idx != tt.expected {
				t.Errorf("hostIndex(%s, %d) = %d, want %d", tt.ip, tt.len, idx, tt.expected)
			}
		})
	}
}
