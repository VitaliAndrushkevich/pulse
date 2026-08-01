package proto

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// reflectionServiceNames are the well-known reflection/infrastructure service names
// that are filtered out by default when discovering application services.
// Users can opt in to include them via the includeSystem parameter.
var reflectionServiceNames = map[string]bool{
	"grpc.reflection.v1.ServerReflection":      true,
	"grpc.reflection.v1alpha.ServerReflection": true,
	"grpc.health.v1.Health":                    true,
}

// defaultReflectTimeout is the maximum time allowed for a reflection operation
// when the caller's context has no deadline set.
const defaultReflectTimeout = 10 * time.Second

// ReflectServices connects to a gRPC server and discovers schemas via Server Reflection.
// Uses the provided TLS config and respects the context deadline.
// Optional metadata is sent as gRPC headers on the reflection stream.
// When includeSystem is true, system services (e.g. grpc.health.v1.Health) are included
// in the results instead of being filtered out.
// Returns a complete FileDescriptorSet with all transitive dependencies.
func ReflectServices(ctx context.Context, target string, tlsCfg *tls.Config, md map[string]string, includeSystem bool) (*descriptorpb.FileDescriptorSet, error) {
	// Enforce a 10-second timeout if the context doesn't already have a deadline.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultReflectTimeout)
		defer cancel()
	}

	// Build transport credentials.
	var creds grpc.DialOption
	if tlsCfg == nil {
		creds = grpc.WithTransportCredentials(insecure.NewCredentials())
	} else {
		creds = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
	}

	// Connect to the gRPC server.
	conn, err := grpc.NewClient(target, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client: %w", err)
	}
	defer conn.Close()

	// Create the reflection client and open a bidi stream.
	// Attach metadata as gRPC headers if provided.
	if len(md) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, metadata.New(md))
	}
	client := reflectpb.NewServerReflectionClient(conn)
	stream, err := client.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, classifyReflectionError(err)
	}

	// Step 1: List all services.
	if err := stream.Send(&reflectpb.ServerReflectionRequest{
		MessageRequest: &reflectpb.ServerReflectionRequest_ListServices{
			ListServices: "",
		},
	}); err != nil {
		return nil, classifyReflectionError(err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, classifyReflectionError(err)
	}

	// Check for error response (server doesn't support reflection).
	if errResp := resp.GetErrorResponse(); errResp != nil {
		return nil, fmt.Errorf("server does not support reflection: %s", errResp.GetErrorMessage())
	}

	listResp := resp.GetListServicesResponse()
	if listResp == nil {
		return nil, fmt.Errorf("server does not support reflection: unexpected response type")
	}

	// Collect all service names from the server.
	var allServiceNames []string
	for _, svc := range listResp.GetService() {
		allServiceNames = append(allServiceNames, svc.GetName())
	}

	// Filter out well-known system services (reflection, health) unless includeSystem is set.
	var serviceNames []string
	for _, name := range allServiceNames {
		if includeSystem {
			// When includeSystem is true, only filter the reflection service itself.
			if strings.Contains(name, "ServerReflection") {
				continue
			}
			serviceNames = append(serviceNames, name)
		} else {
			if !reflectionServiceNames[name] {
				serviceNames = append(serviceNames, name)
			}
		}
	}

	// Fallback: if filtering removed everything, include system services
	// (except reflection itself) so the user can still select health checks.
	if len(serviceNames) == 0 {
		for _, name := range allServiceNames {
			// Always exclude the reflection service itself — it's infrastructure.
			if strings.Contains(name, "ServerReflection") {
				continue
			}
			serviceNames = append(serviceNames, name)
		}
	}

	if len(serviceNames) == 0 {
		return nil, fmt.Errorf("no discoverable services found: server exposes only reflection services")
	}

	// Step 2: For each service, fetch the file descriptor containing it.
	seen := make(map[string]bool)
	var allFiles []*descriptorpb.FileDescriptorProto

	for _, svcName := range serviceNames {
		if err := stream.Send(&reflectpb.ServerReflectionRequest{
			MessageRequest: &reflectpb.ServerReflectionRequest_FileContainingSymbol{
				FileContainingSymbol: svcName,
			},
		}); err != nil {
			return nil, classifyReflectionError(err)
		}

		resp, err := stream.Recv()
		if err != nil {
			return nil, classifyReflectionError(err)
		}

		if errResp := resp.GetErrorResponse(); errResp != nil {
			return nil, fmt.Errorf("failed to get descriptor for service %q: %s", svcName, errResp.GetErrorMessage())
		}

		fdResp := resp.GetFileDescriptorResponse()
		if fdResp == nil {
			return nil, fmt.Errorf("unexpected response type for service %q", svcName)
		}

		// Parse each file descriptor proto from the response.
		// The response includes the requested file and its transitive dependencies.
		for _, rawFD := range fdResp.GetFileDescriptorProto() {
			fdp := &descriptorpb.FileDescriptorProto{}
			if err := proto.Unmarshal(rawFD, fdp); err != nil {
				return nil, fmt.Errorf("failed to unmarshal file descriptor: %w", err)
			}

			name := fdp.GetName()
			if !seen[name] {
				seen[name] = true
				allFiles = append(allFiles, fdp)
			}
		}
	}

	// Close the send side of the stream.
	if err := stream.CloseSend(); err != nil {
		// Non-fatal: we already have the data we need.
		_ = err
	}
	// Drain any remaining responses.
	for {
		if _, err := stream.Recv(); err != nil {
			break
		}
	}

	if len(allFiles) == 0 {
		return nil, fmt.Errorf("no file descriptors retrieved from server")
	}

	return &descriptorpb.FileDescriptorSet{
		File: allFiles,
	}, nil
}

// classifyReflectionError maps gRPC errors to user-friendly reflection error messages.
func classifyReflectionError(err error) error {
	if err == nil {
		return nil
	}

	// Check for context deadline exceeded.
	if ctx := context.DeadlineExceeded; err == ctx {
		return fmt.Errorf("reflection timeout: operation exceeded deadline")
	}

	errStr := err.Error()

	// Check for EOF which typically means reflection is not supported.
	if err == io.EOF {
		return fmt.Errorf("server does not support reflection: connection closed unexpectedly")
	}

	// Check for connection-level failures.
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "dns") ||
		strings.Contains(errStr, "unreachable") ||
		strings.Contains(errStr, "connection reset") {
		return fmt.Errorf("reflection connection failed: %w", err)
	}

	// Check for deadline exceeded in wrapped errors.
	if strings.Contains(errStr, "DeadlineExceeded") || strings.Contains(errStr, "context deadline exceeded") {
		return fmt.Errorf("reflection timeout: %w", err)
	}

	// Check for Unimplemented status which means reflection is not enabled.
	if strings.Contains(errStr, "Unimplemented") || strings.Contains(errStr, "unimplemented") {
		return fmt.Errorf("server does not support reflection: %w", err)
	}

	return fmt.Errorf("reflection error: %w", err)
}
