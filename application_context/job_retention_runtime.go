package application_context

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"mahresources/jobs"
	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	jobHistoryRetentionFenceKey         = "job-history-retention"
	defaultJobRetentionInterval         = 5 * time.Minute
	defaultJobRetentionContinueInterval = 2 * time.Second
	defaultJobRetentionLease            = 2 * time.Minute
	defaultJobRetentionQuiesceTimeout   = 5 * time.Second
)

// JobRetentionRuntimeConfig controls the managed history sweep. One call is
// always capped by BatchSize; a non-empty cursor is continued on the faster
// continuation cadence until the current cycle is complete.
type JobRetentionRuntimeConfig struct {
	Interval             time.Duration
	ContinuationInterval time.Duration
	LeaseDuration        time.Duration
	LeaseRefreshInterval time.Duration
	QuiesceTimeout       time.Duration
	BatchSize            int
}

// JobRetentionRuntime owns the process lifecycle for canonical Job retention.
// Its cursor is deliberately process-local: after a restart the first bounded
// batch begins a fresh cycle, so a cursor can never strand work skipped by an
// earlier process.
type JobRetentionRuntime struct {
	ctx     *MahresourcesContext
	service *jobs.Service

	interval             time.Duration
	continuationInterval time.Duration
	leaseDuration        time.Duration
	leaseRefreshInterval time.Duration
	quiesceTimeout       time.Duration
	batchSize            int

	cursor jobs.SweepCursor

	lifeCtx   context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	stopOnce  sync.Once
	loopWG    sync.WaitGroup
}

// NewJobRetentionRuntime creates the managed sweep loop. The zero config is a
// production-ready five-minute scan with bounded batches and a renewable database
// lease shared by every process connected to the Job database.
func NewJobRetentionRuntime(ctx *MahresourcesContext, service *jobs.Service, config JobRetentionRuntimeConfig) *JobRetentionRuntime {
	if service == nil && ctx != nil {
		service = ctx.JobService()
	}
	if config.Interval <= 0 {
		config.Interval = defaultJobRetentionInterval
	}
	if config.ContinuationInterval <= 0 {
		config.ContinuationInterval = defaultJobRetentionContinueInterval
	}
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = defaultJobRetentionLease
	}
	if config.LeaseRefreshInterval <= 0 || config.LeaseRefreshInterval >= config.LeaseDuration {
		config.LeaseRefreshInterval = config.LeaseDuration / 3
	}
	if config.QuiesceTimeout <= 0 {
		config.QuiesceTimeout = defaultJobRetentionQuiesceTimeout
	}
	if config.BatchSize <= 0 || config.BatchSize > jobs.MaxSweepBatch {
		config.BatchSize = jobs.DefaultSweepBatch
	}
	lifeCtx, cancel := context.WithCancel(context.Background())
	return &JobRetentionRuntime{
		ctx:                  ctx,
		service:              service,
		interval:             config.Interval,
		continuationInterval: config.ContinuationInterval,
		leaseDuration:        config.LeaseDuration,
		leaseRefreshInterval: config.LeaseRefreshInterval,
		quiesceTimeout:       config.QuiesceTimeout,
		batchSize:            config.BatchSize,
		lifeCtx:              lifeCtx,
		cancel:               cancel,
	}
}

// Start begins with an immediate pass so an expired history is noticed on boot.
// It is safe to call more than once.
func (r *JobRetentionRuntime) Start() {
	if r == nil {
		return
	}
	r.startOnce.Do(func() {
		r.loopWG.Add(1)
		go r.run()
	})
}

// Stop cancels an in-flight database or adapter cleanup, then waits for the
// bounded sweep to return. If an adapter ignores cancellation, the renewable
// lease remains owned until that call actually exits or this process ends.
func (r *JobRetentionRuntime) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		r.cancel()
		waitForWaitGroup(&r.loopWG, r.quiesceTimeout)
	})
}

func (r *JobRetentionRuntime) run() {
	defer r.loopWG.Done()
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-r.lifeCtx.Done():
			return
		case <-timer.C:
			continueSoon := r.sweepOneBatch()
			delay := r.interval
			if continueSoon {
				delay = r.continuationInterval
			}
			timer.Reset(delay)
		}
	}
}

// sweepOneBatch owns the cross-process lease for one bounded service call. The
// cursor advances only after a successful sweep while this process still owns
// the lease; a restart or lease loss therefore retries from a fresh safe point.
func (r *JobRetentionRuntime) sweepOneBatch() bool {
	if r == nil || r.ctx == nil || r.service == nil || r.lifeCtx.Err() != nil {
		return false
	}
	token, err := newJobRetentionLeaseToken()
	if err != nil {
		log.Printf("job retention: create sweep lease token failed: %v", err)
		return r.cursor.ID != ""
	}
	acquired, err := acquireJobRetentionLease(r.lifeCtx, r.ctx.db, token, r.leaseDuration)
	if err != nil {
		if isLockContentionError(err) {
			return true
		}
		if r.lifeCtx.Err() == nil {
			log.Printf("job retention: acquire sweep lease failed: %v", err)
		}
		return r.cursor.ID != ""
	}
	if !acquired {
		// Another process owns the current batch. Check again on the bounded
		// continuation cadence so a process can take over promptly if that owner
		// disappears and its lease expires.
		return true
	}

	sweepCtx, cancelSweep := context.WithCancel(r.lifeCtx)
	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan struct{})
	leaseLost := make(chan struct{}, 1)
	go func() {
		defer close(heartbeatDone)
		r.refreshJobRetentionLease(token, stopHeartbeat, cancelSweep, leaseLost)
	}()

	result, sweepErr := r.ctx.SweepJobHistoryContext(sweepCtx, r.cursor, r.batchSize)
	close(stopHeartbeat)
	<-heartbeatDone
	cancelSweep()
	if err := releaseJobRetentionLease(r.ctx.db, token); err != nil {
		log.Printf("job retention: release sweep lease failed: %v", err)
	}

	select {
	case <-leaseLost:
		if r.lifeCtx.Err() == nil {
			log.Printf("job retention: sweep lease was lost; retrying this batch")
		}
		return r.cursor.ID != ""
	default:
	}
	if sweepErr != nil {
		if r.lifeCtx.Err() == nil {
			log.Printf("job retention: sweep failed: %v", sweepErr)
		}
		return r.cursor.ID != ""
	}
	if result.Next == nil {
		r.cursor = jobs.SweepCursor{}
		return false
	}
	r.cursor = *result.Next
	return true
}

func (r *JobRetentionRuntime) refreshJobRetentionLease(token string, stop <-chan struct{}, cancelSweep context.CancelFunc, lost chan<- struct{}) {
	ticker := time.NewTicker(r.leaseRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			err := renewJobRetentionLease(token, r.ctx.db)
			if err == nil {
				continue
			}
			if isLockContentionError(err) {
				// A bounded SQLite sweep can hold its writer transaction while a
				// Kind removes an artifact. That same writer lock temporarily keeps
				// another process from claiming the lease, so retry renewal instead
				// of canceling work that still owns the serialized database turn.
				continue
			}
			select {
			case lost <- struct{}{}:
			default:
			}
			cancelSweep()
			return
		}
	}
}

func newJobRetentionLeaseToken() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}

func acquireJobRetentionLease(ctx context.Context, db *gorm.DB, token string, lease time.Duration) (bool, error) {
	if db == nil {
		return false, errors.New("job retention database is not configured")
	}
	acquired := false
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seed := models.JobRuntimeFence{Key: jobHistoryRetentionFenceKey}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return fmt.Errorf("seed sweep lease: %w", err)
		}
		stalePredicate, staleArgs := jobRetentionLeaseExpiredPredicate(tx, lease)
		result := tx.Model(&models.JobRuntimeFence{}).
			Where("key = ? AND (token = '' OR "+stalePredicate+")", append([]any{jobHistoryRetentionFenceKey}, staleArgs...)...).
			Updates(map[string]any{
				"token":       token,
				"acquired_at": jobRetentionDatabaseClock(tx),
			})
		if result.Error != nil {
			return fmt.Errorf("claim sweep lease: %w", result.Error)
		}
		acquired = result.RowsAffected == 1
		return nil
	})
	return acquired, err
}

func renewJobRetentionLease(token string, db *gorm.DB) error {
	if db == nil {
		return errors.New("job retention database is not configured")
	}
	result := db.Model(&models.JobRuntimeFence{}).
		Where("key = ? AND token = ?", jobHistoryRetentionFenceKey, token).
		Update("acquired_at", jobRetentionDatabaseClock(db))
	if result.Error != nil {
		return fmt.Errorf("renew sweep lease: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("sweep lease token is no longer current")
	}
	return nil
}

func releaseJobRetentionLease(db *gorm.DB, token string) error {
	if db == nil {
		return nil
	}
	result := db.Model(&models.JobRuntimeFence{}).
		Where("key = ? AND token = ?", jobHistoryRetentionFenceKey, token).
		Update("token", "")
	if result.Error != nil {
		return fmt.Errorf("clear sweep lease: %w", result.Error)
	}
	return nil
}

func jobRetentionDatabaseClock(db *gorm.DB) clause.Expr {
	if db.Dialector.Name() == "sqlite" {
		return gorm.Expr("STRFTIME('%Y-%m-%d %H:%M:%f', 'now')")
	}
	return gorm.Expr("CURRENT_TIMESTAMP")
}

func jobRetentionLeaseExpiredPredicate(db *gorm.DB, lease time.Duration) (string, []any) {
	seconds := lease.Seconds()
	if db.Dialector.Name() == "sqlite" {
		return "julianday(acquired_at) <= julianday('now') - (? / 86400.0)", []any{seconds}
	}
	return "acquired_at <= CURRENT_TIMESTAMP - (? * INTERVAL '1 second')", []any{seconds}
}
