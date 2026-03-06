package postgresql

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

// TestPostgreSQLBackend_NewPostgreSQLBackend tests constructor
func TestPostgreSQLBackend_NewPostgreSQLBackend(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	assert.NotNil(t, pb)
	assert.Equal(t, "postgresql", pb.Type())
}

// TestPostgreSQLBackend_CreateTable tests table creation
func TestPostgreSQLBackend_CreateTable(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)
}

// TestPostgreSQLBackend_Set_Get tests basic Set and Get operations
func TestPostgreSQLBackend_Set_Get(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Set a simple value
	err = pb.Set(ctx, "key1", "value1", 0)
	require.NoError(t, err)

	// Get the value
	val, err := pb.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// Set a complex value
	complexVal := map[string]interface{}{
		"count":     10,
		"timestamp": time.Now().Unix(),
	}
	err = pb.Set(ctx, "key2", complexVal, 0)
	require.NoError(t, err)

	// Get the complex value
	val, err = pb.Get(ctx, "key2")
	require.NoError(t, err)

	// Unmarshal and verify
	jsonBytes, _ := json.Marshal(val)
	var retrieved map[string]interface{}
	err = json.Unmarshal(jsonBytes, &retrieved)
	require.NoError(t, err)
	assert.Equal(t, float64(10), retrieved["count"])
}

// TestPostgreSQLBackend_Get_NonExistent tests getting non-existent key
func TestPostgreSQLBackend_Get_NonExistent(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	val, err := pb.Get(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Nil(t, val)
	assert.True(t, errors.Is(err, backend.ErrKeyNotFound) || err.Error() == "key not found")
}

// TestPostgreSQLBackend_Delete tests Delete operation
func TestPostgreSQLBackend_Delete(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Set a value
	err = pb.Set(ctx, "key1", "value1", 0)
	require.NoError(t, err)

	// Delete the value
	err = pb.Delete(ctx, "key1")
	require.NoError(t, err)

	// Verify it's deleted
	val, err := pb.Get(ctx, "key1")
	assert.Error(t, err)
	assert.Nil(t, val)
}

// TestPostgreSQLBackend_Delete_NonExistent tests deleting non-existent key
func TestPostgreSQLBackend_Delete_NonExistent(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Deleting non-existent key should not error
	err = pb.Delete(ctx, "nonexistent")
	assert.NoError(t, err)
}

// TestPostgreSQLBackend_TTL_Expiration tests TTL expiration
func TestPostgreSQLBackend_TTL_Expiration(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Set a value with short TTL
	err = pb.Set(ctx, "expiring_key", "value", 100*time.Millisecond)
	require.NoError(t, err)

	// Get immediately should work
	val, err := pb.Get(ctx, "expiring_key")
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	val, err = pb.Get(ctx, "expiring_key")
	assert.Error(t, err)
	assert.Nil(t, val)
}

// TestPostgreSQLBackend_TTL_ZeroMeansNoExpiration tests that TTL=0 means no expiration
func TestPostgreSQLBackend_TTL_ZeroMeansNoExpiration(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Set a value with TTL=0
	err = pb.Set(ctx, "persistent_key", "value", 0)
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Should still exist
	val, err := pb.Get(ctx, "persistent_key")
	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

// TestPostgreSQLBackend_IncrementAndGet tests atomic increment
func TestPostgreSQLBackend_IncrementAndGet(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// First increment should return 1
	val, err := pb.IncrementAndGet(ctx, "counter", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val)

	// Second increment should return 2
	val, err = pb.IncrementAndGet(ctx, "counter", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), val)

	// Third increment should return 3
	val, err = pb.IncrementAndGet(ctx, "counter", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), val)
}

// TestPostgreSQLBackend_IncrementAndGet_WithTTL tests atomic increment with TTL
func TestPostgreSQLBackend_IncrementAndGet_WithTTL(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Increment with TTL
	val, err := pb.IncrementAndGet(ctx, "counter_ttl", 200*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val)

	// Should still be accessible
	val, err = pb.IncrementAndGet(ctx, "counter_ttl", 200*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, int64(2), val)

	// Wait for expiration
	time.Sleep(250 * time.Millisecond)

	// Should start from 1 again
	val, err = pb.IncrementAndGet(ctx, "counter_ttl", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val)
}

// TestPostgreSQLBackend_Type tests Type method
func TestPostgreSQLBackend_Type(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	assert.Equal(t, "postgresql", pb.Type())
}

// TestPostgreSQLBackend_Ping tests Ping method
func TestPostgreSQLBackend_Ping(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.Ping(ctx)
	assert.NoError(t, err)
}

// TestPostgreSQLBackend_Close tests Close method
func TestPostgreSQLBackend_Close(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Close should work
	err = pb.Close()
	assert.NoError(t, err)

	// Operations after close should fail
	err = pb.Set(ctx, "key", "value", 0)
	assert.Error(t, err)
}

// TestPostgreSQLBackend_ConcurrentWrites tests concurrent write operations
func TestPostgreSQLBackend_ConcurrentWrites(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "key_" + string(rune(id))
			err := pb.Set(ctx, key, id, 0)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
}

// TestPostgreSQLBackend_ConcurrentReads tests concurrent read operations
func TestPostgreSQLBackend_ConcurrentReads(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Set initial value
	err = pb.Set(ctx, "shared_key", "shared_value", 0)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := pb.Get(ctx, "shared_key")
			assert.NoError(t, err)
			assert.Equal(t, "shared_value", val)
		}()
	}

	wg.Wait()
}

// TestPostgreSQLBackend_ConcurrentReadWrite tests concurrent read/write operations
func TestPostgreSQLBackend_ConcurrentReadWrite(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 50

	// Writers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "rw_key"
			err := pb.Set(ctx, key, id, 0)
			assert.NoError(t, err)
		}(i)
	}

	// Readers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pb.Get(ctx, "rw_key")
		}()
	}

	wg.Wait()
}

// TestPostgreSQLBackend_ConcurrentIncrement tests concurrent increment operations
func TestPostgreSQLBackend_ConcurrentIncrement(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 100
	var counter int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := pb.IncrementAndGet(ctx, "concurrent_counter", 0)
			assert.NoError(t, err)
			assert.Greater(t, val, int64(0))
			atomic.AddInt64(&counter, 1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(numGoroutines), counter)

	// Final value should be numGoroutines
	finalVal, err := pb.Get(ctx, "concurrent_counter")
	require.NoError(t, err)
	assert.Equal(t, float64(numGoroutines), finalVal)
}

// TestPostgreSQLBackend_OverwriteKey tests overwriting existing key
func TestPostgreSQLBackend_OverwriteKey(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Set initial value
	err = pb.Set(ctx, "key", "value1", 0)
	require.NoError(t, err)

	// Overwrite
	err = pb.Set(ctx, "key", "value2", 0)
	require.NoError(t, err)

	// Get should return new value
	val, err := pb.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "value2", val)
}

// TestPostgreSQLBackend_EmptyKey tests empty key handling
func TestPostgreSQLBackend_EmptyKey(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Empty key should work (though not recommended)
	err = pb.Set(ctx, "", "value", 0)
	assert.NoError(t, err)

	val, err := pb.Get(ctx, "")
	assert.NoError(t, err)
	assert.Equal(t, "value", val)
}

// TestPostgreSQLBackend_NilValue tests nil value handling
func TestPostgreSQLBackend_NilValue(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Nil value should be handled
	err = pb.Set(ctx, "nil_key", nil, 0)
	assert.NoError(t, err)

	val, err := pb.Get(ctx, "nil_key")
	assert.NoError(t, err)
	assert.Nil(t, val)
}

// TestPostgreSQLBackend_UpdateTTL tests updating TTL for existing key
func TestPostgreSQLBackend_UpdateTTL(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Set with short TTL
	err = pb.Set(ctx, "key", "value", 100*time.Millisecond)
	require.NoError(t, err)

	// Update with longer TTL
	err = pb.Set(ctx, "key", "value", 1*time.Second)
	require.NoError(t, err)

	// Wait past original TTL
	time.Sleep(150 * time.Millisecond)

	// Should still exist
	val, err := pb.Get(ctx, "key")
	assert.NoError(t, err)
	assert.Equal(t, "value", val)
}

// TestPostgreSQLBackend_LargeValue tests storing large values
func TestPostgreSQLBackend_LargeValue(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Create a 1MB value
	largeValue := make([]byte, 1024*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	err = pb.Set(ctx, "large_key", largeValue, 0)
	assert.NoError(t, err)

	val, err := pb.Get(ctx, "large_key")
	assert.NoError(t, err)
	assert.NotNil(t, val)
}

// TestPostgreSQLBackend_ManyKeys tests storing many keys
func TestPostgreSQLBackend_ManyKeys(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	numKeys := 1000
	for i := 0; i < numKeys; i++ {
		key := "key_" + string(rune(i))
		err := pb.Set(ctx, key, i, 0)
		assert.NoError(t, err)
	}

	// Verify some keys
	val, err := pb.Get(ctx, "key_"+string(rune(0)))
	assert.NoError(t, err)
	assert.NotNil(t, val)
}

// TestPostgreSQLBackend_ContextCancellation tests context cancellation
func TestPostgreSQLBackend_ContextCancellation(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	err := pb.CreateTable(context.Background())
	require.NoError(t, err)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Operations with cancelled context should fail
	err = pb.Set(ctx, "key", "value", 0)
	assert.Error(t, err)
}

// TestPostgreSQLBackend_GetSchema tests GetSchema method
func TestPostgreSQLBackend_GetSchema(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	schema := pb.GetSchema(ctx)
	assert.NotEmpty(t, schema)
	assert.Contains(t, schema, "test_rate_limit_state")
}

// TestPostgreSQLBackend_CleanupExpired tests CleanupExpired method
func TestPostgreSQLBackend_CleanupExpired(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Set some keys with short TTL
	err = pb.Set(ctx, "exp1", "value1", 50*time.Millisecond)
	require.NoError(t, err)
	err = pb.Set(ctx, "exp2", "value2", 50*time.Millisecond)
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Cleanup
	count, err := pb.CleanupExpired(ctx)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(0))
}

// TestPostgreSQLBackend_Vacuum tests Vacuum method
func TestPostgreSQLBackend_Vacuum(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Vacuum should work
	err = pb.Vacuum(ctx)
	assert.NoError(t, err)
}

// TestPostgreSQLBackend_GetStats tests GetStats method
func TestPostgreSQLBackend_GetStats(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	stats := pb.GetStats(ctx)
	assert.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.TotalRows, int64(0))
}

// TestPostgreSQLBackend_Transaction tests Transaction method
func TestPostgreSQLBackend_Transaction(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Transaction should work
	err = pb.Transaction(ctx, func(txBackend backend.Backend) error {
		err := txBackend.Set(ctx, "tx_key1", "value1", 0)
		if err != nil {
			return err
		}
		err = txBackend.Set(ctx, "tx_key2", "value2", 0)
		if err != nil {
			return err
		}
		return nil
	})
	assert.NoError(t, err)

	// Verify both keys exist
	val, err := pb.Get(ctx, "tx_key1")
	assert.NoError(t, err)
	assert.Equal(t, "value1", val)

	val, err = pb.Get(ctx, "tx_key2")
	assert.NoError(t, err)
	assert.Equal(t, "value2", val)
}

// TestPostgreSQLBackend_Migrate tests Migrate method
func TestPostgreSQLBackend_Migrate(t *testing.T) {
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	ctx := context.Background()
	err := pb.CreateTable(ctx)
	require.NoError(t, err)

	// Migrate should work
	err = pb.Migrate(ctx)
	assert.NoError(t, err)
}

// Benchmark tests
func BenchmarkPostgreSQLBackend_Set(b *testing.B) {
	if !isPostgreSQLAvailable() {
		b.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "bench_")
	defer pb.Close()

	ctx := context.Background()
	_ = pb.CreateTable(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pb.Set(ctx, "bench_key", i, 0)
	}
}

func BenchmarkPostgreSQLBackend_Get(b *testing.B) {
	if !isPostgreSQLAvailable() {
		b.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "bench_")
	defer pb.Close()

	ctx := context.Background()
	_ = pb.CreateTable(ctx)
	_ = pb.Set(ctx, "bench_key", "value", 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pb.Get(ctx, "bench_key")
	}
}

func BenchmarkPostgreSQLBackend_IncrementAndGet(b *testing.B) {
	if !isPostgreSQLAvailable() {
		b.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "bench_")
	defer pb.Close()

	ctx := context.Background()
	_ = pb.CreateTable(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pb.IncrementAndGet(ctx, "bench_counter", 0)
	}
}

func BenchmarkPostgreSQLBackend_Delete(b *testing.B) {
	if !isPostgreSQLAvailable() {
		b.Skip("PostgreSQL not available")
	}

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "bench_")
	defer pb.Close()

	ctx := context.Background()
	_ = pb.CreateTable(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		_ = pb.Set(ctx, "bench_key", "value", 0)
		b.StartTimer()
		_ = pb.Delete(ctx, "bench_key")
	}
}

// isPostgreSQLAvailable checks if PostgreSQL is available for testing
func isPostgreSQLAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pb := NewPostgreSQLBackend("localhost", 5432, "postgres", "postgres", "postgres", "test_")
	defer pb.Close()

	return pb.Ping(ctx) == nil
}
