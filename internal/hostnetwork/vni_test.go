package hostnetwork

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const testNSName = "testns"

// TODO sleeps

var _ = Describe("VNI configuration", func() {
	cleanTest(testNSName)
	var testNS netns.NsHandle

	BeforeEach(func() {
		testNS = createTestNS(testNSName)
		setupLoopback(testNS)
	})
	AfterEach(func() {
		cleanTest(testNSName)
	})

	It("should work with a single VNI", func() {
		params := VNIParams{
			VRF:        "testred",
			TargetNS:   testNSName,
			VTEPIP:     "192.170.0.9/32",
			VethHostIP: "192.168.9.1/32",
			VethNSIP:   "192.168.9.0/32",
			VNI:        100,
			VXLanPort:  4789,
		}

		err := SetupVNI(context.Background(), params)
		Expect(err).NotTo(HaveOccurred())

		validateHostLeg(params)

		_ = inNamespace(testNS, func() error {
			validateNS(params)
			return nil
		})
	})

	It("should work with multiple vnis + cleanup", func() {
		params := []VNIParams{
			{

				VRF:        "testred",
				TargetNS:   testNSName,
				VTEPIP:     "192.170.0.9/32",
				VethHostIP: "192.168.9.1/32",
				VethNSIP:   "192.168.9.0/32",
				VNI:        100,
				VXLanPort:  4789,
			},
			{
				VRF:        "testblue",
				TargetNS:   testNSName,
				VTEPIP:     "192.170.0.10/32",
				VethHostIP: "192.168.9.2/32",
				VethNSIP:   "192.168.9.3/32",
				VNI:        101,
				VXLanPort:  4789,
			},
		}
		for _, p := range params {
			err := SetupVNI(context.Background(), p)
			Expect(err).NotTo(HaveOccurred())
			validateHostLeg(p)
			_ = inNamespace(testNS, func() error {
				validateNS(p)
				return nil
			})
		}

		remaining := params[0]
		toDelete := params[1]
		err := RemoveNonConfiguredVNIs(testNS, []VNIParams{remaining})
		Expect(err).NotTo(HaveOccurred())

		validateHostLeg(remaining)
		_ = inNamespace(testNS, func() error {
			validateNS(remaining)
			return nil
		})

		hostSide, _ := vethNamesFromVRF(toDelete.VRF)
		checkLinkdeleted(hostSide)
		validateVNIIsNotConfigured(toDelete)
	})

	It("should be idempotent", func() {
		params := VNIParams{
			VRF:        "testred",
			TargetNS:   testNSName,
			VTEPIP:     "192.170.0.9/32",
			VethHostIP: "192.168.9.1/32",
			VethNSIP:   "192.168.9.0/32",
			VNI:        100,
			VXLanPort:  4789,
		}

		err := SetupVNI(context.Background(), params)
		Expect(err).NotTo(HaveOccurred())

		err = SetupVNI(context.Background(), params)
		Expect(err).NotTo(HaveOccurred())

		validateHostLeg(params)

		_ = inNamespace(testNS, func() error {
			validateNS(params)
			return nil
		})

	})
})

func validateHostLeg(params VNIParams) error {
	hostSide, _ := vethNamesFromVRF(params.VRF)
	hostLegLink, err := netlink.LinkByName(hostSide)
	Expect(err).NotTo(HaveOccurred())

	if hostLegLink.Attrs().OperState != netlink.OperUp {
		return fmt.Errorf("host leg %s is not up: %s", hostSide, hostLegLink.Attrs().OperState)
	}
	hasIP, err := interfaceHasIP(hostLegLink, params.VethHostIP)
	Expect(err).NotTo(HaveOccurred())
	if !hasIP {
		addresses, _ := netlink.AddrList(hostLegLink, netlink.FAMILY_ALL)
		return fmt.Errorf("host leg doesn't have ip %s %v", params.VethHostIP, addresses)
	}
	return nil
}

func validateNS(params VNIParams) error {
	loopback, err := netlink.LinkByName(UnderlayLoopback)
	if err != nil {
		return fmt.Errorf("failed to get loopback by name: %v", err)
	}

	vxlanLink, err := netlink.LinkByName(vxLanName(params.VNI))
	if err != nil {
		return fmt.Errorf("failed to get vxlan by name: %v", err)
	}
	vxlan := vxlanLink.(*netlink.Vxlan)
	if vxlan.OperState != netlink.OperUnknown { // todo should we even validate this?
		return fmt.Errorf("vxlan is not unknown: %s", vxlan.OperState)
	}
	addrGenModeNone := checkAddrGenModeNone(vxlan)
	if !addrGenModeNone {
		return fmt.Errorf("failed to check addrGenMode, expecting true")
	}

	vrfLink, err := netlink.LinkByName(params.VRF)
	if err != nil {
		return fmt.Errorf("failed to get vrf by name: %v", err)
	}
	vrf := vrfLink.(*netlink.Vrf)
	if vrf.OperState != netlink.OperUp {
		return fmt.Errorf("vrf is not up: %s", vrf.OperState)
	}

	bridgeLink, err := netlink.LinkByName(bridgeName(params.VNI))
	if err != nil {
		return fmt.Errorf("failed to get vxlan by name: %v", err)
	}
	bridge := bridgeLink.(*netlink.Bridge)
	if bridge.OperState != netlink.OperUp {
		return fmt.Errorf("bridge is not up: %s", bridge.OperState)
	}
	if bridge.MasterIndex != vrf.Index {
		return fmt.Errorf("bridge master is not vrf")
	}

	addrGenModeNone = checkAddrGenModeNone(bridge)
	if !addrGenModeNone {
		return fmt.Errorf("failed to check addrGenMode , expecting true")
	}

	err = checkVXLanConfigured(vxlan, bridge.Index, loopback.Attrs().Index, params)
	if err != nil {
		return fmt.Errorf("invalid vxlan %v", err)
	}

	_, peSide := vethNamesFromVRF(params.VRF)
	peLegLink, err := netlink.LinkByName(peSide)
	if err != nil {
		return fmt.Errorf("failed to get peLegLink by name: %v", err)
	}
	if peLegLink.Attrs().OperState != netlink.OperUp {
		return fmt.Errorf("peLegLink is not up: %s", peLegLink.Attrs().OperState)
	}
	if peLegLink.Attrs().MasterIndex != vrf.Index {
		return fmt.Errorf("peLegLink master is not vrf")
	}

	hasIP, err := interfaceHasIP(peLegLink, params.VethNSIP)
	if err != nil {
		return fmt.Errorf("failed to undersand if pe leg has ip: %v", err)
	}
	if !hasIP {
		return fmt.Errorf("pe leg doesn't have ip %s", params.VethNSIP)
	}

	route, err := hostIPToRoute(vrf, params.VethHostIP, peLegLink)
	if err != nil {
		return fmt.Errorf("failed to convert host ip to route: %v", err)
	}
	isPresent, err := checkRouteIsPresent(route)
	if err != nil {
		return fmt.Errorf("failed to check if route is present: %v", err)
	}
	if !isPresent {
		return fmt.Errorf("route is not added")
	}
	return nil
}

func checkLinkdeleted(name string) {
	_, err := netlink.LinkByName(name)
	Expect(errors.As(err, &netlink.LinkNotFoundError{})).To(BeTrue(), "link not deleted", name, err)
}

func validateVNIIsNotConfigured(params VNIParams) {

	checkLinkdeleted(vxLanName(params.VNI))
	checkLinkdeleted(params.VRF)
	checkLinkdeleted(bridgeName(params.VNI))

	_, peSide := vethNamesFromVRF(params.VRF)
	checkLinkdeleted(peSide)
}

func checkAddrGenModeNone(l netlink.Link) bool {
	fileName := fmt.Sprintf("/proc/sys/net/ipv6/conf/%s/addr_gen_mode", l.Attrs().Name)
	addrGenMode, err := os.ReadFile(fileName)
	Expect(err).NotTo(HaveOccurred())

	if strings.Trim(string(addrGenMode), "\n") == "1" {
		return true
	}
	return false
}

func setupLoopback(ns netns.NsHandle) {
	_ = inNamespace(ns, func() error {
		_, err := netlink.LinkByName(UnderlayLoopback)
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			loopback := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: UnderlayLoopback}}
			err = netlink.LinkAdd(loopback)
			Expect(err).NotTo(HaveOccurred(), "failed to create loopback", UnderlayLoopback)
		}
		return nil
	})
}

func TestIPToRoute(t *testing.T) {
	vrf := &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{
			Index: 12,
		},
		Table: 37,
	}
	peInterface := netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Index: 12,
		},
	}

	tests := []struct {
		name        string
		dst         string
		expectedDst string
	}{
		{
			name:        "/24 cidr",
			dst:         "192.168.10.3/24",
			expectedDst: "192.168.10.3/32",
		},
		{
			name:        "/28 cidr",
			dst:         "192.168.10.3/28",
			expectedDst: "192.168.10.3/32",
		},
	}
	for _, tc := range tests {
		route, err := hostIPToRoute(vrf, tc.dst, &peInterface)
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}
		_, desiredDest, err := net.ParseCIDR(tc.expectedDst)
		if err != nil {
			t.Fatalf("failed to parse expected dst")
		}
		if desiredDest.String() != route.Dst.String() {
			t.Fatalf("expecting %s got %s", desiredDest, route.Dst)
		}
		if route.Table != int(vrf.Table) {
			t.Fatalf("expecting vrf table %d, got %d", vrf.Table, route.Table)
		}
		if route.LinkIndex != peInterface.Index {
			t.Fatalf("expecting pe interface index %d, got %d", peInterface.Index, route.LinkIndex)
		}
	}
}
