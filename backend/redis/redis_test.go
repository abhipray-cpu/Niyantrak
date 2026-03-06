package redis

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

// TestRedisBackend_NewRedisBackend tests constructor
func TestRedisBackend_NewRedisBackend(t *testing.T) {
	// Skip if Redis not available
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	assert.NotNil(t, rb)

	ctx := context.Background()
	err := rb.Ping(ctx)
	require.NoError(t, err)
}

// TestRedisBackend_Set_Get tests basic Set and Get operations
func TestRedisBackend_Set_Get(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	state := map[string]interface{}{
		"tokens":     100,
		"lastRefill": time.Now(),
	}

	// Set value
	err := rb.Set(ctx, "tenant:acme", state, 1*time.Hour)
	require.NoError(t, err)

	// Get value
	result, err := rb.Get(ctx, "tenant:acme")
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify it's a map
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	// JSON roundtrip turns numbers into float64
	assert.Equal(t, float64(100), resultMap["tokens"])
}

// TestRedisBackend_Get_NonExistent tests Get with non-existent key
func TestRedisBackend_Get_NonExistent(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	result, err := rb.Get(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, backend.ErrKeyNotFound))
}

// TestRedisBackend_Delete tests Delete operation
func TestRedisBackend_Delete(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	rb.Set(ctx, "key1", "value1", 1*time.Hour)

	// Delete it
	err := rb.Delete(ctx, "key1")
	require.NoError(t, err)

	// Try to get it
	result, err := rb.Get(ctx, "key1")
	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestRedisBackend_Delete_NonExistent tests Delete with non-existent key
func TestRedisBackend_Delete_NonExistent(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	err := rb.Delete(ctx, "nonexistent")
	require.NoError(t, err)
}

// TestRedisBackend_TTL_Expiration tests TTL and expiration
func TestRedisBackend_TTL_Expiration(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()

	// Redis minimum TTL is 1s; use a longer TTL to avoid truncation.
	err := rb.Set(ctx, "temp", "data", 2*time.Second)
	require.NoError(t, err)

	// Should exist immediately
	result, err := rb.Get(ctx, "temp")
	require.NoError(t, err)
	assert.Equal(t, "data", result)

	// Wait for expiration
	time.Sleep(3 * time.Second)

	// Should be expired
	result, err = rb.Get(ctx, "temp")
	assert.Error(t, err)
	assert.Nil(t, result)
	// Redis auto-deletes expired keys, so we may get ErrKeyNotFound instead of ErrKeyExpired
	assert.True(t, errors.Is(err, backend.ErrKeyExpired) || errors.Is(err, backend.ErrKeyNotFound))
}

// TestRedisBackend_TTL_ZeroMeansNoExpiration tests TTL of 0
func TestRedisBackend_TTL_ZeroMeansNoExpiration(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()

	// Set with TTL of 0 (no expiration)
	err := rb.Set(ctx, "permanent", "data", 0)
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// Should still exist
	result, err := rb.Get(ctx, "permanent")
	require.NoError(t, err)
	assert.Equal(t, "data", result)
}

// TestRedisBackend_IncrementAndGet tests atomic increment
func TestRedisBackend_IncrementAndGet(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()

	// First increment (key doesn't exist, should start at 0)
	val1, err := rb.IncrementAndGet(ctx, "counter", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val1)

	// Second increment
	val2, err := rb.IncrementAndGet(ctx, "counter", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(2), val2)

	// Third increment
	val3, err := rb.IncrementAndGet(ctx, "counter", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(3), val3)
}

// TestRedisBackend_IncrementAndGet_WithTTL tests IncrementAndGet respects TTL
func TestRedisBackend_IncrementAndGet_WithTTL(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()

	// Redis minimum TTL is 1s; use a longer TTL to avoid truncation.
	val1, err := rb.IncrementAndGet(ctx, "tempCounter", 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val1)

	// Wait for expiration
	time.Sleep(3 * time.Second)

	// Should reset to 1 (since key expired)
	val2, err := rb.IncrementAndGet(ctx, "tempCounter", 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val2)
}

// TestRedisBackend_Type tests Type method
func TestRedisBackend_Type(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	assert.Equal(t, "redis", rb.Type())
}

// TestRedisBackend_Ping tests Ping method
func TestRedisBackend_Ping(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	err := rb.Ping(ctx)
	require.NoError(t, err)
}

// TestRedisBackend_Close tests Close method
func TestRedisBackend_Close(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	rb.Set(context.Background(), "key1", "value1", 1*time.Hour)

	// Close
	err := rb.Close()
	require.NoError(t, err)

	// Should not be able to use after close
	ctx := context.Background()
	err = rb.Ping(ctx)
	assert.Error(t, err)
}

// TestRedisBackend_ConcurrentWrites tests concurrent writes with -race flag
func TestRedisBackend_ConcurrentWrites(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	numGoroutines := 100

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			key := "concurrent_write_" + string(rune(index))
			value := map[string]interface{}{"index": index}
			rb.Set(ctx, key, value, 1*time.Hour)
		}(i)
	}

	wg.Wait()
}

// TestRedisBackend_ConcurrentReads tests concurrent reads with -race flag
func TestRedisBackend_ConcurrentReads(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	rb.Set(ctx, "shared_key", "shared_value", 1*time.Hour)

	// Read concurrently
	numGoroutines := 100
	var wg sync.WaitGroup
	successCount := int32(0)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := rb.Get(ctx, "shared_key")
			if err == nil && result == "shared_value" {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int32(numGoroutines), atomic.LoadInt32(&successCount))
}

// TestRedisBackend_ConcurrentReadWrite tests concurrent read/write mix
func TestRedisBackend_ConcurrentReadWrite(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	numGoroutines := 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			key := "key_" + string(rune(index%10))
			rb.Set(ctx, key, index, 1*time.Hour)
			rb.Get(ctx, key)
			rb.Delete(ctx, key)
		}(i)
	}

	wg.Wait()
}

// TestRedisBackend_ConcurrentIncrement tests concurrent IncrementAndGet
func TestRedisBackend_ConcurrentIncrement(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	numGoroutines := 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.IncrementAndGet(ctx, "atomic_counter", 1*time.Hour)
		}()
	}

	wg.Wait()

	// IncrementAndGet uses Redis INCR (not Wrap/Unwrap), but Get goes through
	// Unwrap which does json.Unmarshal into interface{}, returning float64 for numbers.
	final, _ := rb.Get(ctx, "atomic_counter")
	assert.Equal(t, float64(numGoroutines), final)
}

// TestRedisBackend_OverwriteKey tests overwriting existing key
func TestRedisBackend_OverwriteKey(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	rb.Set(ctx, "key", "value1", 1*time.Hour)
	rb.Set(ctx, "key", "value2", 1*time.Hour)

	result, _ := rb.Get(ctx, "key")
	assert.Equal(t, "value2", result)
}

// TestRedisBackend_EmptyKey tests behavior with empty key
func TestRedisBackend_EmptyKey(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	err := rb.Set(ctx, "", "value", 1*time.Hour)
	require.NoError(t, err)

	result, err := rb.Get(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "value", result)
}

// TestRedisBackend_NilValue tests setting nil value
func TestRedisBackend_NilValue(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	err := rb.Set(ctx, "key", nil, 1*time.Hour)
	require.NoError(t, err)

	result, err := rb.Get(ctx, "key")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestRedisBackend_UpdateTTL tests updating TTL of existing key
func TestRedisBackend_UpdateTTL(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	// Redis minimum TTL is 1s; use second-level values.
	rb.Set(ctx, "key", "value", 2*time.Second)

	// Wait 1s
	time.Sleep(1 * time.Second)

	// Update with longer TTL
	rb.Set(ctx, "key", "value", 3*time.Second)

	// Wait another 2s (total 3s from original set — original would have expired)
	time.Sleep(2 * time.Second)

	// Should still exist because we updated TTL
	result, err := rb.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "value", result)
}

// TestRedisBackend_LargeValue tests storing large values
func TestRedisBackend_LargeValue(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	largeValue := make([]byte, 1024*1024) // 1MB
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	err := rb.Set(ctx, "large", largeValue, 1*time.Hour)
	require.NoError(t, err)

	result, err := rb.Get(ctx, "large")
	require.NoError(t, err)
	// []byte is marshalled to a base64-encoded JSON string; Unwrap returns it as a string.
	assert.NotNil(t, result)
	resultStr, ok := result.(string)
	require.True(t, ok, "expected string from JSON roundtrip of []byte")
	assert.NotEmpty(t, resultStr)
}

// TestRedisBackend_ManyKeys tests storing many keys
func TestRedisBackend_ManyKeys(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	numKeys := 1000
	for i := 0; i < numKeys; i++ {
		key := "key_" + string(rune(i))
		rb.Set(ctx, key, i, 1*time.Hour)
	}

	// Verify we can scan keys
	keys, err := rb.Scan(ctx, "key_*", 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(keys), numKeys/2) // At least half should be there
}

// TestRedisBackend_ContextCancellation tests behavior with cancelled context
func TestRedisBackend_ContextCancellation(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	err := rb.Set(ctx, "key", "value", 1*time.Hour)
	require.NoError(t, err)

	cancel()

	// Operations with cancelled context should fail
	_, err = rb.Get(ctx, "key")
	assert.Error(t, err)
}

// TestRedisBackend_GetConnection tests GetConnection method
func TestRedisBackend_GetConnection(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	connInfo := rb.GetConnection(ctx)
	assert.NotNil(t, connInfo)
	assert.Equal(t, "localhost", connInfo.Host)
	assert.Equal(t, 6379, connInfo.Port)
}

// TestRedisBackend_CheckCluster tests CheckCluster method
func TestRedisBackend_CheckCluster(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	isCluster, err := rb.CheckCluster(ctx)
	require.NoError(t, err)
	assert.False(t, isCluster) // Local Redis usually not a cluster
}

// TestRedisBackend_Flush tests Flush method
func TestRedisBackend_Flush(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	rb.Set(ctx, "key1", "value1", 1*time.Hour)

	err := rb.Flush(ctx)
	require.NoError(t, err)

	_, err = rb.Get(ctx, "key1")
	assert.Error(t, err)
}

// BenchmarkRedisBackend_Set benchmarks Set operation
func BenchmarkRedisBackend_Set(b *testing.B) {
	if !isRedisAvailable() {
		b.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	state := map[string]interface{}{"tokens": 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Set(ctx, "key", state, 1*time.Hour)
	}
}

// BenchmarkRedisBackend_Get benchmarks Get operation
func BenchmarkRedisBackend_Get(b *testing.B) {
	if !isRedisAvailable() {
		b.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()
	state := map[string]interface{}{"tokens": 100}
	rb.Set(ctx, "key", state, 1*time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Get(ctx, "key")
	}
}

// BenchmarkRedisBackend_IncrementAndGet benchmarks IncrementAndGet operation
func BenchmarkRedisBackend_IncrementAndGet(b *testing.B) {
	if !isRedisAvailable() {
		b.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.IncrementAndGet(ctx, "counter", 1*time.Hour)
	}
}

// BenchmarkRedisBackend_Delete benchmarks Delete operation
func BenchmarkRedisBackend_Delete(b *testing.B) {
	if !isRedisAvailable() {
		b.Skip("Redis not available")
	}

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "key_" + string(rune(i%1000))
		rb.Set(ctx, key, "value", 1*time.Hour)
		rb.Delete(ctx, key)
	}
}

// isRedisAvailable checks if Redis is available for testing
func isRedisAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rb := NewRedisBackend("localhost:6379", 0, "")
	defer rb.Close()

	return rb.Ping(ctx) == nil
}
