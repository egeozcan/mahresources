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
	// Disabled prevents the hash worker from starting.
	Disabled bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		WorkerCount:         4,
		BatchSize:           500,
		PollInterval:        time.Minute,
		SimilarityThreshold: 10,
		Disabled:            false,
	}
}
