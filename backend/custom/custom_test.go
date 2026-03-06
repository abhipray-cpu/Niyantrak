package custom

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abhipray-cpu/niyantrak/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCustomBackend_NewCustomBackend tests constructor
func TestCustomBackend_NewCustomBackend(t *testing.T) {
	cb := NewCustomBackend("test_custom", map[string]interface{}{
		"max_size": 1000,
		"ttl":      60,
	})
	defer cb.Close()

	assert.NotNil(t, cb)
	assert.Equal(t, "custom", cb.Type())
}

// TestCustomBackend_Set_Get tests basic Set and Get operations
func TestCustomBackend_Set_Get(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Set a simple value
	err := cb.Set(ctx, "key1", "value1", 0)
	require.NoError(t, err)

	// Get the value
	val, err := cb.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// Set a complex value
	complexVal := map[string]interface{}{
		"count":     10,
		"timestamp": time.Now().Unix(),
	}
	err = cb.Set(ctx, "key2", complexVal, 0)
	require.NoError(t, err)

	// Get the complex value
	val, err = cb.Get(ctx, "key2")
	require.NoError(t, err)

	// Unmarshal and verify
	jsonBytes, _ := json.Marshal(val)
	var retrieved map[string]interface{}
	err = json.Unmarshal(jsonBytes, &retrieved)
	require.NoError(t, err)
	assert.Equal(t, float64(10), retrieved["count"])
}

// TestCustomBackend_Get_NonExistent tests getting non-existent key
func TestCustomBackend_Get_NonExistent(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()
	val, err := cb.Get(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Nil(t, val)
	assert.True(t, errors.Is(err, backend.ErrKeyNotFound) || err.Error() == "key not found")
}

// TestCustomBackend_Delete tests Delete operation
func TestCustomBackend_Delete(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Set a value
	err := cb.Set(ctx, "key1", "value1", 0)
	require.NoError(t, err)

	// Delete the value
	err = cb.Delete(ctx, "key1")
	require.NoError(t, err)

	// Verify it's deleted
	val, err := cb.Get(ctx, "key1")
	assert.Error(t, err)
	assert.Nil(t, val)
}

// TestCustomBackend_Delete_NonExistent tests deleting non-existent key
func TestCustomBackend_Delete_NonExistent(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Deleting non-existent key should not error
	err := cb.Delete(ctx, "nonexistent")
	assert.NoError(t, err)
}

// TestCustomBackend_TTL_Expiration tests TTL expiration
func TestCustomBackend_TTL_Expiration(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Set a value with short TTL
	err := cb.Set(ctx, "expiring_key", "value", 100*time.Millisecond)
	require.NoError(t, err)

	// Get immediately should work
	val, err := cb.Get(ctx, "expiring_key")
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	val, err = cb.Get(ctx, "expiring_key")
	assert.Error(t, err)
	assert.Nil(t, val)
}

// TestCustomBackend_TTL_ZeroMeansNoExpiration tests that TTL=0 means no expiration
func TestCustomBackend_TTL_ZeroMeansNoExpiration(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Set a value with TTL=0
	err := cb.Set(ctx, "persistent_key", "value", 0)
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Should still exist
	val, err := cb.Get(ctx, "persistent_key")
	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

// TestCustomBackend_IncrementAndGet tests atomic increment
func TestCustomBackend_IncrementAndGet(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// First increment should return 1
	val, err := cb.IncrementAndGet(ctx, "counter", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val)

	// Second increment should return 2
	val, err = cb.IncrementAndGet(ctx, "counter", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), val)

	// Third increment should return 3
	val, err = cb.IncrementAndGet(ctx, "counter", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), val)
}

// TestCustomBackend_IncrementAndGet_WithTTL tests atomic increment with TTL
func TestCustomBackend_IncrementAndGet_WithTTL(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Increment with TTL
	val, err := cb.IncrementAndGet(ctx, "counter_ttl", 200*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val)

	// Should still be accessible
	val, err = cb.IncrementAndGet(ctx, "counter_ttl", 200*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, int64(2), val)

	// Wait for expiration
	time.Sleep(250 * time.Millisecond)

	// Should start from 1 again
	val, err = cb.IncrementAndGet(ctx, "counter_ttl", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val)
}

// TestCustomBackend_Type tests Type method
func TestCustomBackend_Type(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	assert.Equal(t, "custom", cb.Type())
}

// TestCustomBackend_Ping tests Ping method
func TestCustomBackend_Ping(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()
	err := cb.Ping(ctx)
	assert.NoError(t, err)
}

// TestCustomBackend_Close tests Close method
func TestCustomBackend_Close(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)

	ctx := context.Background()

	// Set a value first
	err := cb.Set(ctx, "key", "value", 0)
	require.NoError(t, err)

	// Close should work
	err = cb.Close()
	assert.NoError(t, err)

	// Operations after close should fail
	err = cb.Set(ctx, "key", "value", 0)
	assert.Error(t, err)
}

// TestCustomBackend_ConcurrentWrites tests concurrent write operations
func TestCustomBackend_ConcurrentWrites(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "key_" + string(rune(id))
			err := cb.Set(ctx, key, id, 0)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
}

// TestCustomBackend_ConcurrentReads tests concurrent read operations
func TestCustomBackend_ConcurrentReads(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Set initial value
	err := cb.Set(ctx, "shared_key", "shared_value", 0)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := cb.Get(ctx, "shared_key")
			assert.NoError(t, err)
			assert.Equal(t, "shared_value", val)
		}()
	}

	wg.Wait()
}

// TestCustomBackend_ConcurrentReadWrite tests concurrent read/write operations
func TestCustomBackend_ConcurrentReadWrite(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 50

	// Writers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "rw_key"
			err := cb.Set(ctx, key, id, 0)
			assert.NoError(t, err)
		}(i)
	}

	// Readers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cb.Get(ctx, "rw_key")
		}()
	}

	wg.Wait()
}

// TestCustomBackend_ConcurrentIncrement tests concurrent increment operations
func TestCustomBackend_ConcurrentIncrement(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 100
	var counter int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := cb.IncrementAndGet(ctx, "concurrent_counter", 0)
			assert.NoError(t, err)
			assert.Greater(t, val, int64(0))
			atomic.AddInt64(&counter, 1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(numGoroutines), counter)

	// Final value should be numGoroutines
	finalVal, err := cb.Get(ctx, "concurrent_counter")
	require.NoError(t, err)
	assert.Equal(t, int64(numGoroutines), finalVal)
}

// TestCustomBackend_OverwriteKey tests overwriting existing key
func TestCustomBackend_OverwriteKey(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Set initial value
	err := cb.Set(ctx, "key", "value1", 0)
	require.NoError(t, err)

	// Overwrite
	err = cb.Set(ctx, "key", "value2", 0)
	require.NoError(t, err)

	// Get should return new value
	val, err := cb.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "value2", val)
}

// TestCustomBackend_EmptyKey tests empty key handling
func TestCustomBackend_EmptyKey(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Empty key should work (though not recommended)
	err := cb.Set(ctx, "", "value", 0)
	assert.NoError(t, err)

	val, err := cb.Get(ctx, "")
	assert.NoError(t, err)
	assert.Equal(t, "value", val)
}

// TestCustomBackend_NilValue tests nil value handling
func TestCustomBackend_NilValue(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Nil value should be handled
	err := cb.Set(ctx, "nil_key", nil, 0)
	assert.NoError(t, err)

	val, err := cb.Get(ctx, "nil_key")
	assert.NoError(t, err)
	assert.Nil(t, val)
}

// TestCustomBackend_UpdateTTL tests updating TTL for existing key
func TestCustomBackend_UpdateTTL(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Set with short TTL
	err := cb.Set(ctx, "key", "value", 100*time.Millisecond)
	require.NoError(t, err)

	// Update with longer TTL
	err = cb.Set(ctx, "key", "value", 1*time.Second)
	require.NoError(t, err)

	// Wait past original TTL
	time.Sleep(150 * time.Millisecond)

	// Should still exist
	val, err := cb.Get(ctx, "key")
	assert.NoError(t, err)
	assert.Equal(t, "value", val)
}

// TestCustomBackend_LargeValue tests storing large values
func TestCustomBackend_LargeValue(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Create a 1MB value
	largeValue := make([]byte, 1024*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	err := cb.Set(ctx, "large_key", largeValue, 0)
	assert.NoError(t, err)

	val, err := cb.Get(ctx, "large_key")
	assert.NoError(t, err)
	assert.NotNil(t, val)
}

// TestCustomBackend_ManyKeys tests storing many keys
func TestCustomBackend_ManyKeys(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	numKeys := 1000
	for i := 0; i < numKeys; i++ {
		key := "key_" + string(rune(i))
		err := cb.Set(ctx, key, i, 0)
		assert.NoError(t, err)
	}

	// Verify some keys
	val, err := cb.Get(ctx, "key_"+string(rune(0)))
	assert.NoError(t, err)
	assert.NotNil(t, val)
}

// TestCustomBackend_ContextCancellation tests context cancellation
func TestCustomBackend_ContextCancellation(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Operations with cancelled context should handle gracefully
	// (Custom backend may or may not check context depending on implementation)
	_ = cb.Set(ctx, "key", "value", 0)
}

// TestCustomBackend_GetMetadata tests GetMetadata method
func TestCustomBackend_GetMetadata(t *testing.T) {
	cb := NewCustomBackend("test_custom", map[string]interface{}{
		"max_size": 1000,
		"version":  "1.0.0",
	})
	defer cb.Close()

	ctx := context.Background()
	metadata := cb.GetMetadata(ctx)

	assert.NotNil(t, metadata)
	assert.Contains(t, metadata, "name")
	assert.Contains(t, metadata, "config")
	assert.Equal(t, "test_custom", metadata["name"])
}

// TestCustomBackend_Execute tests Execute method
func TestCustomBackend_Execute(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Test "count" operation
	result, err := cb.Execute(ctx, "count", nil)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Test "clear" operation
	cb.Set(ctx, "key1", "value1", 0)
	cb.Set(ctx, "key2", "value2", 0)

	result, err = cb.Execute(ctx, "clear", nil)
	require.NoError(t, err)
	assert.Equal(t, "cleared", result)

	// Verify keys are cleared
	_, err = cb.Get(ctx, "key1")
	assert.Error(t, err)

	// Test "stats" operation
	result, err = cb.Execute(ctx, "stats", nil)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Test unknown operation
	_, err = cb.Execute(ctx, "unknown", nil)
	assert.Error(t, err)
}

// TestCustomBackend_ExecuteWithArgs tests Execute method with arguments
func TestCustomBackend_ExecuteWithArgs(t *testing.T) {
	cb := NewCustomBackend("test_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	// Set some values
	cb.Set(ctx, "prefix_key1", "value1", 0)
	cb.Set(ctx, "prefix_key2", "value2", 0)
	cb.Set(ctx, "other_key", "value3", 0)

	// Test "list" operation with prefix argument
	result, err := cb.Execute(ctx, "list", map[string]interface{}{
		"prefix": "prefix_",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Result should be a slice
	keys, ok := result.([]string)
	assert.True(t, ok)
	assert.GreaterOrEqual(t, len(keys), 2)
}

// Benchmark tests
func BenchmarkCustomBackend_Set(b *testing.B) {
	cb := NewCustomBackend("bench_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Set(ctx, "bench_key", i, 0)
	}
}

func BenchmarkCustomBackend_Get(b *testing.B) {
	cb := NewCustomBackend("bench_custom", nil)
	defer cb.Close()

	ctx := context.Background()
	_ = cb.Set(ctx, "bench_key", "value", 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cb.Get(ctx, "bench_key")
	}
}

func BenchmarkCustomBackend_IncrementAndGet(b *testing.B) {
	cb := NewCustomBackend("bench_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cb.IncrementAndGet(ctx, "bench_counter", 0)
	}
}

func BenchmarkCustomBackend_Delete(b *testing.B) {
	cb := NewCustomBackend("bench_custom", nil)
	defer cb.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		_ = cb.Set(ctx, "bench_key", "value", 0)
		b.StartTimer()
		_ = cb.Delete(ctx, "bench_key")
	}
}
