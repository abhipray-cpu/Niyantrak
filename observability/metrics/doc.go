// Package metrics provides a Prometheus-based adapter for the
// [github.com/abhipray-cpu/niyantrak/observability/types.Metrics] interface.
//
// It registers counters, histograms, and gauges that are scraped via the
// standard /metrics HTTP endpoint.
package metrics
