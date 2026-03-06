package grpcmiddleware

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	errdetails "google.golang.org/genproto/googleapis/rpc/errdetails"
)

// grpcMiddleware implements the middleware.GRPCMiddleware interface
type grpcMiddleware struct {
	keyExtractor middleware.GRPCKeyExtractor
}

// New creates a new gRPC middleware instance
func New() middleware.GRPCMiddleware {
	return &grpcMiddleware{
		keyExtractor: defaultKeyExtractor,
	}
}

// UnaryInterceptor returns a gRPC unary server interceptor
func (g *grpcMiddleware) UnaryInterceptor(limiter interface{}, options *middleware.GRPCOptions) interface{} {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Check if method should be skipped
		if g.shouldSkipMethod(info.FullMethod, options) {
			return handler(ctx, req)
		}

		// Extract key
		extractor := g.getExtractor(options)
		key, err := extractor(ctx)
		if err != nil || key == "" {
			return nil, status.Error(codes.InvalidArgument, "failed to extract rate limit key")
		}

		// Check rate limit
		result := g.checkLimit(ctx, limiter, key)

		// Add metadata if enabled
		if options != nil && options.EnableMetadata {
			ctx = g.addMetadata(ctx, result, options)
		}

		if !result.Allowed {
			return nil, g.createLimitExceededError(result, options)
		}

		// Proceed to handler
		return handler(ctx, req)
	}
}

// StreamInterceptor returns a gRPC stream server interceptor
func (g *grpcMiddleware) StreamInterceptor(limiter interface{}, options *middleware.GRPCOptions) interface{} {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := stream.Context()

		// Check if method should be skipped
		if g.shouldSkipMethod(info.FullMethod, options) {
			return handler(srv, stream)
		}

		// Extract key
		extractor := g.getExtractor(options)
		key, err := extractor(ctx)
		if err != nil || key == "" {
			return status.Error(codes.InvalidArgument, "failed to extract rate limit key")
		}

		// Check rate limit
		result := g.checkLimit(ctx, limiter, key)

		// Add metadata if enabled
		if options != nil && options.EnableMetadata {
			g.sendMetadata(stream, result, options)
		}

		if !result.Allowed {
			return g.createLimitExceededError(result, options)
		}

		// Proceed to handler
		return handler(srv, stream)
	}
}

// GetKeyExtractor returns the default key extractor
func (g *grpcMiddleware) GetKeyExtractor() middleware.GRPCKeyExtractor {
	return g.keyExtractor
}

// shouldSkipMethod determines if rate limiting should be skipped for the method
func (g *grpcMiddleware) shouldSkipMethod(fullMethod string, options *middleware.GRPCOptions) bool {
	if options == nil || len(options.SkipMethods) == 0 {
		return false
	}

	for _, method := range options.SkipMethods {
		if method == fullMethod {
			return true
		}
	}

	return false
}

// getExtractor returns the appropriate key extractor
func (g *grpcMiddleware) getExtractor(options *middleware.GRPCOptions) middleware.GRPCKeyExtractor {
	if options != nil && options.Extractor != nil {
		return options.Extractor
	}
	return g.keyExtractor
}

// checkLimit checks the rate limit using the limiter
func (g *grpcMiddleware) checkLimit(ctx context.Context, limiter interface{}, key string) *limiters.LimitResult {
	// Type assert the limiter
	switch l := limiter.(type) {
	case interface {
		Allow(context.Context, string) *limiters.LimitResult
	}:
		return l.Allow(ctx, key)
	default:
		// Return denied result if limiter type is unknown
		return &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     0,
		}
	}
}

// addMetadata adds rate limit information to the context metadata
func (g *grpcMiddleware) addMetadata(ctx context.Context, result *limiters.LimitResult, _ *middleware.GRPCOptions) context.Context {
	md := metadata.Pairs(
		"x-ratelimit-limit", strconv.Itoa(result.Limit),
		"x-ratelimit-remaining", strconv.Itoa(result.Remaining),
	)

	if !result.ResetAt.IsZero() {
		md.Append("x-ratelimit-reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
	}

	if result.RetryAfter > 0 {
		md.Append("retry-after", fmt.Sprintf("%.0f", result.RetryAfter.Seconds()))
	}

	return metadata.NewOutgoingContext(ctx, md)
}

// sendMetadata sends rate limit information via stream header
func (g *grpcMiddleware) sendMetadata(stream grpc.ServerStream, result *limiters.LimitResult, _ *middleware.GRPCOptions) {
	md := metadata.Pairs(
		"x-ratelimit-limit", strconv.Itoa(result.Limit),
		"x-ratelimit-remaining", strconv.Itoa(result.Remaining),
	)

	if !result.ResetAt.IsZero() {
		md.Append("x-ratelimit-reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
	}

	if result.RetryAfter > 0 {
		md.Append("retry-after", fmt.Sprintf("%.0f", result.RetryAfter.Seconds()))
	}

	stream.SendHeader(md)
}

// createLimitExceededError creates a gRPC error for rate limit exceeded,
// enriched with structured error details (RetryInfo, QuotaFailure) so
// well-behaved clients can react programmatically.
func (g *grpcMiddleware) createLimitExceededError(result *limiters.LimitResult, options *middleware.GRPCOptions) error {
	code := codes.ResourceExhausted

	// Use custom status code if provided
	if options != nil && options.CustomStatusCode != "" {
		if customCode := parseStatusCode(options.CustomStatusCode); customCode != codes.Unknown {
			code = customCode
		}
	}

	msg := fmt.Sprintf("rate limit exceeded: %d/%d requests", result.Limit-result.Remaining, result.Limit)
	if result.RetryAfter > 0 {
		msg = fmt.Sprintf("%s, retry after %.0f seconds", msg, result.RetryAfter.Seconds())
	}

	st := status.New(code, msg)

	// Build structured error details.
	retryInfo := &errdetails.RetryInfo{}
	if result.RetryAfter > 0 {
		retryInfo.RetryDelay = durationpb.New(time.Duration(result.RetryAfter))
	}

	quotaFailure := &errdetails.QuotaFailure{
		Violations: []*errdetails.QuotaFailure_Violation{{
			Subject:     "rate_limit",
			Description: msg,
		}},
	}

	stWithDetails, detailErr := st.WithDetails(retryInfo, quotaFailure)
	if detailErr == nil {
		return stWithDetails.Err()
	}
	// If attaching details fails, return the plain status.
	return st.Err()
}

// parseStatusCode parses a string status code to gRPC codes.Code
func parseStatusCode(codeStr string) codes.Code {
	switch codeStr {
	case "OK":
		return codes.OK
	case "CANCELLED":
		return codes.Canceled
	case "UNKNOWN":
		return codes.Unknown
	case "INVALID_ARGUMENT":
		return codes.InvalidArgument
	case "DEADLINE_EXCEEDED":
		return codes.DeadlineExceeded
	case "NOT_FOUND":
		return codes.NotFound
	case "ALREADY_EXISTS":
		return codes.AlreadyExists
	case "PERMISSION_DENIED":
		return codes.PermissionDenied
	case "RESOURCE_EXHAUSTED":
		return codes.ResourceExhausted
	case "FAILED_PRECONDITION":
		return codes.FailedPrecondition
	case "ABORTED":
		return codes.Aborted
	case "OUT_OF_RANGE":
		return codes.OutOfRange
	case "UNIMPLEMENTED":
		return codes.Unimplemented
	case "INTERNAL":
		return codes.Internal
	case "UNAVAILABLE":
		return codes.Unavailable
	case "DATA_LOSS":
		return codes.DataLoss
	case "UNAUTHENTICATED":
		return codes.Unauthenticated
	default:
		return codes.Unknown
	}
}

// defaultKeyExtractor is the default key extraction function
func defaultKeyExtractor(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no metadata in context")
	}

	// Try x-api-key
	if vals := md.Get("x-api-key"); len(vals) > 0 {
		return vals[0], nil
	}

	// Try authorization
	if vals := md.Get("authorization"); len(vals) > 0 {
		return vals[0], nil
	}

	return "", errors.New("no key found in metadata")
}
