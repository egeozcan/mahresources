package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"mahresources/auth"
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
	if creator == nil {
		return nil, errors.New("scheduled download needs a payload")
	}
	payload, err := scheduledDownloadPayload(creator)
	if err != nil {
		return nil, err
	}
	row := models.ScheduledDownload{
		PluginName:      truncateRunes(pluginName, maxHistoryPluginNameLength),
		URL:             truncateRunes(creator.URL, maxHistoryURLLength),
		Payload:         payload,
		DueAt:           dueAt,
		Status:          models.ScheduledDownloadStatusPending,
		CreatedByUserId: copyUintPtr(nonZeroUintPtr(actorUserID)),
	}

	db := ctx.db
	if actorUserID != 0 {
		db = ctx.WithPrincipal(&auth.Principal{UserID: actorUserID}).db
	}
	if err := db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func nonZeroUintPtr(v uint) *uint {
	if v == 0 {
		return nil
	}
	return &v
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
		Where("due_at <= ?", now).
		Where("status = ?", models.ScheduledDownloadStatusPending).
		Where("created_by_user_id IS NOT NULL").
		Where("attempts < ?", scheduledDownloadMaxSubmitAttempts).
		Where("COALESCE(claim_token, '') = '' OR claimed_at IS NULL OR claimed_at < ?",
			now.Add(-ScheduledDownloadClaimTTL)).
		Order("due_at asc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// ClaimScheduledDownload takes the short-lived submit slot for one due row.
func (ctx *MahresourcesContext) ClaimScheduledDownload(id uint, claimToken string, now time.Time) (bool, error) {
	if claimToken == "" {
		return false, errors.New("a scheduled download claim needs a token nobody else can produce")
	}
	res := ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ?", id).
		Where("due_at <= ?", now).
		Where("status = ?", models.ScheduledDownloadStatusPending).
		Where("created_by_user_id IS NOT NULL").
		Where("attempts < ?", scheduledDownloadMaxSubmitAttempts).
		Where("COALESCE(claim_token, '') = '' OR claimed_at IS NULL OR claimed_at < ?",
			now.Add(-ScheduledDownloadClaimTTL)).
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
// recording an attempt. Used when no submit was made, for example because a live
// job is already fetching the same URL.
func (ctx *MahresourcesContext) ReleaseScheduledDownloadClaim(id uint, claimToken string) error {
	return ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Updates(map[string]any{"claim_token": "", "claimed_at": nil}).Error
}

// MarkScheduledDownloadSubmitted records the queue job a fire produced and
// releases the claim in the same update.
func (ctx *MahresourcesContext) MarkScheduledDownloadSubmitted(id uint, claimToken, jobID string, at time.Time) error {
	return ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Updates(map[string]any{
			"claim_token": "",
			"claimed_at":  nil,
			"status":      models.ScheduledDownloadStatusSubmitted,
			"job_id":      jobID,
			"last_error":  "",
			"attempts":    gorm.Expr("attempts + 1"),
			"updated_at":  at,
		}).Error
}

// MarkScheduledDownloadFailed records a terminal refusal/failure and releases
// the claim. Failed scheduled downloads are not retried forever by the tick.
func (ctx *MahresourcesContext) MarkScheduledDownloadFailed(id uint, claimToken string, runErr error, at time.Time) error {
	msg := ""
	if runErr != nil {
		msg = truncateRunes(runErr.Error(), maxHistoryErrorLength)
	}
	return ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Updates(map[string]any{
			"claim_token": "",
			"claimed_at":  nil,
			"status":      models.ScheduledDownloadStatusFailed,
			"last_error":  msg,
			"attempts":    gorm.Expr("attempts + 1"),
			"updated_at":  at,
		}).Error
}

// CancelScheduledDownload cancels a pending scheduled download before it is
// submitted. Already submitted/failed/cancelled rows are left unchanged.
func (ctx *MahresourcesContext) CancelScheduledDownload(id uint) (bool, error) {
	now := time.Now()
	res := ctx.db.Model(&models.ScheduledDownload{}).
		Where("id = ?", id).
		Where("status = ?", models.ScheduledDownloadStatusPending).
		Where("COALESCE(claim_token, '') = '' OR claimed_at IS NULL OR claimed_at < ?",
			now.Add(-ScheduledDownloadClaimTTL)).
		Updates(map[string]any{
			"claim_token": "",
			"claimed_at":  nil,
			"status":      models.ScheduledDownloadStatusCancelled,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// PluginScheduledDownloadsFor lists one plugin's scheduled downloads for the
// admin management surfaces.
func (ctx *MahresourcesContext) PluginScheduledDownloadsFor(pluginName string) ([]models.ScheduledDownload, error) {
	var rows []models.ScheduledDownload
	err := ctx.db.Where("plugin_name = ?", pluginName).Order("due_at asc, id asc").Find(&rows).Error
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
		if _, active := cfg.ActiveDownload(row.URL); active {
			return false, ctx.ReleaseScheduledDownloadClaim(row.ID, claim)
		}
	}
	if cfg.Submit == nil {
		return false, ctx.MarkScheduledDownloadFailed(row.ID, claim, errors.New("scheduled download submitter is not configured"), now)
	}
	owner := actorID
	jobID, err := cfg.Submit(creator, &owner, row.PluginName)
	if err != nil {
		return false, ctx.MarkScheduledDownloadFailed(row.ID, claim, err, now)
	}
	if jobID == "" {
		return false, ctx.MarkScheduledDownloadFailed(row.ID, claim, errors.New("scheduled download submitter returned no job id"), now)
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
