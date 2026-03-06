// Package cost provides a cost-aware rate limiter where different operations
// consume different amounts of quota.
//
// Use [AllowWithCost] to deduct a named operation's cost, and [SetOperationCost]
// to define or update costs at runtime.
package cost
