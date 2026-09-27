package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

const (
	// A scheduled-download claim is held only across a queue submit, not across
	// the transfer. A process that dies in that tiny window must not wedge the
	// row forever, and a still-live process should not be stolen while it is
	// making the SubmitForPlugin call.
	ScheduledDownloadClaimTTL = time.Minute

	// A failed submit is terminal: deferred downloads do not retry forever on a
	// timer. Rows that never submitted (for example because the same URL is
	// already running) keep Attempts at zero and remain pending for a later tick.
	scheduledDownloadMaxSubmitAttempts = 1

	// A row blocked by a live download of the same URL moves out of the immediate
	// due set instead of being released unchanged. Otherwise the oldest colliding
	// rows can occupy the sweep's fixed page forever and starve later due rows.
	scheduledDownloadActiveURLDefer = time.Minute
)

// ScheduledDownloadSubmitFunc is the queue-submission seam used by the scheduler
// and by tests. The production caller adapts DownloadManager.SubmitForPlugin to
// return the job id; tests inject a function and need no real download worker.
type ScheduledDownloadSubmitFunc func(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint, pluginName string) (string, error)

// ScheduledDownloadActiveFunc reports whether a URL is already being fetched.
// Production passes download_queue.ActiveDownloadForURL bound to the live
// manager; tests can provide a deterministic answer.
type ScheduledDownloadActiveFunc func(url string) (jobID string, active bool)

// ScheduledDownloadFireConfig contains the external decisions a scheduler tick
// needs in order to fire due rows while keeping this context testable.
type ScheduledDownloadFireConfig struct {
	Now time.Time
	// Limit bounds one sweep. Zero selects the default.
	Limit int
	// ActiveDownload is optional; nil means no live download is known.
	ActiveDownload ScheduledDownloadActiveFunc
	// Submit is required for a row that reaches the submit step.
	Submit ScheduledDownloadSubmitFunc
	// PluginAvailable is optional. nil checks the live PluginManager's network
	// policy by name, which is the production rule: a disabled or missing plugin
	// refuses the fire rather than falling back to the host policy.
	PluginAvailable func(pluginName string) bool
}

// CreateScheduledDownload persists a deferred plugin download.
//
// The insert is explicitly bound as the actor because the create-stamp callback
// overwrites CreatedByUserId from the db context. Writing the field on the struct
// alone would let a worker/default actor replace the submitter and would make
// the owner predicate below ask about the wrong user.
func (ctx *MahresourcesContext) CreateScheduledDownload(pluginName string, actorUserID uint, creator *query_models.ResourceFromRemoteCreator, dueAt time.Time) (*models.ScheduledDownload, error) {
	if ctx == nil || ctx.db == nil {
		return nil, errors.New("scheduled download store is not available")
	}
	if pluginName == "" {
		return nil, errors.New("refusing to schedule: the calling plugin is not identified")
	}
	if actorUserID == 0 {
		return nil, errors.New("refusing to schedule: the acting user is not identified")
	}
	if creator == nil {
		return nil, errors.New("scheduled download needs a payload")
	}
	payload, err := scheduledDownloadPayload(creator)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		// Read outside the transaction, so that the transaction's first statement
		// is its insert. On SQLite in WAL mode a transaction that reads first
		// holds a snapshot it cannot promote once another connection commits, and
		// that failure skips busy_timeout: under download traffic a deferred
		// submit failed with "database is locked" at once.
		retired, err := ctx.legacyJobInputsRetired()
		if err != nil {
			return nil, err
		}
		row, err := ctx.createScheduledDownloadOnce(pluginName, actorUserID, creator, payload, dueAt, retired)
		if err == nil {
			return row, nil
		}
		if attempt >= scheduledDownloadCreateAttempts-1 {
			return nil, err
		}
		if !errors.Is(err, errWriterEpochMoved) && !isLockContentionError(err) && !isDeadlockError(err) {
			// A retirement barrier refuses a plaintext row: the epoch advanced
			// after it was read, and the next attempt reads it again.
			if current, readErr := ctx.legacyJobInputsRetired(); readErr != nil || current == retired {
				return nil, err
			}
		}
		time.Sleep(scheduledDownloadCreateBackoff * time.Duration(attempt+1))
	}
}

// scheduledDownloadCreateAttempts bounds CreateScheduledDownload's retry of its
// transaction, and scheduledDownloadCreateBackoff is multiplied by the attempt
// number between tries. A failed attempt rolled back, so nothing partial
// persisted and the next one inserts the row exactly once.
const (
	scheduledDownloadCreateAttempts = 4
	scheduledDownloadCreateBackoff  = 25 * time.Millisecond
)

// errWriterEpochMoved rolls back an insert made under a writer epoch that
// changed before the transaction could commit.
var errWriterEpochMoved = errors.New("job writer epoch changed while the deferred download was being stored")

func (ctx *MahresourcesContext) createScheduledDownloadOnce(pluginName string, actorUserID uint, creator *query_models.ResourceFromRemoteCreator, payload types.JSON, dueAt time.Time, retired bool) (*models.ScheduledDownload, error) {
	owner := actorUserID
	row := models.ScheduledDownload{
		PluginName: truncateRunes(pluginName, maxHistoryPluginNameLength),
		// Exact, not truncated: this column is the management surface's copy of
		// the payload URL, and Task 4's active-download check uses the payload URL
		// itself. A lossy row URL would make the two disagree and hide duplicates.
		URL:             creator.URL,
		Payload:         payload,
		DueAt:           dueAt,
		Status:          models.ScheduledDownloadStatusPending,
		CreatedByUserId: &owner,
	}
	if retired {
		row.URL = downloadURLProjection(creator.URL)
		row.Payload = nil
	}
	db := ctx.WithPrincipal(&auth.Principal{UserID: actorUserID}).db
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		// Read again now that the insert holds the writer lock: the row must have
		// been written under the epoch that is current when it commits.
		stillRetired, err := legacyJobInputsRetiredOn(tx)
		if err != nil {
			return err
		}
		if stillRetired != retired {
			return errWriterEpochMoved
		}
		// Keep the compatibility row, accepted Job and migration ledger together.
		// Startup can then neither mistake a crash-limbo row for completed copy nor
		// advance the writer barrier without its canonical replay and handle.
		if err := ctx.acceptDeferredDownloadJob(tx, &row, creator, pluginName); err != nil {
			return err
		}
		if ctx.JobService() != nil && tx.Migrator().HasTable(&models.JobSourceMapping{}) {
			if err := ctx.recordDualPublishedScheduledDownloadTx(tx, row, retired, time.Now().UTC()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// acceptDeferredDownloadJob records a deferred download as a `scheduled` Job.
//
// A one-off deferred operation is already a scheduled Job: it names one concrete
// execution, so it does not need a Schedule, a materialization step or a second
// identity when its due time arrives (§1). What the row keeps is the plugin-facing
// record and the management listing; the Job is what is dispatched.
//
// A deployment with no control plane keeps the row alone, which is what this
// feature was before there was a Job to accept it as.
func (ctx *MahresourcesContext) acceptDeferredDownloadJob(db *gorm.DB, row *models.ScheduledDownload, creator *query_models.ResourceFromRemoteCreator, pluginName string) error {
	service := ctx.JobService()
	if service == nil {
		return nil
	}
	if db == nil {
		return fmt.Errorf("deferred download acceptance has no transaction")
	}
	input, err := remoteDownloadInputJSON(creator, pluginName)
	if err != nil {
		return err
	}
	owner := row.CreatedByUserId
	deps := ctx.jobDeps()
	deps.DB = db
	_, err = service.Accept(deps, jobs.Acceptance{
		Kind:         JobKindDeferredDownload,
		KindVersion:  jobDownloadKindVersion,
		State:        jobs.StateScheduled,
		OwnerUserID:  copyUintPtr(owner),
		ActorUserID:  copyUintPtr(owner),
		Origin:       "schedule",
		Title:        downloadJobTitle(input),
		Replay:       jobs.ReplayInput{Input: input},
		ScheduledFor: &row.DueAt,
		LegacyRefs:   []jobs.LegacyRef{{Namespace: ScheduledDownloadHandleNamespace, Handle: fmt.Sprintf("%d", row.ID)}},
	})
	return err
}

func scheduledDownloadPayload(creator *query_models.ResourceFromRemoteCreator) (types.JSON, error) {
	payload, err := json.Marshal(creator)
	if err != nil {
		return nil, fmt.Errorf("scheduled download: encode payload: %w", err)
	}
	return types.JSON(payload), nil
}

// ScheduledDownloadPayload decodes a stored scheduled-download payload. A row
// whose payload predates this feature or failed to encode still carries URL,
// which is enough to attempt the submission.
func (ctx *MahresourcesContext) ScheduledDownloadPayload(row *models.ScheduledDownload) (*query_models.ResourceFromRemoteCreator, error) {
	if row == nil {
		return nil, errors.New("scheduled download: no row")
	}
	creator := &query_models.ResourceFromRemoteCreator{}
	retired, err := ctx.legacyJobInputsRetired()
	if err != nil {
		return nil, fmt.Errorf("scheduled download: check canonical replay fence: %w", err)
	}
	if retired {
		service := ctx.JobService()
		if service == nil {
			return nil, errors.New("scheduled download: canonical Job service is unavailable")
		}
		jobID, err := service.ResolveLegacyHandle(ctx.jobDeps(), ScheduledDownloadHandleNamespace, fmt.Sprintf("%d", row.ID))
		if err != nil {
			return nil, errors.New("scheduled download: canonical execution input is unavailable")
		}
		opened, err := service.OpenReplay(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
		if err != nil {
			return nil, errors.New("scheduled download: canonical execution input is unavailable")
		}
		var input downloadJobInput
		if err := json.Unmarshal(opened.Input, &input); err != nil || input.Creator == nil {
			return nil, errors.New("scheduled download: canonical execution input is unreadable")
		}
		return input.Creator, nil
	}
	if len(row.Payload) > 0 {
		if err := json.Unmarshal(row.Payload, creator); err != nil {
			return nil, fmt.Errorf("scheduled download: decode stored payload: %w", err)
		}
	}
	if creator.URL == "" {
		creator.URL = row.URL
	}
	if creator.URL == "" {
		return nil, errors.New("scheduled download: row has no URL")
	}
	return creator, nil
}

// DueScheduledDownloads lists rows a tick should attempt to claim. The claim
// repeats every predicate: this is only the cheap prefilter.
func (ctx *MahresourcesContext) DueScheduledDownloads(now time.Time, limit int) ([]models.ScheduledDownload, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []models.ScheduledDownload
	err := ctx.db.
		Where(scheduledDownloadInstant(ctx.db, "due_at")+" <= "+scheduledDownloadInstant(ctx.db, "?"), scheduledDownloadDueBound(now)).
		Where("status = ?", models.ScheduledDownloadStatusPending).
		Where("created_by_user_id IS NOT NULL").
		Where("attempts < ?", scheduledDownloadMaxSubmitAttempts).
		Where(scheduledDownloadClaimFree(ctx.db), now.Add(-ScheduledDownloadClaimTTL)).
		Order(scheduledDownloadInstant(ctx.db, "due_at") + " asc, id asc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// scheduledDownloadInstant wraps a timestamp column or a bound parameter so the
// database compares it as an instant.
//
// SQLite has no timestamp type. go-sqlite3 stores a time as text carrying the
// offset of the value it was given, and compares text as text. A due time is
// written in UTC when it came from `start_at` and in the server's zone when it
// came from `delay`, and a scheduler in another zone compares it with its own
// clock: `2026-09-26 12:53:13+00:00` sorts before `2026-09-26 14:43:13+02:00`
// although it is ten minutes later, so a bare comparison fired such a row at
// once east of UTC and hours late west of it. julianday() reads the offset and
// yields the instant, whichever form a release wrote. PostgreSQL stores
// timestamptz and compares instants already, so there the column stays bare and
// its index usable.
func scheduledDownloadInstant(db *gorm.DB, operand string) string {
	if db.Dialector.Name() == "sqlite" {
		return "julianday(" + operand + ")"
	}
	return operand
}

// scheduledDownloadDueBound is the latest due time a sweep at now may treat as
// come. julianday() resolves milliseconds, rounding, so a row due a fraction of a
// millisecond after now would compare equal to it; one millisecond back is never
// early. Firing a row queues its Job, bypassing the time the Job's own claim
// waits for, so early is the direction that matters.
func scheduledDownloadDueBound(now time.Time) time.Time {
	return now.Add(-time.Millisecond)
}

// scheduledDownloadClaimFree is the predicate for a row nobody holds: no claim,
// or a claim older than the bound it is given, compared as an instant.
func scheduledDownloadClaimFree(db *gorm.DB) string {
	return "COALESCE(claim_token, '') = '' OR claimed_at IS NULL OR " +
		scheduledDownloadInstant(db, "claimed_at") + " < " + scheduledDownloadInstant(db, "?")
}

// ClaimScheduledDownload takes the short-lived submit slot for one due row.
func (ctx *MahresourcesContext) ClaimScheduledDownload(id uint, claimToken string, now time.Time) (bool, error) {
	if claimToken == "" {
		return false, errors.New("a scheduled download claim needs a token nobody else can produce")
	}
	res := ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ?", id).
		Where(scheduledDownloadInstant(ctx.db, "due_at")+" <= "+scheduledDownloadInstant(ctx.db, "?"), scheduledDownloadDueBound(now)).
		Where("status = ?", models.ScheduledDownloadStatusPending).
		Where("created_by_user_id IS NOT NULL").
		Where("attempts < ?", scheduledDownloadMaxSubmitAttempts).
		Where(scheduledDownloadClaimFree(ctx.db), now.Add(-ScheduledDownloadClaimTTL)).
		Updates(map[string]any{"claim_token": claimToken, "claimed_at": now})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (ctx *MahresourcesContext) scheduledDownloadByClaim(id uint, claimToken string) (*models.ScheduledDownload, error) {
	var row models.ScheduledDownload
	err := ctx.db.Where("id = ? AND claim_token = ?", id, claimToken).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ReleaseScheduledDownloadClaim hands a row back to the next tick without
// recording an attempt. Used when no submit was made for a transient reason that
// should keep the row immediately due.
func (ctx *MahresourcesContext) ReleaseScheduledDownloadClaim(id uint, claimToken string) error {
	return ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Updates(map[string]any{"claim_token": "", "claimed_at": nil}).Error
}

// DeferScheduledDownloadClaim releases a claimed row and moves its due time out
// of the current sweep's head. The claim token predicate is the CAS: a stale
// fire path must not move a row it no longer owns.
func (ctx *MahresourcesContext) DeferScheduledDownloadClaim(id uint, claimToken string, dueAt, at time.Time) error {
	res := ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Where("status = ?", models.ScheduledDownloadStatusPending).
		Updates(map[string]any{
			"claim_token": "",
			"claimed_at":  nil,
			"due_at":      dueAt,
			"updated_at":  at,
		})
	return res.Error
}

// ReserveScheduledDownloadSubmit moves a claimed row out of the claimable set
// before the queue side effect. The claim stays on the row until the job id is
// recorded, but status=submitted makes a stale claim unreclaimable: if this
// process stalls after SubmitForPlugin, a later tick cannot create a second job.
func (ctx *MahresourcesContext) ReserveScheduledDownloadSubmit(id uint, claimToken string, at time.Time) (bool, error) {
	res := ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Where("status = ?", models.ScheduledDownloadStatusPending).
		Updates(map[string]any{
			"status":     models.ScheduledDownloadStatusSubmitted,
			"attempts":   gorm.Expr("attempts + 1"),
			"updated_at": at,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// MarkScheduledDownloadSubmitted records the queue job a fire produced and
// releases the claim in the same update.
func (ctx *MahresourcesContext) MarkScheduledDownloadSubmitted(id uint, claimToken, jobID string, at time.Time) error {
	res := ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Where("status = ?", models.ScheduledDownloadStatusSubmitted).
		Updates(map[string]any{
			"claim_token": "",
			"claimed_at":  nil,
			"job_id":      jobID,
			"last_error":  "",
			"updated_at":  at,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		return nil
	}
	return fmt.Errorf("scheduled download %d: %w", id, errScheduledDownloadClaimLost)
}

// errScheduledDownloadClaimLost reports a claimed or reserved row something else
// has since ended: a cancel of its Job, which cancels the row a sweep holds. The
// sweep then has nothing left to record, and the rows behind it still fire.
var errScheduledDownloadClaimLost = errors.New("the row no longer carries this submit claim")

// MarkScheduledDownloadFailed records a terminal refusal/failure and releases
// the claim. Failed scheduled downloads are not retried forever by the tick.
func (ctx *MahresourcesContext) MarkScheduledDownloadFailed(id uint, claimToken string, runErr error, at time.Time) error {
	return ctx.markScheduledDownloadFailed(id, claimToken, runErr, at, true)
}

func (ctx *MahresourcesContext) markScheduledDownloadFailed(id uint, claimToken string, runErr error, at time.Time, incrementAttempt bool) error {
	msg := ""
	if runErr != nil {
		msg = truncateRunes(runErr.Error(), maxHistoryErrorLength)
	}
	updates := map[string]any{
		"claim_token": "",
		"claimed_at":  nil,
		"status":      models.ScheduledDownloadStatusFailed,
		"last_error":  msg,
		"updated_at":  at,
	}
	if incrementAttempt {
		updates["attempts"] = gorm.Expr("attempts + 1")
	}
	res := ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		return nil
	}
	return fmt.Errorf("scheduled download %d: %w", id, errScheduledDownloadClaimLost)
}

// CancelScheduledDownload cancels a pending scheduled download before it is
// submitted. Already submitted/failed/cancelled rows are left unchanged.
//
// The row and its scheduled Job are one deferral, and the dispatch loop runs the
// Job at its due time without consulting the row, so the Job is cancelled in the
// same transaction. A Job that an execution already holds, or that has run, is
// a download that started: the row is then left alone and false returned, and
// the Job's own Cancel is what stops it. A Job that already ended in a way the row
// records instead (deferredRowJob.ended), other than by being cancelled, is
// recorded on the row, and the cancel answers ErrScheduledDownloadEnded.
func (ctx *MahresourcesContext) CancelScheduledDownload(id uint) (bool, error) {
	now := time.Now()
	errStarted := errors.New("the deferred download has started")
	var ended error
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		// PostgreSQL: the Job is locked before the row, the order a cancel of the
		// Job takes them in (the Job, then this row through ApplyHostTransition).
		// The reverse order deadlocks two cancels of one deferral. SQLite has one
		// writer and no row locks, and there the conditional update stays the
		// transaction's first statement, so it takes the writer lock before
		// anything is read.
		postgres := tx.Dialector.Name() == "postgres"
		var rowJob deferredRowJob
		if postgres {
			var err error
			if rowJob, err = ctx.loadDeferredRowJob(tx, id, true); err != nil {
				return err
			}
		}
		res := tx.Model(&models.ScheduledDownload{}).
			Where("id = ?", id).
			Where("status = ?", models.ScheduledDownloadStatusPending).
			Where(scheduledDownloadClaimFree(tx), now.Add(-ScheduledDownloadClaimTTL)).
			Updates(map[string]any{
				"claim_token": "",
				"claimed_at":  nil,
				"status":      models.ScheduledDownloadStatusCancelled,
				"updated_at":  now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errScheduledDownloadNotCancellable
		}
		if !postgres {
			var err error
			if rowJob, err = ctx.loadDeferredRowJob(tx, id, false); err != nil {
				return err
			}
		}
		if state, isEnded := rowJob.ended(); isEnded {
			status, lastError := deferredRowEnd(state)
			if status == models.ScheduledDownloadStatusCancelled {
				return nil
			}
			ended = fmt.Errorf("scheduled download %d: %w: %s", id, ErrScheduledDownloadEnded, lastError)
			return tx.Model(&models.ScheduledDownload{}).Where("id = ?", id).
				Updates(map[string]any{"status": status, "last_error": lastError, "updated_at": now}).Error
		}
		if rowJob.ID == "" {
			return nil
		}
		if rowJob.Job.StartedAt != nil {
			return errStarted
		}
		if rowJob.Job.State.Terminal() {
			return nil
		}
		_, err := ctx.JobService().Transition(ctx.jobDepsWithDB(tx), jobs.Transition{
			JobID:           rowJob.ID,
			ExpectedVersion: rowJob.Job.Version,
			To:              jobs.StateCancelled,
		})
		return err
	})
	if errors.Is(err, errScheduledDownloadNotCancellable) || errors.Is(err, errStarted) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if ended != nil {
		return false, ended
	}
	return true, nil
}

var errScheduledDownloadNotCancellable = errors.New("the scheduled download is not pending")

// ErrScheduledDownloadEnded answers a cancel of a row whose Job had already
// ended in a way the row records instead (deferredRowEnd); the row now says how.
var ErrScheduledDownloadEnded = errors.New("the scheduled download already ended")

// deferredDownloadJobIDOn names the Job a deferred row was accepted with, or ""
// when it has none. That is its source mapping's Job, which nothing moves. The
// row's legacy handle is not: a Retry moves it to its successor, an ordinary
// download the row does not track. A row with no mapping, written before there
// was one, is named by its handle.
func deferredDownloadJobIDOn(db *gorm.DB, rowID uint) (string, error) {
	id := strconv.FormatUint(uint64(rowID), 10)
	if db.Migrator().HasTable(&models.JobSourceMapping{}) {
		var mapping models.JobSourceMapping
		err := db.Where("source_kind = ? AND source_id = ?", jobMigrationScheduledDownload, id).First(&mapping).Error
		if err == nil && mapping.JobID != "" {
			return mapping.JobID, nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	}
	var handle models.JobLegacyHandle
	err := db.Where("namespace = ? AND handle = ?", ScheduledDownloadHandleNamespace, id).First(&handle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return handle.JobID, err
}

// deferredRowJob is the Job a deferred row was accepted with, as the row sees it.
// Every reader of that Job goes through loadDeferredRowJob, so each of them reads
// a Job that no longer exists the same way.
type deferredRowJob struct {
	// ID is "" for a row with no durable Job, written before there was a control
	// plane.
	ID string
	// Job is the Job's snapshot, and the zero value when it is gone.
	Job jobs.Snapshot
	// Gone reports a Job that no longer exists (mappedJobGone): it ended, and how
	// is not known.
	Gone bool
}

// ended reports that the row's Job has ended in a way the row records rather than
// submitting or cancelling it (deferredRowEnd): with the end state of a Job that
// ended before anything ran it, and with no state for a Job that is gone, which
// may have run — the dispatch loop can run a Job before the sweep records its
// row. A Job that started is not ended here: the row names it instead.
func (j deferredRowJob) ended() (jobs.State, bool) {
	if j.Gone {
		return "", true
	}
	if j.ID != "" && j.Job.State.Terminal() && j.Job.StartedAt == nil {
		return j.Job.State, true
	}
	return "", false
}

// loadDeferredRowJob reads the Job a row was accepted with (deferredDownloadJobIDOn)
// through db, and with lock takes the Job's row lock first.
func (ctx *MahresourcesContext) loadDeferredRowJob(db *gorm.DB, rowID uint, lock bool) (deferredRowJob, error) {
	service := ctx.JobService()
	if service == nil {
		return deferredRowJob{}, nil
	}
	jobID, err := deferredDownloadJobIDOn(db, rowID)
	if err != nil || jobID == "" {
		return deferredRowJob{}, err
	}
	if lock {
		var locked models.Job
		err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", jobID).First(&locked).Error
		if err != nil && !mappedJobGone(err) {
			return deferredRowJob{}, err
		}
	}
	job, err := service.Get(ctx.jobDepsWithDB(db), jobs.Access{Administrator: true}, jobID)
	if mappedJobGone(err) {
		return deferredRowJob{ID: jobID, Gone: true}, nil
	}
	if err != nil {
		return deferredRowJob{}, err
	}
	return deferredRowJob{ID: jobID, Job: job}, nil
}

// cancelDeferredDownloadRowTx ends the row behind a deferred Job the host
// cancelled while nothing was running it, inside the transaction that cancelled
// it. That is the pending row, and also a row a sweep has already reserved or
// marked submitted when the Job never started: the sweep queued it and the
// dispatch loop had not claimed it yet, so nothing was downloaded.
//
// The row's claim is overridden rather than respected: the cancellation has
// already won on the Job, and a sweep holding the claim reserves the row only
// while it is still pending, so it finds nothing left to submit.
func cancelDeferredDownloadRowTx(tx *gorm.DB, job jobs.Snapshot, at time.Time) error {
	// The row the Job was accepted with (deferredDownloadJobIDOn): a Retry's
	// successor has taken over a handle, but it is not the row's Job.
	var source string
	if tx.Migrator().HasTable(&models.JobSourceMapping{}) {
		var mapping models.JobSourceMapping
		err := tx.Where("source_kind = ? AND job_id = ?", jobMigrationScheduledDownload, job.ID).First(&mapping).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		source = mapping.SourceID
	} else {
		var handle models.JobLegacyHandle
		err := tx.Where("namespace = ? AND job_id = ?", ScheduledDownloadHandleNamespace, job.ID).First(&handle).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		source = handle.Handle
	}
	rowID, err := strconv.ParseUint(source, 10, 64)
	if err != nil {
		return nil
	}
	query := tx.Model(&models.ScheduledDownload{}).Where("id = ?", uint(rowID))
	if job.StartedAt == nil {
		// A submitted row with no job id yet is one a sweep has reserved and is
		// still recording; it loses its reservation (MarkScheduledDownloadSubmitted).
		query = query.Where("status = ? OR (status = ? AND COALESCE(job_id, '') IN ?)",
			models.ScheduledDownloadStatusPending, models.ScheduledDownloadStatusSubmitted, []string{"", job.ID})
	} else {
		query = query.Where("status = ?", models.ScheduledDownloadStatusPending)
	}
	return query.Updates(map[string]any{
		"claim_token": "",
		"claimed_at":  nil,
		"status":      models.ScheduledDownloadStatusCancelled,
		// A reservation counted a submit attempt; one that recorded no Job made
		// none, as markScheduledDownloadEnded also takes back.
		"attempts": gorm.Expr("CASE WHEN status = ? AND COALESCE(job_id, '') = '' THEN attempts - 1 ELSE attempts END",
			models.ScheduledDownloadStatusSubmitted),
		"updated_at": at,
	}).Error
}

// ReconcileDeferredDownloadRows brings rows earlier releases left behind into
// line with a Job that was cancelled, or otherwise ended, before anything ran it:
// a row still pending behind it, and one the sweep marked submitted naming it.
// Both are recorded as the Job ended, with the submit attempt the sweep counted
// taken back. A pending row whose Job retention has deleted is closed as well,
// with its outcome unknown (deferredRowEnd). A row whose Job started, or has not
// ended, is left alone. Startup runs it once; it pages by row id, so a large table
// costs one bounded query per page.
func (ctx *MahresourcesContext) ReconcileDeferredDownloadRows() (int, error) {
	if ctx == nil || ctx.db == nil || ctx.JobService() == nil {
		return 0, nil
	}
	ended := []string{string(jobs.StateCancelled), string(jobs.StateFailed), string(jobs.StateInterrupted), string(jobs.StateSucceeded)}
	reconciled := 0
	for _, source := range []struct {
		status string
		join   string
		args   []any
	}{
		// A submitted row names the Job the sweep queued.
		{models.ScheduledDownloadStatusSubmitted, "JOIN jobs ON jobs.id = scheduled_downloads.job_id", nil},
		// A pending row is the Job it was accepted with (deferredDownloadJobIDOn).
		// The predicate below is deferredRowJob.ended in SQL: a Job that ended
		// before it started, or one that is gone (mappedJobGone).
		{models.ScheduledDownloadStatusPending,
			"JOIN job_source_mappings AS mapping ON mapping.source_kind = ? AND mapping.source_id = CAST(scheduled_downloads.id AS TEXT) AND mapping.job_id <> '' LEFT JOIN jobs ON jobs.id = mapping.job_id",
			[]any{jobMigrationScheduledDownload}},
	} {
		var after uint
		for {
			var candidates []struct {
				ID    uint
				JobID string
				State string
			}
			if err := ctx.db.Table("scheduled_downloads").
				Select("scheduled_downloads.id AS id, COALESCE(jobs.id, '') AS job_id, COALESCE(jobs.state, '') AS state").
				Joins(source.join, source.args...).
				Where("scheduled_downloads.status = ? AND scheduled_downloads.id > ?", source.status, after).
				Where("jobs.id IS NULL OR (jobs.started_at IS NULL AND jobs.state IN ?)", ended).
				Order("scheduled_downloads.id").Limit(jobMigrationReadinessBatchSize).
				Scan(&candidates).Error; err != nil {
				return reconciled, err
			}
			for _, candidate := range candidates {
				status, lastError := deferredRowEnd(jobs.State(candidate.State))
				updates := map[string]any{
					"claim_token": "",
					"claimed_at":  nil,
					"status":      status,
					"last_error":  lastError,
					"updated_at":  time.Now(),
				}
				// The Job's end is final, so only the row is re-checked: still in the
				// state it was read in, and for a pending row not held by a sweep,
				// which decides it itself.
				query := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ? AND status = ?", candidate.ID, source.status)
				if source.status == models.ScheduledDownloadStatusSubmitted {
					updates["attempts"] = gorm.Expr("CASE WHEN attempts > 0 THEN attempts - 1 ELSE 0 END")
					query = query.Where("job_id = ?", candidate.JobID)
				} else {
					query = query.Where(scheduledDownloadClaimFree(ctx.db), time.Now().Add(-ScheduledDownloadClaimTTL))
				}
				res := query.Updates(updates)
				if res.Error != nil {
					return reconciled, res.Error
				}
				reconciled += int(res.RowsAffected)
			}
			if len(candidates) < jobMigrationReadinessBatchSize {
				break
			}
			after = candidates[len(candidates)-1].ID
		}
	}
	return reconciled, nil
}

// PluginScheduledDownloadsFor lists one plugin's scheduled downloads for the
// admin management surfaces.
func (ctx *MahresourcesContext) PluginScheduledDownloadsFor(pluginName string) ([]models.ScheduledDownload, error) {
	var rows []models.ScheduledDownload
	err := ctx.db.Where("plugin_name = ?", pluginName).
		Order(scheduledDownloadInstant(ctx.db, "due_at") + " asc, id asc").Find(&rows).Error
	return rows, err
}

// FireDueScheduledDownloads claims and submits due rows. It returns the count of
// rows that actually produced queue jobs; rows refused during re-validation or
// submit are marked failed, while rows blocked by a live download of the same URL
// are released still-pending for a later tick.
func (ctx *MahresourcesContext) FireDueScheduledDownloads(cfg ScheduledDownloadFireConfig) (int, error) {
	now := cfg.Now
	if now.IsZero() {
		now = time.Now()
	}
	rows, err := ctx.DueScheduledDownloads(now, cfg.Limit)
	if err != nil {
		return 0, err
	}

	fired := 0
	for _, candidate := range rows {
		claim := fmt.Sprintf("scheduled-download-%d-%d", candidate.ID, time.Now().UnixNano())
		claimed, err := ctx.ClaimScheduledDownload(candidate.ID, claim, now)
		if err != nil {
			return fired, err
		}
		if !claimed {
			continue
		}
		row, err := ctx.scheduledDownloadByClaim(candidate.ID, claim)
		if err != nil {
			_ = ctx.ReleaseScheduledDownloadClaim(candidate.ID, claim)
			return fired, err
		}
		if row == nil {
			continue
		}
		ok, err := ctx.fireClaimedScheduledDownload(row, claim, cfg, now)
		if errors.Is(err, errScheduledDownloadClaimLost) {
			// A cancel of the row's Job ended the row while this sweep held it;
			// there is nothing left to record, and the rows behind it still fire.
			continue
		}
		if err != nil {
			return fired, err
		}
		if ok {
			fired++
		}
	}
	return fired, nil
}

func (ctx *MahresourcesContext) fireClaimedScheduledDownload(row *models.ScheduledDownload, claim string, cfg ScheduledDownloadFireConfig, now time.Time) (bool, error) {
	// A row whose Job already ended without running, as an earlier release left a
	// pending row behind a cancelled Job, records that end before anything else is
	// checked: a refusal found now would record the failure of a deferral that was
	// never going to run. So does a row whose Job retention has deleted, which is
	// never submitted again.
	if rowJob, err := ctx.loadDeferredRowJob(ctx.db, row.ID, false); err != nil {
		return false, err
	} else if state, ended := rowJob.ended(); ended {
		return false, ctx.markScheduledDownloadEnded(row.ID, claim, state, now)
	}
	if !ctx.scheduledDownloadPluginAvailable(row.PluginName, cfg.PluginAvailable) {
		return false, ctx.MarkScheduledDownloadFailed(row.ID, claim,
			fmt.Errorf("refusing to submit scheduled download: plugin %q is not enabled or its network policy is unavailable", row.PluginName), now)
	}
	creator, err := ctx.ScheduledDownloadPayload(row)
	if err != nil {
		return false, ctx.MarkScheduledDownloadFailed(row.ID, claim, err, now)
	}
	if row.CreatedByUserId == nil || *row.CreatedByUserId == 0 {
		return false, ctx.MarkScheduledDownloadFailed(row.ID, claim, errors.New("scheduled download has no owner"), now)
	}
	actorID := *row.CreatedByUserId
	scoped := ctx.WithPrincipal(ctx.principalForPluginActor(actorID))
	if err := scoped.requireWriteRole("submit a scheduled download"); err != nil {
		return false, ctx.MarkScheduledDownloadFailed(row.ID, claim, err, now)
	}
	if err := scoped.validateDownloadTargetsInScope(creator); err != nil {
		return false, ctx.MarkScheduledDownloadFailed(row.ID, claim, err, now)
	}
	if cfg.ActiveDownload != nil {
		if _, active := cfg.ActiveDownload(creator.URL); active {
			return false, ctx.DeferScheduledDownloadClaim(row.ID, claim, now.Add(scheduledDownloadActiveURLDefer), now)
		}
	}
	if cfg.Submit == nil {
		return false, ctx.MarkScheduledDownloadFailed(row.ID, claim, errors.New("scheduled download submitter is not configured"), now)
	}
	reserved, err := ctx.ReserveScheduledDownloadSubmit(row.ID, claim, now)
	if err != nil {
		return false, err
	}
	if !reserved {
		return false, nil
	}

	// A row with a durable Job behind it is materialized rather than submitted: the
	// Job was accepted as `scheduled` when the deferral was made, and its due time
	// moves *that* Job to the queue — one execution, one identity, and the dispatch
	// loop is what starts it. Submitting here as well would be the second execution
	// the design forbids.
	var ended *deferredJobEndedError
	if jobID, materialized, err := ctx.materializeDeferredDownloadJob(row.ID); errors.As(err, &ended) {
		return false, ctx.markScheduledDownloadEnded(row.ID, claim, ended.state, now)
	} else if err != nil {
		return false, ctx.markScheduledDownloadFailed(row.ID, claim, err, now, false)
	} else if materialized {
		if err := ctx.MarkScheduledDownloadSubmitted(row.ID, claim, jobID, now); err != nil {
			return false, err
		}
		return true, nil
	}

	owner := actorID
	jobID, err := cfg.Submit(creator, &owner, row.PluginName)
	if err != nil {
		return false, ctx.markScheduledDownloadFailed(row.ID, claim, err, now, false)
	}
	if jobID == "" {
		return false, ctx.markScheduledDownloadFailed(row.ID, claim, errors.New("scheduled download submitter returned no job id"), now, false)
	}
	if err := ctx.MarkScheduledDownloadSubmitted(row.ID, claim, jobID, now); err != nil {
		return false, err
	}
	return true, nil
}

func (ctx *MahresourcesContext) scheduledDownloadPluginAvailable(pluginName string, override func(string) bool) bool {
	if override != nil {
		return override(pluginName)
	}
	if ctx == nil || ctx.pluginManager == nil || pluginName == "" {
		return false
	}
	_, ok := ctx.pluginManager.NetworkPolicyForPlugin(pluginName)
	return ok
}

// materializeDeferredDownloadJob moves one row's deferred Job from `scheduled` to
// the queue, at the moment the row becomes due.
//
// materialized is false when the row has no durable Job — one written before there
// was a control plane — and the caller falls back to submitting the payload itself.
// A Job that has moved on (already queued by an earlier tick, running, blocked for a
// person, or ended after it started) is left exactly as it is: the row's own claim
// is what stops a second materialization, and a Job in any other state has already
// been decided about. A Job that ended in a way the row records instead
// (deferredRowJob.ended) is reported as a *deferredJobEndedError.
func (ctx *MahresourcesContext) materializeDeferredDownloadJob(rowID uint) (string, bool, error) {
	rowJob, err := ctx.loadDeferredRowJob(ctx.db, rowID, false)
	if err != nil || rowJob.ID == "" {
		return "", false, err
	}
	if state, ended := rowJob.ended(); ended {
		return "", false, &deferredJobEndedError{state: state}
	}
	if rowJob.Job.State != jobs.StateScheduled {
		return rowJob.ID, true, nil
	}
	_, err = ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
		JobID:           rowJob.ID,
		ExpectedVersion: rowJob.Job.Version,
		To:              jobs.StateQueued,
	})
	if errors.Is(err, jobs.ErrVersionConflict) || errors.Is(err, jobs.ErrIllegalTransition) {
		// The Job moved between the read and the write — the dispatch loop
		// promoted it, or someone cancelled it. That is the moved-on case above,
		// reached by losing a race rather than by arriving late, and it gets the
		// same answer. Reporting the conflict instead marked the row failed while
		// the download it asked for ran.
		current, readErr := ctx.loadDeferredRowJob(ctx.db, rowID, false)
		if readErr != nil {
			return "", false, readErr
		}
		if state, ended := current.ended(); ended {
			return "", false, &deferredJobEndedError{state: state}
		}
		if current.Job.State != jobs.StateScheduled {
			return current.ID, true, nil
		}
	}
	if err != nil {
		return "", false, err
	}
	return rowJob.ID, true, nil
}

// deferredJobEndedError reports a deferred Job that ended before anything ran it,
// or, with no state, one retention deleted before the row recorded how it ended.
type deferredJobEndedError struct {
	state jobs.State
}

func (e *deferredJobEndedError) Error() string {
	if e.state == "" {
		return "the deferred download's Job ended and was removed from Job history before this row recorded its outcome, so whether it ran is not known; the row does not submit it again"
	}
	return fmt.Sprintf("the deferred download's Job ended %s before it started", e.state)
}

// deferredRowEnd is what a row records when its Job ended before the row recorded
// a submission: cancelled when the Job was cancelled before it ran, and failed,
// with the reason, otherwise. That includes a Job retention has deleted: whether
// it ran is not known, so the row is refused the way every other row the sweep
// will not submit is, rather than said to be cancelled or submitted.
func deferredRowEnd(state jobs.State) (status, lastError string) {
	if state == jobs.StateCancelled {
		return models.ScheduledDownloadStatusCancelled, ""
	}
	return models.ScheduledDownloadStatusFailed, (&deferredJobEndedError{state: state}).Error()
}

// markScheduledDownloadEnded records, on a row this sweep holds, that its Job
// ended before the row recorded a submission (deferredRowEnd). A reservation counted a submit
// attempt the row never made, so it is taken back.
func (ctx *MahresourcesContext) markScheduledDownloadEnded(id uint, claimToken string, state jobs.State, at time.Time) error {
	status, lastError := deferredRowEnd(state)
	updates := map[string]any{
		"claim_token": "",
		"claimed_at":  nil,
		"status":      status,
		"last_error":  lastError,
		"attempts": gorm.Expr("CASE WHEN status = ? THEN attempts - 1 ELSE attempts END",
			models.ScheduledDownloadStatusSubmitted),
		"updated_at": at,
	}
	res := ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Where("status IN ?", []string{models.ScheduledDownloadStatusPending, models.ScheduledDownloadStatusSubmitted}).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		return nil
	}
	return fmt.Errorf("scheduled download %d: %w", id, errScheduledDownloadClaimLost)
}
