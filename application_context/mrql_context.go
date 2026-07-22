package application_context

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/mrql"
)

const mrqlDiagnosticTextLimit = 4000

// mrqlSQL renders the SQL GORM is about to execute without issuing it. Values
// are interpolated using the active dialect so the timeout log can be copied
// directly into a database console.
func mrqlSQL(db *gorm.DB, operation func(*gorm.DB) *gorm.DB) string {
	// The live context is already canceled on a timeout. Use a background
	// context for DryRun only; no database I/O occurs and the built clauses are
	// retained, allowing diagnostics to be generated lazily after failure.
	dryDB := db.Session(&gorm.Session{DryRun: true}).WithContext(context.Background())
	// GORM stores the live execution error on the returned DB object. Clear it
	// on this isolated DryRun clone so statement construction is not short-circuited.
	dryDB.Error = nil
	if dryDB.Statement != nil {
		dryDB.Statement.Error = nil
	}
	dryRun := operation(dryDB)
	if dryRun == nil || dryRun.Statement == nil {
		return ""
	}
	return db.Dialector.Explain(dryRun.Statement.SQL.String(), dryRun.Statement.Vars...)
}

// logMRQLTimeout records enough context to reproduce a timed-out statement.
// Successful and ordinary failed queries remain covered by the normal and
// slow-query logging paths.
func (ctx *MahresourcesContext) logMRQLTimeout(db *gorm.DB, parsed *mrql.Query, phase, sql string, started time.Time, err error) {
	if !errors.Is(err, context.DeadlineExceeded) {
		return
	}

	timeout := ctx.mrqlQueryTimeout()
	elapsed := time.Since(started)
	mrqlText := truncateString(parsed.Source, mrqlDiagnosticTextLimit)
	sqlText := truncateString(sql, mrqlDiagnosticTextLimit)
	details := map[string]interface{}{
		"mrql":              mrqlText,
		"sql":               sqlText,
		"phase":             phase,
		"entityType":        parsed.EntityType.String(),
		"database":          ctx.db.Config.Dialector.Name(),
		"configuredTimeout": timeout.String(),
		"timeoutMs":         timeout.Milliseconds(),
		"elapsedMs":         elapsed.Milliseconds(),
		"error":             err.Error(),
	}
	if db != nil && db.Statement != nil && db.Statement.Context != nil {
		if deadline, ok := db.Statement.Context.Deadline(); ok {
			details["deadline"] = deadline.UTC().Format(time.RFC3339Nano)
		}
	}
	// The application logger below persists the warning for /logs. Emit the same
	// core diagnostics to the process log as well so container/service logs still
	// capture the failure if the database is unhealthy enough to reject the log row.
	stdlog.Printf("MRQL TIMEOUT phase=%q entity=%q database=%q configured_timeout=%q elapsed=%q mrql=%q sql=%q error=%q",
		phase, parsed.EntityType.String(), ctx.db.Config.Dialector.Name(), timeout, elapsed.Round(time.Millisecond), mrqlText, sqlText, err)

	ctx.Logger().Warning(
		models.LogActionSystem,
		"mrql",
		nil,
		parsed.EntityType.String(),
		fmt.Sprintf("MRQL query timed out during %s after %s", phase, elapsed.Round(time.Millisecond)),
		details,
	)
}

func (ctx *MahresourcesContext) executeMRQLFind(db *gorm.DB, dest any, parsed *mrql.Query, phase string) error {
	started := time.Now()
	err := db.Find(dest).Error
	if errors.Is(err, context.DeadlineExceeded) {
		sql := mrqlSQL(db, func(tx *gorm.DB) *gorm.DB { return tx.Find(dest) })
		ctx.logMRQLTimeout(db, parsed, phase, sql, started, err)
	}
	return err
}

func (ctx *MahresourcesContext) executeMRQLCount(db *gorm.DB, dest *int64, parsed *mrql.Query, phase string) error {
	started := time.Now()
	err := db.Count(dest).Error
	if errors.Is(err, context.DeadlineExceeded) {
		sql := mrqlSQL(db, func(tx *gorm.DB) *gorm.DB { return tx.Count(dest) })
		ctx.logMRQLTimeout(db, parsed, phase, sql, started, err)
	}
	return err
}

// MRQLFilterError wraps an error produced while parsing, validating, or
// translating a list-page MRQL filter expression (the package 5 filter bar). It
// lets callers distinguish a bad user-supplied filter — which must render the
// list fail-closed (zero results + banner) or return HTTP 400 — from an
// infrastructural database error. Pos/Length carry the offending token's
// position in the filter input so the bar can underline it 1:1.
type MRQLFilterError struct {
	Message string
	Pos     int
	Length  int
	err     error
}

func (e *MRQLFilterError) Error() string { return e.Message }
func (e *MRQLFilterError) Unwrap() error { return e.err }

// newMRQLFilterError wraps a parse/validation/translate error, lifting its
// position information when available.
func newMRQLFilterError(err error) *MRQLFilterError {
	fe := &MRQLFilterError{Message: err.Error(), err: err}
	var pe *mrql.ParseError
	var ve *mrql.ValidationError
	var te *mrql.TranslateError
	switch {
	case errors.As(err, &pe):
		fe.Message, fe.Pos, fe.Length = pe.Message, pe.Pos, pe.Length
	case errors.As(err, &ve):
		fe.Message, fe.Pos, fe.Length = ve.Message, ve.Pos, ve.Length
	case errors.As(err, &te):
		fe.Message, fe.Pos = te.Message, te.Pos
	}
	return fe
}

// CheckMRQLFilter parses, validates, and translates a list-page MRQL filter
// expression without composing it onto a query, returning a *MRQLFilterError
// (with the offending token's position) when it is invalid, or nil when it is
// valid or empty. Template context providers call it to render fail-closed (a
// banner + zero results) instead of the unfiltered list, so a broken filter
// cannot silently widen a subsequent bulk action's target set.
func (ctx *MahresourcesContext) CheckMRQLFilter(entity mrql.EntityType, expr string) *MRQLFilterError {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	parsed, err := mrql.ParseFilter(entity, expr)
	if err != nil {
		return newMRQLFilterError(err)
	}
	if err := mrql.Validate(parsed); err != nil {
		return newMRQLFilterError(err)
	}
	if _, err := mrql.TranslateWithOptions(parsed, ctx.db, ctx.mrqlTranslateOptions()); err != nil {
		return newMRQLFilterError(err)
	}
	return nil
}

// applyMRQLFilter composes an optional MRQL filter expression directly onto an
// existing list query. ParseFilter rejects sorting/pagination/scope clauses, so
// applying only its WHERE AST preserves the outer query's authorization scopes,
// joins, ordering, and LIMIT and allows ordered indexes to stop at the page size.
// A bad expression returns *MRQLFilterError so callers remain fail closed.
func (ctx *MahresourcesContext) applyMRQLFilter(db *gorm.DB, entity mrql.EntityType, expr string) (*gorm.DB, error) {
	parsed, err := ctx.prepareMRQLFilter(entity, expr)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return db, nil
	}
	return ctx.applyPreparedMRQLFilter(db, parsed)
}

func (ctx *MahresourcesContext) prepareMRQLFilter(entity mrql.EntityType, expr string) (*mrql.Query, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, nil
	}
	parsed, err := mrql.ParseFilter(entity, expr)
	if err != nil {
		return nil, newMRQLFilterError(err)
	}
	if err := mrql.Validate(parsed); err != nil {
		return nil, newMRQLFilterError(err)
	}
	return parsed, nil
}

func (ctx *MahresourcesContext) applyPreparedMRQLFilter(db *gorm.DB, parsed *mrql.Query) (*gorm.DB, error) {
	filtered, err := mrql.ApplyFilterWithOptions(parsed, db, ctx.mrqlTranslateOptions())
	if err != nil {
		return nil, newMRQLFilterError(err)
	}
	return filtered, nil
}

// MRQLResult holds the results of executing an MRQL query, organized by entity type.
type MRQLResult struct {
	EntityType string            `json:"entityType"`
	Resources  []models.Resource `json:"resources,omitempty"`
	Notes      []models.Note     `json:"notes,omitempty"`
	Groups     []models.Group    `json:"groups,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
	// DefaultLimitApplied is true when the query had no explicit LIMIT clause
	// and the server applied the configured default.
	DefaultLimitApplied bool `json:"default_limit_applied"`
	// AppliedLimit is the effective LIMIT that was applied — either the value
	// parsed from the query or the configured default.
	AppliedLimit int `json:"applied_limit,omitempty"`
}

// MRQLGroupedResult holds the results of a GROUP BY MRQL query.
type MRQLGroupedResult struct {
	EntityType  string           `json:"entityType"`
	Mode        string           `json:"mode"` // "aggregated" or "bucketed"
	Rows        []map[string]any `json:"rows,omitempty"`
	Groups      []MRQLBucket     `json:"groups,omitempty"`
	Warnings    []string         `json:"warnings,omitempty"`
	NextOffset  *int             `json:"nextOffset,omitempty"`  // bucketed: offset for next page (nil if no more)
	TotalGroups int              `json:"totalGroups,omitempty"` // bucketed: total group count (before pagination)
	// DefaultLimitApplied is true when the query had no explicit LIMIT clause
	// and the server applied the configured default.
	DefaultLimitApplied bool `json:"default_limit_applied"`
	// AppliedLimit is the effective LIMIT that was applied — either the value
	// parsed from the query or the configured default.
	AppliedLimit int `json:"applied_limit,omitempty"`
}

// MRQLBucket is a single group of entities in bucketed mode.
type MRQLBucket struct {
	Key   map[string]any `json:"key"`
	Items any            `json:"items"` // []models.Resource, []models.Note, or []models.Group
}

// ExecuteMRQL parses, validates, translates, and executes an MRQL query string.
// For single-entity queries it returns typed results; for cross-entity (no type
// specified) it fans out to resources, notes, and groups, merging the results.
// The optional limit and page parameters override the parsed LIMIT/OFFSET when > 0.
// params binds any $name placeholders before validation (nil/empty when none).
func (ctx *MahresourcesContext) ExecuteMRQL(reqCtx context.Context, queryStr string, limit, page int, params map[string]any) (*MRQLResult, error) {
	queryStr = strings.TrimSpace(queryStr)
	if queryStr == "" {
		return nil, errors.New("query string must not be empty")
	}

	parsed, err := mrql.Parse(queryStr)
	if err != nil {
		return nil, err
	}

	// Bind parameter placeholders before validation (type compatibility is
	// checked against the concrete bound values).
	if err := mrql.BindParams(parsed, params); err != nil {
		return nil, err
	}

	if err := mrql.Validate(parsed); err != nil {
		return nil, err
	}

	return ctx.ExecuteMRQLParsed(reqCtx, parsed, limit, page)
}

// ExecuteMRQLParsed executes an already-parsed, bound, and validated flat MRQL
// query under the interactive result-size policy.
func (ctx *MahresourcesContext) ExecuteMRQLParsed(reqCtx context.Context, parsed *mrql.Query, limit, page int) (*MRQLResult, error) {
	return ctx.executeMRQLParsed(reqCtx, parsed, limit, page, interactiveMRQLPolicy)
}

// ExecuteMRQLParsedExport executes a flat query under the larger, still-bounded
// export policy.
func (ctx *MahresourcesContext) ExecuteMRQLParsedExport(reqCtx context.Context, parsed *mrql.Query, limit, page int) (*MRQLResult, error) {
	return ctx.executeMRQLParsed(reqCtx, parsed, limit, page, exportMRQLPolicy)
}

func (ctx *MahresourcesContext) executeMRQLParsed(reqCtx context.Context, parsed *mrql.Query, limit, page int, policy mrqlExecutionPolicy) (*MRQLResult, error) {
	clone := *parsed
	parsed = &clone
	var err error
	// Override parsed LIMIT/OFFSET with request parameters if provided.
	// limit=0 and page=0 mean "not provided" — use the query's own values.
	if limit > 0 {
		parsed.Limit = limit
	}
	if page >= 1 {
		// Explicit page resets offset. page=1 means offset=0 (first page),
		// which also clears any OFFSET baked into the query itself.
		effectiveLimit := parsed.Limit
		if effectiveLimit < 0 {
			effectiveLimit = min(ctx.defaultMRQLLimit(), policy.maxLimit)
		}
		offset, offsetErr := checkedPageOffset(page, effectiveLimit, policy.maxOffset)
		if offsetErr != nil {
			return nil, offsetErr
		}
		parsed.Offset = offset
	}

	// Compute default-limit signaling before replacing the absent limit with its
	// bounded effective value.
	defaultApplied := parsed.Limit < 0
	appliedLimit := parsed.Limit
	if defaultApplied {
		appliedLimit = min(ctx.defaultMRQLLimit(), policy.maxLimit)
		parsed.Limit = appliedLimit
	}
	if err := validateMRQLExecutionBounds(parsed, policy); err != nil {
		return nil, err
	}

	entityType := mrql.ExtractEntityType(parsed)

	opts, deny, err := ctx.mrqlQueryTranslateOptions(parsed)
	if err != nil {
		return nil, err
	}
	if deny {
		return &MRQLResult{EntityType: entityType.String()}, nil
	}

	var result *MRQLResult
	if entityType != mrql.EntityUnspecified {
		result, err = ctx.executeSingleEntity(reqCtx, parsed, entityType, opts, policy)
	} else {
		// Cross-entity: fan out to all three entity types
		result, err = ctx.executeCrossEntity(reqCtx, parsed, opts, policy)
	}
	if err != nil {
		return nil, err
	}
	result.DefaultLimitApplied = defaultApplied
	result.AppliedLimit = appliedLimit
	return result, nil
}

// DefaultMRQLLimitFallback is the historical default LIMIT value used when
// MahresourcesConfig.MRQLDefaultLimit is unset (0). Tests that instantiate
// MahresourcesConfig{} directly rely on this fallback, and main.go sets a
// lower default (500) via the --mrql-default-limit flag for real deployments.
const DefaultMRQLLimitFallback = 1000

// defaultMRQLLimit returns the configured default MRQL LIMIT, or the fallback
// if none is configured. BH-013.
func (ctx *MahresourcesContext) defaultMRQLLimit() int {
	if ctx.settings != nil {
		if v := ctx.settings.MRQLDefaultLimit(); v > 0 {
			return v
		}
	}
	if ctx.Config != nil && ctx.Config.MRQLDefaultLimit > 0 {
		return ctx.Config.MRQLDefaultLimit
	}
	return DefaultMRQLLimitFallback
}

// MRQLPageQueryBudget returns the maximum number of distinct MRQL queries a
// single page render may execute via inline [mrql] shortcodes. 0 disables the
// budget. Runtime overrides win over the boot-time config; unlike the default
// LIMIT there is no non-zero fallback because 0 is a meaningful "disabled" value.
func (ctx *MahresourcesContext) MRQLPageQueryBudget() int {
	if ctx.settings != nil {
		return ctx.settings.MRQLPageQueryBudget()
	}
	if ctx.Config != nil {
		return ctx.Config.MRQLPageQueryBudget
	}
	return 0
}

// rejectSQLiteRegex returns a TranslateError when the query uses a regex match
// operator (~* / !~*) on a non-PostgreSQL database. SQLite has no native regex,
// so the operator is PostgreSQL-only.
func (ctx *MahresourcesContext) rejectSQLiteRegex(q *mrql.Query) error {
	if mrql.ContainsRegexOperator(q) && ctx.db.Config.Dialector.Name() != "postgres" {
		return &mrql.TranslateError{Message: "regex match (~*) requires PostgreSQL", Pos: 0}
	}
	return nil
}

// mrqlTranslateOptions returns TranslateOptions pre-filled with the runtime
// similarity thresholds, so SIMILAR TO predicates match the resource page's
// similarity sidebar (same read path, same live-tunable thresholds).
func (ctx *MahresourcesContext) mrqlTranslateOptions() mrql.TranslateOptions {
	pThreshold, aThreshold := ctx.similarityThresholds()
	ftsAvailable := ctx.ftsEnabled
	return mrql.TranslateOptions{
		SimilarityThreshold: &pThreshold,
		AHashThreshold:      aThreshold,
		FTSAvailable:        &ftsAvailable,
	}
}

// mrqlQueryTranslateOptions resolves effective query scope without leaking
// out-of-subtree SCOPE metadata. Principal scope is checked first: it overrides
// user text for group-limited principals, while unscoped principals retain
// normal explicit SCOPE behavior.
func (ctx *MahresourcesContext) mrqlQueryTranslateOptions(parsed *mrql.Query) (mrql.TranslateOptions, bool, error) {
	opts := ctx.mrqlTranslateOptions()
	if scopeID, forced, deny := ctx.principalForcedScope(); deny {
		return opts, true, nil
	} else if forced {
		opts.ScopeGroupID = scopeID
		return opts, false, nil
	}
	if parsed.Scope != nil {
		scopeID, err := mrql.ResolveScope(parsed, ctx.db)
		if err != nil {
			return opts, false, err
		}
		opts.ScopeGroupID = scopeID
	}
	return opts, false, nil
}

// mrqlQueryTimeout returns the current MRQL query timeout from runtime settings,
// falling back to 10s if settings haven't been wired (test contexts).
func (ctx *MahresourcesContext) mrqlQueryTimeout() time.Duration {
	if ctx.settings != nil {
		return ctx.settings.MRQLQueryTimeout()
	}
	return 10 * time.Second
}

// MRQLQueryTimeout exposes the runtime-aware timeout to render surfaces that
// must bound the entire post-query rendering phase, including nested MRQL.
func (ctx *MahresourcesContext) MRQLQueryTimeout() time.Duration {
	return ctx.mrqlQueryTimeout()
}

// maxBucketedTotalItems caps the total number of entity items materialized
// across all buckets, preventing a single bucketed query from loading
// maxBuckets × defaultMRQLLimit entities into memory.
const maxBucketedTotalItems = 10000

// maxBucketQueries bounds SQL fan-out until bucket materialization is replaced
// by a set-based window query. Continuation remains available through NextOffset.
const maxBucketQueries = 200

// executeSingleEntity runs the query against a single entity table.
func (ctx *MahresourcesContext) executeSingleEntity(reqCtx context.Context, parsed *mrql.Query, entityType mrql.EntityType, opts mrql.TranslateOptions, policy mrqlExecutionPolicy) (*MRQLResult, error) {
	parsed.EntityType = entityType
	if parsed.Limit < 0 {
		parsed.Limit = min(ctx.defaultMRQLLimit(), policy.maxLimit)
	}
	if err := validateMRQLExecutionBounds(parsed, policy); err != nil {
		return nil, err
	}

	// Derive timeout from the request context so client disconnects cancel the query.
	queryCtx, cancel := context.WithTimeout(reqCtx, ctx.mrqlQueryTimeout())
	defer cancel()

	db, err := mrql.TranslateWithOptions(parsed, ctx.db.WithContext(queryCtx), opts)
	if err != nil {
		return nil, err
	}

	// Apply a default limit cap if the query has no explicit LIMIT.
	if parsed.Limit < 0 {
		db = db.Limit(ctx.defaultMRQLLimit())
	}

	result := &MRQLResult{EntityType: entityType.String()}

	switch entityType {
	case mrql.EntityResource:
		var resources []models.Resource
		if err := ctx.executeMRQLFind(db, &resources, parsed, "flat resource select"); err != nil {
			return nil, err
		}
		result.Resources = resources
	case mrql.EntityNote:
		var notes []models.Note
		if err := ctx.executeMRQLFind(db, &notes, parsed, "flat note select"); err != nil {
			return nil, err
		}
		result.Notes = notes
	case mrql.EntityGroup:
		var groups []models.Group
		if err := ctx.executeMRQLFind(db, &groups, parsed, "flat group select"); err != nil {
			return nil, err
		}
		result.Groups = groups
	}

	return result, nil
}

// ExecuteMRQLGrouped executes a GROUP BY query under the interactive policy.
func (ctx *MahresourcesContext) ExecuteMRQLGrouped(reqCtx context.Context, parsed *mrql.Query) (*MRQLGroupedResult, error) {
	return ctx.executeMRQLGrouped(reqCtx, parsed, interactiveMRQLPolicy)
}

// ExecuteMRQLGroupedExport executes a GROUP BY query under the export policy.
func (ctx *MahresourcesContext) ExecuteMRQLGroupedExport(reqCtx context.Context, parsed *mrql.Query) (*MRQLGroupedResult, error) {
	return ctx.executeMRQLGrouped(reqCtx, parsed, exportMRQLPolicy)
}

func (ctx *MahresourcesContext) executeMRQLGrouped(reqCtx context.Context, parsed *mrql.Query, policy mrqlExecutionPolicy) (*MRQLGroupedResult, error) {
	clone := *parsed
	parsed = &clone
	queryCtx, cancel := context.WithTimeout(reqCtx, ctx.mrqlQueryTimeout())
	defer cancel()

	// BH-013: record whether the default kicked in before mutating parsed.Limit.
	defaultApplied := parsed.Limit < 0

	// Apply a bounded default limit when no explicit LIMIT was specified.
	if parsed.Limit < 0 {
		parsed.Limit = min(ctx.defaultMRQLLimit(), policy.maxLimit)
	}
	if err := validateMRQLExecutionBounds(parsed, policy); err != nil {
		return nil, err
	}

	opts, deny, err := ctx.mrqlQueryTranslateOptions(parsed)
	if err != nil {
		return nil, err
	}
	if deny {
		return &MRQLGroupedResult{}, nil
	}

	var result *MRQLGroupedResult
	if len(parsed.GroupBy.Aggregates) > 0 {
		// Aggregated: Limit is standard row pagination — no clamping
		result, err = ctx.executeAggregatedQuery(queryCtx, parsed, opts)
	} else {
		// Bucketed: clamp per-bucket limit so no single bucket exceeds the item cap
		if parsed.Limit > maxBucketedTotalItems {
			parsed.Limit = maxBucketedTotalItems
		}
		result, err = ctx.executeBucketedQuery(queryCtx, parsed, opts)
	}
	if err != nil {
		return nil, err
	}
	result.DefaultLimitApplied = defaultApplied
	result.AppliedLimit = parsed.Limit
	return result, nil
}

func (ctx *MahresourcesContext) executeAggregatedQuery(reqCtx context.Context, parsed *mrql.Query, opts mrql.TranslateOptions) (*MRQLGroupedResult, error) {
	db := ctx.db.WithContext(reqCtx)
	built, err := mrql.BuildAggregatedGroupBy(parsed, db, opts)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	if err := ctx.executeMRQLFind(built, &rows, parsed, "aggregated group select"); err != nil {
		return nil, err
	}

	// Ensure Rows is never nil for consistent JSON
	if rows == nil {
		rows = []map[string]any{}
	}

	return &MRQLGroupedResult{
		EntityType: parsed.EntityType.String(),
		Mode:       "aggregated",
		Rows:       rows,
	}, nil
}

func (ctx *MahresourcesContext) executeBucketedQuery(reqCtx context.Context, parsed *mrql.Query, opts mrql.TranslateOptions) (*MRQLGroupedResult, error) {
	db := ctx.db.WithContext(reqCtx)

	keysDB, err := mrql.BuildGroupByKeys(parsed, db, opts)
	if err != nil {
		return nil, err
	}
	var allKeys []map[string]any
	if err := ctx.executeMRQLFind(keysDB, &allKeys, parsed, "bucket key discovery"); err != nil {
		return nil, err
	}

	var warnings []string

	// Detect MaxBuckets ceiling truncation (only for unpaginated queries)
	isPaginated := parsed.BucketLimit >= 0 || parsed.Offset >= 0
	if len(allKeys) > mrql.MaxBuckets {
		allKeys = allKeys[:mrql.MaxBuckets]
		if !isPaginated {
			warnings = append(warnings, fmt.Sprintf("Only the first %d groups are shown. Add filters to narrow the result set.", mrql.MaxBuckets))
		}
	}

	// Apply pagination in-memory: OFFSET skips keys, BucketLimit caps page size.
	// This is done here (not in SQL) so the item cap and page slicing interact
	// correctly — a page cut short by the item cap doesn't cause the next page
	// to skip over un-materialized buckets.
	keys := allKeys
	if parsed.Offset > 0 {
		if parsed.Offset >= len(keys) {
			keys = nil
		} else {
			keys = keys[parsed.Offset:]
		}
	}
	pageSize := len(keys)
	if parsed.BucketLimit >= 0 && parsed.BucketLimit < pageSize {
		pageSize = parsed.BucketLimit
	}
	keys = keys[:pageSize]
	requestedKeys := len(keys)
	if len(keys) > maxBucketQueries {
		keys = keys[:maxBucketQueries]
		warnings = append(warnings, fmt.Sprintf("This page is limited to %d bucket queries; continue at the next offset for remaining groups.", maxBucketQueries))
	}

	var buckets []MRQLBucket
	totalItems := 0
	totalKeys := requestedKeys
	capOverflow := false
	for _, key := range keys {
		// Stop adding buckets once we've exceeded the global item cap.
		// Each bucket gets its full per-bucket LIMIT — we never truncate a
		// bucket mid-way, which would make its remaining items unreachable.
		if totalItems >= maxBucketedTotalItems {
			break
		}

		bucketDB, err := mrql.TranslateGroupByBucket(parsed, ctx.db.WithContext(reqCtx), key, opts)
		if err != nil {
			return nil, err
		}

		// Build public key — rename internal _gbid_ fields to user-friendly
		// <field>_id keys so same-named relation buckets are distinguishable.
		publicKey := make(map[string]any, len(key))
		for k, v := range key {
			if strings.HasPrefix(k, "_gbid_") {
				friendlyKey := strings.TrimPrefix(k, "_gbid_") + "_id"
				publicKey[friendlyKey] = v
			} else {
				publicKey[k] = v
			}
		}
		bucket := MRQLBucket{Key: publicKey}
		remaining := maxBucketedTotalItems - totalItems
		probeLimit := remaining + 1
		if parsed.Limit >= 0 && parsed.Limit < probeLimit {
			probeLimit = parsed.Limit
		}
		bucketDB = bucketDB.Limit(probeLimit)
		bucketItems := 0

		switch parsed.EntityType {
		case mrql.EntityResource:
			var resources []models.Resource
			if err := ctx.executeMRQLFind(bucketDB, &resources, parsed, "bucket resource select"); err != nil {
				return nil, err
			}
			bucket.Items, bucketItems = resources, len(resources)
		case mrql.EntityNote:
			var notes []models.Note
			if err := ctx.executeMRQLFind(bucketDB, &notes, parsed, "bucket note select"); err != nil {
				return nil, err
			}
			bucket.Items, bucketItems = notes, len(notes)
		case mrql.EntityGroup:
			var groups []models.Group
			if err := ctx.executeMRQLFind(bucketDB, &groups, parsed, "bucket group select"); err != nil {
				return nil, err
			}
			bucket.Items, bucketItems = groups, len(groups)
		}
		if bucketItems > remaining {
			capOverflow = true
			break
		}
		totalItems += bucketItems
		buckets = append(buckets, bucket)
	}

	if buckets == nil {
		buckets = []MRQLBucket{}
	}

	if (capOverflow || totalItems >= maxBucketedTotalItems) && len(buckets) < totalKeys {
		droppedGroups := totalKeys - len(buckets)
		warnings = append(warnings, fmt.Sprintf(
			"Results truncated at %d items (%d of %d groups shown, %d groups omitted). Narrow your query or add a filter.",
			maxBucketedTotalItems, len(buckets), totalKeys, droppedGroups))
	}

	// Compute cursor for next page based on actual buckets materialized.
	offset := 0
	if parsed.Offset > 0 {
		offset = parsed.Offset
	}
	actualNextOffset := offset + len(buckets)
	var nextOffset *int
	if actualNextOffset < len(allKeys) {
		nextOffset = &actualNextOffset
	}

	return &MRQLGroupedResult{
		EntityType:  parsed.EntityType.String(),
		Mode:        "bucketed",
		Groups:      buckets,
		Warnings:    warnings,
		NextOffset:  nextOffset,
		TotalGroups: len(allKeys),
	}, nil
}

// crossEntityItem wraps any entity with its common sortable fields for global ordering.
type crossEntityItem struct {
	entityType string
	name       string
	created    time.Time
	updated    time.Time
	index      int // original index within its type slice
	rand       int // random sort key for ORDER BY RANDOM() (unique via rand.Perm)
}

var crossEntityTypes = []mrql.EntityType{mrql.EntityResource, mrql.EntityNote, mrql.EntityGroup}

func crossEntitySelectQuery(parsed *mrql.Query, entityType mrql.EntityType, perEntityCap int) mrql.Query {
	branch := *parsed
	branch.EntityType = entityType
	branch.Limit = perEntityCap
	branch.Offset = -1
	return branch
}

// executeCrossEntity runs the query against resources, notes, and groups
// concurrently, then globally sorts and paginates the merged result set.
// Each entity query gets its own timeout so a slow table doesn't block the
// others. If an entity times out, its results are omitted and a warning is
// included in the response.
func (ctx *MahresourcesContext) executeCrossEntity(reqCtx context.Context, parsed *mrql.Query, opts mrql.TranslateOptions, policy mrqlExecutionPolicy) (*MRQLResult, error) {
	// Up-front regex/dialect gate: this path swallows per-entity TranslateErrors
	// (a non-resolvable entity is skipped), so a SQLite regex query without a
	// `type =` filter would otherwise return silent empty results instead of a
	// clear 400. Reject it here before fan-out. The per-comparison TranslateError
	// in the translator remains as defense-in-depth for the determined-entity path.
	if err := ctx.rejectSQLiteRegex(parsed); err != nil {
		return nil, err
	}

	result := &MRQLResult{EntityType: "all"}

	globalLimit, globalOffset, perEntityCap, err := crossEntityWindow(parsed, ctx.defaultMRQLLimit(), policy)
	if err != nil {
		return nil, err
	}

	var (
		allResources []models.Resource
		allNotes     []models.Note
		allGroups    []models.Group
		mu           sync.Mutex
		warnings     []string
	)

	var wg sync.WaitGroup
	errs := make([]error, len(crossEntityTypes))

	for i, et := range crossEntityTypes {
		branch := crossEntitySelectQuery(parsed, et, perEntityCap)

		// Each entity gets its own timeout derived from the request context.
		entityCtx, cancel := context.WithTimeout(reqCtx, ctx.mrqlQueryTimeout())

		db, err := mrql.TranslateWithOptions(&branch, ctx.db.WithContext(entityCtx), opts)
		if err != nil {
			cancel()
			var translateErr *mrql.TranslateError
			if errors.As(err, &translateErr) {
				continue
			}
			return nil, err
		}

		wg.Add(1)
		go func(idx int, et mrql.EntityType, cancel context.CancelFunc, db *gorm.DB, branch mrql.Query) {
			defer wg.Done()
			defer cancel()

			switch et {
			case mrql.EntityResource:
				var resources []models.Resource
				if err := ctx.executeMRQLFind(db, &resources, &branch, "cross-entity resource select"); err != nil {
					errs[idx] = fmt.Errorf("resource query failed: %w", err)
					return
				}
				mu.Lock()
				allResources = resources
				mu.Unlock()
			case mrql.EntityNote:
				var notes []models.Note
				if err := ctx.executeMRQLFind(db, &notes, &branch, "cross-entity note select"); err != nil {
					errs[idx] = fmt.Errorf("note query failed: %w", err)
					return
				}
				mu.Lock()
				allNotes = notes
				mu.Unlock()
			case mrql.EntityGroup:
				var groups []models.Group
				if err := ctx.executeMRQLFind(db, &groups, &branch, "cross-entity group select"); err != nil {
					errs[idx] = fmt.Errorf("group query failed: %w", err)
					return
				}
				mu.Lock()
				allGroups = groups
				mu.Unlock()
			}
		}(i, et, cancel, db, branch)
	}

	wg.Wait()

	// Collect timeout errors as warnings; return non-timeout errors as failures.
	for _, err := range errs {
		if err == nil {
			continue
		}
		if errors.Is(err, context.DeadlineExceeded) {
			warnings = append(warnings, err.Error())
		} else {
			return nil, err
		}
	}
	result.Warnings = warnings

	// Build unified sortable items. When ORDER BY RANDOM() is the primary sort,
	// select a population-proportional sample instead of merging every fetched
	// row: the per-entity cap fetches at most globalOffset+globalLimit rows per
	// type, so merging them all overrepresents rare entity types (99 notes + 1
	// group would include the lone group ~91% of the time instead of ~1%). The
	// downstream shuffle/offset/limit then run over the proportional sample.
	// RANDOM() used only as a tiebreak (e.g. `ORDER BY created, RANDOM()`) keeps
	// the plain merge — the leading field already fixes the global top-k.
	var items []crossEntityItem
	if len(parsed.OrderBy) > 0 && parsed.OrderBy[0].Random {
		items = ctx.proportionalRandomItems(reqCtx, parsed, opts, allResources, allNotes, allGroups, perEntityCap, globalOffset+globalLimit)
	} else {
		items = make([]crossEntityItem, 0, len(allResources)+len(allNotes)+len(allGroups))
		for i, r := range allResources {
			items = append(items, crossEntityItem{entityType: "resource", name: r.Name, created: r.CreatedAt, updated: r.UpdatedAt, index: i})
		}
		for i, n := range allNotes {
			items = append(items, crossEntityItem{entityType: "note", name: n.Name, created: n.CreatedAt, updated: n.UpdatedAt, index: i})
		}
		for i, g := range allGroups {
			items = append(items, crossEntityItem{entityType: "group", name: g.Name, created: g.CreatedAt, updated: g.UpdatedAt, index: i})
		}
	}

	// ORDER BY RANDOM() clauses (Field == nil) compare on a per-item random key.
	// A permutation keeps the keys unique, so the comparator stays a strict
	// order at the clause's position — `ORDER BY RANDOM()` is a global shuffle,
	// `ORDER BY created, RANDOM()` a random tiebreak, matching SQL semantics.
	for _, ob := range parsed.OrderBy {
		if ob.Random {
			for k, p := range rand.Perm(len(items)) {
				items[k].rand = p
			}
			break
		}
	}

	// Global sort if ORDER BY is specified
	if len(parsed.OrderBy) > 0 {
		sort.SliceStable(items, func(i, j int) bool {
			for _, ob := range parsed.OrderBy {
				if ob.Random {
					// Unique keys: never a tie, no direction (parser enforces).
					return items[i].rand < items[j].rand
				}
				fieldName := ob.Field.Name()
				cmp := 0
				switch fieldName {
				case "name":
					cmp = strings.Compare(strings.ToLower(items[i].name), strings.ToLower(items[j].name))
				case "created":
					if items[i].created.Before(items[j].created) {
						cmp = -1
					} else if items[i].created.After(items[j].created) {
						cmp = 1
					}
				case "updated":
					if items[i].updated.Before(items[j].updated) {
						cmp = -1
					} else if items[i].updated.After(items[j].updated) {
						cmp = 1
					}
				default:
					continue // unsortable field in cross-entity context
				}
				if cmp == 0 {
					continue // tie, try next ORDER BY column
				}
				if !ob.Ascending {
					cmp = -cmp
				}
				return cmp < 0
			}
			return false // all equal
		})
	}

	// Apply global OFFSET
	if globalOffset > 0 {
		if globalOffset >= len(items) {
			items = nil
		} else {
			items = items[globalOffset:]
		}
	}

	// Apply global LIMIT
	if len(items) > globalLimit {
		items = items[:globalLimit]
	}

	// Split back into typed slices, preserving the global sort order
	resourceIndices := make(map[int]bool)
	noteIndices := make(map[int]bool)
	groupIndices := make(map[int]bool)
	for _, item := range items {
		switch item.entityType {
		case "resource":
			resourceIndices[item.index] = true
		case "note":
			noteIndices[item.index] = true
		case "group":
			groupIndices[item.index] = true
		}
	}

	// Rebuild slices preserving global order
	result.Resources = make([]models.Resource, 0, len(resourceIndices))
	result.Notes = make([]models.Note, 0, len(noteIndices))
	result.Groups = make([]models.Group, 0, len(groupIndices))
	for _, item := range items {
		switch item.entityType {
		case "resource":
			result.Resources = append(result.Resources, allResources[item.index])
		case "note":
			result.Notes = append(result.Notes, allNotes[item.index])
		case "group":
			result.Groups = append(result.Groups, allGroups[item.index])
		}
	}

	return result, nil
}

// proportionalRandomItems selects up to k items across the three entity types
// with probability proportional to each type's true (filtered) population, so a
// cross-entity ORDER BY RANDOM() sample is not skewed toward small types by the
// per-entity fetch cap. The returned items reference the first take[type] rows of
// each slice (already DB-side randomized); the caller's shuffle/offset/limit run
// over them. Selection is weighted-without-replacement over the populations,
// capped by however many rows were actually fetched.
func (ctx *MahresourcesContext) proportionalRandomItems(
	reqCtx context.Context, parsed *mrql.Query, opts mrql.TranslateOptions,
	resources []models.Resource, notes []models.Note, groups []models.Group,
	perEntityCap, k int,
) []crossEntityItem {
	// A slice shorter than the cap means the whole (filtered) population was
	// fetched, so its length is the true count. One at the cap may have more rows;
	// count it so its weight reflects the real population.
	trueCount := func(et mrql.EntityType, fetched int) int {
		if fetched < perEntityCap {
			return fetched
		}
		n, err := ctx.countCrossEntity(reqCtx, parsed, opts, et)
		if err != nil || int(n) < fetched {
			return fetched
		}
		return int(n)
	}

	order := []string{"resource", "note", "group"}
	avail := map[string]int{"resource": len(resources), "note": len(notes), "group": len(groups)}
	remaining := map[string]int{
		"resource": trueCount(mrql.EntityResource, len(resources)),
		"note":     trueCount(mrql.EntityNote, len(notes)),
		"group":    trueCount(mrql.EntityGroup, len(groups)),
	}
	total := remaining["resource"] + remaining["note"] + remaining["group"]

	take := map[string]int{}
	for picks := 0; picks < k && total > 0; picks++ {
		r := rand.IntN(total)
		chosen := order[len(order)-1]
		acc := 0
		for _, e := range order {
			acc += remaining[e]
			if r < acc {
				chosen = e
				break
			}
		}
		// Never draw more than we physically fetched for a type. If a capped type
		// is exhausted, drop its remaining weight and redraw against the others.
		if take[chosen] >= avail[chosen] {
			total -= remaining[chosen]
			remaining[chosen] = 0
			picks--
			continue
		}
		take[chosen]++
		remaining[chosen]--
		total--
	}

	items := make([]crossEntityItem, 0, take["resource"]+take["note"]+take["group"])
	for i := 0; i < take["resource"]; i++ {
		r := resources[i]
		items = append(items, crossEntityItem{entityType: "resource", name: r.Name, created: r.CreatedAt, updated: r.UpdatedAt, index: i})
	}
	for i := 0; i < take["note"]; i++ {
		n := notes[i]
		items = append(items, crossEntityItem{entityType: "note", name: n.Name, created: n.CreatedAt, updated: n.UpdatedAt, index: i})
	}
	for i := 0; i < take["group"]; i++ {
		g := groups[i]
		items = append(items, crossEntityItem{entityType: "group", name: g.Name, created: g.CreatedAt, updated: g.UpdatedAt, index: i})
	}
	return items
}

// countCrossEntity counts the rows one entity type contributes to a cross-entity
// query, applying the same WHERE/scope translation but no ORDER BY / LIMIT /
// OFFSET, so proportionalRandomItems can weight by true population size.
func (ctx *MahresourcesContext) countCrossEntity(reqCtx context.Context, parsed *mrql.Query, opts mrql.TranslateOptions, et mrql.EntityType) (int64, error) {
	clone := *parsed
	clone.EntityType = et
	clone.OrderBy = nil
	clone.Limit = -1
	clone.Offset = -1

	entityCtx, cancel := context.WithTimeout(reqCtx, ctx.mrqlQueryTimeout())
	defer cancel()

	db, err := mrql.TranslateWithOptions(&clone, ctx.db.WithContext(entityCtx), opts)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := ctx.executeMRQLCount(db, &n, &clone, "cross-entity population count"); err != nil {
		return 0, err
	}
	return n, nil
}

// MRQLExecutionShape describes the SQL fan-out of an Effective MRQL Query
// without executing data-dependent discovery.
type MRQLExecutionShape struct {
	Strategy          string `json:"strategy"`
	PlannedStatements int    `json:"plannedStatements"`
	MinimumStatements int    `json:"minimumStatements"`
	MaximumStatements int    `json:"maximumStatements"`
	DataDependent     bool   `json:"dataDependent"`
	Description       string `json:"description,omitempty"`
}

// MRQLExplainOptions selects optional database work. NativePlan is deliberately
// opt-in because it contacts the optimizer; the HTTP boundary restricts it to
// administrators.
type MRQLExplainOptions struct {
	NativePlan bool
}

// MRQLNativePlanError distinguishes optimizer failures from parse, scope, or
// execution-policy errors that happen before native planning begins.
type MRQLNativePlanError struct {
	Err error
}

func (e *MRQLNativePlanError) Error() string { return e.Err.Error() }
func (e *MRQLNativePlanError) Unwrap() error { return e.Err }

// MRQLExplainResult describes the SQL statement(s) that a query would run,
// without executing the underlying query. Mirrors execution semantics: default
// LIMIT, resolved SCOPE, and RBAC forced scope are reflected in emitted SQL.
type MRQLExplainResult struct {
	EntityType          string                  `json:"entityType"`
	QueryFingerprint    string                  `json:"queryFingerprint"`
	ExecutionShape      MRQLExecutionShape      `json:"executionShape"`
	Statements          []mrql.ExplainStatement `json:"statements"`
	Warnings            []string                `json:"warnings,omitempty"`
	DefaultLimitApplied bool                    `json:"default_limit_applied"`
	AppliedLimit        int                     `json:"applied_limit,omitempty"`
}

// explainDest returns a destination slice pointer of the right model type so
// the DryRun SELECT column list matches what execution would fetch.
func explainDest(et mrql.EntityType) any {
	switch et {
	case mrql.EntityResource:
		return &[]models.Resource{}
	case mrql.EntityNote:
		return &[]models.Note{}
	case mrql.EntityGroup:
		return &[]models.Group{}
	}
	return &[]map[string]any{}
}

// explainTableLabel returns the plural table label used for cross-entity
// statement labels (resources/notes/groups).
func explainTableLabel(et mrql.EntityType) string {
	switch et {
	case mrql.EntityResource:
		return "resources"
	case mrql.EntityNote:
		return "notes"
	case mrql.EntityGroup:
		return "groups"
	}
	return et.String()
}

// ExplainMRQL builds generated SQL without contacting the database optimizer.
func (ctx *MahresourcesContext) ExplainMRQL(reqCtx context.Context, parsed *mrql.Query) (*MRQLExplainResult, error) {
	return ctx.ExplainMRQLWithOptions(reqCtx, parsed, MRQLExplainOptions{})
}

// ExplainMRQLWithOptions describes an already-parsed, bound, and validated
// Effective MRQL Query. Native plans contact only the optimizer and share one
// deadline across every generated statement.
func (ctx *MahresourcesContext) ExplainMRQLWithOptions(reqCtx context.Context, parsed *mrql.Query, explainOptions MRQLExplainOptions) (*MRQLExplainResult, error) {
	clone := *parsed
	parsed = &clone
	entityType := mrql.ExtractEntityType(parsed)
	defaultApplied := parsed.Limit < 0
	if defaultApplied {
		parsed.Limit = min(ctx.defaultMRQLLimit(), interactiveMRQLPolicy.maxLimit)
	}
	if err := validateMRQLExecutionBounds(parsed, interactiveMRQLPolicy); err != nil {
		return nil, err
	}
	result := &MRQLExplainResult{
		EntityType:          entityType.String(),
		Statements:          make([]mrql.ExplainStatement, 0),
		DefaultLimitApplied: defaultApplied,
		AppliedLimit:        parsed.Limit,
	}

	queryCtx := reqCtx
	workingCtx := ctx
	if explainOptions.NativePlan {
		var cancel context.CancelFunc
		queryCtx, cancel = context.WithTimeout(reqCtx, ctx.mrqlQueryTimeout())
		defer cancel()
		copyWithDeadline := *ctx
		copyWithDeadline.db = ctx.db.WithContext(queryCtx)
		workingCtx = &copyWithDeadline
	}

	opts, deny, err := workingCtx.mrqlQueryTranslateOptions(parsed)
	if err != nil {
		return nil, err
	}
	scopeShape := workingCtx.mrqlExplainScopeShape(parsed, opts, deny)
	if deny {
		result.QueryFingerprint = workingCtx.mrqlQueryFingerprint(parsed, scopeShape, opts)
		result.ExecutionShape = MRQLExecutionShape{Strategy: "denied"}
		result.Warnings = append(result.Warnings, "access is scoped to no groups; this query would return no rows")
		return result, nil
	}
	if err := workingCtx.rejectSQLiteRegex(parsed); err != nil {
		return nil, err
	}
	db := workingCtx.db.WithContext(queryCtx)

	switch {
	case parsed.GroupBy != nil:
		if entityType == mrql.EntityUnspecified {
			return nil, errors.New("GROUP BY requires an explicit entity type")
		}
		parsed.EntityType = entityType
		if len(parsed.GroupBy.Aggregates) > 0 {
			built, err := mrql.BuildAggregatedGroupBy(parsed, db, opts)
			if err != nil {
				return nil, err
			}
			result.Statements = append(result.Statements, mrql.ExplainDB(built, entityType.String(), &[]map[string]any{}))
			result.ExecutionShape = fixedExecutionShape("aggregate", len(result.Statements))
			break
		}

		// Bucket item statements depend on discovered keys. Do not execute key
		// discovery or fabricate a representative bucket merely for explain.
		if parsed.Limit > maxBucketedTotalItems {
			parsed.Limit = maxBucketedTotalItems
			result.AppliedLimit = parsed.Limit
		}
		keysDB, err := mrql.BuildGroupByKeys(parsed, db, opts)
		if err != nil {
			return nil, err
		}
		result.Statements = append(result.Statements, mrql.ExplainDB(keysDB, "bucket keys", &[]map[string]any{}))
		result.ExecutionShape = MRQLExecutionShape{
			Strategy:          "bucket_fanout",
			PlannedStatements: 1,
			MinimumStatements: 1,
			MaximumStatements: 1 + maxBucketQueries,
			DataDependent:     true,
			Description:       "one key-discovery statement plus one item statement per discovered bucket",
		}
		result.Warnings = append(result.Warnings, "Bucketed GROUP BY runs one item query per group key; only the key-discovery query is shown here.")

	case entityType != mrql.EntityUnspecified:
		parsed.EntityType = entityType
		built, err := mrql.TranslateWithOptions(parsed, db, opts)
		if err != nil {
			return nil, err
		}
		result.Statements = append(result.Statements, mrql.ExplainDB(built, entityType.String(), explainDest(entityType)))
		result.ExecutionShape = fixedExecutionShape("flat", len(result.Statements))

	default:
		_, _, perEntityCap, err := crossEntityWindow(parsed, ctx.defaultMRQLLimit(), interactiveMRQLPolicy)
		if err != nil {
			return nil, err
		}
		for _, et := range crossEntityTypes {
			branch := crossEntitySelectQuery(parsed, et, perEntityCap)
			built, err := mrql.TranslateWithOptions(&branch, db, opts)
			if err != nil {
				var translateErr *mrql.TranslateError
				if errors.As(err, &translateErr) {
					continue
				}
				return nil, err
			}
			result.Statements = append(result.Statements, mrql.ExplainDB(built, explainTableLabel(et), explainDest(et)))
		}
		result.ExecutionShape = fixedExecutionShape("cross_entity", len(result.Statements))
		if len(parsed.OrderBy) > 0 && parsed.OrderBy[0].Random {
			result.ExecutionShape.DataDependent = true
			result.ExecutionShape.MaximumStatements += len(crossEntityTypes)
			result.ExecutionShape.Description = "entity selects plus up to one population count per capped entity branch"
		}
	}

	result.QueryFingerprint = ctx.mrqlQueryFingerprint(parsed, scopeShape, opts)
	if explainOptions.NativePlan {
		plans := make([]*mrql.NativePlan, len(result.Statements))
		for i, statement := range result.Statements {
			if err := queryCtx.Err(); err != nil {
				return nil, err
			}
			plan, err := mrql.NativeExplain(queryCtx, db, statement)
			if err != nil {
				return nil, &MRQLNativePlanError{Err: err}
			}
			plans[i] = plan
		}
		for i := range result.Statements {
			result.Statements[i].NativePlan = plans[i]
		}
	}
	return result, nil
}

func (ctx *MahresourcesContext) mrqlQueryFingerprint(parsed *mrql.Query, scopeShape mrql.ScopeShape, opts mrql.TranslateOptions) string {
	policy := mrql.QueryShapePolicy{Dialect: ctx.db.Dialector.Name(), AHashThreshold: opts.AHashThreshold}
	if opts.SimilarityThreshold != nil {
		policy.SimilarityThreshold = *opts.SimilarityThreshold
	}
	if opts.FTSAvailable != nil {
		policy.FTSAvailable = *opts.FTSAvailable
	}
	return mrql.QueryShapeFingerprintWithPolicy(parsed, scopeShape, policy)
}

func fixedExecutionShape(strategy string, statements int) MRQLExecutionShape {
	return MRQLExecutionShape{
		Strategy:          strategy,
		PlannedStatements: statements,
		MinimumStatements: statements,
		MaximumStatements: statements,
	}
}

func (ctx *MahresourcesContext) mrqlExplainScopeShape(parsed *mrql.Query, opts mrql.TranslateOptions, deny bool) mrql.ScopeShape {
	if deny {
		return mrql.ScopeShapeDenied
	}
	if _, forced, _ := ctx.principalForcedScope(); forced {
		return mrql.ScopeShapeForced
	}
	if parsed.Scope != nil && opts.ScopeGroupID > 0 {
		return mrql.ScopeShapeExplicit
	}
	return mrql.ScopeShapeNone
}

// ValidateMRQL parses and validates an MRQL query string.
func (ctx *MahresourcesContext) ValidateMRQL(queryStr string) (bool, []map[string]any) {
	valid, errs, _ := ctx.ValidateMRQLWithParams(queryStr)
	return valid, errs
}

// ValidateMRQLWithParams returns validation and placeholder names from one AST,
// avoiding the validation endpoint's historical second full parse.
func (ctx *MahresourcesContext) ValidateMRQLWithParams(queryStr string) (bool, []map[string]any, []string) {
	queryStr = strings.TrimSpace(queryStr)
	if queryStr == "" {
		return false, []map[string]any{
			{"message": "query string must not be empty", "pos": 0, "length": 0},
		}, nil
	}
	parsed, err := mrql.Parse(queryStr)
	if err != nil {
		return false, mrqlErrorPayload(err), nil
	}
	params := mrql.ListParams(parsed)
	if err := mrql.Validate(parsed); err != nil {
		return false, mrqlErrorPayload(err), params
	}
	return true, nil, params
}

// MRQLParams returns the parameter placeholder names ($name, without the '$')
// used by the query, in first-appearance order. A parse failure yields nil.
func (ctx *MahresourcesContext) MRQLParams(queryStr string) []string {
	parsed, err := mrql.Parse(strings.TrimSpace(queryStr))
	if err != nil {
		return nil
	}
	return mrql.ListParams(parsed)
}

// ValidateMRQLFilter parses and validates a bare MRQL filter expression (the
// package 5 list-page bar) for the given entity type, using mrql.ParseFilter so
// error positions match the bar input 1:1. Returns whether it is valid and any
// errors with position information. Filter expressions carry no $name params, so
// no params are reported.
func (ctx *MahresourcesContext) ValidateMRQLFilter(entity mrql.EntityType, queryStr string) (bool, []map[string]any) {
	queryStr = strings.TrimSpace(queryStr)
	if queryStr == "" {
		return false, []map[string]any{
			{"message": "query string must not be empty", "pos": 0, "length": 0},
		}
	}

	parsed, err := mrql.ParseFilter(entity, queryStr)
	if err != nil {
		return false, mrqlErrorPayload(err)
	}
	if err := mrql.Validate(parsed); err != nil {
		return false, mrqlErrorPayload(err)
	}
	return true, nil
}

// mrqlErrorPayload converts an mrql parse/validation error into the positioned
// error payload the validate endpoints return.
func mrqlErrorPayload(err error) []map[string]any {
	var parseErr *mrql.ParseError
	if errors.As(err, &parseErr) {
		return []map[string]any{{"message": parseErr.Message, "pos": parseErr.Pos, "length": parseErr.Length}}
	}
	var validationErr *mrql.ValidationError
	if errors.As(err, &validationErr) {
		return []map[string]any{{"message": validationErr.Message, "pos": validationErr.Pos, "length": validationErr.Length}}
	}
	return []map[string]any{{"message": err.Error(), "pos": 0, "length": 0}}
}

// CompleteMRQL returns autocompletion suggestions for the given MRQL query
// string at the specified cursor position.
func (ctx *MahresourcesContext) CompleteMRQL(queryStr string, cursor int) []mrql.Suggestion {
	return mrql.Complete(queryStr, cursor)
}

// filterSuppressedSuggestions are suggestion values the filter bar must never
// offer: clause keywords (the list page owns sort/pagination) and the `type`
// pseudo-field (implied by the page). All are rejected by ParseFilter anyway.
var filterSuppressedSuggestions = map[string]bool{
	"ORDER BY": true,
	"LIMIT":    true,
	"OFFSET":   true,
	"GROUP BY": true,
	"HAVING":   true,
	"SCOPE":    true,
	"type":     true,
}

// CompleteMRQLFilter returns autocompletion suggestions for a bare filter
// expression on the given entity type. It reuses the full-grammar completer by
// prepending an internal `type = <entity> AND ` guard (so field lists narrow to
// the page's entity) and shifting the cursor accordingly; suggestions carry no
// positions (the client computes the replacement range itself), so no reverse
// offset math is needed. Clause-keyword and `type` suggestions are dropped since
// the filter bar rejects them.
func (ctx *MahresourcesContext) CompleteMRQLFilter(entity mrql.EntityType, queryStr string, cursor int) []mrql.Suggestion {
	prefix := "type = " + entity.String() + " AND "
	suggestions := mrql.Complete(prefix+queryStr, cursor+len(prefix))

	filtered := make([]mrql.Suggestion, 0, len(suggestions))
	for _, s := range suggestions {
		if filterSuppressedSuggestions[s.Value] {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
}

// -- Saved MRQL query CRUD --

// CreateSavedMRQLQuery creates a new saved MRQL query.
func (ctx *MahresourcesContext) CreateSavedMRQLQuery(name, query, description string) (*models.SavedMRQLQuery, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("saved MRQL query name must be non-empty")
	}

	if err := ValidateEntityName(name, "saved MRQL query"); err != nil {
		return nil, err
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("saved MRQL query text must be non-empty")
	}

	// Validate the MRQL query syntax and semantics before saving
	parsed, err := mrql.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("invalid MRQL syntax: %w", err)
	}
	if err := mrql.Validate(parsed); err != nil {
		return nil, fmt.Errorf("invalid MRQL query: %w", err)
	}

	saved := models.SavedMRQLQuery{
		Name:        name,
		Query:       query,
		Description: description,
	}

	if err := ctx.db.Create(&saved).Error; err != nil {
		return nil, friendlyUniqueError("saved MRQL query", err)
	}

	ctx.Logger().Info(models.LogActionCreate, "mrql_query", &saved.ID, saved.Name, "Created saved MRQL query", nil)
	ctx.InvalidateSearchCacheByType(EntityTypeMRQLQuery)

	return &saved, nil
}

// GetSavedMRQLQueries returns all saved MRQL queries, ordered by name.
func (ctx *MahresourcesContext) GetSavedMRQLQueries(offset, limit int) ([]models.SavedMRQLQuery, error) {
	var queries []models.SavedMRQLQuery
	q := ctx.db.Order("name ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Find(&queries).Error; err != nil {
		return nil, err
	}
	return queries, nil
}

// GetSavedMRQLQuery returns a single saved MRQL query by ID.
func (ctx *MahresourcesContext) GetSavedMRQLQuery(id uint) (*models.SavedMRQLQuery, error) {
	var query models.SavedMRQLQuery
	if err := ctx.db.First(&query, id).Error; err != nil {
		return nil, err
	}
	return &query, nil
}

// GetSavedMRQLQueryByName returns a single saved MRQL query by name.
func (ctx *MahresourcesContext) GetSavedMRQLQueryByName(name string) (*models.SavedMRQLQuery, error) {
	var query models.SavedMRQLQuery
	if err := ctx.db.Where("name = ?", name).First(&query).Error; err != nil {
		return nil, err
	}
	return &query, nil
}

// UpdateSavedMRQLQuery updates an existing saved MRQL query.
func (ctx *MahresourcesContext) UpdateSavedMRQLQuery(id uint, name, query, description string) (*models.SavedMRQLQuery, error) {
	var saved models.SavedMRQLQuery
	if err := ctx.db.First(&saved, id).Error; err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("saved MRQL query name must be non-empty")
	}

	if err := ValidateEntityName(name, "saved MRQL query"); err != nil {
		return nil, err
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("saved MRQL query text must be non-empty")
	}

	// Validate the MRQL query syntax and semantics before updating
	parsed, err := mrql.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("invalid MRQL syntax: %w", err)
	}
	if err := mrql.Validate(parsed); err != nil {
		return nil, fmt.Errorf("invalid MRQL query: %w", err)
	}

	saved.Name = name
	saved.Query = query
	saved.Description = description

	if err := ctx.db.Save(&saved).Error; err != nil {
		return nil, friendlyUniqueError("saved MRQL query", err)
	}

	ctx.Logger().Info(models.LogActionUpdate, "mrql_query", &saved.ID, saved.Name, "Updated saved MRQL query", nil)
	ctx.InvalidateSearchCacheByType(EntityTypeMRQLQuery)

	return &saved, nil
}

// DeleteSavedMRQLQuery deletes a saved MRQL query by ID.
func (ctx *MahresourcesContext) DeleteSavedMRQLQuery(id uint) error {
	var saved models.SavedMRQLQuery
	if err := ctx.db.First(&saved, id).Error; err != nil {
		return err
	}

	savedName := saved.Name
	err := ctx.db.Select(clause.Associations).Delete(&saved).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "mrql_query", &id, savedName, "Deleted saved MRQL query", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeMRQLQuery)
	}
	return err
}

// ExecuteSingleEntityWithScope executes a single-entity MRQL query with an
// optional scope filter applied via the translator's recursive CTE mechanism.
// When scopeID is 0, no scope filter is applied (equivalent to global scope).
func (ctx *MahresourcesContext) ExecuteSingleEntityWithScope(reqCtx context.Context, q *mrql.Query, entityType mrql.EntityType, opts mrql.TranslateOptions, scopeID uint) (*MRQLResult, error) {
	clone := *q
	q = &clone
	q.EntityType = entityType
	if q.Limit < 0 {
		q.Limit = min(ctx.defaultMRQLLimit(), interactiveMRQLPolicy.maxLimit)
	}
	if err := validateMRQLExecutionBounds(q, interactiveMRQLPolicy); err != nil {
		return nil, err
	}

	queryCtx, cancel := context.WithTimeout(reqCtx, ctx.mrqlQueryTimeout())
	defer cancel()

	// Pass scope through TranslateOptions so the translator applies the CTE
	if scopeID > 0 {
		opts.ScopeGroupID = scopeID
	}

	db, err := mrql.TranslateWithOptions(q, ctx.db.WithContext(queryCtx), opts)
	if err != nil {
		return nil, err
	}

	if q.Limit < 0 {
		db = db.Limit(ctx.defaultMRQLLimit())
	}

	result := &MRQLResult{EntityType: entityType.String()}

	switch entityType {
	case mrql.EntityResource:
		var resources []models.Resource
		if err := ctx.executeMRQLFind(db, &resources, q, "scoped resource select"); err != nil {
			return nil, err
		}
		result.Resources = resources
	case mrql.EntityNote:
		var notes []models.Note
		if err := ctx.executeMRQLFind(db, &notes, q, "scoped note select"); err != nil {
			return nil, err
		}
		result.Notes = notes
	case mrql.EntityGroup:
		var groups []models.Group
		if err := ctx.executeMRQLFind(db, &groups, q, "scoped group select"); err != nil {
			return nil, err
		}
		result.Groups = groups
	}

	return result, nil
}

// ExecuteMRQLGroupedWithScope executes a GROUP BY MRQL query with an optional
// owner_id scope filter applied at the GORM level before aggregation/bucketing.
// When scopeID is 0, delegates to the unscoped ExecuteMRQLGrouped.
func (ctx *MahresourcesContext) ExecuteMRQLGroupedWithScope(reqCtx context.Context, parsed *mrql.Query, scopeID uint) (*MRQLGroupedResult, error) {
	var err error
	scopeID, err = ctx.effectiveMRQLRequestedScope(scopeID)
	if err != nil {
		return nil, err
	}
	if scopeID == 0 {
		return ctx.ExecuteMRQLGrouped(reqCtx, parsed)
	}
	clone := *parsed
	parsed = &clone

	queryCtx, cancel := context.WithTimeout(reqCtx, ctx.mrqlQueryTimeout())
	defer cancel()

	defaultApplied := parsed.Limit < 0
	if defaultApplied {
		parsed.Limit = min(ctx.defaultMRQLLimit(), interactiveMRQLPolicy.maxLimit)
	}
	if err := validateMRQLExecutionBounds(parsed, interactiveMRQLPolicy); err != nil {
		return nil, err
	}

	var result *MRQLGroupedResult
	if len(parsed.GroupBy.Aggregates) > 0 {
		result, err = ctx.executeAggregatedQueryScoped(queryCtx, parsed, scopeID)
	} else {
		if parsed.Limit > maxBucketedTotalItems {
			parsed.Limit = maxBucketedTotalItems
		}
		result, err = ctx.executeBucketedQueryScoped(queryCtx, parsed, scopeID)
	}
	if err != nil {
		return nil, err
	}
	result.DefaultLimitApplied = defaultApplied
	result.AppliedLimit = parsed.Limit
	return result, nil
}

// executeAggregatedQueryScoped is like executeAggregatedQuery but applies
// scope filtering via the translator's recursive CTE mechanism.
func (ctx *MahresourcesContext) executeAggregatedQueryScoped(reqCtx context.Context, parsed *mrql.Query, scopeID uint) (*MRQLGroupedResult, error) {
	db := ctx.db.WithContext(reqCtx)
	opts := ctx.mrqlTranslateOptions()
	opts.ScopeGroupID = scopeID
	built, err := mrql.BuildAggregatedGroupBy(parsed, db, opts)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	if err := ctx.executeMRQLFind(built, &rows, parsed, "scoped aggregated group select"); err != nil {
		return nil, err
	}

	if rows == nil {
		rows = []map[string]any{}
	}

	return &MRQLGroupedResult{
		EntityType: parsed.EntityType.String(),
		Mode:       "aggregated",
		Rows:       rows,
	}, nil
}

// executeBucketedQueryScoped is like executeBucketedQuery but applies
// scope filtering via the translator's recursive CTE mechanism for both
// key discovery and bucket materialization.
func (ctx *MahresourcesContext) executeBucketedQueryScoped(reqCtx context.Context, parsed *mrql.Query, scopeID uint) (*MRQLGroupedResult, error) {
	scopeOpts := ctx.mrqlTranslateOptions()
	scopeOpts.ScopeGroupID = scopeID
	db := ctx.db.WithContext(reqCtx)

	keysDB, err := mrql.BuildGroupByKeys(parsed, db, scopeOpts)
	if err != nil {
		return nil, err
	}
	var allKeys []map[string]any
	if err := ctx.executeMRQLFind(keysDB, &allKeys, parsed, "scoped bucket key discovery"); err != nil {
		return nil, err
	}

	var warnings []string

	isPaginated := parsed.BucketLimit >= 0 || parsed.Offset >= 0
	if len(allKeys) > mrql.MaxBuckets {
		allKeys = allKeys[:mrql.MaxBuckets]
		if !isPaginated {
			warnings = append(warnings, fmt.Sprintf("Only the first %d groups are shown. Add filters to narrow the result set.", mrql.MaxBuckets))
		}
	}

	keys := allKeys
	if parsed.Offset > 0 {
		if parsed.Offset >= len(keys) {
			keys = nil
		} else {
			keys = keys[parsed.Offset:]
		}
	}
	pageSize := len(keys)
	if parsed.BucketLimit >= 0 && parsed.BucketLimit < pageSize {
		pageSize = parsed.BucketLimit
	}
	keys = keys[:pageSize]
	requestedKeys := len(keys)
	if len(keys) > maxBucketQueries {
		keys = keys[:maxBucketQueries]
		warnings = append(warnings, fmt.Sprintf("This page is limited to %d bucket queries; continue at the next offset for remaining groups.", maxBucketQueries))
	}

	var buckets []MRQLBucket
	totalItems := 0
	totalKeys := requestedKeys
	capOverflow := false
	for _, key := range keys {
		if totalItems >= maxBucketedTotalItems {
			break
		}

		bucketDB, err := mrql.TranslateGroupByBucket(parsed, ctx.db.WithContext(reqCtx), key, scopeOpts)
		if err != nil {
			return nil, err
		}

		publicKey := make(map[string]any, len(key))
		for k, v := range key {
			if strings.HasPrefix(k, "_gbid_") {
				friendlyKey := strings.TrimPrefix(k, "_gbid_") + "_id"
				publicKey[friendlyKey] = v
			} else {
				publicKey[k] = v
			}
		}
		bucket := MRQLBucket{Key: publicKey}
		remaining := maxBucketedTotalItems - totalItems
		probeLimit := remaining + 1
		if parsed.Limit >= 0 && parsed.Limit < probeLimit {
			probeLimit = parsed.Limit
		}
		bucketDB = bucketDB.Limit(probeLimit)
		bucketItems := 0

		switch parsed.EntityType {
		case mrql.EntityResource:
			var resources []models.Resource
			if err := ctx.executeMRQLFind(bucketDB, &resources, parsed, "scoped bucket resource select"); err != nil {
				return nil, err
			}
			bucket.Items, bucketItems = resources, len(resources)
		case mrql.EntityNote:
			var notes []models.Note
			if err := ctx.executeMRQLFind(bucketDB, &notes, parsed, "scoped bucket note select"); err != nil {
				return nil, err
			}
			bucket.Items, bucketItems = notes, len(notes)
		case mrql.EntityGroup:
			var groups []models.Group
			if err := ctx.executeMRQLFind(bucketDB, &groups, parsed, "scoped bucket group select"); err != nil {
				return nil, err
			}
			bucket.Items, bucketItems = groups, len(groups)
		}
		if bucketItems > remaining {
			capOverflow = true
			break
		}
		totalItems += bucketItems
		buckets = append(buckets, bucket)
	}

	if buckets == nil {
		buckets = []MRQLBucket{}
	}

	if (capOverflow || totalItems >= maxBucketedTotalItems) && len(buckets) < totalKeys {
		droppedGroups := totalKeys - len(buckets)
		warnings = append(warnings, fmt.Sprintf(
			"Results truncated at %d items (%d of %d groups shown, %d groups omitted). Narrow your query or add a filter.",
			maxBucketedTotalItems, len(buckets), totalKeys, droppedGroups))
	}

	offset := 0
	if parsed.Offset > 0 {
		offset = parsed.Offset
	}
	actualNextOffset := offset + len(buckets)
	var nextOffset *int
	if actualNextOffset < len(allKeys) {
		nextOffset = &actualNextOffset
	}

	return &MRQLGroupedResult{
		EntityType:  parsed.EntityType.String(),
		Mode:        "bucketed",
		Groups:      buckets,
		Warnings:    warnings,
		NextOffset:  nextOffset,
		TotalGroups: len(allKeys),
	}, nil
}

// effectiveMRQLRequestedScope intersects a shortcode/deferred requested scope
// with the principal's forced subtree. Out-of-subtree requests become the
// unresolved sentinel so nested MRQL fails closed without exposing sibling data.
func (ctx *MahresourcesContext) effectiveMRQLRequestedScope(requested uint) (uint, error) {
	forcedRoot, forced, deny := ctx.principalForcedScope()
	if deny {
		return mrql.UnresolvedScopeSentinel, nil
	}
	if !forced {
		return requested, nil
	}
	if requested == 0 {
		return forcedRoot, nil
	}
	if requested == mrql.UnresolvedScopeSentinel {
		return requested, nil
	}
	allowed, err := mrql.ScopeContains(ctx.db, forcedRoot, requested)
	if err != nil {
		return 0, err
	}
	if !allowed {
		return mrql.UnresolvedScopeSentinel, nil
	}
	return requested, nil
}

// ResolveMRQLScope resolves a parsed query's SCOPE clause to a group ID without
// allowing scoped principals to probe names or IDs outside their subtree.
func (ctx *MahresourcesContext) ResolveMRQLScope(q *mrql.Query) (uint, error) {
	forcedRoot, forced, deny := ctx.principalForcedScope()
	if deny {
		return mrql.UnresolvedScopeSentinel, nil
	}
	if forced {
		return mrql.ResolveScopeWithin(q, ctx.db, forcedRoot)
	}
	return mrql.ResolveScope(q, ctx.db)
}

// ExecuteMRQLScoped executes a pre-parsed MRQL query with scope filtering.
// Supports cross-entity queries.
func (ctx *MahresourcesContext) ExecuteMRQLScoped(reqCtx context.Context, parsed *mrql.Query, scopeGroupID uint) (*MRQLResult, error) {
	effectiveScope, err := ctx.effectiveMRQLRequestedScope(scopeGroupID)
	if err != nil {
		return nil, err
	}
	entityType := mrql.ExtractEntityType(parsed)
	opts := ctx.mrqlTranslateOptions()
	scopeGroupID = effectiveScope
	opts.ScopeGroupID = scopeGroupID
	if entityType != mrql.EntityUnspecified {
		return ctx.executeSingleEntity(reqCtx, parsed, entityType, opts, interactiveMRQLPolicy)
	}
	return ctx.executeCrossEntity(reqCtx, parsed, opts, interactiveMRQLPolicy)
}

// CountMRQLScoped returns the true number of rows a non-grouped MRQL query
// matches, ignoring LIMIT/OFFSET/ORDER BY. It reuses the same WHERE and scope
// translation as ExecuteMRQLScoped (via countCrossEntity), so the count can
// never diverge from the result set the shortcode renders. Cross-entity queries
// sum the per-entity counts. GROUP BY queries are not counted here — the
// [mrql] handler only requests a total for the flat path.
func (ctx *MahresourcesContext) CountMRQLScoped(reqCtx context.Context, parsed *mrql.Query, scopeGroupID uint) (int64, error) {
	effectiveScope, err := ctx.effectiveMRQLRequestedScope(scopeGroupID)
	if err != nil {
		return 0, err
	}
	opts := ctx.mrqlTranslateOptions()
	scopeGroupID = effectiveScope
	opts.ScopeGroupID = scopeGroupID
	entityType := mrql.ExtractEntityType(parsed)
	if entityType != mrql.EntityUnspecified {
		return ctx.countCrossEntity(reqCtx, parsed, opts, entityType)
	}
	var total int64
	for _, et := range []mrql.EntityType{mrql.EntityResource, mrql.EntityNote, mrql.EntityGroup} {
		n, err := ctx.countCrossEntity(reqCtx, parsed, opts, et)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// ResolveParentScopeID returns the owner_id of the group with the given ID.
// Returns mrql.UnresolvedScopeSentinel if the group doesn't exist or has no owner.
func (ctx *MahresourcesContext) ResolveParentScopeID(groupID uint) uint {
	if groupID == 0 {
		return mrql.UnresolvedScopeSentinel
	}
	var ownerID *uint
	err := ctx.db.Table("groups").Select("owner_id").Where("id = ?", groupID).Scan(&ownerID).Error
	if err != nil || ownerID == nil || *ownerID == 0 {
		return mrql.UnresolvedScopeSentinel
	}
	return *ownerID
}

// ResolveRootScopeID walks the ownership chain from the given group ID to find
// the root group. Returns mrql.UnresolvedScopeSentinel if the group doesn't exist.
func (ctx *MahresourcesContext) ResolveRootScopeID(groupID uint) uint {
	if groupID == 0 {
		return mrql.UnresolvedScopeSentinel
	}
	current := groupID
	for i := 0; i < 50; i++ {
		var ownerID *uint
		err := ctx.db.Table("groups").Select("owner_id").Where("id = ?", current).Scan(&ownerID).Error
		if err != nil || ownerID == nil || *ownerID == 0 {
			return current
		}
		current = *ownerID
	}
	return current
}
