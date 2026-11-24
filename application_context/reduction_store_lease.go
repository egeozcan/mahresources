package application_context

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Compute and apply leases (design D11). Both are int64 Unix nanoseconds, so a
// check never compares text timestamps on SQLite.

const (
	// leaseWindow is how far ahead one renewal pushes a lease.
	leaseWindow = 10 * time.Minute
	// leaseCeiling is the longest a run may hold its lease from its start.
	leaseCeiling = 24 * time.Hour
	// leaseRenewEvery is the ticker's period.
	leaseRenewEvery = 30 * time.Second
)

// ErrReductionLeaseLost says the run no longer owns the Reduction: its lease
// expired or its generation was superseded. Whatever it was about to write stops.
var ErrReductionLeaseLost = errors.New("this Resource Reduction's compute or apply lease is no longer held")

// leaseDeadline is min(now + 10 min, started + 24 h).
func leaseDeadline(now, started time.Time) int64 {
	d := now.Add(leaseWindow)
	if ceiling := started.Add(leaseCeiling); d.After(ceiling) {
		d = ceiling
	}
	return d.UnixNano()
}

// renewComputeLease pushes a live compute lease forward without touching
// version or updated_at. The transaction first takes the row without changing it
// (SET id = id, keyed by status and generation), reads the old lease, and
// compares it with the caller's clock read after that lock: an expired lease is
// never renewed, so a run that waited past expiry cannot revive itself. It
// reports whether the lease is still held.
func (s *reductionStore) renewComputeLease(id uint, generation string, started, now time.Time) (bool, error) {
	held := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE resource_reductions SET id = id WHERE id = ? AND status = 'computing' AND compute_generation = ?`, id, generation)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return nil
		}
		var old int64
		if err := tx.Raw(`SELECT compute_lease_until FROM resource_reductions WHERE id = ?`, id).Scan(&old).Error; err != nil {
			return err
		}
		if old <= now.UnixNano() {
			return nil
		}
		held = true
		return tx.Exec(`UPDATE resource_reductions SET compute_lease_until = ? WHERE id = ? AND compute_generation = ?`,
			leaseDeadline(now, started), id, generation).Error
	})
	return held, err
}

// fenceCompute proves a compute still owns the Reduction: it is computing under
// this generation and its lease is live against a clock the caller read after
// the lock was taken. Every staging transaction starts with it.
func (s *reductionStore) fenceCompute(id uint, generation string, now time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error { return fenceComputeIn(tx, id, generation, now) })
}

// fenceComputeIn is fenceCompute inside the caller's transaction.
func fenceComputeIn(tx *gorm.DB, id uint, generation string, now time.Time) error {
	res := tx.Exec(`UPDATE resource_reductions SET id = id WHERE id = ? AND status = 'computing' AND compute_generation = ?`, id, generation)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrReductionLeaseLost
	}
	var lease int64
	if err := tx.Raw(`SELECT compute_lease_until FROM resource_reductions WHERE id = ?`, id).Scan(&lease).Error; err != nil {
		return err
	}
	if lease <= now.UnixNano() {
		return ErrReductionLeaseLost
	}
	return nil
}

// runLeaseTicker renews a lease every leaseRenewEvery until the context ends
// (the run's context, so the ticker stops with the phase it serves). renew is
// told the current time; it reports whether the lease is still held, and the
// ticker stops at the first false, which the run discovers at its next fence.
func runLeaseTicker(ctx context.Context, every time.Duration, renew func(now time.Time) (bool, error)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if held, err := renew(now); err == nil && !held {
				return
			}
		}
	}
}
