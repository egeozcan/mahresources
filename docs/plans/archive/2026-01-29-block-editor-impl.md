# Block-Based Note Editor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Transform notes into a block-based editor with separate content/state, backward compatibility via Description sync, and stateful widgets (todos, tables).

**Architecture:** New `NoteBlock` model with type registry pattern. Blocks stored in separate table with lexicographic positioning. First text block syncs with `Note.Description` for API backward compatibility. Frontend uses Alpine.js structured panels.

**Tech Stack:** Go/GORM, Gorilla Mux, Alpine.js, Tailwind CSS, Playwright E2E

---

## Task 1: NoteBlock Model

**Files:**
- Create: `models/note_block_model.go`
- Test: `models/note_block_model_test.go`

**Step 1: Write the failing test**

```go
// models/note_block_model_test.go
package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoteBlock_TableName(t *testing.T) {
	block := NoteBlock{}
	assert.Equal(t, "note_blocks", block.TableName())
}

func TestNoteBlock_GetType(t *testing.T) {
	block := NoteBlock{Type: "text"}
	assert.Equal(t, "text", block.Type)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./models -run TestNoteBlock -v`
Expected: FAIL with "undefined: NoteBlock"

**Step 3: Write the model**

```go
// models/note_block_model.go
package models

import (
	"mahresources/models/types"
	"time"
)

// NoteBlock represents a content block within a note
type NoteBlock struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	CreatedAt time.Time  `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time  `gorm:"index" json:"updatedAt"`
	NoteID    uint       `gorm:"index;not null" json:"noteId"`
	Note      *Note      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Type      string     `gorm:"not null" json:"type"`
	Position  string     `gorm:"not null;index" json:"position"`
	Content   types.JSON `gorm:"not null;default:'{}'" json:"content"`
	State     types.JSON `gorm:"not null;default:'{}'" json:"state"`
}

func (NoteBlock) TableName() string {
	return "note_blocks"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./models -run TestNoteBlock -v`
Expected: PASS

**Step 5: Commit**

```bash
git add models/note_block_model.go models/note_block_model_test.go
git commit -m "feat(blocks): add NoteBlock model"
```

---

## Task 2: Database Migration

**Files:**
- Modify: `models/database.go`

**Step 1: Read current migration setup**

Check `models/database.go` for AutoMigrate calls.

**Step 2: Add NoteBlock to migration**

Add `&NoteBlock{}` to the AutoMigrate call alongside other models.

**Step 3: Run the application to verify migration**

Run: `go build --tags 'json1 fts5' && ./mahresources -ephemeral -bind-address=:8181`
Check logs for successful table creation.

**Step 4: Commit**

```bash
git add models/database.go
git commit -m "feat(blocks): add note_blocks table migration"
```

---

## Task 3: BlockType Interface and Registry

**Files:**
- Create: `models/block_types/block_type.go`
- Create: `models/block_types/registry.go`
- Test: `models/block_types/registry_test.go`

**Step 1: Write the failing test**

```go
// models/block_types/registry_test.go
package block_types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistry_GetBlockType(t *testing.T) {
	bt := GetBlockType("text")
	assert.NotNil(t, bt)
	assert.Equal(t, "text", bt.Type())
}

func TestRegistry_GetBlockType_Unknown(t *testing.T) {
	bt := GetBlockType("unknown_type")
	assert.Nil(t, bt)
}

func TestRegistry_ValidateContent_Text(t *testing.T) {
	bt := GetBlockType("text")
	content := json.RawMessage(`{"text": "hello"}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestRegistry_ValidateContent_Text_Invalid(t *testing.T) {
	bt := GetBlockType("text")
	content := json.RawMessage(`{"invalid": 123}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./models/block_types -v`
Expected: FAIL with package not found

**Step 3: Write the interface and registry**

```go
// models/block_types/block_type.go
package block_types

import "encoding/json"

// BlockType defines a block type's behavior and validation
type BlockType interface {
	// Type returns the unique identifier
	Type() string

	// ValidateContent checks if content JSON is valid
	ValidateContent(content json.RawMessage) error

	// ValidateState checks if state JSON is valid
	ValidateState(state json.RawMessage) error

	// DefaultContent returns initial content for new blocks
	DefaultContent() json.RawMessage

	// DefaultState returns initial state for new blocks
	DefaultState() json.RawMessage
}
```

```go
// models/block_types/registry.go
package block_types

import "sync"

var (
	registry = make(map[string]BlockType)
	mu       sync.RWMutex
)

// RegisterBlockType registers a block type
func RegisterBlockType(bt BlockType) {
	mu.Lock()
	defer mu.Unlock()
	registry[bt.Type()] = bt
}

// GetBlockType returns a registered block type or nil
func GetBlockType(typeName string) BlockType {
	mu.RLock()
	defer mu.RUnlock()
	return registry[typeName]
}

// GetAllBlockTypes returns all registered block types
func GetAllBlockTypes() []BlockType {
	mu.RLock()
	defer mu.RUnlock()
	types := make([]BlockType, 0, len(registry))
	for _, bt := range registry {
		types = append(types, bt)
	}
	return types
}
```

**Step 4: Run test to verify it still fails (no types registered)**

Run: `go test ./models/block_types -v`
Expected: FAIL with nil pointer

**Step 5: Commit interface and registry**

```bash
git add models/block_types/
git commit -m "feat(blocks): add BlockType interface and registry"
```

---

## Task 4: Text Block Type

**Files:**
- Create: `models/block_types/text.go`
- Modify: `models/block_types/registry_test.go`

**Step 1: Write the text block type**

```go
// models/block_types/text.go
package block_types

import (
	"encoding/json"
	"errors"
)

type textContent struct {
	Text string `json:"text"`
}

type TextBlockType struct{}

func (t TextBlockType) Type() string {
	return "text"
}

func (t TextBlockType) ValidateContent(content json.RawMessage) error {
	var c textContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}
	return nil
}

func (t TextBlockType) ValidateState(state json.RawMessage) error {
	// Text blocks have no state
	return nil
}

func (t TextBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"text": ""}`)
}

func (t TextBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{}`)
}

func init() {
	RegisterBlockType(TextBlockType{})
}
```

**Step 2: Run tests**

Run: `go test ./models/block_types -v`
Expected: PASS

**Step 3: Commit**

```bash
git add models/block_types/text.go
git commit -m "feat(blocks): add text block type"
```

---

## Task 5: Heading Block Type

**Files:**
- Create: `models/block_types/heading.go`
- Modify: `models/block_types/registry_test.go`

**Step 1: Add test for heading**

```go
// Add to registry_test.go
func TestRegistry_ValidateContent_Heading(t *testing.T) {
	bt := GetBlockType("heading")
	assert.NotNil(t, bt)

	content := json.RawMessage(`{"text": "Title", "level": 2}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestRegistry_ValidateContent_Heading_InvalidLevel(t *testing.T) {
	bt := GetBlockType("heading")
	content := json.RawMessage(`{"text": "Title", "level": 7}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
}
```

**Step 2: Write heading block type**

```go
// models/block_types/heading.go
package block_types

import (
	"encoding/json"
	"errors"
)

type headingContent struct {
	Text  string `json:"text"`
	Level int    `json:"level"`
}

type HeadingBlockType struct{}

func (h HeadingBlockType) Type() string {
	return "heading"
}

func (h HeadingBlockType) ValidateContent(content json.RawMessage) error {
	var c headingContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}
	if c.Level < 1 || c.Level > 6 {
		return errors.New("heading level must be 1-6")
	}
	return nil
}

func (h HeadingBlockType) ValidateState(state json.RawMessage) error {
	return nil
}

func (h HeadingBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"text": "", "level": 2}`)
}

func (h HeadingBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{}`)
}

func init() {
	RegisterBlockType(HeadingBlockType{})
}
```

**Step 3: Run tests**

Run: `go test ./models/block_types -v`
Expected: PASS

**Step 4: Commit**

```bash
git add models/block_types/heading.go models/block_types/registry_test.go
git commit -m "feat(blocks): add heading block type"
```

---

## Task 6: Divider Block Type

**Files:**
- Create: `models/block_types/divider.go`

**Step 1: Write divider block type**

```go
// models/block_types/divider.go
package block_types

import "encoding/json"

type DividerBlockType struct{}

func (d DividerBlockType) Type() string {
	return "divider"
}

func (d DividerBlockType) ValidateContent(content json.RawMessage) error {
	return nil
}

func (d DividerBlockType) ValidateState(state json.RawMessage) error {
	return nil
}

func (d DividerBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{}`)
}

func (d DividerBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{}`)
}

func init() {
	RegisterBlockType(DividerBlockType{})
}
```

**Step 2: Run tests**

Run: `go test ./models/block_types -v`
Expected: PASS

**Step 3: Commit**

```bash
git add models/block_types/divider.go
git commit -m "feat(blocks): add divider block type"
```

---

## Task 7: Gallery Block Type

**Files:**
- Create: `models/block_types/gallery.go`

**Step 1: Write gallery block type**

```go
// models/block_types/gallery.go
package block_types

import (
	"encoding/json"
	"errors"
)

type galleryContent struct {
	ResourceIDs []uint `json:"resourceIds"`
}

type galleryState struct {
	Layout string `json:"layout"` // "grid" or "list"
}

type GalleryBlockType struct{}

func (g GalleryBlockType) Type() string {
	return "gallery"
}

func (g GalleryBlockType) ValidateContent(content json.RawMessage) error {
	var c galleryContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}
	return nil
}

func (g GalleryBlockType) ValidateState(state json.RawMessage) error {
	var s galleryState
	if err := json.Unmarshal(state, &s); err != nil {
		return err
	}
	if s.Layout != "" && s.Layout != "grid" && s.Layout != "list" {
		return errors.New("layout must be 'grid' or 'list'")
	}
	return nil
}

func (g GalleryBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"resourceIds": []}`)
}

func (g GalleryBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{"layout": "grid"}`)
}

func init() {
	RegisterBlockType(GalleryBlockType{})
}
```

**Step 2: Commit**

```bash
git add models/block_types/gallery.go
git commit -m "feat(blocks): add gallery block type"
```

---

## Task 8: References Block Type

**Files:**
- Create: `models/block_types/references.go`

**Step 1: Write references block type**

```go
// models/block_types/references.go
package block_types

import "encoding/json"

type referencesContent struct {
	GroupIDs []uint `json:"groupIds"`
}

type ReferencesBlockType struct{}

func (r ReferencesBlockType) Type() string {
	return "references"
}

func (r ReferencesBlockType) ValidateContent(content json.RawMessage) error {
	var c referencesContent
	return json.Unmarshal(content, &c)
}

func (r ReferencesBlockType) ValidateState(state json.RawMessage) error {
	return nil
}

func (r ReferencesBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"groupIds": []}`)
}

func (r ReferencesBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{}`)
}

func init() {
	RegisterBlockType(ReferencesBlockType{})
}
```

**Step 2: Commit**

```bash
git add models/block_types/references.go
git commit -m "feat(blocks): add references block type"
```

---

## Task 9: Todos Block Type

**Files:**
- Create: `models/block_types/todos.go`

**Step 1: Write todos block type**

```go
// models/block_types/todos.go
package block_types

import (
	"encoding/json"
	"errors"
)

type todoItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type todosContent struct {
	Items []todoItem `json:"items"`
}

type todosState struct {
	Checked []string `json:"checked"`
}

type TodosBlockType struct{}

func (t TodosBlockType) Type() string {
	return "todos"
}

func (t TodosBlockType) ValidateContent(content json.RawMessage) error {
	var c todosContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}
	for _, item := range c.Items {
		if item.ID == "" {
			return errors.New("todo item must have an id")
		}
	}
	return nil
}

func (t TodosBlockType) ValidateState(state json.RawMessage) error {
	var s todosState
	return json.Unmarshal(state, &s)
}

func (t TodosBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"items": []}`)
}

func (t TodosBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{"checked": []}`)
}

func init() {
	RegisterBlockType(TodosBlockType{})
}
```

**Step 2: Commit**

```bash
git add models/block_types/todos.go
git commit -m "feat(blocks): add todos block type"
```

---

## Task 10: Table Block Type

**Files:**
- Create: `models/block_types/table.go`

**Step 1: Write table block type**

```go
// models/block_types/table.go
package block_types

import (
	"encoding/json"
	"errors"
)

type tableColumn struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type tableContent struct {
	// Manual table
	Columns []tableColumn   `json:"columns,omitempty"`
	Rows    [][]interface{} `json:"rows,omitempty"`
	// Query-backed table
	QueryID *uint `json:"queryId,omitempty"`
}

type tableState struct {
	SortColumn string `json:"sortColumn,omitempty"`
	SortDir    string `json:"sortDir,omitempty"` // "asc" or "desc"
}

type TableBlockType struct{}

func (t TableBlockType) Type() string {
	return "table"
}

func (t TableBlockType) ValidateContent(content json.RawMessage) error {
	var c tableContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}
	// Must have either manual data or queryId, not both
	hasManual := len(c.Columns) > 0 || len(c.Rows) > 0
	hasQuery := c.QueryID != nil
	if hasManual && hasQuery {
		return errors.New("table cannot have both manual data and queryId")
	}
	return nil
}

func (t TableBlockType) ValidateState(state json.RawMessage) error {
	var s tableState
	if err := json.Unmarshal(state, &s); err != nil {
		return err
	}
	if s.SortDir != "" && s.SortDir != "asc" && s.SortDir != "desc" {
		return errors.New("sortDir must be 'asc' or 'desc'")
	}
	return nil
}

func (t TableBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"columns": [], "rows": []}`)
}

func (t TableBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{}`)
}

func init() {
	RegisterBlockType(TableBlockType{})
}
```

**Step 2: Commit**

```bash
git add models/block_types/table.go
git commit -m "feat(blocks): add table block type"
```

---

## Task 11: Position Utilities

**Files:**
- Create: `lib/position.go`
- Test: `lib/position_test.go`

**Step 1: Write tests**

```go
// lib/position_test.go
package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPositionBetween(t *testing.T) {
	tests := []struct {
		before   string
		after    string
		expected string
	}{
		{"", "", "n"},
		{"", "n", "g"},
		{"n", "", "u"},
		{"a", "c", "b"},
		{"a", "b", "an"},
		{"an", "b", "at"},
	}
	for _, tt := range tests {
		result := PositionBetween(tt.before, tt.after)
		assert.Equal(t, tt.expected, result)
		if tt.before != "" {
			assert.True(t, result > tt.before)
		}
		if tt.after != "" {
			assert.True(t, result < tt.after)
		}
	}
}

func TestFirstPosition(t *testing.T) {
	assert.Equal(t, "n", FirstPosition())
}
```

**Step 2: Write position utilities**

```go
// lib/position.go
package lib

// PositionBetween returns a string that sorts between before and after.
// Uses lexicographic ordering with lowercase letters a-z.
func PositionBetween(before, after string) string {
	if before == "" && after == "" {
		return "n" // middle of alphabet
	}
	if before == "" {
		// Insert before 'after'
		if after[0] > 'a' {
			return string(after[0] - 1)
		}
		return "a" + midChar()
	}
	if after == "" {
		// Insert after 'before'
		if before[0] < 'z' {
			return string(before[0] + 1)
		}
		return before + midChar()
	}
	// Insert between
	return between(before, after)
}

func between(a, b string) string {
	// Find common prefix
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			if b[i]-a[i] > 1 {
				// There's room between
				return a[:i] + string((a[i]+b[i])/2)
			}
			// No room, extend a
			return a[:i+1] + midChar()
		}
	}

	// a is prefix of b or equal length
	if len(a) < len(b) {
		// a is prefix, insert between a and a+b[len(a)]
		return a + string((byte('a')+b[len(a)])/2)
	}

	// Extend a
	return a + midChar()
}

func midChar() string {
	return "n"
}

// FirstPosition returns the initial position for the first block
func FirstPosition() string {
	return "n"
}
```

**Step 3: Run tests**

Run: `go test ./lib -run TestPosition -v`
Expected: PASS

**Step 4: Commit**

```bash
git add lib/position.go lib/position_test.go
git commit -m "feat(blocks): add lexicographic position utilities"
```

---

## Task 12: Block Query Models

**Files:**
- Create: `models/query_models/note_block_query.go`

**Step 1: Create query models**

```go
// models/query_models/note_block_query.go
package query_models

import "encoding/json"

// NoteBlockEditor is used for creating/updating blocks
type NoteBlockEditor struct {
	ID       uint            `schema:"id"`
	NoteID   uint            `schema:"noteId"`
	Type     string          `schema:"type"`
	Position string          `schema:"position"`
	Content  json.RawMessage `schema:"-"`
}

// NoteBlockStateEditor is used for updating block state only
type NoteBlockStateEditor struct {
	ID    uint            `schema:"id"`
	State json.RawMessage `schema:"-"`
}

// NoteBlockReorderEditor is used for batch reordering
type NoteBlockReorderEditor struct {
	NoteID    uint              `json:"noteId"`
	Positions map[uint]string   `json:"positions"` // blockId -> new position
}
```

**Step 2: Commit**

```bash
git add models/query_models/note_block_query.go
git commit -m "feat(blocks): add block query models"
```

---

## Task 13: Block Context Methods

**Files:**
- Create: `application_context/block_context.go`
- Test: `application_context/block_context_test.go`

**Step 1: Write failing tests**

```go
// application_context/block_context_test.go
package application_context

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestBlockContext_CreateBlock(t *testing.T) {
	ctx := setupTestContext(t)

	// Create a note first
	note, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		Name: "Test Note",
	})
	assert.NoError(t, err)

	// Create a block
	editor := &query_models.NoteBlockEditor{
		NoteID:   note.ID,
		Type:     "text",
		Position: "n",
		Content:  json.RawMessage(`{"text": "Hello"}`),
	}
	block, err := ctx.CreateBlock(editor)
	assert.NoError(t, err)
	assert.Equal(t, "text", block.Type)
	assert.Equal(t, "n", block.Position)
}

func TestBlockContext_GetBlocksForNote(t *testing.T) {
	ctx := setupTestContext(t)

	note, _ := ctx.CreateOrUpdateNote(&query_models.NoteEditor{Name: "Test"})

	// Create blocks
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "b", Content: json.RawMessage(`{"text": "Second"}`),
	})
	ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "a", Content: json.RawMessage(`{"text": "First"}`),
	})

	blocks, err := ctx.GetBlocksForNote(note.ID)
	assert.NoError(t, err)
	assert.Len(t, *blocks, 2)
	// Should be ordered by position
	assert.Equal(t, "a", (*blocks)[0].Position)
	assert.Equal(t, "b", (*blocks)[1].Position)
}

func TestBlockContext_UpdateBlockContent(t *testing.T) {
	ctx := setupTestContext(t)

	note, _ := ctx.CreateOrUpdateNote(&query_models.NoteEditor{Name: "Test"})
	block, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "n", Content: json.RawMessage(`{"text": "Old"}`),
	})

	newContent := json.RawMessage(`{"text": "New"}`)
	updated, err := ctx.UpdateBlockContent(block.ID, newContent)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"text": "New"}`, string(updated.Content))
}

func TestBlockContext_UpdateBlockState(t *testing.T) {
	ctx := setupTestContext(t)

	note, _ := ctx.CreateOrUpdateNote(&query_models.NoteEditor{Name: "Test"})
	block, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "todos", Position: "n",
		Content: json.RawMessage(`{"items": [{"id": "x1", "label": "Task"}]}`),
	})

	newState := json.RawMessage(`{"checked": ["x1"]}`)
	updated, err := ctx.UpdateBlockState(block.ID, newState)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"checked": ["x1"]}`, string(updated.State))
}

func TestBlockContext_DeleteBlock(t *testing.T) {
	ctx := setupTestContext(t)

	note, _ := ctx.CreateOrUpdateNote(&query_models.NoteEditor{Name: "Test"})
	block, _ := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID, Type: "text", Position: "n", Content: json.RawMessage(`{"text": "Delete me"}`),
	})

	err := ctx.DeleteBlock(block.ID)
	assert.NoError(t, err)

	blocks, _ := ctx.GetBlocksForNote(note.ID)
	assert.Len(t, *blocks, 0)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./application_context -run TestBlockContext -v`
Expected: FAIL with undefined methods

**Step 3: Write the context methods**

```go
// application_context/block_context.go
package application_context

import (
	"encoding/json"
	"errors"

	"mahresources/models"
	"mahresources/models/block_types"
	"mahresources/models/query_models"
)

func (ctx *MahresourcesContext) CreateBlock(editor *query_models.NoteBlockEditor) (*models.NoteBlock, error) {
	// Validate block type
	bt := block_types.GetBlockType(editor.Type)
	if bt == nil {
		return nil, errors.New("unknown block type: " + editor.Type)
	}

	// Validate content
	if len(editor.Content) == 0 {
		editor.Content = bt.DefaultContent()
	} else if err := bt.ValidateContent(editor.Content); err != nil {
		return nil, err
	}

	block := models.NoteBlock{
		NoteID:   editor.NoteID,
		Type:     editor.Type,
		Position: editor.Position,
		Content:  editor.Content,
		State:    bt.DefaultState(),
	}

	if err := ctx.db.Create(&block).Error; err != nil {
		return nil, err
	}

	// Sync first text block to note description
	if editor.Type == "text" {
		ctx.syncFirstTextBlockToDescription(editor.NoteID)
	}

	return &block, nil
}

func (ctx *MahresourcesContext) GetBlock(id uint) (*models.NoteBlock, error) {
	var block models.NoteBlock
	return &block, ctx.db.First(&block, id).Error
}

func (ctx *MahresourcesContext) GetBlocksForNote(noteID uint) (*[]models.NoteBlock, error) {
	var blocks []models.NoteBlock
	err := ctx.db.Where("note_id = ?", noteID).Order("position ASC").Find(&blocks).Error
	return &blocks, err
}

func (ctx *MahresourcesContext) UpdateBlockContent(blockID uint, content json.RawMessage) (*models.NoteBlock, error) {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return nil, err
	}

	// Validate content against block type
	bt := block_types.GetBlockType(block.Type)
	if bt == nil {
		return nil, errors.New("unknown block type: " + block.Type)
	}
	if err := bt.ValidateContent(content); err != nil {
		return nil, err
	}

	block.Content = content
	if err := ctx.db.Save(&block).Error; err != nil {
		return nil, err
	}

	// Sync first text block to note description
	if block.Type == "text" {
		ctx.syncFirstTextBlockToDescription(block.NoteID)
	}

	return &block, nil
}

func (ctx *MahresourcesContext) UpdateBlockState(blockID uint, state json.RawMessage) (*models.NoteBlock, error) {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return nil, err
	}

	// Validate state against block type
	bt := block_types.GetBlockType(block.Type)
	if bt == nil {
		return nil, errors.New("unknown block type: " + block.Type)
	}
	if err := bt.ValidateState(state); err != nil {
		return nil, err
	}

	block.State = state
	return &block, ctx.db.Save(&block).Error
}

func (ctx *MahresourcesContext) DeleteBlock(blockID uint) error {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return err
	}

	noteID := block.NoteID
	isText := block.Type == "text"

	if err := ctx.db.Delete(&block).Error; err != nil {
		return err
	}

	// Sync first text block to note description
	if isText {
		ctx.syncFirstTextBlockToDescription(noteID)
	}

	return nil
}

func (ctx *MahresourcesContext) ReorderBlocks(noteID uint, positions map[uint]string) error {
	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for blockID, position := range positions {
		if err := tx.Model(&models.NoteBlock{}).Where("id = ? AND note_id = ?", blockID, noteID).
			Update("position", position).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// syncFirstTextBlockToDescription syncs the first text block's content to the note's Description
func (ctx *MahresourcesContext) syncFirstTextBlockToDescription(noteID uint) {
	var blocks []models.NoteBlock
	ctx.db.Where("note_id = ? AND type = ?", noteID, "text").
		Order("position ASC").Limit(1).Find(&blocks)

	if len(blocks) == 0 {
		return
	}

	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(blocks[0].Content, &content); err != nil {
		return
	}

	ctx.db.Model(&models.Note{}).Where("id = ?", noteID).Update("description", content.Text)
}
```

**Step 4: Run tests**

Run: `go test ./application_context -run TestBlockContext -v`
Expected: PASS

**Step 5: Commit**

```bash
git add application_context/block_context.go application_context/block_context_test.go
git commit -m "feat(blocks): add block CRUD context methods"
```

---

## Task 14: Block Interfaces

**Files:**
- Create: `server/interfaces/block_interfaces.go`

**Step 1: Write interfaces**

```go
// server/interfaces/block_interfaces.go
package interfaces

import (
	"encoding/json"
	"mahresources/models"
	"mahresources/models/query_models"
)

type BlockReader interface {
	GetBlock(id uint) (*models.NoteBlock, error)
	GetBlocksForNote(noteID uint) (*[]models.NoteBlock, error)
}

type BlockWriter interface {
	CreateBlock(editor *query_models.NoteBlockEditor) (*models.NoteBlock, error)
	UpdateBlockContent(blockID uint, content json.RawMessage) (*models.NoteBlock, error)
	ReorderBlocks(noteID uint, positions map[uint]string) error
}

type BlockStateWriter interface {
	UpdateBlockState(blockID uint, state json.RawMessage) (*models.NoteBlock, error)
}

type BlockDeleter interface {
	DeleteBlock(blockID uint) error
}
```

**Step 2: Commit**

```bash
git add server/interfaces/block_interfaces.go
git commit -m "feat(blocks): add block interfaces"
```

---

## Task 15: Block API Handlers

**Files:**
- Create: `server/api_handlers/block_api_handlers.go`

**Step 1: Write handlers**

```go
// server/api_handlers/block_api_handlers.go
package api_handlers

import (
	"encoding/json"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
)

func GetBlocksHandler(ctx interfaces.BlockReader) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		noteID := http_utils.GetUIntQueryParameter(request, "noteId", 0)
		if noteID == 0 {
			http_utils.HandleError(nil, writer, request, http.StatusBadRequest)
			return
		}

		blocks, err := ctx.GetBlocksForNote(noteID)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(writer).Encode(blocks)
	}
}

func GetBlockHandler(ctx interfaces.BlockReader) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)
		block, err := ctx.GetBlock(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(writer).Encode(block)
	}
}

func CreateBlockHandler(ctx interfaces.BlockWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor query_models.NoteBlockEditor

		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		// Parse content from body
		if request.Body != nil {
			var body struct {
				Content json.RawMessage `json:"content"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err == nil && len(body.Content) > 0 {
				editor.Content = body.Content
			}
		}

		block, err := ctx.CreateBlock(&editor)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.WriteHeader(http.StatusCreated)
		json.NewEncoder(writer).Encode(block)
	}
}

func UpdateBlockContentHandler(ctx interfaces.BlockWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)

		var body struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		block, err := ctx.UpdateBlockContent(id, body.Content)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(writer).Encode(block)
	}
}

func UpdateBlockStateHandler(ctx interfaces.BlockStateWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)

		var body struct {
			State json.RawMessage `json:"state"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		block, err := ctx.UpdateBlockState(id, body.State)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		json.NewEncoder(writer).Encode(block)
	}
}

func DeleteBlockHandler(ctx interfaces.BlockDeleter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)

		if err := ctx.DeleteBlock(id); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.WriteHeader(http.StatusNoContent)
	}
}

func ReorderBlocksHandler(ctx interfaces.BlockWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body query_models.NoteBlockReorderEditor
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if err := ctx.ReorderBlocks(body.NoteID, body.Positions); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.WriteHeader(http.StatusNoContent)
	}
}
```

**Step 2: Commit**

```bash
git add server/api_handlers/block_api_handlers.go
git commit -m "feat(blocks): add block API handlers"
```

---

## Task 16: Register Block Routes

**Files:**
- Modify: `server/routes.go`

**Step 1: Add block routes to registerRoutes function**

After the existing API routes, add:

```go
// Block API routes
router.Methods(http.MethodGet).Path("/v1/note/blocks").HandlerFunc(api_handlers.GetBlocksHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/note/block").HandlerFunc(api_handlers.GetBlockHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/note/block").HandlerFunc(api_handlers.CreateBlockHandler(appContext))
router.Methods(http.MethodPut).Path("/v1/note/block").HandlerFunc(api_handlers.UpdateBlockContentHandler(appContext))
router.Methods(http.MethodPatch).Path("/v1/note/block/state").HandlerFunc(api_handlers.UpdateBlockStateHandler(appContext))
router.Methods(http.MethodDelete).Path("/v1/note/block").HandlerFunc(api_handlers.DeleteBlockHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/note/blocks/reorder").HandlerFunc(api_handlers.ReorderBlocksHandler(appContext))
```

**Step 2: Commit**

```bash
git add server/routes.go
git commit -m "feat(blocks): register block API routes"
```

---

## Task 17: Update Note Response to Include Blocks

**Files:**
- Modify: `application_context/note_context.go`

**Step 1: Update GetNote to preload blocks**

In `GetNote`, add block preloading:

```go
func (ctx *MahresourcesContext) GetNote(id uint) (*models.Note, error) {
	var note models.Note
	return &note, ctx.db.Preload(clause.Associations, pageLimit).
		Preload("Blocks", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		First(&note, id).Error
}
```

**Step 2: Add Blocks field to Note model**

In `models/note_model.go`, add:

```go
Blocks []*NoteBlock `gorm:"foreignKey:NoteID" json:"blocks,omitempty"`
```

**Step 3: Commit**

```bash
git add models/note_model.go application_context/note_context.go
git commit -m "feat(blocks): include blocks in note response"
```

---

## Task 18: Backward Compatibility - Description Sync

**Files:**
- Modify: `application_context/note_context.go`

**Step 1: Update CreateOrUpdateNote to sync Description to first text block**

When a note is updated via the legacy API (with Description), sync to first text block if blocks exist:

```go
// Add after saving the note in CreateOrUpdateNote
// Sync description to first text block if blocks exist
if noteQuery.ID != 0 {
	var blocks []models.NoteBlock
	if err := tx.Where("note_id = ? AND type = ?", note.ID, "text").Order("position ASC").Limit(1).Find(&blocks).Error; err == nil && len(blocks) > 0 {
		content, _ := json.Marshal(map[string]string{"text": noteQuery.Description})
		tx.Model(&blocks[0]).Update("content", content)
	}
}
```

**Step 2: Commit**

```bash
git add application_context/note_context.go
git commit -m "feat(blocks): sync Description to first text block on legacy API update"
```

---

## Task 19: Block API Tests

**Files:**
- Create: `server/api_tests/block_api_test.go`

**Step 1: Write API tests**

```go
// server/api_tests/block_api_test.go
package api_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
)

func TestBlockAPI_CreateAndGet(t *testing.T) {
	router, ctx := setupTestRouter(t)

	// Create a note first
	note := createTestNote(t, ctx, "Test Note")

	// Create a block
	body := `{"noteId": ` + uintToString(note.ID) + `, "type": "text", "position": "n", "content": {"text": "Hello"}}`
	req := httptest.NewRequest("POST", "/v1/note/block", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var block models.NoteBlock
	json.Unmarshal(w.Body.Bytes(), &block)
	assert.Equal(t, "text", block.Type)

	// Get blocks for note
	req = httptest.NewRequest("GET", "/v1/note/blocks?noteId="+uintToString(note.ID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var blocks []models.NoteBlock
	json.Unmarshal(w.Body.Bytes(), &blocks)
	assert.Len(t, blocks, 1)
}

func TestBlockAPI_UpdateState(t *testing.T) {
	router, ctx := setupTestRouter(t)

	note := createTestNote(t, ctx, "Test Note")
	block := createTestBlock(t, ctx, note.ID, "todos", `{"items": [{"id": "x1", "label": "Task"}]}`)

	// Update state
	body := `{"state": {"checked": ["x1"]}}`
	req := httptest.NewRequest("PATCH", "/v1/note/block/state?id="+uintToString(block.ID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updated models.NoteBlock
	json.Unmarshal(w.Body.Bytes(), &updated)
	assert.Contains(t, string(updated.State), "x1")
}
```

**Step 2: Commit**

```bash
git add server/api_tests/block_api_test.go
git commit -m "test(blocks): add block API tests"
```

---

## Task 20: Frontend - Block Editor Alpine Component

**Files:**
- Create: `src/components/blockEditor.js`

**Step 1: Write the block editor component**

```javascript
// src/components/blockEditor.js
export function blockEditor(noteId, initialBlocks = []) {
  return {
    noteId,
    blocks: initialBlocks,
    editMode: false,
    loading: false,

    async init() {
      if (this.blocks.length === 0 && this.noteId) {
        await this.loadBlocks();
      }
    },

    async loadBlocks() {
      this.loading = true;
      try {
        const res = await fetch(`/v1/note/blocks?noteId=${this.noteId}`);
        this.blocks = await res.json();
      } finally {
        this.loading = false;
      }
    },

    toggleEditMode() {
      this.editMode = !this.editMode;
    },

    async addBlock(type, afterPosition = null) {
      const position = this.calculatePosition(afterPosition);
      const res = await fetch('/v1/note/block', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          noteId: this.noteId,
          type,
          position,
          content: this.getDefaultContent(type)
        })
      });

      if (res.ok) {
        await this.loadBlocks();
      }
    },

    async updateBlockContent(blockId, content) {
      const res = await fetch(`/v1/note/block?id=${blockId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content })
      });

      if (res.ok) {
        const updated = await res.json();
        const idx = this.blocks.findIndex(b => b.id === blockId);
        if (idx >= 0) this.blocks[idx] = updated;
      }
    },

    async updateBlockState(blockId, state) {
      const res = await fetch(`/v1/note/block/state?id=${blockId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ state })
      });

      if (res.ok) {
        const updated = await res.json();
        const idx = this.blocks.findIndex(b => b.id === blockId);
        if (idx >= 0) this.blocks[idx] = updated;
      }
    },

    async deleteBlock(blockId) {
      const res = await fetch(`/v1/note/block?id=${blockId}`, {
        method: 'DELETE'
      });

      if (res.ok) {
        this.blocks = this.blocks.filter(b => b.id !== blockId);
      }
    },

    async moveBlock(blockId, direction) {
      const idx = this.blocks.findIndex(b => b.id === blockId);
      if (idx < 0) return;

      const newIdx = direction === 'up' ? idx - 1 : idx + 1;
      if (newIdx < 0 || newIdx >= this.blocks.length) return;

      // Calculate new positions
      const positions = {};
      const movingBlock = this.blocks[idx];
      const targetBlock = this.blocks[newIdx];

      // Swap positions
      positions[movingBlock.id] = targetBlock.position;
      positions[targetBlock.id] = movingBlock.position;

      const res = await fetch('/v1/note/blocks/reorder', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ noteId: this.noteId, positions })
      });

      if (res.ok) {
        await this.loadBlocks();
      }
    },

    calculatePosition(afterPosition) {
      if (!afterPosition) {
        // Add at end
        if (this.blocks.length === 0) return 'n';
        const last = this.blocks[this.blocks.length - 1];
        return this.positionAfter(last.position);
      }

      const idx = this.blocks.findIndex(b => b.position === afterPosition);
      if (idx < 0 || idx === this.blocks.length - 1) {
        return this.positionAfter(afterPosition);
      }

      return this.positionBetween(afterPosition, this.blocks[idx + 1].position);
    },

    positionAfter(pos) {
      const last = pos.charCodeAt(pos.length - 1);
      if (last < 122) { // 'z'
        return pos.slice(0, -1) + String.fromCharCode(last + 1);
      }
      return pos + 'n';
    },

    positionBetween(a, b) {
      // Simple midpoint for single chars
      if (a.length === 1 && b.length === 1) {
        const mid = Math.floor((a.charCodeAt(0) + b.charCodeAt(0)) / 2);
        if (mid !== a.charCodeAt(0)) {
          return String.fromCharCode(mid);
        }
      }
      return a + 'n';
    },

    getDefaultContent(type) {
      const defaults = {
        text: { text: '' },
        heading: { text: '', level: 2 },
        divider: {},
        gallery: { resourceIds: [] },
        references: { groupIds: [] },
        todos: { items: [] },
        table: { columns: [], rows: [] }
      };
      return defaults[type] || {};
    },

    blockTypes: [
      { type: 'text', label: 'Text', icon: '📝' },
      { type: 'heading', label: 'Heading', icon: '🔤' },
      { type: 'divider', label: 'Divider', icon: '──' },
      { type: 'gallery', label: 'Gallery', icon: '🖼️' },
      { type: 'references', label: 'References', icon: '📁' },
      { type: 'todos', label: 'Todos', icon: '☑️' },
      { type: 'table', label: 'Table', icon: '📊' }
    ]
  };
}
```

**Step 2: Commit**

```bash
git add src/components/blockEditor.js
git commit -m "feat(blocks): add blockEditor Alpine component"
```

---

## Task 21: Frontend - Block Type Components

**Files:**
- Create: `src/components/blocks/blockText.js`
- Create: `src/components/blocks/blockHeading.js`
- Create: `src/components/blocks/blockDivider.js`
- Create: `src/components/blocks/blockTodos.js`
- Create: `src/components/blocks/blockGallery.js`
- Create: `src/components/blocks/blockReferences.js`
- Create: `src/components/blocks/blockTable.js`

These will be individual commits. Each block component handles both view and edit mode, plus state interactions.

**Step 1: Write blockText.js**

```javascript
// src/components/blocks/blockText.js
export function blockText() {
  return {
    get text() {
      return this.block?.content?.text || '';
    },

    updateText(newText) {
      this.$dispatch('update-content', { text: newText });
    }
  };
}
```

**Step 2: Write blockTodos.js**

```javascript
// src/components/blocks/blockTodos.js
export function blockTodos() {
  return {
    get items() {
      return this.block?.content?.items || [];
    },

    get checked() {
      return this.block?.state?.checked || [];
    },

    isChecked(itemId) {
      return this.checked.includes(itemId);
    },

    toggleItem(itemId) {
      const newChecked = this.isChecked(itemId)
        ? this.checked.filter(id => id !== itemId)
        : [...this.checked, itemId];

      this.$dispatch('update-state', { checked: newChecked });
    },

    addItem(label) {
      const newItem = { id: crypto.randomUUID(), label };
      const newItems = [...this.items, newItem];
      this.$dispatch('update-content', { items: newItems });
    },

    removeItem(itemId) {
      const newItems = this.items.filter(i => i.id !== itemId);
      const newChecked = this.checked.filter(id => id !== itemId);
      this.$dispatch('update-content', { items: newItems });
      this.$dispatch('update-state', { checked: newChecked });
    }
  };
}
```

**Continue with other block types following similar patterns...**

**Step 3: Register in main.js**

Add imports and Alpine.data registrations for all block components.

**Step 4: Commit each block type**

```bash
git add src/components/blocks/
git commit -m "feat(blocks): add frontend block type components"
```

---

## Task 22: Frontend - Block Editor Template

**Files:**
- Create: `templates/partials/blockEditor.tpl`
- Modify: `templates/displayNote.tpl`

**Step 1: Create block editor partial**

```html
{# templates/partials/blockEditor.tpl #}
<div x-data="blockEditor({{ note.ID }}, {{ blocks | json }})" class="block-editor">
  <div class="flex justify-between items-center mb-4">
    <h3 class="text-lg font-semibold">Content</h3>
    <button @click="toggleEditMode()" class="btn btn-secondary btn-sm">
      <span x-show="!editMode">Edit</span>
      <span x-show="editMode">Done</span>
    </button>
  </div>

  <div class="space-y-4">
    <template x-for="block in blocks" :key="block.id">
      <div class="block-card border rounded-lg p-4">
        <!-- Block header (edit mode only) -->
        <div x-show="editMode" class="flex justify-between items-center mb-2 pb-2 border-b">
          <span class="text-sm text-gray-500" x-text="block.type"></span>
          <div class="space-x-2">
            <button @click="moveBlock(block.id, 'up')" class="btn btn-xs">↑</button>
            <button @click="moveBlock(block.id, 'down')" class="btn btn-xs">↓</button>
            <button @click="deleteBlock(block.id)" class="btn btn-xs btn-danger">×</button>
          </div>
        </div>

        <!-- Block content -->
        <div class="block-content"
             :class="{ 'editing': editMode }"
             @update-content.stop="updateBlockContent(block.id, $event.detail)"
             @update-state.stop="updateBlockState(block.id, $event.detail)">

          <!-- Text block -->
          <template x-if="block.type === 'text'">
            <div x-data="blockText()" x-init="block = $el.closest('[x-data*=blockEditor]').__x.$data.blocks.find(b => b.id === block.id)">
              <div x-show="!editMode" x-html="text"></div>
              <textarea x-show="editMode"
                        :value="text"
                        @input.debounce.500ms="updateText($event.target.value)"
                        class="w-full p-2 border rounded"></textarea>
            </div>
          </template>

          <!-- Heading block -->
          <template x-if="block.type === 'heading'">
            <div>
              <component :is="'h' + (block.content?.level || 2)"
                         x-show="!editMode"
                         x-text="block.content?.text"></component>
              <div x-show="editMode" class="space-y-2">
                <input type="text" :value="block.content?.text"
                       @input.debounce.500ms="updateBlockContent(block.id, {...block.content, text: $event.target.value})"
                       class="w-full p-2 border rounded">
                <select @change="updateBlockContent(block.id, {...block.content, level: parseInt($event.target.value)})"
                        class="p-2 border rounded">
                  <template x-for="l in [1,2,3,4,5,6]">
                    <option :value="l" :selected="block.content?.level === l" x-text="'H' + l"></option>
                  </template>
                </select>
              </div>
            </div>
          </template>

          <!-- Divider block -->
          <template x-if="block.type === 'divider'">
            <hr class="my-4">
          </template>

          <!-- Todos block -->
          <template x-if="block.type === 'todos'">
            <div x-data="blockTodos()" x-init="block = $el.closest('.block-card').__x.$data">
              <ul class="space-y-2">
                <template x-for="item in items" :key="item.id">
                  <li class="flex items-center space-x-2">
                    <input type="checkbox"
                           :checked="isChecked(item.id)"
                           @change="toggleItem(item.id)">
                    <span :class="{ 'line-through': isChecked(item.id) }" x-text="item.label"></span>
                    <button x-show="editMode" @click="removeItem(item.id)" class="text-red-500">×</button>
                  </li>
                </template>
              </ul>
              <div x-show="editMode" class="mt-2">
                <input type="text" placeholder="Add item..."
                       @keyup.enter="addItem($event.target.value); $event.target.value = ''"
                       class="p-2 border rounded">
              </div>
            </div>
          </template>

          <!-- Add more block types... -->
        </div>
      </div>
    </template>

    <!-- Add block button (edit mode only) -->
    <div x-show="editMode" class="add-block">
      <div x-data="{ showPicker: false }" class="relative">
        <button @click="showPicker = !showPicker" class="btn btn-secondary w-full">
          + Add Block
        </button>
        <div x-show="showPicker" @click.away="showPicker = false"
             class="absolute left-0 right-0 mt-2 bg-white border rounded-lg shadow-lg z-10">
          <template x-for="bt in blockTypes" :key="bt.type">
            <button @click="addBlock(bt.type); showPicker = false"
                    class="block w-full text-left px-4 py-2 hover:bg-gray-100">
              <span x-text="bt.icon"></span>
              <span x-text="bt.label"></span>
            </button>
          </template>
        </div>
      </div>
    </div>
  </div>
</div>
```

**Step 2: Integrate into displayNote.tpl**

Add `{% include "partials/blockEditor.tpl" %}` in the appropriate location.

**Step 3: Commit**

```bash
git add templates/partials/blockEditor.tpl templates/displayNote.tpl
git commit -m "feat(blocks): add block editor template"
```

---

## Task 23: Register Frontend Components in main.js

**Files:**
- Modify: `src/main.js`

**Step 1: Import and register block components**

```javascript
// Add imports
import { blockEditor } from './components/blockEditor.js';
import { blockText } from './components/blocks/blockText.js';
import { blockTodos } from './components/blocks/blockTodos.js';
// ... other block imports

// Register components
Alpine.data('blockEditor', blockEditor);
Alpine.data('blockText', blockText);
Alpine.data('blockTodos', blockTodos);
// ... other registrations
```

**Step 2: Build**

Run: `npm run build-js`

**Step 3: Commit**

```bash
git add src/main.js
git commit -m "feat(blocks): register block components in main.js"
```

---

## Task 24: E2E Tests - Block CRUD

**Files:**
- Create: `e2e/tests/blocks/block-crud.spec.ts`

**Step 1: Write E2E tests**

```typescript
// e2e/tests/blocks/block-crud.spec.ts
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Block Editor CRUD', () => {
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const note = await apiClient.createNote({ name: 'Block Test Note' });
    noteId = note.ID;
  });

  test('should add a text block', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);

    // Enter edit mode
    await page.click('button:has-text("Edit")');

    // Add text block
    await page.click('button:has-text("+ Add Block")');
    await page.click('button:has-text("Text")');

    // Verify block added
    await expect(page.locator('.block-card')).toHaveCount(1);
  });

  test('should edit text block content', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.click('button:has-text("Edit")');

    await page.fill('.block-card textarea', 'Hello World');
    await page.click('button:has-text("Done")');

    // Verify content persisted
    await page.reload();
    await expect(page.locator('.block-content')).toContainText('Hello World');
  });

  test('should add todos block and check items', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.click('button:has-text("Edit")');

    // Add todos block
    await page.click('button:has-text("+ Add Block")');
    await page.click('button:has-text("Todos")');

    // Add todo item
    await page.fill('input[placeholder="Add item..."]', 'Buy milk');
    await page.press('input[placeholder="Add item..."]', 'Enter');

    await page.click('button:has-text("Done")');

    // Check the todo
    await page.click('input[type="checkbox"]');

    // Verify state persisted
    await page.reload();
    await expect(page.locator('input[type="checkbox"]')).toBeChecked();
  });

  test('should delete a block', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.click('button:has-text("Edit")');

    const initialCount = await page.locator('.block-card').count();

    await page.click('.block-card:first-child button:has-text("×")');

    await expect(page.locator('.block-card')).toHaveCount(initialCount - 1);
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteNote(noteId);
  });
});
```

**Step 2: Commit**

```bash
git add e2e/tests/blocks/
git commit -m "test(blocks): add E2E tests for block CRUD"
```

---

## Task 25: E2E Tests - Block State

**Files:**
- Create: `e2e/tests/blocks/block-state.spec.ts`

**Step 1: Write state persistence tests**

```typescript
// e2e/tests/blocks/block-state.spec.ts
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Block State Persistence', () => {
  test('todo checked state persists across page loads', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ name: 'State Test Note' });

    await page.goto(`/note?id=${note.ID}`);
    await page.click('button:has-text("Edit")');

    // Add todos
    await page.click('button:has-text("+ Add Block")');
    await page.click('button:has-text("Todos")');
    await page.fill('input[placeholder="Add item..."]', 'Task 1');
    await page.press('input[placeholder="Add item..."]', 'Enter');
    await page.fill('input[placeholder="Add item..."]', 'Task 2');
    await page.press('input[placeholder="Add item..."]', 'Enter');

    await page.click('button:has-text("Done")');

    // Check first item
    await page.click('input[type="checkbox"]:first-child');

    // Reload and verify
    await page.reload();
    const checkboxes = page.locator('input[type="checkbox"]');
    await expect(checkboxes.first()).toBeChecked();
    await expect(checkboxes.last()).not.toBeChecked();

    await apiClient.deleteNote(note.ID);
  });

  test('table sort state persists', async ({ page, apiClient }) => {
    // Similar test for table sorting
  });
});
```

**Step 2: Commit**

```bash
git add e2e/tests/blocks/block-state.spec.ts
git commit -m "test(blocks): add E2E tests for block state persistence"
```

---

## Task 26: E2E Tests - Backward Compatibility

**Files:**
- Create: `e2e/tests/blocks/block-backward-compat.spec.ts`

**Step 1: Write backward compatibility tests**

```typescript
// e2e/tests/blocks/block-backward-compat.spec.ts
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Block Backward Compatibility', () => {
  test('legacy API Description syncs to first text block', async ({ apiClient }) => {
    // Create note with blocks
    const note = await apiClient.createNote({ name: 'Compat Test', description: 'Original' });

    // Add text block via API
    await apiClient.post('/v1/note/block', {
      noteId: note.ID,
      type: 'text',
      position: 'n',
      content: { text: 'Block content' }
    });

    // Update via legacy API
    await apiClient.updateNote(note.ID, { description: 'Updated via legacy' });

    // Fetch blocks and verify sync
    const blocks = await apiClient.get(`/v1/note/blocks?noteId=${note.ID}`);
    expect(blocks[0].content.text).toBe('Updated via legacy');

    await apiClient.deleteNote(note.ID);
  });

  test('first text block syncs to Description', async ({ apiClient }) => {
    const note = await apiClient.createNote({ name: 'Sync Test' });

    // Add text block
    const block = await apiClient.post('/v1/note/block', {
      noteId: note.ID,
      type: 'text',
      position: 'n',
      content: { text: 'Block text' }
    });

    // Fetch note and verify Description
    const fetchedNote = await apiClient.getNote(note.ID);
    expect(fetchedNote.Description).toBe('Block text');

    await apiClient.deleteNote(note.ID);
  });
});
```

**Step 2: Commit**

```bash
git add e2e/tests/blocks/block-backward-compat.spec.ts
git commit -m "test(blocks): add E2E tests for backward compatibility"
```

---

## Task 27: Documentation - API Reference

**Files:**
- Modify: `docs-site/docs/api/notes.md`

**Step 1: Add block endpoints documentation**

Add a new section for block endpoints with request/response examples.

**Step 2: Commit**

```bash
git add docs-site/docs/api/notes.md
git commit -m "docs(blocks): add block API documentation"
```

---

## Task 28: Documentation - Concepts

**Files:**
- Modify: `docs-site/docs/concepts/notes.md`

**Step 1: Add blocks concept section**

Explain the block model, content vs state, block types.

**Step 2: Commit**

```bash
git add docs-site/docs/concepts/notes.md
git commit -m "docs(blocks): add blocks concept documentation"
```

---

## Task 29: Documentation - User Guide

**Files:**
- Modify: `docs-site/docs/user-guide/managing-notes.md`

**Step 1: Add block editor usage guide**

Document how to use the block editor UI.

**Step 2: Commit**

```bash
git add docs-site/docs/user-guide/managing-notes.md
git commit -m "docs(blocks): add block editor user guide"
```

---

## Task 30: Documentation - Contributor Guide

**Files:**
- Create: `docs-site/docs/contributing/block-types.md`

**Step 1: Write contributor documentation**

```markdown
# Creating Custom Block Types

This guide explains how to add new block types to mahresources.

## Overview

Block types define:
- Content structure (data edited in edit mode)
- State structure (data modified while viewing)
- Validation rules
- Default values

## Backend: Go Implementation

### 1. Create the block type file

Create `models/block_types/yourtype.go`:

```go
package block_types

import (
    "encoding/json"
    "errors"
)

type yourTypeContent struct {
    // Define content fields
}

type yourTypeState struct {
    // Define state fields
}

type YourBlockType struct{}

func (y YourBlockType) Type() string {
    return "yourtype"
}

func (y YourBlockType) ValidateContent(content json.RawMessage) error {
    var c yourTypeContent
    if err := json.Unmarshal(content, &c); err != nil {
        return err
    }
    // Add validation logic
    return nil
}

func (y YourBlockType) ValidateState(state json.RawMessage) error {
    var s yourTypeState
    return json.Unmarshal(state, &s)
}

func (y YourBlockType) DefaultContent() json.RawMessage {
    return json.RawMessage(`{}`)
}

func (y YourBlockType) DefaultState() json.RawMessage {
    return json.RawMessage(`{}`)
}

func init() {
    RegisterBlockType(YourBlockType{})
}
```

### 2. Write tests

Add tests in `models/block_types/registry_test.go`.

## Frontend: Alpine.js Component

### 1. Create component file

Create `src/components/blocks/blockYourtype.js`:

```javascript
export function blockYourtype() {
  return {
    // Component logic
  };
}
```

### 2. Register in main.js

```javascript
import { blockYourtype } from './components/blocks/blockYourtype.js';
Alpine.data('blockYourtype', blockYourtype);
```

### 3. Add template handling

Update `templates/partials/blockEditor.tpl` with a new template case.

## Content vs State

- **Content**: The core data. Only changed in edit mode.
- **State**: Runtime preferences. Changed while viewing (no edit mode needed).

Examples:
- Todo list: content = item labels, state = which items are checked
- Table: content = column definitions + rows, state = sort column/direction
- Gallery: content = resource IDs, state = layout preference

## Testing

1. Add Go unit tests for validation
2. Add E2E tests for UI interaction
3. Test content/state separation
4. Test persistence across page loads
```

**Step 2: Commit**

```bash
git add docs-site/docs/contributing/block-types.md
git commit -m "docs(blocks): add contributor guide for block types"
```

---

## Task 31: Final Integration Test

**Files:**
- None (manual verification)

**Step 1: Build and run**

```bash
npm run build
go build --tags 'json1 fts5'
./mahresources -ephemeral -bind-address=:8181
```

**Step 2: Manual verification**

1. Create a note
2. Add blocks of each type
3. Edit block content
4. Update block state (check todos, sort table)
5. Reload and verify persistence
6. Update via legacy Description field
7. Verify backward compatibility

**Step 3: Run all tests**

```bash
go test ./...
cd e2e && npm run test:with-server
```

**Step 4: Final commit**

```bash
git add .
git commit -m "feat(blocks): complete block editor implementation"
```

---

## Summary

This plan implements the block-based note editor in 31 tasks:

1. **Tasks 1-2**: Database model and migration
2. **Tasks 3-10**: Block type interface and all 7 initial types
3. **Tasks 11-12**: Position utilities and query models
4. **Tasks 13-18**: Backend CRUD (context, interfaces, handlers, routes, backward compat)
5. **Tasks 19**: API tests
6. **Tasks 20-23**: Frontend (Alpine components, templates, registration)
7. **Tasks 24-26**: E2E tests
8. **Tasks 27-30**: Documentation
9. **Task 31**: Final integration verification
