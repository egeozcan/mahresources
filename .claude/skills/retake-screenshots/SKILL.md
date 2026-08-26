---
name: retake-screenshots
description: Retake all docs-site screenshots with richly seeded data. Builds the app, starts an ephemeral server, seeds realistic data, captures 30 screenshots via Playwright, and updates the manifest.
argument-hint: "[--skip-seed] [--only=dashboard,grid-view,...] [--port=8181]"
---

# Retake Docs-Site Screenshots

Captures all 30 screenshots for the docs-site with realistic, populated seed data. The process is fully automated: build, seed, screenshot, cleanup.

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
    """Create a gradient PNG with soft circles. No PIL needed.

    Accumulate into a bytearray. Writing this as `raw = b''` plus a per-pixel
    `raw += ...` is quadratic, because bytes is immutable and every append
    copies the whole buffer: a 1280x720 image then spends 15+ minutes doing
    memcpy instead of well under a second. Circle geometry is hoisted out of
    the pixel loop, and the distance test compares squared distances so
    sqrt runs only for pixels actually inside a circle.
    """
    rng = random.Random(seed_str)
    scale = min(width, height) / 100
    circles = [(rng.randint(0,100)*width/100, rng.randint(0,100)*height/100,
                rng.randint(10,30)*scale, rng.randint(20,60)/100) for _ in range(6)]
    raw = bytearray()
    for y in range(height):
        raw.append(0)  # PNG per-row filter byte: none
        ty = y/height*0.4
        for x in range(width):
            t = x/width*0.6 + ty
            r,g,b = [c1[i]+(c2[i]-c1[i])*t for i in range(3)]
            for cx,cy,radius,alpha in circles:
                dx, dy = x-cx, y-cy
                d2 = dx*dx + dy*dy
                if d2 < radius*radius:
                    blend = alpha*(1-math.sqrt(d2)/radius)
                    r,g,b = [v*(1-blend)+255*blend for v in (r,g,b)]
            raw += bytes((max(0,min(255,int(r))), max(0,min(255,int(g))), max(0,min(255,int(b)))))
    def chunk(t,d): c=t+d; return struct.pack('>I',len(d))+c+struct.pack('>I',zlib.crc32(c)&0xFFFFFFFF)
    with open(filepath,'wb') as f:
        f.write(b'\x89PNG\r\n\x1a\n')
        f.write(chunk(b'IHDR', struct.pack('>IIBBBBB',width,height,8,2,0,0,0)))
        f.write(chunk(b'IDAT', zlib.compress(bytes(raw),6)))
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

#### 3k. Duplicates and a Resource Reduction (4 screenshots)

The Resource Reduction shots need repeats, which the ordinary seed cannot produce:

**Byte-identical pair** — `POST /v1/resource` deduplicates on content hash at
create time and hands back the Resource that already exists, so uploading one
file twice gives one row. Create the twin from a *different* image, then make the
keeper's file its current version:

```bash
curl -X POST "$API_URL/resource/versions?resourceId=$TWIN" \
  -F "file=@$TMPDIR/sunset-golden-gate.png" -F "comment=Same shot, off the phone"
```

The version upload syncs `hash`, `file_size`, `width` and `height` onto the
resource row, so the two rows end up identical in everything the review shows.
Re-send the **local** file: `/v1/resource/view?id=N` answers a 302 to `/files/...`,
so a plain `curl -o` writes 93 bytes of redirect HTML and the version silently
becomes that.

**Near-identical pairs** — render one image and upload it twice, at full size and
as a genuine box-average **downscale** of the same pixels (not a redraw at another
size). pHash is scale-invariant, so the pair is near-certain, and the Cluster then
reads "highest resolution, by 4x the pixels". Give the images bands and hard-edged
blocks: pHash on a smooth gradient is degenerate and the ahash secondary check
suppresses it.

**Wait for the hash worker before computing.** The Near-Identical tier reads the
stored pair table and hashes nothing itself, so a Reduction computed too early
silently contains only the Identical tier. Start the server with
`-hash-poll-interval=2s` and poll `GET /v1/admin/data-stats/expensive` until
`.similarity.totalHashes` covers the uploads and `.similarity.similarPairsFound`
is at least the number of pairs seeded; `POST /v1/admin/similarity/recompute`
kicks it.

**Space the twin's creation** by ~8 seconds. Every earlier criterion ties on a
byte-identical pair, so the Cluster falls through to creation order, and a
sub-second gap reads as "by less than a second earlier".

Then create three Reductions so the list page shows more than one state:

```bash
curl -X POST "$API_URL/reduction" -H "Content-Type: application/json" \
  -d '{"name":"Photo library cleanup","resourceIds":[1,14],"groupIds":[3]}'
# then POST /v1/reduction/compute with {"id":N,"version":V}, where V comes from
# GET /v1/reductions — every write is optimistic-concurrency checked — and poll
# that same endpoint until status is "ready".
```

Leave one uncomputed ("Not computed yet") and give one `"matchingMode":"identical"`.

### Step 4: Take screenshots with Playwright

Write a temporary `e2e/take-screenshots.mjs` script and run it with `npx tsx`:

```javascript
import { chromium } from '@playwright/test';

const page = await context.newPage();
// viewport: { width: 1200, height: 800 }
// waitUntil: 'load' (NOT 'networkidle' — causes timeouts)
```

#### Screenshot inventory (30 here; the manifest also carries 5 schema-editor and timeline shots)

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
| 27 | reduction-overview.png | /reduction?id=1 | — |
| 28 | reduction-review.png | /reduction?id=1 | Scroll to the Clusters heading, clear of the 36px sticky `<header>` |
| 29 | reduction-list.png | /reductions | — |
| 30 | reduction-bulk-action.png | /resources | Click "Select All", click `bulk-reduction-action`, fill `bulk-reduction-name` |

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
- Confirm all 30 PNGs exist in `docs-site/static/img/`
- Confirm `screenshot-manifest.json` is valid JSON

## Gotchas

- **Resource create returns an ARRAY**: `POST /v1/resource` accepts many files per request and answers with a JSON array even for a single file. Read the id from `[0].ID`, not `.ID`, or every downstream step keyed on the new id (version uploads, cross-links) silently skips.
- **Share server port**: if `.env` sets `SHARE_PORT` and that port is already bound, the server exits at startup instead of degrading. Pass `-share-port=` to run without it.
- **Resource upload field**: Use `resource` not `file` for the multipart field name
- **Version upload field**: Use `file` not `resource` for version uploads
- **Version endpoint**: `POST /v1/resource/versions?resourceId=ID` (plural "versions")
- **Block content**: Must be JSON objects matching the block type schema (not plain strings)
- **Todo state**: Separate from content. PATCH `/v1/note/block/state?id=ID` with `{"state":{"checked":["item-id"]}}`
- **Relation types**: Default types (Address, Employer) have category constraints. Custom types with `fromCategoryId=0` may not persist due to an FTS trigger bug.
- **Page load**: Use `waitUntil: 'load'` in Playwright, NOT `'networkidle'` — the latter causes timeouts with the download queue polling.
- **Duplicate hash**: If two generated images happen to produce the same SHA1 hash, the upload silently returns empty. Ensure images have distinct color palettes.
- **Alpine wiring**: the reduction cluster controls carry a bare `disabled` until Alpine binds them. Wait for `[data-testid="cluster-checkbox"]:not([disabled])` or the shot is a page of greyed-out buttons.
- **macOS ships bash 3.2**: no `declare -A`. A seed script written in bash needs plain variables.
- **Selector syntax**: Don't use `:has-text()` pseudo-selectors inside `page.evaluate()` — they're Playwright-only, not valid CSS. Use `querySelectorAll` + JS filtering instead.
