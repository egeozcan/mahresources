package application_context

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	EntityType string        `json:"entityType"`
	Resources  []models.Resource `json:"resources,omitempty"`
	Notes      []models.Note     `json:"notes,omitempty"`
	Groups     []models.Group    `json:"groups,omitempty"`
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
			effectiveLimit = defaultMRQLLimit
		}
		parsed.Offset = (page - 1) * effectiveLimit
	}

	entityType := mrql.ExtractEntityType(parsed)

	opts := mrql.TranslateOptions{}

	if entityType != mrql.EntityUnspecified {
		return ctx.executeSingleEntity(reqCtx, parsed, entityType, opts)
	}

	// Cross-entity: fan out to all three entity types
	return ctx.executeCrossEntity(reqCtx, parsed, opts)
}

// defaultMRQLLimit is applied when the query has no explicit LIMIT clause.
const defaultMRQLLimit = 1000

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
		db = db.Limit(defaultMRQLLimit)
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

// crossEntityItem wraps any entity with its common sortable fields for global ordering.
type crossEntityItem struct {
	entityType string
	name       string
	created    time.Time
	updated    time.Time
	index      int // original index within its type slice
}

// executeCrossEntity runs the query against resources, notes, and groups
// separately, then globally sorts and paginates the merged result set.
func (ctx *MahresourcesContext) executeCrossEntity(reqCtx context.Context, parsed *mrql.Query, opts mrql.TranslateOptions) (*MRQLResult, error) {
	result := &MRQLResult{EntityType: "all"}

	queryCtx, cancel := context.WithTimeout(reqCtx, MRQLQueryTimeout)
	defer cancel()

	globalLimit := defaultMRQLLimit
	if parsed.Limit >= 0 {
		globalLimit = parsed.Limit
	}
	globalOffset := 0
	if parsed.Offset >= 0 {
		globalOffset = parsed.Offset
	}

	// Per-entity cap: fetch enough for offset+limit since we sort globally.
	perEntityCap := globalOffset + globalLimit

	var allResources []models.Resource
	var allNotes []models.Note
	var allGroups []models.Group

	entityTypes := []mrql.EntityType{mrql.EntityResource, mrql.EntityNote, mrql.EntityGroup}

	for _, et := range entityTypes {
		clone := *parsed
		clone.EntityType = et
		clone.Limit = perEntityCap
		clone.Offset = -1

		db, err := mrql.TranslateWithOptions(&clone, ctx.db.WithContext(queryCtx), opts)
		if err != nil {
			var translateErr *mrql.TranslateError
			if errors.As(err, &translateErr) {
				continue
			}
			return nil, err
		}

		switch et {
		case mrql.EntityResource:
			if err := db.Find(&allResources).Error; err != nil {
				return nil, fmt.Errorf("resource query failed: %w", err)
			}
		case mrql.EntityNote:
			if err := db.Find(&allNotes).Error; err != nil {
				return nil, fmt.Errorf("note query failed: %w", err)
			}
		case mrql.EntityGroup:
			if err := db.Find(&allGroups).Error; err != nil {
				return nil, fmt.Errorf("group query failed: %w", err)
			}
		}
	}

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
