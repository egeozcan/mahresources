package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/archive"
	"mahresources/models"
)

// ParseImport reads the tar at tarPath, walks its entries to collect groups,
// notes, resources, series, and schema defs, then resolves name-based mappings
// against the local database. The resulting ImportPlan is persisted as JSON to
// _imports/<jobID>.plan.json and returned.
func (ctx *MahresourcesContext) ParseImport(cancelCtx context.Context, jobID, tarPath string) (*ImportPlan, error) {
	f, err := ctx.fs.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("open import tar: %w", err)
	}
	defer f.Close()

	r, err := archive.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("new archive reader: %w", err)
	}
	defer r.Close()

	manifest, err := r.ReadManifest()
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	collector := &importDataCollector{
		groups:    make(map[string]*archive.GroupPayload),
		notes:     make(map[string]*archive.NotePayload),
		resources: make(map[string]*archive.ResourcePayload),
		series:    make(map[string]*archive.SeriesPayload),
	}

	if err := cancelCtx.Err(); err != nil {
		return nil, err
	}
	if err := r.Walk(collector); err != nil {
		return nil, fmt.Errorf("walk archive: %w", err)
	}

	if err := cancelCtx.Err(); err != nil {
		return nil, err
	}
	plan := &ImportPlan{
		JobID:            jobID,
		SchemaVersion:    manifest.SchemaVersion,
		SourceInstanceID: manifest.SourceInstanceID,
		Counts: ImportPlanCounts{
			Groups:    len(collector.groups),
			Notes:     len(collector.notes),
			Resources: len(collector.resources),
			Series:    len(collector.series),
			Blobs:     manifest.Counts.Blobs,
			Previews:  manifest.Counts.Previews,
			Versions:  manifest.Counts.Versions,
		},
		Warnings: manifest.Warnings,
	}

	// Resolve mappings
	categoryMappings := ctx.resolveCategories(collector.categoryDefs)
	// If the export shipped no schema defs for categories, discover names from payloads.
	if len(collector.categoryDefs) == 0 {
		for _, name := range discoverCategoryNamesFromPayloads(collector) {
			categoryMappings = append(categoryMappings, ctx.resolveCategoryByName(name, false))
		}
	}
	plan.Mappings.Categories = categoryMappings
	plan.Mappings.NoteTypes = ctx.resolveNoteTypes(collector.noteTypeDefs)
	plan.Mappings.ResourceCategories = ctx.resolveResourceCategories(collector.resourceCategoryDefs)
	plan.Mappings.Tags = ctx.resolveTags(collector.tagDefs, collector)
	plan.Mappings.GroupRelationTypes = ctx.resolveGRTDefs(collector.grtDefs)

	if err := cancelCtx.Err(); err != nil {
		return nil, err
	}

	// Resolve series via slug
	plan.SeriesInfo = ctx.resolveSeriesInfo(collector.series)

	// Resolve dangling references from the manifest
	plan.DanglingRefs = resolveDanglingRefs(manifest.Dangling)

	// Count hash conflicts
	plan.Conflicts.ResourceHashMatches = ctx.countHashConflicts(collector.resources)

	// Track manifest-only missing hashes
	plan.ManifestOnlyMissingHashes = countMissingHashes(collector.resources)

	// Build hierarchical item tree
	plan.Items = buildItemTree(collector)

	// Persist the plan
	if err := ctx.persistImportPlan(plan); err != nil {
		return nil, fmt.Errorf("persist import plan: %w", err)
	}

	return plan, nil
}

// LoadImportPlan reads a previously persisted plan from _imports/<jobID>.plan.json.
func (ctx *MahresourcesContext) LoadImportPlan(jobID string) (*ImportPlan, error) {
	planPath := importPlanPath(jobID)
	f, err := ctx.fs.Open(planPath)
	if err != nil {
		return nil, fmt.Errorf("open plan file: %w", err)
	}
	defer f.Close()

	var plan ImportPlan
	if err := json.NewDecoder(f).Decode(&plan); err != nil {
		return nil, fmt.Errorf("decode plan: %w", err)
	}
	return &plan, nil
}

// DeleteImportFiles removes the plan JSON and the tar file for a given job.
func (ctx *MahresourcesContext) DeleteImportFiles(jobID string) error {
	planPath := importPlanPath(jobID)
	_ = ctx.fs.Remove(planPath)

	// Try common tar paths
	tarPath := filepath.Join("_imports", jobID+".tar")
	_ = ctx.fs.Remove(tarPath)
	tarGzPath := filepath.Join("_imports", jobID+".tar.gz")
	_ = ctx.fs.Remove(tarGzPath)

	return nil
}

// persistImportPlan writes the plan as JSON to _imports/<jobID>.plan.json.
func (ctx *MahresourcesContext) persistImportPlan(plan *ImportPlan) error {
	planPath := importPlanPath(plan.JobID)
	dir := filepath.Dir(planPath)
	if err := ctx.fs.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create imports dir: %w", err)
	}

	data, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}

	return afero.WriteFile(ctx.fs, planPath, data, 0644)
}

func importPlanPath(jobID string) string {
	return filepath.Join("_imports", jobID+".plan.json")
}

// --- importDataCollector ---

// importDataCollector implements the visitor interfaces needed to collect
// all entity payloads and schema defs from an archive walk.
type importDataCollector struct {
	groups               map[string]*archive.GroupPayload
	notes                map[string]*archive.NotePayload
	resources            map[string]*archive.ResourcePayload
	series               map[string]*archive.SeriesPayload
	categoryDefs         []archive.CategoryDef
	noteTypeDefs         []archive.NoteTypeDef
	resourceCategoryDefs []archive.ResourceCategoryDef
	tagDefs              []archive.TagDef
	grtDefs              []archive.GroupRelationTypeDef
}

func (c *importDataCollector) OnGroup(p *archive.GroupPayload) error {
	c.groups[p.ExportID] = p
	return nil
}

func (c *importDataCollector) OnNote(p *archive.NotePayload) error {
	c.notes[p.ExportID] = p
	return nil
}

func (c *importDataCollector) OnResource(p *archive.ResourcePayload) error {
	c.resources[p.ExportID] = p
	return nil
}

func (c *importDataCollector) OnSeries(p *archive.SeriesPayload) error {
	c.series[p.ExportID] = p
	return nil
}

func (c *importDataCollector) OnCategoryDefs(defs []archive.CategoryDef) error {
	c.categoryDefs = defs
	return nil
}

func (c *importDataCollector) OnNoteTypeDefs(defs []archive.NoteTypeDef) error {
	c.noteTypeDefs = defs
	return nil
}

func (c *importDataCollector) OnResourceCategoryDefs(defs []archive.ResourceCategoryDef) error {
	c.resourceCategoryDefs = defs
	return nil
}

func (c *importDataCollector) OnTagDefs(defs []archive.TagDef) error {
	c.tagDefs = defs
	return nil
}

func (c *importDataCollector) OnGroupRelationTypeDefs(defs []archive.GroupRelationTypeDef) error {
	c.grtDefs = defs
	return nil
}

// --- Name-based mapping resolvers ---

func (ctx *MahresourcesContext) resolveCategories(defs []archive.CategoryDef) []MappingEntry {
	entries := make([]MappingEntry, 0, len(defs))
	for _, def := range defs {
		entry := MappingEntry{
			SourceKey:      def.Name,
			SourceExportID: def.ExportID,
			HasPayload:     true,
		}
		entry.DecisionKey = DecisionKeyFor("category", entry)

		var cat models.Category
		result := ctx.db.Where("name = ?", def.Name).First(&cat)
		if result.Error == nil {
			entry.Suggestion = "map"
			id := cat.ID
			entry.DestinationID = &id
			entry.DestinationName = cat.Name
		} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			entry.Suggestion = "create"
		} else {
			entry.Suggestion = "create"
		}

		entries = append(entries, entry)
	}
	return entries
}

// resolveCategoryByName creates a MappingEntry for a category name discovered
// from payloads (not from a schema def). HasPayload is set to the given value.
func (ctx *MahresourcesContext) resolveCategoryByName(name string, hasPayload bool) MappingEntry {
	entry := MappingEntry{
		SourceKey:  name,
		HasPayload: hasPayload,
	}
	entry.DecisionKey = DecisionKeyFor("category", entry)

	var cat models.Category
	result := ctx.db.Where("name = ?", name).First(&cat)
	if result.Error == nil {
		entry.Suggestion = "map"
		id := cat.ID
		entry.DestinationID = &id
		entry.DestinationName = cat.Name
	} else {
		entry.Suggestion = "create"
	}
	return entry
}

func (ctx *MahresourcesContext) resolveNoteTypes(defs []archive.NoteTypeDef) []MappingEntry {
	entries := make([]MappingEntry, 0, len(defs))
	for _, def := range defs {
		entry := MappingEntry{
			SourceKey:      def.Name,
			SourceExportID: def.ExportID,
			HasPayload:     true,
		}
		entry.DecisionKey = DecisionKeyFor("note_type", entry)

		var nts []models.NoteType
		ctx.db.Where("name = ?", def.Name).Find(&nts)
		switch len(nts) {
		case 0:
			entry.Suggestion = "create"
		case 1:
			entry.Suggestion = "map"
			id := nts[0].ID
			entry.DestinationID = &id
			entry.DestinationName = nts[0].Name
		default:
			entry.Ambiguous = true
			entry.Suggestion = ""
			for _, nt := range nts {
				entry.Alternatives = append(entry.Alternatives, MappingAlternative{ID: nt.ID, Name: nt.Name})
			}
		}

		entries = append(entries, entry)
	}
	return entries
}

func (ctx *MahresourcesContext) resolveResourceCategories(defs []archive.ResourceCategoryDef) []MappingEntry {
	entries := make([]MappingEntry, 0, len(defs))
	for _, def := range defs {
		entry := MappingEntry{
			SourceKey:      def.Name,
			SourceExportID: def.ExportID,
			HasPayload:     true,
		}
		entry.DecisionKey = DecisionKeyFor("resource_category", entry)

		var rc models.ResourceCategory
		result := ctx.db.Where("name = ?", def.Name).First(&rc)
		if result.Error == nil {
			entry.Suggestion = "map"
			id := rc.ID
			entry.DestinationID = &id
			entry.DestinationName = rc.Name
		} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			entry.Suggestion = "create"
		} else {
			entry.Suggestion = "create"
		}

		entries = append(entries, entry)
	}
	return entries
}

func (ctx *MahresourcesContext) resolveTags(defs []archive.TagDef, collector *importDataCollector) []MappingEntry {
	// Collect all tag names from defs
	seen := make(map[string]bool)
	entries := make([]MappingEntry, 0, len(defs))

	for _, def := range defs {
		seen[def.Name] = true
		entry := MappingEntry{
			SourceKey:      def.Name,
			SourceExportID: def.ExportID,
			HasPayload:     true,
		}
		entry.DecisionKey = DecisionKeyFor("tag", entry)

		var tag models.Tag
		result := ctx.db.Where("name = ?", def.Name).First(&tag)
		if result.Error == nil {
			entry.Suggestion = "map"
			id := tag.ID
			entry.DestinationID = &id
			entry.DestinationName = tag.Name
		} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			entry.Suggestion = "create"
		} else {
			entry.Suggestion = "create"
		}

		entries = append(entries, entry)
	}

	// Also discover tag references from payloads that may not have a def
	// (e.g. when the export did not include schema defs but payloads still
	// reference tags by name).
	for _, g := range collector.groups {
		for _, tr := range g.Tags {
			if tr.Name != "" && !seen[tr.Name] {
				seen[tr.Name] = true
				entries = append(entries, ctx.resolveTagByName(tr.Name))
			}
		}
	}
	for _, n := range collector.notes {
		for _, tr := range n.Tags {
			if tr.Name != "" && !seen[tr.Name] {
				seen[tr.Name] = true
				entries = append(entries, ctx.resolveTagByName(tr.Name))
			}
		}
	}
	for _, r := range collector.resources {
		for _, tr := range r.Tags {
			if tr.Name != "" && !seen[tr.Name] {
				seen[tr.Name] = true
				entries = append(entries, ctx.resolveTagByName(tr.Name))
			}
		}
	}

	return entries
}

func (ctx *MahresourcesContext) resolveTagByName(name string) MappingEntry {
	entry := MappingEntry{
		SourceKey:  name,
		HasPayload: false,
	}
	entry.DecisionKey = DecisionKeyFor("tag", entry)

	var tag models.Tag
	result := ctx.db.Where("name = ?", name).First(&tag)
	if result.Error == nil {
		entry.Suggestion = "map"
		id := tag.ID
		entry.DestinationID = &id
		entry.DestinationName = tag.Name
	} else {
		entry.Suggestion = "create"
	}

	return entry
}

func (ctx *MahresourcesContext) resolveGRTDefs(defs []archive.GroupRelationTypeDef) []MappingEntry {
	entries := make([]MappingEntry, 0, len(defs))
	for _, def := range defs {
		entry := MappingEntry{
			SourceKey:        def.Name,
			SourceExportID:   def.ExportID,
			HasPayload:       true,
			FromCategoryName: def.FromCategoryName,
			ToCategoryName:   def.ToCategoryName,
		}
		entry.DecisionKey = DecisionKeyFor("grt", entry)

		// GRT has a composite unique constraint: name + from_category_id + to_category_id.
		// We try to match by name AND category names.
		matched, matchedGRT := ctx.matchGRT(def.Name, def.FromCategoryName, def.ToCategoryName)
		if matched {
			entry.Suggestion = "map"
			id := matchedGRT.ID
			entry.DestinationID = &id
			entry.DestinationName = matchedGRT.Name
		} else {
			entry.Suggestion = "create"
		}

		entries = append(entries, entry)
	}
	return entries
}

// matchGRT tries to match a GRT by name and category names.
func (ctx *MahresourcesContext) matchGRT(name, fromCatName, toCatName string) (bool, *models.GroupRelationType) {
	query := ctx.db.Where("name = ?", name)

	if fromCatName != "" {
		var fromCat models.Category
		if err := ctx.db.Where("name = ?", fromCatName).First(&fromCat).Error; err != nil {
			// FromCategory not found -> can't match
			return false, nil
		}
		query = query.Where("from_category_id = ?", fromCat.ID)
	} else {
		query = query.Where("from_category_id IS NULL")
	}

	if toCatName != "" {
		var toCat models.Category
		if err := ctx.db.Where("name = ?", toCatName).First(&toCat).Error; err != nil {
			// ToCategory not found -> can't match
			return false, nil
		}
		query = query.Where("to_category_id = ?", toCat.ID)
	} else {
		query = query.Where("to_category_id IS NULL")
	}

	var grt models.GroupRelationType
	if err := query.First(&grt).Error; err != nil {
		return false, nil
	}
	return true, &grt
}

// --- Series resolution ---

func (ctx *MahresourcesContext) resolveSeriesInfo(seriesMap map[string]*archive.SeriesPayload) []SeriesMapping {
	mappings := make([]SeriesMapping, 0, len(seriesMap))
	for _, sp := range seriesMap {
		sm := SeriesMapping{
			ExportID: sp.ExportID,
			Name:     sp.Name,
			Slug:     sp.Slug,
		}

		existing, err := ctx.GetSeriesBySlug(sp.Slug)
		if err == nil && existing != nil {
			sm.Action = "reuse_existing"
			id := existing.ID
			sm.DestID = &id
			sm.DestName = existing.Name
		} else {
			sm.Action = "create"
		}

		mappings = append(mappings, sm)
	}
	return mappings
}

// --- Dangling references ---

func resolveDanglingRefs(manifestDangling []archive.DanglingRef) []DanglingRefPlan {
	refs := make([]DanglingRefPlan, 0, len(manifestDangling))
	for _, d := range manifestDangling {
		refs = append(refs, DanglingRefPlan{
			ID:               d.ID,
			Kind:             d.Kind,
			FromExportID:     d.From,
			FromName:         danglingFromName(d),
			StubSourceID:     d.ToStub.SourceID,
			StubName:         d.ToStub.Name,
			RelationTypeName: d.RelationTypeName,
		})
	}
	return refs
}

func danglingFromName(d archive.DanglingRef) string {
	// Best-effort: the from field is an export ID (e.g. "g0001").
	// We use it as the name since the actual group name isn't in DanglingRef.
	return d.From
}

// --- Hash conflicts ---

func (ctx *MahresourcesContext) countHashConflicts(resources map[string]*archive.ResourcePayload) int {
	count := 0
	for _, rp := range resources {
		if rp.Hash == "" {
			continue
		}
		var existing int64
		ctx.db.Model(&models.Resource{}).Where("hash = ?", rp.Hash).Count(&existing)
		if existing > 0 {
			count++
		}
	}
	return count
}

// countMissingHashes counts resources with BlobMissing flag set in the archive.
func countMissingHashes(resources map[string]*archive.ResourcePayload) int {
	count := 0
	for _, rp := range resources {
		if rp.BlobMissing {
			count++
		}
	}
	return count
}

// --- Item tree building ---

func buildItemTree(collector *importDataCollector) []ImportPlanItem {
	// Build nodes for all groups
	nodes := make(map[string]*ImportPlanItem, len(collector.groups))
	for _, g := range collector.groups {
		nodes[g.ExportID] = &ImportPlanItem{
			ExportID: g.ExportID,
			Kind:     "group",
			Name:     g.Name,
			OwnerRef: g.OwnerRef,
		}
	}

	// Count resources and notes per owning group
	for _, r := range collector.resources {
		if r.OwnerRef != "" {
			if node, ok := nodes[r.OwnerRef]; ok {
				node.ResourceCount++
			}
		}
	}
	for _, n := range collector.notes {
		if n.OwnerRef != "" {
			if node, ok := nodes[n.OwnerRef]; ok {
				node.NoteCount++
			}
		}
	}

	// Build child relationships
	childrenOf := make(map[string][]string) // parentExportID -> child export IDs
	roots := make([]string, 0)
	for id, node := range nodes {
		if node.OwnerRef == "" {
			roots = append(roots, id)
		} else {
			childrenOf[node.OwnerRef] = append(childrenOf[node.OwnerRef], id)
		}
	}

	// Recursively build the tree and roll up descendant counts
	items := make([]ImportPlanItem, 0, len(roots))
	for _, rootID := range roots {
		item := buildNode(rootID, nodes, childrenOf)
		items = append(items, item)
	}

	return items
}

// buildNode recursively builds a tree node and rolls up descendant counts.
func buildNode(exportID string, nodes map[string]*ImportPlanItem, childrenOf map[string][]string) ImportPlanItem {
	node := *nodes[exportID]

	childIDs := childrenOf[exportID]
	if len(childIDs) > 0 {
		node.Children = make([]ImportPlanItem, 0, len(childIDs))
		for _, childID := range childIDs {
			child := buildNode(childID, nodes, childrenOf)
			node.Children = append(node.Children, child)
		}
	}

	// Roll up: this node's own counts + all descendants
	node.DescendantResourceCount = node.ResourceCount
	node.DescendantNoteCount = node.NoteCount
	for _, child := range node.Children {
		node.DescendantResourceCount += child.DescendantResourceCount
		node.DescendantNoteCount += child.DescendantNoteCount
	}

	return node
}

// discoverCategoryNamesFromPayloads collects category names referenced in
// payloads when no schema def was shipped. Returns names not yet in defs.
func discoverCategoryNamesFromPayloads(collector *importDataCollector) []string {
	seen := make(map[string]bool)
	for _, def := range collector.categoryDefs {
		seen[def.Name] = true
	}

	var discovered []string
	for _, g := range collector.groups {
		name := strings.TrimSpace(g.CategoryName)
		if name != "" && !seen[name] {
			seen[name] = true
			discovered = append(discovered, name)
		}
	}
	return discovered
}
