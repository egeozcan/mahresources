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
	// SimilarityThreshold is the maximum Hamming distance to consider resources similar.
	SimilarityThreshold int
	// AHashThreshold is the maximum AHash Hamming distance for the secondary check (BH-018).
	// When DHash distance is within SimilarityThreshold, AHash distance must also be within
	// this value to record the pair as similar. Set to 0 to disable the secondary check.
	AHashThreshold uint64
	// Disabled prevents the hash worker from starting.
	Disabled bool
	// CacheSize is the maximum number of entries in the hash LRU cache.
	CacheSize int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		WorkerCount:         4,
		BatchSize:           500,
		PollInterval:        time.Minute,
		SimilarityThreshold: 10,
		AHashThreshold:      5,
		Disabled:            false,
		CacheSize:           100000,
	}
}
