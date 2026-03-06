package http

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/middleware"
)

// defaultHeaderFormatter is the default implementation of HeaderFormatter
type defaultHeaderFormatter struct {
	headerNames *middleware.HeaderNames
}

// NewDefaultHeaderFormatter creates a new default header formatter
func NewDefaultHeaderFormatter() middleware.HeaderFormatter {
	return &defaultHeaderFormatter{
		headerNames: &middleware.HeaderNames{
			Limit:      "X-RateLimit-Limit",
			Remaining:  "X-RateLimit-Remaining",
			Reset:      "X-RateLimit-Reset",
			RetryAfter: "Retry-After",
		},
	}
}

// FormatHeaders formats and sets rate limit headers
func (f *defaultHeaderFormatter) FormatHeaders(w http.ResponseWriter, result interface{}) {
	limitResult, ok := result.(*limiters.LimitResult)
	if !ok {
		return
	}

	// Set limit header
	w.Header().Set(f.headerNames.Limit, strconv.Itoa(limitResult.Limit))

	// Set remaining header
	w.Header().Set(f.headerNames.Remaining, strconv.Itoa(limitResult.Remaining))

	// Set reset header if available
	if !limitResult.ResetAt.IsZero() {
		w.Header().Set(f.headerNames.Reset, strconv.FormatInt(limitResult.ResetAt.Unix(), 10))
	}

	// Set retry-after header if rate limited
	if !limitResult.Allowed && limitResult.RetryAfter > 0 {
		retryAfterSeconds := limitResult.RetryAfter.Seconds()
		w.Header().Set(f.headerNames.RetryAfter, fmt.Sprintf("%.0f", retryAfterSeconds))
	}
}

// GetHeaderNames returns the header names configuration
func (f *defaultHeaderFormatter) GetHeaderNames() *middleware.HeaderNames {
	return f.headerNames
}

// customHeaderFormatter allows custom header names
type customHeaderFormatter struct {
	headerNames *middleware.HeaderNames
}

// NewCustomHeaderFormatter creates a formatter with custom header names
func NewCustomHeaderFormatter(names *middleware.HeaderNames) middleware.HeaderFormatter {
	if names == nil {
		names = &middleware.HeaderNames{
			Limit:      "X-RateLimit-Limit",
			Remaining:  "X-RateLimit-Remaining",
			Reset:      "X-RateLimit-Reset",
			RetryAfter: "Retry-After",
		}
	}
	return &customHeaderFormatter{
		headerNames: names,
	}
}

// FormatHeaders formats and sets rate limit headers with custom names
func (f *customHeaderFormatter) FormatHeaders(w http.ResponseWriter, result interface{}) {
	limitResult, ok := result.(*limiters.LimitResult)
	if !ok {
		return
	}

	if f.headerNames.Limit != "" {
		w.Header().Set(f.headerNames.Limit, strconv.Itoa(limitResult.Limit))
	}

	if f.headerNames.Remaining != "" {
		w.Header().Set(f.headerNames.Remaining, strconv.Itoa(limitResult.Remaining))
	}

	if f.headerNames.Reset != "" && !limitResult.ResetAt.IsZero() {
		w.Header().Set(f.headerNames.Reset, strconv.FormatInt(limitResult.ResetAt.Unix(), 10))
	}

	if f.headerNames.RetryAfter != "" && !limitResult.Allowed && limitResult.RetryAfter > 0 {
		retryAfterSeconds := limitResult.RetryAfter.Seconds()
		w.Header().Set(f.headerNames.RetryAfter, fmt.Sprintf("%.0f", retryAfterSeconds))
	}
}

// GetHeaderNames returns the custom header names configuration
func (f *customHeaderFormatter) GetHeaderNames() *middleware.HeaderNames {
	return f.headerNames
}
