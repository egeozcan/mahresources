package query_models

// DownloadHistoryQuery defines the filters the /downloads page and the
// GET /v1/downloads endpoint accept.
//
// OwnerUserID and OwnerRestricted are not request fields — the handler sets them
// from the authenticated principal after decoding, so a caller cannot widen its
// own visibility by sending them. Decoding into the same struct is safe because
// the handler overwrites both unconditionally.
type DownloadHistoryQuery struct {
	Status []string // completed | failed | cancelled; empty = all
	URL    string   // LIKE search over URL and name
	// Retried filters on whether the download was ever run again: "yes" keeps
	// only rows that were, "no" only rows that were not, and anything else
	// (empty, or a value the UI never emits) keeps both — the same lenience the
	// other filters apply to input they do not recognise.
	Retried       string
	CreatedBefore string
	CreatedAfter  string
	SortBy        []string

	OwnerUserID     *uint `schema:"-"`
	OwnerRestricted bool  `schema:"-"`
}

// The two values the Retried filter recognises. Anything else means "both".
const (
	DownloadRetriedYes = "yes"
	DownloadRetriedNo  = "no"
)
