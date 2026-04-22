# Cluster 14 — Ingestion Safety (BH-008, BH-034, BH-039)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Task groups A (BH-008 client-side crop), B (BH-034 upload size limits), and C (BH-039 narrow BH-011 over-rejection) touch distinct surfaces; parallel subagents safe. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Tighten upload safety without breaking valid uploads:
- BH-008: crop UI shows a clear "image could not be decoded" banner when `naturalW/H === 0`, disables the Crop button, and removes the invisible-overlay footgun.
- BH-034: add `MaxBytesReader` to resource + version upload paths; new `--max-upload-size` / `MAX_UPLOAD_SIZE` config, default 2 GB.
- BH-039: distinguish `image.ErrFormat` (SVG, ICO, WebP, AVIF, HEIC — Go stdlib doesn't decode these natively) from truncated/corrupt PNG/JPEG/GIF. The BH-011 fix is correct in intent; it just overshoots. Valid non-native-decodable images should be accepted and stored with `Width=0/Height=0`, not rejected with HTTP 400.

**Architecture:**
- **Group A (BH-008):** `src/components/imageCropper.js` gains a `decodeFailed` state. On `img.onerror` OR a load with `naturalWidth === 0`, flip the flag → render a non-dismissable banner, disable Crop. The invisible overlay (visibility hidden when `!naturalW/!naturalH`) is replaced by explicit banner.
- **Group B (BH-034):** New `Config.MaxUploadSize` (int64, bytes). New helper `tryFillStructValuesFromRequestWithLimit(dst, req, maxBytes)` in `server/api_handlers/api_handlers.go` that wraps `req.Body = http.MaxBytesReader(w, r.Body, maxBytes)` before delegating to the existing helper. Resource upload (`GetResourceUploadHandler`) and version upload (`GetResourceAddVersionHandler`) use the new helper. New flag `--max-upload-size` / `MAX_UPLOAD_SIZE` wired in main + CLAUDE.md.
- **Group C (BH-039):** Two-line change in `application_context/resource_upload_context.go:589-603`: `if errors.Is(decErr, image.ErrFormat) { preWidth, preHeight = 0, 0 } else { return &InvalidImageError{Cause: decErr} }`. Preserves the BH-011 regression guard (truncated PNG test still rejects). Un-skip `tests/13-lightbox.spec.ts › Lightbox SVG Support`.

**Tech Stack:** Go (config + http.MaxBytesReader + image decode), Alpine.js (crop UI), Playwright E2E.

**Worktree branch:** `bugfix/c14-ingestion-safety`

**Top-level spec:** `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 14 (scope expanded to include BH-039 at plan time — in-batch).

---

## File structure

**Modified:**
- `src/components/imageCropper.js:158-175` — decode-failed state + banner + Crop disable (BH-008)
- `application_context/context.go` — `MaxUploadSize int64` field on `Config`
- `cmd/mahresources/main.go` (or whatever main file does flag parsing) — register `--max-upload-size` flag
- `server/api_handlers/api_handlers.go:79-89` — new `tryFillStructValuesFromRequestWithLimit` helper (BH-034)
- `server/api_handlers/resource_api_handlers.go:128` — use new helper
- `server/api_handlers/version_api_handlers.go:86` — add `MaxBytesReader` before `ParseMultipartForm`
- `application_context/resource_upload_context.go:589-603` — `errors.Is(decErr, image.ErrFormat)` branch (BH-039)
- `tasks/bug-hunt-log.md` — move BH-008/034/039 to Fixed/closed
- `CLAUDE.md` — document `--max-upload-size` flag
- `e2e/tests/13-lightbox.spec.ts` — un-skip the SVG subtest (BH-039 follow-up)

**Created:**
- `server/api_tests/upload_size_limit_test.go` — BH-034 API test
- `server/api_tests/image_ingestion_accepts_svg_ico_webp_test.go` — BH-039 regression
- `e2e/tests/c14-bh008-crop-zero-dims-banner.spec.ts`

---

## Task 0: Worktree + baseline

- [ ] **Step 1: Worktree**

```bash
cd /Users/egecan/Code/mahresources
git worktree add -b bugfix/c14-ingestion-safety ../mahresources-c14 master
cd ../mahresources-c14
```

- [ ] **Step 2: Confirm baseline**

```bash
go test --tags 'json1 fts5' ./... -count=1
```

Expected: PASS.

---

## Task Group C: BH-039 — Narrow BH-011 over-rejection

(Done first because other tests depend on a working SVG lightbox and it's the smallest fix.)

### Task C1: Write failing API test

**Files:**
- Create: `server/api_tests/image_ingestion_accepts_svg_ico_webp_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api_tests

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"testing"
)

// BH-039: valid SVG/ICO/WebP/AVIF/HEIC uploads were rejected by the BH-011
// guard because Go's stdlib doesn't decode them natively. They should be
// accepted and stored with Width=0/Height=0 (same as the pre-BH-011 path).
// The truncated-PNG rejection must still work.
func TestImageIngestion_AcceptsSVG(t *testing.T) {
	tc := SetupTestEnv(t)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("resource", "logo.svg")
	fw.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><circle cx="16" cy="16" r="15" fill="red"/></svg>`))
	mw.Close()
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource", body,
		withHeader("Content-Type", mw.FormDataContentType()))
	assertStatus(t, resp, 200)
}

func TestImageIngestion_AcceptsICO(t *testing.T) {
	tc := SetupTestEnv(t)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("resource", "favicon.ico")
	// Minimal ICO header (6 bytes) + one 16x16 entry (16 bytes) + 16x16x4 bytes placeholder
	ico := append([]byte{0, 0, 1, 0, 1, 0}, bytes.Repeat([]byte{0}, 16+16*16*4)...)
	fw.Write(ico)
	mw.Close()
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource", body,
		withHeader("Content-Type", mw.FormDataContentType()))
	assertStatus(t, resp, 200)
}

// BH-011 regression must still fail on truncated PNG.
func TestImageIngestion_RejectsTruncatedPNG_StillWorks(t *testing.T) {
	tc := SetupTestEnv(t)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("resource", "broken.png")
	// PNG header only, no IDAT/IEND
	fw.Write([]byte("\x89PNG\r\n\x1a\n"))
	mw.Close()
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource", body,
		withHeader("Content-Type", mw.FormDataContentType()))
	assertStatus(t, resp, 400)
}
```

- [ ] **Step 2: Run 3× to verify the SVG/ICO tests fail**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestImageIngestion_Accepts -v -count=3
```

Expected: SVG + ICO tests FAIL with "uploaded file is not a valid image". Truncated PNG test PASSES (existing behavior).

### Task C2: Narrow the BH-011 guard to preserve valid non-decodable images

**Files:**
- Modify: `application_context/resource_upload_context.go:589-603`

- [ ] **Step 1: Add `errors.Is(decErr, image.ErrFormat)` branch**

Find:

```go
	if strings.HasPrefix(fileMime.String(), "image/") {
		if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		img, _, decErr := image.Decode(tempFile)
		if decErr != nil {
			return nil, &InvalidImageError{Cause: decErr}
		}
		bounds := img.Bounds()
		if bounds.Dx() == 0 || bounds.Dy() == 0 {
			return nil, &InvalidImageError{}
		}
		preWidth = bounds.Max.X
		preHeight = bounds.Max.Y
	}
```

Replace with:

```go
	if strings.HasPrefix(fileMime.String(), "image/") {
		if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		img, _, decErr := image.Decode(tempFile)
		if decErr != nil {
			// BH-039: Go's stdlib only decodes PNG/JPEG/GIF natively. SVG, ICO,
			// WebP, AVIF, HEIC and friends return image.ErrFormat — they're valid
			// images, we just can't extract dimensions here. Accept them and
			// store with Width=0/Height=0; the thumbnail pipeline uses ffmpeg/
			// libreoffice for these anyway.
			if errors.Is(decErr, image.ErrFormat) {
				preWidth = 0
				preHeight = 0
			} else {
				// BH-011: genuine decode failure (truncated PNG, etc.) — reject.
				return nil, &InvalidImageError{Cause: decErr}
			}
		} else {
			bounds := img.Bounds()
			if bounds.Dx() == 0 || bounds.Dy() == 0 {
				return nil, &InvalidImageError{}
			}
			preWidth = bounds.Max.X
			preHeight = bounds.Max.Y
		}
	}
```

Ensure `"errors"` is in the import block (it's probably already there — grep the file's imports).

- [ ] **Step 2: Run 3× to verify pass**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestImageIngestion -v -count=3
```

Expected: all 3 tests PASS × 3.

### Task C3: Un-skip the SVG lightbox E2E

**Files:**
- Modify: `e2e/tests/13-lightbox.spec.ts`

- [ ] **Step 1: Remove the `test.skip` on the SVG subtest**

Find the `test.skip(...` call related to "Lightbox SVG Support" and remove the `.skip` modifier, making it `test(...`. The test was skipped in PR #33 with a comment referencing BH-039. Update the comment to reference this cluster's PR.

### Task C4: Commit

```bash
git add application_context/resource_upload_context.go \
  server/api_tests/image_ingestion_accepts_svg_ico_webp_test.go \
  e2e/tests/13-lightbox.spec.ts
git commit -m "fix(ingestion): BH-039 — narrow BH-011 guard to only truncated decode errors

BH-011 rejected ANY image/* upload that failed image.Decode — but Go's
stdlib only decodes PNG/JPEG/GIF natively. SVG, ICO, WebP, AVIF, and
HEIC were all rejected with HTTP 400 even though they're valid images.

Distinguish image.ErrFormat from other decode errors: valid non-Go-
decodable images are accepted and stored with Width=0/Height=0
(same as pre-BH-011). Truncated PNG regression still rejected.

Also un-skip the Lightbox SVG E2E suite (was skipped in PR #33 pending
this fix).

API test: server/api_tests/image_ingestion_accepts_svg_ico_webp_test.go."
```

---

## Task Group A: BH-008 — Crop overlay zero-dims banner

### Task A1: Write failing E2E

**Files:**
- Create: `e2e/tests/c14-bh008-crop-zero-dims-banner.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-008: crop selection overlay invisible when image W=0/H=0.
 *
 * Scenario (post-c3): a resource with valid SVG content has Width=0/Height=0
 * (BH-039 change accepted SVG but can't extract dims). Crop button should
 * be disabled with a visible "cannot be decoded" banner — not an invisible
 * overlay that accepts click submission and returns a server error.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-008: crop overlay with zero dimensions', () => {
  test('SVG resource crop modal shows banner + disabled button', async ({ page, apiClient }) => {
    // Upload an SVG (valid, but Width=0/Height=0 post-BH-039)
    const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><circle cx="16" cy="16" r="15" fill="red"/></svg>';
    const r = await apiClient.createResourceFromBytes({ name: `BH008-svg-${Date.now()}`, bytes: Buffer.from(svg), filename: 'logo.svg' });

    await page.goto(`/resource?id=${r.ID}`);
    await page.getByRole('button', { name: /crop/i }).click();

    const banner = page.getByTestId('crop-decode-failed-banner');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText(/could not be decoded|not decodable/i);

    const cropBtn = page.getByTestId('crop-submit-button');
    await expect(cropBtn).toBeDisabled();
  });
});
```

- [ ] **Step 2: Run 3× to verify fails**

```bash
cd e2e && npx playwright test c14-bh008-crop-zero-dims-banner --reporter=line --repeat-each=3
```

Expected: FAIL — banner + test-ids don't exist yet.

### Task A2: Implement decode-failed state in `imageCropper.js`

**Files:**
- Modify: `src/components/imageCropper.js`

- [ ] **Step 1: Add state field + transitions**

In the component's return object (find existing state fields):

```javascript
// BH-008: decode-failed signal. Set true on img.onerror OR naturalWidth===0.
decodeFailed: false,
```

In the image-load handler (find `onImgLoad` or equivalent):

```javascript
onImgLoad(e) {
    const img = e.target;
    if (!img.naturalWidth || !img.naturalHeight) {
        this.decodeFailed = true;
        return;
    }
    this.decodeFailed = false;
    this.naturalW = img.naturalWidth;
    this.naturalH = img.naturalHeight;
    // ... existing logic ...
},

onImgError() {
    this.decodeFailed = true;
},
```

- [ ] **Step 2: Disable Crop button + render banner**

Add to the `submit()` method (guard):

```javascript
submit() {
    if (this.decodeFailed || !this.naturalW || !this.naturalH) return;
    // ... existing submit ...
},
```

In the component template (find the cropper template — `templates/partials/imageCropper.tpl` or inline):

```pongo2
<template x-if="decodeFailed">
    <div class="p-3 mb-3 bg-amber-50 border border-amber-200 rounded text-amber-800 text-sm"
         data-testid="crop-decode-failed-banner"
         role="status"
         aria-live="polite">
        This image could not be decoded in the browser; cropping is unavailable. Formats like SVG, ICO, and WebP may need to be re-uploaded as PNG/JPEG to crop.
    </div>
</template>
```

Change the existing Crop button:

```pongo2
<button type="submit"
        :disabled="decodeFailed || !hasSelection()"
        data-testid="crop-submit-button"
        ...existing-classes...>
    Crop
</button>
```

### Task A3: Build + run + commit

```bash
npm run build
cd e2e && npx playwright test c14-bh008-crop-zero-dims-banner --reporter=line
```

Expected: PASS.

```bash
git add src/components/imageCropper.js templates/ public/dist/ public/tailwind.css \
  e2e/tests/c14-bh008-crop-zero-dims-banner.spec.ts
git commit -m "fix(ui): BH-008 — crop modal surfaces 'cannot be decoded' banner on zero-dim images

Previously the crop-selection overlay was invisible when naturalWidth/
naturalHeight were 0 (SVG, ICO, truncated images pre-c3 ingestion tight-
ening). The Crop button stayed enabled and users could submit to a
server error.

imageCropper now tracks decodeFailed (set on img.onerror or naturalWidth
=== 0). When true: a non-dismissable banner explains the situation and
the Crop button is disabled.

E2E: e2e/tests/c14-bh008-crop-zero-dims-banner.spec.ts."
```

---

## Task Group B: BH-034 — Upload size limits

### Task B1: Write failing API test

**Files:**
- Create: `server/api_tests/upload_size_limit_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api_tests

import (
	"bytes"
	"crypto/rand"
	"mime/multipart"
	"net/http"
	"testing"
)

// BH-034: resource upload above MaxUploadSize must be rejected (413 or 400).
func TestResourceUpload_RejectsOversize(t *testing.T) {
	tc := SetupTestEnv(t)
	// Set a low limit for this test.
	tc.Config.MaxUploadSize = 1 << 20 // 1 MB

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("resource", "big.bin")
	buf := make([]byte, 2<<20) // 2 MB > 1 MB limit
	_, _ = rand.Read(buf)
	fw.Write(buf)
	mw.Close()

	resp := tc.MakeRequest(http.MethodPost, "/v1/resource", body,
		withHeader("Content-Type", mw.FormDataContentType()))
	if resp.Code != http.StatusBadRequest && resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 400 or 413 for over-limit upload, got %d; body=%s", resp.Code, resp.Body.String())
	}
}

// BH-034: under-limit upload must succeed.
func TestResourceUpload_AcceptsAtLimit(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.Config.MaxUploadSize = 1 << 20 // 1 MB

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("resource", "small.bin")
	buf := make([]byte, 128<<10) // 128 KB
	_, _ = rand.Read(buf)
	fw.Write(buf)
	mw.Close()

	resp := tc.MakeRequest(http.MethodPost, "/v1/resource", body,
		withHeader("Content-Type", mw.FormDataContentType()))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for under-limit upload, got %d; body=%s", resp.Code, resp.Body.String())
	}
}

// BH-034: version upload path must have the same guard.
func TestResourceVersionUpload_RejectsOversize(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.Config.MaxUploadSize = 1 << 20 // 1 MB

	// Create a resource first
	r := tc.SeedResource(t)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", "v2.bin")
	buf := make([]byte, 2<<20) // 2 MB
	_, _ = rand.Read(buf)
	fw.Write(buf)
	mw.Close()

	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/versions?resourceId="+r.IDString(), body,
		withHeader("Content-Type", mw.FormDataContentType()))
	if resp.Code != http.StatusBadRequest && resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 400 or 413 for version upload over limit, got %d", resp.Code)
	}
}
```

`tc.Config.MaxUploadSize` and `tc.SeedResource(t)` may need to be added to the test harness. Check `server/api_tests/harness.go` or similar.

- [ ] **Step 2: Run 3× to verify fails**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestResourceUpload_Rejects -v -count=3
```

Expected: FAIL — limit isn't enforced (2 MB upload returns 200).

### Task B2: Add `MaxUploadSize` config field + flag

**Files:**
- Modify: `application_context/context.go`
- Modify: `cmd/mahresources/main.go` (or wherever flags are defined — use `MAX_IMPORT_SIZE` as a reference pattern)

- [ ] **Step 1: Add the field**

In `Config` struct (follow the `MaxImportSize` pattern):

```go
// MaxUploadSize is the maximum per-upload body size in bytes for resource
// and version uploads. BH-034. Default 2 GB.
MaxUploadSize int64
```

- [ ] **Step 2: Register the flag**

```go
maxUploadSize := flag.Int64("max-upload-size", envInt64Default("MAX_UPLOAD_SIZE", 2<<30),
    "Maximum upload body size in bytes for resource and version uploads (default: 2 GB)")
```

Wire into the config constructor.

### Task B3: Add `tryFillStructValuesFromRequestWithLimit` helper

**Files:**
- Modify: `server/api_handlers/api_handlers.go:57-97`

- [ ] **Step 1: Add the limited variant**

After the existing `tryFillStructValuesFromRequest`:

```go
// tryFillStructValuesFromRequestWithLimit wraps request.Body in
// http.MaxBytesReader before delegating to tryFillStructValuesFromRequest.
// BH-034: resource + version uploads must bound the body size to prevent
// a single oversize POST from filling the disk.
func tryFillStructValuesFromRequestWithLimit(dst any, w http.ResponseWriter, request *http.Request, maxBytes int64) error {
	if maxBytes > 0 {
		request.Body = http.MaxBytesReader(w, request.Body, maxBytes)
	}
	return tryFillStructValuesFromRequest(dst, request)
}
```

### Task B4: Wire the helper into resource upload

**Files:**
- Modify: `server/api_handlers/resource_api_handlers.go:128-175`
- Modify: the route registration (needs the `Config` value) — look at `server/routes.go` for how the handler is called

- [ ] **Step 1: Change `GetResourceUploadHandler` signature to take `maxUploadSize`**

```go
func GetResourceUploadHandler(ctx interfaces.ResourceCreator, maxUploadSize int64) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.ResourceCreator)

		var remoteCreator = query_models.ResourceFromRemoteCreator{}

		if err := tryFillStructValuesFromRequestWithLimit(&remoteCreator, writer, request, maxUploadSize); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		// ... rest unchanged ...
```

- [ ] **Step 2: Update the route registration**

In `server/routes.go`, find the call to `GetResourceUploadHandler` and pass `appContext.Config.MaxUploadSize`.

### Task B5: Wire the guard into version upload

**Files:**
- Modify: `server/api_handlers/version_api_handlers.go:80-90`

- [ ] **Step 1: Add MaxBytesReader before ParseMultipartForm**

Find the existing `ParseMultipartForm(100 << 20)` call. Add the MaxBytesReader line before it:

```go
func GetResourceAddVersionHandler(ctx interfaces.ResourceVersionCreator, maxUploadSize int64) func(http.ResponseWriter, *http.Request) {
    return func(w http.ResponseWriter, r *http.Request) {
        if maxUploadSize > 0 {
            r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
        }
        if err := r.ParseMultipartForm(100 << 20); err != nil {
            http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
            return
        }
        // ... rest unchanged ...
    }
}
```

Update the route registration in `routes.go` to pass `appContext.Config.MaxUploadSize`.

### Task B6: Document in CLAUDE.md

- [ ] **Step 1: Add the flag row**

```markdown
| `-max-upload-size` | `MAX_UPLOAD_SIZE` | Maximum per-upload body size in bytes (default: 2 GB) |
```

### Task B7: Run + commit

```bash
go test --tags 'json1 fts5' ./... -count=1
cd e2e && npm run test:with-server:all
```

All green → commit:

```bash
git add application_context/context.go cmd/ \
  server/api_handlers/api_handlers.go \
  server/api_handlers/resource_api_handlers.go \
  server/api_handlers/version_api_handlers.go \
  server/routes.go \
  server/api_tests/upload_size_limit_test.go \
  CLAUDE.md
git commit -m "feat(uploads): BH-034 — bound resource + version upload sizes

Previously both /v1/resource and /v1/resource/versions called
ParseMultipartForm with no MaxBytesReader upstream — the 100 MB buffer
limit only controlled in-memory spill, not the actual body size. A
single oversize POST could exhaust disk.

New config MaxUploadSize / flag --max-upload-size / env
MAX_UPLOAD_SIZE, default 2 GB (matches the MAX_IMPORT_SIZE precedent).
New helper tryFillStructValuesFromRequestWithLimit wraps request.Body
in http.MaxBytesReader before the existing parse path.

Bonus safety: under-limit uploads behave unchanged; over-limit return
HTTP 400 (gorilla-mux surfaces MaxBytesReader errors as ParseMultipart
failures) with a clear error message.

API test: server/api_tests/upload_size_limit_test.go.
Docs: CLAUDE.md config table gains the --max-upload-size row."
```

---

## Task D: Full test matrix + PR + merge + log backfill + cleanup

Standard pattern. PR title: `fix(bughunt c14): BH-008/034/039 ingestion safety`.

Log updates:
- BH-008: FIXED
- BH-034: FIXED
- BH-039: FIXED (note: discovered during the c-batch fixture repair, absorbed into c14's scope at plan time)

---

## Self-review checklist

- [ ] BH-008: crop modal shows banner + disables Crop on decodeFailed
- [ ] BH-034: resource + version uploads reject over-limit with clear error
- [ ] BH-034: under-limit uploads behave unchanged
- [ ] BH-039: SVG/ICO/WebP uploads succeed with Width=0/Height=0
- [ ] BH-039: truncated-PNG rejection still works (regression guard)
- [ ] SVG lightbox E2E un-skipped and passing
- [ ] --max-upload-size documented in CLAUDE.md
