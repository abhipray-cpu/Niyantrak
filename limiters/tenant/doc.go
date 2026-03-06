// Package tenant provides a multi-tenant rate limiter that assigns keys to
// tenant identifiers, each with independent rate limits and statistics.
//
// When PersistMappings is enabled, key→tenant assignments are stored in the
// backend under a __tenant_mapping: prefix for distributed consistency.
package tenant
