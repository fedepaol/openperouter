package apiserver

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/openperouter/openperouter/api/grpc"
	pb "github.com/openperouter/openperouter/api/grpc"
	"github.com/openperouter/openperouter/internal/conversion"
)

type ApiServer struct {
	sync.Mutex
	NodeIndex       int
	TargetNamespace string
	ReloaderPort    uint32
	ReloaderIP      string
}

func New() *ApiServer {
	return &ApiServer{}
}

func (s *ApiServer) UpdateReloaderIP(ctx context.Context, reloaderIP string) error {

	s.Lock()
	defer s.Unlock()

	slog.InfoContext(ctx, "updating reloader ip", "reloader ip", reloaderIP)
	s.ReloaderIP = reloaderIP
	return nil
}

func (s *ApiServer) updateNodeIndex(ctx context.Context, nodeIndex uint32) error {
	if nodeIndex == 0 {
		slog.ErrorContext(ctx, "node index can't be 0")
		return fmt.Errorf("node index can't be 0")
	}

	s.Lock()
	defer s.Unlock()

	slog.InfoContext(ctx, "updating node index", "nodeIndex", nodeIndex)
	s.NodeIndex = int(nodeIndex)
	return nil
}

func (s *ApiServer) updateTargetNamespace(ctx context.Context, namespace string) error {
	if namespace == "" {
		slog.ErrorContext(ctx, "empty target namespace")
		return fmt.Errorf("empty namespace")
	}

	s.Lock()
	defer s.Unlock()

	slog.InfoContext(ctx, "updating target namespace", "namespace", namespace)
	s.TargetNamespace = namespace
	return nil
}

func (s *ApiServer) updateAll(ctx context.Context,
	l2vnis []*grpc.L2VNI,
	l3vnis []*grpc.L3VNI,
	l3passthroughs []*grpc.L3Passthrough,
	underlays []*grpc.Underlay) error {

	slog.InfoContext(ctx, "received UpdateAll request start")
	defer slog.InfoContext(ctx, "received UpdateAll request end")

	slog.DebugContext(ctx, "received UpdateAll request",
		"l2vnis", len(l2vnis),
		"l3vnis", len(l3vnis),
		"l3passthroughs", len(l3passthroughs),
		"underlays", len(underlays))

	apiConfig := &conversion.ApiConfigData{
		NodeIndex:     s.NodeIndex,
		Underlays:     underlays,
		L2VNIs:        l2vnis,
		L3VNIs:        l3vnis,
		L3Passthrough: l3passthroughs,
		LegLevel:      s.LogLevel,
	}

	err := configureInterfaces(ctx, interfacesConfiguration{
		RouterPodUUID: string(routerPod.UID),
		PodRuntime:    *r.PodRuntime,
		ApiConfigData: apiConfig,
	})

	// TODO: Implement actual configuration logic for:
	// - L2VNIs: l2vnis
	// - L3VNIs: l3vnis
	// - L3Passthroughs: l3passthroughs
	// - Underlays: underlays

	slog.InfoContext(ctx, "UpdateAll processing completed")
	return nil
}

func (s *ApiServer) UpdateNodeIndex(ctx context.Context, req *pb.UpdateNodeIndexRequest) (*pb.UpdateNodeIndexResponse, error) {
	err := s.updateNodeIndex(ctx, req.NodeIndex)
	if err != nil {
		return &pb.UpdateNodeIndexResponse{
			Status: pb.UpdateNodeIndexResponse_FAILURE,
			Error:  &[]string{err.Error()}[0],
		}, nil
	}

	return &pb.UpdateNodeIndexResponse{
		Status: pb.UpdateNodeIndexResponse_SUCCESS,
	}, nil
}

func (s *ApiServer) UpdateTargetNamespace(ctx context.Context, req *pb.UpdateTargetNamespaceRequest) (*pb.UpdateTargetNamespaceResponse, error) {
	err := s.updateTargetNamespace(ctx, req.TargetNamespace)
	if err != nil {
		return &pb.UpdateTargetNamespaceResponse{
			Status: pb.UpdateTargetNamespaceResponse_FAILURE,
			Error:  &[]string{err.Error()}[0],
		}, nil
	}

	return &pb.UpdateTargetNamespaceResponse{
		Status: pb.UpdateTargetNamespaceResponse_SUCCESS,
	}, nil
}
