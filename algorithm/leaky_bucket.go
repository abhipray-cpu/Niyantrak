package algorithm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// LeakyBucketState represents the state of a leaky bucket
type LeakyBucketState struct {
	// QueueSize is the current number of requests in the queue
	QueueSize int

	// LastLeakTime is when requests were last leaked
	LastLeakTime time.Time
}

// LeakyBucketResult represents the result of a leaky bucket check
type LeakyBucketResult struct {
	// Allowed indicates if the request is allowed
	Allowed bool

	// QueueSize is the current queue size after this request
	QueueSize int

	// QueueCapacity is the maximum queue capacity
	QueueCapacity int

	// ResetTime is when the queue will be empty
	ResetTime time.Time

	// RetryAfter is how long to wait before retrying (if denied)
	RetryAfter time.Duration
}

// LeakyBucketStats represents statistics for the leaky bucket
type LeakyBucketStats struct {
	// CurrentQueueSize is the current number of requests in queue
	CurrentQueueSize int

	// Capacity is the maximum queue capacity
	Capacity int

	// LeakRate is requests processed per leak period
	LeakRate int

	// LeakPeriod is the time between leak operations
	LeakPeriod time.Duration

	// LastLeakTime is when requests were last leaked
	LastLeakTime time.Time

	// NextLeakTime is when requests will next be leaked
	NextLeakTime time.Time

	// EstimatedWaitTime is estimated time for queue to empty
	EstimatedWaitTime time.Duration
}

// leakyBucket implements the Algorithm interface for Leaky Bucket rate limiting
type leakyBucket struct {
	config LeakyBucketConfig
	clock  func() time.Time
}

// Compile-time check to ensure leakyBucket implements Algorithm interface
var _ Algorithm = (*leakyBucket)(nil)

// NewLeakyBucket creates a new Leaky Bucket algorithm instance
func NewLeakyBucket(config LeakyBucketConfig) Algorithm {
	return NewLeakyBucketWithClock(config, nil)
}

// NewLeakyBucketWithClock creates a new Leaky Bucket algorithm instance with a custom clock.
// If clock is nil, time.Now is used.
func NewLeakyBucketWithClock(config LeakyBucketConfig, clock Clock) Algorithm {
	if clock == nil {
		clock = time.Now
	}
	return &leakyBucket{config: config, clock: clock}
}

// Allow checks if a request is allowed and updates the bucket state
func (lb *leakyBucket) Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error) {
	if state == nil {
		// Initialize state
		now := lb.clock()
		state = &LeakyBucketState{
			QueueSize:    0,
			LastLeakTime: now,
		}
	}

	bucketState, ok := state.(*LeakyBucketState)
	if !ok {
		return nil, nil, errors.New("invalid state type for leaky bucket")
	}

	now := lb.clock()

	// Leak requests based on elapsed time
	leakedState := lb.leakRequests(*bucketState, now)

	// Check if we have space in the queue for this request
	if leakedState.QueueSize+cost <= lb.config.Capacity {
		// Allow the request by adding to queue
		newState := LeakyBucketState{
			QueueSize:    leakedState.QueueSize + cost,
			LastLeakTime: leakedState.LastLeakTime,
		}

		result := LeakyBucketResult{
			Allowed:       true,
			QueueSize:     newState.QueueSize,
			QueueCapacity: lb.config.Capacity,
			ResetTime:     lb.calculateResetTime(newState, now),
		}

		return &newState, &result, nil
	}

	// Deny the request - queue is full
	result := LeakyBucketResult{
		Allowed:       false,
		QueueSize:     leakedState.QueueSize,
		QueueCapacity: lb.config.Capacity,
		ResetTime:     lb.calculateResetTime(leakedState, now),
		RetryAfter:    lb.calculateRetryAfter(leakedState, cost, now),
	}

	return &leakedState, &result, nil
}

// Reset resets the leaky bucket to initial state
func (lb *leakyBucket) Reset(ctx context.Context) (interface{}, error) {
	now := lb.clock()
	state := &LeakyBucketState{
		QueueSize:    0,
		LastLeakTime: now,
	}

	return state, nil
}

// GetStats returns current statistics for the leaky bucket
func (lb *leakyBucket) GetStats(ctx context.Context, state interface{}) interface{} {
	if state == nil {
		return &LeakyBucketStats{
			Capacity:   lb.config.Capacity,
			LeakRate:   lb.config.LeakRate,
			LeakPeriod: lb.config.LeakPeriod,
		}
	}

	bucketState, ok := state.(*LeakyBucketState)
	if !ok {
		return nil
	}

	nextLeak := bucketState.LastLeakTime.Add(lb.config.LeakPeriod)

	// Calculate estimated wait time for queue to empty
	var waitTime time.Duration
	if bucketState.QueueSize > 0 {
		periodsNeeded := (bucketState.QueueSize + lb.config.LeakRate - 1) / lb.config.LeakRate // Ceiling division
		waitTime = time.Duration(periodsNeeded) * lb.config.LeakPeriod
	}

	return &LeakyBucketStats{
		CurrentQueueSize:  bucketState.QueueSize,
		Capacity:          lb.config.Capacity,
		LeakRate:          lb.config.LeakRate,
		LeakPeriod:        lb.config.LeakPeriod,
		LastLeakTime:      bucketState.LastLeakTime,
		NextLeakTime:      nextLeak,
		EstimatedWaitTime: waitTime,
	}
}

// ValidateConfig validates the leaky bucket configuration
func (lb *leakyBucket) ValidateConfig(config interface{}) error {
	lbConfig, ok := config.(LeakyBucketConfig)
	if !ok {
		return errors.New("invalid config type for leaky bucket")
	}

	if lbConfig.Capacity <= 0 {
		return errors.New("capacity must be positive")
	}

	if lbConfig.LeakRate <= 0 {
		return errors.New("leak rate must be positive")
	}

	if lbConfig.LeakPeriod <= 0 {
		return errors.New("leak period must be positive")
	}

	return nil
}

// Name returns the algorithm name
func (lb *leakyBucket) Name() string {
	return "leaky_bucket"
}

// Description returns a human-readable description
func (lb *leakyBucket) Description() string {
	return fmt.Sprintf("Leaky Bucket: capacity=%d, leak_rate=%d/%v",
		lb.config.Capacity, lb.config.LeakRate, lb.config.LeakPeriod)
}

// leakRequests calculates how many requests should be leaked based on elapsed time
func (lb *leakyBucket) leakRequests(state LeakyBucketState, now time.Time) LeakyBucketState {
	if state.QueueSize == 0 {
		return LeakyBucketState{
			QueueSize:    0,
			LastLeakTime: now,
		}
	}

	elapsed := now.Sub(state.LastLeakTime)
	if elapsed <= 0 {
		return state
	}

	// Calculate how many leak periods have passed
	leakPeriods := int(elapsed / lb.config.LeakPeriod)
	if leakPeriods == 0 {
		return state
	}

	// Calculate how many requests to leak
	requestsToLeak := leakPeriods * lb.config.LeakRate

	// Calculate new queue size (cannot go below 0)
	newQueueSize := state.QueueSize - requestsToLeak
	if newQueueSize < 0 {
		newQueueSize = 0
	}

	// Update last leak time to the last complete leak period
	newLastLeakTime := state.LastLeakTime.Add(time.Duration(leakPeriods) * lb.config.LeakPeriod)

	return LeakyBucketState{
		QueueSize:    newQueueSize,
		LastLeakTime: newLastLeakTime,
	}
}

// calculateResetTime calculates when the queue will be empty
func (lb *leakyBucket) calculateResetTime(state LeakyBucketState, now time.Time) time.Time {
	if state.QueueSize == 0 {
		return now
	}

	// Calculate how many leak periods needed to empty the queue
	periodsNeeded := (state.QueueSize + lb.config.LeakRate - 1) / lb.config.LeakRate // Ceiling division
	timeNeeded := time.Duration(periodsNeeded) * lb.config.LeakPeriod

	return now.Add(timeNeeded)
}

// calculateRetryAfter calculates how long to wait before retrying
func (lb *leakyBucket) calculateRetryAfter(state LeakyBucketState, cost int, now time.Time) time.Duration {
	// Calculate how many requests need to leak to make space
	spaceNeeded := (state.QueueSize + cost) - lb.config.Capacity
	if spaceNeeded <= 0 {
		return 0
	}

	// Calculate how many leak periods needed
	periodsNeeded := (spaceNeeded + lb.config.LeakRate - 1) / lb.config.LeakRate // Ceiling division

	// Calculate time until next leak plus additional periods needed
	timeUntilNextLeak := lb.config.LeakPeriod - (now.Sub(state.LastLeakTime) % lb.config.LeakPeriod)
	additionalTime := time.Duration(periodsNeeded-1) * lb.config.LeakPeriod

	return timeUntilNextLeak + additionalTime
}
