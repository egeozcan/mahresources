package models

import "time"

// PluginSchedule is the durable half of a plugin's recurring work.
//
// The other half is not durable and cannot be: a schedule's handler is a
// *lua.LFunction bound to the *lua.LState that created it, so it means nothing
// after a reload. mah.schedule therefore registers the handler in memory, in the
// plugin manager, exactly like every other registration, and this row carries
// everything that has to outlive the process — the interval, the owner, when the
// schedule is next due, and the claim that stops two processes running one tick.
//
// The two halves meet by name, on (PluginName, ScheduleID). That pairing is what
// makes the awkward cases safe by construction rather than by a cleanup pass: a
// row whose plugin is disabled, whose schedule id was renamed, or whose
// plugin.lua was downgraded has no live registration, and a row with no live
// registration is never claimed.
type PluginSchedule struct {
	ID uint `gorm:"primarykey" json:"id"`

	// PluginName and ScheduleID are the plugin's own name for this schedule. The
	// unique index is what makes registration an upsert: enabling a plugin twice,
	// or restarting the process, must not accumulate rows.
	PluginName string `gorm:"uniqueIndex:idx_plugin_schedule_key;size:200;not null" json:"pluginName"`
	ScheduleID string `gorm:"uniqueIndex:idx_plugin_schedule_key;size:200;not null" json:"scheduleId"`

	// EverySeconds is the declared interval. Stored as seconds rather than as a
	// duration string so the due-time arithmetic is the database's and not a
	// parse away.
	EverySeconds int64 `gorm:"not null" json:"everySeconds"`

	// Overlap is what to do when a run is still going at the next due time.
	Overlap string `gorm:"size:20;not null;default:'skip'" json:"overlap"`

	// NextDueAt is the only thing the tick reads to decide what runs. Indexed
	// because the tick's query is "everything due", on every tick, forever.
	NextDueAt time.Time `gorm:"index:idx_plugin_schedule_next_due;not null" json:"nextDueAt"`

	// ClaimToken and ClaimedAt are the compare-and-set slot. A non-empty token
	// means some process is between claiming this row and finishing its run;
	// ClaimedAt is what lets a claim whose process died be treated as abandoned.
	//
	// Unlike the download-history retry slot this claim is held for the whole
	// run, not just across a submit, because ActionJob is in-memory and
	// per-process: a second process has no way to ask whether the first one's run
	// is still live. See ScheduleClaimTTL, which is derived from that fact.
	ClaimToken string     `gorm:"size:80" json:"-"`
	ClaimedAt  *time.Time `json:"claimedAt,omitempty"`

	// The last outcome, for the manage page. LastStatus is one of the
	// PluginScheduleStatus* values below.
	LastRunAt  *time.Time `json:"lastRunAt,omitempty"`
	LastStatus string     `gorm:"size:20" json:"lastStatus,omitempty"`
	LastError  string     `gorm:"size:2000" json:"lastError,omitempty"`
	Runs       int64      `gorm:"not null;default:0" json:"runs"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// CreatedByUserId is the operator who enabled the plugin, and it is the
	// identity every run of this schedule executes as. Named to match the other
	// stamped models so the user-deletion sweep (nullCreatorReferences) covers
	// this table for free.
	//
	// That sweep is load-bearing rather than incidental here. Deleting the
	// operator NULLs this column, a row with no owner is never claimed, and the
	// schedule stops. An unowned timer holding an unbound database handle is
	// precisely what must not happen, so "the owner is gone" resolves to "stop"
	// rather than to "run as root".
	CreatedByUserId *uint `gorm:"index:idx_plugin_schedule_created_by" json:"createdByUserId,omitempty"`
}

func (p PluginSchedule) GetId() uint {
	return p.ID
}

// Overlap policies. Duplicated rather than imported because models/ sits below
// plugin_system in the dependency direction, the same reason the download
// history statuses are duplicated here.
//
// PluginScheduleOverlapAllow buys queueing, not concurrency: two runs of one
// plugin still serialize on that plugin's VM lock.
const (
	PluginScheduleOverlapSkip  = "skip"
	PluginScheduleOverlapAllow = "allow"
)

// Outcomes a completed run records in LastStatus.
const (
	PluginScheduleStatusCompleted = "completed"
	PluginScheduleStatusFailed    = "failed"
)

// ValidPluginScheduleOverlap reports whether a declared overlap policy is one the
// host implements. A plugin declaring anything else is refused at registration
// rather than silently defaulted: "skip" and "allow" mean opposite things when a
// run overruns, so guessing is guessing about the author's intent.
func ValidPluginScheduleOverlap(policy string) bool {
	return policy == PluginScheduleOverlapSkip || policy == PluginScheduleOverlapAllow
}
