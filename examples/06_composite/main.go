// Example 6: Composite Rate Limiting
//
// Apply multiple rate limiting rules simultaneously.
// E.g., limit both per-second AND per-hour to enforce both short and long-term constraints.
//
// Run with: go run examples/06_composite/main.go

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend/memory"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/limiters/composite"
)

func main() {
	fmt.Println("=== Example 6: Composite Rate Limiting ===")
	fmt.Println()

	backend := memory.NewMemoryBackend()
	defer backend.Close()

	// Create first limiter: 10 requests per second
	algo1 := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     10,
		RefillRate:   10,
		RefillPeriod: time.Second,
	})

	config := limiters.CompositeConfig{
		Name: "burst_and_quota_protection",
		Limits: []limiters.LimitConfig{
			{
				Name:   "per_second",
				Limit:  10,
				Window: time.Second,
			},
			{
				Name:   "per_minute",
				Limit:  100,
				Window: time.Minute,
			},
		},
	}

	limiter, err := composite.NewCompositeLimiter(algo1, backend, config)
	if err != nil {
		log.Fatalf("Failed to create composite limiter: %v", err)
	}
	defer limiter.Close()
	fmt.Println("✓ Created CompositeLimiter")
	fmt.Println("  - Per-second burst protection: 10 req/sec")
	fmt.Println("  - Per-minute quota: 100 req/min")
	fmt.Println()

	ctx := context.Background()

	// Step 1: Quick burst - should hit per-second limit
	fmt.Println("--- Testing quick burst (12 requests in 100ms) ---")
	userKey := "user:rapid"
	for i := 0; i < 12; i++ {
		result := limiter.Allow(ctx, userKey)
		status := "✓"
		if !result.Allowed {
			status = "✗"
		}
		fmt.Printf("Request %2d - %s (remaining: %d/%d)\n", i+1, status, result.Remaining, result.Limit)
	}
	fmt.Println()

	// Step 2: Steady requests - should be allowed
	fmt.Println("--- Testing steady requests (1 per 200ms) ---")
	for i := 0; i < 5; i++ {
		time.Sleep(200 * time.Millisecond)
		result := limiter.Allow(ctx, userKey)
		status := "✓"
		if !result.Allowed {
			status = "✗"
		}
		fmt.Printf("Request %d - %s (remaining: %d/%d)\n", i+1, status, result.Remaining, result.Limit)
	}
	fmt.Println()

	// Step 3: Different user - independent limits
	fmt.Println("--- Different user with same limits ---")
	userKey2 := "user:steady"
	for i := 0; i < 3; i++ {
		result := limiter.Allow(ctx, userKey2)
		fmt.Printf("Request %d - remaining: %d/%d\n", i+1, result.Remaining, result.Limit)
	}
	fmt.Println()

	// Step 4: Demonstrate why composite is needed
	fmt.Println("--- Why Composite Limiting Matters ---")
	fmt.Println("Scenario: Netflix-like video platform")
	fmt.Println("  Rule 1: Max 100 requests per 10 seconds (handles burst playback adjustments)")
	fmt.Println("  Rule 2: Max 1000 requests per hour (ensures fair resource distribution)")
	fmt.Println()
	fmt.Println("Without composite:")
	fmt.Println("  - Only per-second limit → allows 360k req/hour if spread evenly")
	fmt.Println("  - Only per-hour limit → allows 3.3k req/second in one second")
	fmt.Println()
	fmt.Println("With composite (AND):")
	fmt.Println("  - Must respect BOTH limits simultaneously")
	fmt.Println("  - True protection against both burst and sustained abuse")
	fmt.Println()

	fmt.Println("✓ Example completed successfully!")
	fmt.Println("\nKey learnings:")
	fmt.Println("1. Combine multiple algorithms for comprehensive protection")
	fmt.Println("2. Protect against both burst (short-term) and abuse (long-term)")
	fmt.Println("3. Use AND logic when both limits must be satisfied")
	fmt.Println("4. Each key tracks state across all algorithms independently")
	fmt.Println("5. Perfect for production APIs needing multi-layer defense")
}
