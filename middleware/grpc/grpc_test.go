package grpcmiddleware

import (
	"context"
	"testing"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mockLimiter is a simple mock limiter for testing
type mockLimiter struct {
	allowResult *limiters.LimitResult
}

func (m *mockLimiter) Allow(ctx context.Context, key string) *limiters.LimitResult {
	return m.allowResult
}

func (m *mockLimiter) AllowN(ctx context.Context, key string, n int) *limiters.LimitResult {
	return m.allowResult
}

func (m *mockLimiter) Reset(ctx context.Context, key string) error {
	return nil
}

func (m *mockLimiter) GetStats(ctx context.Context, key string) interface{} {
	return nil
}

func (m *mockLimiter) Close() error {
	return nil
}

func (m *mockLimiter) SetLimit(ctx context.Context, key string, limit int, window interface{}) error {
	return nil
}

func (m *mockLimiter) Type() string {
	return "mock"
}

// TestGRPCMiddleware_UnaryInterceptor tests the unary interceptor
func TestGRPCMiddleware_UnaryInterceptor(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	options := &middleware.GRPCOptions{
		Extractor: func(ctx context.Context) (string, error) {
			return "test-key", nil
		},
	}

	interceptor := mw.UnaryInterceptor(limiter, options)
	unaryInterceptor, ok := interceptor.(func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error))
	if !ok {
		t.Fatal("expected UnaryServerInterceptor function")
	}

	handlerCalled := false
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		return "response", nil
	}

	ctx := context.Background()
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	resp, err := unaryInterceptor(ctx, "request", info, handler)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Error("handler should have been called")
	}

	if resp != "response" {
		t.Errorf("expected response 'response', got %v", resp)
	}
}

// TestGRPCMiddleware_UnaryInterceptor_Denied tests rate limit exceeded
func TestGRPCMiddleware_UnaryInterceptor_Denied(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     10,
		},
	}

	mw := New()

	options := &middleware.GRPCOptions{
		Extractor: func(ctx context.Context) (string, error) {
			return "test-key", nil
		},
	}

	interceptor := mw.UnaryInterceptor(limiter, options)
	unaryInterceptor := interceptor.(func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error))

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called when rate limited")
		return nil, nil
	}

	ctx := context.Background()
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	resp, err := unaryInterceptor(ctx, "request", info, handler)

	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	if err == nil {
		t.Error("expected error for rate limit exceeded")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Error("expected gRPC status error")
	}

	if st.Code() != codes.ResourceExhausted {
		t.Errorf("expected ResourceExhausted code, got %v", st.Code())
	}
}

// TestGRPCMiddleware_StreamInterceptor tests the stream interceptor
func TestGRPCMiddleware_StreamInterceptor(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	options := &middleware.GRPCOptions{
		Extractor: func(ctx context.Context) (string, error) {
			return "test-key", nil
		},
	}

	interceptor := mw.StreamInterceptor(limiter, options)
	streamInterceptor, ok := interceptor.(func(interface{}, grpc.ServerStream, *grpc.StreamServerInfo, grpc.StreamHandler) error)
	if !ok {
		t.Fatal("expected StreamServerInterceptor function")
	}

	handlerCalled := false
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}

	mockStream := &mockServerStream{ctx: context.Background()}
	err := streamInterceptor("srv", mockStream, info, handler)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Error("handler should have been called")
	}
}

// TestGRPCMiddleware_StreamInterceptor_Denied tests stream rate limit exceeded
func TestGRPCMiddleware_StreamInterceptor_Denied(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     10,
		},
	}

	mw := New()

	options := &middleware.GRPCOptions{
		Extractor: func(ctx context.Context) (string, error) {
			return "test-key", nil
		},
	}

	interceptor := mw.StreamInterceptor(limiter, options)
	streamInterceptor := interceptor.(func(interface{}, grpc.ServerStream, *grpc.StreamServerInfo, grpc.StreamHandler) error)

	handler := func(srv interface{}, stream grpc.ServerStream) error {
		t.Error("handler should not be called when rate limited")
		return nil
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}

	mockStream := &mockServerStream{ctx: context.Background()}
	err := streamInterceptor("srv", mockStream, info, handler)

	if err == nil {
		t.Error("expected error for rate limit exceeded")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Error("expected gRPC status error")
	}

	if st.Code() != codes.ResourceExhausted {
		t.Errorf("expected ResourceExhausted code, got %v", st.Code())
	}
}

// TestGRPCMiddleware_GetKeyExtractor tests the GetKeyExtractor method
func TestGRPCMiddleware_GetKeyExtractor(t *testing.T) {
	mw := New()

	extractor := mw.GetKeyExtractor()
	if extractor == nil {
		t.Error("expected non-nil key extractor")
	}

	// Test default extractor
	md := metadata.New(map[string]string{"x-api-key": "test-key"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	key, err := extractor(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if key != "test-key" {
		t.Errorf("expected key 'test-key', got %s", key)
	}
}

// TestGRPCMiddleware_CustomKeyExtractor tests custom key extraction
func TestGRPCMiddleware_CustomKeyExtractor(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	options := &middleware.GRPCOptions{
		Extractor: func(ctx context.Context) (string, error) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok {
				return "", nil
			}
			vals := md.Get("custom-key")
			if len(vals) > 0 {
				return vals[0], nil
			}
			return "", nil
		},
	}

	interceptor := mw.UnaryInterceptor(limiter, options)
	unaryInterceptor := interceptor.(func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error))

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	md := metadata.New(map[string]string{"custom-key": "custom-value"})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	_, err := unaryInterceptor(ctx, "request", info, handler)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestGRPCMiddleware_SkipMethods tests method skipping
func TestGRPCMiddleware_SkipMethods(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false, // Would deny if checked
			Remaining: 0,
			Limit:     10,
		},
	}

	mw := New()

	options := &middleware.GRPCOptions{
		SkipMethods: []string{"/grpc.health.v1.Health/Check", "/test.Service/Health"},
		Extractor: func(ctx context.Context) (string, error) {
			return "test-key", nil
		},
	}

	interceptor := mw.UnaryInterceptor(limiter, options)
	unaryInterceptor := interceptor.(func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error))

	tests := []struct {
		method      string
		shouldAllow bool
	}{
		{"/grpc.health.v1.Health/Check", true}, // Should skip
		{"/test.Service/Health", true},         // Should skip
		{"/test.Service/API", false},           // Should check and deny
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				return "response", nil
			}

			ctx := context.Background()
			info := &grpc.UnaryServerInfo{
				FullMethod: tt.method,
			}

			_, err := unaryInterceptor(ctx, "request", info, handler)

			if tt.shouldAllow && err != nil {
				t.Errorf("method %s: expected success, got error: %v", tt.method, err)
			}

			if !tt.shouldAllow && err == nil {
				t.Errorf("method %s: expected error, got success", tt.method)
			}
		})
	}
}

// TestGRPCMiddleware_EnableMetadata tests metadata inclusion
func TestGRPCMiddleware_EnableMetadata(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	options := &middleware.GRPCOptions{
		EnableMetadata: true,
		MetadataKey:    "x-ratelimit",
		Extractor: func(ctx context.Context) (string, error) {
			return "test-key", nil
		},
	}

	interceptor := mw.UnaryInterceptor(limiter, options)
	unaryInterceptor := interceptor.(func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error))

	handlerCalled := false
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		// Check if metadata was added to context
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok || len(md.Get("x-ratelimit-limit")) == 0 {
			t.Error("expected rate limit metadata in context")
		}
		return "response", nil
	}

	ctx := context.Background()
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	_, err := unaryInterceptor(ctx, "request", info, handler)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Error("handler should have been called")
	}
}

// TestGRPCMiddleware_CustomStatusCode tests custom status code
func TestGRPCMiddleware_CustomStatusCode(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     10,
		},
	}

	mw := New()

	options := &middleware.GRPCOptions{
		CustomStatusCode: "UNAVAILABLE",
		Extractor: func(ctx context.Context) (string, error) {
			return "test-key", nil
		},
	}

	interceptor := mw.UnaryInterceptor(limiter, options)
	unaryInterceptor := interceptor.(func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error))

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, nil
	}

	ctx := context.Background()
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	_, err := unaryInterceptor(ctx, "request", info, handler)

	st, ok := status.FromError(err)
	if !ok {
		t.Error("expected gRPC status error")
	}

	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable code, got %v", st.Code())
	}
}

// TestGRPCMiddleware_InterfaceCompliance verifies interface compliance
func TestGRPCMiddleware_InterfaceCompliance(t *testing.T) {
	var _ middleware.GRPCMiddleware = (*grpcMiddleware)(nil)
}

// mockServerStream is a mock implementation of grpc.ServerStream
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func (m *mockServerStream) SendHeader(md metadata.MD) error {
	return nil
}

func (m *mockServerStream) SetTrailer(md metadata.MD) {
}

// BenchmarkGRPCMiddleware_UnaryInterceptor benchmarks the unary interceptor
func BenchmarkGRPCMiddleware_UnaryInterceptor(b *testing.B) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	options := &middleware.GRPCOptions{
		Extractor: func(ctx context.Context) (string, error) {
			return "test-key", nil
		},
	}

	interceptor := mw.UnaryInterceptor(limiter, options)
	unaryInterceptor := interceptor.(func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error))

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	ctx := context.Background()
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unaryInterceptor(ctx, "request", info, handler)
	}
}

// BenchmarkGRPCMiddleware_StreamInterceptor benchmarks the stream interceptor
func BenchmarkGRPCMiddleware_StreamInterceptor(b *testing.B) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	options := &middleware.GRPCOptions{
		Extractor: func(ctx context.Context) (string, error) {
			return "test-key", nil
		},
	}

	interceptor := mw.StreamInterceptor(limiter, options)
	streamInterceptor := interceptor.(func(interface{}, grpc.ServerStream, *grpc.StreamServerInfo, grpc.StreamHandler) error)

	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}

	mockStream := &mockServerStream{ctx: context.Background()}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		streamInterceptor("srv", mockStream, info, handler)
	}
}
