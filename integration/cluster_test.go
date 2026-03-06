//go:build integration

// Redis Cluster integration tests.
// Run with:
//
//	docker-compose up -d
//	go test -tags integration -v -count=1 -run TestCluster ./integration/...
//	docker-compose down
package integration_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend"
	redisbackend "github.com/abhipray-cpu/niyantrak/backend/redis"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/limiters/basic"
	"github.com/abhipray-cpu/niyantrak/limiters/cost"
	"github.com/abhipray-cpu/niyantrak/limiters/tier"
	goredis "github.com/redis/go-redis/v9"

	// Trigger init() for envelope type registration.
	_ "github.com/abhipray-cpu/niyantrak"
)

// clusterAddrs returns the Redis Cluster seed addresses.
func clusterAddrs() []string {
	if v := os.Getenv("REDIS_CLUSTER_ADDRS"); v != "" {
		return strings.Split(v, ",")
	}
	return []string{
		"localhost:7001",
		"localhost:7002",
		"localhost:7003",
		"localhost:7004",
		"localhost:7005",
		"localhost:7006",
	}
}

// newClusterBackend creates a Redis Cluster backend with a unique prefix.
// It uses a custom dialer to remap host.docker.internal → localhost so that
// cluster node discovery works from the macOS host (Docker/Colima setup).
func newClusterBackend(t *testing.T, prefix string) backend.Backend {
	t.Helper()

	client := goredis.NewClusterClient(&goredis.ClusterOptions{
		Addrs:        clusterAddrs(),
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   3,
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Remap host.docker.internal → localhost for macOS Docker setups.
			addr = strings.Replace(addr, "host.docker.internal", "localhost", 1)
			return net.DialTimeout(network, addr, 5*time.Second)
		},
	})
	return redisbackend.NewRedisBackendFromClient(client, prefix)
}

// skipIfClusterUnavailable probes the cluster and skips the test if unreachable.
func skipIfClusterUnavailable(t *testing.T) {
	t.Helper()
	be := newClusterBackend(t, "probe:")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := be.(interface{ Ping(context.Context) error }).Ping(ctx); err != nil {
		be.Close()
		t.Skipf("Redis Cluster not available at %v: %v", clusterAddrs(), err)
	}
	be.Close()
}

// ==========================================================================
// Test 1: CheckCluster returns true for cluster client
// ==========================================================================

func TestCluster_CheckCluster(t *testing.T) {
	skipIfClusterUnavailable(t)

	be := newClusterBackend(t, "chk:")
	defer be.Close()

	rb, ok := be.(interface {
		CheckCluster(ctx context.Context) (bool, error)
	})
	if !ok {
		t.Fatal("backend does not implement CheckCluster")
	}

	isCluster, err := rb.CheckCluster(context.Background())
	if err != nil {
		t.Fatalf("CheckCluster error: %v", err)
	}
	if !isCluster {
		t.Fatal("CheckCluster should return true for a cluster client")
	}
}

// ==========================================================================
// Test 2: CheckCluster returns false for standalone
// ==========================================================================

func TestCluster_StandaloneIsNotCluster(t *testing.T) {
	// Use the standalone Redis on port 6379
	be := redisbackend.NewRedisBackend(redisAddr(), 0, "standalone:")
	defer be.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := be.(interface{ Ping(context.Context) error }).Ping(ctx); err != nil {
		t.Skipf("Standalone Redis not available: %v", err)
	}

	rb := be.(interface {
		CheckCluster(ctx context.Context) (bool, error)
	})
	isCluster, err := rb.CheckCluster(context.Background())
	if err != nil {
		t.Fatalf("CheckCluster error: %v", err)
	}
	if isCluster {
		t.Fatal("CheckCluster should return false for standalone Redis")
	}
}

// ==========================================================================
// Test 3: Basic Get/Set/Delete across cluster slots
// ==========================================================================

func TestCluster_BasicOperations(t *testing.T) {
	skipIfClusterUnavailable(t)

	be := newClusterBackend(t, fmt.Sprintf("ops%d:", time.Now().UnixNano()))
	defer be.Close()

	ctx := context.Background()

	// Write keys that hash to different slots
	keys := []string{"user:alice", "user:bob", "ip:192.168.1.1", "api:v2:list", "session:xyz"}

	for _, key := range keys {
		err := be.Set(ctx, key, "value-for-"+key, 30*time.Second)
		if err != nil {
			t.Fatalf("Set(%s): %v", key, err)
		}
	}

	// Read them back
	for _, key := range keys {
		val, err := be.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get(%s): %v", key, err)
		}
		expected := "value-for-" + key
		if val != expected {
			t.Errorf("Get(%s) = %v, want %v", key, val, expected)
		}
	}

	// Delete
	for _, key := range keys {
		if err := be.Delete(ctx, key); err != nil {
			t.Fatalf("Delete(%s): %v", key, err)
		}
	}

	// Verify deleted
	for _, key := range keys {
		_, err := be.Get(ctx, key)
		if err != backend.ErrKeyNotFound {
			t.Errorf("Get(%s) after delete: got err=%v, want ErrKeyNotFound", key, err)
		}
	}
}

// ==========================================================================
// Test 4: Lua CAS (AtomicUpdate) works in cluster mode
// ==========================================================================

func TestCluster_LuaCAS_AtomicUpdate(t *testing.T) {
	skipIfClusterUnavailable(t)

	prefix := fmt.Sprintf("cas%d:", time.Now().UnixNano())
	be := newClusterBackend(t, prefix)
	defer be.Close()

	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     10,
		RefillRate:   10,
		RefillPeriod: time.Minute,
	})

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
	key := fmt.Sprintf("cas-key-%d", time.Now().UnixNano())

	// Consume all 10 tokens sequentially
	for i := 0; i < 10; i++ {
		r := limiter.Allow(ctx, key)
		if r.Error != nil {
			t.Fatalf("request %d error: %v", i, r.Error)
		}
		if !r.Allowed {
			t.Fatalf("request %d should be allowed", i)
		}
	}

	// 11th should be denied
	r := limiter.Allow(ctx, key)
	if r.Error != nil {
		t.Fatalf("over-limit request error: %v", r.Error)
	}
	if r.Allowed {
		t.Fatal("over-limit request should be denied")
	}
}

// ==========================================================================
// Test 5: Concurrent CAS under cluster — no panics, no races
// ==========================================================================

func TestCluster_ConcurrentCAS(t *testing.T) {
	skipIfClusterUnavailable(t)

	prefix := fmt.Sprintf("conc%d:", time.Now().UnixNano())
	be := newClusterBackend(t, prefix)
	defer be.Close()

	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     100,
		RefillRate:   100,
		RefillPeriod: time.Minute,
	})

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
	key := fmt.Sprintf("concurrent-cluster-%d", time.Now().UnixNano())

	const goroutines = 30
	const requestsPerGoroutine = 10

	var allowed, denied, errors int64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
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
		}()
	}
	wg.Wait()

	total := allowed + denied + errors
	t.Logf("Cluster concurrent: total=%d allowed=%d denied=%d errors=%d",
		total, allowed, denied, errors)

	if total != goroutines*requestsPerGoroutine {
		t.Fatalf("expected %d total, got %d", goroutines*requestsPerGoroutine, total)
	}
	if allowed == 0 {
		t.Error("no requests allowed — cluster limiter broken")
	}
}

// ==========================================================================
// Test 6: Multi-key across slots — keys hash to different cluster nodes
// ==========================================================================

func TestCluster_MultiKey_DifferentSlots(t *testing.T) {
	skipIfClusterUnavailable(t)

	prefix := fmt.Sprintf("multi%d:", time.Now().UnixNano())
	be := newClusterBackend(t, prefix)
	defer be.Close()

	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     5,
		RefillRate:   5,
		RefillPeriod: time.Minute,
	})

	limiter, err := basic.NewBasicLimiter(algo, be, limiters.BasicConfig{
		DefaultLimit:  5,
		DefaultWindow: time.Minute,
		KeyTTL:        time.Minute,
	})
	if err != nil {
		t.Fatalf("NewBasicLimiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()

	// Keys chosen to hash to different CRC16 slots
	keys := []string{
		fmt.Sprintf("alpha-%d", time.Now().UnixNano()),
		fmt.Sprintf("bravo-%d", time.Now().UnixNano()),
		fmt.Sprintf("charlie-%d", time.Now().UnixNano()),
		fmt.Sprintf("delta-%d", time.Now().UnixNano()),
	}

	for _, key := range keys {
		// Each key should independently allow 5 requests
		for i := 0; i < 5; i++ {
			r := limiter.Allow(ctx, key)
			if r.Error != nil {
				t.Fatalf("key=%s request %d error: %v", key, i, r.Error)
			}
			if !r.Allowed {
				t.Fatalf("key=%s request %d should be allowed", key, i)
			}
		}

		// 6th should be denied
		r := limiter.Allow(ctx, key)
		if r.Allowed {
			t.Errorf("key=%s over-limit should be denied", key)
		}
	}
}

// ==========================================================================
// Test 7: Tier-based limiter on cluster
// ==========================================================================

func TestCluster_TierBased(t *testing.T) {
	skipIfClusterUnavailable(t)

	prefix := fmt.Sprintf("tier%d:", time.Now().UnixNano())
	be := newClusterBackend(t, prefix)
	defer be.Close()

	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     3,
		RefillRate:   3,
		RefillPeriod: time.Minute,
	})

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
	key := fmt.Sprintf("cluster-tier-%d", time.Now().UnixNano())

	// Free tier: allow 3
	for i := 0; i < 3; i++ {
		r := limiter.Allow(ctx, key)
		if r.Error != nil {
			t.Fatalf("tier request %d error: %v", i, r.Error)
		}
		if !r.Allowed {
			t.Fatalf("tier request %d should be allowed", i)
		}
	}

	r := limiter.Allow(ctx, key)
	if r.Allowed {
		t.Error("tier request 4 should be denied")
	}
}

// ==========================================================================
// Test 8: Cost-based limiter on cluster
// ==========================================================================

func TestCluster_CostBased(t *testing.T) {
	skipIfClusterUnavailable(t)

	prefix := fmt.Sprintf("cost%d:", time.Now().UnixNano())
	be := newClusterBackend(t, prefix)
	defer be.Close()

	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     10,
		RefillRate:   10,
		RefillPeriod: time.Minute,
	})

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
	key := fmt.Sprintf("cluster-cost-%d", time.Now().UnixNano())

	// 2 expensive ops = 10 tokens
	for i := 0; i < 2; i++ {
		r := limiter.AllowWithCost(ctx, key, 5)
		if r.Error != nil {
			t.Fatalf("expensive op %d error: %v", i, r.Error)
		}
		if !r.Allowed {
			t.Fatalf("expensive op %d should be allowed", i)
		}
	}

	// Even cheap should fail now
	r := limiter.AllowWithCost(ctx, key, 1)
	if r.Allowed {
		t.Error("should be denied after exhausting budget")
	}
}

// ==========================================================================
// Test 9: State roundtrip across cluster — write with one limiter, read with another
// ==========================================================================

func TestCluster_StateRoundtrip(t *testing.T) {
	skipIfClusterUnavailable(t)

	prefix := fmt.Sprintf("rt%d:", time.Now().UnixNano())
	be := newClusterBackend(t, prefix)
	defer be.Close()

	ctx := context.Background()
	key := fmt.Sprintf("roundtrip-cluster-%d", time.Now().UnixNano())

	// Limiter 1: consume 3 of 10 tokens
	algo1 := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity: 10, RefillRate: 10, RefillPeriod: time.Minute,
	})
	lim1, err := basic.NewBasicLimiter(algo1, be, limiters.BasicConfig{
		DefaultLimit: 10, DefaultWindow: time.Minute, KeyTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lim1.Close()

	for i := 0; i < 3; i++ {
		r := lim1.Allow(ctx, key)
		if !r.Allowed {
			t.Fatalf("lim1 request %d should be allowed", i)
		}
	}

	// Limiter 2: same backend, same key — should see 7 remaining
	algo2 := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity: 10, RefillRate: 10, RefillPeriod: time.Minute,
	})
	lim2, err := basic.NewBasicLimiter(algo2, be, limiters.BasicConfig{
		DefaultLimit: 10, DefaultWindow: time.Minute, KeyTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lim2.Close()

	for i := 0; i < 7; i++ {
		r := lim2.Allow(ctx, key)
		if !r.Allowed {
			t.Fatalf("lim2 request %d should be allowed (7 remaining)", i)
		}
	}

	// Should now be exhausted
	r := lim2.Allow(ctx, key)
	if r.Allowed {
		t.Fatal("should be denied after roundtrip")
	}
}

// ==========================================================================
// Test 10: IncrementAndGet on cluster
// ==========================================================================

func TestCluster_IncrementAndGet(t *testing.T) {
	skipIfClusterUnavailable(t)

	prefix := fmt.Sprintf("incr%d:", time.Now().UnixNano())
	be := newClusterBackend(t, prefix)
	defer be.Close()

	// IncrementAndGet is on the RedisBackend interface
	rb, ok := be.(interface {
		IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error)
	})
	if !ok {
		t.Fatal("backend does not implement IncrementAndGet")
	}

	ctx := context.Background()
	key := fmt.Sprintf("counter-%d", time.Now().UnixNano())

	for i := int64(1); i <= 5; i++ {
		val, err := rb.IncrementAndGet(ctx, key, 30*time.Second)
		if err != nil {
			t.Fatalf("IncrementAndGet(%d): %v", i, err)
		}
		if val != i {
			t.Errorf("IncrementAndGet(%d) = %d, want %d", i, val, i)
		}
	}
}

// ==========================================================================
// Test 11: All 5 algorithms work on cluster
// ==========================================================================

func TestCluster_AllAlgorithms(t *testing.T) {
	skipIfClusterUnavailable(t)

	algorithms := []struct {
		name  string
		build func() algorithm.Algorithm
		limit int
	}{
		{"TokenBucket", func() algorithm.Algorithm {
			return algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
				Capacity: 5, RefillRate: 5, RefillPeriod: time.Minute,
			})
		}, 5},
		{"LeakyBucket", func() algorithm.Algorithm {
			return algorithm.NewLeakyBucket(algorithm.LeakyBucketConfig{
				Capacity: 5, LeakRate: 5, LeakPeriod: time.Minute,
			})
		}, 5},
		{"FixedWindow", func() algorithm.Algorithm {
			return algorithm.NewFixedWindow(algorithm.FixedWindowConfig{
				Limit: 5, Window: time.Minute,
			})
		}, 5},
		{"SlidingWindow", func() algorithm.Algorithm {
			return algorithm.NewSlidingWindow(algorithm.SlidingWindowConfig{
				Limit: 5, Window: time.Minute,
			})
		}, 5},
		{"GCRA", func() algorithm.Algorithm {
			return algorithm.NewGCRA(algorithm.GCRAConfig{
				Limit: 5, Period: time.Minute, BurstSize: 5,
			})
		}, 5},
	}

	for _, alg := range algorithms {
		t.Run(alg.name, func(t *testing.T) {
			prefix := fmt.Sprintf("algo%s%d:", alg.name, time.Now().UnixNano())
			be := newClusterBackend(t, prefix)
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
			key := fmt.Sprintf("algo-%s-%d", alg.name, time.Now().UnixNano())

			// Allow up to limit
			for i := 0; i < alg.limit; i++ {
				r := limiter.Allow(ctx, key)
				if r.Error != nil {
					t.Fatalf("request %d error: %v", i, r.Error)
				}
				if !r.Allowed {
					t.Fatalf("request %d should be allowed", i)
				}
			}

			// Over limit → denied
			r := limiter.Allow(ctx, key)
			if r.Error != nil {
				t.Fatalf("over-limit error: %v", r.Error)
			}
			if r.Allowed {
				t.Fatalf("over-limit should be denied (algo=%s)", alg.name)
			}
		})
	}
}
