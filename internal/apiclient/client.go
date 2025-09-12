// SPDX-License-Identifier:Apache-2.0

package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/openperouter/openperouter/api/grpc"
)

// NonRecoverableError represents an error that cannot be recovered from
type NonRecoverableError struct {
	message string
	errors  []string
}

func (e *NonRecoverableError) Error() string {
	if len(e.errors) > 0 {
		return fmt.Sprintf("%s: %s", e.message, strings.Join(e.errors, "; "))
	}
	return e.message
}

// IsNonRecoverable checks if an error is a non-recoverable error
func IsNonRecoverable(err error) bool {
	var nonRecErr *NonRecoverableError
	return errors.As(err, &nonRecErr)
}

// parseUpdateResponse converts an UpdateResponse to an appropriate error
func parseUpdateResponse(resp *pb.UpdateResponse, operation string) error {
	if resp == nil {
		return fmt.Errorf("%s failed: received nil response", operation)
	}

	switch resp.Status {
	case pb.UpdateResponse_SUCCESS:
		return nil
	case pb.UpdateResponse_FAILURE:
		if len(resp.Errors) > 0 {
			return fmt.Errorf("%s failed: %s", operation, strings.Join(resp.Errors, "; "))
		}
		return fmt.Errorf("%s failed with no error details", operation)
	case pb.UpdateResponse_NON_RECOVERABLE_FAILURE:
		return &NonRecoverableError{
			message: fmt.Sprintf("%s failed with non-recoverable error", operation),
			errors:  resp.Errors,
		}
	default:
		return fmt.Errorf("%s failed with unknown status: %d", operation, resp.Status)
	}
}

// Client wraps the gRPC client and provides a convenient interface
type Client struct {
	conn   *grpc.ClientConn
	client pb.OpenPERouterServiceClient
}

// New creates a new API client that connects to the Unix socket
func New(socketPath string) (*Client, error) {
	return NewWithTimeout(socketPath, 10*time.Second)
}

// NewWithTimeout creates a new API client with a custom connection timeout
func NewWithTimeout(socketPath string, timeout time.Duration) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", socketPath, timeout)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server at %s: %w", socketPath, err)
	}

	client := pb.NewOpenPERouterServiceClient(conn)

	return &Client{
		conn:   conn,
		client: client,
	}, nil
}

// Close closes the gRPC connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// UpdateAll sends a complete configuration update request
func (c *Client) UpdateAll(ctx context.Context, req *pb.UpdateAllRequest) error {
	resp, err := c.client.UpdateAll(ctx, req)
	if err != nil {
		return fmt.Errorf("UpdateAll RPC failed: %w", err)
	}
	return parseUpdateResponse(resp, "UpdateAll")
}

// UpdateNodeIndex updates the node index
func (c *Client) UpdateNodeIndex(ctx context.Context, nodeIndex uint32) error {
	req := &pb.UpdateNodeIndexRequest{
		NodeIndex: nodeIndex,
	}
	resp, err := c.client.UpdateNodeIndex(ctx, req)
	if err != nil {
		return fmt.Errorf("UpdateNodeIndex RPC failed: %w", err)
	}
	return parseUpdateResponse(resp, "UpdateNodeIndex")
}

// UpdateTargetNamespace updates the target namespace
func (c *Client) UpdateTargetNamespace(ctx context.Context, targetNamespace string) error {
	req := &pb.UpdateTargetNamespaceRequest{
		TargetNamespace: targetNamespace,
	}
	resp, err := c.client.UpdateTargetNamespace(ctx, req)
	if err != nil {
		return fmt.Errorf("UpdateTargetNamespace RPC failed: %w", err)
	}
	return parseUpdateResponse(resp, "UpdateTargetNamespace")
}

// UpdateReloaderIP updates the reloader IP address
func (c *Client) UpdateReloaderIP(ctx context.Context, reloaderIP string) error {
	req := &pb.UpdateReloaderIPRequest{
		ReloaderIp: reloaderIP,
	}
	resp, err := c.client.UpdateReloaderIP(ctx, req)
	if err != nil {
		return fmt.Errorf("UpdateReloaderIP RPC failed: %w", err)
	}
	return parseUpdateResponse(resp, "UpdateReloaderIP")
}
