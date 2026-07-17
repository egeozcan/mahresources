package application_context

import (
	"context"
	"sort"
	"sync"

	"mahresources/models"
	"mahresources/mrql"
)

// MRQLRenderScope contains the hierarchy values used by scoped shortcodes for a
// group. Unresolved values use mrql.UnresolvedScopeSentinel.
type MRQLRenderScope struct {
	ParentGroupID uint
	RootGroupID   uint
}

// MRQLRenderData is the batch-loaded scalar data needed to render MRQL cards.
// Carrier associations are intentionally not preloaded.
type MRQLRenderData struct {
	ResourceCategories map[uint]*models.ResourceCategory
	NoteTypes          map[uint]*models.NoteType
	Categories         map[uint]*models.Category
	Scopes             map[uint]MRQLRenderScope
}

type mrqlRenderDataCacheKey struct{}

type mrqlRenderDataCache struct {
	mu sync.Mutex

	resourceCategories map[uint]*models.ResourceCategory
	noteTypes          map[uint]*models.NoteType
	categories         map[uint]*models.Category
	scopes             map[uint]MRQLRenderScope

	loadedResourceCategories map[uint]bool
	loadedNoteTypes          map[uint]bool
	loadedCategories         map[uint]bool
	loadedScopes             map[uint]bool
}

func newMRQLRenderDataCache() *mrqlRenderDataCache {
	return &mrqlRenderDataCache{
		resourceCategories:       make(map[uint]*models.ResourceCategory),
		noteTypes:                make(map[uint]*models.NoteType),
		categories:               make(map[uint]*models.Category),
		scopes:                   make(map[uint]MRQLRenderScope),
		loadedResourceCategories: make(map[uint]bool),
		loadedNoteTypes:          make(map[uint]bool),
		loadedCategories:         make(map[uint]bool),
		loadedScopes:             make(map[uint]bool),
	}
}

// WithMRQLRenderDataCache attaches a request-local carrier/ancestry cache.
func WithMRQLRenderDataCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, mrqlRenderDataCacheKey{}, newMRQLRenderDataCache())
}

func renderDataCacheFromContext(ctx context.Context) *mrqlRenderDataCache {
	if ctx == nil {
		return nil
	}
	cache, _ := ctx.Value(mrqlRenderDataCacheKey{}).(*mrqlRenderDataCache)
	return cache
}

// LoadMRQLRenderData batch-loads distinct carrier and group IDs. The four ID
// slices may contain duplicates and zero values. When reqCtx carries a render
// cache, overlapping calls query only missing IDs.
func (ctx *MahresourcesContext) LoadMRQLRenderData(
	reqCtx context.Context,
	resourceCategoryIDs, noteTypeIDs, categoryIDs, scopeGroupIDs []uint,
) (*MRQLRenderData, error) {
	if err := reqCtx.Err(); err != nil {
		return nil, err
	}

	cache := renderDataCacheFromContext(reqCtx)
	if cache == nil {
		cache = newMRQLRenderDataCache()
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()

	resourceCategoryIDs = uniqueNonZeroIDs(resourceCategoryIDs)
	noteTypeIDs = uniqueNonZeroIDs(noteTypeIDs)
	categoryIDs = uniqueNonZeroIDs(categoryIDs)
	scopeGroupIDs = uniqueNonZeroIDs(scopeGroupIDs)

	if err := ctx.loadMRQLResourceCategories(reqCtx, cache, resourceCategoryIDs); err != nil {
		return nil, err
	}
	if err := ctx.loadMRQLNoteTypes(reqCtx, cache, noteTypeIDs); err != nil {
		return nil, err
	}
	if err := ctx.loadMRQLCategories(reqCtx, cache, categoryIDs); err != nil {
		return nil, err
	}
	if err := ctx.loadMRQLScopes(reqCtx, cache, scopeGroupIDs); err != nil {
		return nil, err
	}

	data := &MRQLRenderData{
		ResourceCategories: make(map[uint]*models.ResourceCategory, len(resourceCategoryIDs)),
		NoteTypes:          make(map[uint]*models.NoteType, len(noteTypeIDs)),
		Categories:         make(map[uint]*models.Category, len(categoryIDs)),
		Scopes:             make(map[uint]MRQLRenderScope, len(scopeGroupIDs)),
	}
	for _, id := range resourceCategoryIDs {
		if carrier := cache.resourceCategories[id]; carrier != nil {
			data.ResourceCategories[id] = carrier
		}
	}
	for _, id := range noteTypeIDs {
		if carrier := cache.noteTypes[id]; carrier != nil {
			data.NoteTypes[id] = carrier
		}
	}
	for _, id := range categoryIDs {
		if carrier := cache.categories[id]; carrier != nil {
			data.Categories[id] = carrier
		}
	}
	for _, id := range scopeGroupIDs {
		data.Scopes[id] = cache.scopes[id]
	}
	return data, nil
}

func uniqueNonZeroIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func unloadedIDs(ids []uint, loaded map[uint]bool) []uint {
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if !loaded[id] {
			out = append(out, id)
		}
	}
	return out
}

func (ctx *MahresourcesContext) loadMRQLResourceCategories(reqCtx context.Context, cache *mrqlRenderDataCache, ids []uint) error {
	missing := unloadedIDs(ids, cache.loadedResourceCategories)
	if len(missing) == 0 {
		return nil
	}
	var rows []models.ResourceCategory
	if err := ctx.db.WithContext(reqCtx).Where("id IN ?", missing).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		cache.resourceCategories[rows[i].ID] = &rows[i]
	}
	for _, id := range missing {
		cache.loadedResourceCategories[id] = true
	}
	return nil
}

func (ctx *MahresourcesContext) loadMRQLNoteTypes(reqCtx context.Context, cache *mrqlRenderDataCache, ids []uint) error {
	missing := unloadedIDs(ids, cache.loadedNoteTypes)
	if len(missing) == 0 {
		return nil
	}
	var rows []models.NoteType
	if err := ctx.db.WithContext(reqCtx).Where("id IN ?", missing).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		cache.noteTypes[rows[i].ID] = &rows[i]
	}
	for _, id := range missing {
		cache.loadedNoteTypes[id] = true
	}
	return nil
}

func (ctx *MahresourcesContext) loadMRQLCategories(reqCtx context.Context, cache *mrqlRenderDataCache, ids []uint) error {
	missing := unloadedIDs(ids, cache.loadedCategories)
	if len(missing) == 0 {
		return nil
	}
	var rows []models.Category
	if err := ctx.db.WithContext(reqCtx).Where("id IN ?", missing).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		cache.categories[rows[i].ID] = &rows[i]
	}
	for _, id := range missing {
		cache.loadedCategories[id] = true
	}
	return nil
}

func (ctx *MahresourcesContext) loadMRQLScopes(reqCtx context.Context, cache *mrqlRenderDataCache, ids []uint) error {
	missing := unloadedIDs(ids, cache.loadedScopes)
	if len(missing) == 0 {
		return nil
	}

	type ancestryRow struct {
		StartID uint  `gorm:"column:start_id"`
		ID      uint  `gorm:"column:id"`
		OwnerID *uint `gorm:"column:owner_id"`
		Depth   int   `gorm:"column:depth"`
	}
	var rows []ancestryRow
	err := ctx.db.WithContext(reqCtx).Raw(`
		WITH RECURSIVE mrql_ancestry(start_id, id, owner_id, depth) AS (
			SELECT id, id, owner_id, 0 FROM groups WHERE id IN ?
			UNION ALL
			SELECT a.start_id, g.id, g.owner_id, a.depth + 1
			FROM mrql_ancestry a
			JOIN groups g ON g.id = a.owner_id
			WHERE a.depth < 50
		)
		SELECT start_id, id, owner_id, depth
		FROM mrql_ancestry
		ORDER BY start_id, depth
	`, missing).Scan(&rows).Error
	if err != nil {
		if reqErr := reqCtx.Err(); reqErr != nil {
			return reqErr
		}
		return err
	}

	sentinel := mrql.UnresolvedScopeSentinel
	for _, id := range missing {
		cache.scopes[id] = MRQLRenderScope{ParentGroupID: sentinel, RootGroupID: sentinel}
	}
	for _, row := range rows {
		scope := cache.scopes[row.StartID]
		if row.Depth == 0 && row.OwnerID != nil && *row.OwnerID > 0 {
			scope.ParentGroupID = *row.OwnerID
		}
		// Rows are depth ordered; the deepest existing ancestor is the root, or
		// the same bounded fallback returned by ResolveRootScopeID for a cycle.
		scope.RootGroupID = row.ID
		cache.scopes[row.StartID] = scope
	}
	for _, id := range missing {
		cache.loadedScopes[id] = true
	}
	return nil
}
