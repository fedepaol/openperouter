// SPDX-License-Identifier:Apache-2.0

package conversion

import (
	"github.com/openperouter/openperouter/api/grpc"
	"github.com/openperouter/openperouter/internal/hostnetwork"
)

type ApiConfigData struct {
	NodeIndex     int
	Underlays     []grpc.Underlay
	L3VNIs        []grpc.L3VNI
	L2VNIs        []grpc.L2VNI
	L3Passthrough []grpc.L3Passthrough
	LogLevel      string
}

type HostConfigData struct {
	Underlay      hostnetwork.UnderlayParams
	L3VNIs        []hostnetwork.L3VNIParams
	L2VNIs        []hostnetwork.L2VNIParams
	L3Passthrough *hostnetwork.PassthroughParams
}
