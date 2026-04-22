package hash_worker

import "time"

// Config holds configuration for the HashWorker.
type Config struct {
	// WorkerCount is the number of concurrent hash calculation workers.
	WorkerCount int
	// BatchSize is the number of resources to process per batch cycle.
	BatchSize int
	// PollInterval is the time between batch processing cycles.
	PollInterval time.Duration
	// SimilarityThresholdFn returns the max DHash Hamming distance to consider
	// resources similar. Called per pair comparison so runtime settings changes
	// take effect without restart.
	SimilarityThresholdFn func() int
	// AHashThresholdFn returns the max AHash Hamming distance for the secondary
	// similarity check (BH-018). Return 0 to disable. Called per pair comparison.
	AHashThresholdFn func() uint64
	// Disabled prevents the hash worker from starting.
	Disabled bool
	// CacheSize is the maximum number of entries in the hash LRU cache.
	CacheSize int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		WorkerCount:           4,
		BatchSize:             500,
		PollInterval:          time.Minute,
		SimilarityThresholdFn: func() int { return 10 },
		AHashThresholdFn:      func() uint64 { return 5 },
		Disabled:              false,
		CacheSize:             100000,
	}
}
