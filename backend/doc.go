// Package backend defines storage interfaces and helpers for rate limiter state.
//
// It provides the [Backend] and [AtomicBackend] interfaces that all storage
// implementations must satisfy, plus the [Envelope] type for typed JSON
// serialization used by Redis and PostgreSQL backends.
//
// Concrete implementations live in sub-packages:
//   - [github.com/abhipray-cpu/niyantrak/backend/memory] — in-process with optional GC
//   - [github.com/abhipray-cpu/niyantrak/backend/redis] — Redis with Lua CAS
//   - [github.com/abhipray-cpu/niyantrak/backend/postgresql] — PostgreSQL with row locking
//   - [github.com/abhipray-cpu/niyantrak/backend/custom] — user-supplied implementation
package backend
