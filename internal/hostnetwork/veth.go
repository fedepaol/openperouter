// SPDX-License-Identifier:Apache-2.0

package hostnetwork

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// setupVeth sets up a veth pair with the name generated from the given name and one leg in the
// given namespace.
func setupVeth(ctx context.Context, name string, targetNS netns.NsHandle) (netlink.Link, netlink.Link, error) {
	logger := slog.Default().With("veth", name)
	logger.DebugContext(ctx, "setting up veth")

	hostSide, err := createVeth(ctx, logger, name)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create veth for VRF %s: %w", name, err)
	}

	peerIndex, err := netlink.VethPeerIndex(hostSide)
	if err != nil {
		return nil, nil, fmt.Errorf("could not find peer veth for %s: %w", hostSide.Name, err)
	}
	peSide, err := netlink.LinkByIndex(peerIndex)
	if err == nil { // found, and in the default ns. We need to move it
		if err = netlink.LinkSetNsFd(peSide, int(targetNS)); err != nil {
			return nil, nil, fmt.Errorf("setupUnderlay: Failed to move %s to network namespace %s: %w", peSide.Attrs().Name, targetNS.String(), err)
		}
		slog.DebugContext(ctx, "pe leg moved to ns", "pe veth", peSide.Attrs().Name)
		return hostSide, peSide, nil
	}
	if err != nil && !errors.As(err, &netlink.LinkNotFoundError{}) { // real error
		return nil, nil, fmt.Errorf("could not find peer by index for %s: %w", hostSide.Name, err)
	}

	if errors.As(err, &netlink.LinkNotFoundError{}) { // Let's try to look into the namespace
		if err := inNamespace(targetNS, func() error {
			peSide, err = netlink.LinkByIndex(peerIndex) // we need to reassign here, because we want the one in the ns
			if err != nil {
				return err
			}
			return nil
		}); err != nil {
			return nil, nil, fmt.Errorf("could not find peer veth by index for %s: %w", name, err)
		}
		slog.DebugContext(ctx, "pe leg already in ns", "pe veth", peSide.Attrs().Name)
	}

	slog.DebugContext(ctx, "veth is set up", "vrf", name)
	return hostSide, peSide, nil
}

func createVeth(ctx context.Context, logger *slog.Logger, vrfName string) (*netlink.Veth, error) {
	hostSide, peSide := vethNamesFromVRF(vrfName)
	vethHost := &netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: hostSide}, PeerName: peSide}

	link, err := netlink.LinkByName(hostSide)
	if errors.As(err, &netlink.LinkNotFoundError{}) {
		logger.DebugContext(ctx, "veth does not exist, creating")
		if err := netlink.LinkAdd(vethHost); err != nil {
			return nil, fmt.Errorf("failed to add veth for vrf %s/%s: %w", hostSide, peSide, err)
		}
		logger.DebugContext(ctx, "veth created")
		return vethHost, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get link by name for vrf %s/%s: %w", hostSide, peSide, err)
	}

	vethHost, ok := link.(*netlink.Veth)
	if ok {
		return vethHost, nil
	}
	logger.DebugContext(ctx, "veth exists, but not a veth, deleting and creating")
	if err := netlink.LinkDel(link); err != nil {
		return nil, fmt.Errorf("failed to delete link %v: %w", link, err)
	}
	if err := netlink.LinkAdd(vethHost); err != nil {
		return nil, fmt.Errorf("failed to add veth for vrf %s/%s: %w", hostSide, peSide, err)
	}

	slog.DebugContext(ctx, "veth recreated", "veth", hostSide)
	return vethHost, nil
}

const HostVethPrefix = "host"
const PEVethPrefix = "pe"

// vethNamesFromVRF returns the names of the veth legs
// corresponding to the default namespace and the target namespace.
func vethNamesFromVRF(name string) (string, string) {
	hostSide := HostVethPrefix + name
	peSide := PEVethPrefix + name
	return hostSide, peSide
}

func vrfFromHostVeth(hostVethName string) string {
	return strings.TrimPrefix(hostVethName, HostVethPrefix)
}
