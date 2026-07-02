package proto

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

var registerOnce sync.Once

// =============================================================================
// Test helpers
// =============================================================================

// registerTestServiceInGlobalRegistry creates and registers a file descriptor for a
// test service in protoregistry.GlobalFiles so that grpc-go's reflection server can
// serve it. Safe to call from multiple tests — uses sync.Once.
func registerTestServiceInGlobalRegistry(t *testing.T) {
	t.Helper()

	registerOnce.Do(func() {
		fdp := &descriptorpb.FileDescriptorProto{
			Name:    strPtr("reflecttest/echo.proto"),
			Package: strPtr("reflecttest"),
			Syntax:  strPtr("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{
				{
					Name: strPtr("EchoRequest"),
					Field: []*descriptorpb.FieldDescriptorProto{
						{
							Name:     strPtr("message"),
							Number:   int32Ptr(1),
							Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							JsonName: strPtr("message"),
						},
					},
				},
				{
					Name: strPtr("EchoResponse"),
					Field: []*descriptorpb.FieldDescriptorProto{
						{
							Name:     strPtr("reply"),
							Number:   int32Ptr(1),
							Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							JsonName: strPtr("reply"),
						},
					},
				},
			},
			Service: []*descriptorpb.ServiceDescriptorProto{
				{
					Name: strPtr("EchoService"),
					Method: []*descriptorpb.MethodDescriptorProto{
						{
							Name:       strPtr("Echo"),
							InputType:  strPtr(".reflecttest.EchoRequest"),
							OutputType: strPtr(".reflecttest.EchoResponse"),
						},
					},
				},
			},
		}

		fd, err := protodesc.NewFile(fdp, nil)
		if err != nil {
			// Can't use t.Fatalf inside Once (different goroutine may call).
			panic("failed to create file descriptor: " + err.Error())
		}

		if err := protoregistry.GlobalFiles.RegisterFile(fd); err != nil {
			// If already registered (e.g., from a previous test run in the same process), that's OK.
			if _, lookupErr := protoregistry.GlobalFiles.FindFileByPath("reflecttest/echo.proto"); lookupErr != nil {
				panic("failed to register file descriptor: " + err.Error())
			}
		}
	})
}

// startTestServerWithEchoService creates a gRPC server with the echo service registered
// and optionally with reflection. The echo service has a proper file descriptor registered
// in protoregistry.GlobalFiles so reflection can discover and serve its schema.
func startTestServerWithEchoService(t *testing.T, withReflection bool) string {
	t.Helper()

	registerTestServiceInGlobalRegistry(t)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()

	// Register the service with a ServiceDesc pointing to our registered proto file.
	sd := grpc.ServiceDesc{
		ServiceName: "reflecttest.EchoService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Echo",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					return nil, nil
				},
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "reflecttest/echo.proto",
	}
	server.RegisterService(&sd, struct{}{})

	if withReflection {
		reflection.Register(server)
	}

	go func() {
		_ = server.Serve(lis)
	}()
	t.Cleanup(func() {
		server.Stop()
		lis.Close()
	})

	return lis.Addr().String()
}

func int32Ptr(i int32) *int32 {
	return &i
}

// =============================================================================
// Tests
// =============================================================================

// TestReflectServices_FullDiscovery verifies that ReflectServices successfully discovers
// a service's schema including messages and methods via Server Reflection.
func TestReflectServices_FullDiscovery(t *testing.T) {
	t.Parallel()

	addr := startTestServerWithEchoService(t, true)

	ctx := context.Background()
	fds, err := ReflectServices(ctx, addr, nil)
	if err != nil {
		t.Fatalf("ReflectServices failed: %v", err)
	}

	if fds == nil {
		t.Fatal("expected non-nil FileDescriptorSet")
	}
	if len(fds.GetFile()) == 0 {
		t.Fatal("expected at least one file descriptor in result")
	}

	// Verify the returned FileDescriptorSet contains our test service's file.
	var foundFile bool
	for _, fd := range fds.GetFile() {
		if fd.GetName() == "reflecttest/echo.proto" {
			foundFile = true

			// Verify service is present.
			if len(fd.GetService()) == 0 {
				t.Error("expected at least one service in echo.proto descriptor")
			} else {
				svc := fd.GetService()[0]
				if svc.GetName() != "EchoService" {
					t.Errorf("expected service name 'EchoService', got %q", svc.GetName())
				}
				if len(svc.GetMethod()) == 0 {
					t.Error("expected at least one method in EchoService")
				} else if svc.GetMethod()[0].GetName() != "Echo" {
					t.Errorf("expected method name 'Echo', got %q", svc.GetMethod()[0].GetName())
				}
			}

			// Verify messages are present.
			if len(fd.GetMessageType()) < 2 {
				t.Errorf("expected at least 2 message types, got %d", len(fd.GetMessageType()))
			}
			msgNames := make(map[string]bool)
			for _, msg := range fd.GetMessageType() {
				msgNames[msg.GetName()] = true
			}
			if !msgNames["EchoRequest"] {
				t.Error("expected EchoRequest message in file descriptor")
			}
			if !msgNames["EchoResponse"] {
				t.Error("expected EchoResponse message in file descriptor")
			}
			break
		}
	}
	if !foundFile {
		t.Error("expected to find reflecttest/echo.proto in FileDescriptorSet")
		for _, fd := range fds.GetFile() {
			t.Logf("  found file: %s (services: %d, messages: %d)",
				fd.GetName(), len(fd.GetService()), len(fd.GetMessageType()))
		}
	}
}

// TestReflectServices_NoReflection verifies that ReflectServices returns an error
// when the server does not have reflection enabled.
func TestReflectServices_NoReflection(t *testing.T) {
	t.Parallel()

	// Start server WITHOUT reflection.
	addr := startTestServerWithEchoService(t, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ReflectServices(ctx, addr, nil)
	if err == nil {
		t.Fatal("expected error when server does not support reflection")
	}

	// Error should indicate reflection is not available.
	errStr := err.Error()
	if !containsStr(errStr, "reflection") && !containsStr(errStr, "Unimplemented") {
		t.Errorf("error should mention reflection is unavailable, got: %v", err)
	}
}

// TestReflectServices_Timeout verifies that ReflectServices respects context deadlines
// and returns a timeout error when the deadline is exceeded.
func TestReflectServices_Timeout(t *testing.T) {
	t.Parallel()

	// Use a non-routable IP address with a very short deadline to trigger timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := ReflectServices(ctx, "10.255.255.1:50051", nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}

	errStr := err.Error()
	if !containsStr(errStr, "timeout") && !containsStr(errStr, "deadline") &&
		!containsStr(errStr, "DeadlineExceeded") && !containsStr(errStr, "context deadline exceeded") {
		t.Errorf("error should indicate timeout, got: %v", err)
	}
}

// TestReflectServices_Unreachable verifies that ReflectServices returns an error
// when the target address is unreachable (connection refused).
func TestReflectServices_Unreachable(t *testing.T) {
	t.Parallel()

	// localhost:1 is extremely unlikely to have a gRPC server.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := ReflectServices(ctx, "localhost:1", nil)
	if err == nil {
		t.Fatal("expected connection error for unreachable target")
	}

	// Should receive some form of connection/reflection error.
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

// TestReflectServices_OnlyReflectionService verifies that ReflectServices returns an
// error when the server has reflection enabled but no application services registered.
func TestReflectServices_OnlyReflectionService(t *testing.T) {
	t.Parallel()

	// Start a gRPC server with ONLY reflection — no application services.
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	reflection.Register(server)

	go func() {
		_ = server.Serve(lis)
	}()
	t.Cleanup(func() {
		server.Stop()
		lis.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	addr := lis.Addr().String()
	_, err = ReflectServices(ctx, addr, nil)
	if err == nil {
		t.Fatal("expected error when server has only reflection service (no application services)")
	}

	errStr := err.Error()
	if !containsStr(errStr, "no discoverable services") {
		t.Errorf("error should mention no discoverable services, got: %v", err)
	}
}
