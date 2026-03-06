// Package grpcmiddleware provides gRPC unary and streaming interceptors that
// enforce rate limits.
//
// Denied RPCs return codes.ResourceExhausted with google.rpc.RetryInfo and
// google.rpc.QuotaFailure error details, enabling well-behaved clients to
// implement programmatic retry logic.
package grpcmiddleware
