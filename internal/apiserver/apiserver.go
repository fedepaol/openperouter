package apiserver

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/openperouter/openperouter/api/grpc"
	pb "github.com/openperouter/openperouter/api/grpc"
)

type ApiServer struct {
	pb.UnimplementedOpenPERouterServiceServer
	sync.Mutex
	nodeIndex       uint32
	targetNamespace string
}

func New() *ApiServer {
	return &ApiServer{}
}

func (s *ApiServer) updateNodeIndex(ctx context.Context, nodeIndex uint32) error {
	if nodeIndex == 0 {
		slog.ErrorContext(ctx, "node index can't be 0")
		return fmt.Errorf("node index can't be 0")
	}

	s.Lock()
	defer s.Unlock()

	slog.InfoContext(ctx, "updating node index", "nodeIndex", nodeIndex)
	s.nodeIndex = nodeIndex
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
	s.targetNamespace = namespace
	return nil
}

func (s *ApiServer) UpdateAll(ctx context.Context,
	L2Vnis []*grpc.L2VNI,
	L3Vnis []*grpc.L3VNI,
	L3Passthroughs []*grpc.L3Passthrough,
	Underlays []*grpc.Underlay) error {

	slog.InfoContext(ctx, "received UpdateAll request start")
	defer slog.InfoContext(ctx, "received UpdateAll request end")

	slog.DebugContext(ctx, "received UpdateAll request",
		"l2vnis", len(req.L2Vnis),
		"l3vnis", len(req.L3Vnis),
		"l3passthroughs", len(req.L3Passthroughs),
		"underlays", len(req.Underlays))

	var errors []string

	// TODO: Implement actual configuration logic for:
	// - L2VNIs: req.L2Vnis
	// - L3VNIs: req.L3Vnis
	// - L3Passthroughs: req.L3Passthroughs
	// - Underlays: req.Underlays

	slog.InfoContext(ctx, "UpdateAll processing completed", "errors", len(errors))

	if len(errors) > 0 {
		return &pb.UpdateAllResponse{
			Status: pb.UpdateAllResponse_FAILURE,
			Errors: errors,
		}, nil
	}

	return &pb.UpdateAllResponse{
		Status: pb.UpdateAllResponse_SUCCESS,
		Errors: []string{},
	}, nil
}

// gRPC RPC method implementations

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
