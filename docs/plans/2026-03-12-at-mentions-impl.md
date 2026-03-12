# @-Mentions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add @-mention autocomplete to description textareas and NoteBlock text editors, with server-side rendering and automatic relation syncing.

**Architecture:** Textarea overlay approach — a new Alpine.js component listens for `@` in textareas, shows a search dropdown, inserts `@[type:id:name]` markers. A Go parser extracts markers for both template rendering (pongo2 filter) and relation syncing (called after entity save). No schema changes, no new API endpoints, no new dependencies.

**Tech Stack:** Alpine.js, pongo2, GORM, existing `/v1/search` API, Tailwind CSS

---

### Task 1: Go mention parser + tests

**Files:**
- Create: `lib/mentions.go`
- Create: `lib/mentions_test.go`

**Step 1: Write the test file**

```go
// lib/mentions_test.go
package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMentions_Empty(t *testing.T) {
	result := ParseMentions("")
	assert.Empty(t, result)
}

func TestParseMentions_NoMentions(t *testing.T) {
	result := ParseMentions("just some regular text")
	assert.Empty(t, result)
}

func TestParseMentions_SingleMention(t *testing.T) {
	result := ParseMentions("check out @[resource:42:photo.jpg] please")
	assert.Len(t, result, 1)
	assert.Equal(t, Mention{Type: "resource", ID: 42, Name: "photo.jpg"}, result[0])
}

func TestParseMentions_MultipleMentions(t *testing.T) {
	result := ParseMentions("see @[note:7:Meeting Notes] and @[group:15:Project Alpha]")
	assert.Len(t, result, 2)
	assert.Equal(t, Mention{Type: "note", ID: 7, Name: "Meeting Notes"}, result[0])
	assert.Equal(t, Mention{Type: "group", ID: 15, Name: "Project Alpha"}, result[1])
}

func TestParseMentions_ColonInName(t *testing.T) {
	result := ParseMentions("see @[resource:1:file: report.pdf]")
	assert.Len(t, result, 1)
	assert.Equal(t, "file: report.pdf", result[0].Name)
}

func TestParseMentions_InvalidID(t *testing.T) {
	result := ParseMentions("@[resource:abc:name]")
	assert.Empty(t, result)
}

func TestParseMentions_Deduplication(t *testing.T) {
	result := ParseMentions("@[tag:3:important] and again @[tag:3:important]")
	assert.Len(t, result, 1)
}

func TestParseMentions_AllTypes(t *testing.T) {
	text := "@[resource:1:a] @[note:2:b] @[group:3:c] @[tag:4:d]"
	result := ParseMentions(text)
	assert.Len(t, result, 4)
}

func TestIsMentionOnlyOnLine_Standalone(t *testing.T) {
	assert.True(t, IsMentionOnlyOnLine("@[resource:42:photo.jpg]", "@[resource:42:photo.jpg]"))
}

func TestIsMentionOnlyOnLine_StandaloneWithWhitespace(t *testing.T) {
	assert.True(t, IsMentionOnlyOnLine("  @[resource:42:photo.jpg]  ", "@[resource:42:photo.jpg]"))
}

func TestIsMentionOnlyOnLine_Inline(t *testing.T) {
	assert.False(t, IsMentionOnlyOnLine("check @[resource:42:photo.jpg] out", "@[resource:42:photo.jpg]"))
}

func TestIsMentionOnlyOnLine_MultilineStandalone(t *testing.T) {
	text := "some text\n@[resource:42:photo.jpg]\nmore text"
	assert.True(t, IsMentionOnlyOnLine(text, "@[resource:42:photo.jpg]"))
}

func TestIsMentionOnlyOnLine_MultilineInline(t *testing.T) {
	text := "some text\nsee @[resource:42:photo.jpg] here\nmore text"
	assert.False(t, IsMentionOnlyOnLine(text, "@[resource:42:photo.jpg]"))
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./lib/ -run TestParseMentions -v`
Expected: FAIL — `ParseMentions` not defined

**Step 3: Write the parser implementation**

```go
// lib/mentions.go
package lib

import (
	"regexp"
	"strconv"
	"strings"
)

// Mention represents a parsed @-mention marker from text.
type Mention struct {
	Type string
	ID   uint
	Name string
}

// mentionRegex matches @[type:id:name] markers.
// Type and ID are captured as groups 1 and 2; everything after the second colon
// up to the closing ] is the display name (group 3), which may itself contain colons.
var mentionRegex = regexp.MustCompile(`@\[([a-zA-Z]+):(\d+):([^\]]+)\]`)

// ParseMentions extracts all unique @[type:id:name] markers from text.
// Duplicates (same type+id) are removed, keeping the first occurrence.
func ParseMentions(text string) []Mention {
	matches := mentionRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []Mention

	for _, match := range matches {
		entityType := strings.ToLower(match[1])
		id, err := strconv.ParseUint(match[2], 10, 64)
		if err != nil || id == 0 {
			continue
		}

		key := entityType + ":" + match[2]
		if seen[key] {
			continue
		}
		seen[key] = true

		result = append(result, Mention{
			Type: entityType,
			ID:   uint(id),
			Name: match[3],
		})
	}

	return result
}

// IsMentionOnlyOnLine returns true if the given marker is the only non-whitespace
// content on its line within the full text.
func IsMentionOnlyOnLine(fullText, marker string) bool {
	lines := strings.Split(fullText, "\n")
	for _, line := range lines {
		if strings.Contains(line, marker) {
			trimmed := strings.TrimSpace(line)
			if trimmed == marker {
				return true
			}
		}
	}
	return false
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./lib/ -run TestParseMentions -v && go test ./lib/ -run TestIsMentionOnlyOnLine -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add lib/mentions.go lib/mentions_test.go
git commit -m "feat: add @-mention marker parser with tests"
```

---

### Task 2: Pongo2 `render_mentions` template filter

**Files:**
- Create: `server/template_handlers/template_filters/mentions_filter.go`
- Modify: `server/template_handlers/template_filters/template_filters.go:63-68` — add registration

**Step 1: Write the filter implementation**

```go
// server/template_handlers/template_filters/mentions_filter.go
package template_filters

import (
	"fmt"
	"html"
	"strings"

	"github.com/flosch/pongo2/v4"
	"mahresources/lib"
)

func renderMentionsFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	text := in.String()
	if text == "" {
		return in, nil
	}

	mentions := lib.ParseMentions(text)
	if len(mentions) == 0 {
		return in, nil
	}

	result := text

	for _, m := range mentions {
		marker := fmt.Sprintf("@[%s:%d:%s]", m.Type, m.ID, m.Name)
		escapedName := html.EscapeString(m.Name)
		var replacement string

		standalone := lib.IsMentionOnlyOnLine(text, marker)

		if m.Type == "resource" {
			if standalone {
				replacement = fmt.Sprintf(
					`<a href="/resource?id=%d" class="mention-card"><img src="/v1/resource/thumbnail?id=%d" alt="%s" class="mention-card-thumb"><span class="mention-card-name">%s</span></a>`,
					m.ID, m.ID, escapedName, escapedName,
				)
			} else {
				replacement = fmt.Sprintf(
					`<a href="/resource?id=%d" class="mention-inline"><img src="/v1/resource/thumbnail?id=%d" alt="" class="mention-inline-thumb">%s</a>`,
					m.ID, m.ID, escapedName,
				)
			}
		} else {
			entityPath := entityPaths[m.Type]
			if entityPath == "" {
				entityPath = "/" + m.Type
			}
			replacement = fmt.Sprintf(
				`<a href="%s?id=%d" class="mention-badge mention-%s">%s</a>`,
				entityPath, m.ID, m.Type, escapedName,
			)
		}

		result = strings.Replace(result, marker, replacement, 1)
	}

	return pongo2.AsValue(result), nil
}
```

**Step 2: Register the filter**

In `server/template_handlers/template_filters/template_filters.go`, after the `entityPath` registration (line 63-67), add:

```go
	mentionsErr := pongo2.RegisterFilter("render_mentions", renderMentionsFilter)

	if mentionsErr != nil {
		fmt.Println("error when registering render_mentions filter", mentionsErr)
	}
```

**Step 3: Run existing tests to verify nothing breaks**

Run: `go test ./...`
Expected: all PASS

**Step 4: Commit**

```bash
git add server/template_handlers/template_filters/mentions_filter.go server/template_handlers/template_filters/template_filters.go
git commit -m "feat: add render_mentions pongo2 template filter"
```

---

### Task 3: Apply `render_mentions` filter to description rendering

**Files:**
- Modify: `templates/partials/description.tpl:10-11` — pipe through render_mentions
- Modify: `templates/partials/blockEditor.tpl:74` — pipe block text through render_mentions

**Step 1: Update description.tpl**

In `templates/partials/description.tpl`, change lines 10-11 from:
```
{% if !preview %}{{ description|markdown2 }}{% endif %}
{% if preview %}{{ description|markdown|truncatechars_html:250 }}{% endif %}
```
to:
```
{% if !preview %}{{ description|render_mentions|markdown2 }}{% endif %}
{% if preview %}{{ description|render_mentions|markdown|truncatechars_html:250 }}{% endif %}
```

Note: `render_mentions` runs first to convert markers to HTML before markdown processes the surrounding text.

**Step 2: Update blockEditor.tpl**

In `templates/partials/blockEditor.tpl`, line 74 currently renders:
```html
<div class="prose max-w-none font-sans" x-html="renderMarkdown(block.content?.text || '')"></div>
```

The block editor renders on the client side via `renderMarkdown()`, so we need a different approach here. The mention rendering for blocks will happen client-side. We'll handle this in Task 7 (frontend mention renderer).

**Step 3: Verify the app builds and renders correctly**

Run: `npm run build && go build --tags 'json1 fts5'`
Expected: builds without errors

**Step 4: Commit**

```bash
git add templates/partials/description.tpl
git commit -m "feat: pipe descriptions through render_mentions filter"
```

---

### Task 4: CSS styles for mention rendering

**Files:**
- Modify: `public/index.css` — add mention styles

**Step 1: Add mention CSS**

Add to the end of `public/index.css`:

```css
/* @-mention styles */
.mention-inline {
    text-decoration: none;
    color: inherit;
}

.mention-inline-thumb {
    display: inline;
    height: 1.25rem;
    vertical-align: text-bottom;
    border-radius: 0.25rem;
    margin-right: 0.125rem;
}

.mention-card {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem;
    border: 1px solid #d6d3d1;
    border-radius: 0.5rem;
    text-decoration: none;
    color: inherit;
    max-width: 20rem;
    margin: 0.5rem 0;
    transition: background-color 0.15s;
}

.mention-card:hover {
    background-color: #f5f5f4;
}

.mention-card-thumb {
    width: 4rem;
    height: 4rem;
    object-fit: cover;
    border-radius: 0.375rem;
    flex-shrink: 0;
}

.mention-card-name {
    font-size: 0.875rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.mention-badge {
    display: inline-flex;
    align-items: center;
    padding: 0.0625rem 0.375rem;
    border-radius: 0.25rem;
    font-size: 0.8125rem;
    font-weight: 500;
    text-decoration: none;
    vertical-align: baseline;
}

.mention-note {
    background-color: #dbeafe;
    color: #1e40af;
}

.mention-group {
    background-color: #dcfce7;
    color: #166534;
}

.mention-tag {
    background-color: #fef3c7;
    color: #92400e;
}

.mention-resource {
    background-color: #e0e7ff;
    color: #3730a3;
}

.mention-category {
    background-color: #f3e8ff;
    color: #6b21a8;
}

.mention-missing {
    color: #9ca3af;
    text-decoration: line-through;
    font-style: italic;
}
```

**Step 2: Verify CSS loads**

Run: `npm run build`
Expected: builds successfully

**Step 3: Commit**

```bash
git add public/index.css
git commit -m "feat: add CSS styles for @-mention rendering"
```

---

### Task 5: Relation syncing on entity save

**Files:**
- Create: `lib/mention_relations.go`
- Create: `lib/mention_relations_test.go`
- Modify: `application_context/note_context.go:156-158` — call syncer after note commit
- Modify: `application_context/group_crud_context.go:93-107` — call syncer after group create commit
- Modify: `application_context/group_crud_context.go:194-208` — call syncer after group update commit
- Modify: `application_context/resource_crud_context.go:289-306` — call syncer after resource edit commit
- Modify: `application_context/resource_upload_context.go` — call syncer after resource create commit

**Step 1: Write the relation grouping helper + tests**

```go
// lib/mention_relations.go
package lib

// GroupMentionsByType groups parsed mentions by entity type.
// Returns maps of type -> []uint for convenient relation syncing.
func GroupMentionsByType(mentions []Mention) map[string][]uint {
	result := make(map[string][]uint)
	for _, m := range mentions {
		result[m.Type] = append(result[m.Type], m.ID)
	}
	return result
}
```

```go
// lib/mention_relations_test.go
package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroupMentionsByType(t *testing.T) {
	mentions := []Mention{
		{Type: "resource", ID: 1, Name: "a"},
		{Type: "tag", ID: 2, Name: "b"},
		{Type: "resource", ID: 3, Name: "c"},
		{Type: "group", ID: 4, Name: "d"},
	}
	grouped := GroupMentionsByType(mentions)
	assert.ElementsMatch(t, []uint{1, 3}, grouped["resource"])
	assert.ElementsMatch(t, []uint{2}, grouped["tag"])
	assert.ElementsMatch(t, []uint{4}, grouped["group"])
}

func TestGroupMentionsByType_Empty(t *testing.T) {
	grouped := GroupMentionsByType(nil)
	assert.Empty(t, grouped)
}
```

**Step 2: Run tests**

Run: `go test ./lib/ -run TestGroupMentionsByType -v`
Expected: all PASS

**Step 3: Add mention syncing to note context**

In `application_context/note_context.go`, after `tx.Commit()` succeeds (line 158) and before the logging (line 160), add:

```go
	// Sync mentions from description to relations
	ctx.syncMentionsForNote(&note)
```

Then add the helper method somewhere in the same file (or a new file `application_context/mention_sync.go`):

Create `application_context/mention_sync.go`:

```go
package application_context

import (
	"encoding/json"
	"log"

	"mahresources/lib"
	"mahresources/models"
)

// syncMentionsForNote parses @-mentions from the note's description and block content,
// then adds any referenced entities as relations.
func (ctx *MahresourcesContext) syncMentionsForNote(note *models.Note) {
	text := note.Description

	// Also gather text from note blocks
	var blocks []models.NoteBlock
	if err := ctx.db.Where("note_id = ? AND type = ?", note.ID, "text").Find(&blocks).Error; err == nil {
		for _, block := range blocks {
			var content struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(block.Content, &content) == nil {
				text += "\n" + content.Text
			}
		}
	}

	mentions := lib.ParseMentions(text)
	if len(mentions) == 0 {
		return
	}

	grouped := lib.GroupMentionsByType(mentions)

	if ids, ok := grouped["tag"]; ok {
		if err := ctx.AddTagsToNote(note.ID, ids); err != nil {
			log.Printf("mention sync: failed to add tags to note %d: %v", note.ID, err)
		}
	}
	if ids, ok := grouped["group"]; ok {
		if err := ctx.AddGroupsToNote(note.ID, ids); err != nil {
			log.Printf("mention sync: failed to add groups to note %d: %v", note.ID, err)
		}
	}
	if ids, ok := grouped["resource"]; ok {
		if err := ctx.AddResourcesToNote(note.ID, ids); err != nil {
			log.Printf("mention sync: failed to add resources to note %d: %v", note.ID, err)
		}
	}
}

// syncMentionsForGroup parses @-mentions from the group's description
// and adds any referenced entities as relations.
func (ctx *MahresourcesContext) syncMentionsForGroup(group *models.Group) {
	mentions := lib.ParseMentions(group.Description)
	if len(mentions) == 0 {
		return
	}

	grouped := lib.GroupMentionsByType(mentions)

	if ids, ok := grouped["tag"]; ok {
		tags := BuildAssociationSlice(ids, TagFromID)
		if err := ctx.db.Model(group).Association("Tags").Append(&tags); err != nil {
			log.Printf("mention sync: failed to add tags to group %d: %v", group.ID, err)
		}
	}
	if ids, ok := grouped["note"]; ok {
		notes := BuildAssociationSlice(ids, NoteFromID)
		if err := ctx.db.Model(group).Association("RelatedNotes").Append(&notes); err != nil {
			log.Printf("mention sync: failed to add notes to group %d: %v", group.ID, err)
		}
	}
	if ids, ok := grouped["resource"]; ok {
		resources := BuildAssociationSlice(ids, ResourceFromID)
		if err := ctx.db.Model(group).Association("RelatedResources").Append(&resources); err != nil {
			log.Printf("mention sync: failed to add resources to group %d: %v", group.ID, err)
		}
	}
	if ids, ok := grouped["group"]; ok {
		groups := BuildAssociationSlice(ids, GroupFromID)
		if err := ctx.db.Model(group).Association("RelatedGroups").Append(&groups); err != nil {
			log.Printf("mention sync: failed to add groups to group %d: %v", group.ID, err)
		}
	}
}

// syncMentionsForResource parses @-mentions from the resource's description
// and adds any referenced entities as relations.
func (ctx *MahresourcesContext) syncMentionsForResource(resource *models.Resource) {
	mentions := lib.ParseMentions(resource.Description)
	if len(mentions) == 0 {
		return
	}

	grouped := lib.GroupMentionsByType(mentions)

	if ids, ok := grouped["tag"]; ok {
		tags := BuildAssociationSlice(ids, TagFromID)
		if err := ctx.db.Model(resource).Association("Tags").Append(&tags); err != nil {
			log.Printf("mention sync: failed to add tags to resource %d: %v", resource.ID, err)
		}
	}
	if ids, ok := grouped["note"]; ok {
		notes := BuildAssociationSlice(ids, NoteFromID)
		if err := ctx.db.Model(resource).Association("Notes").Append(&notes); err != nil {
			log.Printf("mention sync: failed to add notes to resource %d: %v", resource.ID, err)
		}
	}
	if ids, ok := grouped["group"]; ok {
		groups := BuildAssociationSlice(ids, GroupFromID)
		if err := ctx.db.Model(resource).Association("Groups").Append(&groups); err != nil {
			log.Printf("mention sync: failed to add groups to resource %d: %v", resource.ID, err)
		}
	}
}
```

**Step 4: Add sync calls to save functions**

In `application_context/note_context.go`, after line 158 (`tx.Commit()` check), before line 160 (logging), add:
```go
	ctx.syncMentionsForNote(&note)
```

In `application_context/group_crud_context.go` `CreateGroup`, after line 95 (`tx.Commit()` check), before line 97 (logging), add:
```go
	ctx.syncMentionsForGroup(&group)
```

In `application_context/group_crud_context.go` `UpdateGroup`, after line 196 (`tx.Commit()` check), before line 198 (logging), add:
```go
	ctx.syncMentionsForGroup(group)
```

In `application_context/resource_crud_context.go` `EditResource`, after line 293 (err check after `WithTransaction`), before line 295 (logging), add:
```go
	ctx.syncMentionsForResource(&resource)
```

In `application_context/resource_upload_context.go` `AddResource`, find the commit + logging section (after the resource is fully saved and committed) and add:
```go
	ctx.syncMentionsForResource(res)
```

**Step 5: Run all tests**

Run: `go test ./...`
Expected: all PASS

**Step 6: Commit**

```bash
git add lib/mention_relations.go lib/mention_relations_test.go application_context/mention_sync.go application_context/note_context.go application_context/group_crud_context.go application_context/resource_crud_context.go application_context/resource_upload_context.go
git commit -m "feat: sync @-mention relations on entity save"
```

---

### Task 6: Frontend mention textarea component

**Files:**
- Create: `src/components/mentionTextarea.js`
- Modify: `src/main.js:40` — add import
- Modify: `src/main.js:110` — add registration

**Step 1: Create the Alpine component**

```js
// src/components/mentionTextarea.js
import { abortableFetch } from '../index.js';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';

export function mentionTextarea(allowedTypes = '') {
  return {
    mentionActive: false,
    mentionQuery: '',
    mentionResults: [],
    mentionSelectedIndex: 0,
    mentionLoading: false,
    mentionStart: -1,  // caret position where @ was typed
    _requestAborter: null,
    _debounceTimer: null,
    _liveRegion: null,

    typeIcons: {
      resource: '\u{1F4C4}',
      note: '\u{1F4DD}',
      group: '\u{1F465}',
      tag: '\u{1F3F7}',
      category: '\u{1F4C1}',
    },

    typeLabels: {
      resource: 'Resource',
      note: 'Note',
      group: 'Group',
      tag: 'Tag',
      category: 'Category',
    },

    init() {
      this._liveRegion = createLiveRegion(this.$el.closest('form') || this.$el.parentElement);
    },

    destroy() {
      this._liveRegion?.destroy();
      this._cleanup();
    },

    _cleanup() {
      if (this._debounceTimer) {
        clearTimeout(this._debounceTimer);
        this._debounceTimer = null;
      }
      if (this._requestAborter) {
        this._requestAborter();
        this._requestAborter = null;
      }
    },

    onKeydown(e) {
      if (!this.mentionActive) return;

      if (e.key === 'ArrowDown') {
        e.preventDefault();
        this.mentionSelectedIndex = (this.mentionSelectedIndex + 1) % this.mentionResults.length;
        this._announceSelected();
        this._scrollToSelected();
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        this.mentionSelectedIndex = this.mentionSelectedIndex === 0
          ? this.mentionResults.length - 1
          : this.mentionSelectedIndex - 1;
        this._announceSelected();
        this._scrollToSelected();
      } else if (e.key === 'Enter' && this.mentionResults.length > 0) {
        e.preventDefault();
        this.selectMention(this.mentionResults[this.mentionSelectedIndex]);
      } else if (e.key === 'Escape') {
        e.preventDefault();
        this.closeMention();
      }
    },

    onInput(e) {
      const textarea = e.target;
      const cursorPos = textarea.selectionStart;
      const text = textarea.value;

      // Find the @ that triggered this mention
      if (this.mentionActive) {
        // Check if we've moved before the @ or deleted it
        if (cursorPos <= this.mentionStart || text[this.mentionStart] !== '@') {
          this.closeMention();
          return;
        }

        // Extract query: text between @ and cursor
        const query = text.substring(this.mentionStart + 1, cursorPos);

        // Close if query contains newline or the user typed a space right after @
        if (query.includes('\n') || (query.length === 0 && text[cursorPos - 1] === ' ')) {
          this.closeMention();
          return;
        }

        this.mentionQuery = query;
        this._searchMentions();
        return;
      }

      // Detect new @ trigger
      if (cursorPos > 0 && text[cursorPos - 1] === '@') {
        // Only trigger if @ is at start or preceded by whitespace
        const charBefore = cursorPos > 1 ? text[cursorPos - 2] : ' ';
        if (/\s/.test(charBefore) || cursorPos === 1) {
          this.mentionActive = true;
          this.mentionStart = cursorPos - 1;
          this.mentionQuery = '';
          this.mentionResults = [];
          this.mentionSelectedIndex = 0;
          this._liveRegion?.announce('Mention autocomplete activated. Type to search.');
        }
      }
    },

    _searchMentions() {
      this._cleanup();

      const query = this.mentionQuery.trim();
      if (query.length < 2) {
        this.mentionResults = [];
        return;
      }

      const debounceTime = query.length < 3 ? 300 : 150;

      this._debounceTimer = setTimeout(() => {
        this.mentionLoading = true;

        let url = `/v1/search?q=${encodeURIComponent(query)}&limit=10`;
        if (allowedTypes) {
          url += `&types=${encodeURIComponent(allowedTypes)}`;
        }

        const { abort, ready } = abortableFetch(url);
        this._requestAborter = abort;

        ready.then(r => r.json())
          .then(data => {
            if (this.mentionQuery.trim() === query) {
              this.mentionResults = data.results || [];
              this.mentionSelectedIndex = 0;
              if (this.mentionResults.length > 0) {
                this._liveRegion?.announce(
                  `${this.mentionResults.length} result${this.mentionResults.length === 1 ? '' : 's'} found. Use arrow keys to navigate.`
                );
              } else {
                this._liveRegion?.announce('No results found.');
              }
            }
          })
          .catch(err => {
            if (err.name !== 'AbortError') {
              console.error('Mention search error:', err);
            }
          })
          .finally(() => {
            this.mentionLoading = false;
          });
      }, debounceTime);
    },

    selectMention(result) {
      const textarea = this.$refs.mentionInput || this.$el.querySelector('textarea');
      if (!textarea) return;

      const before = textarea.value.substring(0, this.mentionStart);
      const after = textarea.value.substring(textarea.selectionStart);
      const marker = `@[${result.type}:${result.id}:${result.name}]`;

      textarea.value = before + marker + after;

      // Update Alpine model if bound
      textarea.dispatchEvent(new Event('input', { bubbles: true }));

      // Position cursor after the marker
      const newPos = before.length + marker.length;
      textarea.setSelectionRange(newPos, newPos);
      textarea.focus();

      this._liveRegion?.announce(`Inserted mention: ${result.name}`);
      this.closeMention();
    },

    closeMention() {
      this.mentionActive = false;
      this.mentionQuery = '';
      this.mentionResults = [];
      this.mentionSelectedIndex = 0;
      this._cleanup();
    },

    _announceSelected() {
      const result = this.mentionResults[this.mentionSelectedIndex];
      if (result) {
        const label = this.typeLabels[result.type] || result.type;
        this._liveRegion?.announce(
          `${result.name}, ${label}, ${this.mentionSelectedIndex + 1} of ${this.mentionResults.length}`
        );
      }
    },

    _scrollToSelected() {
      this.$nextTick(() => {
        const dropdown = this.$refs.mentionDropdown;
        const selected = dropdown?.querySelector('[data-mention-selected="true"]');
        if (selected) {
          selected.scrollIntoView({ block: 'nearest' });
        }
      });
    },

    getDropdownStyle() {
      const textarea = this.$refs.mentionInput || this.$el.querySelector('textarea');
      if (!textarea) return {};

      // Position dropdown below the textarea (simple approach)
      // A more sophisticated version would calculate caret coordinates
      const rect = textarea.getBoundingClientRect();
      const parentRect = (textarea.offsetParent || textarea.parentElement).getBoundingClientRect();

      return {
        position: 'absolute',
        left: '0px',
        top: (textarea.offsetTop + textarea.offsetHeight) + 'px',
        width: Math.min(rect.width, 400) + 'px',
        zIndex: '50',
      };
    },

    getIcon(type) {
      return this.typeIcons[type] || '\u{1F4CC}';
    },

    getLabel(type) {
      return this.typeLabels[type] || type;
    },

    escapeHTML(str) {
      const div = document.createElement('div');
      div.textContent = str;
      return div.innerHTML;
    },

    highlightMatch(text, query) {
      if (!text || !query) return this.escapeHTML(text);
      const escaped = this.escapeHTML(text);
      const escapedQuery = this.escapeHTML(query);
      const regex = new RegExp(`(${escapedQuery.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi');
      return escaped.replace(regex, '<mark class="bg-yellow-200">$1</mark>');
    },
  };
}
```

**Step 2: Register in main.js**

In `src/main.js`, add after line 40 (the blocks import):
```js
import { mentionTextarea } from './components/mentionTextarea.js';
```

After line 110 (`cardActionMenu` registration), add:
```js
Alpine.data('mentionTextarea', mentionTextarea);
```

**Step 3: Build JS**

Run: `npm run build-js`
Expected: builds successfully

**Step 4: Commit**

```bash
git add src/components/mentionTextarea.js src/main.js
git commit -m "feat: add mentionTextarea Alpine.js component"
```

---

### Task 7: Apply mention component to form templates

**Files:**
- Modify: `templates/partials/form/createFormTextareaInput.tpl` — wrap textarea with mention component
- Modify: `templates/createNote.tpl:10` — pass allowed types
- Modify: `templates/createGroup.tpl:27` — pass allowed types
- Modify: `templates/createResource.tpl:36-44` — pass allowed types

**Step 1: Update createFormTextareaInput.tpl to support mentions**

The textarea partial needs an optional `mentionTypes` variable. When set, wrap the textarea with the mention component and add the dropdown template.

Replace the content of `templates/partials/form/createFormTextareaInput.tpl` with:

```django
{% with field_id=id|default:name %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-stone-200 sm:pt-5">
    <label for="{{ field_id }}" class="block text-sm font-mono font-medium text-stone-700 sm:mt-px sm:pt-2">
        {{ title }} {% if required %}<span class="text-red-700">*</span>{% endif %}
    </label>
    <div class="mt-1 sm:mt-0 sm:col-span-2">
        {% if mentionTypes %}
        <div class="relative" x-data="mentionTextarea('{{ mentionTypes }}')">
        {% endif %}
            <textarea
                    id="{{ field_id }}"
                    name="{{ name }}"
                    rows="3"
                    {% if required %}required aria-required="true"{% endif %}
                    {% if mentionTypes %}
                    x-ref="mentionInput"
                    @input="onInput($event)"
                    @keydown="onKeydown($event)"
                    role="combobox"
                    aria-autocomplete="list"
                    :aria-expanded="mentionActive && mentionResults.length > 0"
                    aria-haspopup="listbox"
                    {% endif %}
                    class="{% if big %}{% else %}max-w-lg{% endif %} shadow-sm block w-full focus:ring-amber-600 focus:border-amber-600 sm:text-sm border-stone-300 rounded-md"
            >{{ value }}</textarea>
        {% if mentionTypes %}
            {% include "/partials/form/mentionDropdown.tpl" %}
        </div>
        {% endif %}
        {% if required %}
        <span class="text-sm font-sans text-stone-500" id="{{ field_id }}-description">Required</span>
        <script>
            document.getElementById("{{ field_id }}").setAttribute("aria-describedby", "{{ field_id }}-description");
        </script>
        {% endif %}
    </div>
</div>
{% endwith %}
```

**Step 2: Create the mention dropdown partial**

Create `templates/partials/form/mentionDropdown.tpl`:

```django
<div x-ref="mentionDropdown"
     x-show="mentionActive && mentionResults.length > 0"
     x-cloak
     :style="getDropdownStyle()"
     role="listbox"
     aria-label="Mention suggestions"
     class="bg-white border border-stone-300 rounded-lg shadow-lg max-h-60 overflow-y-auto">
    <template x-for="(result, index) in mentionResults" :key="result.type + ':' + result.id">
        <button type="button"
                @click.prevent="selectMention(result)"
                @mouseenter="mentionSelectedIndex = index"
                :data-mention-selected="index === mentionSelectedIndex"
                :class="index === mentionSelectedIndex ? 'bg-amber-50' : ''"
                class="w-full text-left px-3 py-2 flex items-center gap-2 hover:bg-stone-50 cursor-pointer text-sm"
                role="option"
                :aria-selected="index === mentionSelectedIndex">
            <span class="flex-shrink-0" x-text="getIcon(result.type)" aria-hidden="true"></span>
            <span class="flex-1 min-w-0">
                <span class="font-medium truncate block" x-html="highlightMatch(result.name, mentionQuery)"></span>
                <span class="text-xs text-stone-500 truncate block" x-text="result.description" x-show="result.description"></span>
            </span>
            <span class="flex-shrink-0 text-xs font-mono px-1.5 py-0.5 rounded"
                  :class="{
                      'bg-blue-100 text-blue-700': result.type === 'note',
                      'bg-green-100 text-green-700': result.type === 'group',
                      'bg-yellow-100 text-yellow-700': result.type === 'tag',
                      'bg-indigo-100 text-indigo-700': result.type === 'resource',
                      'bg-purple-100 text-purple-700': result.type === 'category',
                      'bg-stone-100 text-stone-700': !['note','group','tag','resource','category'].includes(result.type)
                  }"
                  x-text="getLabel(result.type)">
            </span>
        </button>
    </template>
</div>
<div x-show="mentionActive && mentionLoading" class="absolute left-0 bg-white border border-stone-300 rounded-lg shadow-lg p-3 text-sm text-stone-500" :style="getDropdownStyle()">
    Searching...
</div>
```

**Step 3: Update create templates to pass mentionTypes**

In `templates/createNote.tpl`, where the description include happens (line 10), change to pass mentionTypes:
```django
{% include "/partials/form/createFormTextareaInput.tpl" with name="Description" title="Description" value=note.Description mentionTypes="resource,group,tag" %}
```

In `templates/createGroup.tpl`, at the description include (line 27), add:
```django
{% include "/partials/form/createFormTextareaInput.tpl" with name="Description" title="Description" value=group.Description mentionTypes="resource,note,group,tag" %}
```

In `templates/createResource.tpl`, at the description textarea (lines 36-44), this one uses an inline textarea rather than the partial. Either convert it to use the partial with mentionTypes, or wrap it manually. Check the exact template content and adapt. The resource form's description should get `mentionTypes="note,group,tag"`.

**Step 4: Build and verify**

Run: `npm run build && go build --tags 'json1 fts5'`
Expected: builds without errors

**Step 5: Commit**

```bash
git add templates/partials/form/createFormTextareaInput.tpl templates/partials/form/mentionDropdown.tpl templates/createNote.tpl templates/createGroup.tpl templates/createResource.tpl
git commit -m "feat: wire up @-mention autocomplete in create/edit forms"
```

---

### Task 8: Client-side mention rendering for NoteBlock text

**Files:**
- Create: `src/utils/renderMentions.js`
- Modify: `src/components/blocks/blockText.js` — apply mention component
- Modify: `templates/partials/blockEditor.tpl:74` — use renderMentions in view mode
- Modify: `templates/partials/blockEditor.tpl:77-85` — wrap textarea with mention component

**Step 1: Create client-side mention renderer**

```js
// src/utils/renderMentions.js

const mentionRegex = /@\[([a-zA-Z]+):(\d+):([^\]]+)\]/g;

const entityPaths = {
  resource: '/resource',
  note: '/note',
  group: '/group',
  tag: '/tag',
  category: '/category',
};

function escapeHTML(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

/**
 * Replaces @[type:id:name] markers in text with HTML links/thumbnails.
 * @param {string} text - raw text that may contain mention markers
 * @returns {string} - HTML string with mentions rendered
 */
export function renderMentions(text) {
  if (!text) return text;

  // Process line by line to detect standalone vs inline
  const lines = text.split('\n');
  const processedLines = lines.map(line => {
    const trimmed = line.trim();
    const matches = [...trimmed.matchAll(mentionRegex)];

    // Check if this line contains exactly one mention and nothing else
    if (matches.length === 1 && trimmed === matches[0][0]) {
      const [, type, id, name] = matches[0];
      const escapedName = escapeHTML(name);

      if (type === 'resource') {
        return `<a href="/resource?id=${id}" class="mention-card"><img src="/v1/resource/thumbnail?id=${id}" alt="${escapedName}" class="mention-card-thumb"><span class="mention-card-name">${escapedName}</span></a>`;
      }

      const path = entityPaths[type] || '/' + type;
      return `<a href="${path}?id=${id}" class="mention-badge mention-${type}">${escapedName}</a>`;
    }

    // Inline mentions
    return line.replace(mentionRegex, (match, type, id, name) => {
      const escapedName = escapeHTML(name);

      if (type === 'resource') {
        return `<a href="/resource?id=${id}" class="mention-inline"><img src="/v1/resource/thumbnail?id=${id}" alt="" class="mention-inline-thumb">${escapedName}</a>`;
      }

      const path = entityPaths[type] || '/' + type;
      return `<a href="${path}?id=${id}" class="mention-badge mention-${type}">${escapedName}</a>`;
    });
  });

  return processedLines.join('\n');
}
```

**Step 2: Expose renderMentions globally**

In `src/main.js`, add import:
```js
import { renderMentions } from './utils/renderMentions.js';
```

And expose globally:
```js
window.renderMentions = renderMentions;
```

**Step 3: Update blockEditor.tpl view mode**

In `templates/partials/blockEditor.tpl`, line 74, change:
```html
<div class="prose max-w-none font-sans" x-html="renderMarkdown(block.content?.text || '')"></div>
```
to:
```html
<div class="prose max-w-none font-sans" x-html="renderMarkdown(renderMentions(block.content?.text || ''))"></div>
```

**Step 4: Update blockEditor.tpl edit mode for text blocks**

In `templates/partials/blockEditor.tpl`, lines 76-85, wrap the textarea with the mention component. Change:
```html
<template x-if="editMode">
    <div x-data="blockText(block, (id, content) => updateBlockContent(id, content), (id, content) => updateBlockContentDebounced(id, content))">
        <textarea
            x-model="text"
            @input="onInput()"
            @blur="save()"
            class="w-full min-h-[100px] p-2 border border-stone-300 rounded resize-y"
            placeholder="Enter text..."
        ></textarea>
    </div>
</template>
```
to:
```html
<template x-if="editMode">
    <div x-data="blockText(block, (id, content) => updateBlockContent(id, content), (id, content) => updateBlockContentDebounced(id, content))">
        <div class="relative" x-data="mentionTextarea('resource,group,tag')">
            <textarea
                x-ref="mentionInput"
                x-model="text"
                @input="onInput(); mentionTextarea_onInput && mentionTextarea_onInput($event)"
                @keydown="onKeydown($event)"
                @blur="save()"
                class="w-full min-h-[100px] p-2 border border-stone-300 rounded resize-y"
                placeholder="Enter text..."
                role="combobox"
                aria-autocomplete="list"
                :aria-expanded="mentionActive && mentionResults.length > 0"
                aria-haspopup="listbox"
            ></textarea>
            {% include "/partials/form/mentionDropdown.tpl" %}
        </div>
    </div>
</template>
```

Note: The `@input` needs to call both `blockText.onInput()` and `mentionTextarea.onInput()`. Since these are nested Alpine scopes, the outer `mentionTextarea` scope's `onInput` is accessible. The exact wiring may need adjustment — the `@input` should call the blockText's `onInput()` (for debounced saving) and the event will also bubble to the mentionTextarea wrapper which handles the `@input` via `onInput($event)`. Restructure so that:

- The `mentionTextarea` div has `@input="onInput($event)"` for mention detection
- The textarea has `@input="onInput()"` for blockText saving and `@keydown="$parent.onKeydown($event)"` to delegate to the mention component

The cleanest approach:
```html
<template x-if="editMode">
    <div x-data="blockText(block, (id, content) => updateBlockContent(id, content), (id, content) => updateBlockContentDebounced(id, content))">
        <div class="relative" x-data="mentionTextarea('resource,group,tag')" @input="onInput($event)">
            <textarea
                x-ref="mentionInput"
                x-model="text"
                @input="$parent.onInput()"
                @keydown="onKeydown($event)"
                @blur="$parent.save()"
                class="w-full min-h-[100px] p-2 border border-stone-300 rounded resize-y"
                placeholder="Enter text..."
                role="combobox"
                aria-autocomplete="list"
                :aria-expanded="mentionActive && mentionResults.length > 0"
                aria-haspopup="listbox"
            ></textarea>
            {% include "/partials/form/mentionDropdown.tpl" %}
        </div>
    </div>
</template>
```

Here `@input` on the wrapper div captures the bubbling event for mention detection, while `@input` on the textarea calls blockText's `onInput()` via `$parent`. The `@keydown` on the textarea goes to the mentionTextarea's `onKeydown`. The `@blur` calls blockText's `save()` via `$parent`.

**Step 5: Build and verify**

Run: `npm run build && go build --tags 'json1 fts5'`
Expected: builds without errors

**Step 6: Commit**

```bash
git add src/utils/renderMentions.js src/main.js src/components/blocks/blockText.js templates/partials/blockEditor.tpl
git commit -m "feat: add client-side mention rendering for NoteBlock text blocks"
```

---

### Task 9: E2E test for @-mentions

**Files:**
- Create: `e2e/tests/mentions.spec.ts`

**Step 1: Write the E2E test**

```typescript
// e2e/tests/mentions.spec.ts
import { test, expect } from '../fixtures/base.fixture';

test.describe('Mention Autocomplete', () => {
  test('should show autocomplete dropdown when typing @ in note description', async ({ page, api }) => {
    // Create a group to be mentionable
    const group = await api.createGroup({ Name: 'Test Mention Group' });

    // Navigate to create note page
    await page.goto('/note/create');

    // Type @ followed by search query in description
    const description = page.locator('#Description');
    await description.fill('Check out @Te');
    // Wait for dropdown to appear
    const dropdown = page.locator('[role="listbox"]');
    await expect(dropdown).toBeVisible({ timeout: 5000 });

    // Should show the group in results
    await expect(dropdown.locator('button').first()).toContainText('Test Mention Group');
  });

  test('should insert mention marker on selection', async ({ page, api }) => {
    const group = await api.createGroup({ Name: 'Mentioned Group' });

    await page.goto('/note/create');

    const description = page.locator('#Description');
    await description.fill('See @Mentioned');

    const dropdown = page.locator('[role="listbox"]');
    await expect(dropdown).toBeVisible({ timeout: 5000 });

    // Click the first result
    await dropdown.locator('button').first().click();

    // Verify the marker was inserted
    const value = await description.inputValue();
    expect(value).toContain(`@[group:${group.ID}:Mentioned Group]`);
  });

  test('should add relation when saving note with mention', async ({ page, api }) => {
    const group = await api.createGroup({ Name: 'Related Via Mention' });
    const ownerGroup = await api.createGroup({ Name: 'Owner' });

    await page.goto('/note/create');
    await page.locator('#Name').fill('Note With Mention');

    // Manually set description with mention marker
    await page.locator('#Description').fill(`Links to @[group:${group.ID}:Related Via Mention]`);

    // Set owner (required)
    // Use the autocompleter for owner
    const ownerInput = page.locator('[data-autocompleter-name="OwnerId"] input[type="text"]');
    await ownerInput.fill('Owner');
    await page.locator('[data-autocompleter-name="OwnerId"] .autocompleter-result').first().click();

    // Submit
    await page.locator('button[type="submit"]').click();

    // Verify the note was created and has the group as a relation
    await expect(page).toHaveURL(/\/note\?id=\d+/);

    // The group should appear in related groups
    await expect(page.locator('text=Related Via Mention')).toBeVisible();
  });
});
```

**Step 2: Run the E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Mention"`
Expected: tests pass (adjust selectors as needed based on actual DOM structure)

**Step 3: Commit**

```bash
git add e2e/tests/mentions.spec.ts
git commit -m "test: add E2E tests for @-mention autocomplete"
```

---

### Task 10: Manual integration testing

**No files to modify** — this is a verification task.

**Step 1: Build and start server**

Run: `npm run build && go build --tags 'json1 fts5' && ./mahresources -ephemeral -bind-address=:8181`

**Step 2: Verify the following manually**

1. Navigate to `/note/create`, type `@` followed by text in description — dropdown should appear
2. Select an entity — marker should be inserted
3. Save the note — the mentioned entity should appear as a relation
4. View the note — the mention should render as a styled link/thumbnail
5. Repeat for groups and resources
6. Test keyboard navigation: arrow keys, Enter, Escape
7. Test standalone vs inline resource mentions (put a resource mention alone on a line vs inline with text)

**Step 3: Run full test suite**

Run: `go test ./... && cd e2e && npm run test:with-server`
Expected: all PASS

---

Plan complete and saved to `docs/plans/2026-03-12-at-mentions-impl.md`. Two execution options:

**1. Subagent-Driven (this session)** — I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** — Open new session with executing-plans, batch execution with checkpoints

Which approach?
