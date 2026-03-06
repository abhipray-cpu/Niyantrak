package memory

import (
	"context"
	"errors"
	"sync"
	"time"
	"unsafe"

	"github.com/abhipray-cpu/niyantrak/backend"
)

// Local error definition for memory-specific error
var (
	ErrMaxSizeExceeded = errors.New("max size exceeded")
)

// entry represents a stored value with metadata
type entry struct {
	Data      interface{}
	CreatedAt time.Time
	ExpiresAt time.Time // Zero value means no expiration
}

// memoryBackend is an in-memory storage backend using sync.RWMutex
type memoryBackend struct {
	mu      sync.RWMutex
	data    map[string]*entry
	maxSize int // 0 means unlimited
	closed  bool

	// GC fields — only active when created via NewMemoryBackendWithGC
	gcDone chan struct{} // closed on Close() to stop the GC goroutine
}

// Compile-time check to ensure memoryBackend implements backend.MemoryBackend
var _ backend.MemoryBackend = (*memoryBackend)(nil)

// Compile-time check to ensure memoryBackend implements backend.AtomicBackend
var _ backend.AtomicBackend = (*memoryBackend)(nil)

// NewMemoryBackend creates a new memory backend with unlimited size
func NewMemoryBackend() backend.MemoryBackend {
	return &memoryBackend{
		data:    make(map[string]*entry),
		maxSize: 0, // unlimited by default
		closed:  false,
	}
}

// NewMemoryBackendWithGC creates a memory backend that periodically sweeps
// expired keys. The GC goroutine runs every gcInterval and is stopped
// automatically when Close() is called.
//
// For long-running processes this prevents unbounded memory growth from
// keys that expire but are never read again (lazy expiry alone won't
// reclaim them).
func NewMemoryBackendWithGC(gcInterval time.Duration) backend.MemoryBackend {
	m := &memoryBackend{
		data:    make(map[string]*entry),
		maxSize: 0,
		closed:  false,
		gcDone:  make(chan struct{}),
	}

	go m.runGC(gcInterval)
	return m
}

// runGC periodically deletes expired entries.
func (m *memoryBackend) runGC(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.sweepExpired()
		case <-m.gcDone:
			return
		}
	}
}

// sweepExpired removes all expired entries under a write lock.
func (m *memoryBackend) sweepExpired() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, ent := range m.data {
		if !ent.ExpiresAt.IsZero() && now.After(ent.ExpiresAt) {
			delete(m.data, key)
		}
	}
}

// Get retrieves the current state for a key
func (m *memoryBackend) Get(ctx context.Context, key string) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, backend.ErrBackendClosed
	}

	ent, exists := m.data[key]
	if !exists {
		return nil, backend.ErrKeyNotFound
	}

	// Check if expired
	if !ent.ExpiresAt.IsZero() && time.Now().After(ent.ExpiresAt) {
		delete(m.data, key)
		return nil, backend.ErrKeyExpired
	}

	return ent.Data, nil
}

// Set updates the state for a key with optional TTL
func (m *memoryBackend) Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return backend.ErrBackendClosed
	}

	// Check max size (if key doesn't exist and we're at max)
	if m.maxSize > 0 && len(m.data) >= m.maxSize && m.data[key] == nil {
		return ErrMaxSizeExceeded
	}

	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	m.data[key] = &entry{
		Data:      state,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	return nil
}

// IncrementAndGet atomically increments the value for a key and returns new value
func (m *memoryBackend) IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, backend.ErrBackendClosed
	}

	var value int64

	// Check if key exists and is not expired
	if ent, exists := m.data[key]; exists {
		if !ent.ExpiresAt.IsZero() && time.Now().After(ent.ExpiresAt) {
			// Key expired, start fresh
			value = 0
		} else {
			// Key exists, get current value
			if v, ok := ent.Data.(int64); ok {
				value = v
			}
		}
	}

	// Increment
	value++

	// Check max size before adding new key
	if m.maxSize > 0 && len(m.data) >= m.maxSize && m.data[key] == nil {
		return 0, ErrMaxSizeExceeded
	}

	// Set new value
	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	m.data[key] = &entry{
		Data:      value,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	return value, nil
}

// Delete removes a key from the backend
func (m *memoryBackend) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return backend.ErrBackendClosed
	}

	delete(m.data, key)
	return nil
}

// Update atomically reads, transforms, and writes the state for a key.
// The entire operation runs under a single write lock, ensuring no
// concurrent access can interleave.
func (m *memoryBackend) Update(ctx context.Context, key string, ttl time.Duration, fn backend.UpdateFunc) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, backend.ErrBackendClosed
	}

	// Read current state
	var current interface{}
	if ent, exists := m.data[key]; exists {
		if !ent.ExpiresAt.IsZero() && time.Now().After(ent.ExpiresAt) {
			// Key expired — treat as missing
			delete(m.data, key)
			current = nil
		} else {
			current = ent.Data
		}
	}

	// Execute the transform function
	newState, result, err := fn(current)
	if err != nil {
		return nil, err
	}

	// Write back
	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	if m.maxSize > 0 && len(m.data) >= m.maxSize && m.data[key] == nil {
		return nil, ErrMaxSizeExceeded
	}

	m.data[key] = &entry{
		Data:      newState,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	return result, nil
}

// Close cleans up resources and prevents further operations
func (m *memoryBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.closed && m.gcDone != nil {
		close(m.gcDone)
	}

	m.closed = true
	m.data = make(map[string]*entry)
	return nil
}

// Ping checks if backend is healthy
func (m *memoryBackend) Ping(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return backend.ErrBackendClosed
	}

	return nil
}

// Type returns the backend type name
func (m *memoryBackend) Type() string {
	return "memory"
}

// GetSize returns number of stored keys
func (m *memoryBackend) GetSize(ctx context.Context) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return 0, backend.ErrBackendClosed
	}

	return len(m.data), nil
}

// Clear removes all keys
func (m *memoryBackend) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return backend.ErrBackendClosed
	}

	m.data = make(map[string]*entry)
	return nil
}

// GetMemoryUsage returns approximate memory usage in bytes
func (m *memoryBackend) GetMemoryUsage(ctx context.Context) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return 0, backend.ErrBackendClosed
	}

	var totalSize int64

	// Approximate memory usage
	// This is a rough estimate and not guaranteed to be accurate
	for key, ent := range m.data {
		// Key size
		totalSize += int64(len(key))

		// Entry overhead
		totalSize += int64(unsafe.Sizeof(*ent))

		// Value size estimation
		if ent.Data != nil {
			switch v := ent.Data.(type) {
			case string:
				totalSize += int64(len(v))
			case []byte:
				totalSize += int64(len(v))
			case map[string]interface{}:
				totalSize += int64(len(v)) * 16 // rough estimate
			case int64:
				totalSize += 8
			default:
				totalSize += 8 // base estimate
			}
		}
	}

	return totalSize, nil
}

// SetMaxSize sets maximum number of keys to store
func (m *memoryBackend) SetMaxSize(ctx context.Context, maxSize int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return backend.ErrBackendClosed
	}

	// Check if current size exceeds new max
	if maxSize > 0 && len(m.data) > maxSize {
		return errors.New("current size exceeds new max size")
	}

	m.maxSize = maxSize
	return nil
}
