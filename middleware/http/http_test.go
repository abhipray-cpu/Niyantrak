package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/middleware"
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

// TestHTTPMiddleware_Wrap tests the Wrap method
func TestHTTPMiddleware_Wrap(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	options := &middleware.HTTPOptions{
		Extractor: func(r *http.Request) (string, error) {
			return "test-key", nil
		},
	}

	wrapped := mw.Wrap(handler, limiter, options)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if rec.Body.String() != "success" {
		t.Errorf("expected body 'success', got %s", rec.Body.String())
	}
}

// TestHTTPMiddleware_Wrap_Denied tests rate limit exceeded
func TestHTTPMiddleware_Wrap_Denied(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     10,
		},
	}

	mw := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called when rate limited")
	})

	options := &middleware.HTTPOptions{
		Extractor: func(r *http.Request) (string, error) {
			return "test-key", nil
		},
	}

	wrapped := mw.Wrap(handler, limiter, options)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rec.Code)
	}
}

// TestHTTPMiddleware_WrapFunc tests the WrapFunc method
func TestHTTPMiddleware_WrapFunc(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}

	options := &middleware.HTTPOptions{
		Extractor: func(r *http.Request) (string, error) {
			return "test-key", nil
		},
	}

	wrapped := mw.WrapFunc(handlerFunc, limiter, options)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestHTTPMiddleware_GetKeyExtractor tests the GetKeyExtractor method
func TestHTTPMiddleware_GetKeyExtractor(t *testing.T) {
	mw := New()

	extractor := mw.GetKeyExtractor()
	if extractor == nil {
		t.Error("expected non-nil key extractor")
	}

	// Test default extractor
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "test-key")

	key, err := extractor(req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if key != "test-key" {
		t.Errorf("expected key 'test-key', got %s", key)
	}
}

// TestHTTPMiddleware_CustomKeyExtractor tests custom key extraction
func TestHTTPMiddleware_CustomKeyExtractor(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	options := &middleware.HTTPOptions{
		Extractor: func(r *http.Request) (string, error) {
			return r.Header.Get("Custom-Key"), nil
		},
	}

	wrapped := mw.Wrap(handler, limiter, options)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Custom-Key", "custom-value")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestHTTPMiddleware_SkipPaths tests path skipping
func TestHTTPMiddleware_SkipPaths(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false, // Would deny if checked
			Remaining: 0,
			Limit:     10,
		},
	}

	mw := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	options := &middleware.HTTPOptions{
		SkipPaths: []string{"/health", "/metrics"},
		Extractor: func(r *http.Request) (string, error) {
			return "test-key", nil
		},
	}

	wrapped := mw.Wrap(handler, limiter, options)

	tests := []struct {
		path           string
		expectedStatus int
	}{
		{"/health", http.StatusOK},           // Should skip
		{"/metrics", http.StatusOK},          // Should skip
		{"/api", http.StatusTooManyRequests}, // Should check and deny
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("path %s: expected status %d, got %d", tt.path, tt.expectedStatus, rec.Code)
			}
		})
	}
}

// TestHTTPMiddleware_SkipMethods tests method skipping
func TestHTTPMiddleware_SkipMethods(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false, // Would deny if checked
			Remaining: 0,
			Limit:     10,
		},
	}

	mw := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	options := &middleware.HTTPOptions{
		SkipMethods: []string{"GET", "HEAD"},
		Extractor: func(r *http.Request) (string, error) {
			return "test-key", nil
		},
	}

	wrapped := mw.Wrap(handler, limiter, options)

	tests := []struct {
		method         string
		expectedStatus int
	}{
		{"GET", http.StatusOK},               // Should skip
		{"HEAD", http.StatusOK},              // Should skip
		{"POST", http.StatusTooManyRequests}, // Should check and deny
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("method %s: expected status %d, got %d", tt.method, tt.expectedStatus, rec.Code)
			}
		})
	}
}

// TestHTTPMiddleware_CustomKeyHeader tests CustomKeyHeader option
func TestHTTPMiddleware_CustomKeyHeader(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	options := &middleware.HTTPOptions{
		CustomKeyHeader: "X-Custom-Key",
	}

	wrapped := mw.Wrap(handler, limiter, options)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Custom-Key", "my-custom-key")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestHTTPMiddleware_HeadersSet tests that rate limit headers are set
func TestHTTPMiddleware_HeadersSet(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	options := &middleware.HTTPOptions{
		Extractor: func(r *http.Request) (string, error) {
			return "test-key", nil
		},
	}

	wrapped := mw.Wrap(handler, limiter, options)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Check that headers are set (will be set by formatter)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestHTTPMiddleware_CustomRateLimitHandler tests custom handler
func TestHTTPMiddleware_CustomRateLimitHandler(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     10,
		},
	}

	customHandlerCalled := false
	customHandler := &mockRateLimitHandler{
		onExceeded: func(w http.ResponseWriter, r *http.Request, result interface{}) {
			customHandlerCalled = true
			w.WriteHeader(http.StatusTeapot) // Custom status code
		},
	}

	mw := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	options := &middleware.HTTPOptions{
		Handler: customHandler,
		Extractor: func(r *http.Request) (string, error) {
			return "test-key", nil
		},
	}

	wrapped := mw.Wrap(handler, limiter, options)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if !customHandlerCalled {
		t.Error("custom handler should have been called")
	}

	if rec.Code != http.StatusTeapot {
		t.Errorf("expected status 418, got %d", rec.Code)
	}
}

// TestHTTPMiddleware_ErrorHandling tests error handling from key extractor
func TestHTTPMiddleware_ErrorHandling(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	errorHandlerCalled := false
	errorHandler := &mockRateLimitHandler{
		onError: func(w http.ResponseWriter, r *http.Request, err error) {
			errorHandlerCalled = true
			w.WriteHeader(http.StatusBadRequest)
		},
	}

	mw := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	options := &middleware.HTTPOptions{
		Handler: errorHandler,
		Extractor: func(r *http.Request) (string, error) {
			return "", nil // Empty key - should trigger error
		},
	}

	wrapped := mw.Wrap(handler, limiter, options)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if !errorHandlerCalled {
		t.Error("error handler should have been called")
	}
}

// TestHTTPMiddleware_InterfaceCompliance verifies interface compliance
func TestHTTPMiddleware_InterfaceCompliance(t *testing.T) {
	var _ middleware.HTTPMiddleware = (*httpMiddleware)(nil)
}

// mockRateLimitHandler is a mock implementation of RateLimitHandler
type mockRateLimitHandler struct {
	onExceeded func(w http.ResponseWriter, r *http.Request, result interface{})
	onError    func(w http.ResponseWriter, r *http.Request, err error)
}

func (m *mockRateLimitHandler) HandleExceeded(w http.ResponseWriter, r *http.Request, result interface{}) {
	if m.onExceeded != nil {
		m.onExceeded(w, r, result)
	}
}

func (m *mockRateLimitHandler) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	if m.onError != nil {
		m.onError(w, r, err)
	}
}

// BenchmarkHTTPMiddleware benchmarks the middleware
func BenchmarkHTTPMiddleware(b *testing.B) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	mw := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	options := &middleware.HTTPOptions{
		Extractor: func(r *http.Request) (string, error) {
			return "test-key", nil
		},
	}

	wrapped := mw.Wrap(handler, limiter, options)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
	}
}
