# Documentation Perfection Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make all docs under `docs-site/` perfect and up-to-date with zero AI-slop, full accuracy, contextual screenshots, and complete feature coverage.

**Architecture:** Parallel agent team with phased execution. Summarizers produce ground truth reports, checkers cross-reference docs against reports, writing coach applies fixes, screenshot agent captures contextual images via Playwright. Conductor coordinates and does final review.

**Tech Stack:** Subagents for parallelism, Playwright for screenshots, existing `e2e/helpers/api-client.ts` patterns for seeding, ephemeral server for screenshot capture.

---

### Task 1: Build App and Start Ephemeral Server

**Files:**
- Read: `e2e/scripts/run-tests.js` (reference for server startup pattern)

**Step 1: Build the application**

```bash
cd /Users/egecan/Code/mahresources && npm run build
```

Expected: Build succeeds, `mahresources` binary created.

**Step 2: Find available port and start ephemeral server**

```bash
# Find a free port, start server in background
cd /Users/egecan/Code/mahresources
./mahresources -ephemeral -bind-address=:8282 -max-db-connections=2 -hash-worker-disabled &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"
```

Expected: Server starts on port 8282 (or next available).

**Step 3: Verify server is responding**

```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:8282/
```

Expected: `200`

---

### Task 2: Dispatch Summarizer Agents (Parallel)

Run two Explore subagents simultaneously. They produce reports only — no doc edits.

**Step 1: Dispatch Technical Summarizer A — Entities & CRUD**

Launch subagent with prompt:

```
You are Technical Summarizer A. Your job is to produce a GROUND TRUTH REPORT of all entity-related features in mahresources by reading the actual source code. Do NOT read or modify any docs.

Read these code files and produce a structured report:

ENTITIES TO COVER: Resources, Notes, Groups, Tags, Categories, ResourceCategories, Series, Relations/RelationTypes, Queries, NoteTypes

FOR EACH ENTITY, DOCUMENT:
1. All model fields (from models/*_model.go)
2. All CRUD operations (from application_context/*_context.go, *_crud_context.go)
3. All API endpoints — method, path, query params, body format (from server/routes.go, server/routes_openapi.go, server/api_handlers/)
4. All template pages — what they show (from templates/)
5. All query/filter parameters (from models/query_models/)
6. Bulk operations available
7. Relationships to other entities (many-to-many, ownership)

Key files to read:
- server/routes.go and server/routes_openapi.go for ALL route registrations
- models/ for all entity models
- application_context/ for business logic
- models/query_models/ for filter/query DTOs

OUTPUT FORMAT: Structured markdown report with one section per entity. Include exact field names, endpoint paths, and parameter names. This will be compared against documentation for accuracy checking.
```

**Step 2: Dispatch Technical Summarizer B — Advanced Features & Plugins**

Launch subagent with prompt:

```
You are Technical Summarizer B. Your job is to produce a GROUND TRUTH REPORT of all advanced features in mahresources by reading the actual source code. Do NOT read or modify any docs.

FEATURES TO COVER:
1. Resource Versioning — version_context.go, version model, compare endpoints, UI
2. Image Similarity — perceptual hashing, similarity detection, hash worker config
3. Note Block System — block types, block API, block editor (src/components/blocks/)
4. Search — FTS, global search, search caching
5. Bulk Operations — what's available per entity type
6. Download Queue / Job System — download cockpit, job states, SSE events
7. Note Sharing — share tokens, public server, share endpoints
8. Plugin System — plugin discovery, lifecycle, hooks, actions, pages, JSON API, KV store, block types, settings, Lua API (read plugins/ directory structure and server/plugin*.go files)
9. Activity Log — what gets logged, log model, log endpoints
10. Thumbnail Generation — supported formats, ffmpeg/libreoffice integration, thumbnail worker
11. Custom Templates — pongo2 template system, custom headers/sidebars/summaries/avatars
12. Meta Schemas — JSON schema validation for metadata
13. Entity Picker — how it works, what entities it supports
14. Dashboard — what data it shows
15. Configuration — ALL flags and env vars (from main.go or context.go initialization)

Key files to read:
- application_context/ for feature implementations
- server/routes.go and server/routes_openapi.go for endpoints
- src/components/ for frontend feature implementations
- plugins/ directory for plugin system
- main.go for configuration parsing

OUTPUT FORMAT: Structured markdown report with one section per feature. Include exact config flag names, endpoint paths, and behavioral details. This will be compared against documentation for accuracy.
```

Expected: Both agents return comprehensive ground truth reports.

---

### Task 3: Seed Content into Ephemeral Server

While summarizers run, seed the ephemeral instance with realistic content for screenshots.

**Step 1: Create test image files for seeding**

Use existing test assets from `e2e/test-assets/`. The directory has 35 sample PNG images and a text document.

**Step 2: Seed via API calls**

Use `curl` or a script to create content. The API uses form-encoded POST. Order matters (categories before groups, tags before resources):

```bash
BASE=http://localhost:8282

# 1. Categories
curl -s -X POST "$BASE/v1/category" -d "name=Media&Description=Photos, videos, and other media files"
curl -s -X POST "$BASE/v1/category" -d "name=Documents&Description=Text documents, PDFs, and spreadsheets"

# 2. Resource Categories
curl -s -X POST "$BASE/v1/resourceCategory" -d "name=Photographs&Description=Camera photos and digital images"
curl -s -X POST "$BASE/v1/resourceCategory" -d "name=Screenshots&Description=Screen captures and UI mockups"

# 3. Tags
curl -s -X POST "$BASE/v1/tag" -d "name=landscape&Description=Nature and outdoor scenery"
curl -s -X POST "$BASE/v1/tag" -d "name=portrait&Description=People and faces"
curl -s -X POST "$BASE/v1/tag" -d "name=draft&Description=Work in progress"
curl -s -X POST "$BASE/v1/tag" -d "name=reviewed&Description=Has been reviewed"
curl -s -X POST "$BASE/v1/tag" -d "name=important&Description=High priority items"
curl -s -X POST "$BASE/v1/tag" -d "name=archived&Description=No longer active"

# 4. Note Types
curl -s -X POST "$BASE/v1/note/noteType" -d "name=Meeting Notes&Description=Notes from meetings and discussions"
curl -s -X POST "$BASE/v1/note/noteType" -d "name=Research&Description=Research notes and findings"

# 5. Groups (need category IDs — use 1 and 2 from above)
curl -s -X POST "$BASE/v1/group" -d "name=Photography Projects&Description=All photography-related work&categoryId=1"
curl -s -X POST "$BASE/v1/group" -d "name=Landscapes&Description=Landscape photography collection&categoryId=1&ownerId=1"
curl -s -X POST "$BASE/v1/group" -d "name=Research Papers&Description=Academic papers and references&categoryId=2"
curl -s -X POST "$BASE/v1/group" -d "name=Travel 2025&Description=Photos and notes from 2025 trips&categoryId=1"

# 6. Relation Types
curl -s -X POST "$BASE/v1/relationType" -d "name=Related To&Description=General relationship between groups"

# 7. Relations (connect Photography Projects and Travel 2025)
curl -s -X POST "$BASE/v1/relation" -H "Content-Type: application/json" -d '{"Name":"Shared subjects","FromGroupId":1,"ToGroupId":4,"GroupRelationTypeId":1}'

# 8. Resources (upload images with tags and groups)
for i in 1 2 3 4 5 6 7 8; do
  curl -s -X POST "$BASE/v1/resource" \
    -F "resource=@e2e/test-assets/sample-image-$i.png" \
    -F "Name=Sample Image $i" \
    -F "Description=A sample image for documentation screenshots" \
    -F "OwnerId=1"
done

# Upload text document
curl -s -X POST "$BASE/v1/resource" \
  -F "resource=@e2e/test-assets/sample-document.txt" \
  -F "Name=Project Notes Document" \
  -F "Description=Text document with project notes"

# 9. Add tags to resources
curl -s -X POST "$BASE/v1/resources/addTags" -d "ID=1&ID=2&ID=3&EditedId=1&EditedId=5"
curl -s -X POST "$BASE/v1/resources/addTags" -d "ID=4&ID=5&EditedId=2&EditedId=4"
curl -s -X POST "$BASE/v1/resources/addTags" -d "ID=6&ID=7&ID=8&EditedId=3"

# 10. Add resources to groups
curl -s -X POST "$BASE/v1/resources/addGroups" -d "ID=1&ID=2&ID=3&EditedId=1"
curl -s -X POST "$BASE/v1/resources/addGroups" -d "ID=1&ID=2&EditedId=2"
curl -s -X POST "$BASE/v1/resources/addGroups" -d "ID=4&ID=5&EditedId=4"

# 11. Notes (with groups and tags)
curl -s -X POST "$BASE/v1/note" -d "Name=Weekly Team Standup&Description=Notes from the weekly standup meeting&NoteTypeId=1&groups=1&tags=4&tags=5"
curl -s -X POST "$BASE/v1/note" -d "Name=Landscape Photography Tips&Description=Collection of tips and techniques for landscape photography&NoteTypeId=2&groups=2&tags=1&Resources=1&Resources=2"
curl -s -X POST "$BASE/v1/note" -d "Name=Travel Planning&Description=Planning notes for upcoming trips&groups=4&tags=3"

# 12. Note Blocks (add blocks to first note)
curl -s -X POST "$BASE/v1/note/block" -H "Content-Type: application/json" -d '{
  "noteId": 1, "type": "heading", "position": "a",
  "content": {"text": "## Weekly Standup - March 7, 2026", "level": 2}
}'
curl -s -X POST "$BASE/v1/note/block" -H "Content-Type: application/json" -d '{
  "noteId": 1, "type": "text", "position": "b",
  "content": {"text": "Discussed project milestones and upcoming deadlines. Team agreed on priorities for next sprint."}
}'
curl -s -X POST "$BASE/v1/note/block" -H "Content-Type: application/json" -d '{
  "noteId": 1, "type": "todos", "position": "c",
  "content": {"items": [
    {"text": "Update documentation screenshots", "done": true},
    {"text": "Review API endpoint coverage", "done": false},
    {"text": "Fix image similarity thresholds", "done": false},
    {"text": "Deploy staging environment", "done": true}
  ]}
}'

# 13. Saved Query
curl -s -X POST "$BASE/v1/query" -d "name=Recent Large Resources&Text=SELECT r.id, r.name, r.content_type, r.file_size FROM resources r WHERE r.file_size > 1000 ORDER BY r.created_at DESC LIMIT 20&Description=Find recently added resources larger than 1KB"

# 14. Create a series (upload 3 images with same series slug)
for i in 10 11 12; do
  curl -s -X POST "$BASE/v1/resource" \
    -F "resource=@e2e/test-assets/sample-image-$i.png" \
    -F "Name=Sunset Series $i" \
    -F "Description=Part of the sunset photography series" \
    -F "SeriesSlug=sunset-series" \
    -F "OwnerId=2"
done
```

Expected: Ephemeral instance populated with realistic content visible on all pages.

**Step 3: Verify seeding worked**

```bash
curl -s http://localhost:8282/v1/resources | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'{len(d)} resources')"
curl -s http://localhost:8282/v1/groups | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'{len(d)} groups')"
curl -s http://localhost:8282/v1/notes | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'{len(d)} notes')"
curl -s http://localhost:8282/v1/tags | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'{len(d)} tags')"
```

Expected: 12 resources, 4 groups, 3 notes, 6 tags (approximately).

---

### Task 4: Dispatch Doc Checker Agents (Parallel, after Summarizers complete)

Wait for both summarizer reports from Task 2. Then dispatch two checker agents simultaneously.

**Step 1: Dispatch Doc Checker A — Concepts, Getting Started, User Guide**

Launch subagent with prompt (include the summarizer reports as context):

```
You are Doc Checker A. You have two ground truth reports about mahresources features (provided below). Your job is to compare the following documentation files against the ground truth and produce an ISSUE REPORT. Do NOT edit any docs.

GROUND TRUTH REPORTS:
[paste Summarizer A report]
[paste Summarizer B report]

FILES TO CHECK (read each one):
- docs-site/docs/intro.md
- docs-site/docs/getting-started/installation.md
- docs-site/docs/getting-started/quick-start.md
- docs-site/docs/getting-started/first-steps.md
- docs-site/docs/concepts/overview.md
- docs-site/docs/concepts/resources.md
- docs-site/docs/concepts/notes.md
- docs-site/docs/concepts/note-blocks.md
- docs-site/docs/concepts/groups.md
- docs-site/docs/concepts/tags-categories.md
- docs-site/docs/concepts/relationships.md
- docs-site/docs/concepts/series.md
- docs-site/docs/user-guide/navigation.md
- docs-site/docs/user-guide/managing-resources.md
- docs-site/docs/user-guide/managing-notes.md
- docs-site/docs/user-guide/organizing-with-groups.md
- docs-site/docs/user-guide/search.md
- docs-site/docs/user-guide/bulk-operations.md

FOR EACH FILE, REPORT:
1. INACCURATE — things that don't match the code (wrong field names, wrong endpoints, wrong behavior descriptions)
2. OUTDATED — features described that have changed or been removed
3. MISSING — features in ground truth that should be mentioned but aren't
4. AI-SLOP — specific phrases to remove/rewrite: "seamlessly", "leverages", "robust", "streamlined", "comprehensive", "effortlessly", "powerful", generic openings like "Create, edit, and organize...", filler sentences that restate the heading
5. STRUCTURAL — sections that need reordering, splitting, or merging

If a file is fine, say "NO ISSUES" for that file.

OUTPUT FORMAT: One section per file with categorized issues. Include line numbers where possible. Quote the problematic text and suggest specific fixes.
```

**Step 2: Dispatch Doc Checker B — Features, API, Config, Deployment**

Launch subagent with same format, checking:

```
FILES TO CHECK:
- docs-site/docs/features/versioning.md
- docs-site/docs/features/image-similarity.md
- docs-site/docs/features/saved-queries.md
- docs-site/docs/features/custom-templates.md
- docs-site/docs/features/meta-schemas.md
- docs-site/docs/features/note-sharing.md
- docs-site/docs/features/download-queue.md
- docs-site/docs/features/job-system.md
- docs-site/docs/features/activity-log.md
- docs-site/docs/features/thumbnail-generation.md
- docs-site/docs/features/custom-block-types.md
- docs-site/docs/features/entity-picker.md
- docs-site/docs/features/plugin-system.md
- docs-site/docs/features/plugin-actions.md
- docs-site/docs/features/plugin-hooks.md
- docs-site/docs/features/plugin-lua-api.md
- docs-site/docs/api/overview.md
- docs-site/docs/api/resources.md
- docs-site/docs/api/notes.md
- docs-site/docs/api/groups.md
- docs-site/docs/api/plugins.md
- docs-site/docs/api/other-endpoints.md
- docs-site/docs/configuration/overview.md
- docs-site/docs/configuration/database.md
- docs-site/docs/configuration/storage.md
- docs-site/docs/configuration/advanced.md
- docs-site/docs/deployment/docker.md
- docs-site/docs/deployment/systemd.md
- docs-site/docs/deployment/reverse-proxy.md
- docs-site/docs/deployment/public-sharing.md
- docs-site/docs/deployment/backups.md
- docs-site/docs/troubleshooting.md
```

Same output format as Checker A.

Expected: Both agents return detailed issue reports per file.

---

### Task 5: Capture Screenshots with Playwright (Parallel with Task 4)

While checkers run, capture screenshots from the seeded ephemeral instance.

**Step 1: Create screenshot capture script**

Create: `e2e/scripts/capture-docs-screenshots.js`

```javascript
const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

const BASE_URL = process.env.BASE_URL || 'http://localhost:8282';
const OUTPUT_DIR = path.resolve(__dirname, '../../docs-site/static/img');

async function main() {
  // Ensure output directory exists
  fs.mkdirSync(OUTPUT_DIR, { recursive: true });

  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 1200, height: 800 },
    colorScheme: 'light',
  });
  const page = await context.newPage();

  const screenshots = [
    {
      name: 'dashboard.png',
      url: '/dashboard',
      waitFor: '.container, main',
      description: 'Dashboard with populated data',
    },
    {
      name: 'grid-view.png',
      url: '/resources',
      waitFor: '.resource-card, .card, main',
      description: 'Resource grid view with images',
    },
    {
      name: 'resource-detail.png',
      url: '/resource?Id=1',
      waitFor: 'main',
      description: 'Resource detail page with tags and metadata',
    },
    {
      name: 'upload-form.png',
      url: '/resource/new',
      waitFor: 'form, main',
      description: 'Create resource form',
    },
    {
      name: 'note-blocks.png',
      url: '/note?Id=1',
      waitFor: 'main',
      description: 'Note with heading, text, and todo blocks',
    },
    {
      name: 'group-tree.png',
      url: '/group/tree?Id=1',
      waitFor: 'main',
      description: 'Hierarchical group tree view',
    },
    {
      name: 'group-detail.png',
      url: '/group?Id=1',
      waitFor: 'main',
      description: 'Group detail with owned resources and notes',
    },
    {
      name: 'search-results.png',
      url: '/resources?Name=Sample',
      waitFor: 'main',
      description: 'Filtered resource list (search results)',
    },
    {
      name: 'query-editor.png',
      url: '/query?Id=1',
      waitFor: 'main',
      description: 'Saved query with SQL editor',
    },
    {
      name: 'activity-log.png',
      url: '/logs',
      waitFor: 'main',
      description: 'Activity log entries',
    },
    {
      name: 'tag-list.png',
      url: '/tags',
      waitFor: 'main',
      description: 'Tags list page',
    },
    {
      name: 'note-list.png',
      url: '/notes',
      waitFor: 'main',
      description: 'Notes list page',
    },
    {
      name: 'group-list.png',
      url: '/groups',
      waitFor: 'main',
      description: 'Groups list page',
    },
  ];

  for (const shot of screenshots) {
    try {
      console.log(`Capturing ${shot.name}: ${shot.url}`);
      await page.goto(`${BASE_URL}${shot.url}`, { waitUntil: 'networkidle' });
      // Wait a beat for any Alpine.js rendering
      await page.waitForTimeout(500);
      try {
        await page.waitForSelector(shot.waitFor, { timeout: 5000 });
      } catch {
        console.log(`  Warning: selector "${shot.waitFor}" not found, capturing anyway`);
      }
      await page.screenshot({
        path: path.join(OUTPUT_DIR, shot.name),
        fullPage: false,
      });
      console.log(`  Saved ${shot.name}`);
    } catch (err) {
      console.error(`  Error capturing ${shot.name}: ${err.message}`);
    }
  }

  // Special screenshots that need interaction

  // Global search modal
  try {
    console.log('Capturing global-search.png: opening search modal');
    await page.goto(`${BASE_URL}/resources`, { waitUntil: 'networkidle' });
    await page.waitForTimeout(500);
    await page.keyboard.press('Meta+k');
    await page.waitForTimeout(500);
    // Type a search query
    const searchInput = page.locator('[x-model="query"], [type="search"], input[placeholder*="earch"]').first();
    if (await searchInput.isVisible({ timeout: 2000 })) {
      await searchInput.fill('Sample');
      await page.waitForTimeout(800);
    }
    await page.screenshot({
      path: path.join(OUTPUT_DIR, 'global-search.png'),
      fullPage: false,
    });
    console.log('  Saved global-search.png');
    // Close modal
    await page.keyboard.press('Escape');
  } catch (err) {
    console.error(`  Error capturing global-search.png: ${err.message}`);
  }

  // Bulk selection
  try {
    console.log('Capturing bulk-selection.png');
    await page.goto(`${BASE_URL}/resources`, { waitUntil: 'networkidle' });
    await page.waitForTimeout(500);
    // Try to select a few resource checkboxes
    const checkboxes = page.locator('input[type="checkbox"][name="ID"], input[type="checkbox"][data-bulk]');
    const count = await checkboxes.count();
    for (let i = 0; i < Math.min(3, count); i++) {
      await checkboxes.nth(i).check();
    }
    await page.waitForTimeout(300);
    await page.screenshot({
      path: path.join(OUTPUT_DIR, 'bulk-selection.png'),
      fullPage: false,
    });
    console.log('  Saved bulk-selection.png');
  } catch (err) {
    console.error(`  Error capturing bulk-selection.png: ${err.message}`);
  }

  await browser.close();
  console.log(`\nDone! Screenshots saved to ${OUTPUT_DIR}`);
}

main().catch(console.error);
```

**Step 2: Run the screenshot script**

```bash
cd /Users/egecan/Code/mahresources
BASE_URL=http://localhost:8282 npx --yes playwright install chromium 2>/dev/null
BASE_URL=http://localhost:8282 node e2e/scripts/capture-docs-screenshots.js
```

Expected: 15+ PNG files saved to `docs-site/static/img/`.

**Step 3: Verify screenshots captured**

```bash
ls -la docs-site/static/img/*.png
```

Expected: Multiple PNG files with reasonable sizes (>10KB each).

---

### Task 6: Dispatch Writing Coach (After Checkers complete)

Wait for both checker reports from Task 4. Compile all issues, then dispatch the Writing Coach.

**Step 1: Compile issue reports**

Merge the reports from Doc Checker A and Doc Checker B into a single consolidated issue list.

**Step 2: Dispatch Writing Coach agent**

Launch a general-purpose subagent that can EDIT files. Prompt:

```
You are the Writing Coach. Your job is to edit documentation files to fix issues found by doc checkers. You have the compiled issue reports below plus the screenshot files that were captured.

ISSUE REPORTS:
[paste consolidated checker reports]

AVAILABLE SCREENSHOTS (in docs-site/static/img/):
[list actual captured screenshot filenames]

YOUR RULES:
FIX:
- Inaccurate descriptions (doesn't match code behavior)
- Missing features (exist in code, not in docs)
- AI-slop phrases — remove/rewrite: "seamlessly", "leverages", "robust", "streamlined", "comprehensive", "effortlessly", "powerful", generic openings, filler sentences
- Vague descriptions — replace with specific, actionable instructions
- Add screenshot references where contextually appropriate

DON'T:
- Touch docs that have no issues
- Add boilerplate admonitions (tip/note/warning) unless genuinely helpful
- Restructure docs that work fine
- Change accurate terminology
- Add exclamation marks or enthusiasm

TONE:
- Use "you" for the reader
- Short sentences, lead with action
- Code examples over prose when possible
- State limitations directly

SCREENSHOT REFERENCES:
Add images to docs using this format (Docusaurus uses root-relative paths from static/):
![Description of what the screenshot shows](/img/filename.png)

Place screenshots near the text they illustrate. Don't clump them all at the top or bottom.

NEW DOC THRESHOLD:
If the issue reports identify an undocumented feature with its own UI page or 3+ API endpoints, create a new doc page for it. Otherwise fold the info into existing pages.

If you create new pages, note which sidebar entry they need so the conductor can update sidebars.ts.

WORK THROUGH EVERY FILE THAT HAS ISSUES. Edit each one. Skip files with no issues.
```

Expected: All doc files with issues are edited. Writing coach returns a summary of changes made and any new files created.

---

### Task 7: Final Review and Commit

**Step 1: Review all changes**

```bash
cd /Users/egecan/Code/mahresources
git diff --stat docs-site/
```

Review the diff for:
- Consistency in terminology across docs
- Screenshot references point to files that exist
- No new AI-slop introduced by the writing coach
- Cross-references between docs still work
- Sidebar entries match actual files

**Step 2: Update sidebars.ts if new doc pages were created**

Read: `docs-site/sidebars.ts`
If Writing Coach created new pages, add them to the appropriate sidebar category.

**Step 3: Verify docs site builds**

```bash
cd /Users/egecan/Code/mahresources/docs-site && npm run build
```

Expected: Build succeeds with no broken links or missing files.

**Step 4: Spot-check screenshot rendering**

```bash
cd /Users/egecan/Code/mahresources/docs-site && npx docusaurus serve --port 3030 &
```

Open a few pages that have screenshots and verify they render correctly.

**Step 5: Stop the ephemeral server**

```bash
kill $SERVER_PID 2>/dev/null
```

**Step 6: Clean up temporary files**

Delete `e2e/scripts/capture-docs-screenshots.js` (temporary, not needed after screenshots are captured).

**Step 7: Commit all changes**

```bash
cd /Users/egecan/Code/mahresources
git add docs-site/
git add -f docs-site/static/img/*.png
git commit -m "docs: comprehensive docs update — accuracy fixes, screenshots, de-slop

- Fix accuracy issues found by cross-referencing docs against source code
- Add contextual screenshots for key UI pages
- Remove AI-generated filler phrases and generic openings
- Fill documentation gaps for undocumented features
- Update configuration references to match current flags

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

Expected: Clean commit with all doc improvements.

---

## Execution Summary

| Phase | Tasks | Parallelism | Duration Estimate |
|-------|-------|-------------|-------------------|
| 1 | Build + Start Server, Dispatch Summarizers, Seed Content | Tasks 1-3 parallel | ~5 min |
| 2 | Dispatch Checkers, Capture Screenshots | Tasks 4-5 parallel | ~5 min |
| 3 | Writing Coach | Task 6 sequential | ~10 min |
| 4 | Final Review + Commit | Task 7 sequential | ~5 min |

Total: ~7 tasks, ~25 minutes with parallelism.
