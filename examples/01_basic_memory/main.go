// Example 1: Basic Rate Limiting with Memory Backend
//
// This example demonstrates the simplest use case: single-key rate limiting
// with an in-memory backend using the Token Bucket algorithm.
//
// Use this when:
// - Single instance application
// - Development/testing
// - Simple per-user rate limiting
// - Highest performance needed
//
// Run with: go run examples/01_basic_memory/main.go

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend/memory"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/limiters/basic"
)

func main() {
	fmt.Println("=== Example 1: Basic Rate Limiting (Memory Backend) ===")
	fmt.Println()

	// Step 1: Create backend (in-memory storage)
	backend := memory.NewMemoryBackend()
	defer backend.Close()
	fmt.Println("✓ Created in-memory backend")

	// Step 2: Create algorithm (Token Bucket)
	tokenBucketAlgo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     100,         // Max 100 tokens
		RefillRate:   10,          // Add 10 tokens per period
		RefillPeriod: time.Second, // Every second
	})
	fmt.Println("✓ Created Token Bucket algorithm (100 cap, 10 per sec)")

	// Step 3: Create limiter with configuration
	config := limiters.BasicConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		KeyTTL:        time.Hour,
	}

	limiter, err := basic.NewBasicLimiter(tokenBucketAlgo, backend, config)
	if err != nil {
		log.Fatalf("Failed to create limiter: %v", err)
	}
	defer limiter.Close()
	fmt.Println("✓ Created BasicLimiter")
	fmt.Println()

	// Step 4: Simulate requests from different users
	ctx := context.Background()

	fmt.Println("--- Testing rate limits ---")

	// User 1: Multiple requests
	for i := 1; i <= 5; i++ {
		result := limiter.Allow(ctx, "user:1")
		status := "✓ ALLOWED"
		if !result.Allowed {
			status = "✗ DENIED"
		}
		fmt.Printf("Request %d for user:1 - %s (Remaining: %d/%d)\n",
			i, status, result.Remaining, result.Limit)
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println()

	// User 2: Different key, independent limit
	for i := 1; i <= 3; i++ {
		result := limiter.Allow(ctx, "user:2")
		status := "✓ ALLOWED"
		if !result.Allowed {
			status = "✗ DENIED"
		}
		fmt.Printf("Request %d for user:2 - %s (Remaining: %d/%d)\n",
			i, status, result.Remaining, result.Limit)
	}

	fmt.Println()

	// Step 5: Demonstrate AllowN (multiple tokens at once)
	fmt.Println("--- Testing cost-based requests (AllowN) ---")
	result := limiter.AllowN(ctx, "user:3", 10) // Request 10 tokens
	status := "✓ ALLOWED"
	if !result.Allowed {
		status = "✗ DENIED"
	}
	fmt.Printf("Request 10 tokens for user:3 - %s (Remaining: %d/%d)\n",
		status, result.Remaining, result.Limit)

	fmt.Println()

	// Step 6: Get statistics
	fmt.Println("--- Statistics ---")
	stats := limiter.GetStats(ctx, "user:1")
	fmt.Printf("Stats for user:1: %+v\n", stats)

	fmt.Println()
	fmt.Println("✓ Example completed successfully!")
	fmt.Println("\nKey learnings:")
	fmt.Println("1. Backend provides storage (memory, redis, postgresql)")
	fmt.Println("2. Algorithm implements rate limiting logic (5 types available)")
	fmt.Println("3. Limiter orchestrates backend + algorithm")
	fmt.Println("4. Each key has independent rate limit state")
	fmt.Println("5. AllowN() allows consuming multiple tokens at once")
}
