package memory

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abhipray-cpu/niyantrak/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemoryBackend_NewMemoryBackend tests constructor
func TestMemoryBackend_NewMemoryBackend(t *testing.T) {
	mb := NewMemoryBackend()
	assert.NotNil(t, mb)

	ctx := context.Background()
	size, err := mb.GetSize(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, size)
}

// TestMemoryBackend_Set_Get tests basic Set and Get operations
func TestMemoryBackend_Set_Get(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	state := map[string]interface{}{
		"tokens":     100,
		"lastRefill": time.Now(),
	}

	// Set value
	err := mb.Set(ctx, "tenant:acme", state, 1*time.Hour)
	require.NoError(t, err)

	// Get value
	result, err := mb.Get(ctx, "tenant:acme")
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify it's a map
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 100, resultMap["tokens"])
}

// TestMemoryBackend_Get_NonExistent tests Get with non-existent key
func TestMemoryBackend_Get_NonExistent(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	result, err := mb.Get(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, backend.ErrKeyNotFound))
}

// TestMemoryBackend_Delete tests Delete operation
func TestMemoryBackend_Delete(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	// Set value
	mb.Set(ctx, "key1", "value1", 1*time.Hour)

	// Delete it
	err := mb.Delete(ctx, "key1")
	require.NoError(t, err)

	// Try to get it
	result, err := mb.Get(ctx, "key1")
	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestMemoryBackend_Delete_NonExistent tests Delete with non-existent key
func TestMemoryBackend_Delete_NonExistent(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	// Deleting non-existent key should not error
	err := mb.Delete(ctx, "nonexistent")
	require.NoError(t, err)
}

// TestMemoryBackend_TTL_Expiration tests TTL and expiration
func TestMemoryBackend_TTL_Expiration(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Set with short TTL
	err := mb.Set(ctx, "temp", "data", 100*time.Millisecond)
	require.NoError(t, err)

	// Should exist immediately
	result, err := mb.Get(ctx, "temp")
	require.NoError(t, err)
	assert.Equal(t, "data", result)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	result, err = mb.Get(ctx, "temp")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, backend.ErrKeyExpired))
}

// TestMemoryBackend_TTL_ZeroMeansNoExpiration tests TTL of 0
func TestMemoryBackend_TTL_ZeroMeansNoExpiration(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Set with TTL of 0 (no expiration)
	err := mb.Set(ctx, "permanent", "data", 0)
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// Should still exist
	result, err := mb.Get(ctx, "permanent")
	require.NoError(t, err)
	assert.Equal(t, "data", result)
}

// TestMemoryBackend_IncrementAndGet tests atomic increment
func TestMemoryBackend_IncrementAndGet(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// First increment (key doesn't exist, should start at 0)
	val1, err := mb.IncrementAndGet(ctx, "counter", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val1)

	// Second increment
	val2, err := mb.IncrementAndGet(ctx, "counter", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(2), val2)

	// Third increment
	val3, err := mb.IncrementAndGet(ctx, "counter", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(3), val3)
}

// TestMemoryBackend_IncrementAndGet_WithTTL tests IncrementAndGet respects TTL
func TestMemoryBackend_IncrementAndGet_WithTTL(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Set with short TTL
	val1, err := mb.IncrementAndGet(ctx, "tempCounter", 100*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val1)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should reset to 1 (since key expired)
	val2, err := mb.IncrementAndGet(ctx, "tempCounter", 100*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val2)
}

// TestMemoryBackend_GetSize tests GetSize method
func TestMemoryBackend_GetSize(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	size, err := mb.GetSize(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, size)

	// Add some keys
	mb.Set(ctx, "key1", "value1", 1*time.Hour)
	mb.Set(ctx, "key2", "value2", 1*time.Hour)

	size, err = mb.GetSize(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, size)
}

// TestMemoryBackend_Clear tests Clear method
func TestMemoryBackend_Clear(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Add some keys
	mb.Set(ctx, "key1", "value1", 1*time.Hour)
	mb.Set(ctx, "key2", "value2", 1*time.Hour)

	size, _ := mb.GetSize(ctx)
	assert.Equal(t, 2, size)

	// Clear all
	err := mb.Clear(ctx)
	require.NoError(t, err)

	size, _ = mb.GetSize(ctx)
	assert.Equal(t, 0, size)
}

// TestMemoryBackend_Type tests Type method
func TestMemoryBackend_Type(t *testing.T) {
	mb := NewMemoryBackend()

	assert.Equal(t, "memory", mb.Type())
}

// TestMemoryBackend_Ping tests Ping method
func TestMemoryBackend_Ping(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	err := mb.Ping(ctx)
	require.NoError(t, err)
}

// TestMemoryBackend_Close tests Close method
func TestMemoryBackend_Close(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Add some keys
	mb.Set(ctx, "key1", "value1", 1*time.Hour)

	// Close
	err := mb.Close()
	require.NoError(t, err)

	// Should not be able to get keys after close
	_, err = mb.Get(ctx, "key1")
	assert.Error(t, err)
}

// TestMemoryBackend_ConcurrentWrites tests concurrent writes with -race flag
func TestMemoryBackend_ConcurrentWrites(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()
	numGoroutines := 100

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			key := "concurrent_write_" + string(rune(index))
			value := map[string]interface{}{"index": index}
			mb.Set(ctx, key, value, 1*time.Hour)
		}(i)
	}

	wg.Wait()

	size, _ := mb.GetSize(ctx)
	assert.Equal(t, numGoroutines, size)
}

// TestMemoryBackend_ConcurrentReads tests concurrent reads with -race flag
func TestMemoryBackend_ConcurrentReads(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Set a value
	mb.Set(ctx, "shared_key", "shared_value", 1*time.Hour)

	// Read concurrently
	numGoroutines := 100
	var wg sync.WaitGroup
	successCount := int32(0)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := mb.Get(ctx, "shared_key")
			if err == nil && result == "shared_value" {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, int32(numGoroutines), atomic.LoadInt32(&successCount))
}

// TestMemoryBackend_ConcurrentReadWrite tests concurrent read/write mix
func TestMemoryBackend_ConcurrentReadWrite(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	numGoroutines := 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Write
			key := "key_" + string(rune(index%10))
			mb.Set(ctx, key, index, 1*time.Hour)

			// Read
			mb.Get(ctx, key)

			// Delete
			mb.Delete(ctx, key)
		}(i)
	}

	wg.Wait()
	// If no race condition detected, test passes
}

// TestMemoryBackend_ConcurrentIncrement tests concurrent IncrementAndGet
func TestMemoryBackend_ConcurrentIncrement(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	numGoroutines := 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mb.IncrementAndGet(ctx, "atomic_counter", 1*time.Hour)
		}()
	}

	wg.Wait()

	final, _ := mb.Get(ctx, "atomic_counter")
	assert.Equal(t, int64(numGoroutines), final)
}

// TestMemoryBackend_SetMaxSize tests SetMaxSize method
func TestMemoryBackend_SetMaxSize(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Set max size to 2
	err := mb.SetMaxSize(ctx, 2)
	require.NoError(t, err)

	// Add 2 keys
	mb.Set(ctx, "key1", "value1", 1*time.Hour)
	mb.Set(ctx, "key2", "value2", 1*time.Hour)

	// Try to add 3rd key - should error
	err = mb.Set(ctx, "key3", "value3", 1*time.Hour)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMaxSizeExceeded))
}

// TestMemoryBackend_GetMemoryUsage tests GetMemoryUsage method
func TestMemoryBackend_GetMemoryUsage(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	usage1, err := mb.GetMemoryUsage(ctx)
	require.NoError(t, err)
	// Empty backend may return 0
	assert.GreaterOrEqual(t, usage1, int64(0))

	// Add some data
	mb.Set(ctx, "key1", "a very long string value that takes up memory", 1*time.Hour)

	usage2, err := mb.GetMemoryUsage(ctx)
	require.NoError(t, err)
	assert.Greater(t, usage2, usage1)
	assert.Greater(t, usage2, usage1)
}

// TestMemoryBackend_OverwriteKey tests overwriting existing key
func TestMemoryBackend_OverwriteKey(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Set initial value
	mb.Set(ctx, "key", "value1", 1*time.Hour)

	// Overwrite
	mb.Set(ctx, "key", "value2", 1*time.Hour)

	// Get should return new value
	result, _ := mb.Get(ctx, "key")
	assert.Equal(t, "value2", result)
}

// TestMemoryBackend_EmptyKey tests behavior with empty key
func TestMemoryBackend_EmptyKey(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Empty key should work (allow any string, even empty)
	err := mb.Set(ctx, "", "value", 1*time.Hour)
	require.NoError(t, err)

	result, err := mb.Get(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "value", result)
}

// TestMemoryBackend_NilValue tests setting nil value
func TestMemoryBackend_NilValue(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Setting nil should work
	err := mb.Set(ctx, "key", nil, 1*time.Hour)
	require.NoError(t, err)

	result, err := mb.Get(ctx, "key")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestMemoryBackend_UpdateTTL tests updating TTL of existing key
func TestMemoryBackend_UpdateTTL(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Set with short TTL
	mb.Set(ctx, "key", "value", 100*time.Millisecond)

	// Wait 80ms
	time.Sleep(80 * time.Millisecond)

	// Update with longer TTL
	mb.Set(ctx, "key", "value", 200*time.Millisecond)

	// Wait another 100ms (total 180ms from original set)
	time.Sleep(100 * time.Millisecond)

	// Should still exist because we updated TTL
	result, err := mb.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "value", result)
}

// TestMemoryBackend_LargeValue tests storing large values
func TestMemoryBackend_LargeValue(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	// Create large byte array
	largeValue := make([]byte, 1024*1024) // 1MB
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	err := mb.Set(ctx, "large", largeValue, 1*time.Hour)
	require.NoError(t, err)

	result, err := mb.Get(ctx, "large")
	require.NoError(t, err)
	assert.Equal(t, largeValue, result)
}

// TestMemoryBackend_ManyKeys tests storing many keys
func TestMemoryBackend_ManyKeys(t *testing.T) {
	mb := NewMemoryBackend()

	ctx := context.Background()

	numKeys := 10000
	for i := 0; i < numKeys; i++ {
		key := "key_" + string(rune(i))
		mb.Set(ctx, key, i, 1*time.Hour)
	}

	size, _ := mb.GetSize(ctx)
	assert.Equal(t, numKeys, size)
}

// TestMemoryBackend_ContextCancellation tests behavior with cancelled context
func TestMemoryBackend_ContextCancellation(t *testing.T) {
	mb := NewMemoryBackend()

	ctx, cancel := context.WithCancel(context.Background())

	// Set should work before cancel
	err := mb.Set(ctx, "key", "value", 1*time.Hour)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Operations should still work (memory backend doesn't depend on context)
	// but context is cancelled
	err = mb.Set(ctx, "key2", "value2", 1*time.Hour)
	// Memory backend should still work even if context is cancelled
	require.NoError(t, err)
}

// BenchmarkMemoryBackend_Set benchmarks Set operation
func BenchmarkMemoryBackend_Set(b *testing.B) {
	mb := NewMemoryBackend()
	ctx := context.Background()
	state := map[string]interface{}{"tokens": 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mb.Set(ctx, "key", state, 1*time.Hour)
	}
}

// BenchmarkMemoryBackend_Get benchmarks Get operation
func BenchmarkMemoryBackend_Get(b *testing.B) {
	mb := NewMemoryBackend()
	ctx := context.Background()
	state := map[string]interface{}{"tokens": 100}
	mb.Set(ctx, "key", state, 1*time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mb.Get(ctx, "key")
	}
}

// BenchmarkMemoryBackend_IncrementAndGet benchmarks IncrementAndGet operation
func BenchmarkMemoryBackend_IncrementAndGet(b *testing.B) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mb.IncrementAndGet(ctx, "counter", 1*time.Hour)
	}
}

// BenchmarkMemoryBackend_Delete benchmarks Delete operation
func BenchmarkMemoryBackend_Delete(b *testing.B) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "key_" + string(rune(i%1000))
		mb.Set(ctx, key, "value", 1*time.Hour)
		mb.Delete(ctx, key)
	}
}
