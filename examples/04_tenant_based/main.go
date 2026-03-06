// Example 4: Tenant-Based Rate Limiting
//
// Multi-tenant SaaS where each organization has separate limits.
// Track usage per organization and aggregate across all users in that organization.
//
// Run with: go run examples/04_tenant_based/main.go

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend/memory"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/limiters/tenant"
)

func main() {
	fmt.Println("=== Example 4: Tenant-Based Rate Limiting ===")
	fmt.Println()

	backend := memory.NewMemoryBackend()
	defer backend.Close()

	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     50000,
		RefillRate:   5000,
		RefillPeriod: time.Hour,
	})

	config := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  5000,
			DefaultWindow: time.Hour,
		},
		DefaultTenant: "org:acme",
		Tenants: map[string]limiters.TenantLimit{
			"org:acme":     {Limit: 10000, Window: time.Hour},
			"org:techcorp": {Limit: 50000, Window: time.Hour},
			"org:startup":  {Limit: 5000, Window: time.Hour},
		},
	}

	limiter, err := tenant.NewTenantBasedLimiter(algo, backend, config)
	if err != nil {
		log.Fatalf("Failed to create tenant limiter: %v", err)
	}
	defer limiter.Close()
	fmt.Println("✓ Created TenantBasedLimiter")
	fmt.Println()

	ctx := context.Background()

	// Step 1: Configure tenant limits
	fmt.Println("--- Configuring tenants ---")
	tenants := map[string]int{
		"org:acme":     10000,
		"org:techcorp": 50000,
		"org:startup":  5000,
	}

	for tenantID, limit := range tenants {
		limiter.SetTenantLimit(ctx, tenantID, limit, time.Hour)
		fmt.Printf("✓ Set tenant '%s': %d req/hour\n", tenantID, limit)
	}
	fmt.Println()

	// Step 2: Assign users to tenants
	fmt.Println("--- Assigning users to tenants ---")
	userAssignments := map[string]string{
		"user:alice":   "org:acme",
		"user:bob":     "org:acme",
		"user:charlie": "org:techcorp",
		"user:diana":   "org:techcorp",
		"user:eve":     "org:startup",
	}

	for userID, tenantID := range userAssignments {
		limiter.AssignKeyToTenant(ctx, userID, tenantID)
		fmt.Printf("✓ Assigned %s to tenant '%s'\n", userID, tenantID)
	}
	fmt.Println()

	// Step 3: Test requests
	fmt.Println("--- Testing requests ---")
	for i := 0; i < 5; i++ {
		result := limiter.Allow(ctx, "user:alice")
		fmt.Printf("user:alice request %d - Remaining: %d/%d\n", i+1, result.Remaining, result.Limit)
	}
	fmt.Println()

	// Step 4: Get tenant stats (aggregated across all users)
	fmt.Println("--- Tenant Statistics ---")
	stats := limiter.GetTenantStats(ctx, "org:acme")
	fmt.Printf("Org ACME total requests: %d\n", stats.TotalRequests)
	fmt.Printf("Org ACME allowed: %d, denied: %d\n", stats.AllowedCount, stats.DeniedCount)
	fmt.Printf("Org ACME users: %d\n", stats.TotalKeys)
	fmt.Println()

	fmt.Println("✓ Example completed successfully!")
	fmt.Println("\nKey learnings:")
	fmt.Println("1. Each tenant has independent limits")
	fmt.Println("2. Multiple users share same tenant limit")
	fmt.Println("3. Get aggregated stats across tenant users")
	fmt.Println("4. Perfect for multi-tenant SaaS platforms")
}
