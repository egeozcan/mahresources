---
name: retake-screenshots
description: Retake all docs-site screenshots with richly seeded data. Builds the app, starts an ephemeral server, seeds realistic data, captures 26 screenshots via Playwright, and updates the manifest.
argument-hint: "[--skip-seed] [--only=dashboard,grid-view,...] [--port=8181]"
---

# Retake Docs-Site Screenshots

Captures all 26 screenshots for the docs-site with realistic, populated seed data. The process is fully automated: build, seed, screenshot, cleanup.

## Usage

```
/retake-screenshots
/retake-screenshots --only=dashboard,note-blocks,group-tree
/retake-screenshots --skip-seed   # reuse already-running seeded server
```

## Instructions

### Step 1: Build the application

```bash
npm run build
```

### Step 2: Start an ephemeral server

Find an available port (default 8181) and start the server:

```bash
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2 &
```

Wait until `curl -s http://localhost:8181 > /dev/null` succeeds.

Skip this step if the user passed `--skip-seed` and a server is already running.

### Step 3: Seed realistic data

Skip this step if the user passed `--skip-seed`.

Seed data via the API in this exact order (dependencies matter). Use `http://localhost:$PORT/v1` as base URL.

#### 3a. Categories (add to defaults: Person=1, Location=2, Business=3)

Create: Project, Media, Document (IDs will be 4, 5, 6).

```bash
curl -s -X POST "$API_URL/category" -d "name=Project&Description=Active projects and initiatives"
curl -s -X POST "$API_URL/category" -d "name=Media&Description=Photos, videos, and media assets"
curl -s -X POST "$API_URL/category" -d "name=Document&Description=Documents, specs, and written materials"
```

#### 3b. Tags (12)

Create: favorite, reference, draft, reviewed, important, archived, landscape, portrait, tutorial, in-progress, published, needs-review — each with a short Description.

#### 3c. Note Types (4)

Create: Meeting Notes, Technical Spec, Research Notes, Journal Entry via `POST /v1/note/noteType`.

#### 3d. Groups (12) with parent-child hierarchy

Root groups (5):
- Photography (categoryId=5/Media)
- Research Papers (categoryId=6/Document)
- Travel 2025 (categoryId=2/Location)
- Work Projects (categoryId=4/Project)
- Family (categoryId=1/Person)

Child groups (7):
- Landscapes, Portraits, Street Photography → ownerId=Photography
- Machine Learning, Computer Vision → ownerId=Research Papers
- Web App Redesign, API Migration → ownerId=Work Projects

Assign tags to groups for richer display.

#### 3e. Resources (13+) with generated images

Generate PNG images using pure Python (struct + zlib, no PIL needed):

```python
import struct, zlib, math, random

def create_png(filepath, width, height, c1, c2, seed_str):
    """Create gradient PNG with semi-transparent circles."""
    rng = random.Random(seed_str)
    circles = [(rng.randint(0,100), rng.randint(0,100), rng.randint(10,30), rng.randint(20,60)) for _ in range(6)]
    raw = b''
    for y in range(height):
        raw += b'\x00'
        for x in range(width):
            t = x/width*0.6 + y/height*0.4
            r,g,b = [int(c1[i]+(c2[i]-c1[i])*t) for i in range(3)]
            for cx_p,cy_p,rad_p,alpha in circles:
                cx,cy = cx_p*width/100, cy_p*height/100
                radius = rad_p*min(width,height)/100
                dist = math.sqrt((x-cx)**2+(y-cy)**2)
                if dist < radius:
                    blend = (alpha/100)*(1-dist/radius)
                    r,g,b = [int(v*(1-blend)+255*blend) for v in (r,g,b)]
            raw += struct.pack('BBB', max(0,min(255,r)), max(0,min(255,g)), max(0,min(255,b)))
    def chunk(t,d): c=t+d; return struct.pack('>I',len(d))+c+struct.pack('>I',zlib.crc32(c)&0xFFFFFFFF)
    with open(filepath,'wb') as f:
        f.write(b'\x89PNG\r\n\x1a\n')
        f.write(chunk(b'IHDR', struct.pack('>IIBBBBB',width,height,8,2,0,0,0)))
        f.write(chunk(b'IDAT', zlib.compress(raw,9)))
        f.write(chunk(b'IEND', b''))
```

Generate 15 images with different color palettes and sizes (800x600, 1024x768, 1280x720, etc.). Use realistic filenames like `sunset-golden-gate.png`, `architecture-diagram-v2.png`, `ml-training-results.png`.

**IMPORTANT**: The upload form field name is `resource` (not `file`):
```bash
curl -s -X POST "$API_URL/resource" \
  -F "resource=@$TMPDIR/sunset-golden-gate.png" \
  -F "name=Sunset at Golden Gate Bridge" \
  -F "Description=Golden hour shot from Battery Spencer viewpoint" \
  -F "ownerId=6" -F "tags=1" -F "tags=7" \
  -H "Accept: application/json"
```

#### 3f. Notes (8) with blocks

Create notes via `POST /v1/note` with Name, Description, ownerId, noteTypeId, tags.

Then add blocks via `POST /v1/note/block` with JSON body. Block content schemas:
- **heading**: `{"NoteID":1,"Type":"heading","Position":"a","Content":{"text":"...","level":2}}`
- **text**: `{"NoteID":1,"Type":"text","Position":"b","Content":{"text":"..."}}`
- **todos**: `{"NoteID":1,"Type":"todos","Position":"c","Content":{"items":[{"id":"t1","label":"..."},{"id":"t2","label":"..."}]}}`

To check a todo item, PATCH the block state:
```bash
curl -s -X PATCH "$API_URL/note/block/state?id=$BLOCK_ID" \
  -H "Content-Type: application/json" \
  -d '{"state":{"checked":["t1"]}}'
```

**Required**: Note 1 MUST have heading + text + todos blocks with at least 1 checked item (this is the note-blocks screenshot).

#### 3g. Resource Versions (3 for resource 1)

Upload 2 additional versions for the first resource:
```bash
curl -s -X POST "$API_URL/resource/versions?resourceId=1" \
  -F "file=@$TMPDIR/variant-v2.png" \
  -F "comment=Updated with warmer color grading"
```

The version upload field IS `file` (not `resource`), and the endpoint is `/v1/resource/versions?resourceId=ID`.

#### 3h. Relations

Create at least 1 relation using the default Address type (Person→Location):
```bash
curl -s -X POST "$API_URL/relation" \
  -H "Content-Type: application/json" \
  -d '{"FromGroupId":$FAMILY_ID,"ToGroupId":$TRAVEL_ID,"GroupRelationTypeId":1}'
```

#### 3i. Queries (3)

Create via `POST /v1/query` with `--data-urlencode "name=..."` and `--data-urlencode "Text=SELECT ..."`.

#### 3j. Cross-links

- Add resources to groups: `POST /v1/resources/addGroups` with `ID=<resource_id>&EditedId=<group_id>`
- Add groups to notes: `POST /v1/notes/addGroups` with `ID=<note_id>&EditedId=<group_id>`

### Step 4: Take screenshots with Playwright

Write a temporary `e2e/take-screenshots.mjs` script and run it with `npx tsx`:

```javascript
import { chromium } from '@playwright/test';

const page = await context.newPage();
// viewport: { width: 1200, height: 800 }
// waitUntil: 'load' (NOT 'networkidle' — causes timeouts)
```

#### Screenshot inventory (26 total)

| # | File | URL | Interactions |
|---|------|-----|-------------|
| 1 | dashboard.png | /dashboard | — |
| 2 | grid-view.png | /resources | — |
| 3 | resource-detail.png | /resource?id=1 | — |
| 4 | resource-detail-view.png | /resources/details | — |
| 5 | upload-form.png | /resource/new | — |
| 6 | resource-versions.png | /resource?id=1 | Scroll to versions heading |
| 7 | version-compare.png | /resource/compare?r1=1&v1=1&r2=1&v2=3 | — |
| 8 | note-list.png | /notes | — |
| 9 | note-blocks.png | /note?id=1 | — |
| 10 | note-edit.png | /note/edit?id=1 | — |
| 11 | note-sharing.png | /note?id=1 | — |
| 12 | group-tree.png | /group/tree | Try expanding tree nodes |
| 13 | group-list.png | /groups | — |
| 14 | group-detail.png | /group?id=1 | — |
| 15 | group-edit.png | /group/edit?id=1 | — |
| 16 | tag-list.png | /tags | — |
| 17 | category-list.png | /categories | — |
| 18 | search-results.png | /resources?name=sunset | — |
| 19 | global-search.png | /resources | Press Meta+k, type "arch", wait 800ms |
| 20 | bulk-selection.png | /resources | Click "Select All" button |
| 21 | query-editor.png | /query?id=1 | — |
| 22 | activity-log.png | /logs | — |
| 23 | plugin-management.png | /plugins/manage | — |
| 24 | download-queue.png | /dashboard | — |
| 25 | relation-list.png | /relations | — |
| 26 | relation-types.png | /relationTypes | — |

All screenshots go to `docs-site/static/img/`.

If `--only=name1,name2` was passed, only take those screenshots.

### Step 5: Update screenshot-manifest.json

Update `docs-site/static/img/screenshot-manifest.json` with the current date for all retaken screenshots. Keep the existing structure — update `capturedDate` and `seedDetails`/`description` if the content changed.

### Step 6: Cleanup

- Delete the temporary `e2e/take-screenshots.mjs` script
- Kill the ephemeral server: `kill $(lsof -ti :$PORT)`
- Remove temp image directory

### Step 7: Verify

- Run `cd docs-site && npm run build` to verify all image references resolve
- Confirm all 26 PNGs exist in `docs-site/static/img/`
- Confirm `screenshot-manifest.json` is valid JSON

## Gotchas

- **Resource upload field**: Use `resource` not `file` for the multipart field name
- **Version upload field**: Use `file` not `resource` for version uploads
- **Version endpoint**: `POST /v1/resource/versions?resourceId=ID` (plural "versions")
- **Block content**: Must be JSON objects matching the block type schema (not plain strings)
- **Todo state**: Separate from content. PATCH `/v1/note/block/state?id=ID` with `{"state":{"checked":["item-id"]}}`
- **Relation types**: Default types (Address, Employer) have category constraints. Custom types with `fromCategoryId=0` may not persist due to an FTS trigger bug.
- **Page load**: Use `waitUntil: 'load'` in Playwright, NOT `'networkidle'` — the latter causes timeouts with the download queue polling.
- **Duplicate hash**: If two generated images happen to produce the same SHA1 hash, the upload silently returns empty. Ensure images have distinct color palettes.
- **Selector syntax**: Don't use `:has-text()` pseudo-selectors inside `page.evaluate()` — they're Playwright-only, not valid CSS. Use `querySelectorAll` + JS filtering instead.
