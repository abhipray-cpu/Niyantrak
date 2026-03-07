package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/abhipray-cpu/niyantrak/backend"
	goredis "github.com/redis/go-redis/v9"
)

// RedisOptions configures a Redis backend with full control over
// connection pooling, timeouts, and topology (single-node, Sentinel, Cluster).
type RedisOptions struct {
	// Addrs is a list of host:port addresses.
	// Single element  → standalone Redis.
	// Multiple        → Redis Cluster or Sentinel depending on MasterName.
	Addrs []string

	// DB selects a database (standalone/Sentinel only; Cluster always uses 0).
	DB int

	// MasterName, when set, enables Sentinel mode.
	MasterName string

	// Password for Redis AUTH.
	Password string

	// Prefix is prepended to every key for namespacing.
	Prefix string

	// DialTimeout is the maximum time to establish a connection.
	DialTimeout time.Duration

	// ReadTimeout is the maximum time to wait for a response.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum time to wait for a write to complete.
	WriteTimeout time.Duration

	// PoolSize is the maximum number of socket connections.
	PoolSize int

	// MinIdleConns is the minimum number of idle connections to keep.
	MinIdleConns int

	// MaxRetries is the maximum number of command retries before giving up.
	MaxRetries int
}

// luaCAS is a Lua script that performs an atomic Compare-And-Swap:
//  1. GET current value
//  2. Return it to the caller (via EVALSHA return)
//
// The actual CAS loop is done client-side: read → transform → conditional write.
// We use a separate Lua script for the write side to keep atomicity.
//
// luaConditionalSet atomically:
//  1. GET current value
//  2. If it matches the expected old value, SET the new value + TTL
//  3. Return "OK" on success, or the actual current value on conflict
var luaConditionalSet = goredis.NewScript(`
local cur = redis.call('GET', KEYS[1])
local expected = ARGV[1]
local newval   = ARGV[2]
local ttl_ms   = tonumber(ARGV[3])

-- Treat nil/false from GET as empty string for comparison
if cur == false then cur = "" end

if cur ~= expected then
    return {"CONFLICT", cur}
end

if ttl_ms > 0 then
    redis.call('SET', KEYS[1], newval, 'PX', ttl_ms)
else
    redis.call('SET', KEYS[1], newval)
end
return {"OK", ""}
`)

// redisBackend is a Redis-based distributed storage backend
type redisBackend struct {
	client goredis.UniversalClient
	prefix string
}

// Compile-time checks
var _ backend.RedisBackend = (*redisBackend)(nil)
var _ backend.AtomicBackend = (*redisBackend)(nil)

// NewRedisBackend creates a new Redis backend with simple parameters.
// For advanced configuration (Cluster, Sentinel, timeouts) use
// NewRedisBackendFromOptions.
func NewRedisBackend(addr string, db int, prefix string) backend.RedisBackend {
	return NewRedisBackendFromOptions(RedisOptions{
		Addrs:  []string{addr},
		DB:     db,
		Prefix: prefix,
	})
}

// NewRedisBackendFromOptions creates a Redis backend from full options.
func NewRedisBackendFromOptions(opts RedisOptions) backend.RedisBackend {
	uniOpts := &goredis.UniversalOptions{
		Addrs:        opts.Addrs,
		DB:           opts.DB,
		MasterName:   opts.MasterName,
		Password:     opts.Password,
		DialTimeout:  opts.DialTimeout,
		ReadTimeout:  opts.ReadTimeout,
		WriteTimeout: opts.WriteTimeout,
		PoolSize:     opts.PoolSize,
		MinIdleConns: opts.MinIdleConns,
		MaxRetries:   opts.MaxRetries,
	}

	return &redisBackend{
		client: goredis.NewUniversalClient(uniOpts),
		prefix: opts.Prefix,
	}
}

// NewRedisBackendFromClient creates a Redis backend from a pre-configured
// UniversalClient. This is useful when you need full control over the client
// configuration (e.g. custom Dialer for Docker/Cluster address remapping).
func NewRedisBackendFromClient(client goredis.UniversalClient, prefix string) backend.RedisBackend {
	return &redisBackend{
		client: client,
		prefix: prefix,
	}
}

// prefixedKey returns the key with the optional prefix prepended.
func (r *redisBackend) prefixedKey(key string) string {
	if r.prefix == "" {
		return key
	}
	return r.prefix + key
}

// Get retrieves the current state for a key
func (r *redisBackend) Get(ctx context.Context, key string) (interface{}, error) {
	data, err := r.client.Get(ctx, r.prefixedKey(key)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, backend.ErrKeyNotFound
		}
		return nil, err
	}

	return backend.Unwrap(data)
}

// Set updates the state for a key with optional TTL
func (r *redisBackend) Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error {
	data, err := backend.Wrap(state)
	if err != nil {
		return err
	}

	if ttl > 0 {
		return r.client.Set(ctx, r.prefixedKey(key), data, ttl).Err()
	}
	return r.client.Set(ctx, r.prefixedKey(key), data, 0).Err()
}

// Update performs an atomic read-modify-write using a Lua CAS loop.
// It retries up to 10 times on conflict.
func (r *redisBackend) Update(ctx context.Context, key string, ttl time.Duration, fn backend.UpdateFunc) (interface{}, error) {
	pk := r.prefixedKey(key)
	const maxRetries = 10

	for attempt := 0; attempt < maxRetries; attempt++ {
		// 1. Read current value
		oldBytes, err := r.client.Get(ctx, pk).Bytes()
		if err != nil && !errors.Is(err, goredis.Nil) {
			return nil, err
		}

		// Deserialise current state
		var current interface{}
		oldStr := ""
		if !errors.Is(err, goredis.Nil) {
			oldStr = string(oldBytes)
			current, err = backend.Unwrap(oldBytes)
			if err != nil {
				return nil, fmt.Errorf("redis: unwrap current state: %w", err)
			}
		}

		// 2. Apply the transform
		newState, result, fnErr := fn(current)
		if fnErr != nil {
			return nil, fnErr
		}

		// 3. Serialise new state
		newBytes, err := backend.Wrap(newState)
		if err != nil {
			return nil, fmt.Errorf("redis: wrap new state: %w", err)
		}

		ttlMs := int64(0)
		if ttl > 0 {
			ttlMs = ttl.Milliseconds()
		}

		// 4. Conditional set via Lua
		res, err := luaConditionalSet.Run(ctx, r.client, []string{pk},
			oldStr, string(newBytes), ttlMs,
		).StringSlice()
		if err != nil {
			return nil, fmt.Errorf("redis: lua CAS: %w", err)
		}

		if res[0] == "OK" {
			return result, nil
		}
		// CONFLICT — another writer changed the value; retry
	}

	return nil, fmt.Errorf("redis: CAS failed after %d retries for key %q", maxRetries, key)
}

// IncrementAndGet atomically increments the value for a key and returns new value
func (r *redisBackend) IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pk := r.prefixedKey(key)

	val, err := r.client.Incr(ctx, pk).Result()
	if err != nil {
		return 0, err
	}

	// Set expiration if this is the first increment (val == 1) or always refresh
	if ttl > 0 {
		r.client.Expire(ctx, pk, ttl)
	}

	return val, nil
}

// Delete removes a key from the backend
func (r *redisBackend) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, r.prefixedKey(key)).Err()
}

// Close cleans up the Redis connection
func (r *redisBackend) Close() error {
	return r.client.Close()
}

// Ping checks if the Redis connection is healthy
func (r *redisBackend) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Type returns the backend type name
func (r *redisBackend) Type() string {
	return "redis"
}

// GetConnection returns Redis connection information
func (r *redisBackend) GetConnection(ctx context.Context) *backend.ConnectionInfo {
	// Extract host/port from the first address
	addr := ""
	switch c := r.client.(type) {
	case *goredis.Client:
		addr = c.Options().Addr
	case *goredis.ClusterClient:
		// Use first address
		opts := c.Options()
		if len(opts.Addrs) > 0 {
			addr = opts.Addrs[0]
		}
	}

	host := addr
	port := 6379
	if parts := strings.SplitN(addr, ":", 2); len(parts) == 2 {
		host = parts[0]
		if p, err := strconv.Atoi(parts[1]); err == nil {
			port = p
		}
	}

	isConnected := r.Ping(ctx) == nil

	return &backend.ConnectionInfo{
		Host:        host,
		Port:        port,
		IsConnected: isConnected,
	}
}

// CheckCluster checks if connected to a Redis cluster
func (r *redisBackend) CheckCluster(ctx context.Context) (bool, error) {
	_, isCluster := r.client.(*goredis.ClusterClient)
	return isCluster, nil
}

// GetReplication checks Redis replication status
func (r *redisBackend) GetReplication(ctx context.Context) *backend.ReplicationStatus {
	info, err := r.client.Info(ctx, "replication").Result()
	if err != nil {
		return &backend.ReplicationStatus{Status: "error: " + err.Error()}
	}

	status := &backend.ReplicationStatus{Status: "ok"}

	for _, line := range strings.Split(info, "\r\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "role":
			status.Role = parts[1]
		case "connected_slaves":
			if n, err := strconv.Atoi(parts[1]); err == nil {
				status.ConnectedSlaves = n
			}
		case "master_repl_offset":
			if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				status.Offset = n
			}
		case "repl_backlog_size":
			if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				status.BacklogSize = n
			}
		}
	}

	return status
}

// Flush flushes all data from the current database
func (r *redisBackend) Flush(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

// Scan scans keys matching a pattern
func (r *redisBackend) Scan(ctx context.Context, pattern string, count int) ([]string, error) {
	var allKeys []string
	var cursor uint64

	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, r.prefixedKey(pattern), int64(count)).Result()
		if err != nil {
			return nil, err
		}

		allKeys = append(allKeys, keys...)
		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	return allKeys, nil
}
