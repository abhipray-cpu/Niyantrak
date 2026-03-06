package http

import (
	"encoding/json"
	"net/http"

	"github.com/abhipray-cpu/niyantrak/middleware"
)

// defaultRateLimitHandler is the default implementation of RateLimitHandler
type defaultRateLimitHandler struct{}

// NewDefaultRateLimitHandler creates a new default rate limit handler
func NewDefaultRateLimitHandler() middleware.RateLimitHandler {
	return &defaultRateLimitHandler{}
}

// HandleExceeded handles rate limit exceeded scenarios
func (h *defaultRateLimitHandler) HandleExceeded(w http.ResponseWriter, r *http.Request, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"error":   "rate limit exceeded",
		"message": "Too many requests. Please try again later.",
	}

	json.NewEncoder(w).Encode(response)
}

// HandleError handles errors during rate limiting
func (h *defaultRateLimitHandler) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	response := map[string]interface{}{
		"error":   "rate limit error",
		"message": err.Error(),
	}

	json.NewEncoder(w).Encode(response)
}
