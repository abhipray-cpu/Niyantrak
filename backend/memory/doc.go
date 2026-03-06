// Package memory provides an in-process rate limiter backend using sync.RWMutex.
//
// Use [NewMemoryBackend] for a simple store where expired entries are cleaned
// lazily on read, or [NewMemoryBackendWithGC] to add a periodic background
// garbage-collection goroutine that prevents unbounded growth under
// write-heavy workloads.
//
// The memory backend implements both [github.com/abhipray-cpu/niyantrak/backend.Backend]
// and [github.com/abhipray-cpu/niyantrak/backend.AtomicBackend].
package memory
