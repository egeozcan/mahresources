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
	// CompletedBefore and CompletedAfter bound when the download *finished*,
	// which CreatedBefore/CreatedAfter deliberately do not: those bound when it
	// was submitted, and a download queued on Monday can finish on Tuesday.
	CompletedBefore string
	CompletedAfter  string
	// Reason keeps only downloads whose stored error text classifies into one of
	// the DownloadFailureReasons below, or into DownloadReasonOther. Anything
	// else — empty, or a key the UI never emits — matches every row.
	Reason string
	// Error is a LIKE search over the stored error text, AND-ed with Reason: the
	// category narrows to a kind of failure, this narrows to a particular one.
	Error  string
	SortBy []string

	OwnerUserID     *uint `schema:"-"`
	OwnerRestricted bool  `schema:"-"`
}

// The two values the Retried filter recognises. Anything else means "both".
const (
	DownloadRetriedYes = "yes"
	DownloadRetriedNo  = "no"
)

// The keys the Reason filter recognises. DownloadReasonOther is deliberately not
// in DownloadFailureReasons: it is defined as the complement of every entry
// there, so it cannot be given patterns of its own without contradicting them.
const (
	DownloadReasonHTTP        = "http"
	DownloadReasonTimeout     = "timeout"
	DownloadReasonBlocked     = "blocked"
	DownloadReasonUnsupported = "unsupported"
	DownloadReasonLimit       = "limit"
	DownloadReasonStorage     = "storage"
	DownloadReasonCancelled   = "cancelled"
	DownloadReasonOther       = "other"
)

// DownloadFailureReason is one bucket of the failure-reason filter.
type DownloadFailureReason struct {
	Key   string
	Title string
	// Patterns classify a stored error message into this reason. They are
	// matched with the engine's case-insensitive LIKE operator and are bound as
	// parameters, never concatenated into the SQL.
	Patterns []string
}

// DownloadFailureReasons is the whole classification, in the order the filter
// offers it. One table rather than two, because the "Other" bucket is the
// complement of every pattern here and a second copy would drift from it the
// first time a bucket gained a pattern.
//
// The buckets deliberately overlap: `HTTP 504: Gateway Timeout` is in both
// `http` and `timeout`, and it is the right answer to either question. Making
// them exclusive would need a priority order, and every order hides that row
// from somebody who went looking for it. Only "Other" is defined by exclusion.
//
// This is a best-effort match on text, not a stored classification: the download
// path's failures are mostly fmt.Errorf strings with no type to match on (see
// download_queue/manager.go, hls/fetch.go, plugin_system/egress.go, whose error
// type is unexported), so there is nothing more precise available at query time.
// Classifying at write time instead would buy exactness for new rows only and
// cost a column plus a migration, and would stop improvements to this table from
// reaching the rows already stored. Every pattern below is taken from a literal
// that exists in the tree.
var DownloadFailureReasons = []DownloadFailureReason{
	{
		Key:   DownloadReasonHTTP,
		Title: "HTTP error from the server",
		// download_queue/manager.go's and hls/fetch.go's "HTTP %d: %s", which both
		// raise on *any* non-2xx — so 1xx and 3xx are reachable too, and matching
		// only 4xx and 5xx filed an unfollowed 304 under "Other".
		Patterns: []string{"%http 1%", "%http 3%", "%http 4%", "%http 5%"},
	},
	{
		Key:   DownloadReasonTimeout,
		Title: "Timed out or stalled",
		Patterns: []string{
			"%timed out%",
			"%timeout%",
			"%stopped sending data%",
			"%deadline exceeded%",
		},
	},
	{
		Key:   DownloadReasonBlocked,
		Title: "Refused by network policy",
		// "blocked request", not "blocked request to": the egress refusal omits
		// the host when naming it would tell the caller what an address resolved
		// to, and that host-hiding form is the one an ordinary loopback URL
		// produces — so matching on the longer prefix missed the single most
		// common blocked download there is.
		Patterns: []string{
			"%blocked request%",
			"%refusing to fetch%",
		},
	},
	{
		Key:   DownloadReasonUnsupported,
		Title: "Unsupported or DRM-protected",
		// hls refuses by writing a sentence, and there is no marker common to all
		// of them — so these are the distinctive fragments of each, with
		// "%this server cannot%" covering the several that end "which this server
		// cannot <verb>". A refusal that lands in none of them shows up under
		// "Other", which is the point of that bucket.
		Patterns: []string{
			"%drm%",
			"%cannot be downloaded%",
			"%this server cannot%",
			"%does not handle%",
			"%no playable%",
			"%trick-play%",
			// A live stream: "only complete recordings can be downloaded", which
			// the "cannot be downloaded" pattern reads past.
			"%only complete recordings%",
			"%no media segments%",
			"%levels deep%",
		},
	},
	{
		Key:   DownloadReasonLimit,
		Title: "Over a server limit",
		Patterns: []string{
			"%over this server's limit%",
			"%queue is full%",
			"%larger than the%",
		},
	},
	{
		Key:   DownloadReasonStorage,
		Title: "Could not write the file",
		// The last three are the raw *os.PathError a filesystem write surfaces
		// unwrapped through AddResource — the ordinary way a storage failure
		// actually reaches a history row.
		Patterns: []string{
			"%could not write%",
			"%could not create a working directory%",
			"%no space left%",
			"%permission denied%",
			"%read-only file system%",
			"%file exists%",
		},
	},
	{
		Key:   DownloadReasonCancelled,
		Title: "Cancelled",
		// The word alone, because a cancellation is stamped with four different
		// sentences depending on where it landed — "Download cancelled", "Cancelled
		// before starting", and "Cancelled after the file had been saved" among
		// them — and enumerating them means this bucket silently loses one every
		// time download_queue gains a case, with the row surfacing under "Other".
		Patterns: []string{"%cancelled%"},
	},
}
