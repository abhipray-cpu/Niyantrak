// Package tier provides a tier-based rate limiter that assigns keys to named
// tiers (e.g. "free", "pro", "enterprise"), each with its own rate limit.
//
// When PersistMappings is enabled, key→tier assignments are stored in the
// backend under a __tier_mapping: prefix so that all instances in a
// distributed deployment share a consistent view.
package tier
