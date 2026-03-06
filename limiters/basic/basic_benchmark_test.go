package basic

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend/memory"
	"github.com/abhipray-cpu/niyantrak/limiters"
)

// newBenchLimiter creates a BasicLimiter configured for benchmarking.
func newBenchLimiter(b *testing.B, algo algorithm.Algorithm) *basicLimiter {
	b.Helper()
	be := memory.NewMemoryBackend()
	l, err := NewBasicLimiter(algo, be, limiters.BasicConfig{
		DefaultLimit:  1_000_000, // very high so we never hit the limit
		DefaultWindow: time.Minute,
		KeyTTL:        time.Hour,
	})
	if err != nil {
		b.Fatal(err)
	}
	return l.(*basicLimiter)
}

// ---------------------------------------------------------------------------
// Single-goroutine benchmarks
// ---------------------------------------------------------------------------

func BenchmarkAllow_TokenBucket(b *testing.B) {
	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity: 1_000_000, RefillRate: 1_000_000, RefillPeriod: time.Second,
	})
	l := newBenchLimiter(b, algo)
	defer l.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow(ctx, "key")
	}
}

func BenchmarkAllow_LeakyBucket(b *testing.B) {
	algo := algorithm.NewLeakyBucket(algorithm.LeakyBucketConfig{
		Capacity: 1_000_000, LeakRate: 1_000_000, LeakPeriod: time.Second,
	})
	l := newBenchLimiter(b, algo)
	defer l.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow(ctx, "key")
	}
}

func BenchmarkAllow_FixedWindow(b *testing.B) {
	algo := algorithm.NewFixedWindow(algorithm.FixedWindowConfig{
		Limit: 1_000_000, Window: time.Minute,
	})
	l := newBenchLimiter(b, algo)
	defer l.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow(ctx, "key")
	}
}

func BenchmarkAllow_SlidingWindow(b *testing.B) {
	algo := algorithm.NewSlidingWindow(algorithm.SlidingWindowConfig{
		Limit: 1_000_000, Window: time.Minute,
	})
	l := newBenchLimiter(b, algo)
	defer l.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow(ctx, "key")
	}
}

func BenchmarkAllow_GCRA(b *testing.B) {
	algo := algorithm.NewGCRA(algorithm.GCRAConfig{
		Period: time.Minute, Limit: 1_000_000, BurstSize: 100,
	})
	l := newBenchLimiter(b, algo)
	defer l.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow(ctx, "key")
	}
}

// ---------------------------------------------------------------------------
// AllowN
// ---------------------------------------------------------------------------

func BenchmarkAllowN_TokenBucket(b *testing.B) {
	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity: 1_000_000, RefillRate: 1_000_000, RefillPeriod: time.Second,
	})
	l := newBenchLimiter(b, algo)
	defer l.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.AllowN(ctx, "key", 5)
	}
}

// ---------------------------------------------------------------------------
// Parallel benchmarks (measure contention)
// ---------------------------------------------------------------------------

func BenchmarkAllowParallel_TokenBucket(b *testing.B) {
	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity: 1_000_000, RefillRate: 1_000_000, RefillPeriod: time.Second,
	})
	l := newBenchLimiter(b, algo)
	defer l.Close()
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Allow(ctx, "key")
		}
	})
}

func BenchmarkAllowParallel_SlidingWindow(b *testing.B) {
	algo := algorithm.NewSlidingWindow(algorithm.SlidingWindowConfig{
		Limit: 1_000_000, Window: time.Minute,
	})
	l := newBenchLimiter(b, algo)
	defer l.Close()
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Allow(ctx, "key")
		}
	})
}

// ---------------------------------------------------------------------------
// Multi-key benchmarks (simulate realistic traffic)
// ---------------------------------------------------------------------------

func BenchmarkAllow_MultiKey_TokenBucket(b *testing.B) {
	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity: 1_000_000, RefillRate: 1_000_000, RefillPeriod: time.Second,
	})
	l := newBenchLimiter(b, algo)
	defer l.Close()
	ctx := context.Background()

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("user:%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow(ctx, keys[i%len(keys)])
	}
}

func BenchmarkAllowParallel_MultiKey_TokenBucket(b *testing.B) {
	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity: 1_000_000, RefillRate: 1_000_000, RefillPeriod: time.Second,
	})
	l := newBenchLimiter(b, algo)
	defer l.Close()
	ctx := context.Background()

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("user:%d", i)
	}

	var idx uint64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			l.Allow(ctx, keys[i%len(keys)])
			i++
		}
		_ = idx
	})
}
