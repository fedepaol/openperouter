package hostnetwork

import (
	"fmt"
	"math"

	"github.com/vishvananda/netlink"
)

// createVRF creates a new VRF and sets it up.
func createVRF(name string) (*netlink.Vrf, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("createVRF: Failed to find links %v", err)
	}

	tableID, err := findFreeRoutingTableID(links)
	if err != nil {
		return nil, err
	}

	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = name
	vrf := &netlink.Vrf{
		LinkAttrs: linkAttrs,
		Table:     tableID,
	}

	err = netlink.LinkAdd(vrf)
	if err != nil {
		return nil, fmt.Errorf("could not add VRF %s: %v", name, err)
	}
	err = netlink.LinkSetUp(vrf)
	if err != nil {
		return nil, fmt.Errorf("could not set link up for VRF %s: %v", name, err)
	}

	return vrf, nil
}

func findFreeRoutingTableID(links []netlink.Link) (uint32, error) {
	takenTables := make(map[uint32]struct{}, len(links))
	for _, l := range links {
		if vrf, ok := l.(*netlink.Vrf); ok {
			takenTables[vrf.Table] = struct{}{}
		}
	}

	for res := uint32(1); res < math.MaxUint32; res++ {
		if _, ok := takenTables[res]; !ok {
			return res, nil
		}
	}
	return 0, fmt.Errorf("findFreeRoutingTableID: Failed to find an available routing id")
}