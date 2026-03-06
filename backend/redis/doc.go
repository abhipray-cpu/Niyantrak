// Package redis provides a Redis-backed rate limiter storage backend.
//
// It uses a Lua Compare-And-Swap (CAS) script for atomic read-modify-write
// operations, avoiding WATCH/MULTI/EXEC and working correctly with Redis
// Cluster. The backend accepts a [github.com/redis/go-redis/v9.UniversalClient],
// transparently supporting Standalone, Sentinel, and Cluster topologies.
//
// Constructors:
//   - [NewRedisBackend] — connect by address, DB index, and key prefix
//   - [NewRedisBackendFromOptions] — connect with full [RedisOptions]
//   - [NewRedisBackendFromClient] — wrap an existing UniversalClient
package redis
