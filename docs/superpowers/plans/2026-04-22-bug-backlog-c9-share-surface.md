# Cluster 9 — Share Surface (BH-032, BH-033, BH-035, BH-038)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Four disjoint bugs; groups A–D can run as parallel subagents where worktree files don't collide. The schema migration (Group C) should land before Group D in the same worktree to keep the admin page's `shareCreatedAt` display honest. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Harden the share server with security headers (BH-032), stop constructing absolute share URLs from the server bind address (BH-033), add a centralized `/admin/shares` dashboard with `shareCreatedAt` tracking and bulk-revoke (BH-035), and stop serializing `shareToken` into Alpine `x-data` on the `/notes` listing (BH-038).

**Architecture:**

- **Group A (BH-032):** New middleware on `server/share_server.go::Start()` + `Handler()`. Sets `X-Frame-Options: DENY`, `Content-Security-Policy` (strict default-src 'self'), `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`, `Strict-Transport-Security: max-age=15552000`. Same middleware applied to the primary server in a **separate commit within the same PR** so the primary-server CSP can be rolled back independently if a template breaks.
- **Group B (BH-033):** New `Config.SharePublicURL string` + flag `--share-public-url` / env `SHARE_PUBLIC_URL` (empty default). The share URL constructor in `note_template_context.go` uses it when set; when unset, returns empty string and UI renders a warning "Share URL base is not configured — set SHARE_PUBLIC_URL" + the relative `/s/<token>` path. No more bind-address fallback — it's wrong for any non-loopback private bind.
- **Group C (BH-035):** Nullable `ShareCreatedAt *time.Time` column on `Note` (NULL for existing rows, not back-filled — we don't know when they were minted). GORM auto-migration handles the column add. Set in the handler that mints the token. New `GET /admin/shares` handler + `templates/adminShares.tpl` showing `Name | Public URL | Created | Revoke` columns, with a bulk-revoke form. Reuses existing `DELETE /v1/note/share?id=<noteId>` for single revoke; new `POST /v1/admin/shares/bulk-revoke` for bulk.
- **Group D (BH-038):** Change the notes-list context provider to strip `ShareToken` from card payloads before JSON-encoding into `x-data`. Expose `HasShare bool` for any UI that needs the signal.

**Tech Stack:** Go (http middleware, GORM migration, new handler), Pongo2 (new template), Alpine.js, Playwright E2E.

**Worktree branch:** `bugfix/c9-share-surface`

**Top-level spec:** `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 9 (largest cluster, penultimate in execution order).

---

## File structure

**Modified:**
- `server/share_server.go:56-91` — add `withSecurityHeaders` middleware to `Handler()` and `Start()` (BH-032)
- `server/routes.go` or wherever the primary server's middleware is wired — apply the same headers middleware as a second commit (BH-032)
- `application_context/context.go` — `SharePublicURL string` field on `Config` (BH-033)
- `cmd/mahresources/main.go` or flags file — `--share-public-url` flag (BH-033)
- `server/template_handlers/template_context_providers/note_template_context.go:253-258` — use `SharePublicURL` when set; empty otherwise (BH-033)
- `templates/partials/noteShare.tpl` — render warning when URL is empty (BH-033)
- `models/note_model.go` — new `ShareCreatedAt *time.Time` field (BH-035)
- `application_context/note_context.go` — set `ShareCreatedAt = now` when minting token (BH-035)
- `server/routes.go` — register `/admin/shares`, `/v1/admin/shares/bulk-revoke` (BH-035)
- `server/template_handlers/admin_shares_handler.go` (new) (BH-035)
- `templates/adminShares.tpl` (new) (BH-035)
- `server/template_handlers/template_context_providers/notes_list_template_context.go` (find via grep) — strip `ShareToken`, add `HasShare` (BH-038)
- `templates/partials/note.tpl` or the notes-list card template — use `HasShare` instead of `shareToken` (BH-038)
- `CLAUDE.md` — document `--share-public-url` flag

**Created:**
- `server/api_tests/share_server_security_headers_test.go`
- `server/api_tests/share_url_public_url_test.go`
- `server/api_tests/admin_shares_test.go`
- `server/api_tests/notes_list_no_share_token_leak_test.go`
- `e2e/tests/c9-bh035-admin-shares-dashboard.spec.ts`

---

## Task 0: Worktree + baseline

- [ ] **Step 1: Worktree**

```bash
cd /Users/egecan/Code/mahresources
git worktree add -b bugfix/c9-share-surface ../mahresources-c9 master
cd ../mahresources-c9
```

- [ ] **Step 2: Baseline tests + migration sanity**

```bash
go test --tags 'json1 fts5' ./... -count=1
# Sanity — start the server briefly against a clean sqlite db to verify
# current migration runs cleanly before we add a new column
./mahresources -ephemeral -bind-address=:0 &
kill %1 2>/dev/null
```

---

## Task Group D: BH-038 — Strip `shareToken` from notes list payload

(Done first — smallest surface, no schema change.)

### Task D1: Write failing API test

**Files:**
- Create: `server/api_tests/notes_list_no_share_token_leak_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api_tests

import (
	"net/http"
	"strings"
	"testing"
)

// BH-038: /notes?shared=true serializes note JSON into Alpine x-data,
// which included ShareToken. No plaintext share tokens should appear in
// the HTML.
func TestNotesListDoesNotLeakShareTokens(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.SeedNote(t)
	token, err := tc.appCtx.MintShareToken(note.ID)
	if err != nil {
		t.Fatalf("mint share token: %v", err)
	}

	resp := tc.MakeRequest(http.MethodGet, "/notes?shared=true", nil)
	assertStatus(t, resp, 200)
	body := resp.Body.String()

	if strings.Contains(body, token) {
		t.Fatalf("share token %q leaked into /notes HTML response", token)
	}
	// The shareToken field name in general shouldn't appear either
	if strings.Contains(body, `"shareToken":"`) || strings.Contains(body, "shareToken:&quot;") {
		t.Fatalf("shareToken field appears in serialized x-data; should be stripped")
	}
}
```

- [ ] **Step 2: Run 3× to verify fails**

Expected: FAIL — token appears in body.

### Task D2: Strip `ShareToken` in notes-list context provider

**Files:**
- Locate: `grep -rn "shared=true\|NotesList" server/template_handlers/`
- Modify: the `/notes` handler's context provider

- [ ] **Step 1: Find the serialization point**

Typically `server/template_handlers/template_context_providers/notes_list_template_context.go` or similar. Look for where `note` is passed to the template — before the template renders, build a stripped view struct:

```go
type notesListCard struct {
    // Only the fields the list card actually needs:
    ID          uint       `json:"ID"`
    Name        string     `json:"Name"`
    Description string     `json:"Description"`
    CreatedAt   time.Time  `json:"CreatedAt"`
    UpdatedAt   time.Time  `json:"UpdatedAt"`
    Meta        types.JSON `json:"Meta,omitempty"`
    OwnerId     *uint      `json:"OwnerId,omitempty"`
    NoteTypeId  *uint      `json:"NoteTypeId,omitempty"`
    HasShare    bool       `json:"hasShare"`
    // NOTE: ShareToken deliberately omitted (BH-038)
}
```

Map each `models.Note` to this struct before passing to the template. If the notes-list card template reads fields that the stripped struct doesn't include, add them — but NEVER `ShareToken`.

### Task D3: Update notes-list card template to use `hasShare`

**Files:**
- Modify: `templates/partials/note.tpl` (or the list card variant — check `grep -rn "shareToken" templates/`)

- [ ] **Step 1: Replace any `shareToken`-based Alpine expressions with `hasShare`**

Any conditional like `x-show="shareToken"` becomes `x-show="hasShare"`. Any construction of a URL from `shareToken` on a list card is wrong at this level — the card shouldn't construct share URLs; those live on the detail page.

### Task D4: Run + commit

```bash
npm run build
go test --tags 'json1 fts5' ./server/api_tests/ -run TestNotesListDoesNotLeakShareTokens -v -count=3
```

Expected: PASS.

```bash
git add server/template_handlers/ templates/ public/dist/ public/tailwind.css \
  server/api_tests/notes_list_no_share_token_leak_test.go
git commit -m "fix(notes): BH-038 — strip shareToken from /notes list x-data

Previously /notes (and /notes?shared=true) serialized full note objects
into every card's Alpine x-data attribute, including ShareToken. Any
page-cache, browser history snapshot, or log aggregator with access to
/notes captured plaintext share tokens.

Notes-list context now maps each note through a stripped view struct
exposing only the fields the list card renders. hasShare boolean replaces
ShareToken for 'this note is shared' UI signals.

API test: server/api_tests/notes_list_no_share_token_leak_test.go."
```

---

## Task Group A: BH-032 — Security headers middleware

### Task A1: Write failing test

**Files:**
- Create: `server/api_tests/share_server_security_headers_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/server"
)

func TestShareServer_SecurityHeaders(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.SeedNote(t)
	token, _ := tc.appCtx.MintShareToken(note.ID)

	ss := server.NewShareServer(tc.appCtx)
	handler := ss.Handler()
	req := httptest.NewRequest(http.MethodGet, "/s/"+token, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	required := map[string]string{
		"X-Frame-Options":         "DENY",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
	}
	for hdr, want := range required {
		got := w.Header().Get(hdr)
		if got != want {
			t.Errorf("%s: expected %q, got %q", hdr, want, got)
		}
	}
	// CSP + HSTS just need to be set (exact value checked elsewhere)
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy header missing")
	}
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("Strict-Transport-Security header missing")
	}
}

func TestShareServer_SecurityHeaders_ErrorPath(t *testing.T) {
	// Even on 404 / errors, nosniff must be set
	tc := SetupTestEnv(t)
	ss := server.NewShareServer(tc.appCtx)
	req := httptest.NewRequest(http.MethodGet, "/s/doesnotexist", nil)
	w := httptest.NewRecorder()
	ss.Handler().ServeHTTP(w, req)
	if !strings.EqualFold(w.Header().Get("X-Content-Type-Options"), "nosniff") {
		t.Error("nosniff missing on error path")
	}
}
```

- [ ] **Step 2: Run 3× to verify fails**

Expected: FAIL — headers missing.

### Task A2: Add middleware

**Files:**
- Modify: `server/share_server.go:56-91`

- [ ] **Step 1: Write the middleware**

Add at the top of `share_server.go` (near `Handler`):

```go
// withSecurityHeaders wraps an http.Handler and applies the baseline security
// headers the share server needs: clickjacking protection, MIME-type sniffing
// off, referrer suppression (to stop tokens leaking via Referer), a strict
// default CSP, and HSTS. Applied to success and error paths alike. BH-032.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		// CSP: default-src 'self' blocks remote scripts/styles/images by default.
		// 'unsafe-inline' is required for Alpine.js expressions on x-data/x-show.
		// img-src includes data: for any base64-encoded previews.
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"frame-ancestors 'none'")
		h.Set("Strict-Transport-Security", "max-age=15552000")
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 2: Wrap in `Handler()` and `Start()`**

```go
func (s *ShareServer) Handler() http.Handler {
	router := mux.NewRouter()
	s.registerShareRoutes(router)
	return withSecurityHeaders(router)   // BH-032
}

// in Start():
router := mux.NewRouter()
s.registerShareRoutes(router)
s.server = &http.Server{
	Addr:         addr,
	Handler:      withSecurityHeaders(router),  // BH-032
	// ...
}
```

- [ ] **Step 3: Run 3× to verify pass**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestShareServer_SecurityHeaders -v -count=3
```

Expected: PASS.

### Task A3: Commit (share server only)

```bash
git add server/share_server.go server/api_tests/share_server_security_headers_test.go
git commit -m "fix(share): BH-032 — security headers on share server responses

Adds withSecurityHeaders middleware around the ShareServer router:
X-Frame-Options: DENY, X-Content-Type-Options: nosniff,
Referrer-Policy: no-referrer (stops share tokens leaking via Referer
to external-loaded fonts/images), Content-Security-Policy (strict
default-src 'self' plus unsafe-inline for Alpine), and
Strict-Transport-Security. Error paths keep nosniff.

API test: server/api_tests/share_server_security_headers_test.go.

Primary server receives the same middleware in a follow-up commit in
this PR so it can be rolled back independently if a template breaks."
```

### Task A4: Apply same middleware to primary server (second commit — rollback safety)

**Files:**
- Modify: `server/routes.go` or wherever middleware is configured

- [ ] **Step 1: Wrap primary router**

Find where the primary server's root router is built/served. Apply `withSecurityHeaders` there too.

- [ ] **Step 2: Run the full existing primary-server test suite to catch CSP regressions**

```bash
cd e2e && npm run test:with-server:all 2>&1 | tail -30
```

If CSP breaks any template (inline script/style violations), tighten or loosen the CSP as needed. Do NOT remove the middleware — instead adjust the CSP string to match the primary's actual needs. Document any differences in commit message.

- [ ] **Step 3: Commit**

```bash
git add server/routes.go server/
git commit -m "fix(primary): BH-032 — apply share-server security headers to primary too

Defense-in-depth: primary server gets the same headers middleware. Applied
in a separate commit so rolling back the primary half doesn't touch the
share server half.

CLAUDE.md notes the primary is intended for private networks; these
headers still harden against common misconfig (public accidental exposure,
iframe sandboxing by partners)."
```

---

## Task Group B: BH-033 — SHARE_PUBLIC_URL config

### Task B1: Write failing test

**Files:**
- Create: `server/api_tests/share_url_public_url_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api_tests

import (
	"net/http"
	"strings"
	"testing"
)

// BH-033: when SHARE_PUBLIC_URL is empty, the share-URL sidebar on /note?id=X
// must NOT construct an absolute URL from the bind address. Instead it
// shows a warning + the relative /s/<token> path.
func TestShareURL_NoFallback_WhenUnconfigured(t *testing.T) {
	tc := SetupTestEnvWithConfig(t, TestEnvConfig{SharePublicURL: ""})
	note := tc.SeedNote(t)
	_, _ = tc.appCtx.MintShareToken(note.ID)

	resp := tc.MakeRequest(http.MethodGet, "/note?id="+note.IDString(), nil)
	assertStatus(t, resp, 200)
	body := resp.Body.String()

	// The sidebar must NOT contain http://127.0.0.1:8383/ or similar bind-address URL
	if strings.Contains(body, "http://127.0.0.1:") || strings.Contains(body, "http://0.0.0.0:") || strings.Contains(body, "http://::1:") {
		t.Fatalf("share URL constructed from bind address even though SHARE_PUBLIC_URL is empty: %s", body)
	}

	// Warning message must appear somewhere on the page
	if !strings.Contains(body, "SHARE_PUBLIC_URL") {
		t.Error("warning message missing — page should reference SHARE_PUBLIC_URL config")
	}
}

func TestShareURL_UsesPublicURL_WhenSet(t *testing.T) {
	tc := SetupTestEnvWithConfig(t, TestEnvConfig{SharePublicURL: "https://share.example.com"})
	note := tc.SeedNote(t)
	tok, _ := tc.appCtx.MintShareToken(note.ID)

	resp := tc.MakeRequest(http.MethodGet, "/note?id="+note.IDString(), nil)
	assertStatus(t, resp, 200)
	body := resp.Body.String()

	expected := "https://share.example.com/s/" + tok
	if !strings.Contains(body, expected) {
		t.Errorf("expected share URL %q in page body; body=%s", expected, body)
	}
}
```

- [ ] **Step 2: Run 3× to verify fails**

Expected: FAIL — `SharePublicURL` isn't a config field yet; fallback still uses bind address.

### Task B2: Add `SharePublicURL` config + flag

**Files:**
- Modify: `application_context/context.go`, flag file

Follow the same pattern as `ShareBindAddress`. Default empty string.

### Task B3: Update the URL constructor

**Files:**
- Modify: `server/template_handlers/template_context_providers/note_template_context.go:253-258`

- [ ] **Step 1: Replace bind-address construction**

```go
// BH-033: only construct absolute share URL when SHARE_PUBLIC_URL is set.
// Bind address (even non-loopback internal IPs like 10.x/192.168.x) is not
// reliable for external recipients. If unset, expose the relative path and
// a warning the template can surface.
var shareBaseUrl string
if context.Config.SharePublicURL != "" {
    shareBaseUrl = strings.TrimRight(context.Config.SharePublicURL, "/")
}
// (no else branch — shareBaseUrl stays empty; template handles the warning)
```

Expose both `shareBaseUrl` and a new `shareUrlConfigured bool` to the template.

### Task B4: Update share-sidebar template

**Files:**
- Modify: `templates/partials/noteShare.tpl`

- [ ] **Step 1: Render conditional warning**

```pongo2
{% if shareUrlConfigured %}
<a href="{{ shareBaseUrl }}/s/{{ note.ShareToken }}" class="...">
    {{ shareBaseUrl }}/s/{{ note.ShareToken }}
</a>
{% else %}
<div class="p-2 bg-amber-50 border border-amber-200 rounded text-xs text-amber-800" data-testid="share-url-unconfigured-warning">
    <p class="font-medium">Share URL base is not configured.</p>
    <p class="mt-1">Set <code>SHARE_PUBLIC_URL</code> (flag: <code>--share-public-url=https://example.com</code>) to enable shareable links. The token path is <code>/s/{{ note.ShareToken }}</code>; prepend your server's public URL before sending.</p>
</div>
{% endif %}
```

### Task B5: Document + commit

Add to `CLAUDE.md` config table:

```markdown
| `-share-public-url` | `SHARE_PUBLIC_URL` | Externally-routable base URL for shared notes (e.g., `https://share.example.com`). Required for the share sidebar to construct absolute URLs; if unset, the UI shows a warning. |
```

```bash
git add application_context/context.go cmd/ server/template_handlers/ templates/ \
  public/dist/ public/tailwind.css \
  server/api_tests/share_url_public_url_test.go \
  CLAUDE.md
git commit -m "fix(share): BH-033 — SHARE_PUBLIC_URL config replaces bind-address fallback

New config SHARE_PUBLIC_URL / flag --share-public-url. When set, share
sidebar renders {SHARE_PUBLIC_URL}/s/<token>. When unset, sidebar shows
a warning + the relative /s/<token> path — NOT an absolute URL
constructed from the server's bind address.

Reason: bind addresses (127.0.0.1, 0.0.0.0, 10.x, 192.168.x, container
IPs, internal hostnames) are not reliable for external recipients. The
old fallback silently produced non-routable URLs for any non-localhost
deployment.

API test: server/api_tests/share_url_public_url_test.go.
Docs: CLAUDE.md."
```

---

## Task Group C: BH-035 — Admin shares dashboard

### Task C1: Write failing E2E for the dashboard

**Files:**
- Create: `e2e/tests/c9-bh035-admin-shares-dashboard.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-035: centralized /admin/shares dashboard for managing shared notes.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-035: admin shares dashboard', () => {
  test('/admin/shares lists notes with active tokens', async ({ page, apiClient }) => {
    const n1 = await apiClient.createNote({ name: `BH035-shared-${Date.now()}` });
    await apiClient.shareNote(n1.ID); // mints a token
    const n2 = await apiClient.createNote({ name: `BH035-unshared-${Date.now()}` });

    await page.goto('/admin/shares');
    await expect(page.getByText(n1.Name ?? n1.name)).toBeVisible();
    await expect(page.getByText(n2.Name ?? n2.name)).not.toBeVisible();
  });

  test('revoke single share removes the row', async ({ page, apiClient }) => {
    const n = await apiClient.createNote({ name: `BH035-revoke-${Date.now()}` });
    await apiClient.shareNote(n.ID);

    await page.goto('/admin/shares');
    const row = page.locator(`[data-share-note-id="${n.ID}"]`);
    await expect(row).toBeVisible();
    await row.getByRole('button', { name: /revoke/i }).click();
    await page.getByRole('button', { name: /confirm|revoke/i }).click();

    await expect(row).not.toBeVisible();
  });

  test('bulk revoke works for multiple selections', async ({ page, apiClient }) => {
    const a = await apiClient.createNote({ name: `BH035-bulk-a-${Date.now()}` });
    const b = await apiClient.createNote({ name: `BH035-bulk-b-${Date.now()}` });
    await apiClient.shareNote(a.ID);
    await apiClient.shareNote(b.ID);

    await page.goto('/admin/shares');
    await page.locator(`[data-share-note-id="${a.ID}"] input[type="checkbox"]`).check();
    await page.locator(`[data-share-note-id="${b.ID}"] input[type="checkbox"]`).check();
    await page.getByRole('button', { name: /bulk.*revoke|revoke selected/i }).click();
    await page.getByRole('button', { name: /confirm|revoke/i }).click();

    await expect(page.locator(`[data-share-note-id="${a.ID}"]`)).not.toBeVisible();
    await expect(page.locator(`[data-share-note-id="${b.ID}"]`)).not.toBeVisible();
  });
});
```

- [ ] **Step 2: Run 3× to verify fails**

Expected: FAIL — 404 on `/admin/shares`.

### Task C2: Add `ShareCreatedAt` to Note model

**Files:**
- Modify: `models/note_model.go:28`

- [ ] **Step 1: Add the field**

```go
ShareToken     *string    `gorm:"uniqueIndex;size:32" json:"shareToken,omitempty"`
ShareCreatedAt *time.Time `gorm:"index" json:"shareCreatedAt,omitempty"` // BH-035
```

GORM's auto-migration will add the column to `notes`. Existing rows will have NULL.

### Task C3: Set `ShareCreatedAt` when minting a token

**Files:**
- Modify: wherever share tokens are minted — typically `application_context/note_context.go` or `note_share_context.go`. Grep `MintShareToken|GenerateShareToken`.

- [ ] **Step 1: Set both fields in the same transaction**

```go
now := time.Now()
note.ShareToken = &token
note.ShareCreatedAt = &now
// ... existing save path ...
```

- [ ] **Step 2: Clear `ShareCreatedAt` when revoking**

```go
note.ShareToken = nil
note.ShareCreatedAt = nil
```

### Task C4: Add new handler + route + template

**Files:**
- Create: `server/template_handlers/admin_shares_handler.go`
- Modify: `server/routes.go`
- Create: `templates/adminShares.tpl`

- [ ] **Step 1: Handler**

```go
package template_handlers

// GetAdminSharesHandler renders /admin/shares: a table of every note with an
// active ShareToken, columns Name | Public URL | Created | Revoke.
func GetAdminSharesHandler(appContext *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
    return func(w http.ResponseWriter, r *http.Request) {
        var notes []models.Note
        err := appContext.GetDB().
            Where("share_token IS NOT NULL").
            Order("share_created_at DESC NULLS LAST").
            Find(&notes).Error
        if err != nil {
            http.Error(w, "failed to load shares: "+err.Error(), http.StatusInternalServerError)
            return
        }
        context := make(map[string]any)
        context["shares"] = notes
        context["shareBaseUrl"] = strings.TrimRight(appContext.Config.SharePublicURL, "/")
        context["shareUrlConfigured"] = appContext.Config.SharePublicURL != ""
        renderTemplate(w, r, "adminShares.tpl", context)
    }
}

// GetAdminSharesBulkRevokeHandler revokes share tokens for all note IDs
// in the posted list. POST /v1/admin/shares/bulk-revoke with ids=1&ids=2...
func GetAdminSharesBulkRevokeHandler(appContext *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
    return func(w http.ResponseWriter, r *http.Request) {
        if err := r.ParseForm(); err != nil {
            http.Error(w, "bad form", http.StatusBadRequest)
            return
        }
        ids := r.Form["ids"]
        for _, idStr := range ids {
            id, err := strconv.ParseUint(idStr, 10, 64)
            if err != nil { continue }
            _ = appContext.RevokeShareToken(uint(id))
        }
        http.Redirect(w, r, "/admin/shares", http.StatusSeeOther)
    }
}
```

Adjust imports + types to match the repo's existing handler signatures (look at `admin_export_template_handler.go` for a reference).

- [ ] **Step 2: Register routes**

In `server/routes.go`:

```go
router.Methods(http.MethodGet).Path("/admin/shares").HandlerFunc(template_handlers.GetAdminSharesHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/admin/shares/bulk-revoke").HandlerFunc(template_handlers.GetAdminSharesBulkRevokeHandler(appContext))
```

- [ ] **Step 3: Template**

Create `templates/adminShares.tpl`:

```pongo2
{% extends "/layouts/base.tpl" %}

{% block body %}
<section class="p-6">
    <h1 class="text-xl font-semibold mb-4">Shared Notes</h1>

    {% if shares|length == 0 %}
    <p class="text-stone-600">No notes are currently shared.</p>
    {% else %}
    <form method="post" action="/v1/admin/shares/bulk-revoke"
          x-data="confirmAction({ message: 'Revoke all selected share tokens?' })"
          x-bind="events">
        <table class="w-full text-sm" data-testid="admin-shares-table">
            <thead>
                <tr class="text-left border-b">
                    <th class="p-2"><input type="checkbox" aria-label="Select all" @change="selectAll($event.target.checked)"></th>
                    <th class="p-2">Name</th>
                    <th class="p-2">Public URL</th>
                    <th class="p-2">Created</th>
                    <th class="p-2">Revoke</th>
                </tr>
            </thead>
            <tbody>
                {% for note in shares %}
                <tr data-share-note-id="{{ note.ID }}" class="border-b">
                    <td class="p-2">
                        <input type="checkbox" name="ids" value="{{ note.ID }}" aria-label="Select {{ note.Name|escapejs }}">
                    </td>
                    <td class="p-2">
                        <a href="/note?id={{ note.ID }}" class="text-amber-700 hover:underline">{{ note.Name }}</a>
                    </td>
                    <td class="p-2 font-mono text-xs break-all">
                        {% if shareUrlConfigured %}
                        <a href="{{ shareBaseUrl }}/s/{{ note.ShareToken }}" target="_blank" rel="noopener">{{ shareBaseUrl }}/s/{{ note.ShareToken }}</a>
                        {% else %}
                        <span class="text-amber-700">(URL base not configured — set SHARE_PUBLIC_URL)</span>
                        <code>/s/{{ note.ShareToken }}</code>
                        {% endif %}
                    </td>
                    <td class="p-2 text-stone-600">
                        {% if note.ShareCreatedAt %}{{ note.ShareCreatedAt|date:"2006-01-02 15:04" }}{% else %}(unknown){% endif %}
                    </td>
                    <td class="p-2">
                        <form method="post" action="/v1/note/share?id={{ note.ID }}&_method=DELETE"
                              x-data="confirmAction({ message: 'Revoke share for \"{{ note.Name|escapejs }}\"?' })"
                              x-bind="events" class="inline">
                            <button type="submit" class="text-red-700 hover:text-red-900 text-xs">Revoke</button>
                        </form>
                    </td>
                </tr>
                {% endfor %}
            </tbody>
        </table>
        <div class="mt-4">
            <button type="submit" class="px-3 py-1 text-sm bg-red-700 text-white rounded hover:bg-red-800">
                Revoke Selected
            </button>
        </div>
    </form>
    {% endif %}
</section>
{% endblock %}
```

### Task C5: Build + E2E + commit

```bash
npm run build
cd e2e && npx playwright test c9-bh035-admin-shares-dashboard --reporter=line
```

Expected: PASS.

```bash
git add models/note_model.go application_context/ \
  server/template_handlers/admin_shares_handler.go \
  server/routes.go templates/adminShares.tpl \
  public/dist/ public/tailwind.css \
  e2e/tests/c9-bh035-admin-shares-dashboard.spec.ts \
  server/api_tests/admin_shares_test.go
git commit -m "feat(share): BH-035 — centralized /admin/shares dashboard + shareCreatedAt

New nullable ShareCreatedAt column on Note (GORM auto-migrates on
startup). Set when MintShareToken runs; cleared on revoke. Existing
rows get NULL — the UI renders '(unknown)' rather than back-filling
with inaccurate NOW().

New GET /admin/shares page listing every note with an active token.
Columns: Name | Public URL | Created | Revoke. Revoke via per-row
button (reuses DELETE /v1/note/share). Bulk-revoke via
POST /v1/admin/shares/bulk-revoke.

E2E: e2e/tests/c9-bh035-admin-shares-dashboard.spec.ts."
```

---

## Task E: Update log, open PR, merge, backfill, cleanup

Mark BH-032/033/035/038 FIXED. PR title: `fix(bughunt c9): BH-032/033/035/038 share surface hardening`. Expect this PR to be the largest of the batch — multiple commits within it by design.

---

## Self-review checklist

- [ ] Security headers present on share-server responses (success + error)
- [ ] Same headers on primary server (separate commit, rollback-safe)
- [ ] `SHARE_PUBLIC_URL` unset → warning + relative path, no bind-address URL
- [ ] `SHARE_PUBLIC_URL` set → absolute URL uses it
- [ ] `shareCreatedAt` set on new token mints, cleared on revoke
- [ ] Existing NULL rows render "(unknown)" — no back-fill
- [ ] `/admin/shares` lists, single-revokes, bulk-revokes
- [ ] `/notes?shared=true` body contains NO plaintext `shareToken` values
- [ ] `hasShare` boolean replaces `shareToken` in notes-list card template
- [ ] `--share-public-url` documented in CLAUDE.md
- [ ] Postgres migration applies cleanly (run `test:with-server:postgres`)
