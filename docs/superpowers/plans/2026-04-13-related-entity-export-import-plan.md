# Related Entity Export/Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add configurable `RelatedDepth` BFS traversal to group export/import so m2m-related entities are included in the archive rather than becoming dangling references.

**Architecture:** The export plan builder gains a BFS phase (between phases C and D) that follows enabled m2m edges from in-scope groups up to N hops deep. Discovered groups become "shell" entries (metadata only, no owned-entity collection). Discovered resources/notes get full payloads. Import gains per-shell-group decisions (`create` or `map_to_existing`) with conflict-ignore semantics for relation deduplication.

**Tech Stack:** Go, GORM, SQLite, Pongo2 templates, Alpine.js, Playwright (E2E)

**Spec:** `docs/superpowers/specs/2026-04-13-related-entity-export-import-design.md`

---

### Task 1: Manifest and ExportRequest Struct Additions

**Files:**
- Modify: `archive/manifest.go:22-35` (ExportOptions, ExportScope), `archive/manifest.go:50-58` (Counts), `archive/manifest.go:67-72` (GroupEntry), `archive/manifest.go:138-155` (GroupPayload)
- Modify: `application_context/export_context.go:20-26` (ExportRequest)

- [ ] **Step 1: Add `RelatedDepth` to `ExportOptions`**

In `archive/manifest.go`, add `RelatedDepth` to `ExportOptions`:

```go
type ExportOptions struct {
	Scope        ExportScope      `json:"scope"`
	Fidelity     ExportFidelity   `json:"fidelity"`
	SchemaDefs   ExportSchemaDefs `json:"schema_defs"`
	Gzip         bool             `json:"gzip"`
	RelatedDepth int              `json:"related_depth,omitempty"`
}
```

- [ ] **Step 2: Add `Shell` to `GroupEntry` and `GroupPayload`**

In `archive/manifest.go`, add `Shell bool` to `GroupEntry`:

```go
type GroupEntry struct {
	ExportID string `json:"export_id"`
	Name     string `json:"name"`
	SourceID uint   `json:"source_id"`
	Path     string `json:"path"`
	Shell    bool   `json:"shell,omitempty"`
}
```

Add `Shell bool` to `GroupPayload`:

```go
type GroupPayload struct {
	ExportID         string                 `json:"export_id"`
	SourceID         uint                   `json:"source_id"`
	Shell            bool                   `json:"shell,omitempty"`
	Name             string                 `json:"name"`
	// ... rest unchanged
```

- [ ] **Step 3: Add `ShellGroups` to `Counts`**

In `archive/manifest.go`:

```go
type Counts struct {
	Groups      int `json:"groups"`
	ShellGroups int `json:"shell_groups,omitempty"`
	Notes       int `json:"notes"`
	Resources   int `json:"resources"`
	Series      int `json:"series"`
	Blobs       int `json:"blobs"`
	Previews    int `json:"previews"`
	Versions    int `json:"versions"`
}
```

- [ ] **Step 4: Add `RelatedDepth` to `ExportRequest`**

In `application_context/export_context.go`:

```go
type ExportRequest struct {
	RootGroupIDs []uint                   `json:"rootGroupIds"`
	Scope        archive.ExportScope      `json:"scope"`
	Fidelity     archive.ExportFidelity   `json:"fidelity"`
	SchemaDefs   archive.ExportSchemaDefs `json:"schemaDefs"`
	Gzip         bool                     `json:"gzip"`
	RelatedDepth int                      `json:"relatedDepth,omitempty"`
}
```

- [ ] **Step 5: Run existing tests to verify no regressions**

Run: `go test --tags 'json1 fts5' ./archive/... ./application_context/...`
Expected: All existing tests pass (additive fields only, zero-value defaults preserve behavior).

- [ ] **Step 6: Commit**

```bash
git add archive/manifest.go application_context/export_context.go
git commit -m "feat: add RelatedDepth, Shell, ShellGroups to manifest and ExportRequest"
```

---

### Task 2: Export Plan BFS Phase — Scaffold and First Test

**Files:**
- Modify: `application_context/export_context.go:86-227` (exportPlan struct, buildExportPlan)
- Modify: `application_context/export_context_test.go` (new test helpers, first BFS test)

- [ ] **Step 1: Add test helpers for m2m resource/note linking**

In `application_context/export_context_test.go`, add after `mustLinkRelatedGroup`:

```go
func mustLinkRelatedResource(t *testing.T, ctx *MahresourcesContext, groupID, resourceID uint) {
	t.Helper()
	var g models.Group
	if err := ctx.db.First(&g, groupID).Error; err != nil {
		t.Fatalf("load group: %v", err)
	}
	var r models.Resource
	if err := ctx.db.First(&r, resourceID).Error; err != nil {
		t.Fatalf("load resource: %v", err)
	}
	if err := ctx.db.Model(&g).Association("RelatedResources").Append(&r); err != nil {
		t.Fatalf("append related resource: %v", err)
	}
	t.Cleanup(func() {
		_ = ctx.db.Model(&g).Association("RelatedResources").Delete(&r)
	})
}

func mustLinkRelatedNote(t *testing.T, ctx *MahresourcesContext, groupID, noteID uint) {
	t.Helper()
	var g models.Group
	if err := ctx.db.First(&g, groupID).Error; err != nil {
		t.Fatalf("load group: %v", err)
	}
	var n models.Note
	if err := ctx.db.First(&n, noteID).Error; err != nil {
		t.Fatalf("load note: %v", err)
	}
	if err := ctx.db.Model(&g).Association("RelatedNotes").Append(&n); err != nil {
		t.Fatalf("append related note: %v", err)
	}
	t.Cleanup(func() {
		_ = ctx.db.Model(&g).Association("RelatedNotes").Delete(&n)
	})
}
```

- [ ] **Step 2: Write failing test — depth 1 basic**

In `application_context/export_context_test.go`:

```go
func TestStreamExport_RelatedDepth1_IncludesRelatedEntities(t *testing.T) {
	ctx := createTestContext(t)

	// GroupA (root) owns nothing, but has m2m relationships:
	//   RelatedResource -> res (owned by GroupB)
	//   RelatedNote -> note (owned by GroupB)
	//   RelatedGroup -> GroupC (unrelated to root)
	groupA := mustCreateGroup(t, ctx, "GroupA", nil)
	groupB := mustCreateGroup(t, ctx, "GroupB", nil)
	groupC := mustCreateGroup(t, ctx, "GroupC", nil)

	res := mustCreateResource(t, ctx, "related.txt", &groupB.ID, []byte("RELDATA"))
	note := mustCreateNote(t, ctx, "Related Note", &groupB.ID)

	mustLinkRelatedResource(t, ctx, groupA.ID, res.ID)
	mustLinkRelatedNote(t, ctx, groupA.ID, note.ID)
	mustLinkRelatedGroup(t, ctx, groupA.ID, groupC.ID)

	var buf bytes.Buffer
	err := ctx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{OwnedResources: true, OwnedNotes: true, RelatedM2M: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true, Tags: true},
		RelatedDepth: 1,
	}, &buf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	coll := newExportCollector()
	manifest, err := archive.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), coll)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}

	// GroupA is in scope (root), GroupB and GroupC should be shell groups.
	if len(coll.groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(coll.groups))
	}

	// Verify shell flags
	for _, gp := range coll.groups {
		if gp.Name == "GroupA" {
			if gp.Shell {
				t.Errorf("root group GroupA should not be shell")
			}
		} else {
			if !gp.Shell {
				t.Errorf("group %q should be shell", gp.Name)
			}
		}
	}

	// Resource and note should be included
	if len(coll.resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(coll.resources))
	}
	if len(coll.notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(coll.notes))
	}

	// Resource blob should be present
	if len(coll.blobs) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(coll.blobs))
	}

	// Manifest should show shell group count
	if manifest.Counts.ShellGroups != 2 {
		t.Errorf("expected ShellGroups=2, got %d", manifest.Counts.ShellGroups)
	}

	// RelatedDepth should be in ExportOptions
	if manifest.ExportOptions.RelatedDepth != 1 {
		t.Errorf("expected RelatedDepth=1, got %d", manifest.ExportOptions.RelatedDepth)
	}

	// No dangling refs — everything is in scope now
	if len(manifest.Dangling) != 0 {
		t.Errorf("expected 0 dangling refs, got %d", len(manifest.Dangling))
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestStreamExport_RelatedDepth1 -v`
Expected: FAIL — no BFS phase exists yet, so related entities won't be included.

- [ ] **Step 4: Add `shellGroupIDs` to `exportPlan` struct**

In `application_context/export_context.go`, add to the `exportPlan` struct (after `warnings`):

```go
	// shellGroupIDs tracks groups discovered via BFS (related depth traversal)
	// rather than ownership. Shell groups get Shell: true in the manifest and
	// their owned entities are NOT collected.
	shellGroupIDs map[uint]bool
```

- [ ] **Step 5: Implement BFS phase in `buildExportPlan`**

In `application_context/export_context.go`, replace the section from `// Phase D:` through `return plan, nil` (lines 209-226) with:

```go
	// Phase BFS: follow m2m edges to related entities up to RelatedDepth hops.
	plan.shellGroupIDs = map[uint]bool{}
	if req.RelatedDepth > 0 {
		if err := ctx.bfsRelatedEntities(plan); err != nil {
			return nil, fmt.Errorf("bfs related entities: %w", err)
		}
	}

	// Phase D: collect series referenced by in-scope resources (both owned and BFS-discovered).
	if req.Fidelity.ResourceSeries && len(plan.resourceIDs) > 0 {
		seriesIDs, err := ctx.findSeriesForResources(plan.resourceIDs)
		if err != nil {
			return nil, err
		}
		for _, sid := range seriesIDs {
			if _, exists := plan.seriesExportID[sid]; exists {
				continue // already collected
			}
			plan.seriesIDs = append(plan.seriesIDs, sid)
			plan.seriesExportID[sid] = fmt.Sprintf("s%04d", len(plan.seriesExportID)+1)
		}
	}

	// Phase E: detect dangling references (m2m / GroupRelations / Series siblings).
	if err := ctx.collectDanglingRefs(plan); err != nil {
		return nil, err
	}

	return plan, nil
```

- [ ] **Step 6: Implement `bfsRelatedEntities`**

In `application_context/export_context.go`, add after `buildExportPlan`:

```go
// bfsRelatedEntities runs a BFS from in-scope groups, following enabled m2m
// edges up to plan.req.RelatedDepth hops. Discovered groups become shells,
// discovered resources/notes get full payloads. Only groups spawn the next hop.
func (ctx *MahresourcesContext) bfsRelatedEntities(plan *exportPlan) error {
	req := plan.req
	frontier := make([]uint, len(plan.groupIDs))
	copy(frontier, plan.groupIDs)

	for level := 1; level <= req.RelatedDepth; level++ {
		if len(frontier) == 0 {
			break
		}

		var newGroupIDs []uint

		if req.Scope.RelatedM2M {
			ids, err := ctx.bfsCollectM2M(plan, frontier)
			if err != nil {
				return fmt.Errorf("bfs m2m level %d: %w", level, err)
			}
			newGroupIDs = append(newGroupIDs, ids...)
		}

		if req.Scope.GroupRelations {
			ids, err := ctx.bfsCollectGroupRelations(plan, frontier)
			if err != nil {
				return fmt.Errorf("bfs group-relations level %d: %w", level, err)
			}
			newGroupIDs = append(newGroupIDs, ids...)
		}

		// For newly discovered resources/notes, ensure their owner is in scope.
		if err := ctx.bfsEnsureOwners(plan); err != nil {
			return fmt.Errorf("bfs ensure owners level %d: %w", level, err)
		}

		frontier = newGroupIDs
	}

	return nil
}

// bfsCollectM2M follows RelatedGroups, RelatedResources, RelatedNotes edges
// from frontier groups. Returns newly discovered group IDs.
func (ctx *MahresourcesContext) bfsCollectM2M(plan *exportPlan, frontier []uint) ([]uint, error) {
	var groups []models.Group
	if err := ctx.db.
		Preload("RelatedGroups").
		Preload("RelatedResources").
		Preload("RelatedNotes").
		Where("id IN ?", frontier).
		Find(&groups).Error; err != nil {
		return nil, err
	}

	var newGroupIDs []uint

	for _, g := range groups {
		for _, rg := range g.RelatedGroups {
			if _, exists := plan.groupExportID[rg.ID]; !exists {
				plan.groupIDs = append(plan.groupIDs, rg.ID)
				plan.groupExportID[rg.ID] = fmt.Sprintf("g%04d", len(plan.groupExportID)+1)
				plan.shellGroupIDs[rg.ID] = true
				newGroupIDs = append(newGroupIDs, rg.ID)
			}
		}

		for _, rr := range g.RelatedResources {
			if _, exists := plan.resourceExportID[rr.ID]; !exists {
				plan.resourceIDs = append(plan.resourceIDs, rr.ID)
				plan.resourceExportID[rr.ID] = fmt.Sprintf("r%04d", len(plan.resourceExportID)+1)
				if rr.Hash != "" {
					if _, seen := plan.uniqueHashes[rr.Hash]; !seen {
						plan.uniqueHashes[rr.Hash] = rr.FileSize
						plan.totalBytes += rr.FileSize
					}
				}
			}
		}

		for _, rn := range g.RelatedNotes {
			if _, exists := plan.noteExportID[rn.ID]; !exists {
				plan.noteIDs = append(plan.noteIDs, rn.ID)
				plan.noteExportID[rn.ID] = fmt.Sprintf("n%04d", len(plan.noteExportID)+1)
			}
		}
	}

	return newGroupIDs, nil
}

// bfsCollectGroupRelations follows typed GroupRelation edges from frontier
// groups. Returns newly discovered group IDs (as shells).
func (ctx *MahresourcesContext) bfsCollectGroupRelations(plan *exportPlan, frontier []uint) ([]uint, error) {
	var relations []models.GroupRelation
	if err := ctx.db.
		Preload("ToGroup").
		Where("from_group_id IN ?", frontier).
		Find(&relations).Error; err != nil {
		return nil, err
	}

	var newGroupIDs []uint

	for _, rel := range relations {
		if rel.ToGroupId == nil {
			continue
		}
		toID := *rel.ToGroupId
		if _, exists := plan.groupExportID[toID]; !exists {
			plan.groupIDs = append(plan.groupIDs, toID)
			plan.groupExportID[toID] = fmt.Sprintf("g%04d", len(plan.groupExportID)+1)
			plan.shellGroupIDs[toID] = true
			newGroupIDs = append(newGroupIDs, toID)
		}
	}

	return newGroupIDs, nil
}

// bfsEnsureOwners adds owning groups of BFS-discovered resources/notes as
// shell groups if they're not already in scope. Uses batch queries to avoid N+1.
func (ctx *MahresourcesContext) bfsEnsureOwners(plan *exportPlan) error {
	// Batch query resource owners
	if len(plan.resourceIDs) > 0 {
		type ownerRow struct {
			ID      uint
			OwnerID *uint
		}
		var rows []ownerRow
		if err := ctx.db.Model(&models.Resource{}).
			Select("id, owner_id").
			Where("id IN ? AND owner_id IS NOT NULL", plan.resourceIDs).
			Scan(&rows).Error; err != nil {
			return fmt.Errorf("bfs ensure resource owners: %w", err)
		}
		for _, row := range rows {
			if row.OwnerID == nil {
				continue
			}
			if _, exists := plan.groupExportID[*row.OwnerID]; !exists {
				plan.groupIDs = append(plan.groupIDs, *row.OwnerID)
				plan.groupExportID[*row.OwnerID] = fmt.Sprintf("g%04d", len(plan.groupExportID)+1)
				plan.shellGroupIDs[*row.OwnerID] = true
			}
		}
	}

	// Batch query note owners
	if len(plan.noteIDs) > 0 {
		type ownerRow struct {
			ID      uint
			OwnerID *uint
		}
		var rows []ownerRow
		if err := ctx.db.Model(&models.Note{}).
			Select("id, owner_id").
			Where("id IN ? AND owner_id IS NOT NULL", plan.noteIDs).
			Scan(&rows).Error; err != nil {
			return fmt.Errorf("bfs ensure note owners: %w", err)
		}
		for _, row := range rows {
			if row.OwnerID == nil {
				continue
			}
			if _, exists := plan.groupExportID[*row.OwnerID]; !exists {
				plan.groupIDs = append(plan.groupIDs, *row.OwnerID)
				plan.groupExportID[*row.OwnerID] = fmt.Sprintf("g%04d", len(plan.groupExportID)+1)
				plan.shellGroupIDs[*row.OwnerID] = true
			}
		}
	}

	return nil
}
```

- [ ] **Step 7: Update `toManifest` to set Shell and ShellGroups**

In `application_context/export_context.go`, in the `toManifest` function:

1. Set `ShellGroups` in Counts (near line 1582):
```go
	Counts: archive.Counts{
		Groups:      len(p.groupIDs),
		ShellGroups: len(p.shellGroupIDs),
		Notes:       len(p.noteIDs),
		// ... rest unchanged
```

2. Set `RelatedDepth` in ExportOptions (near line 1576):
```go
	ExportOptions: archive.ExportOptions{
		Scope:        req.Scope,
		Fidelity:     req.Fidelity,
		SchemaDefs:   req.SchemaDefs,
		Gzip:         req.Gzip,
		RelatedDepth: req.RelatedDepth,
	},
```

3. Set `Shell` on GroupEntry (near line 1615):
```go
		m.Entries.Groups = append(m.Entries.Groups, archive.GroupEntry{
			ExportID: p.groupExportID[row.ID],
			Name:     row.Name,
			SourceID: row.ID,
			Path:     "groups/" + p.groupExportID[row.ID] + ".json",
			Shell:    p.shellGroupIDs[row.ID],
		})
```

- [ ] **Step 8: Update `loadGroupPayload` to set Shell**

In `application_context/export_context.go`, in `loadGroupPayload` (near line 1153), add `Shell` to the payload:

```go
	p := &archive.GroupPayload{
		ExportID:         plan.groupExportID[g.ID],
		SourceID:         g.ID,
		Shell:            plan.shellGroupIDs[g.ID],
		Name:             g.Name,
		// ... rest unchanged
```

- [ ] **Step 9: Run the test**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestStreamExport_RelatedDepth1 -v`
Expected: PASS

- [ ] **Step 10: Run all existing tests to verify no regressions**

Run: `go test --tags 'json1 fts5' ./application_context/... -v`
Expected: All tests PASS

- [ ] **Step 11: Commit**

```bash
git add application_context/export_context.go application_context/export_context_test.go
git commit -m "feat: implement BFS related-depth traversal in export pipeline"
```

---

### Task 3: Additional Export BFS Tests

**Files:**
- Modify: `application_context/export_context_test.go`

- [ ] **Step 1: Write test — depth 0 backward compat**

```go
func TestStreamExport_RelatedDepth0_NoBFS(t *testing.T) {
	ctx := createTestContext(t)

	groupA := mustCreateGroup(t, ctx, "GroupA", nil)
	groupB := mustCreateGroup(t, ctx, "GroupB", nil)
	res := mustCreateResource(t, ctx, "external.txt", &groupB.ID, []byte("EXT"))
	mustLinkRelatedResource(t, ctx, groupA.ID, res.ID)

	var buf bytes.Buffer
	err := ctx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{OwnedResources: true, RelatedM2M: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		RelatedDepth: 0,
	}, &buf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	coll := newExportCollector()
	manifest, _ := archive.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), coll)

	if len(coll.groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(coll.groups))
	}
	if len(coll.resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(coll.resources))
	}
	// Resource should appear as dangling ref
	if len(manifest.Dangling) != 1 {
		t.Errorf("expected 1 dangling ref, got %d", len(manifest.Dangling))
	}
}
```

- [ ] **Step 2: Write test — depth 2 chaining**

```go
func TestStreamExport_RelatedDepth2_Chaining(t *testing.T) {
	ctx := createTestContext(t)

	// A -> (related) -> B -> (related) -> C, C owns resource
	groupA := mustCreateGroup(t, ctx, "A", nil)
	groupB := mustCreateGroup(t, ctx, "B", nil)
	groupC := mustCreateGroup(t, ctx, "C", nil)
	res := mustCreateResource(t, ctx, "deep.txt", &groupC.ID, []byte("DEEP"))

	mustLinkRelatedGroup(t, ctx, groupA.ID, groupB.ID)
	mustLinkRelatedResource(t, ctx, groupB.ID, res.ID)

	var buf bytes.Buffer
	err := ctx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{OwnedResources: true, RelatedM2M: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		RelatedDepth: 2,
	}, &buf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	coll := newExportCollector()
	manifest, _ := archive.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), coll)

	// A (root) + B (shell, depth 1) + C (shell, owner of res)
	if len(coll.groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(coll.groups))
	}
	if len(coll.resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(coll.resources))
	}
	if manifest.Counts.ShellGroups != 2 {
		t.Errorf("expected 2 shell groups, got %d", manifest.Counts.ShellGroups)
	}
}
```

- [ ] **Step 3: Write test — early termination**

```go
func TestStreamExport_RelatedDepth_EarlyTermination(t *testing.T) {
	ctx := createTestContext(t)

	// A -> (related) -> B, but B has no further relations. Depth 3 should stop at depth 1.
	groupA := mustCreateGroup(t, ctx, "A", nil)
	groupB := mustCreateGroup(t, ctx, "B", nil)
	mustLinkRelatedGroup(t, ctx, groupA.ID, groupB.ID)

	var buf bytes.Buffer
	err := ctx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{RelatedM2M: true},
		RelatedDepth: 3,
	}, &buf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	coll := newExportCollector()
	archive.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), coll)

	if len(coll.groups) != 2 {
		t.Errorf("expected 2 groups (A + B shell), got %d", len(coll.groups))
	}
}
```

- [ ] **Step 4: Write test — deduplication**

```go
func TestStreamExport_RelatedDepth_Dedup(t *testing.T) {
	ctx := createTestContext(t)

	// Both A and B relate to the same resource
	groupA := mustCreateGroup(t, ctx, "A", nil)
	groupB := mustCreateGroup(t, ctx, "B", &groupA.ID)
	groupC := mustCreateGroup(t, ctx, "C", nil)
	res := mustCreateResource(t, ctx, "shared.txt", &groupC.ID, []byte("SHARED"))

	mustLinkRelatedResource(t, ctx, groupA.ID, res.ID)
	mustLinkRelatedResource(t, ctx, groupB.ID, res.ID)

	var buf bytes.Buffer
	err := ctx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true, RelatedM2M: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		RelatedDepth: 1,
	}, &buf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	coll := newExportCollector()
	archive.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), coll)

	if len(coll.resources) != 1 {
		t.Errorf("expected 1 resource (no duplicates), got %d", len(coll.resources))
	}
}
```

- [ ] **Step 5: Write test — scope flag interaction**

```go
func TestStreamExport_RelatedDepth_NoRelatedM2M(t *testing.T) {
	ctx := createTestContext(t)

	groupA := mustCreateGroup(t, ctx, "A", nil)
	groupB := mustCreateGroup(t, ctx, "B", nil)
	mustLinkRelatedGroup(t, ctx, groupA.ID, groupB.ID)

	var buf bytes.Buffer
	err := ctx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{RelatedM2M: false},
		RelatedDepth: 1,
	}, &buf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	coll := newExportCollector()
	archive.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), coll)

	// RelatedM2M off = no BFS even with depth > 0
	if len(coll.groups) != 1 {
		t.Errorf("expected 1 group (no BFS), got %d", len(coll.groups))
	}
}
```

- [ ] **Step 6: Write test — series on BFS resources**

```go
func TestStreamExport_RelatedDepth_SeriesOnBFSResource(t *testing.T) {
	ctx := createTestContext(t)

	groupA := mustCreateGroup(t, ctx, "A", nil)
	groupB := mustCreateGroup(t, ctx, "B", nil)

	series := &models.Series{Name: "Test Series", Slug: "test-series"}
	if err := ctx.db.Create(series).Error; err != nil {
		t.Fatalf("create series: %v", err)
	}
	t.Cleanup(func() { ctx.db.Unscoped().Delete(&models.Series{}, series.ID) })

	res := mustCreateResource(t, ctx, "series-member.txt", &groupB.ID, []byte("SERIESDATA"))
	ctx.db.Model(res).Update("series_id", series.ID)

	mustLinkRelatedResource(t, ctx, groupA.ID, res.ID)

	var buf bytes.Buffer
	err := ctx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{OwnedResources: true, RelatedM2M: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true, ResourceSeries: true},
		RelatedDepth: 1,
	}, &buf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	coll := newExportCollector()
	archive.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), coll)

	if len(coll.series) != 1 {
		t.Errorf("expected 1 series, got %d", len(coll.series))
	}
	// Resource should have SeriesRef set
	for _, rp := range coll.resources {
		if rp.SeriesRef == "" {
			t.Errorf("expected SeriesRef to be set on BFS-discovered resource")
		}
	}
}
```

- [ ] **Step 7: Write test — dangling beyond depth**

```go
func TestStreamExport_RelatedDepth_DanglingBeyondDepth(t *testing.T) {
	ctx := createTestContext(t)

	// A -> (related) -> B -> (related) -> C. Depth 1 should include B as shell, C as dangling.
	groupA := mustCreateGroup(t, ctx, "A", nil)
	groupB := mustCreateGroup(t, ctx, "B", nil)
	groupC := mustCreateGroup(t, ctx, "C", nil)
	mustLinkRelatedGroup(t, ctx, groupA.ID, groupB.ID)
	mustLinkRelatedGroup(t, ctx, groupB.ID, groupC.ID)

	var buf bytes.Buffer
	err := ctx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{RelatedM2M: true},
		RelatedDepth: 1,
	}, &buf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	coll := newExportCollector()
	manifest, _ := archive.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), coll)

	if len(coll.groups) != 2 {
		t.Errorf("expected 2 groups (A + B shell), got %d", len(coll.groups))
	}
	// C should be a dangling ref from B
	if len(manifest.Dangling) != 1 {
		t.Fatalf("expected 1 dangling ref, got %d", len(manifest.Dangling))
	}
	if manifest.Dangling[0].Kind != archive.DanglingKindRelatedGroup {
		t.Errorf("expected dangling kind %s, got %s", archive.DanglingKindRelatedGroup, manifest.Dangling[0].Kind)
	}
}
```

- [ ] **Step 8: Run all tests**

Run: `go test --tags 'json1 fts5' ./application_context/... -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add application_context/export_context_test.go
git commit -m "test: add export BFS depth tests (depth 0/1/2, dedup, series, dangling)"
```

---

### Task 4: Import Plan Shell Support

**Files:**
- Modify: `application_context/import_plan.go:35-45` (ImportPlanItem), `application_context/import_plan.go:115-122` (ImportDecisions), `application_context/import_plan.go:187-195` (ValidateForApply)
- Modify: `application_context/import_context.go:582-629` (buildItemTree)
- Modify: `application_context/import_plan_test.go` (new test)

- [ ] **Step 1: Write failing test — ImportPlanItem.Shell is set for shell groups**

In `application_context/import_plan_test.go` (or `import_context_test.go` if more appropriate), add:

```go
func TestParseImport_ShellGroups_MarkedInPlan(t *testing.T) {
	srcCtx := createTestContext(t)

	groupA := mustCreateGroup(t, srcCtx, "GroupA", nil)
	groupB := mustCreateGroup(t, srcCtx, "GroupB", nil)
	res := mustCreateResource(t, srcCtx, "rel.txt", &groupB.ID, []byte("REL"))
	mustLinkRelatedResource(t, srcCtx, groupA.ID, res.ID)

	// Export with depth 1
	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{OwnedResources: true, RelatedM2M: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true, Tags: true},
		RelatedDepth: 1,
	}, &tarBuf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Parse on destination
	dstCtx := createTestContext(t)
	jobID := "test-shell-plan"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBuf.Bytes(), 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Find the shell group in the plan items
	var foundShell bool
	var walkItems func(items []ImportPlanItem)
	walkItems = func(items []ImportPlanItem) {
		for _, item := range items {
			if item.Name == "GroupB" {
				if !item.Shell {
					t.Errorf("expected GroupB to have Shell=true in plan")
				}
				foundShell = true
				// Shell group should show resource count (it owns the pulled-in resource)
				if item.ResourceCount != 1 {
					t.Errorf("expected shell group resource count=1, got %d", item.ResourceCount)
				}
			}
			walkItems(item.Children)
		}
	}
	walkItems(plan.Items)
	if !foundShell {
		t.Error("shell group GroupB not found in plan items")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestParseImport_ShellGroups -v`
Expected: FAIL — `ImportPlanItem` has no `Shell` field.

- [ ] **Step 3: Add `Shell` to `ImportPlanItem`**

In `application_context/import_plan.go`:

```go
type ImportPlanItem struct {
	ExportID                string           `json:"export_id"`
	Kind                    string           `json:"kind"`
	Name                    string           `json:"name"`
	Shell                   bool             `json:"shell,omitempty"`
	OwnerRef                string           `json:"owner_ref,omitempty"`
	ResourceCount           int              `json:"resource_count,omitempty"`
	NoteCount               int              `json:"note_count,omitempty"`
	DescendantResourceCount int              `json:"descendant_resource_count,omitempty"`
	DescendantNoteCount     int              `json:"descendant_note_count,omitempty"`
	Children                []ImportPlanItem `json:"children,omitempty"`
}
```

- [ ] **Step 4: Set `Shell` in `buildItemTree`**

In `application_context/import_context.go`, in `buildItemTree`, when creating nodes:

```go
	for _, g := range collector.groups {
		nodes[g.ExportID] = &ImportPlanItem{
			ExportID: g.ExportID,
			Kind:     "group",
			Name:     g.Name,
			Shell:    g.Shell,
			OwnerRef: g.OwnerRef,
		}
	}
```

- [ ] **Step 5: Add `ShellGroupAction` and `ShellGroupActions` to `ImportDecisions`**

In `application_context/import_plan.go`, add after `DanglingAction`:

```go
type ShellGroupAction struct {
	Action        string `json:"action"`                    // "create" or "map_to_existing"
	DestinationID *uint  `json:"destination_id,omitempty"`  // required when Action = "map_to_existing"
}
```

Add `ShellGroupActions` to `ImportDecisions`:

```go
type ImportDecisions struct {
	ParentGroupID            *uint                        `json:"parent_group_id,omitempty"`
	ResourceCollisionPolicy  string                       `json:"resource_collision_policy"`
	AcknowledgeMissingHashes bool                         `json:"acknowledge_missing_hashes,omitempty"`
	MappingActions           map[string]MappingAction      `json:"mapping_actions"`
	DanglingActions          map[string]DanglingAction     `json:"dangling_actions"`
	ExcludedItems            []string                     `json:"excluded_items"`
	ShellGroupActions        map[string]ShellGroupAction  `json:"shell_group_actions,omitempty"`
}
```

- [ ] **Step 6: Extend `ValidateForApply` for shell group decisions**

In `application_context/import_plan.go`, add to `ValidateForApply`:

```go
func (p *ImportPlan) ValidateForApply(decisions *ImportDecisions) error {
	if p.HasUnresolvedAmbiguities(decisions) {
		return fmt.Errorf("unresolved ambiguous mappings in review plan")
	}
	if p.ManifestOnlyMissingHashes > 0 && !decisions.AcknowledgeMissingHashes {
		return fmt.Errorf("missing-hash acknowledgement required: %d resources have no bytes", p.ManifestOnlyMissingHashes)
	}
	// Validate shell group decisions
	for exportID, action := range decisions.ShellGroupActions {
		if action.Action == "map_to_existing" && action.DestinationID == nil {
			return fmt.Errorf("shell group %s: map_to_existing requires a destination_id", exportID)
		}
	}
	return nil
}
```

- [ ] **Step 7: Run the test**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestParseImport_ShellGroups -v`
Expected: PASS

- [ ] **Step 8: Write test — ValidateForApply rejects nil DestinationID**

```go
func TestValidateForApply_ShellGroupMapWithoutDest(t *testing.T) {
	plan := &ImportPlan{}
	decisions := &ImportDecisions{
		ResourceCollisionPolicy: "skip",
		MappingActions:          map[string]MappingAction{},
		DanglingActions:         map[string]DanglingAction{},
		ShellGroupActions: map[string]ShellGroupAction{
			"g0005": {Action: "map_to_existing", DestinationID: nil},
		},
	}
	err := plan.ValidateForApply(decisions)
	if err == nil {
		t.Fatal("expected validation error for map_to_existing without destination_id")
	}
}
```

- [ ] **Step 9: Run all tests**

Run: `go test --tags 'json1 fts5' ./application_context/... -v`
Expected: All PASS

- [ ] **Step 10: Commit**

```bash
git add application_context/import_plan.go application_context/import_context.go application_context/import_plan_test.go application_context/import_context_test.go
git commit -m "feat: add Shell flag to import plan, ShellGroupActions to ImportDecisions"
```

---

### Task 5: Apply Import — Shell Group Decisions and Conflict-Ignore

**Files:**
- Modify: `application_context/apply_import.go:596-675` (applyGroups), `application_context/apply_import.go:1130-1153` (GroupRelation wiring), `application_context/apply_import.go:135-159` (ImportApplyResult)
- Modify: `application_context/apply_import_test.go`

- [ ] **Step 1: Write failing test — shell group create round-trip**

In `application_context/apply_import_test.go`:

```go
func TestApplyImport_ShellGroupCreate(t *testing.T) {
	srcCtx := createTestContext(t)

	groupA := mustCreateGroup(t, srcCtx, "GroupA", nil)
	groupB := mustCreateGroup(t, srcCtx, "GroupB", nil)
	res := mustCreateResource(t, srcCtx, "rel.txt", &groupB.ID, []byte("SHELLDATA"))
	mustLinkRelatedResource(t, srcCtx, groupA.ID, res.ID)

	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{OwnedResources: true, RelatedM2M: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true, Tags: true},
		RelatedDepth: 1,
	}, &tarBuf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	dstCtx := createTestContext(t)
	jobID := "test-shell-create"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBuf.Bytes(), 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "duplicate"
	// Default: shell groups get "create"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Should have created 2 groups (A + shell B) and 1 resource
	if result.CreatedGroups != 2 {
		t.Errorf("expected 2 created groups, got %d", result.CreatedGroups)
	}
	if result.CreatedResources != 1 {
		t.Errorf("expected 1 created resource, got %d", result.CreatedResources)
	}
	if result.CreatedShellGroups != 1 {
		t.Errorf("expected 1 created shell group, got %d", result.CreatedShellGroups)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestApplyImport_ShellGroupCreate -v`
Expected: FAIL — `CreatedShellGroups` field doesn't exist yet.

- [ ] **Step 3: Add shell group result counters**

In `application_context/import_plan.go` (or `apply_import.go` where `ImportApplyResult` is defined), add:

```go
type ImportApplyResult struct {
	// ... existing fields ...
	CreatedShellGroups int `json:"created_shell_groups"`
	MappedShellGroups  int `json:"mapped_shell_groups"`
	// ... rest unchanged ...
}
```

- [ ] **Step 4: Modify `applyGroups` to handle shell group decisions**

In `application_context/apply_import.go`, in `applyGroups`, after `if s.isExcluded(item.ExportID)` and before the group creation code, add shell group handling:

```go
			// Shell group handling: check ShellGroupActions
			if gp.Shell && s.decisions.ShellGroupActions != nil {
				if action, ok := s.decisions.ShellGroupActions[item.ExportID]; ok {
					if action.Action == "map_to_existing" && action.DestinationID != nil {
						s.idMap[item.ExportID] = *action.DestinationID
						s.result.MappedShellGroups++
						// Recurse into children (there shouldn't be any for shells, but be safe)
						if err := walk(item.Children); err != nil {
							return err
						}
						continue
					}
				}
			}
```

After the `s.result.CreatedGroups++` line, add shell group counter:

```go
			if gp.Shell {
				s.result.CreatedShellGroups++
			}
```

- [ ] **Step 5: Update `buildDefaultDecisions` in test helper**

In `application_context/apply_import_test.go`, update `buildDefaultDecisions` to initialize `ShellGroupActions`:

```go
func buildDefaultDecisions(plan *ImportPlan) *ImportDecisions {
	d := &ImportDecisions{
		ResourceCollisionPolicy: "skip",
		MappingActions:          make(map[string]MappingAction),
		DanglingActions:         make(map[string]DanglingAction),
		ShellGroupActions:       make(map[string]ShellGroupAction),
	}
```

- [ ] **Step 6: Run the test**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestApplyImport_ShellGroupCreate -v`
Expected: PASS

- [ ] **Step 7: Write test — shell group map_to_existing**

```go
func TestApplyImport_ShellGroupMapToExisting(t *testing.T) {
	srcCtx := createTestContext(t)

	groupA := mustCreateGroup(t, srcCtx, "GroupA", nil)
	groupB := mustCreateGroup(t, srcCtx, "GroupB", nil)
	res := mustCreateResource(t, srcCtx, "rel.txt", &groupB.ID, []byte("MAPDATA"))
	mustLinkRelatedResource(t, srcCtx, groupA.ID, res.ID)

	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{OwnedResources: true, RelatedM2M: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true, Tags: true},
		RelatedDepth: 1,
	}, &tarBuf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	dstCtx := createTestContext(t)
	// Create a target group to map the shell to
	targetGroup := mustCreateGroup(t, dstCtx, "TargetGroup", nil)

	jobID := "test-shell-map"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBuf.Bytes(), 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "duplicate"

	// Find the shell group export ID and map it to targetGroup
	var shellExportID string
	var walkFind func(items []ImportPlanItem)
	walkFind = func(items []ImportPlanItem) {
		for _, item := range items {
			if item.Shell {
				shellExportID = item.ExportID
			}
			walkFind(item.Children)
		}
	}
	walkFind(plan.Items)
	if shellExportID == "" {
		t.Fatal("no shell group found in plan")
	}

	decisions.ShellGroupActions[shellExportID] = ShellGroupAction{
		Action:        "map_to_existing",
		DestinationID: &targetGroup.ID,
	}

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	if result.MappedShellGroups != 1 {
		t.Errorf("expected 1 mapped shell group, got %d", result.MappedShellGroups)
	}

	// Verify the resource was assigned to the target group
	var importedRes models.Resource
	if err := dstCtx.db.Where("name = ?", "rel.txt").Order("id DESC").First(&importedRes).Error; err != nil {
		t.Fatalf("find imported resource: %v", err)
	}
	if importedRes.OwnerId == nil || *importedRes.OwnerId != targetGroup.ID {
		t.Errorf("expected resource owner to be target group %d, got %v", targetGroup.ID, importedRes.OwnerId)
	}
}
```

- [ ] **Step 8: Run the map test**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestApplyImport_ShellGroupMap -v`
Expected: PASS

- [ ] **Step 9: Write test — GroupRelation conflict-ignore**

```go
func TestApplyImport_ShellGroupMap_DuplicateGroupRelation(t *testing.T) {
	srcCtx := createTestContext(t)

	grt := mustCreateGroupRelationType(t, srcCtx, "TestRelType")
	groupA := mustCreateGroup(t, srcCtx, "A", nil)
	groupB := mustCreateGroup(t, srcCtx, "B", nil)
	groupC := mustCreateGroup(t, srcCtx, "C", nil)

	mustCreateGroupRelation(t, srcCtx, groupB.ID, groupC.ID, grt.ID)
	mustLinkRelatedGroup(t, srcCtx, groupA.ID, groupB.ID)

	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{RelatedM2M: true, GroupRelations: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true, Tags: true, GroupRelationTypes: true},
		RelatedDepth: 1,
	}, &tarBuf, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	dstCtx := createTestContext(t)

	// Create target group + the same relation type + a group to be the "C" target
	dstGRT := mustCreateGroupRelationType(t, dstCtx, "TestRelType")
	targetGroup := mustCreateGroup(t, dstCtx, "TargetB", nil)
	dstGroupC := mustCreateGroup(t, dstCtx, "TargetC", nil)
	// Pre-create the relation that would conflict
	mustCreateGroupRelation(t, dstCtx, targetGroup.ID, dstGroupC.ID, dstGRT.ID)

	jobID := "test-shell-dup-rel"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBuf.Bytes(), 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	decisions := buildDefaultDecisions(plan)

	// Map shell group B -> targetGroup
	var shellExportID string
	var walkFind func(items []ImportPlanItem)
	walkFind = func(items []ImportPlanItem) {
		for _, item := range items {
			if item.Shell {
				shellExportID = item.ExportID
			}
			walkFind(item.Children)
		}
	}
	walkFind(plan.Items)

	decisions.ShellGroupActions[shellExportID] = ShellGroupAction{
		Action:        "map_to_existing",
		DestinationID: &targetGroup.ID,
	}

	// This should NOT fail even though the relation already exists
	_, err = dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply should succeed with conflict-ignore, got: %v", err)
	}
}
```

- [ ] **Step 10: Implement conflict-ignore for GroupRelation creation**

In `application_context/apply_import.go`, in the GroupRelation wiring section (~line 1150), replace `s.ctx.db.Create(&gr)` with conflict-ignore logic:

```go
			if err := s.ctx.db.Create(&gr).Error; err != nil {
				// Conflict-ignore: if this is a unique constraint violation
				// (e.g., when a shell group is mapped to an existing group that
				// already has this relation), skip silently.
				if isUniqueConstraintError(err) {
					continue
				}
				return fmt.Errorf("group %s relation to %s: %w", exportID, rel.ToRef, err)
			}
```

Add the helper (in `apply_import.go`):

```go
// isUniqueConstraintError checks if a GORM error is a unique constraint violation.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique")
}
```

Add `"strings"` to the imports if not already present.

- [ ] **Step 11: Run the conflict test**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestApplyImport_ShellGroupMap_Duplicate -v`
Expected: PASS

- [ ] **Step 12: Run all tests**

Run: `go test --tags 'json1 fts5' ./application_context/... -v`
Expected: All PASS

- [ ] **Step 13: Commit**

```bash
git add application_context/apply_import.go application_context/apply_import_test.go application_context/import_plan.go
git commit -m "feat: implement shell group decisions and conflict-ignore in import apply"
```

---

### Task 6: CLI Export — `--related-depth` Flag

**Files:**
- Modify: `cmd/mr/commands/group_export.go:55-77` (request body), `cmd/mr/commands/group_export.go:144-244` (options, flags)

- [ ] **Step 1: Add `RelatedDepth` to `exportCmdOptions`**

In `cmd/mr/commands/group_export.go`, add to `exportCmdOptions`:

```go
type exportCmdOptions struct {
	// ... existing fields ...
	RelatedDepth int
}
```

- [ ] **Step 2: Register the flag**

In `registerExportFlags`, add after the `--timeout` flag (near line 243):

```go
	cmd.Flags().IntVar(&opts.RelatedDepth, "related-depth", 0, "follow m2m relationships up to N hops deep (0 = off)")
```

- [ ] **Step 3: Wire into request body**

In the `RunE` function, add `RelatedDepth` to the request (near line 77):

```go
			req := application_context.ExportRequest{
				// ... existing fields ...
				Gzip:         opts.Gzip,
				RelatedDepth: opts.RelatedDepth,
			}
```

- [ ] **Step 4: Run Go tests**

Run: `go test --tags 'json1 fts5' ./cmd/mr/...`
Expected: PASS (if tests exist), or at minimum `go build --tags 'json1 fts5' ./cmd/mr` succeeds.

- [ ] **Step 5: Commit**

```bash
git add cmd/mr/commands/group_export.go
git commit -m "feat: add --related-depth flag to CLI export command"
```

---

### Task 7: CLI Import — Shell Group Decisions

**Files:**
- Modify: `cmd/mr/commands/group_import.go:186-250` (buildCLIDecisions)

- [ ] **Step 1: Extend `buildCLIDecisions` to populate `ShellGroupActions`**

In `cmd/mr/commands/group_import.go`, in `buildCLIDecisions`, add `ShellGroupActions` initialization and default population.

Initialize the map:

```go
	d := application_context.ImportDecisions{
		ResourceCollisionPolicy:  opts.OnResourceConflict,
		AcknowledgeMissingHashes: opts.AcknowledgeMissingHashes,
		MappingActions:           make(map[string]application_context.MappingAction),
		DanglingActions:          make(map[string]application_context.DanglingAction),
		ShellGroupActions:        make(map[string]application_context.ShellGroupAction),
	}
```

After the dangling actions section, add shell group defaults:

```go
	// Default all shell groups to "create".
	var walkItems func(items []application_context.ImportPlanItem)
	walkItems = func(items []application_context.ImportPlanItem) {
		for _, item := range items {
			if item.Shell {
				d.ShellGroupActions[item.ExportID] = application_context.ShellGroupAction{
					Action: "create",
				}
			}
			walkItems(item.Children)
		}
	}
	walkItems(plan.Items)
```

- [ ] **Step 2: Handle `--decisions` file override for shell group actions**

In the import command's `RunE`, where `--decisions` file is loaded (look for `opts.Decisions` usage), the JSON file is parsed into an `ImportDecisions` struct. Since `ShellGroupActions` is already a field on `ImportDecisions`, the JSON file will naturally override the CLI defaults when `--decisions` is specified. No additional code needed — the existing `json.Unmarshal` handles it.

Verify this by checking how `--decisions` is used. If the code merges the file into the CLI-built decisions rather than replacing, make sure `ShellGroupActions` from the file takes precedence.

- [ ] **Step 3: Build and verify**

Run: `go build --tags 'json1 fts5' ./cmd/mr`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/commands/group_import.go
git commit -m "feat: populate ShellGroupActions defaults in CLI import decisions"
```

---

### Task 8: Template UI — Export Form and Import Review

**Files:**
- Modify: `templates/adminExport.tpl` (add depth input)
- Modify: `src/components/adminExport.js` (add relatedDepth state)
- Modify: `templates/adminImport.tpl` (shell group display)
- Modify: `src/components/adminImport.js` (shell group decisions)

- [ ] **Step 1: Add `relatedDepth` to export JS component**

In `src/components/adminExport.js`, add to the data object (after `schemaDefs`):

```js
    relatedDepth: 0,
```

Update `requestBody()`:

```js
    requestBody() {
      return {
        rootGroupIds: this.selectedGroups.map(g => g.id),
        scope: this.scope,
        fidelity: this.fidelity,
        schemaDefs: this.schemaDefs,
        relatedDepth: this.relatedDepth,
      };
    },
```

- [ ] **Step 2: Add depth input to export template**

In `templates/adminExport.tpl`, after the `group_relations` checkbox (line 37), add:

```html
      <div class="flex items-center gap-2 mt-2" x-show="scope.related_m2m || scope.group_relations">
        <label for="related-depth" class="text-sm text-stone-600">Related depth (0 = off):</label>
        <input type="number" id="related-depth" x-model.number="relatedDepth" min="0" max="10"
               class="w-20 text-sm border-stone-300 rounded focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
               data-testid="export-related-depth">
      </div>
```

- [ ] **Step 3: Add shell group count to estimate output**

In `templates/adminExport.tpl`, in the estimate output section (after the Groups count div):

```html
      <div x-show="(estimateResult?.counts?.shell_groups || 0) > 0">Shell groups: <span x-text="estimateResult?.counts?.shell_groups || 0"></span></div>
```

- [ ] **Step 4: Add shell group indicator in import plan tree**

In `templates/adminImport.tpl`, find where plan items are displayed and add a visual indicator. Look for where `item.name` is rendered and add:

```html
<span x-show="item.shell" class="ml-1 text-xs text-stone-400 font-mono">(shell)</span>
```

- [ ] **Step 5: Add shell group decision controls in import review**

In `src/components/adminImport.js`, add `shellGroupActions` to the component state and populate it from the plan. Add a method to handle shell group decision changes. When submitting apply, include `shell_group_actions` in the decisions payload.

This depends on the exact structure of `adminImport.js` — the implementation should follow the existing pattern for dangling ref decisions.

- [ ] **Step 6: Build frontend**

Run: `npm run build-js && npm run build-css`
Expected: BUILD SUCCESS

- [ ] **Step 7: Commit**

```bash
git add templates/adminExport.tpl templates/adminImport.tpl src/components/adminExport.js src/components/adminImport.js
git commit -m "feat: add related-depth UI to export form and shell group display to import review"
```

---

### Task 9: E2E Tests

**Files:**
- Modify: `e2e/tests/cli/group-export.spec.ts`
- Modify: `e2e/tests/cli/group-import-apply.spec.ts`

- [ ] **Step 1: Write CLI export round-trip test with `--related-depth`**

In `e2e/tests/cli/group-export.spec.ts`, add:

```typescript
test('export with --related-depth includes related entities', async ({ cli, request }) => {
  // Create groups and a m2m relationship
  const groupA = await request.post('/v1/group', { data: { Name: 'ExportDepthA' } });
  const groupAData = await groupA.json();
  const groupB = await request.post('/v1/group', { data: { Name: 'ExportDepthB' } });
  const groupBData = await groupB.json();

  // Create a resource owned by B
  const formData = new FormData();
  formData.append('Name', 'depth-test.txt');
  formData.append('OwnerId', String(groupBData.ID || groupBData.id));
  // ... add file data

  // Link A -> B as RelatedGroup
  await request.post(`/v1/group/addRelatedGroups`, {
    data: { id: groupAData.ID || groupAData.id, relatedGroupIds: [groupBData.ID || groupBData.id] }
  });

  // Export with --related-depth 1
  const result = await cli.run(['group', 'export', String(groupAData.ID || groupAData.id),
    '--related-depth', '1', '-o', '/tmp/depth-test.tar']);
  expect(result.exitCode).toBe(0);

  // Import and verify
  const importResult = await cli.run(['group', 'import', '/tmp/depth-test.tar']);
  expect(importResult.exitCode).toBe(0);
  expect(importResult.stdout).toContain('ExportDepthB'); // shell group name should appear
});
```

Adapt this to the exact E2E test helper patterns used in the existing CLI tests.

- [ ] **Step 2: Run E2E tests**

Run: `cd e2e && npm run test:with-server:cli -- --grep "related-depth"`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/
git commit -m "test(e2e): add CLI round-trip test for --related-depth export"
```

---

### Task 10: Full Test Suite Verification

- [ ] **Step 1: Build the application**

Run: `npm run build`
Expected: BUILD SUCCESS

- [ ] **Step 2: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All PASS

- [ ] **Step 3: Run all E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: All PASS

- [ ] **Step 4: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`
Expected: All PASS

- [ ] **Step 5: Final commit if any remaining changes**

```bash
git status  # check for any unstaged files
```
