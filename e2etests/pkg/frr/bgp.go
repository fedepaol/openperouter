// SPDX-License-Identifier:Apache-2.0

package frr

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/openperouter/openperouter/e2etests/pkg/executor"
)

// NeighborInfo returns informations for the given neighbor in the given
// executor.
func NeighborInfo(neighborName string, exec executor.Executor) (*FRRNeighbor, error) {
	res, err := exec.Exec("vtysh", "-c", fmt.Sprintf("show bgp neighbor %s json", neighborName))

	if err != nil {
		return nil, errors.Join(err, fmt.Errorf("Failed to query neighbour %s", neighborName))
	}
	neighbor, err := parseNeighbour(res)
	if err != nil {
		return nil, errors.Join(err, fmt.Errorf("Failed to parse neighbour %s", neighborName))
	}
	return neighbor, nil
}

const EstablishedState = "Established"

type FRRNeighbor struct {
	BGPNeighborAddr              string      `json:"bgpNeighborAddr"`
	RemoteAs                     int         `json:"remoteAs"`
	LocalAs                      int         `json:"localAs"`
	RemoteRouterID               string      `json:"remoteRouterId"`
	BgpVersion                   int         `json:"bgpVersion"`
	BgpState                     string      `json:"bgpState"`
	PortForeign                  int         `json:"portForeign"`
	PeerBFDInfo                  PeerBFDInfo `json:"peerBfdInfo"`
	VRFName                      string      `json:"vrf"`
	ConfiguredHoldTimeMSecs      int         `json:"bgpTimerConfiguredHoldTimeMsecs"`
	ConfiguredKeepAliveTimeMSecs int         `json:"bgpTimerConfiguredKeepAliveIntervalMsecs"`
	ConnectRetryTimer            int         `json:"connectRetryTimer"`
	AddressFamilyInfo            map[string]struct {
		SentPrefixCounter int `json:"sentPrefixCounter"`
	} `json:"addressFamilyInfo"`
	ConnectionsDropped int  `json:"connectionsDropped"`
	NoSuchNeighbor     bool `json:"bgpNoSuchNeighbor"`
}

type PeerBFDInfo struct {
	Type             string `json:"type"`
	DetectMultiplier int    `json:"detectMultiplier"`
	RxMinInterval    int    `json:"rxMinInterval"`
	TxMinInterval    int    `json:"txMinInterval"`
	Status           string `json:"status"`
	LastUpdate       string `json:"lastUpdate"`
}

type IPInfo struct {
	Routes map[string][]FRRRoute `json:"routes"`
}

type FRRRoute struct {
	Stale     bool   `json:"stale"`
	Valid     bool   `json:"valid"`
	PeerID    string `json:"peerId"`
	LocalPref uint32 `json:"locPrf"`
	Origin    string `json:"origin"`
	PathFrom  string `json:"pathFrom"`
	Nexthops  []struct {
		IP    string `json:"ip"`
		Scope string `json:"scope"`
	} `json:"nexthops"`
}

type BFDPeer struct {
	Multihop                  bool   `json:"multihop"`
	Peer                      string `json:"peer"`
	Local                     string `json:"local"`
	Vrf                       string `json:"vrf"`
	Interface                 string `json:"interface"`
	ID                        int    `json:"id"`
	RemoteID                  int64  `json:"remote-id"`
	PassiveMode               bool   `json:"passive-mode"`
	Status                    string `json:"status"`
	Uptime                    int    `json:"uptime"`
	Diagnostic                string `json:"diagnostic"`
	RemoteDiagnostic          string `json:"remote-diagnostic"`
	ReceiveInterval           int    `json:"receive-interval"`
	TransmitInterval          int    `json:"transmit-interval"`
	EchoReceiveInterval       int    `json:"echo-receive-interval"`
	EchoTransmitInterval      int    `json:"echo-transmit-interval"`
	DetectMultiplier          int    `json:"detect-multiplier"`
	RemoteReceiveInterval     int    `json:"remote-receive-interval"`
	RemoteTransmitInterval    int    `json:"remote-transmit-interval"`
	RemoteEchoInterval        int    `json:"remote-echo-interval"`
	RemoteEchoReceiveInterval int    `json:"remote-echo-receive-interval"`
	RemoteDetectMultiplier    int    `json:"remote-detect-multiplier"`
}

// parseNeighbour takes the result of a show bgp neighbor x.y.w.z
// and parses the informations related to the neighbour.
func parseNeighbour(vtyshRes string) (*FRRNeighbor, error) {
	res := map[string]FRRNeighbor{}
	err := json.Unmarshal([]byte(vtyshRes), &res)
	if err != nil {
		return nil, errors.Join(err, fmt.Errorf("failed to parse vtysh response: %s", vtyshRes))
	}
	if len(res) > 1 {
		return nil, errors.New("more than one peer were returned")
	}
	if len(res) == 0 {
		return nil, errors.New("no peers were returned")
	}
	for _, n := range res {
		return &n, nil
	}
	return nil, nil
}
