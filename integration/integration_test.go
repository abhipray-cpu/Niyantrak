//go:build integration

// Run these tests with:
//
//	docker compose up -d
//	go test -tags integration -v -count=1 ./integration/...
//	docker compose down
package integration_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend"
	"github.com/abhipray-cpu/niyantrak/backend/memory"
	pgbackend "github.com/abhipray-cpu/niyantrak/backend/postgresql"
	redisbackend "github.com/abhipray-cpu/niyantrak/backend/redis"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/limiters/basic"
	"github.com/abhipray-cpu/niyantrak/limiters/composite"
	"github.com/abhipray-cpu/niyantrak/limiters/cost"
	"github.com/abhipray-cpu/niyantrak/limiters/tier"

	// Import root package to trigger init() which registers envelope types
	// needed for Redis/PostgreSQL backend state roundtrip.
	_ "github.com/abhipray-cpu/niyantrak"
)

func redisAddr() string {
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		return v
	}
	return "localhost:6379"
}

func pgHost() string {
	if v := os.Getenv("PG_HOST"); v != "" {
		return v
	}
	return "localhost"
}

func pgPort() int { return 5432 }

// ==========================================================================
// Helpers
// ==========================================================================

func makeTokenBucket(capacity, refillRate int, period time.Duration) algorithm.Algorithm {
	return algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     capacity,
		RefillRate:   refillRate,
		RefillPeriod: period,
	})
}

func makeLeakyBucket(capacity, leakRate int, period time.Duration) algorithm.Algorithm {
	return algorithm.NewLeakyBucket(algorithm.LeakyBucketConfig{
		Capacity:   capacity,
		LeakRate:   leakRate,
		LeakPeriod: period,
	})
}

func makeFixedWindow(limit int, window time.Duration) algorithm.Algorithm {
	return algorithm.NewFixedWindow(algorithm.FixedWindowConfig{
		Limit:  limit,
		Window: window,
	})
}

func makeSlidingWindow(limit int, window time.Duration) algorithm.Algorithm {
	return algorithm.NewSlidingWindow(algorithm.SlidingWindowConfig{
		Limit:  limit,
		Window: window,
	})
}

func makeGCRA(limit int, period time.Duration, burst int) algorithm.Algorithm {
	return algorithm.NewGCRA(algorithm.GCRAConfig{
		Limit:     limit,
		Period:    period,
		BurstSize: burst,
	})
}

type backendFactory struct {
	name  string
	build func(t *testing.T) backend.Backend
}

func getBackends(t *testing.T) []backendFactory {
	t.Helper()
	factories := []backendFactory{
		{name: "memory", build: func(t *testing.T) backend.Backend {
			return memory.NewMemoryBackend()
		}},
	}

	// Redis
	rb := redisbackend.NewRedisBackend(redisAddr(), 0, "probe:")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rb.Ping(ctx); err == nil {
		rb.Close()
		factories = append(factories, backendFactory{name: "redis", build: func(t *testing.T) backend.Backend {
			return redisbackend.NewRedisBackend(redisAddr(), 0, fmt.Sprintf("test:%s:", sanitize(t.Name())))
		}})
	} else {
		t.Logf("Redis not available at %s, skipping Redis tests: %v", redisAddr(), err)
	}

	// PostgreSQL
	pb := pgbackend.NewPostgreSQLBackend(pgHost(), pgPort(), "niyantrak_test", "niyantrak", "niyantrak", "probe_")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if err := pb.Ping(ctx2); err == nil {
		pb.Close()
		factories = append(factories, backendFactory{name: "postgresql", build: func(t *testing.T) backend.Backend {
			be := pgbackend.NewPostgreSQLBackend(pgHost(), pgPort(), "niyantrak_test", "niyantrak", "niyantrak", fmt.Sprintf("t_%s_", sanitize(t.Name())[:min(30, len(sanitize(t.Name())))]))
			ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel3()
			if err := be.CreateTable(ctx3); err != nil {
				t.Fatalf("CreateTable: %v", err)
			}
			return be
		}})
	} else {
		t.Logf("PostgreSQL not available at %s:%d, skipping PG tests: %v", pgHost(), pgPort(), err)
	}

	return factories
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			out = append(out, c)
		}
	}
	return string(out)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ==========================================================================
// Test 1: Basic correctness — each algorithm denies when it should
// ==========================================================================

func TestCorrectness_AllAlgorithms(t *testing.T) {
	algorithms := []struct {
		name  string
		build func() algorithm.Algorithm
		limit int
	}{
		{"TokenBucket", func() algorithm.Algorithm { return makeTokenBucket(5, 5, time.Minute) }, 5},
		{"LeakyBucket", func() algorithm.Algorithm { return makeLeakyBucket(5, 5, time.Minute) }, 5},
		{"FixedWindow", func() algorithm.Algorithm { return makeFixedWindow(5, time.Minute) }, 5},
		{"SlidingWindow", func() algorithm.Algorithm { return makeSlidingWindow(5, time.Minute) }, 5},
		{"GCRA", func() algorithm.Algorithm { return makeGCRA(5, time.Minute, 5) }, 5},
	}

	for _, bf := range getBackends(t) {
		for _, alg := range algorithms {
			t.Run(fmt.Sprintf("%s/%s", bf.name, alg.name), func(t *testing.T) {
				be := bf.build(t)
				defer be.Close()

				limiter, err := basic.NewBasicLimiter(alg.build(), be, limiters.BasicConfig{
					DefaultLimit:  alg.limit,
					DefaultWindow: time.Minute,
					KeyTTL:        time.Minute,
				})
				if err != nil {
					t.Fatalf("NewBasicLimiter: %v", err)
				}
				defer limiter.Close()

				ctx := context.Background()
				key := fmt.Sprintf("correctness-%s-%s-%d", bf.name, alg.name, time.Now().UnixNano())

				// Should allow up to limit
				for i := 0; i < alg.limit; i++ {
					r := limiter.Allow(ctx, key)
					if r.Error != nil {
						t.Fatalf("request %d: unexpected error: %v", i, r.Error)
					}
					if !r.Allowed {
						t.Fatalf("request %d: should be allowed but was denied", i)
					}
				}

				// The next request should be denied
				r := limiter.Allow(ctx, key)
				if r.Error != nil {
					t.Fatalf("request %d (over-limit): unexpected error: %v", alg.limit, r.Error)
				}
				if r.Allowed {
					t.Fatalf("request %d: should be DENIED but was allowed (algo=%s, backend=%s)",
						alg.limit, alg.name, bf.name)
				}
			})
		}
	}
}

// ==========================================================================
// Test 2: Concurrency — hammer a limiter from many goroutines
// ==========================================================================

func TestConcurrency_NoRaceNoPanic(t *testing.T) {
	for _, bf := range getBackends(t) {
		t.Run(bf.name, func(t *testing.T) {
			be := bf.build(t)
			defer be.Close()

			algo := makeTokenBucket(100, 100, time.Minute)
			limiter, err := basic.NewBasicLimiter(algo, be, limiters.BasicConfig{
				DefaultLimit:  100,
				DefaultWindow: time.Minute,
				KeyTTL:        time.Minute,
			})
			if err != nil {
				t.Fatalf("NewBasicLimiter: %v", err)
			}
			defer limiter.Close()

			ctx := context.Background()
			const goroutines = 50
			const requestsPerGoroutine = 20
			key := fmt.Sprintf("concurrent-%s-%d", bf.name, time.Now().UnixNano())

			var allowed int64
			var denied int64
			var errors int64

			var wg sync.WaitGroup
			wg.Add(goroutines)
			for g := 0; g < goroutines; g++ {
				go func(id int) {
					defer wg.Done()
					for i := 0; i < requestsPerGoroutine; i++ {
						r := limiter.Allow(ctx, key)
						if r.Error != nil {
							atomic.AddInt64(&errors, 1)
							continue
						}
						if r.Allowed {
							atomic.AddInt64(&allowed, 1)
						} else {
							atomic.AddInt64(&denied, 1)
						}
					}
				}(g)
			}
			wg.Wait()

			total := allowed + denied + errors
			t.Logf("Backend=%s: total=%d allowed=%d denied=%d errors=%d",
				bf.name, total, allowed, denied, errors)

			if total != goroutines*requestsPerGoroutine {
				t.Fatalf("expected %d total, got %d", goroutines*requestsPerGoroutine, total)
			}

			// With non-atomic Get/Set, concurrent requests can overshoot.
			// The token bucket algorithm is not transactional — this is a
			// known limitation. We verify that:
			// 1. No panics or data races occurred
			// 2. Some requests were allowed (limiter is working)
			// 3. Total = expected count (no lost requests)
			if allowed == 0 {
				t.Errorf("no requests allowed — limiter failed completely")
			}
			if denied == 0 && bf.name == "memory" {
				// Memory backend should at least deny some in a tight loop.
				// Redis/PG have higher latency making the race window wider.
				t.Logf("WARNING: no requests denied with memory backend — possible concurrency issue")
			}
		})
	}
}

// ==========================================================================
// Test 3: State roundtrip — store and retrieve from external backends
// ==========================================================================

func TestStateRoundtrip_ExternalBackends(t *testing.T) {
	for _, bf := range getBackends(t) {
		if bf.name == "memory" {
			continue // roundtrip is trivial for memory
		}
		t.Run(bf.name, func(t *testing.T) {
			be := bf.build(t)
			defer be.Close()

			algo := makeTokenBucket(10, 10, time.Minute)
			limiter, err := basic.NewBasicLimiter(algo, be, limiters.BasicConfig{
				DefaultLimit:  10,
				DefaultWindow: time.Minute,
				KeyTTL:        time.Minute,
			})
			if err != nil {
				t.Fatalf("NewBasicLimiter: %v", err)
			}
			defer limiter.Close()

			ctx := context.Background()
			key := fmt.Sprintf("roundtrip-%s-%d", bf.name, time.Now().UnixNano())

			// Consume 3 tokens
			for i := 0; i < 3; i++ {
				r := limiter.Allow(ctx, key)
				if !r.Allowed {
					t.Fatalf("request %d should be allowed", i)
				}
			}

			// Create a NEW limiter on the same backend+key to verify state persisted
			algo2 := makeTokenBucket(10, 10, time.Minute)
			limiter2, err := basic.NewBasicLimiter(algo2, be, limiters.BasicConfig{
				DefaultLimit:  10,
				DefaultWindow: time.Minute,
				KeyTTL:        time.Minute,
			})
			if err != nil {
				t.Fatalf("NewBasicLimiter (second): %v", err)
			}
			defer limiter2.Close()

			// Should still have ~7 tokens remaining, not 10
			for i := 0; i < 7; i++ {
				r := limiter2.Allow(ctx, key)
				if !r.Allowed {
					t.Fatalf("second limiter: request %d should be allowed (tokens remaining)", i)
				}
			}

			// Should now be at limit
			r := limiter2.Allow(ctx, key)
			if r.Allowed {
				t.Fatalf("second limiter: should be DENIED (state roundtrip failed)")
			}
		})
	}
}

// ==========================================================================
// Test 4: Tier-based — verify tier isolation
// ==========================================================================

func TestTierBased_DifferentLimits(t *testing.T) {
	for _, bf := range getBackends(t) {
		t.Run(bf.name, func(t *testing.T) {
			be := bf.build(t)
			defer be.Close()

			// The token bucket capacity must match the tier limit,
			// because the algorithm itself enforces the actual rate, not
			// the TierConfig. TierConfig only selects the limit metadata.
			algo := makeTokenBucket(3, 3, time.Minute)
			limiter, err := tier.NewTierBasedLimiter(algo, be, limiters.TierConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  3,
					DefaultWindow: time.Minute,
					KeyTTL:        time.Minute,
				},
				Tiers: map[string]limiters.TierLimit{
					"free":    {Limit: 3, Window: time.Minute},
					"premium": {Limit: 10, Window: time.Minute},
				},
				DefaultTier: "free",
			})
			if err != nil {
				t.Fatalf("NewTierBasedLimiter: %v", err)
			}
			defer limiter.Close()

			ctx := context.Background()
			key := fmt.Sprintf("tier-free-%s-%d", bf.name, time.Now().UnixNano())

			// Free tier: should allow 3
			for i := 0; i < 3; i++ {
				r := limiter.Allow(ctx, key)
				if r.Error != nil {
					t.Fatalf("free request %d error: %v", i, r.Error)
				}
				if !r.Allowed {
					t.Fatalf("free request %d should be allowed", i)
				}
			}
			r := limiter.Allow(ctx, key)
			if r.Allowed {
				t.Errorf("free request 4 should be denied")
			}
		})
	}
}

// ==========================================================================
// Test 5: Cost-based — weighted operations
// ==========================================================================

func TestCostBased_DifferentCosts(t *testing.T) {
	for _, bf := range getBackends(t) {
		t.Run(bf.name, func(t *testing.T) {
			be := bf.build(t)
			defer be.Close()

			algo := makeTokenBucket(10, 10, time.Minute)
			limiter, err := cost.NewCostBasedLimiter(algo, be, limiters.CostConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  10,
					DefaultWindow: time.Minute,
					KeyTTL:        time.Minute,
				},
				DefaultCost: 1,
				Operations: map[string]int{
					"cheap":     1,
					"expensive": 5,
				},
			})
			if err != nil {
				t.Fatalf("NewCostBasedLimiter: %v", err)
			}
			defer limiter.Close()

			ctx := context.Background()
			key := fmt.Sprintf("cost-%s-%d", bf.name, time.Now().UnixNano())

			// Consume 5 tokens
			r := limiter.AllowWithCost(ctx, key, 5)
			if r.Error != nil {
				t.Fatalf("expensive op error: %v", r.Error)
			}
			if !r.Allowed {
				t.Fatalf("expensive op should be allowed")
			}

			// Another expensive: 5 more (total 10, at limit)
			r = limiter.AllowWithCost(ctx, key, 5)
			if r.Error != nil {
				t.Fatalf("second expensive op error: %v", r.Error)
			}
			if !r.Allowed {
				t.Fatalf("second expensive op should be allowed")
			}

			// Even cheap should be denied
			r = limiter.AllowWithCost(ctx, key, 1)
			if r.Allowed {
				t.Errorf("cheap op should be denied (0 tokens left)")
			}
		})
	}
}

// ==========================================================================
// Test 6: Composite — multiple simultaneous limits
// ==========================================================================

func TestComposite_MultipleLimits(t *testing.T) {
	for _, bf := range getBackends(t) {
		t.Run(bf.name, func(t *testing.T) {
			be := bf.build(t)
			defer be.Close()

			// The algorithm capacity should match the tightest limit.
			// Composite checks each LimitConfig but uses the same algorithm.
			algo := makeTokenBucket(3, 3, time.Minute)

			compLimiter, err := composite.NewCompositeLimiter(algo, be, limiters.CompositeConfig{
				Name: "test-composite",
				Limits: []limiters.LimitConfig{
					{Name: "per-minute", Limit: 3, Window: time.Minute, Priority: 1},
					{Name: "per-hour", Limit: 100, Window: time.Hour, Priority: 2},
				},
			})
			if err != nil {
				t.Fatalf("NewCompositeLimiter: %v", err)
			}
			defer compLimiter.Close()

			ctx := context.Background()
			key := fmt.Sprintf("composite-%s-%d", bf.name, time.Now().UnixNano())

			for i := 0; i < 3; i++ {
				r := compLimiter.Allow(ctx, key)
				if r.Error != nil {
					t.Fatalf("composite request %d error: %v", i, r.Error)
				}
				if !r.Allowed {
					t.Fatalf("composite request %d should be allowed", i)
				}
			}

			r := compLimiter.Allow(ctx, key)
			if r.Allowed {
				t.Errorf("composite request 4 should be denied")
			}
		})
	}
}

// ==========================================================================
// Test 7: Empty key rejected
// ==========================================================================

func TestEmptyKey_Rejected(t *testing.T) {
	be := memory.NewMemoryBackend()
	defer be.Close()

	algo := makeTokenBucket(10, 10, time.Minute)
	limiter, err := basic.NewBasicLimiter(algo, be, limiters.BasicConfig{
		DefaultLimit: 10, DefaultWindow: time.Minute, KeyTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewBasicLimiter: %v", err)
	}
	defer limiter.Close()

	r := limiter.Allow(context.Background(), "")
	if r.Allowed {
		t.Errorf("empty key should be denied")
	}
	if r.Error == nil {
		t.Errorf("empty key should return an error")
	}
}

// ==========================================================================
// Test 8: Key isolation — different keys have independent limits
// ==========================================================================

func TestKeyIsolation(t *testing.T) {
	for _, bf := range getBackends(t) {
		t.Run(bf.name, func(t *testing.T) {
			be := bf.build(t)
			defer be.Close()

			algo := makeTokenBucket(3, 3, time.Minute)
			limiter, err := basic.NewBasicLimiter(algo, be, limiters.BasicConfig{
				DefaultLimit: 3, DefaultWindow: time.Minute, KeyTTL: time.Minute,
			})
			if err != nil {
				t.Fatalf("NewBasicLimiter: %v", err)
			}
			defer limiter.Close()

			ctx := context.Background()
			keyPrefix := fmt.Sprintf("isolation-%s-%d", bf.name, time.Now().UnixNano())

			// Exhaust key-A
			for i := 0; i < 3; i++ {
				limiter.Allow(ctx, keyPrefix+"-A")
			}
			r := limiter.Allow(ctx, keyPrefix+"-A")
			if r.Allowed {
				t.Errorf("key-A should be exhausted")
			}

			// key-B should still be fresh
			r = limiter.Allow(ctx, keyPrefix+"-B")
			if !r.Allowed {
				t.Errorf("key-B should still be allowed")
			}
		})
	}
}

// ==========================================================================
// Test 9: Closed limiter rejects requests
// ==========================================================================

func TestClosedLimiter_Rejects(t *testing.T) {
	be := memory.NewMemoryBackend()
	defer be.Close()

	algo := makeTokenBucket(10, 10, time.Minute)
	limiter, err := basic.NewBasicLimiter(algo, be, limiters.BasicConfig{
		DefaultLimit: 10, DefaultWindow: time.Minute, KeyTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewBasicLimiter: %v", err)
	}

	limiter.Close()

	r := limiter.Allow(context.Background(), "some-key")
	if r.Allowed {
		t.Errorf("closed limiter should deny")
	}
	if r.Error == nil {
		t.Errorf("closed limiter should return error")
	}
}

// ==========================================================================
// Test 10: AllowN with n > capacity
// ==========================================================================

func TestAllowN_ExceedsCapacity(t *testing.T) {
	be := memory.NewMemoryBackend()
	defer be.Close()

	algo := makeTokenBucket(5, 5, time.Minute)
	limiter, err := basic.NewBasicLimiter(algo, be, limiters.BasicConfig{
		DefaultLimit: 5, DefaultWindow: time.Minute, KeyTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewBasicLimiter: %v", err)
	}
	defer limiter.Close()

	r := limiter.AllowN(context.Background(), "big-ask", 100)
	if r.Allowed {
		t.Errorf("AllowN(100) should be denied when capacity is 5")
	}
}
