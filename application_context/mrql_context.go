package application_context

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/mrql"
)

// MRQLQueryTimeout is the maximum execution time for MRQL queries.
// It can be configured via the -mrql-query-timeout flag.
var MRQLQueryTimeout = 10 * time.Second

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
func (ctx *MahresourcesContext) ExecuteMRQL(reqCtx context.Context, queryStr string, limit, page int) (*MRQLResult, error) {
	queryStr = strings.TrimSpace(queryStr)
	if queryStr == "" {
		return nil, errors.New("query string must not be empty")
	}

	parsed, err := mrql.Parse(queryStr)
	if err != nil {
		return nil, err
	}

	if err := mrql.Validate(parsed); err != nil {
		return nil, err
	}

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
			effectiveLimit = ctx.defaultMRQLLimit()
		}
		parsed.Offset = (page - 1) * effectiveLimit
	}

	// BH-013: compute default-limit flag + applied limit before translation.
	// parsed.Limit < 0 means "no explicit LIMIT" after request-param overrides.
	defaultApplied := parsed.Limit < 0
	appliedLimit := parsed.Limit
	if defaultApplied {
		appliedLimit = ctx.defaultMRQLLimit()
	}

	entityType := mrql.ExtractEntityType(parsed)

	opts := mrql.TranslateOptions{}
	if parsed.Scope != nil {
		scopeID, err := mrql.ResolveScope(parsed, ctx.db)
		if err != nil {
			return nil, err
		}
		opts.ScopeGroupID = scopeID
	}

	var result *MRQLResult
	if entityType != mrql.EntityUnspecified {
		result, err = ctx.executeSingleEntity(reqCtx, parsed, entityType, opts)
	} else {
		// Cross-entity: fan out to all three entity types
		result, err = ctx.executeCrossEntity(reqCtx, parsed, opts)
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

// maxBucketedTotalItems caps the total number of entity items materialized
// across all buckets, preventing a single bucketed query from loading
// maxBuckets × defaultMRQLLimit entities into memory.
const maxBucketedTotalItems = 10000

// executeSingleEntity runs the query against a single entity table.
func (ctx *MahresourcesContext) executeSingleEntity(reqCtx context.Context, parsed *mrql.Query, entityType mrql.EntityType, opts mrql.TranslateOptions) (*MRQLResult, error) {
	parsed.EntityType = entityType

	// Derive timeout from the request context so client disconnects cancel the query.
	queryCtx, cancel := context.WithTimeout(reqCtx, MRQLQueryTimeout)
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
		if err := db.Find(&resources).Error; err != nil {
			return nil, err
		}
		result.Resources = resources
	case mrql.EntityNote:
		var notes []models.Note
		if err := db.Find(&notes).Error; err != nil {
			return nil, err
		}
		result.Notes = notes
	case mrql.EntityGroup:
		var groups []models.Group
		if err := db.Find(&groups).Error; err != nil {
			return nil, err
		}
		result.Groups = groups
	}

	return result, nil
}

// ExecuteMRQLGrouped executes a GROUP BY MRQL query and returns grouped results.
// The parsed query must have GroupBy set and EntityType populated.
func (ctx *MahresourcesContext) ExecuteMRQLGrouped(reqCtx context.Context, parsed *mrql.Query) (*MRQLGroupedResult, error) {
	queryCtx, cancel := context.WithTimeout(reqCtx, MRQLQueryTimeout)
	defer cancel()

	// BH-013: record whether the default kicked in before mutating parsed.Limit.
	defaultApplied := parsed.Limit < 0

	// Apply default limit when no explicit LIMIT was specified.
	if parsed.Limit < 0 {
		parsed.Limit = ctx.defaultMRQLLimit()
	}

	opts := mrql.TranslateOptions{}
	if parsed.Scope != nil {
		scopeID, err := mrql.ResolveScope(parsed, ctx.db)
		if err != nil {
			return nil, err
		}
		opts.ScopeGroupID = scopeID
	}

	var result *MRQLGroupedResult
	var err error
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
	gbResult, err := mrql.TranslateGroupBy(parsed, db, opts)
	if err != nil {
		return nil, err
	}

	// Ensure Rows is never nil for consistent JSON
	if gbResult.Rows == nil {
		gbResult.Rows = []map[string]any{}
	}

	return &MRQLGroupedResult{
		EntityType: parsed.EntityType.String(),
		Mode:       gbResult.Mode,
		Rows:       gbResult.Rows,
	}, nil
}

func (ctx *MahresourcesContext) executeBucketedQuery(reqCtx context.Context, parsed *mrql.Query, opts mrql.TranslateOptions) (*MRQLGroupedResult, error) {
	db := ctx.db.WithContext(reqCtx)

	allKeys, err := mrql.TranslateGroupByKeys(parsed, db, opts)
	if err != nil {
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

	var buckets []MRQLBucket
	totalItems := 0
	totalKeys := len(keys)
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

		switch parsed.EntityType {
		case mrql.EntityResource:
			var resources []models.Resource
			if err := bucketDB.Find(&resources).Error; err != nil {
				return nil, err
			}
			bucket.Items = resources
			totalItems += len(resources)
		case mrql.EntityNote:
			var notes []models.Note
			if err := bucketDB.Find(&notes).Error; err != nil {
				return nil, err
			}
			bucket.Items = notes
			totalItems += len(notes)
		case mrql.EntityGroup:
			var groups []models.Group
			if err := bucketDB.Find(&groups).Error; err != nil {
				return nil, err
			}
			bucket.Items = groups
			totalItems += len(groups)
		}

		buckets = append(buckets, bucket)
	}

	if buckets == nil {
		buckets = []MRQLBucket{}
	}

	if totalItems >= maxBucketedTotalItems && len(buckets) < totalKeys {
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
}

// executeCrossEntity runs the query against resources, notes, and groups
// concurrently, then globally sorts and paginates the merged result set.
// Each entity query gets its own timeout so a slow table doesn't block the
// others. If an entity times out, its results are omitted and a warning is
// included in the response.
func (ctx *MahresourcesContext) executeCrossEntity(reqCtx context.Context, parsed *mrql.Query, opts mrql.TranslateOptions) (*MRQLResult, error) {
	result := &MRQLResult{EntityType: "all"}

	globalLimit := ctx.defaultMRQLLimit()
	if parsed.Limit >= 0 {
		globalLimit = parsed.Limit
	}
	globalOffset := 0
	if parsed.Offset >= 0 {
		globalOffset = parsed.Offset
	}

	// Per-entity cap: fetch enough for offset+limit since we sort globally.
	perEntityCap := globalOffset + globalLimit

	var (
		allResources []models.Resource
		allNotes     []models.Note
		allGroups    []models.Group
		mu           sync.Mutex
		warnings     []string
	)

	var wg sync.WaitGroup
	entityTypes := []mrql.EntityType{mrql.EntityResource, mrql.EntityNote, mrql.EntityGroup}
	errs := make([]error, len(entityTypes))

	for i, et := range entityTypes {
		clone := *parsed
		clone.EntityType = et
		clone.Limit = perEntityCap
		clone.Offset = -1

		// Each entity gets its own timeout derived from the request context.
		entityCtx, cancel := context.WithTimeout(reqCtx, MRQLQueryTimeout)

		db, err := mrql.TranslateWithOptions(&clone, ctx.db.WithContext(entityCtx), opts)
		if err != nil {
			cancel()
			var translateErr *mrql.TranslateError
			if errors.As(err, &translateErr) {
				continue
			}
			return nil, err
		}

		wg.Add(1)
		go func(idx int, et mrql.EntityType, cancel context.CancelFunc) {
			defer wg.Done()
			defer cancel()

			switch et {
			case mrql.EntityResource:
				var resources []models.Resource
				if err := db.Find(&resources).Error; err != nil {
					errs[idx] = fmt.Errorf("resource query failed: %w", err)
					return
				}
				mu.Lock()
				allResources = resources
				mu.Unlock()
			case mrql.EntityNote:
				var notes []models.Note
				if err := db.Find(&notes).Error; err != nil {
					errs[idx] = fmt.Errorf("note query failed: %w", err)
					return
				}
				mu.Lock()
				allNotes = notes
				mu.Unlock()
			case mrql.EntityGroup:
				var groups []models.Group
				if err := db.Find(&groups).Error; err != nil {
					errs[idx] = fmt.Errorf("group query failed: %w", err)
					return
				}
				mu.Lock()
				allGroups = groups
				mu.Unlock()
			}
		}(i, et, cancel)
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

	// Build unified sortable items
	items := make([]crossEntityItem, 0, len(allResources)+len(allNotes)+len(allGroups))
	for i, r := range allResources {
		items = append(items, crossEntityItem{"resource", r.Name, r.CreatedAt, r.UpdatedAt, i})
	}
	for i, n := range allNotes {
		items = append(items, crossEntityItem{"note", n.Name, n.CreatedAt, n.UpdatedAt, i})
	}
	for i, g := range allGroups {
		items = append(items, crossEntityItem{"group", g.Name, g.CreatedAt, g.UpdatedAt, i})
	}

	// Global sort if ORDER BY is specified
	if len(parsed.OrderBy) > 0 {
		sort.SliceStable(items, func(i, j int) bool {
			for _, ob := range parsed.OrderBy {
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

// ValidateMRQL parses and validates an MRQL query string, returning whether it
// is valid and any errors with position information.
func (ctx *MahresourcesContext) ValidateMRQL(queryStr string) (bool, []map[string]any) {
	queryStr = strings.TrimSpace(queryStr)
	if queryStr == "" {
		return false, []map[string]any{
			{"message": "query string must not be empty", "pos": 0, "length": 0},
		}
	}

	parsed, err := mrql.Parse(queryStr)
	if err != nil {
		var parseErr *mrql.ParseError
		if errors.As(err, &parseErr) {
			return false, []map[string]any{
				{"message": parseErr.Message, "pos": parseErr.Pos, "length": parseErr.Length},
			}
		}
		return false, []map[string]any{
			{"message": err.Error(), "pos": 0, "length": 0},
		}
	}

	if err := mrql.Validate(parsed); err != nil {
		var validationErr *mrql.ValidationError
		if errors.As(err, &validationErr) {
			return false, []map[string]any{
				{"message": validationErr.Message, "pos": validationErr.Pos, "length": validationErr.Length},
			}
		}
		return false, []map[string]any{
			{"message": err.Error(), "pos": 0, "length": 0},
		}
	}

	return true, nil
}

// CompleteMRQL returns autocompletion suggestions for the given MRQL query
// string at the specified cursor position.
func (ctx *MahresourcesContext) CompleteMRQL(queryStr string, cursor int) []mrql.Suggestion {
	return mrql.Complete(queryStr, cursor)
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
	}
	return err
}

// ExecuteSingleEntityWithScope executes a single-entity MRQL query with an
// optional scope filter applied via the translator's recursive CTE mechanism.
// When scopeID is 0, no scope filter is applied (equivalent to global scope).
func (ctx *MahresourcesContext) ExecuteSingleEntityWithScope(reqCtx context.Context, q *mrql.Query, entityType mrql.EntityType, opts mrql.TranslateOptions, scopeID uint) (*MRQLResult, error) {
	q.EntityType = entityType

	queryCtx, cancel := context.WithTimeout(reqCtx, MRQLQueryTimeout)
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
		if err := db.Find(&resources).Error; err != nil {
			return nil, err
		}
		result.Resources = resources
	case mrql.EntityNote:
		var notes []models.Note
		if err := db.Find(&notes).Error; err != nil {
			return nil, err
		}
		result.Notes = notes
	case mrql.EntityGroup:
		var groups []models.Group
		if err := db.Find(&groups).Error; err != nil {
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
	if scopeID == 0 {
		return ctx.ExecuteMRQLGrouped(reqCtx, parsed)
	}

	queryCtx, cancel := context.WithTimeout(reqCtx, MRQLQueryTimeout)
	defer cancel()

	defaultApplied := parsed.Limit < 0
	if defaultApplied {
		parsed.Limit = ctx.defaultMRQLLimit()
	}

	var result *MRQLGroupedResult
	var err error
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
	gbResult, err := mrql.TranslateGroupBy(parsed, db, mrql.TranslateOptions{ScopeGroupID: scopeID})
	if err != nil {
		return nil, err
	}

	if gbResult.Rows == nil {
		gbResult.Rows = []map[string]any{}
	}

	return &MRQLGroupedResult{
		EntityType: parsed.EntityType.String(),
		Mode:       gbResult.Mode,
		Rows:       gbResult.Rows,
	}, nil
}

// executeBucketedQueryScoped is like executeBucketedQuery but applies
// scope filtering via the translator's recursive CTE mechanism for both
// key discovery and bucket materialization.
func (ctx *MahresourcesContext) executeBucketedQueryScoped(reqCtx context.Context, parsed *mrql.Query, scopeID uint) (*MRQLGroupedResult, error) {
	scopeOpts := mrql.TranslateOptions{ScopeGroupID: scopeID}
	db := ctx.db.WithContext(reqCtx)

	allKeys, err := mrql.TranslateGroupByKeys(parsed, db, scopeOpts)
	if err != nil {
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

	var buckets []MRQLBucket
	totalItems := 0
	totalKeys := len(keys)
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

		switch parsed.EntityType {
		case mrql.EntityResource:
			var resources []models.Resource
			if err := bucketDB.Find(&resources).Error; err != nil {
				return nil, err
			}
			bucket.Items = resources
			totalItems += len(resources)
		case mrql.EntityNote:
			var notes []models.Note
			if err := bucketDB.Find(&notes).Error; err != nil {
				return nil, err
			}
			bucket.Items = notes
			totalItems += len(notes)
		case mrql.EntityGroup:
			var groups []models.Group
			if err := bucketDB.Find(&groups).Error; err != nil {
				return nil, err
			}
			bucket.Items = groups
			totalItems += len(groups)
		}

		buckets = append(buckets, bucket)
	}

	if buckets == nil {
		buckets = []MRQLBucket{}
	}

	if totalItems >= maxBucketedTotalItems && len(buckets) < totalKeys {
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

// ResolveMRQLScope resolves a parsed query's SCOPE clause to a group ID.
func (ctx *MahresourcesContext) ResolveMRQLScope(q *mrql.Query) (uint, error) {
	return mrql.ResolveScope(q, ctx.db)
}

// ExecuteMRQLScoped executes a pre-parsed MRQL query with scope filtering.
// Supports cross-entity queries.
func (ctx *MahresourcesContext) ExecuteMRQLScoped(reqCtx context.Context, parsed *mrql.Query, scopeGroupID uint) (*MRQLResult, error) {
	entityType := mrql.ExtractEntityType(parsed)
	opts := mrql.TranslateOptions{ScopeGroupID: scopeGroupID}
	if entityType != mrql.EntityUnspecified {
		return ctx.executeSingleEntity(reqCtx, parsed, entityType, opts)
	}
	return ctx.executeCrossEntity(reqCtx, parsed, opts)
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
