# Note Sharing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a public share server that exposes selected notes via cryptographically random URLs with interactive block support.

**Architecture:** Two HTTP servers in one process - main server (local-only) and share server (network-accessible). Notes get a `ShareToken` field; share server validates tokens and serves minimal read-only views with interactive todos.

**Tech Stack:** Go (Gorilla Mux, GORM), Pongo2 templates, Alpine.js, Tailwind CSS, crypto/rand for tokens.

---

## Task 1: Add ShareToken Field to Note Model

**Files:**
- Modify: `models/note_model.go:9-26`

**Step 1: Add the ShareToken field**

Open `models/note_model.go` and add the ShareToken field after line 24 (after `NoteTypeId`):

```go
type Note struct {
	ID          uint      `gorm:"primarykey"`
	CreatedAt   time.Time `gorm:"index"`
	UpdatedAt   time.Time `gorm:"index"`
	Name        string    `gorm:"index"`
	Description string
	Meta        types.JSON
	Tags        []*Tag      `gorm:"many2many:note_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Resources   []*Resource `gorm:"many2many:resource_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Groups      []*Group    `gorm:"many2many:groups_related_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Owner       *Group      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	OwnerId     *uint
	StartDate   *time.Time
	EndDate     *time.Time
	NoteType    *NoteType `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	NoteTypeId  *uint
	ShareToken  *string      `gorm:"uniqueIndex;size:32" json:"shareToken,omitempty"`
	Blocks      []*NoteBlock `gorm:"foreignKey:NoteID" json:"blocks,omitempty"`
}
```

**Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds with no errors

**Step 3: Commit**

```bash
git add models/note_model.go
git commit -m "feat(models): add ShareToken field to Note model"
```

---

## Task 2: Add Shared Filter to Note Query Model

**Files:**
- Modify: `models/query_models/note_query.go:21-37`

**Step 1: Add Shared filter field**

Open `models/query_models/note_query.go` and add the Shared field at the end of NoteQuery struct:

```go
type NoteQuery struct {
	Name            string
	Description     string
	OwnerId         uint
	Groups          []uint
	Tags            []uint
	CreatedBefore   string
	CreatedAfter    string
	StartDateBefore string
	StartDateAfter  string
	EndDateBefore   string
	EndDateAfter    string
	SortBy          []string
	Ids             []uint
	MetaQuery       []ColumnMeta
	NoteTypeId      uint
	Shared          *bool
}
```

**Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add models/query_models/note_query.go
git commit -m "feat(models): add Shared filter to NoteQuery"
```

---

## Task 3: Add Shared Filter to Database Scope

**Files:**
- Modify: `models/database_scopes/note_scope.go`

**Step 1: Add the shared filter logic**

Open `models/database_scopes/note_scope.go` and add the shared filter before the return statement in the `NoteQuery` function (around line 100):

```go
	if query.Shared != nil {
		dbQuery = dbQuery.Where("share_token IS NOT NULL")
	}
```

**Step 2: Run existing tests**

Run: `go test ./models/database_scopes/... -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add models/database_scopes/note_scope.go
git commit -m "feat(scopes): add Shared filter to note database scope"
```

---

## Task 4: Add Configuration Flags

**Files:**
- Modify: `main.go`

**Step 1: Add share server config fields to MahresourcesConfig struct**

Find `MahresourcesConfig` struct (around line 26) and add after `BindAddress`:

```go
	SharePort        string
	ShareBindAddress string
```

**Step 2: Add share server config fields to MahresourcesInputConfig struct**

Find `MahresourcesInputConfig` struct (around line 42) and add after `BindAddress`:

```go
	SharePort        string
	ShareBindAddress string
```

**Step 3: Add flag definitions**

Find flag definitions (around line 67-102) and add before `flag.Parse()`:

```go
	sharePort := flag.String("share-port", os.Getenv("SHARE_PORT"), "Port for public share server (env: SHARE_PORT)")
	shareBindAddress := flag.String("share-bind-address", getEnvOrDefault("SHARE_BIND_ADDRESS", "0.0.0.0"), "Bind address for share server (env: SHARE_BIND_ADDRESS)")
```

**Step 4: Add to config initialization**

Find config initialization (around line 140) and add the new fields:

```go
		SharePort:        *sharePort,
		ShareBindAddress: *shareBindAddress,
```

Also add to InputConfig around line 175:

```go
		SharePort:        *sharePort,
		ShareBindAddress: *shareBindAddress,
```

**Step 5: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add main.go
git commit -m "feat(config): add share server configuration flags"
```

---

## Task 5: Create Share Token Generation Utility

**Files:**
- Create: `lib/token.go`
- Create: `lib/token_test.go`

**Step 1: Write the failing test**

Create `lib/token_test.go`:

```go
package lib

import (
	"testing"
)

func TestGenerateShareToken(t *testing.T) {
	token := GenerateShareToken()

	if len(token) != 32 {
		t.Errorf("Expected token length 32, got %d", len(token))
	}

	// Verify it's hex characters only
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Token contains non-hex character: %c", c)
		}
	}
}

func TestGenerateShareTokenUniqueness(t *testing.T) {
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token := GenerateShareToken()
		if tokens[token] {
			t.Errorf("Duplicate token generated: %s", token)
		}
		tokens[token] = true
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./lib/... -v -run TestGenerateShareToken`
Expected: FAIL with "undefined: GenerateShareToken"

**Step 3: Write the implementation**

Create `lib/token.go`:

```go
package lib

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateShareToken generates a cryptographically secure 32-character hex token
func GenerateShareToken() string {
	bytes := make([]byte, 16) // 16 bytes = 128 bits = 32 hex chars
	if _, err := rand.Read(bytes); err != nil {
		panic(err) // crypto/rand should never fail
	}
	return hex.EncodeToString(bytes)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./lib/... -v -run TestGenerateShareToken`
Expected: PASS

**Step 5: Commit**

```bash
git add lib/token.go lib/token_test.go
git commit -m "feat(lib): add cryptographic share token generation"
```

---

## Task 6: Add NoteSharer Interface

**Files:**
- Modify: `server/interfaces/note_interfaces.go`

**Step 1: Add the NoteSharer interface**

Open `server/interfaces/note_interfaces.go` and add after the existing interfaces:

```go
type NoteSharer interface {
	ShareNote(noteId uint) (string, error)
	UnshareNote(noteId uint) error
	GetNoteByShareToken(token string) (*models.Note, error)
}
```

**Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add server/interfaces/note_interfaces.go
git commit -m "feat(interfaces): add NoteSharer interface"
```

---

## Task 7: Implement Share Methods in Application Context

**Files:**
- Modify: `application_context/note_context.go`

**Step 1: Add the ShareNote method**

Open `application_context/note_context.go` and add after the `DeleteNote` function:

```go
func (ctx *MahresourcesContext) ShareNote(noteId uint) (string, error) {
	var note models.Note
	if err := ctx.db.First(&note, noteId).Error; err != nil {
		return "", err
	}

	// If already shared, return existing token
	if note.ShareToken != nil {
		return *note.ShareToken, nil
	}

	token := lib.GenerateShareToken()
	if err := ctx.db.Model(&note).Update("share_token", token).Error; err != nil {
		return "", err
	}

	ctx.Logger().Info(models.LogActionUpdate, "note", &noteId, note.Name, "Created share token", nil)
	return token, nil
}
```

**Step 2: Add the UnshareNote method**

```go
func (ctx *MahresourcesContext) UnshareNote(noteId uint) error {
	var note models.Note
	if err := ctx.db.First(&note, noteId).Error; err != nil {
		return err
	}

	if err := ctx.db.Model(&note).Update("share_token", nil).Error; err != nil {
		return err
	}

	ctx.Logger().Info(models.LogActionUpdate, "note", &noteId, note.Name, "Removed share token", nil)
	return nil
}
```

**Step 3: Add the GetNoteByShareToken method**

```go
func (ctx *MahresourcesContext) GetNoteByShareToken(token string) (*models.Note, error) {
	if token == "" {
		return nil, errors.New("share token required")
	}

	var note models.Note
	err := ctx.db.
		Preload("Blocks", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		Preload("Resources").
		Preload("NoteType").
		Where("share_token = ?", token).
		First(&note).Error

	if err != nil {
		return nil, err
	}

	return &note, nil
}
```

**Step 4: Add import for lib package**

Add to imports at top of file:

```go
import (
	// ... existing imports
	"mahresources/lib"
)
```

**Step 5: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add application_context/note_context.go
git commit -m "feat(context): implement share/unshare methods for notes"
```

---

## Task 8: Write Share API Handler Tests

**Files:**
- Modify: `server/api_tests/note_api_test.go`

**Step 1: Add share API tests**

Open `server/api_tests/note_api_test.go` and add new test functions:

```go
func TestShareNote(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("Test Share Note")

	t.Run("Share note creates token", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/share?id=%d", note.ID)
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var result map[string]interface{}
		json.Unmarshal(resp.Body.Bytes(), &result)
		assert.NotEmpty(t, result["shareToken"])
		assert.NotEmpty(t, result["shareUrl"])
	})

	t.Run("Share note returns same token on repeat", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/share?id=%d", note.ID)
		resp1 := tc.MakeRequest(http.MethodPost, url, nil)
		resp2 := tc.MakeRequest(http.MethodPost, url, nil)

		var result1, result2 map[string]interface{}
		json.Unmarshal(resp1.Body.Bytes(), &result1)
		json.Unmarshal(resp2.Body.Bytes(), &result2)
		assert.Equal(t, result1["shareToken"], result2["shareToken"])
	})

	t.Run("Unshare note removes token", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/share?id=%d", note.ID)
		tc.MakeRequest(http.MethodDelete, url, nil)

		// Verify note is no longer shared
		updatedNote, _ := tc.AppCtx.GetNote(note.ID)
		assert.Nil(t, updatedNote.ShareToken)
	})

	t.Run("Share nonexistent note returns error", func(t *testing.T) {
		url := "/v1/note/share?id=99999"
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server/api_tests/... -v -run TestShareNote`
Expected: FAIL (endpoint doesn't exist yet)

**Step 3: Commit the failing test**

```bash
git add server/api_tests/note_api_test.go
git commit -m "test: add share note API tests (failing)"
```

---

## Task 9: Create Share API Handlers

**Files:**
- Create: `server/api_handlers/share_handlers.go`

**Step 1: Create the share handlers file**

Create `server/api_handlers/share_handlers.go`:

```go
package api_handlers

import (
	"encoding/json"
	"net/http"

	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
)

type ShareResponse struct {
	ShareToken string `json:"shareToken"`
	ShareUrl   string `json:"shareUrl"`
}

func GetShareNoteHandler(ctx interfaces.NoteSharer) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		noteId, err := http_utils.GetUIntQueryParameter(request, "id")
		if err != nil {
			http_utils.HandleError(writer, err, http.StatusBadRequest)
			return
		}

		token, err := ctx.ShareNote(noteId)
		if err != nil {
			if err.Error() == "record not found" {
				http_utils.HandleError(writer, err, http.StatusNotFound)
				return
			}
			http_utils.HandleError(writer, err, http.StatusInternalServerError)
			return
		}

		shareUrl := "/s/" + token
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(ShareResponse{
			ShareToken: token,
			ShareUrl:   shareUrl,
		})
	}
}

func GetUnshareNoteHandler(ctx interfaces.NoteSharer) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		noteId, err := http_utils.GetUIntQueryParameter(request, "id")
		if err != nil {
			http_utils.HandleError(writer, err, http.StatusBadRequest)
			return
		}

		err = ctx.UnshareNote(noteId)
		if err != nil {
			if err.Error() == "record not found" {
				http_utils.HandleError(writer, err, http.StatusNotFound)
				return
			}
			http_utils.HandleError(writer, err, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]bool{"success": true})
	}
}
```

**Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add server/api_handlers/share_handlers.go
git commit -m "feat(api): add share/unshare note handlers"
```

---

## Task 10: Register Share API Routes

**Files:**
- Modify: `server/routes.go`

**Step 1: Add share routes**

Open `server/routes.go` and add the share routes in the `registerRoutes` function (around line 247, before the closing brace):

```go
	// Note sharing routes
	router.Methods(http.MethodPost).Path("/v1/note/share").HandlerFunc(api_handlers.GetShareNoteHandler(appContext))
	router.Methods(http.MethodDelete).Path("/v1/note/share").HandlerFunc(api_handlers.GetUnshareNoteHandler(appContext))
```

**Step 2: Run share tests**

Run: `go test ./server/api_tests/... -v -run TestShareNote`
Expected: All tests pass

**Step 3: Commit**

```bash
git add server/routes.go
git commit -m "feat(routes): register share/unshare API endpoints"
```

---

## Task 11: Create Share Server

**Files:**
- Create: `server/share_server.go`

**Step 1: Create the share server file**

Create `server/share_server.go`:

```go
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"mahresources/application_context"
)

type ShareServer struct {
	server     *http.Server
	appContext *application_context.MahresourcesContext
}

func NewShareServer(appContext *application_context.MahresourcesContext) *ShareServer {
	return &ShareServer{
		appContext: appContext,
	}
}

func (s *ShareServer) Start(bindAddress string, port string) error {
	if port == "" {
		return nil // Share server disabled
	}

	router := mux.NewRouter()
	s.registerShareRoutes(router)

	addr := fmt.Sprintf("%s:%s", bindAddress, port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Share server starting on %s", addr)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Share server error: %v", err)
		}
	}()

	return nil
}

func (s *ShareServer) Stop() error {
	if s.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *ShareServer) registerShareRoutes(router *mux.Router) {
	// Shared note view
	router.Methods(http.MethodGet).Path("/s/{token}").HandlerFunc(s.handleSharedNote)

	// Block state update (for interactive todos)
	router.Methods(http.MethodPut).Path("/s/{token}/block/{blockId}/state").HandlerFunc(s.handleBlockStateUpdate)

	// Resource serving (for gallery images)
	router.Methods(http.MethodGet).Path("/s/{token}/resource/{hash}").HandlerFunc(s.handleSharedResource)

	// Static assets
	router.PathPrefix("/public/").Handler(http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))
}

func (s *ShareServer) handleSharedNote(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]

	note, err := s.appContext.GetNoteByShareToken(token)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Render the shared note template
	s.renderSharedNote(w, note)
}

func (s *ShareServer) handleBlockStateUpdate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]
	blockIdStr := vars["blockId"]

	// Verify token and get note
	note, err := s.appContext.GetNoteByShareToken(token)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Parse block ID
	var blockId uint
	fmt.Sscanf(blockIdStr, "%d", &blockId)

	// Verify block belongs to this note
	blockBelongsToNote := false
	for _, block := range note.Blocks {
		if block.ID == blockId {
			blockBelongsToNote = true
			break
		}
	}
	if !blockBelongsToNote {
		http.Error(w, "Block not found", http.StatusNotFound)
		return
	}

	// Update block state
	err = s.appContext.UpdateBlockStateFromRequest(blockId, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true}`))
}

func (s *ShareServer) handleSharedResource(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]
	hash := vars["hash"]

	// Verify token
	note, err := s.appContext.GetNoteByShareToken(token)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Verify resource belongs to this note
	resourceBelongsToNote := false
	for _, resource := range note.Resources {
		if resource.Hash == hash {
			resourceBelongsToNote = true
			break
		}
	}
	if !resourceBelongsToNote {
		http.Error(w, "Resource not found", http.StatusNotFound)
		return
	}

	// Serve the resource
	s.appContext.ServeResourceByHash(w, r, hash)
}

func (s *ShareServer) renderSharedNote(w http.ResponseWriter, note interface{}) {
	// Placeholder - will be implemented with template rendering
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<html><body><h1>Shared Note</h1><p>Template rendering coming soon</p></body></html>"))
}
```

**Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds (may have some undefined methods - we'll fix those)

**Step 3: Commit**

```bash
git add server/share_server.go
git commit -m "feat(server): create share server skeleton"
```

---

## Task 12: Add Missing Context Methods for Share Server

**Files:**
- Modify: `application_context/note_context.go`
- Modify: `application_context/block_context.go`
- Modify: `application_context/resource_context.go`

**Step 1: Add UpdateBlockStateFromRequest method**

Open `application_context/block_context.go` and add:

```go
func (ctx *MahresourcesContext) UpdateBlockStateFromRequest(blockId uint, r *http.Request) error {
	var stateUpdate map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&stateUpdate); err != nil {
		return err
	}

	return ctx.UpdateBlockState(blockId, stateUpdate)
}
```

Add to imports:
```go
import (
	"encoding/json"
	"net/http"
)
```

**Step 2: Add ServeResourceByHash method**

Open `application_context/resource_context.go` and add:

```go
func (ctx *MahresourcesContext) ServeResourceByHash(w http.ResponseWriter, r *http.Request, hash string) {
	resource, err := ctx.GetResourceByHash(hash)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	file, err := ctx.Fs.Open(resource.GetPathOnDisk())
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", resource.ContentType)
	http.ServeContent(w, r, resource.Name, resource.UpdatedAt, file)
}
```

Add to imports:
```go
import (
	"net/http"
)
```

**Step 3: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add application_context/block_context.go application_context/resource_context.go
git commit -m "feat(context): add helper methods for share server"
```

---

## Task 13: Integrate Share Server into Main

**Files:**
- Modify: `main.go`

**Step 1: Start share server in main**

Open `main.go` and find where the main server starts (around line 200). Add after server initialization:

```go
	// Start share server if configured
	if config.SharePort != "" {
		shareServer := server.NewShareServer(appContext)
		if err := shareServer.Start(config.ShareBindAddress, config.SharePort); err != nil {
			log.Fatalf("Failed to start share server: %v", err)
		}
		defer shareServer.Stop()
		log.Printf("Share server available at http://%s:%s", config.ShareBindAddress, config.SharePort)
	}
```

**Step 2: Test manually**

Run: `go run --tags 'json1 fts5' main.go -ephemeral -share-port=8383`
Expected: See log message "Share server starting on 0.0.0.0:8383"

**Step 3: Commit**

```bash
git add main.go
git commit -m "feat: integrate share server startup into main"
```

---

## Task 14: Create Shared Note Template

**Files:**
- Create: `templates/shared/base.tpl`
- Create: `templates/shared/displayNote.tpl`

**Step 1: Create base template**

Create `templates/shared/base.tpl`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ pageTitle }}</title>
    <link rel="stylesheet" href="/public/tailwind.css">
    <link rel="stylesheet" href="/public/index.css">
</head>
<body class="bg-gray-50 min-h-screen">
    <div class="max-w-4xl mx-auto py-8 px-4">
        {% block content %}{% endblock %}
    </div>
    <script src="/public/dist/main.js" defer></script>
</body>
</html>
```

**Step 2: Create shared note display template**

Create `templates/shared/displayNote.tpl`:

```html
{% extends "shared/base.tpl" %}

{% block content %}
<article class="bg-white rounded-lg shadow-sm p-6">
    <header class="mb-6">
        <h1 class="text-2xl font-bold text-gray-900">{{ note.Name }}</h1>
        {% if note.Description %}
        <div class="mt-4 prose prose-sm max-w-none text-gray-600">
            {{ note.Description|safe }}
        </div>
        {% endif %}
    </header>

    {% if note.Blocks %}
    <div class="space-y-4" x-data="{ shareToken: '{{ shareToken }}' }">
        {% for block in note.Blocks %}
            {% include "partials/blocks/sharedBlock.tpl" %}
        {% endfor %}
    </div>
    {% endif %}
</article>

<footer class="mt-8 text-center text-sm text-gray-500">
    Shared via <a href="https://github.com/your/mahresources" class="text-blue-600 hover:underline">Mahresources</a>
</footer>
{% endblock %}
```

**Step 3: Create shared block partial**

Create `templates/partials/blocks/sharedBlock.tpl`:

```html
{% if block.Type == "text" %}
    <div class="prose prose-sm max-w-none">
        {{ block.Content.text|default:""|safe }}
    </div>
{% elif block.Type == "heading" %}
    {% if block.Content.level == 1 %}
        <h2 class="text-xl font-bold text-gray-900">{{ block.Content.text }}</h2>
    {% elif block.Content.level == 2 %}
        <h3 class="text-lg font-semibold text-gray-900">{{ block.Content.text }}</h3>
    {% else %}
        <h4 class="text-base font-medium text-gray-900">{{ block.Content.text }}</h4>
    {% endif %}
{% elif block.Type == "divider" %}
    <hr class="border-gray-200">
{% elif block.Type == "todos" %}
    <div class="space-y-2" x-data="sharedTodos({{ block.ID }}, {{ block.State|json }}, '{{ shareToken }}')">
        {% for item in block.Content.items %}
        <label class="flex items-center gap-2 cursor-pointer">
            <input
                type="checkbox"
                :checked="isChecked('{{ item.id }}')"
                @change="toggleItem('{{ item.id }}')"
                class="w-4 h-4 text-blue-600 rounded border-gray-300 focus:ring-blue-500"
            >
            <span :class="{ 'line-through text-gray-400': isChecked('{{ item.id }}') }">
                {{ item.label }}
            </span>
        </label>
        {% endfor %}
    </div>
{% elif block.Type == "gallery" %}
    <div class="grid grid-cols-2 md:grid-cols-3 gap-4">
        {% for resourceId in block.Content.resourceIds %}
        <img
            src="/s/{{ shareToken }}/resource/{{ resourceId }}"
            alt="Gallery image"
            class="w-full h-48 object-cover rounded-lg"
        >
        {% endfor %}
    </div>
{% endif %}
```

**Step 4: Commit**

```bash
git add templates/shared/
git commit -m "feat(templates): add shared note display templates"
```

---

## Task 15: Add Shared Todos Alpine Component

**Files:**
- Create: `src/components/sharedTodos.js`
- Modify: `src/main.js`

**Step 1: Create sharedTodos component**

Create `src/components/sharedTodos.js`:

```javascript
export function sharedTodos(blockId, initialState, shareToken) {
    return {
        checked: initialState?.checked || [],
        blockId: blockId,
        shareToken: shareToken,

        isChecked(itemId) {
            return this.checked.includes(itemId);
        },

        async toggleItem(itemId) {
            // Optimistic update
            if (this.isChecked(itemId)) {
                this.checked = this.checked.filter(id => id !== itemId);
            } else {
                this.checked = [...this.checked, itemId];
            }

            // Save to server
            try {
                const response = await fetch(`/s/${this.shareToken}/block/${this.blockId}/state`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ checked: this.checked })
                });

                if (!response.ok) {
                    // Revert on error
                    if (this.isChecked(itemId)) {
                        this.checked = this.checked.filter(id => id !== itemId);
                    } else {
                        this.checked = [...this.checked, itemId];
                    }
                }
            } catch (error) {
                console.error('Failed to save todo state:', error);
            }
        }
    };
}
```

**Step 2: Register in main.js**

Open `src/main.js` and add the import and registration:

```javascript
import { sharedTodos } from './components/sharedTodos.js';

// In Alpine initialization section
Alpine.data('sharedTodos', sharedTodos);
```

**Step 3: Rebuild JS**

Run: `npm run build-js`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add src/components/sharedTodos.js src/main.js
git commit -m "feat(frontend): add sharedTodos Alpine component"
```

---

## Task 16: Update Share Server to Use Templates

**Files:**
- Modify: `server/share_server.go`

**Step 1: Update renderSharedNote to use Pongo2**

Replace the placeholder `renderSharedNote` method:

```go
func (s *ShareServer) renderSharedNote(w http.ResponseWriter, note *models.Note, token string) {
	tpl, err := pongo2.FromFile("templates/shared/displayNote.tpl")
	if err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	ctx := pongo2.Context{
		"pageTitle":  "Shared: " + note.Name,
		"note":       note,
		"shareToken": token,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tpl.ExecuteWriter(ctx, w); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}
```

**Step 2: Update handleSharedNote to pass token**

```go
func (s *ShareServer) handleSharedNote(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]

	note, err := s.appContext.GetNoteByShareToken(token)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	s.renderSharedNote(w, note, token)
}
```

**Step 3: Add pongo2 import**

```go
import (
	"github.com/flosch/pongo2/v6"
)
```

**Step 4: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add server/share_server.go
git commit -m "feat(server): integrate Pongo2 template rendering in share server"
```

---

## Task 17: Add Share Button to Note UI

**Files:**
- Modify: `templates/displayNote.tpl`
- Create: `src/components/shareButton.js`

**Step 1: Create shareButton component**

Create `src/components/shareButton.js`:

```javascript
export function shareButton(noteId, currentToken) {
    return {
        noteId: noteId,
        shareToken: currentToken || null,
        isLoading: false,

        get isShared() {
            return this.shareToken !== null;
        },

        get shareUrl() {
            if (!this.shareToken) return '';
            return window.location.origin.replace(/:\d+$/, ':' + (window.sharePort || '8383')) + '/s/' + this.shareToken;
        },

        async share() {
            this.isLoading = true;
            try {
                const response = await fetch(`/v1/note/share?id=${this.noteId}`, {
                    method: 'POST'
                });
                const data = await response.json();
                this.shareToken = data.shareToken;
                await this.copyToClipboard();
            } catch (error) {
                console.error('Failed to share:', error);
                alert('Failed to create share link');
            } finally {
                this.isLoading = false;
            }
        },

        async unshare() {
            if (!confirm('Remove the public share link? Anyone with the link will no longer be able to access this note.')) {
                return;
            }

            this.isLoading = true;
            try {
                await fetch(`/v1/note/share?id=${this.noteId}`, {
                    method: 'DELETE'
                });
                this.shareToken = null;
            } catch (error) {
                console.error('Failed to unshare:', error);
                alert('Failed to remove share link');
            } finally {
                this.isLoading = false;
            }
        },

        async copyToClipboard() {
            try {
                await navigator.clipboard.writeText(this.shareUrl);
                alert('Share link copied to clipboard!');
            } catch (error) {
                prompt('Copy this share link:', this.shareUrl);
            }
        }
    };
}
```

**Step 2: Register in main.js**

Open `src/main.js` and add:

```javascript
import { shareButton } from './components/shareButton.js';

Alpine.data('shareButton', shareButton);
```

**Step 3: Add to displayNote.tpl**

Open `templates/displayNote.tpl` and add the share button in the sidebar (find the appropriate location based on existing template):

```html
<div class="mt-4" x-data="shareButton({{ note.ID }}, {{ note.ShareToken|json }})">
    <template x-if="!isShared">
        <button
            @click="share()"
            :disabled="isLoading"
            class="w-full px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50"
        >
            <span x-show="!isLoading">Share Note</span>
            <span x-show="isLoading">Creating link...</span>
        </button>
    </template>

    <template x-if="isShared">
        <div class="space-y-2">
            <div class="flex items-center gap-2 p-2 bg-green-50 rounded-md border border-green-200">
                <svg class="w-4 h-4 text-green-600" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/>
                </svg>
                <span class="text-sm text-green-800">Shared</span>
            </div>
            <button
                @click="copyToClipboard()"
                class="w-full px-3 py-1.5 text-sm text-blue-600 border border-blue-300 rounded hover:bg-blue-50"
            >
                Copy Link
            </button>
            <button
                @click="unshare()"
                :disabled="isLoading"
                class="w-full px-3 py-1.5 text-sm text-red-600 border border-red-300 rounded hover:bg-red-50 disabled:opacity-50"
            >
                Unshare
            </button>
        </div>
    </template>
</div>
```

**Step 4: Rebuild JS**

Run: `npm run build-js`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add src/components/shareButton.js src/main.js templates/displayNote.tpl
git commit -m "feat(ui): add share button to note detail page"
```

---

## Task 18: Add Shared Filter to Notes List

**Files:**
- Modify: `templates/listNotes.tpl`
- Modify: `server/template_handlers/template_context_providers/note_template_context.go`

**Step 1: Add shared filter to template context**

Open `server/template_handlers/template_context_providers/note_template_context.go` and update the notes list context provider to parse the shared filter from query params:

```go
// In NotesContextProvider function, add:
sharedParam := request.URL.Query().Get("shared")
var sharedFilter *bool
if sharedParam == "true" {
    t := true
    sharedFilter = &t
}

// Add to query:
query.Shared = sharedFilter
```

**Step 2: Add filter checkbox to listNotes.tpl**

Open `templates/listNotes.tpl` and add a filter checkbox in the filter section:

```html
<label class="flex items-center gap-2">
    <input
        type="checkbox"
        name="shared"
        value="true"
        {% if request.URL.Query.Get("shared") == "true" %}checked{% endif %}
        onchange="this.form.submit()"
    >
    <span class="text-sm">Shared only</span>
</label>
```

**Step 3: Commit**

```bash
git add templates/listNotes.tpl server/template_handlers/template_context_providers/note_template_context.go
git commit -m "feat(ui): add shared filter to notes list"
```

---

## Task 19: Write E2E Tests for Note Sharing

**Files:**
- Create: `e2e/tests/share/note-sharing.spec.ts`

**Step 1: Create E2E test file**

Create `e2e/tests/share/note-sharing.spec.ts`:

```typescript
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Note Sharing', () => {
    test('can share a note and access it via share URL', async ({ page, api }) => {
        // Create a note
        const note = await api.createNote({
            name: 'Test Shared Note',
            description: 'This note will be shared'
        });

        // Go to note detail page
        await page.goto(`/note?id=${note.id}`);

        // Click share button
        await page.click('button:has-text("Share Note")');

        // Should see success message and shared indicator
        await expect(page.locator('text=Shared')).toBeVisible();
        await expect(page.locator('button:has-text("Copy Link")')).toBeVisible();
    });

    test('can unshare a note', async ({ page, api }) => {
        // Create and share a note
        const note = await api.createNote({ name: 'Note to Unshare' });
        await api.shareNote(note.id);

        await page.goto(`/note?id=${note.id}`);

        // Should see shared indicator
        await expect(page.locator('text=Shared')).toBeVisible();

        // Click unshare
        page.on('dialog', dialog => dialog.accept());
        await page.click('button:has-text("Unshare")');

        // Should see share button again
        await expect(page.locator('button:has-text("Share Note")')).toBeVisible();
    });

    test('can filter notes list by shared', async ({ page, api }) => {
        // Create notes
        const sharedNote = await api.createNote({ name: 'Shared Note' });
        const unsharedNote = await api.createNote({ name: 'Unshared Note' });
        await api.shareNote(sharedNote.id);

        // Go to notes list with shared filter
        await page.goto('/notes?shared=true');

        // Should only see shared note
        await expect(page.locator(`text=${sharedNote.name}`)).toBeVisible();
        await expect(page.locator(`text=${unsharedNote.name}`)).not.toBeVisible();
    });
});
```

**Step 2: Add API helper methods**

Open `e2e/helpers/api.ts` and add:

```typescript
async shareNote(noteId: number): Promise<{ shareToken: string; shareUrl: string }> {
    const response = await this.request.post(`/v1/note/share?id=${noteId}`);
    return response.json();
}

async unshareNote(noteId: number): Promise<void> {
    await this.request.delete(`/v1/note/share?id=${noteId}`);
}
```

**Step 3: Commit**

```bash
git add e2e/tests/share/ e2e/helpers/api.ts
git commit -m "test(e2e): add note sharing tests"
```

---

## Task 20: Create Documentation

**Files:**
- Create: `docs-site/docs/features/note-sharing.md`
- Create: `docs-site/docs/deployment/public-sharing.md`
- Modify: `docs-site/docs/configuration/overview.md`
- Modify: `docs-site/sidebars.ts`

**Step 1: Create feature documentation**

Create `docs-site/docs/features/note-sharing.md`:

```markdown
---
sidebar_position: 5
---

# Note Sharing

Share notes publicly via secure, unguessable URLs. Shared notes are read-only but support interactive elements like todo checkboxes.

## How It Works

When you share a note:
1. A cryptographically secure 32-character token is generated
2. The note becomes accessible at `/s/{token}` on the share server
3. The share URL is copied to your clipboard

Anyone with the URL can:
- View the note content and all blocks
- See embedded images and galleries
- Check/uncheck todo items (changes persist globally)

They cannot:
- See tags, groups, or organizational structure
- Edit note content or blocks
- Access other notes or resources

## Sharing a Note

1. Open the note you want to share
2. Click **Share Note** in the sidebar
3. The share URL is automatically copied to your clipboard
4. Share the URL with anyone you want to have access

## Managing Shared Notes

### Copy Share Link
Click **Copy Link** to copy the share URL again.

### Unshare a Note
Click **Unshare** to remove public access. The share URL will immediately stop working.

### Find Shared Notes
Use the **Shared only** filter on the Notes list to see all your shared notes.

## Configuration

The share server must be enabled to use this feature. See [Public Sharing Deployment](../deployment/public-sharing.md) for setup instructions.

## Security

- Share tokens are 128-bit cryptographically random values
- Tokens cannot be guessed or enumerated
- Invalid tokens return a generic 404 (no information leakage)
- Resources are only served if they belong to the shared note
- Block state updates validate the block belongs to the note

:::tip
Share URLs are permanent until you explicitly unshare. Consider unsharing notes when you no longer need them to be public.
:::
```

**Step 2: Create deployment documentation**

Create `docs-site/docs/deployment/public-sharing.md`:

```markdown
---
sidebar_position: 5
---

# Public Sharing

Configure the share server to enable public note sharing.

:::danger Security Warning
The share server is designed to be exposed to the internet. Ensure proper security measures are in place.
:::

## Enabling the Share Server

Add the `-share-port` flag when starting Mahresources:

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./data/mahresources.db \
  -file-save-path=./data/files \
  -bind-address=127.0.0.1:8181 \
  -share-port=8383
```

Or use environment variables:

```bash
SHARE_PORT=8383 ./mahresources
```

## Configuration Options

| Flag | Env Variable | Description | Default |
|------|--------------|-------------|---------|
| `-share-port` | `SHARE_PORT` | Port for share server | (disabled) |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | Bind address | `0.0.0.0` |

## Reverse Proxy Setup

For production, place the share server behind a reverse proxy with HTTPS.

### Nginx Example

```nginx
server {
    listen 443 ssl;
    server_name share.example.com;

    ssl_certificate /etc/letsencrypt/live/share.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/share.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8383;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### Caddy Example

```
share.example.com {
    reverse_proxy 127.0.0.1:8383
}
```

## Rate Limiting

Consider adding rate limiting at the reverse proxy level:

```nginx
limit_req_zone $binary_remote_addr zone=share:10m rate=10r/s;

server {
    location / {
        limit_req zone=share burst=20 nodelay;
        proxy_pass http://127.0.0.1:8383;
    }
}
```

## Monitoring

Monitor shared note access via server logs. The share server logs all requests including the share token and client IP.

## Firewall Rules

If not using a reverse proxy, ensure only the share port is exposed:

```bash
# Allow share server port
ufw allow 8383/tcp

# Keep main server local-only
# (already bound to 127.0.0.1 by default)
```
```

**Step 3: Update configuration overview**

Open `docs-site/docs/configuration/overview.md` and add to the Quick Reference table:

```markdown
| `-share-port` | `SHARE_PORT` | Port for public share server | (disabled) |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | Bind address for share server | `0.0.0.0` |
```

**Step 4: Update sidebars.ts**

Open `docs-site/sidebars.ts` and add the new pages:

In Advanced Features section:
```typescript
'features/note-sharing',
```

In Deployment section:
```typescript
'deployment/public-sharing',
```

**Step 5: Commit**

```bash
git add docs-site/
git commit -m "docs: add note sharing documentation"
```

---

## Task 21: Run All Tests and Final Verification

**Step 1: Run Go tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 2: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass

**Step 3: Manual verification**

1. Start server: `go run --tags 'json1 fts5' main.go -ephemeral -share-port=8383`
2. Create a note with todo block
3. Share the note
4. Open share URL in incognito window
5. Verify todo checkboxes work
6. Unshare and verify URL no longer works

**Step 4: Final commit**

```bash
git add -A
git commit -m "feat: complete note sharing implementation"
```

---

## Summary

This plan implements:
- ShareToken field on Note model
- Share/unshare API endpoints
- Separate share server on configurable port
- Read-only shared note view with interactive todos
- Share button UI in note detail page
- Shared filter in notes list
- Comprehensive documentation
- E2E tests

Total tasks: 21
Estimated commits: ~25
