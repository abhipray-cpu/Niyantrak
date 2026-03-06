// Package features provides runtime extensions for rate limiters:
// dynamic limit adjustment and backend failover.
//
//   - [DynamicLimitManager] — change limits at runtime based on external
//     signals (load, time of day, admin controls).
//   - [FailoverHandler] — automatically switch to a fallback when the primary
//     backend is unavailable, with periodic health-check recovery.
//     Strategies: FailOpen, FailClosed, LocalFallback.
package features
