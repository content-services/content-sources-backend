package rbac

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/project-kessel/kessel-sdk-go/kessel/inventory/v1beta2"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

// MockKesselInventoryServiceServer implements the KesselInventoryService gRPC interface
type MockKesselInventoryServiceServer struct {
	v1beta2.UnimplementedKesselInventoryServiceServer
}

func (s *MockKesselInventoryServiceServer) Check(ctx context.Context, req *v1beta2.CheckRequest) (*v1beta2.CheckResponse, error) {
	readUsers := config.Get().Mocks.Kessel.UserRead
	readWriteUsers := config.Get().Mocks.Kessel.UserReadWrite
	allAllowedUsers := append(readUsers, readWriteUsers...)

	resourceID := req.Subject.GetResource().ResourceId
	userID := strings.Split(resourceID, "/")[1]

	if slices.Contains(allAllowedUsers, userID) {
		return &v1beta2.CheckResponse{
			Allowed: v1beta2.Allowed_ALLOWED_TRUE,
		}, nil
	}

	return &v1beta2.CheckResponse{
		Allowed: v1beta2.Allowed_ALLOWED_FALSE,
	}, nil
}

func (s *MockKesselInventoryServiceServer) CheckForUpdate(ctx context.Context, req *v1beta2.CheckForUpdateRequest) (*v1beta2.CheckForUpdateResponse, error) {
	readWriteUsers := config.Get().Mocks.Kessel.UserReadWrite

	resourceID := req.Subject.GetResource().ResourceId
	userID := strings.Split(resourceID, "/")[1]

	if slices.Contains(readWriteUsers, userID) {
		return &v1beta2.CheckForUpdateResponse{
			Allowed: v1beta2.Allowed_ALLOWED_TRUE,
		}, nil
	}

	return &v1beta2.CheckForUpdateResponse{
		Allowed: v1beta2.Allowed_ALLOWED_FALSE,
	}, nil
}

// MockGrpcServer represents a Mock gRPC server for testing
type MockGrpcServer struct {
	server   *grpc.Server
	listener net.Listener
	address  string
}

// NewMockInventoryServer creates a new Mock gRPC server for testing the kessel inventory APIs
func NewMockInventoryServer() *MockGrpcServer {
	server := grpc.NewServer()

	// Register the Mock Kessel inventory service
	inventoryService := &MockKesselInventoryServiceServer{}
	v1beta2.RegisterKesselInventoryServiceServer(server, inventoryService)

	return &MockGrpcServer{
		server: server,
	}
}

// Start starts the Mock gRPC server on the specified address
func (d *MockGrpcServer) Start(address string) error {
	listener, err := net.Listen("tcp", strings.TrimPrefix(address, "http://"))
	if err != nil {
		return err
	}

	d.listener = listener
	d.address = listener.Addr().String()

	go func() {
		if err := d.server.Serve(listener); err != nil {
			if errors.Is(err, grpc.ErrServerStopped) {
				return
			} else {
				log.Error().Err(err).Msg("grpc server stopped")
			}
		}
	}()

	return nil
}

// Stop stops the Mock gRPC server
func (d *MockGrpcServer) Stop() {
	if d.server != nil {
		d.server.Stop()
	}
	if d.listener != nil {
		d.listener.Close()
	}
}

// GetAddress returns the address the server is listening on
func (d *MockGrpcServer) GetAddress() string {
	return d.address
}
