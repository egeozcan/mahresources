# Cluster 3 — Image Ingestion + Hashing (BH-011, BH-018)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. **Serial inside cluster: BH-011 completes before BH-018.** Steps use checkbox (`- [ ]`) syntax.

**Goal:** Reject truncated / undecodable image uploads at ingestion (BH-011) and eliminate perceptual-hash false positives between solid-color images (BH-018).

**Architecture:** BH-011 — in `application_context/resource_media_context.go`, if `image.Decode` errors or returns `Dx()==0 || Dy()==0`, reject the upload with 400. BH-018 — in `hash_worker/worker.go`'s `findAndStoreSimilarities`, when DHash Hamming ≤ lower-threshold, also require AHash Hamming ≤ a separate threshold before recording the pair. Introduce `--hash-ahash-threshold` flag.

**Tech Stack:** Go, `image` stdlib, `imgsim` library for perceptual hashes.

**Worktree branch:** `bugfix/c3-image-hashing`

---

## File structure

**Modified:**
- `application_context/resource_media_context.go` — image decode check
- `application_context/context.go` / config — new `HashAHashThreshold` field, new flag `--hash-ahash-threshold`
- `hash_worker/worker.go` — AHash check in `findAndStoreSimilarities`

**Created:**
- `server/api_tests/image_ingestion_rejects_truncated_test.go`
- `hash_worker/worker_solid_color_test.go`

---

## Pre-work: recon

- [ ] **Step 1: Locate the exact function in `resource_media_context.go` that decodes images**

```bash
grep -n "image.Decode\|Decode(\|Width\s*=\|Height\s*=" application_context/resource_media_context.go | head -30
grep -n "DHash\|AHash\|DifferenceHash\|AverageHash\|findAndStoreSimilarities" hash_worker/worker.go | head -40
```

Expected to find `image.Decode` and the width/height assignment. Note the function name and line — it's where the rejection branch goes.

---

## Task A: BH-011 — Reject truncated images

### Task A1: Write the failing API test

**Files:**
- Create: `server/api_tests/image_ingestion_rejects_truncated_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api_tests

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Minimal PNG header (8 bytes) without IDAT/IEND — decode must fail.
// \x89PNG\r\n\x1a\n followed by a fake IHDR chunk that's cut short.
var truncatedPNG = []byte{
	0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
	0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0xc8, 0x00, 0x00, 0x00, 0xc8,
	0x08, 0x02, 0x00, 0x00, 0x00,
	// Truncated — no IDAT, no IEND, no trailing CRC table.
}

func TestImageIngestion_RejectsTruncatedPNG(t *testing.T) {
	tc := SetupTestEnv(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("File", "bh011-truncated.png")
	require.NoError(t, err)
	_, err = io.Copy(part, bytes.NewReader(truncatedPNG))
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("Name", "bh011-truncated"))
	require.NoError(t, writer.WriteField("ContentType", "image/png"))
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, "/v1/resource", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "truncated image must be rejected with 400")
	assert.Contains(t, rr.Body.String(), "decode", "error message must reference decode failure")
}

func TestImageIngestion_AcceptsValidImage(t *testing.T) {
	tc := SetupTestEnv(t)

	// Valid 1×1 PNG (from stdlib):
	validPNG := []byte{
		0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
		0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0D, 'I', 'D', 'A', 'T',
		0x78, 0x9C, 0x62, 0x00, 0x01, 0x00, 0x00, 0x05,
		0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00,
		0x00, 0x00, 'I', 'E', 'N', 'D', 0xAE, 0x42, 0x60, 0x82,
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("File", "bh011-valid.png")
	io.Copy(part, bytes.NewReader(validPNG))
	writer.WriteField("Name", "bh011-valid")
	writer.WriteField("ContentType", "image/png")
	writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "/v1/resource", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "valid image must still succeed")
}
```

- [ ] **Step 2: Run 3× to verify fail**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestImageIngestion_RejectsTruncatedPNG -v -count=3
```

Expected: FAIL all 3 runs. Failure: `rr.Code` is `200` (the bug: truncated accepted).

### Task A2: Add the decode-reject branch

**Files:**
- Modify: `application_context/resource_media_context.go`

- [ ] **Step 1: Locate the image-ingestion entry point (likely a function named `extractImageDimensions`, `processImageUpload`, `populateResourceMeta`, or similar). Wrap the `image.Decode` call:**

```go
img, _, err := image.Decode(bytes.NewReader(fileBytes))
if err != nil {
    return fmt.Errorf("uploaded file is not a valid image (failed to decode): %w", err)
}
bounds := img.Bounds()
if bounds.Dx() == 0 || bounds.Dy() == 0 {
    return fmt.Errorf("uploaded file is not a valid image (zero dimensions)")
}
resource.Width = uint(bounds.Dx())
resource.Height = uint(bounds.Dy())
```

Adjust the surrounding function signature to thread the error back up to the HTTP handler so it becomes a 400.

**If the current code silently sets W=0/H=0 and continues, this is the exact behavior BH-011 reports. The fix is to return the error instead.**

- [ ] **Step 2: Ensure the HTTP handler maps this to 400**

Check `server/api_handlers/resource_api_handlers.go` — if `AddResource` (or whatever the create path calls) returns an error, it must surface via `HandleError` with a 400 status code. If the handler currently catches all errors as 500, wrap the decode error in something that maps to 400 (either a sentinel error type or check `strings.Contains(err.Error(), "decode")`).

- [ ] **Step 3: Run the API tests 3× to verify pass**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestImageIngestion -v -count=3
```

Expected: PASS all 3 runs.

- [ ] **Step 4: Run related resource tests to confirm no regression**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run 'TestResource|TestImage' -v
```

Expected: PASS. If an existing test relied on accepting zero-dim images, update it to use valid imagery.

- [ ] **Step 5: Commit**

```bash
cd <worktree>
git add application_context/resource_media_context.go server/api_tests/image_ingestion_rejects_truncated_test.go
git commit -m "fix(image): BH-011 — reject undecodable / zero-dimension image uploads"
```

---

## Task B: BH-018 — AHash secondary check for solid-color false positives

### Task B1: Add the config flag

**Files:**
- Modify: `application_context/context.go` (or wherever `MahresourcesConfig` is defined)
- Modify: `main.go` (flag registration)

- [ ] **Step 1: Add field + flag**

Config struct:

```go
type MahresourcesConfig struct {
    // ...existing fields...
    HashAHashThreshold uint64 // BH-018: max Hamming distance for AHash secondary check
}
```

main.go (command-line flag registration):

```go
flag.Uint64Var(&config.HashAHashThreshold, "hash-ahash-threshold", 5,
    "Max Hamming distance for AHash secondary check to confirm DHash similarity (BH-018)")
```

And the env-var counterpart via the existing env-loader pattern:

```go
if v := os.Getenv("HASH_AHASH_THRESHOLD"); v != "" {
    if n, err := strconv.ParseUint(v, 10, 64); err == nil {
        config.HashAHashThreshold = n
    }
}
```

### Task B2: Write the failing hash-worker unit test

**Files:**
- Create: `hash_worker/worker_solid_color_test.go`

- [ ] **Step 1: Write the failing test**

```go
package hash_worker_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/Nr90/imgsim"
)

// Reproduce BH-018: AHash distinguishes solid colors; DHash does not.
// This test asserts that any new similarity predicate (combining DHash + AHash)
// returns FALSE for two different solid colors.
func TestSolidColorHashes_AHashDistinguishes(t *testing.T) {
	lightblue := makeSolidPNG(t, color.RGBA{R: 173, G: 216, B: 230, A: 255})
	orange := makeSolidPNG(t, color.RGBA{R: 255, G: 165, B: 0, A: 255})

	dhashA := imgsim.DifferenceHash(decodePNG(t, lightblue))
	dhashB := imgsim.DifferenceHash(decodePNG(t, orange))
	ahashA := imgsim.AverageHash(decodePNG(t, lightblue))
	ahashB := imgsim.AverageHash(decodePNG(t, orange))

	// Confirm the DHash false-positive (both zero)
	if imgsim.Distance(dhashA, dhashB) > 2 {
		t.Fatalf("pre-condition failed: DHash was expected to be near-zero for solid colors, got distance %d",
			imgsim.Distance(dhashA, dhashB))
	}

	// AHash must produce a non-trivial distance
	ahashDist := imgsim.Distance(ahashA, ahashB)
	if ahashDist < 10 {
		t.Fatalf("AHash should distinguish solid lightblue from solid orange, got distance %d", ahashDist)
	}

	t.Logf("DHash distance: %d, AHash distance: %d", imgsim.Distance(dhashA, dhashB), ahashDist)
}

func makeSolidPNG(t *testing.T, c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			img.Set(x, y, c)
		}
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodePNG(t *testing.T, data []byte) image.Image {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}
```

- [ ] **Step 2: Run 3× — the invariants should PASS on baseline (this is the baseline for BH-018)**

```bash
go test --tags 'json1 fts5' ./hash_worker/ -run TestSolidColorHashes -v -count=3
```

Expected: PASS (the precondition). This test documents the hashing invariant. Next test below asserts the actual behavior change.

### Task B3: Write the failing end-to-end similarity test

**Files:**
- Append to: `hash_worker/worker_solid_color_test.go`

- [ ] **Step 1: Write the failing test using a test-specific similarity helper**

```go
func TestSimilarity_SolidColorsMustNotMatch(t *testing.T) {
	lightblueImg := decodePNG(t, makeSolidPNG(t, color.RGBA{173, 216, 230, 255}))
	orangeImg := decodePNG(t, makeSolidPNG(t, color.RGBA{255, 165, 0, 255}))

	dA, aA := imgsim.DifferenceHash(lightblueImg), imgsim.AverageHash(lightblueImg)
	dB, aB := imgsim.DifferenceHash(orangeImg), imgsim.AverageHash(orangeImg)

	dhashThr := uint64(10) // matches HashSimilarityThreshold default
	ahashThr := uint64(5)  // new BH-018 threshold

	similar := hash_worker.AreSimilar(dA, aA, dB, aB, dhashThr, ahashThr)
	if similar {
		t.Fatalf("BH-018: lightblue and orange solid PNGs must NOT be recorded as similar")
	}
}

func TestSimilarity_NearDupesStillMatch(t *testing.T) {
	base := makeGradientPNG(t, 0)
	near := makeGradientPNG(t, 3) // tiny perturbation

	dA, aA := imgsim.DifferenceHash(decodePNG(t, base)), imgsim.AverageHash(decodePNG(t, base))
	dB, aB := imgsim.DifferenceHash(decodePNG(t, near)), imgsim.AverageHash(decodePNG(t, near))

	similar := hash_worker.AreSimilar(dA, aA, dB, aB, 10, 5)
	if !similar {
		t.Fatalf("near-duplicate gradients must still register as similar after BH-018 fix")
	}
}

func makeGradientPNG(t *testing.T, offset int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			r := uint8(x + offset)
			g := uint8(y + offset)
			img.Set(x, y, color.RGBA{r, g, 128, 255})
		}
	}
	buf := &bytes.Buffer{}
	png.Encode(buf, img)
	return buf.Bytes()
}
```

Add import `"mahresources/hash_worker"`.

- [ ] **Step 2: Run 3× — expect compile failure because `hash_worker.AreSimilar` does not yet exist**

```bash
go test --tags 'json1 fts5' ./hash_worker/ -run TestSimilarity -v -count=3
```

Expected: 3× compile errors about `AreSimilar` undefined — this confirms the pre-implementation state.

### Task B4: Implement `AreSimilar` and use it in `findAndStoreSimilarities`

**Files:**
- Modify: `hash_worker/worker.go`

- [ ] **Step 1: Export a pure-logic `AreSimilar` helper and use it from `findAndStoreSimilarities`**

```go
// AreSimilar returns true when two images should be recorded as perceptually
// similar. BH-018: DHash alone falsely matches all uniform/solid-color images
// (Hamming distance 0 for every pair). When DHash distance is small, we
// additionally require AHash distance to be small.
func AreSimilar(dHashA, aHashA, dHashB, aHashB, dHashThr, aHashThr uint64) bool {
    dDist := uint64(imgsim.Distance(imgsim.Hash(dHashA), imgsim.Hash(dHashB)))
    if dDist > dHashThr {
        return false
    }
    aDist := uint64(imgsim.Distance(imgsim.Hash(aHashA), imgsim.Hash(aHashB)))
    return aDist <= aHashThr
}
```

(If `imgsim.Distance` takes native `uint64` directly rather than a Hash struct, adjust accordingly — check the imgsim version already in use.)

- [ ] **Step 2: Update `findAndStoreSimilarities` (around line 423 / 431 per bug log) to consult both DHash and AHash**

Pass AHash into the function signature and use it:

```go
// Previously: findAndStoreSimilarities(resource.ID, dHashInt)
findAndStoreSimilarities(resource.ID, dHashInt, aHashInt)
```

Inside the function, for each candidate row in `image_hashes`:

```go
if !AreSimilar(candidate.DHashInt, candidate.AHashInt, dHashInt, aHashInt,
    cfg.HashSimilarityThreshold, cfg.HashAHashThreshold) {
    continue
}
// ... existing insertion logic
```

- [ ] **Step 3: Run the unit tests 3× to verify pass**

```bash
go test --tags 'json1 fts5' ./hash_worker/ -run TestSimilarity -v -count=3
```

Expected: PASS all 3 runs.

- [ ] **Step 4: Commit**

```bash
cd <worktree>
git add hash_worker/ application_context/context.go main.go
git commit -m "fix(hash): BH-018 — AHash secondary check eliminates solid-color false positives"
```

---

## Cluster PR gate

- [ ] **Step 1: Full Go suite**

```bash
cd <worktree>
go test --tags 'json1 fts5' ./...
```

Expected: PASS.

- [ ] **Step 2: Rebase + full E2E + Postgres per master plan.**

- [ ] **Step 3: Open PR, self-merge**

```bash
gh pr create --title "fix(image): BH-011, BH-018 — ingestion + perceptual-hash correctness" --body "$(cat <<'EOF'
Closes BH-011, BH-018.

## Changes

- `application_context/resource_media_context.go` — reject uploads where `image.Decode` errors or bounds are zero. 400 with "failed to decode" message.
- `hash_worker/worker.go` — new `AreSimilar` helper combines DHash (existing) with a secondary AHash check. `findAndStoreSimilarities` now requires both distances below threshold before recording a pair.
- New flag `--hash-ahash-threshold` (default 5) / env `HASH_AHASH_THRESHOLD`.

## Tests

- Go API: ✓ truncated PNG → 400, valid PNG → 200, pass 3× pre-fix red / post-fix green.
- Go unit: ✓ two solid colors NOT recorded as similar; two near-dupe gradients still recorded.
- Full `go test ./...`: ✓
- Full E2E (browser + CLI): ✓
- Postgres: ✓

## Operator note

Existing DB rows with `Width=0 AND ContentType LIKE 'image/%'` were created before this fix. Audit query to list them for manual remediation:

```sql
SELECT id, name, content_type, size FROM resources WHERE content_type LIKE 'image/%' AND (width = 0 OR height = 0);
```

## Bug-hunt-log update

Post-merge: move BH-011, BH-018 to Fixed / closed.
EOF
)"
gh pr merge --merge --delete-branch
```

Then master plan Step F.
