package algorithm

// This file contains shared types uses across all the algorithms

// AlgorithmResult represents a generic result from any algorithm
type AlgorithmResult interface {
	IsAllowed() bool
	GetRemaining() interface{}
	GetResetTime() interface{}
}

// AlgorithmStats represents generic statistics from any algorithm
type AlgorithmStats interface {
	GetCurrentState() interface{}
	GetConfiguration() interface{}
}
