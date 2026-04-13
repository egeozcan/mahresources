# Entity GUID Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add UUID v7 GUIDs to 8 entity types, carry them through export/import archives, and support merge/skip/replace import semantics when a GUID-matched entity already exists locally.

**Architecture:** New `*string` GUID column on 8 models with BeforeCreate hooks. Export backfills NULL GUIDs lazily with atomic conditional UPDATE. Import resolves GUIDs before name-based matching and applies a user-chosen collision policy (merge/skip/replace). MRQL gets `guid` as a common field.

**Tech Stack:** Go, GORM, UUID v7 (stdlib crypto/rand + time), Pongo2 templates, Alpine.js, Playwright E2E tests.

**Spec:** `docs/superpowers/specs/2026-04-13-entity-guid-design.md`

---

### Task 1: UUID v7 Helper

**Files:**
- Create: `models/types/uuid_v7.go`
- Create: `models/types/uuid_v7_test.go`

- [ ] **Step 1: Write the failing test**

```go
// models/types/uuid_v7_test.go
package types

import (
	"regexp"
	"testing"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUIDv7_Format(t *testing.T) {
	id := NewUUIDv7()
	if !uuidPattern.MatchString(id) {
		t.Fatalf("invalid UUID v7 format: %s", id)
	}
}

func TestNewUUIDv7_Unique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := NewUUIDv7()
		if seen[id] {
			t.Fatalf("duplicate UUID v7: %s", id)
		}
		seen[id] = true
	}
}

func TestNewUUIDv7_TimeSorted(t *testing.T) {
	a := NewUUIDv7()
	b := NewUUIDv7()
	if a >= b {
		// This can only fail if the system clock is extremely coarse AND
		// the random tie-breaker happens to sort backwards. Practically
		// impossible, but guard against it with a retry.
		b = NewUUIDv7()
		if a >= b {
			t.Fatalf("expected %s < %s (time-sorted)", a, b)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./models/types/ -run TestNewUUIDv7 -v`
Expected: FAIL — `NewUUIDv7` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// models/types/uuid_v7.go
package types

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// NewUUIDv7 generates a UUID v7 per RFC 9562: 48-bit millisecond timestamp,
// 4-bit version (0b0111), 12-bit random, 2-bit variant (0b10), 62-bit random.
// No external dependencies.
func NewUUIDv7() string {
	ms := uint64(time.Now().UnixMilli())

	var buf [16]byte
	// Bytes 0-5: 48-bit big-endian timestamp
	binary.BigEndian.PutUint16(buf[0:2], uint16(ms>>32))
	binary.BigEndian.PutUint32(buf[2:6], uint32(ms))

	// Bytes 6-15: random
	if _, err := rand.Read(buf[6:]); err != nil {
		panic(fmt.Sprintf("uuid_v7: crypto/rand failed: %v", err))
	}

	// Version 7: high nibble of byte 6
	buf[6] = (buf[6] & 0x0f) | 0x70
	// Variant 10: high 2 bits of byte 8
	buf[8] = (buf[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(buf[0:4]),
		binary.BigEndian.Uint16(buf[4:6]),
		binary.BigEndian.Uint16(buf[6:8]),
		binary.BigEndian.Uint16(buf[8:10]),
		buf[10:16],
	)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./models/types/ -run TestNewUUIDv7 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add models/types/uuid_v7.go models/types/uuid_v7_test.go
git commit -m "feat: add UUID v7 generation helper"
```

---

### Task 2: Add GUID Column to 8 Models

**Files:**
- Modify: `models/group_model.go:9` (Group struct)
- Modify: `models/note_model.go:9` (Note struct)
- Modify: `models/resource_model.go:10` (Resource struct)
- Modify: `models/tag_model.go:8` (Tag struct)
- Modify: `models/category_model.go:8` (Category struct)
- Modify: `models/note_type_model.go:8` (NoteType struct)
- Modify: `models/resource_category_model.go:8` (ResourceCategory struct)
- Modify: `models/group_relation_model.go:7` (GroupRelationType struct)

- [ ] **Step 1: Add GUID field to all 8 models**

Add this field to each struct, right after the `UpdatedAt` field:

```go
GUID *string `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`
```

For `GroupRelationType` (which has no `UpdatedAt` field after the struct opening), add after the `UpdatedAt` line.

Each model gets this exact same field. The `*string` type maps to nullable SQL, allowing multiple NULLs in the unique index.

- [ ] **Step 2: Add BeforeCreate hooks to all 8 models**

Add to each model file, after the struct definition. Pattern for Group (repeat for all 8):

```go
func (g *Group) BeforeCreate(tx *gorm.DB) error {
	if g.GUID == nil {
		guid := types.NewUUIDv7()
		g.GUID = &guid
	}
	return nil
}
```

The 8 models and their receiver names:
- `Group` → `g *Group`
- `Note` → `a *Note` (matches existing `a Note` receivers)
- `Resource` → `r *Resource`
- `Tag` → `t *Tag`
- `Category` → `c *Category`
- `NoteType` → `a *NoteType` (matches existing `a NoteType` receivers)
- `ResourceCategory` → `c *ResourceCategory`
- `GroupRelationType` → `r *GroupRelationType`

Each file needs `"gorm.io/gorm"` imported. Files that already import `types` as `"mahresources/models/types"` don't need a new import; files that don't will need it added.

Files that already import `gorm`: check each. Files that don't: `tag_model.go`, `category_model.go`, `resource_category_model.go`, `group_relation_model.go` — these need `"gorm.io/gorm"` added.

- [ ] **Step 3: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Success — no errors.

- [ ] **Step 4: Run existing tests to verify no regressions**

Run: `go test --tags 'json1 fts5' ./... 2>&1 | tail -30`
Expected: All pass. The BeforeCreate hooks auto-populate GUIDs for new entities in tests.

- [ ] **Step 5: Commit**

```bash
git add models/group_model.go models/note_model.go models/resource_model.go models/tag_model.go models/category_model.go models/note_type_model.go models/resource_category_model.go models/group_relation_model.go
git commit -m "feat: add GUID column and BeforeCreate hooks to 8 entity models"
```

---

### Task 3: GUID in Archive Format

**Files:**
- Modify: `archive/manifest.go` (add GUID fields to payloads and entries)

- [ ] **Step 1: Add GUID to payload types**

Add `GUID string` field to these structs in `archive/manifest.go`:

```go
// In GroupPayload, after ExportID/SourceID:
GUID string `json:"guid,omitempty"`

// In NotePayload, after ExportID/SourceID:
GUID string `json:"guid,omitempty"`

// In ResourcePayload, after ExportID/SourceID:
GUID string `json:"guid,omitempty"`

// In CategoryDef, after ExportID/SourceID:
GUID string `json:"guid,omitempty"`

// In TagDef, after ExportID/SourceID:
GUID string `json:"guid,omitempty"`

// In GroupRelationTypeDef, after ExportID/SourceID:
GUID string `json:"guid,omitempty"`
```

Note: `NoteTypeDef = CategoryDef` so it inherits automatically. `ResourceCategoryDef` embeds `CategoryDef` so it also inherits.

- [ ] **Step 2: Add GUID to manifest entry types**

```go
// In GroupEntry, after ExportID:
GUID string `json:"guid,omitempty"`

// In NoteEntry, after ExportID:
GUID string `json:"guid,omitempty"`

// In ResourceEntry, after ExportID:
GUID string `json:"guid,omitempty"`

// In SchemaDefEntry, after ExportID:
GUID string `json:"guid,omitempty"`
```

- [ ] **Step 3: Verify it compiles and existing archive tests pass**

Run: `go build --tags 'json1 fts5' && go test --tags 'json1 fts5' ./archive/... -v`
Expected: All pass. New fields are `omitempty` so existing round-trip tests still work with empty GUIDs.

- [ ] **Step 4: Commit**

```bash
git add archive/manifest.go
git commit -m "feat: add GUID field to archive payload and entry types"
```

---

### Task 4: Export Writes GUIDs (with Lazy Backfill)

**Files:**
- Modify: `application_context/export_context.go` (loadGroupPayload, loadNotePayload, loadResourcePayload, writeCategoryDefs, writeNoteTypeDefs, writeResourceCategoryDefs, writeTagDefs, writeGroupRelationTypeDefs, toManifest)

- [ ] **Step 1: Add ensureGUID helper**

Add a private helper function in `export_context.go` that atomically assigns a GUID to any entity:

```go
// ensureGUID assigns a UUID v7 to the given entity row if it has none.
// Uses an atomic conditional UPDATE to handle concurrent exports safely.
// Returns the GUID (either pre-existing or newly assigned).
func (ctx *MahresourcesContext) ensureGUID(table string, id uint, existing *string) string {
	if existing != nil && *existing != "" {
		return *existing
	}
	guid := types.NewUUIDv7()
	result := ctx.db.Exec(
		fmt.Sprintf("UPDATE %s SET guid = ? WHERE id = ? AND guid IS NULL", table),
		guid, id,
	)
	if result.RowsAffected == 0 {
		// Another concurrent export already assigned a GUID — read it back.
		var row struct{ GUID *string }
		ctx.db.Raw(fmt.Sprintf("SELECT guid FROM %s WHERE id = ?", table), id).Scan(&row)
		if row.GUID != nil {
			return *row.GUID
		}
	}
	return guid
}
```

Add necessary imports: `"mahresources/models/types"` if not already present.

- [ ] **Step 2: Wire GUID into loadGroupPayload**

In `loadGroupPayload` (line ~1360), after building the `GroupPayload` struct, add:

```go
p.GUID = ctx.ensureGUID("groups", g.ID, g.GUID)
```

- [ ] **Step 3: Wire GUID into loadNotePayload**

In `loadNotePayload` (line ~1469), after building the `NotePayload` struct, add:

```go
p.GUID = ctx.ensureGUID("notes", n.ID, n.GUID)
```

- [ ] **Step 4: Wire GUID into loadResourcePayload**

In `loadResourcePayload` (line ~1555), after building the `ResourcePayload` struct, add:

```go
p.GUID = ctx.ensureGUID("resources", r.ID, r.GUID)
```

- [ ] **Step 5: Wire GUID into schema def writers**

In each of the 5 `write*Defs` functions, add GUID to the def struct being built. For example in `writeCategoryDefs` (line ~1162):

```go
// After building the CategoryDef literal, add:
defs = append(defs, archive.CategoryDef{
	// ... existing fields ...
	GUID: ctx.ensureGUID("categories", row.ID, row.GUID),
})
```

Repeat for:
- `writeNoteTypeDefs` — table `"note_types"`, field `row.GUID`
- `writeResourceCategoryDefs` — table `"resource_categories"`, field `row.GUID`
- `writeTagDefs` — table `"tags"`, field `row.GUID`
- `writeGroupRelationTypeDefs` — table `"group_relation_types"`, field `row.GUID`

Each of these functions loads rows via `ctx.db.Where("id IN ?", ids).Find(&rows)`. The model structs now have GUID fields, so `row.GUID` is available.

- [ ] **Step 6: Wire GUID into toManifest entries**

In `toManifest` (line ~1819), the manifest entries are built from `nameRow` structs. Extend the `nameRow` type to include GUID:

```go
type nameRow struct {
	ID   uint
	Name string
	GUID *string
}
```

Then for each entry builder, add the GUID. For groups (line ~1825):

```go
m.Entries.Groups = append(m.Entries.Groups, archive.GroupEntry{
	// ... existing fields ...
	GUID: ptrToString(row.GUID),
})
```

Add a small helper if needed:

```go
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
```

Repeat for notes and resources entries. For schema def entries (`SchemaDefEntry`), the `writeCategoryDefs` etc. functions build the `SchemaDefIndex` — those entries also need GUID. Check how `SchemaDefs` entries are built in `toManifest` and add GUID there.

- [ ] **Step 7: Verify it compiles and existing tests pass**

Run: `go build --tags 'json1 fts5' && go test --tags 'json1 fts5' ./... 2>&1 | tail -30`
Expected: All pass.

- [ ] **Step 8: Commit**

```bash
git add application_context/export_context.go
git commit -m "feat: export writes GUIDs with lazy atomic backfill"
```

---

### Task 5: Deep Merge Utility

**Files:**
- Create: `models/types/deep_merge.go`
- Create: `models/types/deep_merge_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// models/types/deep_merge_test.go
package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func parse(s string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		panic(err)
	}
	return m
}

func TestDeepMerge_IncomingWins(t *testing.T) {
	base := parse(`{"a": 1, "b": 2}`)
	incoming := parse(`{"b": 99, "c": 3}`)
	result := DeepMergeJSON(base, incoming)
	expected := parse(`{"a": 1, "b": 99, "c": 3}`)
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("got %v, want %v", result, expected)
	}
}

func TestDeepMerge_NestedMerge(t *testing.T) {
	base := parse(`{"nested": {"x": 1, "y": 2}, "top": "base"}`)
	incoming := parse(`{"nested": {"y": 99, "z": 3}, "top": "incoming"}`)
	result := DeepMergeJSON(base, incoming)
	expected := parse(`{"nested": {"x": 1, "y": 99, "z": 3}, "top": "incoming"}`)
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("got %v, want %v", result, expected)
	}
}

func TestDeepMerge_IncomingOverwritesNonMap(t *testing.T) {
	base := parse(`{"a": {"nested": true}}`)
	incoming := parse(`{"a": "string_now"}`)
	result := DeepMergeJSON(base, incoming)
	expected := parse(`{"a": "string_now"}`)
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("got %v, want %v", result, expected)
	}
}

func TestDeepMerge_NilBase(t *testing.T) {
	incoming := parse(`{"a": 1}`)
	result := DeepMergeJSON(nil, incoming)
	if !reflect.DeepEqual(result, incoming) {
		t.Fatalf("got %v, want %v", result, incoming)
	}
}

func TestDeepMerge_NilIncoming(t *testing.T) {
	base := parse(`{"a": 1}`)
	result := DeepMergeJSON(base, nil)
	if !reflect.DeepEqual(result, base) {
		t.Fatalf("got %v, want %v", result, base)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./models/types/ -run TestDeepMerge -v`
Expected: FAIL — `DeepMergeJSON` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// models/types/deep_merge.go
package types

// DeepMergeJSON recursively merges incoming into base. Incoming keys
// overwrite base keys. When both base and incoming values for the same
// key are map[string]any, they are merged recursively. Otherwise,
// the incoming value wins. Neither input is mutated; a new map is returned.
func DeepMergeJSON(base, incoming map[string]any) map[string]any {
	if base == nil {
		return incoming
	}
	if incoming == nil {
		return base
	}
	result := make(map[string]any, len(base)+len(incoming))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range incoming {
		if vMap, ok := v.(map[string]any); ok {
			if bMap, ok := result[k].(map[string]any); ok {
				result[k] = DeepMergeJSON(bMap, vMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./models/types/ -run TestDeepMerge -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add models/types/deep_merge.go models/types/deep_merge_test.go
git commit -m "feat: add deep merge utility for JSON meta fields"
```

---

### Task 6: MRQL guid Field

**Files:**
- Modify: `mrql/fields.go:22` (commonFields)
- Modify: `mrql/fields.go` (tests reference this — the completer/validator pick up fields automatically)

- [ ] **Step 1: Write a failing MRQL test**

Add to an existing test file or create one. Simplest: test that `LookupField` recognizes `guid`:

```go
// In a new test or appended to an existing mrql test file
func TestLookupField_GUID(t *testing.T) {
	for _, et := range []EntityType{EntityResource, EntityNote, EntityGroup} {
		fd, ok := LookupField(et, "guid")
		if !ok {
			t.Fatalf("guid not found for entity type %v", et)
		}
		if fd.Type != FieldString {
			t.Fatalf("guid should be FieldString, got %v", fd.Type)
		}
		if fd.Column != "guid" {
			t.Fatalf("guid column should be 'guid', got %q", fd.Column)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./mrql/ -run TestLookupField_GUID -v`
Expected: FAIL — `guid` not found.

- [ ] **Step 3: Add guid to commonFields**

In `mrql/fields.go`, add to the `commonFields` slice after the `tags` entry:

```go
{Name: "guid", Type: FieldString, Column: "guid"},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./mrql/ -run TestLookupField_GUID -v`
Expected: PASS

- [ ] **Step 5: Run full MRQL test suite for regressions**

Run: `go test --tags 'json1 fts5' ./mrql/... -v 2>&1 | tail -30`
Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add mrql/fields.go mrql/*_test.go
git commit -m "feat: add guid as common MRQL field for resource/note/group"
```

---

### Task 7: Import Plan — GUID Matching

**Files:**
- Modify: `application_context/import_plan.go` (add GUIDCollisionPolicy to ImportDecisions, GUIDMatch fields to ImportPlanItem, GUIDMatches to ConflictSummary, GUIDConflict/RenameTo to MappingEntry/MappingAction)
- Modify: `application_context/import_context.go` (add GUID-first resolution to resolve functions, GUID match detection in buildItemTree)

- [ ] **Step 1: Add new fields to import plan types**

In `application_context/import_plan.go`:

Add to `ImportPlanItem` struct:
```go
GUIDMatch     bool   `json:"guid_match,omitempty"`
GUIDMatchID   uint   `json:"guid_match_id,omitempty"`
GUIDMatchName string `json:"guid_match_name,omitempty"`
```

Add to `ConflictSummary` struct:
```go
GUIDMatches int `json:"guid_matches"`
```

Add to `ImportDecisions` struct:
```go
GUIDCollisionPolicy string `json:"guid_collision_policy,omitempty"`
```

Add to `MappingEntry` struct:
```go
GUIDConflict bool   `json:"guid_conflict,omitempty"`
GUIDMatchID  uint   `json:"guid_match_id,omitempty"`
GUIDMatchName string `json:"guid_match_name,omitempty"`
```

Add to `MappingAction` struct:
```go
RenameTo string `json:"rename_to,omitempty"`
```

- [ ] **Step 2: Add GUID-first resolution to resolveCategories**

In `import_context.go`, modify `resolveCategories` to check GUID before name. Before the existing `ctx.db.Where("name = ?", def.Name).First(&cat)` call, add:

```go
// GUID-first resolution
if def.GUID != "" {
	var guidMatch models.Category
	if err := ctx.db.Where("guid = ?", def.GUID).First(&guidMatch).Error; err == nil {
		entry.Suggestion = "map"
		id := guidMatch.ID
		entry.DestinationID = &id
		entry.DestinationName = guidMatch.Name
		entry.GUIDMatchID = guidMatch.ID
		entry.GUIDMatchName = guidMatch.Name

		// Check for name conflict: GUID points to one entity, name points to another
		if guidMatch.Name != def.Name {
			var nameMatch models.Category
			if err := ctx.db.Where("name = ?", def.Name).First(&nameMatch).Error; err == nil && nameMatch.ID != guidMatch.ID {
				entry.GUIDConflict = true
				entry.Ambiguous = true
				entry.Alternatives = append(entry.Alternatives, MappingAlternative{
					ID:   nameMatch.ID,
					Name: nameMatch.Name,
				})
			}
		}
		entries = append(entries, entry)
		continue
	}
}
```

The `continue` skips the existing name-based fallback. If no GUID match, the existing name-based code runs as before.

- [ ] **Step 3: Apply the same GUID-first pattern to the other 4 resolve functions**

Apply the identical pattern to:
- `resolveNoteTypes` — query `models.NoteType` by GUID
- `resolveResourceCategories` — query `models.ResourceCategory` by GUID
- `resolveTags` — query `models.Tag` by GUID
- `resolveGRTDefs` — query `models.GroupRelationType` by GUID (note: composite uniqueness here is Name+FromCategoryId+ToCategoryId, so the name-conflict check should check all three)

- [ ] **Step 4: Add GUID match detection to buildItemTree**

In `import_context.go`, find the `buildItemTree` function. For each group, note, and resource item, if the payload has a non-empty GUID, query the local DB. If found, set the `GUIDMatch`, `GUIDMatchID`, and `GUIDMatchName` fields on the `ImportPlanItem` and increment the `ConflictSummary.GUIDMatches` counter.

The `buildItemTree` function may not have access to the DB context. If so, move the GUID matching to `ParseImport` after `buildItemTree` returns, walking the items to populate GUID match fields via a new helper `ctx.resolveGUIDMatches(plan, collector)`.

- [ ] **Step 5: Verify it compiles and existing import tests pass**

Run: `go build --tags 'json1 fts5' && go test --tags 'json1 fts5' ./application_context/... -v 2>&1 | tail -30`
Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add application_context/import_plan.go application_context/import_context.go
git commit -m "feat: GUID-first resolution in import plan with conflict detection"
```

---

### Task 8: Import Apply — Merge/Skip/Replace for Groups and Notes

**Files:**
- Modify: `application_context/apply_import.go` (applyGroups, applyNotes)

- [ ] **Step 1: Add a GUID collision policy default**

In `apply_import.go`, in the `ApplyImport` function, after loading the plan and before phase 1, add a default:

```go
if decisions.GUIDCollisionPolicy == "" {
	decisions.GUIDCollisionPolicy = "merge"
}
```

- [ ] **Step 2: Add GUID merge/skip/replace logic to applyGroups**

In `applyGroups`, inside the `walk` function, after the shell group handling block and before the `g := models.Group{...}` creation, add GUID collision handling:

```go
// GUID collision handling
if gp.GUID != "" {
	var existing models.Group
	if err := s.ctx.db.Where("guid = ?", gp.GUID).First(&existing).Error; err == nil {
		s.idMap[item.ExportID] = existing.ID

		switch s.decisions.GUIDCollisionPolicy {
		case "skip":
			// Map and move on
			if err := walk(item.Children); err != nil {
				return err
			}
			continue
		case "merge":
			if err := s.mergeGroup(&existing, gp); err != nil {
				return fmt.Errorf("merge group %q: %w", gp.Name, err)
			}
		case "replace":
			if err := s.replaceGroup(&existing, gp); err != nil {
				return fmt.Errorf("replace group %q: %w", gp.Name, err)
			}
		}

		if err := walk(item.Children); err != nil {
			return err
		}
		continue
	}
}
```

- [ ] **Step 3: Implement mergeGroup**

```go
func (s *applyState) mergeGroup(existing *models.Group, gp *archive.GroupPayload) error {
	// Scalars: incoming wins
	updates := map[string]any{
		"name":        gp.Name,
		"description": gp.Description,
		"updated_at":  time.Now(),
	}
	if gp.URL != "" {
		parsed, err := url.Parse(gp.URL)
		if err == nil {
			u := types.URL(*parsed)
			updates["url"] = &u
		}
	}

	// Meta: deep merge
	if gp.Meta != nil {
		existingMeta := jsonToMap(existing.Meta)
		merged := types.DeepMergeJSON(existingMeta, gp.Meta)
		m, _ := json.Marshal(merged)
		updates["meta"] = types.JSON(m)
	}

	// Owner
	if gp.OwnerRef != "" {
		if ownerID, ok := s.idMap[gp.OwnerRef]; ok {
			updates["owner_id"] = ownerID
		}
	}

	// Category
	catID := s.resolveCategoryID(gp.CategoryRef, gp.CategoryName)
	if catID != nil {
		updates["category_id"] = *catID
	}

	if err := s.ctx.db.Model(existing).Updates(updates).Error; err != nil {
		return err
	}

	// M2M: union tags
	if err := s.unionGroupTags(existing.ID, gp); err != nil {
		return err
	}

	return nil
}
```

- [ ] **Step 4: Implement replaceGroup**

```go
func (s *applyState) replaceGroup(existing *models.Group, gp *archive.GroupPayload) error {
	// Scalars: incoming overwrites all
	updates := map[string]any{
		"name":        gp.Name,
		"description": gp.Description,
		"updated_at":  time.Now(),
	}
	if gp.URL != "" {
		parsed, err := url.Parse(gp.URL)
		if err == nil {
			u := types.URL(*parsed)
			updates["url"] = &u
		}
	} else {
		updates["url"] = nil
	}

	// Meta: incoming replaces entirely
	if gp.Meta != nil {
		m, _ := json.Marshal(gp.Meta)
		updates["meta"] = types.JSON(m)
	} else {
		updates["meta"] = types.JSON("null")
	}

	// Owner
	if gp.OwnerRef != "" {
		if ownerID, ok := s.idMap[gp.OwnerRef]; ok {
			updates["owner_id"] = ownerID
		}
	}

	// Category
	catID := s.resolveCategoryID(gp.CategoryRef, gp.CategoryName)
	if catID != nil {
		updates["category_id"] = *catID
	}

	if err := s.ctx.db.Model(existing).Updates(updates).Error; err != nil {
		return err
	}

	// M2M: clear all existing links, set exactly incoming
	s.ctx.db.Exec("DELETE FROM group_tags WHERE group_id = ?", existing.ID)
	if err := s.unionGroupTags(existing.ID, gp); err != nil {
		return err
	}

	return nil
}
```

- [ ] **Step 5: Add helper functions**

```go
// resolveCategoryID resolves a category from ref or name via the idMap.
func (s *applyState) resolveCategoryID(catRef, catName string) *uint {
	if catRef != "" {
		if catID, ok := s.idMap[catRef]; ok {
			return &catID
		}
	}
	if catName != "" {
		catKey := DecisionKeyFor("category", MappingEntry{SourceKey: catName})
		if catID, ok := s.idMap[catKey]; ok {
			return &catID
		}
	}
	return nil
}

// unionGroupTags adds incoming tags to the group without removing existing ones.
func (s *applyState) unionGroupTags(groupID uint, gp *archive.GroupPayload) error {
	for _, tr := range gp.Tags {
		tagID := s.resolveTagID(tr)
		if tagID == 0 {
			continue
		}
		// INSERT OR IGNORE — if the link already exists, skip silently
		s.ctx.db.Exec(
			"INSERT OR IGNORE INTO group_tags (group_id, tag_id) VALUES (?, ?)",
			groupID, tagID,
		)
	}
	return nil
}

// resolveTagID finds a tag's DB ID from an archive TagRef.
func (s *applyState) resolveTagID(tr archive.TagRef) uint {
	if tr.Ref != "" {
		if id, ok := s.idMap[tr.Ref]; ok {
			return id
		}
	}
	tagKey := DecisionKeyFor("tag", MappingEntry{SourceKey: tr.Name})
	if id, ok := s.idMap[tagKey]; ok {
		return id
	}
	return 0
}
```

Note: For Postgres compatibility, the `INSERT OR IGNORE` needs to be `INSERT ... ON CONFLICT DO NOTHING`. Check how the existing codebase handles this — there may already be a pattern. If so, follow it.

- [ ] **Step 6: Apply analogous GUID logic to applyNotes**

The same pattern applies: check GUID, switch on policy, implement `mergeNote` and `replaceNote`. Notes have scalars (Name, Description), Meta, NoteTypeId, OwnerId, StartDate, EndDate, Blocks, and M2M (Tags, Resources, Groups).

For merge: incoming scalars win, deep merge meta, union M2M, keep existing blocks (blocks are owned content, similar to blob for resources).
For replace: overwrite all, clear M2M and re-link, delete existing blocks and import incoming ones.

- [ ] **Step 7: Verify it compiles and existing tests pass**

Run: `go build --tags 'json1 fts5' && go test --tags 'json1 fts5' ./application_context/... 2>&1 | tail -30`
Expected: All pass.

- [ ] **Step 8: Commit**

```bash
git add application_context/apply_import.go
git commit -m "feat: GUID merge/skip/replace for groups and notes during import"
```

---

### Task 9: Import Apply — Resource-Specific GUID Behavior

**Files:**
- Modify: `application_context/apply_import.go` (applyOneResource)

- [ ] **Step 1: Add GUID check before hash collision check in applyOneResource**

In `applyOneResource` (line ~779), before the existing `if rp.Hash != ""` hash collision check, add:

```go
// GUID collision takes precedence over hash collision
if rp.GUID != "" {
	var existing models.Resource
	if err := tx.Where("guid = ?", rp.GUID).First(&existing).Error; err == nil {
		s.idMap[exportID] = existing.ID

		switch s.decisions.GUIDCollisionPolicy {
		case "skip":
			return nil
		case "merge":
			return s.mergeResource(tx, &existing, rp, batch)
		case "replace":
			return s.replaceResource(tx, &existing, rp, batch)
		default:
			return s.mergeResource(tx, &existing, rp, batch)
		}
	}
}
```

- [ ] **Step 2: Implement mergeResource**

```go
func (s *applyState) mergeResource(tx *gorm.DB, existing *models.Resource, rp *archive.ResourcePayload, batch *batchAccumulator) error {
	// Non-blob scalars: incoming wins
	updates := map[string]any{
		"name":              rp.Name,
		"original_name":     rp.OriginalName,
		"original_location": rp.OriginalLocation,
		"description":       rp.Description,
		"category":          rp.Category,
		"updated_at":        time.Now(),
	}

	// OwnMeta: deep merge
	if rp.OwnMeta != nil {
		existingOwnMeta := jsonToMap(existing.OwnMeta)
		merged := types.DeepMergeJSON(existingOwnMeta, rp.OwnMeta)
		m, _ := json.Marshal(merged)
		updates["own_meta"] = types.JSON(m)
	}

	// Meta: deep merge
	if rp.Meta != nil {
		existingMeta := jsonToMap(existing.Meta)
		merged := types.DeepMergeJSON(existingMeta, rp.Meta)
		m, _ := json.Marshal(merged)
		updates["meta"] = types.JSON(m)
	}

	// ResourceCategory
	if rp.ResourceCategoryRef != "" {
		if rcID, ok := s.idMap[rp.ResourceCategoryRef]; ok {
			updates["resource_category_id"] = rcID
		}
	}

	// Owner
	if rp.OwnerRef != "" {
		if ownerID, ok := s.idMap[rp.OwnerRef]; ok {
			updates["owner_id"] = ownerID
		}
	}

	// Blob-derived metadata (ContentType, Width, Height, FileSize, ContentCategory):
	// NOT updated — they stay in sync with the kept blob.

	// Blob-coupled fields (Hash, Location, etc.): NOT updated.
	// Log warning if hashes differ.
	if rp.Hash != "" && rp.Hash != existing.Hash {
		s.result.Warnings = append(s.result.Warnings,
			fmt.Sprintf("Resource %q: GUID merge kept existing blob (hash %s), incoming has different hash %s", rp.Name, existing.Hash, rp.Hash))
	}

	if err := tx.Model(existing).Updates(updates).Error; err != nil {
		return err
	}

	// M2M: union tags
	for _, tr := range rp.Tags {
		tagID := s.resolveTagID(tr)
		if tagID == 0 {
			continue
		}
		tx.Exec("INSERT OR IGNORE INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", existing.ID, tagID)
	}

	return nil
}
```

- [ ] **Step 3: Implement replaceResource**

```go
func (s *applyState) replaceResource(tx *gorm.DB, existing *models.Resource, rp *archive.ResourcePayload, batch *batchAccumulator) error {
	hasBlobInArchive := s.blobPaths[rp.Hash] != ""

	if !hasBlobInArchive {
		// No blob in archive — update non-blob scalars and M2M only
		s.result.Warnings = append(s.result.Warnings,
			fmt.Sprintf("Resource %q: blob not present in archive, keeping existing file", rp.Name))

		updates := map[string]any{
			"name":              rp.Name,
			"original_name":     rp.OriginalName,
			"original_location": rp.OriginalLocation,
			"description":       rp.Description,
			"category":          rp.Category,
			"updated_at":        time.Now(),
		}
		if rp.Meta != nil {
			m, _ := json.Marshal(rp.Meta)
			updates["meta"] = types.JSON(m)
		}
		if rp.OwnMeta != nil {
			m, _ := json.Marshal(rp.OwnMeta)
			updates["own_meta"] = types.JSON(m)
		}
		return tx.Model(existing).Updates(updates).Error
	}

	// Full replace: blob + all fields
	updates := map[string]any{
		"name":              rp.Name,
		"original_name":     rp.OriginalName,
		"original_location": rp.OriginalLocation,
		"description":       rp.Description,
		"category":          rp.Category,
		"hash":              rp.Hash,
		"hash_type":         rp.HashType,
		"file_size":         rp.FileSize,
		"content_type":      rp.ContentType,
		"content_category":  rp.ContentCategory,
		"width":             rp.Width,
		"height":            rp.Height,
		"updated_at":        time.Now(),
	}
	if rp.Meta != nil {
		m, _ := json.Marshal(rp.Meta)
		updates["meta"] = types.JSON(m)
	}
	if rp.OwnMeta != nil {
		m, _ := json.Marshal(rp.OwnMeta)
		updates["own_meta"] = types.JSON(m)
	}

	// Replace blob on disk: copy from blobPaths to the existing resource's location
	blobSrc := s.blobPaths[rp.Hash]
	if existing.Location != "" && blobSrc != "" {
		srcFile, err := s.ctx.fs.Open(blobSrc)
		if err != nil {
			return fmt.Errorf("open replacement blob: %w", err)
		}
		defer srcFile.Close()

		dstFile, err := s.ctx.fs.Create(existing.GetCleanLocation())
		if err != nil {
			return fmt.Errorf("create replacement blob: %w", err)
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return fmt.Errorf("copy replacement blob: %w", err)
		}
		updates["location"] = existing.Location // keep same location path
	}

	if err := tx.Model(existing).Updates(updates).Error; err != nil {
		return err
	}

	// Delete existing versions, previews
	tx.Where("resource_id = ?", existing.ID).Delete(&models.ResourceVersion{})
	tx.Where("resource_id = ?", existing.ID).Delete(&models.Preview{})

	// Import incoming versions and previews via the existing batch/creation logic
	// Mark as "created" so previews/versions get processed
	s.createdResourceIDs[rp.ExportID] = true

	// Clear existing M2M, set incoming
	tx.Exec("DELETE FROM resource_tags WHERE resource_id = ?", existing.ID)
	for _, tr := range rp.Tags {
		tagID := s.resolveTagID(tr)
		if tagID == 0 {
			continue
		}
		tx.Exec("INSERT OR IGNORE INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", existing.ID, tagID)
	}

	return nil
}
```

- [ ] **Step 4: Verify it compiles and existing tests pass**

Run: `go build --tags 'json1 fts5' && go test --tags 'json1 fts5' ./application_context/... 2>&1 | tail -30`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add application_context/apply_import.go
git commit -m "feat: resource-specific GUID merge/skip/replace with blob handling"
```

---

### Task 10: Import Apply — Schema Def GUID Handling

**Files:**
- Modify: `application_context/apply_import.go` (applySchemaDefDecisions)

- [ ] **Step 1: Add GUID-aware handling to applySchemaDefDecisions**

In `applySchemaDefDecisions`, for each mapping entry that has a `GUIDConflict`, handle the `"guid_rename"` action:

```go
case "guid_rename":
	if action.RenameTo == "" {
		return fmt.Errorf("guid_rename action for %s requires rename_to", entry.DecisionKey)
	}
	// Update the GUID-matched entity with the new name
	if err := s.ctx.db.Model(&models.Category{}).
		Where("id = ?", entry.GUIDMatchID).
		Update("name", action.RenameTo).Error; err != nil {
		return fmt.Errorf("guid_rename %s: %w", entry.DecisionKey, err)
	}
	s.idMap[entry.DecisionKey] = entry.GUIDMatchID
```

Repeat for each schema def type (NoteType, ResourceCategory, Tag, GroupRelationType).

- [ ] **Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Success.

- [ ] **Step 3: Commit**

```bash
git add application_context/apply_import.go
git commit -m "feat: handle guid_rename action for schema def GUID conflicts"
```

---

### Task 11: UI — GUID Display on Detail Pages

**Files:**
- Modify: `templates/displayGroup.tpl` (sidebar)
- Modify: `templates/displayNote.tpl` (sidebar)
- Modify: `templates/displayResource.tpl` (sidebar)

- [ ] **Step 1: Add GUID display to group detail page**

In `templates/displayGroup.tpl`, in the `{% block sidebar %}` section, add a new `sidebar-group` div after the last existing one (before `{% endblock %}`):

```html
{% if group.GUID %}
<div class="sidebar-group">
    <p class="text-xs text-stone-400 break-all cursor-pointer" title="Click to copy GUID" onclick="navigator.clipboard.writeText('{{ group.GUID }}')">
        GUID: {{ group.GUID }}
    </p>
</div>
{% endif %}
```

- [ ] **Step 2: Add GUID display to note detail page**

Same pattern in `templates/displayNote.tpl`, using `note.GUID`:

```html
{% if note.GUID %}
<div class="sidebar-group">
    <p class="text-xs text-stone-400 break-all cursor-pointer" title="Click to copy GUID" onclick="navigator.clipboard.writeText('{{ note.GUID }}')">
        GUID: {{ note.GUID }}
    </p>
</div>
{% endif %}
```

- [ ] **Step 3: Add GUID display to resource detail page**

Same pattern in `templates/displayResource.tpl`, using `resource.GUID`:

```html
{% if resource.GUID %}
<div class="sidebar-group">
    <p class="text-xs text-stone-400 break-all cursor-pointer" title="Click to copy GUID" onclick="navigator.clipboard.writeText('{{ resource.GUID }}')">
        GUID: {{ resource.GUID }}
    </p>
</div>
{% endif %}
```

- [ ] **Step 4: Commit**

```bash
git add templates/displayGroup.tpl templates/displayNote.tpl templates/displayResource.tpl
git commit -m "feat: display GUID on entity detail page sidebars"
```

---

### Task 12: UI — Import Review GUID Collision Policy

**Files:**
- Modify: `templates/adminImport.tpl` (add GUID collision policy selector)
- Modify: `src/components/adminImport.js` (add guid_collision_policy to decisions object)

- [ ] **Step 1: Add GUID collision policy dropdown to import template**

In `templates/adminImport.tpl`, near the existing `collision-policy` select (line ~113), add after it:

```html
<div x-show="plan && plan.conflicts && plan.conflicts.guid_matches > 0">
    <label class="block text-sm font-medium text-stone-700 mb-1" for="guid-collision-policy">GUID Collision Policy</label>
    <p class="text-xs text-stone-500 mb-2">
        <span x-text="plan.conflicts.guid_matches"></span> entities match by GUID. Choose what happens to existing entities.
    </p>
    <select id="guid-collision-policy" x-model="decisions.guid_collision_policy"
            class="mt-0.5 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">
        <option value="merge">Merge (update fields, union tags)</option>
        <option value="skip">Skip (keep existing)</option>
        <option value="replace">Replace (overwrite everything)</option>
    </select>
</div>
```

- [ ] **Step 2: Add GUID matches summary line**

In the import summary section of the template, add a line showing GUID match count:

```html
<template x-if="plan.conflicts.guid_matches > 0">
    <p class="text-sm text-amber-700">
        <span x-text="plan.conflicts.guid_matches"></span> entities match by GUID
    </p>
</template>
```

- [ ] **Step 3: Add guid_collision_policy to decisions in Alpine component**

In `src/components/adminImport.js`, find where the `decisions` object is initialized. Add:

```js
guid_collision_policy: 'merge',
```

- [ ] **Step 4: Build JS bundle**

Run: `npm run build-js`
Expected: Success.

- [ ] **Step 5: Commit**

```bash
git add templates/adminImport.tpl src/components/adminImport.js
npm run build-js
git add public/dist/
git commit -m "feat: GUID collision policy selector in import review UI"
```

---

### Task 13: E2E Tests — Export/Import Round-Trip with GUIDs

**Files:**
- Create: `e2e/tests/cli/guid-round-trip.spec.ts` (or add to existing import/export test files depending on convention)

- [ ] **Step 1: Check existing CLI E2E test patterns**

Read existing CLI test files in `e2e/tests/cli/` to understand the test fixture and patterns (CliRunner, server management). The tests should follow the same patterns.

- [ ] **Step 2: Write E2E test for GUID round-trip with merge**

Test flow:
1. Create a group with some tags and meta via API
2. Export it
3. Modify the group locally (change name, add a tag)
4. Re-import the export with `guid_collision_policy: "merge"`
5. Verify: name reverted to export's name (incoming wins), tags unioned, meta deep-merged

- [ ] **Step 3: Write E2E test for GUID round-trip with skip**

Test flow:
1. Create a group, export it
2. Modify the group locally
3. Re-import with `guid_collision_policy: "skip"`
4. Verify: group unchanged

- [ ] **Step 4: Write E2E test for GUID round-trip with replace**

Test flow:
1. Create a group with tags, export it
2. Locally add extra tags
3. Re-import with `guid_collision_policy: "replace"`
4. Verify: tags are exactly the export's tags (extras removed)

- [ ] **Step 5: Write E2E test for backward compatibility (no GUIDs)**

Test flow:
1. Manually create a minimal archive tar without GUID fields
2. Import it
3. Verify: import succeeds, entities created normally

- [ ] **Step 6: Write E2E test for resource GUID merge (different hash)**

Test flow:
1. Create a resource (upload a file), export it
2. Upload a different file to the same resource (or create a new resource with the same GUID manually)
3. Re-import with merge
4. Verify: original blob kept, warning about differing hash

- [ ] **Step 7: Run all E2E tests**

Run: `cd e2e && npm run test:with-server:all`
Expected: All pass.

- [ ] **Step 8: Commit**

```bash
git add e2e/tests/
git commit -m "test: E2E tests for GUID export/import round-trip"
```

---

### Task 14: Go Unit Tests — Concurrent Backfill and Import Logic

**Files:**
- Create or modify: `application_context/export_context_test.go` or a new `application_context/guid_test.go`

- [ ] **Step 1: Write concurrent backfill test**

Test that two goroutines calling `ensureGUID` on the same entity converge on the same GUID:

```go
func TestEnsureGUID_ConcurrentConverges(t *testing.T) {
	// Set up in-memory SQLite DB with a group that has no GUID
	// Launch 10 goroutines all calling ensureGUID for the same group ID
	// Verify all 10 return the same GUID value
}
```

- [ ] **Step 2: Write MRQL integration test for guid field**

```go
func TestMRQL_GUIDQuery(t *testing.T) {
	// Create a group with a known GUID
	// Run MRQL query: type = "group" AND guid = "<the-guid>"
	// Verify it returns exactly that group
}
```

- [ ] **Step 3: Run all Go tests**

Run: `go test --tags 'json1 fts5' ./... 2>&1 | tail -30`
Expected: All pass.

- [ ] **Step 4: Commit**

```bash
git add application_context/
git commit -m "test: concurrent GUID backfill and MRQL guid query tests"
```

---

### Task 15: Validation Gate — Full Test Suite

**Files:** None (verification only)

- [ ] **Step 1: Build the application**

Run: `npm run build`
Expected: CSS + JS + Go binary all build successfully.

- [ ] **Step 2: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All pass.

- [ ] **Step 3: Run full E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: All pass.

- [ ] **Step 4: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
Expected: All pass. (Requires Docker running.)

- [ ] **Step 5: Run E2E Postgres tests**

Run: `cd e2e && npm run test:with-server:postgres`
Expected: All pass.

- [ ] **Step 6: Spot-check INSERT OR IGNORE vs ON CONFLICT**

Review all `INSERT OR IGNORE` statements added in Tasks 8 and 9. If the codebase needs Postgres compatibility, these must be `INSERT ... ON CONFLICT DO NOTHING` instead. Check how the existing M2M insertion code in `apply_import.go` handles this — follow the same pattern.

- [ ] **Step 7: Final commit if any fixups needed**

```bash
git add -A && git commit -m "fix: address test/Postgres issues found during validation"
```
