// SPDX-License-Identifier:Apache-2.0

package hostnetwork

import (
	"errors"
	"fmt"
	"math"

	"github.com/vishvananda/netlink"
)

// setupVRF creates a new VRF and sets it up.
func setupVRF(name string) (*netlink.Vrf, error) {
	link, err := netlink.LinkByName(name)
	// does not exist. Let's create.
	if err != nil && errors.As(err, &netlink.LinkNotFoundError{}) {
		link, err = createVRF(name)
		if err != nil {
			return nil, fmt.Errorf("failed to create vrf %s: %w", name, err)
		}
	}
	vrf, ok := link.(*netlink.Vrf)
	if !ok {
		// exists but not of the right type, let's remove and recreate.
		err := netlink.LinkDel(link)
		if err != nil {
			return nil, fmt.Errorf("failed to delete link %v: %w", link, err)
		}
		vrf, err = createVRF(name)
		if err != nil {
			return nil, fmt.Errorf("failed to create vrf %s: %w", name, err)
		}
	}

	err = netlink.LinkSetUp(vrf)
	if err != nil {
		return nil, fmt.Errorf("could not set link up for VRF %s: %v", name, err)
	}

	return vrf, nil
}

func createVRF(name string) (*netlink.Vrf, error) {
	tableID, err := findFreeRoutingTableID()
	if err != nil {
		return nil, err
	}

	vrf := &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{Name: name},
		Table:     tableID,
	}

	link, err := netlink.LinkByName(name)

	// does not exist. Let's create.
	if errors.As(err, &netlink.LinkNotFoundError{}) {
		if err := netlink.LinkAdd(vrf); err != nil {
			return nil, fmt.Errorf("could not add VRF %s: %v", name, err)
		}
		return vrf, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could get link by name %s: %v", name, err)
	}

	// exists
	vrf, ok := link.(*netlink.Vrf)
	if ok {
		return vrf, nil
	}

	// exists but not of the right type, let's remove and recreate.
	err = netlink.LinkDel(link)
	if err != nil {
		return nil, fmt.Errorf("failed to delete link %v: %w", link, err)
	}
	if err := netlink.LinkAdd(vrf); err != nil {
		return nil, fmt.Errorf("could not add VRF %s: %v", name, err)
	}
	return vrf, nil
}

func findFreeRoutingTableID() (uint32, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return 0, fmt.Errorf("createVRF: Failed to find links %v", err)
	}

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
