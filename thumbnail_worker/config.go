package thumbnail_worker

import "time"

// Config holds configuration for the ThumbnailWorker.
type Config struct {
	// WorkerCount is the number of concurrent thumbnail generation workers.
	WorkerCount int
	// BatchSize is the number of videos to process per backfill cycle.
	BatchSize int
	// PollInterval is the time between backfill processing cycles.
	PollInterval time.Duration
	// Disabled prevents the thumbnail worker from starting.
	Disabled bool
	// Backfill enables batch catch-up for existing videos without thumbnails.
	// When false (default), only videos queued during upload are processed.
	Backfill bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		WorkerCount:  2,
		BatchSize:    10,
		PollInterval: time.Minute,
		Disabled:     false,
		Backfill:     false,
	}
}
