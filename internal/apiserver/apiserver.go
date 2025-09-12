package apiserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/openperouter/openperouter/api/grpc"
	"github.com/openperouter/openperouter/internal/conversion"
)

type ApiServer struct {
	sync.Mutex
	FRRConfigPath   string
	NodeIndex       int
	LogLevel        string
	TargetNamespace string
	ReloaderPort    int
	ReloaderIP      string
}

func (s *ApiServer) UpdateReloaderIP(ctx context.Context, reloaderIP string) error {
	if net.ParseIP(reloaderIP) == nil {
		slog.ErrorContext(ctx, "invalid IP address format", "reloader_ip", reloaderIP)
		return fmt.Errorf("invalid IP address format: %s", reloaderIP)
	}

	s.Lock()
	defer s.Unlock()

	slog.InfoContext(ctx, "updating reloader ip", "reloader ip", reloaderIP)
	s.ReloaderIP = reloaderIP
	return nil
}

func (s *ApiServer) UpdateNodeIndex(ctx context.Context, nodeIndex uint32) error {
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

func (s *ApiServer) UpdateTargetNamespace(ctx context.Context, namespace string) error {
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

	if err := s.validate(); err != nil {
		return err
	}

	slog.DebugContext(ctx, "received UpdateAll request",
		"l2vnis", len(l2vnis),
		"l3vnis", len(l3vnis),
		"l3passthroughs", len(l3passthroughs),
		"underlays", len(underlays))

	apiConfig := conversion.ApiConfigData{
		NodeIndex:     s.NodeIndex,
		Underlays:     underlays,
		L2VNIs:        l2vnis,
		L3VNIs:        l3vnis,
		L3Passthrough: l3passthroughs,
		LogLevel:      s.LogLevel,
	}

	if err := configureInterfaces(ctx, apiConfig, s.TargetNamespace, s.NodeIndex); err != nil {
		return fmt.Errorf("failed to configure interfaces %w", err)
	}

	frrConfig := frrConfigData{
		configFile:    s.FRRConfigPath,
		address:       s.ReloaderIP,
		port:          s.ReloaderPort,
		ApiConfigData: apiConfig,
	}
	if err := configureFRR(ctx, frrConfig); err != nil {
		return fmt.Errorf("failed to configure frr %w", err)
	}
	slog.InfoContext(ctx, "UpdateAll processing completed")
	return nil
}

func (s *ApiServer) validate() error {
	var errors []string

	if s.NodeIndex == 0 {
		errors = append(errors, "node index is required and cannot be 0")
	}

	if s.TargetNamespace == "" {
		errors = append(errors, "target namespace is required")
	}

	if s.ReloaderIP == "" {
		errors = append(errors, "reloader IP is required")
	} else if net.ParseIP(s.ReloaderIP) == nil {
		errors = append(errors, fmt.Sprintf("invalid reloader IP format: %s", s.ReloaderIP))
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed: %v", errors)
	}

	return nil
}
