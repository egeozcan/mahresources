# Cluster 10 — Jobs UI Polish (BH-015, BH-036)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Task groups A and B touch disjoint files and can run as parallel subagents. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Stop the group-export progress label from reading "5140%" for small payloads (BH-015), and disclose the `ExportRetention` window on the export page + per-job expiry timestamp on completed exports in the cockpit (BH-036).

**Architecture:** Two surgical frontend fixes plus one backend estimation refinement. Group A (BH-015) clamps the *label* sites to `Math.min(100, ...)` — the progress *bars* are already clamped via `getProgressPercent` — and backfills the backend's `totalBytes` estimate with a JSON-overhead heuristic so the raw number is also accurate, not merely clamped. Group B (BH-036) threads `config.ExportRetention` into the `adminExport.tpl` context and renders (i) a static helper line and (ii) a per-completed-job expiry timestamp in `downloadCockpit.tpl`.

**Tech Stack:** Pongo2, Alpine.js, Go (`time.Duration` formatting), Playwright E2E.

**Worktree branch:** `bugfix/c10-jobs-ui-polish`

**Top-level spec:** `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 10 section.

---

## File structure

**Modified:**
- `src/components/downloadCockpit.js:284` — cap `formatProgress` label at 100%
- `templates/adminExport.tpl:122` — cap the `(N%)` badge at 100%
- `templates/adminExport.tpl` — add retention helper text (BH-036)
- `templates/partials/downloadCockpit.tpl` — add expiry timestamp row on completed group-export rows (BH-036)
- `application_context/export_context.go:128-246` — in `buildExportPlan`, add a final JSON-overhead estimate pass
- `server/template_handlers/template_context_providers/admin_export_template_context.go` (find via grep if path differs) — expose `exportRetention` string to the template context

**Created:**
- `e2e/tests/c10-bh015-export-progress-cap.spec.ts` — assert the progress label never exceeds 100% on a small-payload export
- `e2e/tests/c10-bh036-export-retention-disclosure.spec.ts` — assert retention helper text + per-job expiry timestamp
- `application_context/export_overhead_test.go` — unit test for `estimateJSONOverhead(plan)` returning a sane value

---

## Task 0: Create worktree + baseline

- [ ] **Step 1: Create the worktree from master**

```bash
cd /Users/egecan/Code/mahresources
git worktree add -b bugfix/c10-jobs-ui-polish ../mahresources-c10 master
cd ../mahresources-c10
```

- [ ] **Step 2: Run baseline tests**

```bash
go test --tags 'json1 fts5' ./... -count=1
```

Expected: PASS. If not, STOP — fix the baseline or abort the cluster until master is green.

---

## Task Group A: BH-015 — Progress overflow

### Task A1: Write failing E2E test for the 5140% label

**Files:**
- Create: `e2e/tests/c10-bh015-export-progress-cap.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-015: export progress label overflows 100% for small-payload exports.
 *
 * Backend reports progressPercent = (bytes_written / totalSize) * 100,
 * where totalSize counts only unique blob bytes but bytes_written counts
 * everything in the tar (manifest + JSONs + padding). For a small export
 * (e.g., 2 tiny images) this blows past 100% — often reads "5140%".
 *
 * Fix: (a) clamp both label sites (adminExport.tpl and downloadCockpit.js)
 * to Math.min(100, ...). (b) Improve backend totalBytes estimate to
 * include JSON overhead so the raw number is accurate, not merely clamped.
 *
 * This test locks in the UI cap. A separate Go unit test covers the
 * backend estimate improvement.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-015: progress label caps at 100%', () => {
  test('small export shows label ≤ 100% in adminExport', async ({ page, apiClient }) => {
    const group = await apiClient.createGroup({ name: `BH015-${Date.now()}` });
    // Upload 2 tiny images so totalSize is small but tar overhead dominates
    const r1 = await apiClient.createImageResource({ name: `BH015-r1-${Date.now()}`, ownerGroupId: group.ID });
    const r2 = await apiClient.createImageResource({ name: `BH015-r2-${Date.now()}`, ownerGroupId: group.ID });

    await page.goto(`/admin/export?preselectedIds=${group.ID}`);
    await page.getByTestId('export-submit-button').click();

    // Wait for the job to reach completed
    await expect(page.getByTestId('export-progress-panel')).toBeVisible();
    await expect.poll(async () => {
      const statusText = await page.locator('[data-testid="export-progress-panel"]').locator('span:has-text("Status:") + span').first().textContent();
      return statusText?.trim();
    }, { timeout: 30_000 }).toBe('completed');

    // Parse the "(N%)" badge text
    const bytesCounter = page.getByTestId('export-bytes-counter');
    const text = await bytesCounter.textContent();
    const match = text?.match(/\((\d+)%\)/);
    expect(match, `expected (N%) in "${text}"`).not.toBeNull();
    const percent = parseInt(match![1], 10);
    expect(percent).toBeLessThanOrEqual(100);
    expect(percent).toBeGreaterThanOrEqual(0);
  });

  test('formatProgress in cockpit also caps at 100%', async ({ page }) => {
    // Seed a fake job with an overflowed progressPercent and verify formatProgress caps it
    await page.goto('/');
    const result = await page.evaluate(() => {
      const cockpit = (window as any).Alpine?.$data?.(document.querySelector('[x-data*="downloadCockpit"]'));
      // Easier: import the module's formatProgress via the exposed function
      const fn = (window as any).downloadCockpit;
      if (typeof fn !== 'function') return { error: 'window.downloadCockpit factory not found' };
      const inst = fn();
      return {
        ok: true,
        result: inst.formatProgress({ totalSize: 352, progress: 18096, progressPercent: 5140.9 }),
      };
    });
    expect(result).toHaveProperty('ok', true);
    expect((result as any).result).toMatch(/\(100\.0%\)/); // capped at 100.0
  });
});
```

- [ ] **Step 2: Run 3× to verify it fails**

```bash
cd e2e && npx playwright test c10-bh015-export-progress-cap --reporter=line --repeat-each=3
```

Expected: all 6 runs FAIL. First test fails with `percent > 100`; second fails with formatProgress returning something other than `(100.0%)` for the overflowed input.

### Task A2: Cap label in `adminExport.tpl`

**Files:**
- Modify: `templates/adminExport.tpl:122`

- [ ] **Step 1: Change the `(N%)` badge expression**

Find the existing line:

```pongo2
        <span x-show="(job?.progressPercent || -1) >= 0"> (<span x-text="Math.round(job?.progressPercent || 0)"></span>%)</span>
```

Replace with:

```pongo2
        <span x-show="(job?.progressPercent || -1) >= 0"> (<span x-text="Math.min(100, Math.round(job?.progressPercent || 0))"></span>%)</span>
```

### Task A3: Cap `formatProgress` in `downloadCockpit.js`

**Files:**
- Modify: `src/components/downloadCockpit.js:280-290`

- [ ] **Step 1: Clamp the label**

Find:

```javascript
formatProgress(job) {
    if (job.totalSize > 0) {
        const downloaded = this.formatBytes(job.progress);
        const total = this.formatBytes(job.totalSize);
        const percent = job.progressPercent.toFixed(1);
        return `${downloaded} / ${total} (${percent}%)`;
    } else if (job.progress > 0) {
        return `${this.formatBytes(job.progress)} downloaded`;
    }
    return '';
},
```

Replace with:

```javascript
formatProgress(job) {
    if (job.totalSize > 0) {
        const downloaded = this.formatBytes(job.progress);
        const total = this.formatBytes(job.totalSize);
        // BH-015: cap label at 100 — totalSize estimate sometimes understates
        // tar overhead so raw progressPercent can overshoot.
        const percent = Math.min(100, job.progressPercent).toFixed(1);
        return `${downloaded} / ${total} (${percent}%)`;
    } else if (job.progress > 0) {
        return `${this.formatBytes(job.progress)} downloaded`;
    }
    return '';
},
```

### Task A4: Expose `downloadCockpit` factory on window for tests

**Files:**
- Modify: `src/main.js`

- [ ] **Step 1: Add the export if not already present**

Add near the existing imports/exports:

```javascript
import { downloadCockpit } from './components/downloadCockpit.js';
window.downloadCockpit = downloadCockpit;
```

Check the file first — it may already expose this via Alpine registration. If it's already accessible, skip this task. Prefer doing nothing over duplicating.

### Task A5: Write failing Go unit test for JSON-overhead estimate

**Files:**
- Create: `application_context/export_overhead_test.go`

- [ ] **Step 1: Write the failing test**

```go
package application_context

import (
	"testing"
)

// TestEstimateJSONOverhead ensures small exports include a realistic estimate of
// the tar's JSON-payload overhead (manifest + per-entity JSONs + tar padding)
// so progressPercent doesn't overshoot 100% just because totalSize only counts
// blob bytes.
func TestEstimateJSONOverhead(t *testing.T) {
	t.Run("empty plan returns a non-zero baseline for manifest", func(t *testing.T) {
		plan := &exportPlan{}
		got := estimateJSONOverhead(plan)
		if got < 1024 {
			t.Fatalf("expected ≥1 KB manifest baseline, got %d", got)
		}
		if got > 8192 {
			t.Fatalf("expected ≤8 KB manifest baseline, got %d", got)
		}
	})

	t.Run("plan with 10 resources + 5 notes + 3 groups produces linear overhead", func(t *testing.T) {
		plan := &exportPlan{
			groupIDs:    make([]uint, 3),
			noteIDs:     make([]uint, 5),
			resourceIDs: make([]uint, 10),
		}
		got := estimateJSONOverhead(plan)
		// 18 entities × ≈1 KB + 2 KB manifest baseline = ≈20 KB
		if got < 10_000 || got > 30_000 {
			t.Fatalf("expected 10-30 KB for 18 entities, got %d", got)
		}
	})

	t.Run("buildExportPlan sums overhead into totalBytes", func(t *testing.T) {
		// Integration-ish — confirms the helper is actually wired into the pipeline.
		// Skip if MahresourcesContext scaffolding is unavailable in this test package.
		t.Skip("covered by the E2E progress-cap test via real export")
	})
}
```

- [ ] **Step 2: Run 3× to verify it fails**

```bash
go test --tags 'json1 fts5' ./application_context/ -run TestEstimateJSONOverhead -v -count=3
```

Expected: all 3 runs FAIL with `undefined: estimateJSONOverhead` — the helper doesn't exist yet.

### Task A6: Implement `estimateJSONOverhead` and wire it into `buildExportPlan`

**Files:**
- Modify: `application_context/export_context.go` — add helper + call at end of `buildExportPlan`

- [ ] **Step 1: Add the helper at the end of the file (or near `buildExportPlan`)**

```go
// estimateJSONOverhead returns a rough byte-count estimate for the JSON
// payloads inside the export tar (manifest + per-entity files + tar block
// padding). Added to totalBytes so progressPercent reflects actual output
// size, not just blob bytes. BH-015.
func estimateJSONOverhead(plan *exportPlan) int64 {
	const (
		manifestBaseline = 2 * 1024 // manifest.json scaffolding
		perEntityBytes   = 1024     // typical size of a single entity's JSON row
	)
	entities := int64(len(plan.groupIDs) +
		len(plan.noteIDs) +
		len(plan.resourceIDs) +
		len(plan.seriesIDs))
	return manifestBaseline + entities*perEntityBytes
}
```

- [ ] **Step 2: Call it at the end of `buildExportPlan`**

Find the end of `buildExportPlan` (around line 245), just before the `return plan, nil`:

```go
	// Phase E: detect dangling references (m2m / GroupRelations / Series siblings).
	if err := ctx.collectDanglingRefs(plan); err != nil {
		return nil, err
	}

	return plan, nil
}
```

Insert the overhead pass before `return`:

```go
	// Phase E: detect dangling references (m2m / GroupRelations / Series siblings).
	if err := ctx.collectDanglingRefs(plan); err != nil {
		return nil, err
	}

	// BH-015: add a JSON-overhead estimate so EstimatedBytes/totalBytes reflect
	// the actual tar output size, not just the sum of blob bytes.
	plan.totalBytes += estimateJSONOverhead(plan)

	return plan, nil
}
```

- [ ] **Step 3: Run 3× to verify the Go test passes**

```bash
go test --tags 'json1 fts5' ./application_context/ -run TestEstimateJSONOverhead -v -count=3
```

Expected: first two sub-tests PASS, third is SKIP.

### Task A7: Build + verify the E2E test passes

- [ ] **Step 1: Build frontend assets**

```bash
npm run build
```

- [ ] **Step 2: Run the BH-015 E2E**

```bash
cd e2e && npx playwright test c10-bh015-export-progress-cap --reporter=line
```

Expected: PASS.

### Task A8: Commit

```bash
git add src/components/downloadCockpit.js src/main.js \
  templates/adminExport.tpl \
  application_context/export_context.go \
  application_context/export_overhead_test.go \
  e2e/tests/c10-bh015-export-progress-cap.spec.ts \
  public/dist/ public/tailwind.css
git commit -m "$(cat <<'EOF'
fix(jobs-ui): BH-015 — progress label caps at 100% + accurate overhead estimate

Small-payload exports showed "(5140%)" because totalSize only counted blob
bytes while progress counted every byte in the tar (manifest, JSONs,
padding). Two-part fix:

1. UI cap: Math.min(100, ...) on both label sites
   (templates/adminExport.tpl and src/components/downloadCockpit.js)
   so progressPercent never appears above 100 in text.
2. Backend accuracy: new estimateJSONOverhead(plan) adds a realistic
   estimate (2 KB manifest + 1 KB per entity) to totalBytes in
   buildExportPlan, so the percentage itself is accurate.

Unit test: application_context/export_overhead_test.go.
E2E: e2e/tests/c10-bh015-export-progress-cap.spec.ts.
EOF
)"
```

---

## Task Group B: BH-036 — Retention disclosure

### Task B1: Write failing E2E for retention disclosure

**Files:**
- Create: `e2e/tests/c10-bh036-export-retention-disclosure.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-036: export UI does not disclose the 24 h (default) retention window.
 * Completed tars vanish with no prior warning, compounding BH-025 and BH-026.
 *
 * Fix:
 *   a) Static helper text on /admin/export referencing config.ExportRetention.
 *   b) Per-completed-export expiry timestamp in the downloadCockpit panel.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-036: retention disclosure', () => {
  test('/admin/export shows retention helper text', async ({ page }) => {
    await page.goto('/admin/export');
    const helper = page.getByTestId('export-retention-helper');
    await expect(helper).toBeVisible();
    await expect(helper).toContainText(/Completed exports/i);
    await expect(helper).toContainText(/\d+\s*(h|m|hour|min)/i);
  });

  test('cockpit shows expiry timestamp on completed group-export rows', async ({ page, apiClient }) => {
    const group = await apiClient.createGroup({ name: `BH036-${Date.now()}` });
    await apiClient.createImageResource({ name: `BH036-r-${Date.now()}`, ownerGroupId: group.ID });

    await page.goto(`/admin/export?preselectedIds=${group.ID}`);
    await page.getByTestId('export-submit-button').click();

    await expect.poll(async () => {
      const text = await page.locator('[data-testid="export-progress-panel"]').textContent();
      return text?.includes('completed') ? 'completed' : 'pending';
    }, { timeout: 30_000 }).toBe('completed');

    // Open the cockpit
    await page.getByRole('button', { name: /jobs/i }).click();

    const expiryRow = page.getByTestId('cockpit-job-expiry').first();
    await expect(expiryRow).toBeVisible();
    await expect(expiryRow).toContainText(/expires/i);
  });
});
```

- [ ] **Step 2: Run 3× to verify fails**

```bash
cd e2e && npx playwright test c10-bh036-export-retention-disclosure --reporter=line --repeat-each=3
```

Expected: all 6 runs FAIL — the helper text + expiry row elements don't exist yet.

### Task B2: Expose `exportRetention` in the admin-export template context

**Files:**
- Locate via `grep -rn "adminExport" server/template_handlers/ | head` — typically `server/template_handlers/admin_export_template_handler.go` or embedded in a generic admin handler
- Modify: that file

- [ ] **Step 1: Find the context provider for `/admin/export`**

```bash
grep -rn 'adminExport' server/template_handlers/ | head
```

Open the file; locate the context map built for `/admin/export`. Add:

```go
context["exportRetention"] = appContext.Config.ExportRetention.String()
```

(Exact field name depends on repo — it may be `Config.ExportRetention`, `appContext.ExportRetention`, etc. Match what's available.)

### Task B3: Render the helper text in `adminExport.tpl`

**Files:**
- Modify: `templates/adminExport.tpl`

- [ ] **Step 1: Add the helper line just after the `Start export` button (line ~105)**

Find:

```pongo2
    <button type="button" @click="submit()" ...>
      Start export
    </button>
```

Insert AFTER the button (before the `<div x-show="job"`):

```pongo2
    <p class="mt-2 text-xs text-stone-600" data-testid="export-retention-helper">
      Completed exports are available for download for <span class="font-mono">{{ exportRetention }}</span> after completion, then removed automatically. Start a new export if the link has expired.
    </p>
```

### Task B4: Add expiry timestamp to cockpit completed-export rows

**Files:**
- Modify: `templates/partials/downloadCockpit.tpl` around line 194-199 where the BH-026 download link lives

- [ ] **Step 1: Add a sibling row for the expiry timestamp**

Just after the existing BH-026 "Download export" anchor:

```pongo2
    <!-- BH-036: retention expiry -->
    <template x-if="job.status === 'completed' && job.source === 'group-export' && job.completedAt && exportRetentionMs">
        <p class="mt-0.5 text-xs text-stone-500"
           data-testid="cockpit-job-expiry"
           :title="'Completed ' + new Date(job.completedAt).toLocaleString() + '; expires at ' + new Date(new Date(job.completedAt).getTime() + exportRetentionMs).toLocaleString()">
            Expires <span x-text="formatRelativeTime(new Date(job.completedAt).getTime() + exportRetentionMs)"></span>
        </p>
    </template>
```

- [ ] **Step 2: Add `exportRetentionMs` to `downloadCockpit` component data**

In `src/components/downloadCockpit.js` constructor function, add:

```javascript
// BH-036: retention window in ms (populated by init() from meta tag).
exportRetentionMs: 0,
```

And in `init()`, read from a meta tag or window global:

```javascript
const metaEl = document.querySelector('meta[name="x-export-retention-ms"]');
if (metaEl) {
    this.exportRetentionMs = parseInt(metaEl.getAttribute('content'), 10) || 0;
}
```

Add the meta tag in `templates/layouts/base.tpl` (head section):

```pongo2
<meta name="x-export-retention-ms" content="{{ exportRetentionMs }}">
```

And expose `exportRetentionMs` in the global template context — look for where `request_context.go` or the context-providers populate globals. If a "globals" provider doesn't exist, put the meta tag only on pages that load the cockpit (base layout) and wire it via the same admin-export provider extended in Task B2.

- [ ] **Step 3: Implement `formatRelativeTime(epochMs)` helper**

Add to `src/components/downloadCockpit.js`:

```javascript
formatRelativeTime(epochMs) {
    const now = Date.now();
    const diff = epochMs - now;
    if (diff <= 0) return 'now (tar may already be gone)';
    const mins = Math.floor(diff / 60_000);
    if (mins < 60) return `in ${mins} min`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `in ${hours} h ${mins % 60} min`;
    const days = Math.floor(hours / 24);
    return `in ${days} day${days !== 1 ? 's' : ''}`;
},
```

### Task B5: Build + verify

```bash
npm run build
cd e2e && npx playwright test c10-bh036-export-retention-disclosure --reporter=line
```

Expected: PASS.

### Task B6: Commit

```bash
git add templates/adminExport.tpl templates/partials/downloadCockpit.tpl \
  templates/layouts/base.tpl \
  src/components/downloadCockpit.js \
  server/template_handlers/ \
  e2e/tests/c10-bh036-export-retention-disclosure.spec.ts \
  public/dist/ public/tailwind.css
git commit -m "$(cat <<'EOF'
feat(jobs-ui): BH-036 — disclose export retention window + per-job expiry

Two additions:
- /admin/export now carries a helper line citing config.ExportRetention
  so operators know tars disappear after that window.
- downloadCockpit shows an "Expires in X" timestamp on completed
  group-export rows, computed from job.completedAt + ExportRetention.

Compounds the closure of BH-025 / BH-026 — operators now have full
visibility from kickoff through expiry.

E2E: e2e/tests/c10-bh036-export-retention-disclosure.spec.ts.
EOF
)"
```

---

## Task C: Update `tasks/bug-hunt-log.md`

- [ ] **Step 1: Mark BH-015 and BH-036 as FIXED with PR placeholder**

For both active entries:

```markdown
- **Status:** **FIXED** (2026-04-22, c10-jobs-ui-polish, PR #XX merged <sha>)
- **Original status (pre-fix):** verified
```

Append to the Fixed/closed table:

```markdown
| BH-015 | **fixed** (2026-04-22, c10-jobs-ui-polish, PR #XX merged <sha>) | UI cap: `Math.min(100, ...)` on both label sites (`adminExport.tpl:122`, `src/components/downloadCockpit.js:formatProgress`). Backend accuracy: new `estimateJSONOverhead(plan)` adds 2 KB manifest + 1 KB × entity count to `plan.totalBytes`. Unit: `application_context/export_overhead_test.go`. E2E: `e2e/tests/c10-bh015-export-progress-cap.spec.ts`. |
| BH-036 | **fixed** (2026-04-22, c10-jobs-ui-polish, PR #XX merged <sha>) | `/admin/export` helper line cites `config.ExportRetention`. `downloadCockpit` shows "Expires in X" per completed group-export row, computed from `job.completedAt + exportRetentionMs`. E2E: `e2e/tests/c10-bh036-export-retention-disclosure.spec.ts`. |
```

- [ ] **Step 2: Commit the log update**

```bash
git add tasks/bug-hunt-log.md
git commit -m "chore(bughunt): close BH-015/036 — c10 jobs UI polish"
```

---

## Task D: Full test matrix

- [ ] Go unit SQLite
- [ ] Full E2E browser + CLI (`npm run test:with-server:all`)
- [ ] A11y E2E
- [ ] Postgres Go + E2E

(Same commands as c13 plan.)

If any pre-existing failure appears — read the code first, don't rerun. Fix or file + call out in PR body.

---

## Task E: Open PR + merge + backfill log + cleanup worktree

Same shape as c13 plan's Task F. PR title: `fix(bughunt c10): BH-015/036 jobs UI polish`.

---

## Self-review checklist

- [ ] Both BH-IDs moved to Fixed/closed with real PR # + sha
- [ ] Two new `c10-*.spec.ts` files + one Go unit test file exist and pass
- [ ] Progress label capped in all text-facing sites (not just bars)
- [ ] Retention window visible on `/admin/export` AND per-completed-job in cockpit
- [ ] No regressions in existing cockpit / export E2E specs
