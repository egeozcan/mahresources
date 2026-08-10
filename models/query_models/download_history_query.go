package query_models

// DownloadHistoryQuery defines the filters the /downloads page and the
// GET /v1/downloads endpoint accept.
//
// OwnerUserID and OwnerRestricted are not request fields — the handler sets them
// from the authenticated principal after decoding, so a caller cannot widen its
// own visibility by sending them. Decoding into the same struct is safe because
// the handler overwrites both unconditionally.
type DownloadHistoryQuery struct {
	Status        []string // completed | failed | cancelled; empty = all
	URL           string   // LIKE search over URL and name
	CreatedBefore string
	CreatedAfter  string
	SortBy        []string

	OwnerUserID     *uint `schema:"-"`
	OwnerRestricted bool  `schema:"-"`
}
