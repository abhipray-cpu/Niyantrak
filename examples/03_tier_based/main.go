// Example 3: Tier-Based Rate Limiting
//
// This example demonstrates rate limiting different users based on their tier.
// Common in SaaS: Free, Pro, Enterprise users have different limits.
//
// Run with: go run examples/03_tier_based/main.go

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend/memory"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/limiters/tier"
)

func main() {
	fmt.Println("=== Example 3: Tier-Based Rate Limiting ===")
	fmt.Println()

	// Step 1: Create backend and algorithm
	backend := memory.NewMemoryBackend()
	defer backend.Close()

	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     1000,
		RefillRate:   100,
		RefillPeriod: time.Second,
	})

	// Step 2: Create tier-based limiter
	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Minute,
		},
		DefaultTier: "free",
		Tiers: map[string]limiters.TierLimit{
			"free":       {Limit: 100, Window: time.Hour},
			"pro":        {Limit: 10000, Window: time.Hour},
			"enterprise": {Limit: 1000000, Window: time.Hour},
		},
	}

	limiter, err := tier.NewTierBasedLimiter(algo, backend, config)
	if err != nil {
		log.Fatalf("Failed to create tier limiter: %v", err)
	}
	defer limiter.Close()
	fmt.Println("✓ Created TierBasedLimiter")
	fmt.Println()

	ctx := context.Background()

	// Step 3: Configure tier limits
	fmt.Println("--- Configuring tier limits ---")
	tiers := map[string]struct {
		limit  int
		window time.Duration
	}{
		"free":       {limit: 100, window: time.Hour},
		"pro":        {limit: 10000, window: time.Hour},
		"enterprise": {limit: 1000000, window: time.Hour},
	}

	for tierName, tierConfig := range tiers {
		limiter.SetTierLimit(ctx, tierName, tierConfig.limit, tierConfig.window)
		fmt.Printf("✓ Set tier '%s': %d req/hour\n", tierName, tierConfig.limit)
	}
	fmt.Println()

	// Step 4: Assign users to tiers
	fmt.Println("--- Assigning users to tiers ---")
	users := map[string]string{
		"user:1": "free",
		"user:2": "pro",
		"user:3": "enterprise",
	}

	for userID, tierName := range users {
		limiter.AssignKeyToTier(ctx, userID, tierName)
		fmt.Printf("✓ Assigned %s to tier '%s'\n", userID, tierName)
	}
	fmt.Println()

	// Step 5: Test rate limits for each tier
	fmt.Println("--- Testing requests per tier ---")
	for userID, tierName := range users {
		// Make several requests for each user
		for i := 1; i <= 3; i++ {
			result := limiter.Allow(ctx, userID)
			status := "✓"
			if !result.Allowed {
				status = "✗"
			}
			fmt.Printf("%s %s (tier: %s) - Request %d - %s\n", status, userID, tierName, i, "ALLOWED")
		}
		fmt.Println()
	}

	fmt.Println("✓ Example completed successfully!")
	fmt.Println("\nKey learnings:")
	fmt.Println("1. Configure different limits per tier")
	fmt.Println("2. Assign users to tiers dynamically")
	fmt.Println("3. Each user gets independent limit tracking")
	fmt.Println("4. Perfect for SaaS subscription models")
	fmt.Println("5. Can change tiers without affecting other users")
}
