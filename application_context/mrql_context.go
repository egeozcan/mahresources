package application_context

import (
	"context"
	"errors"
	"fmt"
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
func (ctx *MahresourcesContext) ExecuteMRQL(queryStr string, limit, page int) (*MRQLResult, error) {
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
	if limit > 0 {
		parsed.Limit = limit
	}
	if page > 1 {
		effectiveLimit := parsed.Limit
		if effectiveLimit < 0 {
			effectiveLimit = 1000 // default cap
		}
		parsed.Offset = (page - 1) * effectiveLimit
	}

	entityType := mrql.ExtractEntityType(parsed)

	opts := mrql.TranslateOptions{}

	if entityType != mrql.EntityUnspecified {
		return ctx.executeSingleEntity(parsed, entityType, opts)
	}

	// Cross-entity: fan out to all three entity types
	return ctx.executeCrossEntity(parsed, opts)
}

// defaultMRQLLimit is applied when the query has no explicit LIMIT clause.
const defaultMRQLLimit = 1000

// executeSingleEntity runs the query against a single entity table.
func (ctx *MahresourcesContext) executeSingleEntity(parsed *mrql.Query, entityType mrql.EntityType, opts mrql.TranslateOptions) (*MRQLResult, error) {
	parsed.EntityType = entityType

	// Create a timeout context scoped to this execution so it gets cancelled
	// when execution completes, avoiding context leaks.
	queryCtx, cancel := context.WithTimeout(context.Background(), MRQLQueryTimeout)
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

// executeCrossEntity runs the query against resources, notes, and groups
// separately and merges the results. LIMIT/OFFSET apply to the merged
// result set, not per-entity, to avoid returning up to 3x the requested limit.
func (ctx *MahresourcesContext) executeCrossEntity(parsed *mrql.Query, opts mrql.TranslateOptions) (*MRQLResult, error) {
	result := &MRQLResult{EntityType: "all"}

	// Create a timeout context scoped to this execution so it gets cancelled
	// when execution completes, avoiding context leaks.
	queryCtx, cancel := context.WithTimeout(context.Background(), MRQLQueryTimeout)
	defer cancel()

	// Determine the global limit and offset for the merged result set.
	globalLimit := defaultMRQLLimit
	if parsed.Limit >= 0 {
		globalLimit = parsed.Limit
	}
	globalOffset := 0
	if parsed.Offset >= 0 {
		globalOffset = parsed.Offset
	}

	// Each entity needs enough rows to cover offset + limit in the merged set.
	// Without this, OFFSET 1500 LIMIT 50 with per-entity cap of 50 would
	// fetch far too few rows and return an empty or wrong window.
	perEntityCap := globalOffset + globalLimit
	if perEntityCap > defaultMRQLLimit {
		perEntityCap = defaultMRQLLimit // hard cap to prevent unbounded fetches
	}

	entityTypes := []mrql.EntityType{mrql.EntityResource, mrql.EntityNote, mrql.EntityGroup}

	for _, et := range entityTypes {
		// Clone the parsed query for each entity type, without LIMIT/OFFSET
		clone := *parsed
		clone.EntityType = et
		clone.Limit = perEntityCap // enough rows to cover offset+limit in merged set
		clone.Offset = -1          // offset applied to merged set below

		db, err := mrql.TranslateWithOptions(&clone, ctx.db.WithContext(queryCtx), opts)
		if err != nil {
			// Skip entities where the query fields don't apply
			var translateErr *mrql.TranslateError
			if errors.As(err, &translateErr) {
				continue
			}
			return nil, err
		}

		switch et {
		case mrql.EntityResource:
			var resources []models.Resource
			if err := db.Find(&resources).Error; err != nil {
				// Propagate real DB errors; only translate errors are skipped above
				return nil, fmt.Errorf("resource query failed: %w", err)
			}
			result.Resources = resources
		case mrql.EntityNote:
			var notes []models.Note
			if err := db.Find(&notes).Error; err != nil {
				return nil, fmt.Errorf("note query failed: %w", err)
			}
			result.Notes = notes
		case mrql.EntityGroup:
			var groups []models.Group
			if err := db.Find(&groups).Error; err != nil {
				return nil, fmt.Errorf("group query failed: %w", err)
			}
			result.Groups = groups
		}
	}

	// Apply global LIMIT/OFFSET to the merged result set.
	// We interleave results (resources first, then notes, then groups)
	// and trim to the requested window.
	totalCount := len(result.Resources) + len(result.Notes) + len(result.Groups)

	if globalOffset > 0 || totalCount > globalLimit {
		// Flatten into a counted sequence and apply offset+limit
		remaining := globalLimit
		skip := globalOffset

		// Trim resources
		if skip >= len(result.Resources) {
			skip -= len(result.Resources)
			result.Resources = nil
		} else {
			result.Resources = result.Resources[skip:]
			skip = 0
		}
		if len(result.Resources) > remaining {
			result.Resources = result.Resources[:remaining]
		}
		remaining -= len(result.Resources)

		// Trim notes
		if skip >= len(result.Notes) {
			skip -= len(result.Notes)
			result.Notes = nil
		} else {
			result.Notes = result.Notes[skip:]
			skip = 0
		}
		if remaining <= 0 {
			result.Notes = nil
		} else if len(result.Notes) > remaining {
			result.Notes = result.Notes[:remaining]
		}
		remaining -= len(result.Notes)

		// Trim groups
		if skip >= len(result.Groups) {
			result.Groups = nil
		} else {
			result.Groups = result.Groups[skip:]
		}
		if remaining <= 0 {
			result.Groups = nil
		} else if len(result.Groups) > remaining {
			result.Groups = result.Groups[:remaining]
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
func (ctx *MahresourcesContext) GetSavedMRQLQueries() ([]models.SavedMRQLQuery, error) {
	var queries []models.SavedMRQLQuery
	if err := ctx.db.Order("name ASC").Find(&queries).Error; err != nil {
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

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("saved MRQL query text must be non-empty")
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
