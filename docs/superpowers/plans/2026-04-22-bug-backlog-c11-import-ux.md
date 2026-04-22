# Cluster 11 — Import UX (BH-016, BH-017)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Task groups A and B touch disjoint files; can run as parallel subagents. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make the import-result UI truthful when an import merges or re-links instead of creating (BH-016), and replace the misleading "unsupported schema_version 0" error with "missing required field" when `schema_version` is absent (BH-017).

**Architecture:** Two surgical fixes.

- **Group A (BH-016):** Extend `ImportApplyResult` in `application_context/import_plan.go` with `MergedGroups/Resources/Notes`, `LinkedByGUIDGroups/Resources/Notes`, `SkippedByPolicyGroups/Resources/Notes` counters. Wire them into the merge/skip/link branches in `application_context/apply_import.go`. Update `templates/adminImport.tpl` result panel to surface "N created, M merged, P re-linked, Q skipped".
- **Group B (BH-017):** `archive/reader.go::ReadManifest` decodes into a raw `map[string]json.RawMessage` first to detect the presence of `schema_version`, THEN unmarshals into the typed `Manifest`. New error type `ErrMissingSchemaVersion` for the absent case. The existing "unsupported" branch stays for present-but-invalid versions.

**Tech Stack:** Go (encoding/json, GORM), Pongo2, Alpine.js, Playwright E2E.

**Worktree branch:** `bugfix/c11-import-ux`

**Top-level spec:** `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 11.

---

## File structure

**Modified:**
- `application_context/import_plan.go:154-188` — extend `ImportApplyResult` struct
- `application_context/apply_import.go` — increment counters at merge/skip/link branches (groups ~1010, resources ~1270, notes ~1775)
- `templates/adminImport.tpl:380-406` — surface new counters
- `archive/reader.go:56-79` — presence-check `schema_version`
- `archive/version.go` — add `ErrMissingSchemaVersion` type

**Created:**
- `archive/version_test.go` — unit test for presence-detection parse (if not already covered)
- `application_context/import_counters_test.go` — unit tests that merge/skip/link bump the right counters
- `e2e/tests/c11-bh016-import-counters.spec.ts` — E2E covering re-link path
- `e2e/tests/c11-bh017-missing-schema-version.spec.ts` — API test posting a manifest with no `schema_version`

---

## Task 0: Worktree + baseline

- [ ] **Step 1: Create worktree from master**

```bash
cd /Users/egecan/Code/mahresources
git worktree add -b bugfix/c11-import-ux ../mahresources-c11 master
cd ../mahresources-c11
```

- [ ] **Step 2: Run Go baseline**

```bash
go test --tags 'json1 fts5' ./... -count=1
```

Expected: PASS.

---

## Task Group A: BH-016 — Import counters

### Task A1: Write failing unit test for counter increments

**Files:**
- Create: `application_context/import_counters_test.go`

- [ ] **Step 1: Write the failing test**

```go
package application_context

import (
	"testing"
)

// TestImportApplyResult_NewCounters documents the expected fields added by BH-016.
// The counters are only meaningful once apply_import wires them — the compile
// check alone is enough to confirm the struct is extended.
func TestImportApplyResult_NewCounters(t *testing.T) {
	r := ImportApplyResult{}

	// BH-016: new counters — merged (GUID-collision policy=merge)
	_ = r.MergedGroups
	_ = r.MergedResources
	_ = r.MergedNotes

	// BH-016: new counters — linked by GUID (re-link path: existing GUID row
	// referenced by an incoming payload that targets the same entity)
	_ = r.LinkedByGUIDGroups
	_ = r.LinkedByGUIDResources
	_ = r.LinkedByGUIDNotes

	// BH-016: new counters — skipped by GUID-collision policy=skip
	_ = r.SkippedByPolicyGroups
	_ = r.SkippedByPolicyResources
	_ = r.SkippedByPolicyNotes
}

// TestHasMutations_NewCounters ensures the new counters participate in the
// "did this import actually change anything" check, so retry-safety logic
// treats a merge-only import as a mutation too.
func TestHasMutations_NewCounters(t *testing.T) {
	mergeOnly := ImportApplyResult{MergedGroups: 1}
	if !mergeOnly.HasMutations() {
		t.Fatal("expected HasMutations=true when MergedGroups>0")
	}
	linkOnly := ImportApplyResult{LinkedByGUIDResources: 1}
	if !linkOnly.HasMutations() {
		t.Fatal("expected HasMutations=true when LinkedByGUIDResources>0")
	}
	// SkippedByPolicy is NOT a mutation — the row existed before.
	skipOnly := ImportApplyResult{SkippedByPolicyNotes: 2}
	if skipOnly.HasMutations() {
		t.Fatal("expected HasMutations=false when only SkippedByPolicy>0")
	}
}
```

- [ ] **Step 2: Run 3× to verify fails**

```bash
go test --tags 'json1 fts5' ./application_context/ -run TestImportApplyResult_NewCounters -v -count=3
go test --tags 'json1 fts5' ./application_context/ -run TestHasMutations_NewCounters -v -count=3
```

Expected: all 6 runs FAIL with `r.MergedGroups undefined` (field doesn't exist) — that's the pre-implementation symptom.

### Task A2: Extend `ImportApplyResult` struct

**Files:**
- Modify: `application_context/import_plan.go:154-188`

- [ ] **Step 1: Add the new fields**

Find the struct definition at line 154:

```go
type ImportApplyResult struct {
	CreatedCategories         int      `json:"created_categories"`
	// ... existing fields ...
	CreatedShellGroups        int      `json:"created_shell_groups"`
	MappedShellGroups         int      `json:"mapped_shell_groups"`
	Warnings                  []string `json:"warnings"`
```

Add after `MappedShellGroups`, before `Warnings`:

```go
	// BH-016: GUID-collision policy=merge counters — merged existing rows with incoming payload
	MergedGroups    int `json:"merged_groups"`
	MergedResources int `json:"merged_resources"`
	MergedNotes     int `json:"merged_notes"`

	// BH-016: re-link counters — incoming GUID matched an existing row; no new row
	// created, but the plan's foreign-key targets were wired to the existing row.
	LinkedByGUIDGroups    int `json:"linked_by_guid_groups"`
	LinkedByGUIDResources int `json:"linked_by_guid_resources"`
	LinkedByGUIDNotes     int `json:"linked_by_guid_notes"`

	// BH-016: GUID-collision policy=skip counters — incoming payload skipped,
	// existing row untouched (no data mutation).
	SkippedByPolicyGroups    int `json:"skipped_by_policy_groups"`
	SkippedByPolicyResources int `json:"skipped_by_policy_resources"`
	SkippedByPolicyNotes     int `json:"skipped_by_policy_notes"`
```

- [ ] **Step 2: Update `HasMutations` to include merge + link counters**

Find the existing method (line ~194):

```go
func (r *ImportApplyResult) HasMutations() bool {
	// ... existing checks ...
}
```

Add the new counters to the check (merge + linkByGUID count as mutations; skipped-by-policy does NOT):

```go
func (r *ImportApplyResult) HasMutations() bool {
	if r == nil {
		return false
	}
	if r.CreatedCategories > 0 || r.CreatedNoteTypes > 0 ||
		r.CreatedResourceCategories > 0 || r.CreatedTags > 0 ||
		r.CreatedGRTs > 0 || r.CreatedSeries > 0 ||
		r.CreatedGroups > 0 || r.CreatedResources > 0 ||
		r.CreatedNotes > 0 || r.CreatedShellGroups > 0 ||
		r.CreatedVersions > 0 || r.CreatedPreviews > 0 {
		return true
	}
	// BH-016: merges and re-links are mutations (existing rows updated / wired)
	if r.MergedGroups > 0 || r.MergedResources > 0 || r.MergedNotes > 0 ||
		r.LinkedByGUIDGroups > 0 || r.LinkedByGUIDResources > 0 || r.LinkedByGUIDNotes > 0 {
		return true
	}
	return len(r.CreatedGroupIDs) > 0 || len(r.CreatedResourceIDs) > 0 || len(r.CreatedNoteIDs) > 0
}
```

- [ ] **Step 3: Run unit tests to verify pass**

```bash
go test --tags 'json1 fts5' ./application_context/ -run TestImportApplyResult_NewCounters -v -count=3
go test --tags 'json1 fts5' ./application_context/ -run TestHasMutations_NewCounters -v -count=3
```

Expected: all 6 PASS.

### Task A3: Wire counters into `apply_import.go`

**Files:**
- Modify: `application_context/apply_import.go` — three hotspots (group, resource, note GUID-collision switches)

- [ ] **Step 1: Increment `MergedGroups` at the group merge branch (line ~1009-1011)**

Find:

```go
					case "merge":
						if err := s.mergeGroup(&existing, gp); err != nil {
							return fmt.Errorf("merge group %q: %w", gp.Name, err)
						}
```

Change to:

```go
					case "merge":
						if err := s.mergeGroup(&existing, gp); err != nil {
							return fmt.Errorf("merge group %q: %w", gp.Name, err)
						}
						s.result.MergedGroups++
```

Find the skip branch in the same switch (look for `case "skip":` nearby):

```go
					case "skip":
						// existing behavior: skip
```

Change to:

```go
					case "skip":
						s.result.SkippedByPolicyGroups++
						// existing behavior: skip
```

- [ ] **Step 2: Do the same for resources at line ~1268-1278**

Find the resource merge switch block and increment `MergedResources` in the merge case, `SkippedByPolicyResources` in the skip case.

- [ ] **Step 3: Do the same for notes at line ~1769-1779**

Find the note merge switch block and increment `MergedNotes` in the merge case, `SkippedByPolicyNotes` in the skip case.

- [ ] **Step 4: Increment `LinkedByGUID*` counters on the "link existing by GUID" path**

Search `apply_import.go` for where an incoming plan mapping targets an existing GUID without creating a new row. This is the re-link behavior iter-5 described (import succeeds; `created_*=0` because the GUID-referenced rows already exist and plan targets wire up to them). Look for keywords: `"existing"`, `DestinationID`, `map to existing`. Increment `LinkedByGUIDGroups++` / `LinkedByGUIDResources++` / `LinkedByGUIDNotes++` at the corresponding branches.

IF this path is unclear after 20 minutes of code reading, file a small research task via a subagent:
> "In application_context/apply_import.go, where does an incoming payload that matches an existing row by GUID get linked (as opposed to merged, skipped, or created)? Point at the exact line(s) for groups, resources, and notes."

Wire the counters at those lines.

### Task A4: Expose counters in `adminImport.tpl`

**Files:**
- Modify: `templates/adminImport.tpl:383-406`

- [ ] **Step 1: Add rows for the new counters**

Find the `<dl class="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">` block (line ~383) and add rows after the existing `Created` rows (but keep ordering stable — created first, then merged, linked, skipped):

```pongo2
            <dl class="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
              <dt class="text-stone-500">Groups created</dt>
              <dd x-text="applyResult.created_groups"></dd>
              <dt class="text-stone-500">Groups merged</dt>
              <dd x-text="applyResult.merged_groups || 0"></dd>
              <dt class="text-stone-500">Groups re-linked (GUID)</dt>
              <dd x-text="applyResult.linked_by_guid_groups || 0"></dd>
              <dt class="text-stone-500">Groups skipped (policy)</dt>
              <dd x-text="applyResult.skipped_by_policy_groups || 0"></dd>

              <dt class="text-stone-500">Resources created</dt>
              <dd x-text="applyResult.created_resources"></dd>
              <dt class="text-stone-500">Resources merged</dt>
              <dd x-text="applyResult.merged_resources || 0"></dd>
              <dt class="text-stone-500">Resources re-linked (GUID)</dt>
              <dd x-text="applyResult.linked_by_guid_resources || 0"></dd>
              <dt class="text-stone-500">Resources skipped (policy)</dt>
              <dd x-text="applyResult.skipped_by_policy_resources || 0"></dd>

              <dt class="text-stone-500">Notes created</dt>
              <dd x-text="applyResult.created_notes"></dd>
              <dt class="text-stone-500">Notes merged</dt>
              <dd x-text="applyResult.merged_notes || 0"></dd>
              <dt class="text-stone-500">Notes re-linked (GUID)</dt>
              <dd x-text="applyResult.linked_by_guid_notes || 0"></dd>
              <dt class="text-stone-500">Notes skipped (policy)</dt>
              <dd x-text="applyResult.skipped_by_policy_notes || 0"></dd>

              <dt class="text-stone-500">Skipped (hash match)</dt>
              <dd x-text="applyResult.skipped_by_hash"></dd>
              <dt class="text-stone-500">Skipped (missing bytes)</dt>
              <dd x-text="applyResult.skipped_missing_bytes"></dd>

              <!-- Secondary entities: categories, tags, series, previews, versions -->
              <dt class="text-stone-500">Categories created</dt>
              <dd x-text="applyResult.created_categories"></dd>
              <dt class="text-stone-500">Tags created</dt>
              <dd x-text="applyResult.created_tags"></dd>
              <dt class="text-stone-500">Series created</dt>
              <dd x-text="applyResult.created_series"></dd>
              <dt class="text-stone-500">Series reused</dt>
              <dd x-text="applyResult.reused_series"></dd>
              <dt class="text-stone-500">Previews created</dt>
              <dd x-text="applyResult.created_previews"></dd>
              <dt class="text-stone-500">Versions created</dt>
              <dd x-text="applyResult.created_versions"></dd>
            </dl>
```

### Task A5: Write failing E2E test for re-link counters

**Files:**
- Create: `e2e/tests/c11-bh016-import-counters.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-016: Import result UI hides GUID-reused AND GUID-merged entities.
 *
 * Scenario: export a group with notes + resources, keep the source,
 * re-import with default policy=merge. Before the fix: result showed
 * "0 created" for everything with no indication that merges occurred.
 * After the fix: "N merged" counters are surfaced alongside "created".
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-016: import counters surface merge + re-link', () => {
  test('re-importing a group triggers a visible merged_groups count', async ({ page, apiClient }) => {
    const group = await apiClient.createGroup({ name: `BH016-${Date.now()}` });
    await apiClient.createImageResource({ name: `BH016-r-${Date.now()}`, ownerGroupId: group.ID });
    await apiClient.createNote({ name: `BH016-n-${Date.now()}`, ownerGroupId: group.ID });

    // Export
    await page.goto(`/admin/export?preselectedIds=${group.ID}`);
    await page.getByTestId('export-submit-button').click();
    await expect.poll(async () => {
      const t = await page.locator('[data-testid="export-progress-panel"]').textContent();
      return t?.includes('completed');
    }, { timeout: 30_000 }).toBe(true);

    // Download the tar via the download link
    const downloadPromise = page.waitForEvent('download');
    await page.getByTestId('export-download-link').click();
    const dl = await downloadPromise;
    const tarPath = await dl.path();
    expect(tarPath).not.toBeNull();

    // Re-import it (source still present → merge path)
    await page.goto('/admin/import');
    await page.locator('input[type="file"]').setInputFiles(tarPath!);
    await page.getByTestId('import-upload-submit').click();
    await page.getByTestId('import-review-apply').click();

    // Assert the result panel shows merged_groups > 0 OR linked_by_guid_groups > 0
    const result = page.getByTestId('import-apply-result');
    await expect(result).toBeVisible({ timeout: 30_000 });
    const mergedRow = result.locator('dt:has-text("Groups merged") + dd');
    const linkedRow = result.locator('dt:has-text("Groups re-linked (GUID)") + dd');
    const merged = parseInt((await mergedRow.textContent()) || '0', 10);
    const linked = parseInt((await linkedRow.textContent()) || '0', 10);
    expect(merged + linked, `expected merged>0 OR re-linked>0 after reimport, got merged=${merged}, linked=${linked}`)
      .toBeGreaterThan(0);
  });
});
```

- [ ] **Step 2: Run 3× to verify fails** — data-testids may not exist yet; if so adjust selectors to existing ones. Check `templates/adminImport.tpl` for the current test-id conventions (e.g., `data-testid="import-*"`).

```bash
cd e2e && npx playwright test c11-bh016-import-counters --reporter=line --repeat-each=3
```

Expected: FAIL — either selectors missing or counter shows 0.

### Task A6: Run full test matrix for Group A and commit

```bash
go test --tags 'json1 fts5' ./application_context/ -v -count=1
npm run build
cd e2e && npx playwright test c11-bh016-import-counters --reporter=line
```

All green → commit:

```bash
git add application_context/import_plan.go application_context/apply_import.go \
  application_context/import_counters_test.go \
  templates/adminImport.tpl \
  e2e/tests/c11-bh016-import-counters.spec.ts \
  public/dist/ public/tailwind.css
git commit -m "feat(import): BH-016 — surface merged + re-linked + skipped counters

Previously ImportApplyResult only tracked Created* counters; merges and
re-links on GUID collision were invisible — users saw '0 created' and
couldn't tell if the import did anything.

Extend ImportApplyResult with MergedGroups/Resources/Notes,
LinkedByGUID* (3), SkippedByPolicy* (3). Wire them into the apply_import
merge/skip/link branches. Surface all 12 counters in adminImport.tpl.

Unit: application_context/import_counters_test.go.
E2E: e2e/tests/c11-bh016-import-counters.spec.ts."
```

---

## Task Group B: BH-017 — Missing schema_version error

### Task B1: Write failing test for the missing-field message

**Files:**
- Modify/Create: `archive/reader_test.go` (add a subtest) OR a new file `archive/reader_missing_schema_version_test.go`

- [ ] **Step 1: Write the failing test**

```go
package archive_test

import (
	"archive/tar"
	"bytes"
	"errors"
	"testing"

	"mahresources/archive"
)

// buildManifestTar packs a single manifest.json entry whose content is the
// supplied JSON bytes. Returns the tar stream so tests can feed it to
// archive.NewReader.
func buildManifestTar(t *testing.T, manifestJSON []byte) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o600, Size: int64(len(manifestJSON))}); err != nil {
		t.Fatalf("tw.WriteHeader: %v", err)
	}
	if _, err := tw.Write(manifestJSON); err != nil {
		t.Fatalf("tw.Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tw.Close: %v", err)
	}
	return &buf
}

// BH-017: omitting schema_version entirely should produce a "missing required
// field" error, NOT the misleading "unsupported schema_version 0".
func TestReadManifest_MissingSchemaVersion(t *testing.T) {
	// No schema_version field — Go's int default would be 0, and the old code
	// reported "unsupported schema_version 0".
	json := []byte(`{"created_at":"2026-04-22T00:00:00Z","created_by":"test"}`)
	buf := buildManifestTar(t, json)

	r, err := archive.NewReader(buf)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	_, err = r.ReadManifest()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var missing *archive.ErrMissingSchemaVersion
	if !errors.As(err, &missing) {
		t.Fatalf("expected ErrMissingSchemaVersion, got %T: %v", err, err)
	}
	// Sanity: the error message must mention "missing" + "schema_version".
	msg := err.Error()
	for _, substr := range []string{"missing", "schema_version"} {
		if !contains(msg, substr) {
			t.Errorf("error message %q missing substring %q", msg, substr)
		}
	}
}

// TestReadManifest_UnsupportedVersion keeps coverage on the existing branch.
func TestReadManifest_UnsupportedVersion(t *testing.T) {
	json := []byte(`{"schema_version":9999,"created_at":"2026-04-22T00:00:00Z"}`)
	buf := buildManifestTar(t, json)

	r, err := archive.NewReader(buf)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	_, err = r.ReadManifest()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var unsup *archive.ErrUnsupportedSchemaVersion
	if !errors.As(err, &unsup) {
		t.Fatalf("expected ErrUnsupportedSchemaVersion, got %T: %v", err, err)
	}
}

func contains(s, substr string) bool { return bytes.Contains([]byte(s), []byte(substr)) }
```

- [ ] **Step 2: Run 3× to verify fails**

```bash
go test --tags 'json1 fts5' ./archive/ -run TestReadManifest_MissingSchemaVersion -v -count=3
```

Expected: all 3 runs FAIL with `ErrMissingSchemaVersion undefined` OR (if type exists but branch isn't wired) with the old "unsupported schema_version 0" message.

### Task B2: Add `ErrMissingSchemaVersion` type

**Files:**
- Modify: `archive/version.go`

- [ ] **Step 1: Add the error type**

Append to `archive/version.go`:

```go
// ErrMissingSchemaVersion is returned by Reader.ReadManifest when the
// manifest.json lacks the `schema_version` field entirely. Distinguished
// from ErrUnsupportedSchemaVersion (present but invalid value) because
// "schema_version:0" was previously reported as "unsupported version 0",
// which misled users who had simply omitted the field.
type ErrMissingSchemaVersion struct{}

func (e *ErrMissingSchemaVersion) Error() string {
	return "archive: manifest is missing required field `schema_version`"
}
```

### Task B3: Detect absence in `ReadManifest`

**Files:**
- Modify: `archive/reader.go:56-79`

- [ ] **Step 1: Rewrite to presence-check via a two-pass decode**

Find the existing `ReadManifest` (line 56-79):

```go
func (r *Reader) ReadManifest() (*Manifest, error) {
	if r.manifest != nil {
		return r.manifest, nil
	}
	hdr, err := r.tr.Next()
	if err != nil {
		return nil, fmt.Errorf("archive: read first entry: %w", err)
	}
	if hdr.Name != "manifest.json" {
		return nil, fmt.Errorf("archive: first entry %q != manifest.json", hdr.Name)
	}
	var m Manifest
	dec := json.NewDecoder(r.tr)
	// Do NOT call DisallowUnknownFields — §6.4 requires forward compatibility
	// with unknown top-level keys.
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("archive: parse manifest: %w", err)
	}
	if !isSupportedVersion(m.SchemaVersion) {
		return nil, &ErrUnsupportedSchemaVersion{Got: m.SchemaVersion, Supported: SupportedVersions}
	}
	r.manifest = &m
	return &m, nil
}
```

Replace with (read bytes once, parse as map for presence-check, re-parse into typed struct):

```go
func (r *Reader) ReadManifest() (*Manifest, error) {
	if r.manifest != nil {
		return r.manifest, nil
	}
	hdr, err := r.tr.Next()
	if err != nil {
		return nil, fmt.Errorf("archive: read first entry: %w", err)
	}
	if hdr.Name != "manifest.json" {
		return nil, fmt.Errorf("archive: first entry %q != manifest.json", hdr.Name)
	}

	// Read the entire manifest body once so we can parse it twice: once as a
	// map to check for the presence of required fields (BH-017), once into
	// the typed Manifest.
	raw, err := io.ReadAll(r.tr)
	if err != nil {
		return nil, fmt.Errorf("archive: read manifest body: %w", err)
	}

	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawFields); err != nil {
		return nil, fmt.Errorf("archive: parse manifest: %w", err)
	}
	if _, hasVersion := rawFields["schema_version"]; !hasVersion {
		return nil, &ErrMissingSchemaVersion{}
	}

	var m Manifest
	// Do NOT call DisallowUnknownFields — §6.4 requires forward compatibility
	// with unknown top-level keys.
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("archive: parse manifest: %w", err)
	}
	if !isSupportedVersion(m.SchemaVersion) {
		return nil, &ErrUnsupportedSchemaVersion{Got: m.SchemaVersion, Supported: SupportedVersions}
	}
	r.manifest = &m
	return &m, nil
}
```

Add `"io"` to the import block if not already present.

- [ ] **Step 2: Run 3× to verify pass**

```bash
go test --tags 'json1 fts5' ./archive/ -run 'TestReadManifest_MissingSchemaVersion|TestReadManifest_UnsupportedVersion' -v -count=3
```

Expected: both PASS all 3 runs. The existing `TestReader_ReadManifest` and `roundtrip_test.go` must also still PASS — run `./archive/... -count=1` to confirm.

### Task B4: Verify existing archive tests still pass

```bash
go test --tags 'json1 fts5' ./archive/... -v -count=1
```

Expected: full PASS. If a test fails, the io.ReadAll path may not handle large manifests correctly — adjust buffer size or stream the first decode.

### Task B5: Commit

```bash
git add archive/version.go archive/reader.go \
  archive/reader_missing_schema_version_test.go
git commit -m "fix(archive): BH-017 — distinguish missing vs unsupported schema_version

Previously a manifest without schema_version decoded into Go's int zero
value and was reported as 'unsupported schema_version 0', which misled
users who had simply omitted the field.

Read manifest body once, parse into a map to presence-check required
fields, then re-parse into the typed Manifest. New ErrMissingSchemaVersion
returned when the field is absent; the existing ErrUnsupportedSchemaVersion
branch still handles present-but-invalid values.

Unit: archive/reader_missing_schema_version_test.go."
```

---

## Task C: Update `tasks/bug-hunt-log.md`

Same pattern as prior clusters. Mark BH-016 and BH-017 FIXED with PR placeholders; append to Fixed/closed table.

---

## Task D: Full test matrix + PR + merge + log backfill + cleanup

Same shape as prior plans. PR title: `fix(bughunt c11): BH-016/017 import UX`.

---

## Self-review checklist

- [ ] Both BH-IDs moved to Fixed/closed with real PR + sha
- [ ] `ImportApplyResult` gains 9 new counter fields
- [ ] `apply_import.go` increments counters in all 3 × 3 = 9 branches (group × {merge, link, skip}, resource × 3, note × 3)
- [ ] `adminImport.tpl` shows all 12 counter rows (3 existing created + 9 new)
- [ ] `archive/reader.go` error path is stable — missing/unsupported/ok all produce expected results
- [ ] Existing `archive/` roundtrip tests still pass
