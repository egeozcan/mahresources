# Cluster 8 — Share Server Block-State Allowlist (BH-031)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Solo subagent, smallest cluster. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Prevent anonymous share-link holders from persisting arbitrary state to non-todo blocks.

**Architecture:** In `server/share_server.go`'s `handleBlockStateUpdate`, add a block-type allowlist (`{"todos": true}`) after the existing note/block-membership checks. Non-matching types return 403.

**Tech Stack:** Go, net/http, existing share-server test scaffolding.

**Worktree branch:** `bugfix/c8-share-allowlist`

---

## File structure

**Modified:**
- `server/share_server.go` — block-type allowlist in `handleBlockStateUpdate`

**Created:**
- `server/api_tests/share_server_block_state_allowlist_test.go`

---

## Task 1: Failing API test

**Files:**
- Create: `server/api_tests/share_server_block_state_allowlist_test.go`

- [ ] **Step 1: Find the existing share-server test patterns**

```bash
grep -rn "share_server\|ShareServer\|handleBlockStateUpdate\|shareToken" server/api_tests/ | head -20
```

Check whether an existing test file already seeds a shared note with a specific block type. Reuse the seeding helper if present; otherwise follow the test patterns in `share_server.go` itself.

- [ ] **Step 2: Write the failing test**

```go
package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

func TestShareBlockState_RejectsNonTodoBlocks(t *testing.T) {
	tc := SetupTestEnv(t)
	shareRouter, shareBaseUrl := setupShareServer(t, tc) // existing helper or equivalent
	_ = shareBaseUrl

	// Create a note and share it.
	note := tc.CreateDummyNote("bh031-note")
	token := tc.ShareNote(note.ID) // helper; or construct via API

	// Add a gallery block to the note.
	galleryContent, _ := json.Marshal(map[string]any{"resourceIds": []int{}})
	gallery := &models.NoteBlock{
		NoteID:  note.ID,
		Type:    "gallery",
		Content: string(galleryContent),
	}
	require.NoError(t, tc.DB.Create(gallery).Error)

	// Attempt to write state to the gallery block via the share server.
	stateBody, _ := json.Marshal(map[string]any{"layout": "list", "injected": true})
	url := fmt.Sprintf("/s/%s/block/%d/state", token, gallery.ID)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(stateBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	shareRouter.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code,
		"BH-031: gallery block state write via share token must be rejected with 403")
}

func TestShareBlockState_AllowsTodoBlocks(t *testing.T) {
	tc := SetupTestEnv(t)
	shareRouter, _ := setupShareServer(t, tc)

	note := tc.CreateDummyNote("bh031-note-todo")
	token := tc.ShareNote(note.ID)

	todosContent, _ := json.Marshal(map[string]any{"items": []map[string]any{{"id": "t1", "checked": false}}})
	todos := &models.NoteBlock{NoteID: note.ID, Type: "todos", Content: string(todosContent)}
	require.NoError(t, tc.DB.Create(todos).Error)

	newState := []byte(`{"items":[{"id":"t1","checked":true}]}`)
	url := fmt.Sprintf("/s/%s/block/%d/state", token, todos.ID)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(newState))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	shareRouter.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code,
		"todos block state write must still succeed")
}
```

**Helper functions**

Before writing the test body above, grep for existing equivalents:

```bash
grep -rn "setupShareServer\|shareRouter\|ShareNote\|shareToken\|ShareServer" server/api_tests/ server/*.go | head -20
```

If a share-server test harness already exists (look in `server/api_tests/` for any file with `share` in the name), use it. Otherwise add these helpers to `api_test_utils.go`:

```go
// Returns the share server's http.Handler and a fake base URL for testing.
func setupShareServer(t *testing.T, tc *TestContext) (http.Handler, string) {
    srv := server.NewShareServer(tc.AppCtx) // actual constructor name per server/share_server.go
    return srv.Handler(), "http://127.0.0.1:18383"
}

// Creates a share token for a note via the primary API.
func (tc *TestContext) ShareNote(noteID uint) string {
    form := url.Values{}
    form.Set("NoteId", fmt.Sprintf("%d", noteID))
    rr := tc.MakeFormRequest(http.MethodPost, "/v1/note/share", form)
    if rr.Code != http.StatusOK {
        t := tc.AppCtx
        _ = t
        panic(fmt.Sprintf("ShareNote failed: HTTP %d body=%s", rr.Code, rr.Body.String()))
    }
    var body map[string]any
    json.Unmarshal(rr.Body.Bytes(), &body)
    token, _ := body["token"].(string)
    if token == "" {
        token, _ = body["shareToken"].(string) // fallback naming
    }
    return token
}
```

Substitute the actual `NewShareServer` constructor name found in `server/share_server.go` (likely `NewShareServer(ctx)` or `server.CreateShareServer(...)` — grep to confirm).

- [ ] **Step 3: Run 3× — expect fail (gallery state write currently returns 200)**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestShareBlockState -v -count=3
```

Expected: 3× FAIL on the gallery case (status 200 instead of 403). The todo case may already pass.

## Task 2: Add the allowlist check

**Files:**
- Modify: `server/share_server.go:131-173` (inside `handleBlockStateUpdate`)

- [ ] **Step 1: Read current handler**

```bash
cat server/share_server.go | sed -n '125,180p'
```

- [ ] **Step 2: Add the block-type check after the note/block-membership validation**

```go
// server/share_server.go, inside handleBlockStateUpdate, after resolving `note`
// and before calling UpdateBlockStateFromRequest:
var allowedStateTypes = map[string]bool{
    "todos": true,
    // BH-031: todos is the only block type intended for share-side state writes.
    // Future: expose specific calendar view-state if needed, via explicit opt-in.
}

var targetBlock *models.NoteBlock
for i := range note.Blocks {
    if note.Blocks[i].ID == blockId {
        targetBlock = &note.Blocks[i]
        break
    }
}
if targetBlock == nil || !allowedStateTypes[targetBlock.Type] {
    http.Error(w, "Block type does not allow share-token state writes", http.StatusForbidden)
    return
}
```

Ensure this check runs AFTER the token→note resolution (preventing type-confusion probes from revealing block-existence info on invalid tokens), and BEFORE any state-write operation.

- [ ] **Step 3: Run tests 3× to verify pass**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestShareBlockState -v -count=3
```

Expected: PASS all 3 runs.

- [ ] **Step 4: Run full share-server test set to confirm no regression**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run '(?i)share' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/share_server.go server/api_tests/share_server_block_state_allowlist_test.go
git commit -m "fix(share): BH-031 — block-type allowlist on share-token state writes (todos only)"
```

---

## Cluster PR gate

- [ ] **Step 1: Full Go**

```bash
cd <worktree>
go test --tags 'json1 fts5' ./...
```

- [ ] **Step 2: Rebase + full E2E + Postgres**

- [ ] **Step 3: Open PR, self-merge**

```bash
gh pr create --title "fix(share): BH-031 — block-state write allowlist" --body "$(cat <<'EOF'
Closes BH-031.

## Changes

- `server/share_server.go` — `handleBlockStateUpdate` now resolves the target block's type after note/ownership checks and requires it to be in the `{todos}` allowlist. Non-matching types return 403.

## Tests

- Go API: ✓ gallery-block state write via share token → 403; todos-block state write → 200. Pass 3× pre red / post green.
- Full Go suite: ✓
- Full E2E: ✓
- Postgres: ✓

## Security note

Previously, any holder of a share token could persist arbitrary JSON state to any block type in a shared note. Impact was bounded to the block's `state` column (not content) but represented vandalism and integrity risk.

## Bug-hunt-log update

Post-merge: BH-031 → Fixed / closed.
EOF
)"
gh pr merge --merge --delete-branch
```

Then master plan Step F.
