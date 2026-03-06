package custom

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/abhipray-cpu/niyantrak/backend"
)

// entry represents a stored value with metadata
type entry struct {
	value     interface{}
	expiresAt time.Time
	hasExpiry bool
}

// customBackend is a user-defined example storage backend
// This serves as a reference implementation showing how users can create their own backends
type customBackend struct {
	data   map[string]*entry
	mu     sync.RWMutex
	closed bool
	name   string
	config map[string]interface{}
}

// Compile-time check to ensure customBackend implements backend.CustomBackend
var _ backend.CustomBackend = (*customBackend)(nil)

// Compile-time check to ensure customBackend implements backend.AtomicBackend
var _ backend.AtomicBackend = (*customBackend)(nil)

// NewCustomBackend creates a new custom backend
// name: Name of the custom backend instance
// config: Optional configuration map
func NewCustomBackend(name string, config map[string]interface{}) backend.CustomBackend {
	if config == nil {
		config = make(map[string]interface{})
	}

	return &customBackend{
		data:   make(map[string]*entry),
		closed: false,
		name:   name,
		config: config,
	}
}

// Get retrieves the current state for a key
func (c *customBackend) Get(ctx context.Context, key string) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, backend.ErrBackendClosed
	}

	e, exists := c.data[key]
	if !exists {
		return nil, backend.ErrKeyNotFound
	}

	// Check if expired
	if e.hasExpiry && time.Now().After(e.expiresAt) {
		return nil, backend.ErrKeyExpired
	}

	return e.value, nil
}

// Set updates the state for a key with optional TTL
func (c *customBackend) Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return backend.ErrBackendClosed
	}

	e := &entry{
		value:     state,
		hasExpiry: ttl > 0,
	}

	if ttl > 0 {
		e.expiresAt = time.Now().Add(ttl)
	}

	c.data[key] = e
	return nil
}

// IncrementAndGet atomically increments the value for a key
func (c *customBackend) IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, backend.ErrBackendClosed
	}

	e, exists := c.data[key]

	// Check if key exists and hasn't expired
	if !exists || (e.hasExpiry && time.Now().After(e.expiresAt)) {
		// Start from 0
		newEntry := &entry{
			value:     int64(1),
			hasExpiry: ttl > 0,
		}
		if ttl > 0 {
			newEntry.expiresAt = time.Now().Add(ttl)
		}
		c.data[key] = newEntry
		return 1, nil
	}

	// Increment existing value
	var currentVal int64
	switch v := e.value.(type) {
	case int64:
		currentVal = v
	case int:
		currentVal = int64(v)
	case float64:
		currentVal = int64(v)
	default:
		// Can't increment non-numeric value
		currentVal = 0
	}

	newVal := currentVal + 1
	e.value = newVal

	// Update TTL if specified
	if ttl > 0 {
		e.hasExpiry = true
		e.expiresAt = time.Now().Add(ttl)
	}

	return newVal, nil
}

// Delete removes a key from the backend
func (c *customBackend) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return backend.ErrBackendClosed
	}

	delete(c.data, key)
	return nil
}

// Update atomically reads, transforms, and writes the state for a key.
// The entire operation runs under a single write lock.
func (c *customBackend) Update(ctx context.Context, key string, ttl time.Duration, fn backend.UpdateFunc) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, backend.ErrBackendClosed
	}

	// Read current state
	var current interface{}
	if e, exists := c.data[key]; exists {
		if e.hasExpiry && time.Now().After(e.expiresAt) {
			delete(c.data, key)
			current = nil
		} else {
			current = e.value
		}
	}

	// Execute the transform function
	newState, result, err := fn(current)
	if err != nil {
		return nil, err
	}

	// Write back
	c.data[key] = &entry{
		value:     newState,
		hasExpiry: ttl > 0,
	}
	if ttl > 0 {
		c.data[key].expiresAt = time.Now().Add(ttl)
	}

	return result, nil
}

// Close cleans up resources
func (c *customBackend) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	c.data = nil
	return nil
}

// Ping checks if backend is healthy
func (c *customBackend) Ping(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return backend.ErrBackendClosed
	}

	return nil
}

// Type returns the backend type name
func (c *customBackend) Type() string {
	return "custom"
}

// GetMetadata returns custom metadata about the backend
func (c *customBackend) GetMetadata(ctx context.Context) map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metadata := map[string]interface{}{
		"name":   c.name,
		"type":   "custom",
		"closed": c.closed,
		"config": c.config,
	}

	// Add statistics
	if !c.closed {
		metadata["key_count"] = len(c.data)

		expiredCount := 0
		for _, e := range c.data {
			if e.hasExpiry && time.Now().After(e.expiresAt) {
				expiredCount++
			}
		}
		metadata["expired_count"] = expiredCount
	}

	return metadata
}

// Execute executes a custom operation
func (c *customBackend) Execute(ctx context.Context, operation string, args map[string]interface{}) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, backend.ErrBackendClosed
	}

	switch operation {
	case "count":
		// Return the number of keys
		return len(c.data), nil

	case "clear":
		// Clear all keys
		c.data = make(map[string]*entry)
		return "cleared", nil

	case "stats":
		// Return statistics
		stats := map[string]interface{}{
			"total_keys": len(c.data),
		}

		expiredCount := 0
		activeCount := 0
		for _, e := range c.data {
			if e.hasExpiry && time.Now().After(e.expiresAt) {
				expiredCount++
			} else {
				activeCount++
			}
		}
		stats["expired_keys"] = expiredCount
		stats["active_keys"] = activeCount

		return stats, nil

	case "list":
		// List keys with optional prefix filter
		var prefix string
		if args != nil {
			if p, ok := args["prefix"].(string); ok {
				prefix = p
			}
		}

		keys := make([]string, 0)
		for key := range c.data {
			if prefix == "" || len(key) >= len(prefix) && key[:len(prefix)] == prefix {
				keys = append(keys, key)
			}
		}
		return keys, nil

	case "cleanup":
		// Remove expired keys
		removed := 0
		for key, e := range c.data {
			if e.hasExpiry && time.Now().After(e.expiresAt) {
				delete(c.data, key)
				removed++
			}
		}
		return removed, nil

	case "export":
		// Export all data as JSON
		exported := make(map[string]interface{})
		for key, e := range c.data {
			if !e.hasExpiry || !time.Now().After(e.expiresAt) {
				exported[key] = e.value
			}
		}

		jsonData, err := json.Marshal(exported)
		if err != nil {
			return nil, err
		}
		return string(jsonData), nil

	case "import":
		// Import data from JSON
		if args == nil || args["data"] == nil {
			return nil, errors.New("missing data argument")
		}

		jsonStr, ok := args["data"].(string)
		if !ok {
			return nil, errors.New("data must be a JSON string")
		}

		var imported map[string]interface{}
		err := json.Unmarshal([]byte(jsonStr), &imported)
		if err != nil {
			return nil, err
		}

		count := 0
		for key, value := range imported {
			c.data[key] = &entry{
				value:     value,
				hasExpiry: false,
			}
			count++
		}
		return count, nil

	default:
		return nil, errors.New("unknown operation: " + operation)
	}
}
