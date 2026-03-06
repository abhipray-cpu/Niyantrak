// Example 5: Cost-Based Rate Limiting
//
// Operations have different costs. More expensive operations consume more quota.
// Perfect for APIs where some endpoints are more resource-intensive than others.
//
// Run with: go run examples/05_cost_based/main.go

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend/memory"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/limiters/cost"
)

func main() {
	fmt.Println("=== Example 5: Cost-Based Rate Limiting ===")
	fmt.Println()

	backend := memory.NewMemoryBackend()
	defer backend.Close()

	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     100,
		RefillRate:   20,
		RefillPeriod: time.Minute,
	})

	config := limiters.CostConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Minute,
		},
		Operations: map[string]int{
			"list_users":      1,
			"get_user":        2,
			"search":          5,
			"export_csv":      25,
			"generate_report": 50,
		},
		DefaultCost: 1,
	}

	limiter, err := cost.NewCostBasedLimiter(algo, backend, config)
	if err != nil {
		log.Fatalf("Failed to create cost limiter: %v", err)
	}
	defer limiter.Close()
	fmt.Println("✓ Created CostBasedLimiter")
	fmt.Println()

	ctx := context.Background()

	// Step 1: Define operation costs
	fmt.Println("--- Operation Costs ---")
	operations := map[string]int{
		"list_users":      1,
		"get_user":        2,
		"search":          5,
		"export_csv":      25,
		"generate_report": 50,
	}

	for op, cost := range operations {
		fmt.Printf("✓ %s: %d credits\n", op, cost)
	}
	fmt.Println()

	// Step 2: Test various operations for a user
	fmt.Println("--- Testing requests with different costs ---")
	userKey := "api_key:sk_test_123"
	requests := []struct {
		op   string
		cost int
	}{
		{"list_users", 1},
		{"get_user", 2},
		{"list_users", 1},
		{"search", 5},
		{"get_user", 2},
		{"export_csv", 25},
		{"generate_report", 50},
	}

	for i, req := range requests {
		result := limiter.AllowWithCost(ctx, userKey, req.cost)
		status := "✓ ALLOWED"
		if !result.Allowed {
			status = "✗ DENIED"
		}
		fmt.Printf("Request %d - %s (cost: %d) - %s (remaining: %d/%d)\n",
			i+1, req.op, req.cost, status, result.Remaining, result.Limit)
	}
	fmt.Println()

	// Step 3: Batch operation
	fmt.Println("--- Batch check for multiple operations ---")
	ops := []struct {
		name string
		cost int
	}{
		{"search", 5},
		{"export_csv", 25},
	}

	totalCost := 0
	for _, op := range ops {
		totalCost += op.cost
	}

	if totalCost <= 50 {
		fmt.Printf("✓ Batch operation allowed (total cost: %d)\n", totalCost)
	} else {
		fmt.Printf("✗ Batch operation denied (total cost: %d exceeds budget)\n", totalCost)
	}
	fmt.Println()

	// Step 4: Final request to see updated state
	fmt.Println("--- Final state ---")
	result := limiter.AllowWithCost(ctx, userKey, 10)
	if result.Allowed {
		fmt.Printf("✓ Final request allowed\n")
	} else {
		fmt.Printf("✗ Final request denied (insufficient quota)\n")
	}
	fmt.Printf("Remaining quota: %d/%d\n", result.Remaining, result.Limit)
	fmt.Println()

	fmt.Println("✓ Example completed successfully!")
	fmt.Println("\nKey learnings:")
	fmt.Println("1. Different operations have different costs")
	fmt.Println("2. Check AllowWithCost before executing expensive operations")
	fmt.Println("3. Combine low-cost and high-cost operations efficiently")
	fmt.Println("4. Perfect for APIs with variable resource requirements")
}
