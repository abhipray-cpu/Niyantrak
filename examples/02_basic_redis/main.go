// Example 2: Basic Rate Limiting with Redis Backend
//
// This example shows how to use Redis for distributed rate limiting.
// Useful when you have multiple instances that need to share rate limit state.
//
// Run with: go run examples/02_basic_redis/main.go
// Note: Requires Redis running on localhost:6379

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	redisbackend "github.com/abhipray-cpu/niyantrak/backend/redis"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/limiters/basic"
)

func main() {
	fmt.Println("=== Example 2: Basic Rate Limiting (Redis Backend) ===")
	fmt.Println()

	// Step 1: Create Redis backend
	// Parameters: address, db number, key prefix
	backend := redisbackend.NewRedisBackend("localhost:6379", 0, "rate_limit:")
	defer backend.Close()
	fmt.Println("✓ Created Redis backend with prefix 'rate_limit:'")

	// Step 3: Create algorithm
	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     100,
		RefillRate:   10,
		RefillPeriod: time.Second,
	})
	fmt.Println("✓ Created Token Bucket algorithm")

	// Step 4: Create limiter
	config := limiters.BasicConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		KeyTTL:        time.Hour,
	}

	limiter, err := basic.NewBasicLimiter(algo, backend, config)
	if err != nil {
		log.Fatalf("Failed to create limiter: %v", err)
	}
	defer limiter.Close()
	fmt.Println("✓ Created BasicLimiter with Redis backend")
	fmt.Println()

	// Step 5: Simulate requests (shared across instances)
	ctx := context.Background()
	fmt.Println("--- Testing distributed rate limits ---")

	for i := 1; i <= 5; i++ {
		result := limiter.Allow(ctx, "api:user:1")
		status := "✓ ALLOWED"
		if !result.Allowed {
			status = "✗ DENIED"
		}
		fmt.Printf("Request %d - %s (Remaining: %d/%d)\n",
			i, status, result.Remaining, result.Limit)
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println()
	fmt.Println("✓ Example completed successfully!")
	fmt.Println("\nKey differences from memory backend:")
	fmt.Println("1. State shared across multiple instances")
	fmt.Println("2. Requires Redis to be running")
	fmt.Println("3. Slightly higher latency (~5ms) but still fast")
	fmt.Println("4. Persistent across restarts (within Redis TTL)")
	fmt.Println("5. Better for production distributed systems")
}
