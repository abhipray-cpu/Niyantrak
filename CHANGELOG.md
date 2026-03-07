# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] — 2026-03-07

### Added

- **5 rate limiting algorithms** — Token Bucket, Leaky Bucket, Fixed Window,
  Sliding Window (O(1)), GCRA — all with clock injection for deterministic
  testing.
- **4 storage backends** — Memory (with configurable GC), Redis (Lua CAS,
  UniversalClient supporting Standalone / Sentinel / Cluster),
  PostgreSQL (SELECT FOR UPDATE + prefix sanitization), Custom.
- **Builder API** — `niyantrak.New()`, `NewTierBased()`, `NewTenantBased()`,
  `NewCostBased()`, `NewComposite()` with functional options.
- **5 limiter types** — Basic, TierBased, TenantBased, CostBased, Composite.
- **Distributed key mappings** — `PersistMappings` for tier/tenant limiters
  stores assignments in the backend for multi-instance consistency.
- **HTTP middleware** — `net/http` handler with `X-RateLimit-*` headers and
  `Retry-After`.
- **gRPC interceptors** — Unary + streaming with `RetryInfo` and
  `QuotaFailure` error details.
- **Framework adapters** — Gin, Chi, Echo, Fiber.
- **Observability** — opt-in interfaces in `obstypes` package with default
  no-op implementations; adapters for Zerolog, Prometheus, OpenTelemetry.
- **Dynamic limits** — runtime limit changes via `DynamicLimitManager`.
- **Failover** — `FailOpen`, `FailClosed`, `LocalFallback` strategies with
  periodic health-check recovery.
- **Typed serialization** — `backend.Wrap` / `backend.Unwrap` envelope for
  JSON-backed stores.
- **6 examples** — basic memory, basic Redis, tier-based, tenant-based,
  cost-based, composite.
- **Architecture docs** — C4 diagrams (System Context, Container, Component),
  request-flow sequence, algorithm/backend/limiter deep-dives.
- **CI pipeline** — GitHub Actions with test (5 Go versions × 3 OS),
  coverage (Codecov), lint (golangci-lint, staticcheck), security (gosec),
  build, documentation checks.
- **Integration tests** — Redis standalone, PostgreSQL, and 6-node Redis
  Cluster with custom dialer address remapping.
