package application_context

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"mahresources/jobs"
)

// A dispatch that re-checks the account its Job acts as can find that the check
// itself did not answer: the users or groups read failed. That is not a refusal,
// and blocking the Job on it made a passing outage into a Job only Resume could
// restart. The execution goes back to the queue instead, under its own token, with
// a row that says what it waits for, and this process's dispatch loop passes over
// it for a while before asking again: one second, doubling with each failure in a
// row, up to thirty (giveBackDeferral, the plugin-action admission's backoff).
//
// The deferral is this process's memory. Another process may claim the Job at once
// and ask its own database connection; a restart forgets the streak and asks again.

// jobDispatchChecksUnansweredReason is the queued event's reason for such a return.
const jobDispatchChecksUnansweredReason = "checks-unanswered"

// dispatchCheckForget is how long after its last deferral ran out a Job's streak is
// kept. A Job that has not come back by then ended or ran elsewhere, and its next
// failure starts again from one second.
const dispatchCheckForget = 2 * pluginActionGiveBackCap

type dispatchCheckDeferral struct {
	kind   string
	streak int
	until  time.Time
}

// dispatchCheckDeferrals is one process's record of the Jobs whose dispatch could
// not read the account they act as.
type dispatchCheckDeferrals struct {
	mu    sync.Mutex
	byJob map[string]dispatchCheckDeferral
}

func newDispatchCheckDeferrals() *dispatchCheckDeferrals {
	return &dispatchCheckDeferrals{byJob: make(map[string]dispatchCheckDeferral)}
}

// deferJob records one more failed check for a Job and answers how long it stays
// out of the claim loop.
func (d *dispatchCheckDeferrals) deferJob(jobID, kind string, now time.Time) time.Duration {
	if d == nil {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	entry := d.byJob[jobID]
	if !entry.until.IsZero() && now.Sub(entry.until) > dispatchCheckForget {
		entry.streak = 0
	}
	entry.kind = kind
	entry.streak++
	wait := giveBackDeferral(entry.streak)
	entry.until = now.Add(wait)
	d.byJob[jobID] = entry
	return wait
}

// answered forgets a Job whose checks answered, refusal or not.
func (d *dispatchCheckDeferrals) answered(jobID string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.byJob, jobID)
}

// passOver lists the Jobs of one Kind still inside their deferral, and forgets the
// ones whose streak has lapsed.
func (d *dispatchCheckDeferrals) passOver(kind string, now time.Time) []string {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var waiting []string
	for jobID, entry := range d.byJob {
		if now.Sub(entry.until) > dispatchCheckForget {
			delete(d.byJob, jobID)
			continue
		}
		if entry.kind == kind && now.Before(entry.until) {
			waiting = append(waiting, jobID)
		}
	}
	sort.Strings(waiting)
	return waiting
}

// deferDispatch hands a claimed execution whose account or scope read failed back
// to the queue, and keeps this process from claiming it again until its deferral
// runs out. A cancellation already recorded against the Job ends it instead
// (requeueDownloadExecution).
func (ctx *MahresourcesContext) deferDispatch(execution jobs.Execution, readErr error) error {
	wait := ctx.dispatchChecks.deferJob(execution.JobID, execution.Kind, time.Now())
	log.Printf("warning: the account Job %s acts as could not be checked (%v); it waits in the queue and is asked again in %s",
		execution.JobID, readErr, wait)
	return ctx.requeueDownloadExecution(execution, jobDispatchChecksUnansweredReason,
		downloadWaitingPhase, pluginActionWaitingForChecks)
}

// dispatchChecksAnswered records that a Job's dispatch could check its account.
func (ctx *MahresourcesContext) dispatchChecksAnswered(jobID string) {
	ctx.dispatchChecks.answered(jobID)
}

// dispatchBinding binds the account a claimed execution acts as, for the checks a
// dispatch makes and the work it then runs. It is the same resolution
// principalForPluginActor makes (a deleted or disabled account binds deny-all,
// which is an answer), except that a read which failed — the account's, or the
// scope subtree's — is returned rather than bound as deny-all, so the caller can
// defer the Job instead of refusing it. Work the host runs as itself (no account)
// gets this context unchanged.
func (ctx *MahresourcesContext) dispatchBinding(userID uint) (*MahresourcesContext, error) {
	if userID == 0 {
		return ctx, nil
	}
	principal, err := commandActorLookup(ctx.db, userID)
	if err != nil {
		return nil, err
	}
	bound, err := ctx.withPrincipalWithin(context.Background(), principal)
	if err != nil {
		return nil, err
	}
	return bound, nil
}
