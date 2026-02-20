---
sidebar_position: 5
---

# Custom Block Types

The block editor uses an extensible block type system. Contributors can add new content block types by implementing a backend Go type for validation and storage, plus a frontend Alpine.js component for rendering and editing.

## Overview

Block types define how different types of content (text, headings, images, tables, etc.) are validated, stored, and displayed within notes. The system uses a registry pattern where block types auto-register themselves at startup.

**Existing block types:**
- `text` - Plain text content with Markdown support
- `heading` - Section headings (H1-H6)
- `divider` - Visual separator
- `gallery` - Collection of resource images
- `references` - Links to groups
- `todos` - Checklist items
- `table` - Tabular data
- `calendar` - Calendar with iCal sources and custom events

## Architecture

A complete block type implementation requires:

1. **Backend (Go)** - Type definition, validation, and default values
2. **Frontend (JavaScript)** - Alpine.js component for UI
3. **Template (Pongo2)** - HTML structure in the block editor template

```
models/block_types/
├── block_type.go      # Interface definition
├── registry.go        # Global type registry
├── text.go           # Text block implementation
├── heading.go        # Heading block implementation
└── your_block.go     # Your new block type

src/components/blocks/
├── index.js          # Exports all block components
├── blockText.js      # Text block component
├── blockHeading.js   # Heading block component
└── blockYourType.js  # Your new component

templates/partials/
└── blockEditor.tpl   # Template with block rendering
```

## Backend: Go Implementation

### Step 1: Create the Block Type File

Create a new file in `models/block_types/` for your block type:

```go
// models/block_types/quote.go
package block_types

import (
    "encoding/json"
    "errors"
)
```

### Step 2: Define Content and State Schemas

Content holds the block's persistent data. State holds UI-related data that may change without affecting the core content.

```go
// quoteContent represents the content schema for quote blocks.
type quoteContent struct {
    Text       string `json:"text"`
    Author     string `json:"author"`
    SourceURL  string `json:"sourceUrl,omitempty"`
}

// quoteState represents the state schema for quote blocks.
type quoteState struct {
    Collapsed bool `json:"collapsed"`
}
```

### Step 3: Implement the BlockType Interface

Create a struct and implement all interface methods:

```go
// QuoteBlockType implements BlockType for quotation content.
type QuoteBlockType struct{}

func (q QuoteBlockType) Type() string {
    return "quote"
}

func (q QuoteBlockType) ValidateContent(content json.RawMessage) error {
    var c quoteContent
    if err := json.Unmarshal(content, &c); err != nil {
        return err
    }
    if c.Text == "" {
        return errors.New("quote block must have text content")
    }
    return nil
}

func (q QuoteBlockType) ValidateState(state json.RawMessage) error {
    var s quoteState
    if err := json.Unmarshal(state, &s); err != nil {
        return err
    }
    // Collapsed is a boolean, no additional validation needed
    return nil
}

func (q QuoteBlockType) DefaultContent() json.RawMessage {
    return json.RawMessage(`{"text": "", "author": "", "sourceUrl": ""}`)
}

func (q QuoteBlockType) DefaultState() json.RawMessage {
    return json.RawMessage(`{"collapsed": false}`)
}
```

### Step 4: Auto-Register via init()

The `init()` function automatically registers the block type when the package loads:

```go
func init() {
    RegisterBlockType(QuoteBlockType{})
}
```

### Complete Backend Example

Here is the complete `quote.go` file:

```go
// models/block_types/quote.go
package block_types

import (
    "encoding/json"
    "errors"
)

// quoteContent represents the content schema for quote blocks.
type quoteContent struct {
    Text      string `json:"text"`
    Author    string `json:"author"`
    SourceURL string `json:"sourceUrl,omitempty"`
}

// quoteState represents the state schema for quote blocks.
type quoteState struct {
    Collapsed bool `json:"collapsed"`
}

// QuoteBlockType implements BlockType for quotation content.
type QuoteBlockType struct{}

func (q QuoteBlockType) Type() string {
    return "quote"
}

func (q QuoteBlockType) ValidateContent(content json.RawMessage) error {
    var c quoteContent
    if err := json.Unmarshal(content, &c); err != nil {
        return err
    }
    if c.Text == "" {
        return errors.New("quote block must have text content")
    }
    return nil
}

func (q QuoteBlockType) ValidateState(state json.RawMessage) error {
    var s quoteState
    if err := json.Unmarshal(state, &s); err != nil {
        return err
    }
    return nil
}

func (q QuoteBlockType) DefaultContent() json.RawMessage {
    return json.RawMessage(`{"text": "", "author": "", "sourceUrl": ""}`)
}

func (q QuoteBlockType) DefaultState() json.RawMessage {
    return json.RawMessage(`{"collapsed": false}`)
}

func init() {
    RegisterBlockType(QuoteBlockType{})
}
```

## Frontend: Alpine.js Component

### Step 1: Create the Component File

Create a new file in `src/components/blocks/`:

```javascript
// src/components/blocks/blockQuote.js
export function blockQuote() {
  return {
    // Getters for content properties
    get text() {
      return this.block?.content?.text || '';
    },
    get author() {
      return this.block?.content?.author || '';
    },
    get sourceUrl() {
      return this.block?.content?.sourceUrl || '';
    },

    // Getters for state properties
    get collapsed() {
      return this.block?.state?.collapsed || false;
    },

    // Methods to update content
    updateQuote(text, author, sourceUrl) {
      this.$dispatch('update-content', { text, author, sourceUrl });
    },

    // Methods to update state
    toggleCollapsed() {
      this.$dispatch('update-state', { collapsed: !this.collapsed });
    }
  };
}
```

### Step 2: Export from index.js

Add your component to `src/components/blocks/index.js`:

```javascript
// src/components/blocks/index.js
export { blockText } from './blockText.js';
export { blockHeading } from './blockHeading.js';
export { blockDivider } from './blockDivider.js';
export { blockTodos } from './blockTodos.js';
export { blockGallery } from './blockGallery.js';
export { blockReferences } from './blockReferences.js';
export { blockTable } from './blockTable.js';
export { blockQuote } from './blockQuote.js';  // Add this line
```

### Step 3: Register in main.js

Import and register your component in `src/main.js`:

```javascript
// In the import section:
import {
  blockText,
  blockHeading,
  blockDivider,
  blockTodos,
  blockGallery,
  blockReferences,
  blockTable,
  blockQuote  // Add this
} from './components/blocks/index.js';

// In the Alpine.data registration section:
Alpine.data('blockQuote', blockQuote);
```

### Step 4: Update blockEditor.js

Add default content and type metadata in `src/components/blockEditor.js`:

```javascript
// In getDefaultContent method:
getDefaultContent(type) {
  const defaults = {
    text: { text: '' },
    heading: { text: '', level: 2 },
    divider: {},
    gallery: { resourceIds: [] },
    references: { groupIds: [] },
    todos: { items: [] },
    table: { columns: [], rows: [] },
    quote: { text: '', author: '', sourceUrl: '' }  // Add this
  };
  return defaults[type] || {};
}

// In blockTypes array (showing built-in types plus your addition):
blockTypes: [
  { type: 'text', label: 'Text', icon: '📝' },
  { type: 'heading', label: 'Heading', icon: '🔤' },
  { type: 'divider', label: 'Divider', icon: '──' },
  { type: 'gallery', label: 'Gallery', icon: '🖼️' },
  { type: 'references', label: 'References', icon: '📁' },
  { type: 'todos', label: 'Todos', icon: '☑️' },
  { type: 'table', label: 'Table', icon: '📊' },
  { type: 'calendar', label: 'Calendar', icon: '📅' },
  { type: 'quote', label: 'Quote', icon: '💬' }  // Add this
]
```

### Step 5: Add Template in blockEditor.tpl

Add the rendering template in `templates/partials/blockEditor.tpl`:

```html
{# Quote block #}
<template x-if="block.type === 'quote'">
    <div x-data="blockQuote(block, editMode, (id, content) => updateBlockContent(id, content), (id, state) => updateBlockState(id, state))">
        <template x-if="!editMode">
            <blockquote class="border-l-4 border-gray-300 pl-4 italic">
                <p x-text="text" class="text-lg"></p>
                <template x-if="author">
                    <footer class="mt-2 text-sm text-gray-600">
                        &mdash; <span x-text="author"></span>
                        <template x-if="sourceUrl">
                            <a :href="sourceUrl" class="ml-1 text-blue-600 hover:underline" target="_blank">(source)</a>
                        </template>
                    </footer>
                </template>
            </blockquote>
        </template>
        <template x-if="editMode">
            <div class="space-y-2">
                <textarea
                    x-model="text"
                    @blur="updateQuote(text, author, sourceUrl)"
                    class="w-full min-h-[100px] p-2 border border-gray-300 rounded resize-y"
                    placeholder="Quote text..."
                ></textarea>
                <input
                    type="text"
                    x-model="author"
                    @blur="updateQuote(text, author, sourceUrl)"
                    class="w-full p-2 border border-gray-300 rounded"
                    placeholder="Author name"
                >
                <input
                    type="url"
                    x-model="sourceUrl"
                    @blur="updateQuote(text, author, sourceUrl)"
                    class="w-full p-2 border border-gray-300 rounded"
                    placeholder="Source URL (optional)"
                >
            </div>
        </template>
    </div>
</template>
```

## Content vs State

Understanding the difference between content and state is crucial:

### Content

- **Persistent data** that defines what the block contains
- Saved with the note and synced across devices
- Changes when the user explicitly edits the block
- Examples: text, heading level, resource IDs, table rows

### State

- **UI-related data** that affects how the block displays
- Can be user-specific or session-specific
- May change without user editing (e.g., collapsing a section)
- Examples: collapsed state, sort order, selected view mode

### When to Use Each

| Use Content For | Use State For |
|-----------------|---------------|
| Text/titles | Collapsed/expanded |
| References to other entities | Sort column/direction |
| Structural data (rows, items) | View mode (grid/list) |
| User-created identifiers | Temporary selections |

### Example: Todos Block

```javascript
// Content: The todo items themselves
{
  "items": [
    { "id": "abc123", "label": "Buy groceries" },
    { "id": "def456", "label": "Write documentation" }
  ]
}

// State: Which items are checked (UI state)
{
  "checked": ["abc123"]
}
```

With this separation:
- Checking/unchecking items does not modify content
- Different users can have different checked states
- Content changes are tracked separately from state changes

## Testing

### Backend Tests

Add tests in `models/block_types/registry_test.go`:

```go
func TestRegistry_GetBlockType_Quote(t *testing.T) {
    bt := GetBlockType("quote")
    assert.NotNil(t, bt)
    assert.Equal(t, "quote", bt.Type())
}

func TestRegistry_ValidateContent_Quote_Valid(t *testing.T) {
    bt := GetBlockType("quote")
    content := json.RawMessage(`{"text": "To be or not to be", "author": "Shakespeare"}`)
    err := bt.ValidateContent(content)
    assert.NoError(t, err)
}

func TestRegistry_ValidateContent_Quote_MissingText(t *testing.T) {
    bt := GetBlockType("quote")
    content := json.RawMessage(`{"text": "", "author": "Someone"}`)
    err := bt.ValidateContent(content)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "must have text content")
}

func TestRegistry_ValidateState_Quote(t *testing.T) {
    bt := GetBlockType("quote")
    state := json.RawMessage(`{"collapsed": true}`)
    err := bt.ValidateState(state)
    assert.NoError(t, err)
}
```

Run tests:

```bash
go test ./models/block_types/...
```

### E2E Tests

Add Playwright tests in `e2e/tests/blocks/`:

```typescript
test('can create and edit quote block', async ({ page }) => {
  // Create a note
  // Add a quote block
  // Edit the quote text and author
  // Verify the quote renders correctly in view mode
});
```

Run E2E tests:

```bash
cd e2e && npm run test:with-server
```

## Validation Best Practices

1. **Validate required fields** - Return clear error messages for missing data
2. **Validate data types** - Ensure numbers are in valid ranges, strings are not too long
3. **Validate relationships** - If referencing other entities, validate the references exist (if possible)
4. **Allow empty state** - State should typically allow an empty object `{}`
5. **Use meaningful error messages** - Help users understand what went wrong

```go
func (q QuoteBlockType) ValidateContent(content json.RawMessage) error {
    var c quoteContent
    if err := json.Unmarshal(content, &c); err != nil {
        return err
    }

    // Required field validation
    if c.Text == "" {
        return errors.New("quote block must have text content")
    }

    // Length validation
    if len(c.Text) > 10000 {
        return errors.New("quote text cannot exceed 10000 characters")
    }

    // Optional URL validation
    if c.SourceURL != "" {
        if _, err := url.Parse(c.SourceURL); err != nil {
            return errors.New("sourceUrl must be a valid URL")
        }
    }

    return nil
}
```

## Checklist for New Block Types

- [ ] Create `models/block_types/yourtype.go` with content/state structs
- [ ] Implement all `BlockType` interface methods
- [ ] Add `init()` function to register the type
- [ ] Create `src/components/blocks/blockYourType.js` component
- [ ] Export from `src/components/blocks/index.js`
- [ ] Register in `src/main.js` with `Alpine.data()`
- [ ] Add default content in `blockEditor.js` `getDefaultContent()`
- [ ] Add type metadata in `blockEditor.js` `blockTypes` array
- [ ] Add template in `templates/partials/blockEditor.tpl`
- [ ] Write backend tests in `models/block_types/registry_test.go`
- [ ] Write E2E tests in `e2e/tests/blocks/`
- [ ] Run `npm run build-js` to rebuild the frontend bundle
- [ ] Test the new block type manually in the application
