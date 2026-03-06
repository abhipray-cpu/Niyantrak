// Package basic provides the default single-key rate limiter implementation.
//
// A BasicLimiter combines one [github.com/abhipray-cpu/niyantrak/algorithm.Algorithm]
// with one [github.com/abhipray-cpu/niyantrak/backend.Backend] to perform
// per-key rate limiting via [Allow] and [AllowN].
package basic
