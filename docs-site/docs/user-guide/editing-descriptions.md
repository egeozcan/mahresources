---
sidebar_position: 7
title: Editing Descriptions
---

# Editing Descriptions

Descriptions are written in Markdown. Every Description field, a Note's **Text** field, and every text block in the block editor use the same Markdown editor: a source editor with syntax highlighting, a formatting toolbar, and autocomplete. The editor does not reformat your text.

The lightbox's description box is a plain text box and does not use this editor.

## Where the editor appears

- **Create and edit forms** for Resources, Notes, Groups, Tags, Categories, Resource Categories, Note Types, Queries, Relations, Relation Types and Template Partials.
- **Detail pages and note cards.** Double-click a Description on a detail page, or on a note card in a note list, to edit it in place. Saving needs permission to edit that entity; without it, the editor shows the error and keeps your text.
- **Text blocks.** Click **Edit Blocks** on a Note to edit its text blocks.

While the editor loads, the field is a plain text box you can already type in. Text typed during loading is kept. If the editor fails to load, the plain text box stays and works as before.

## Toolbar

| Button | Inserts |
|--------|---------|
| **B** | `**bold**` around the selection |
| **I** | `*italic*` around the selection |
| **Link** | `[selection](url)`, with `url` selected so you can type over it |
| **H** | `## ` at the start of each selected line |
| **• List** | `- ` at the start of each selected line |
| **1. List** | `1. `, `2. `, ... at the start of each selected line |
| **&lt;/&gt;** | `` `code` `` around the selection |

With nothing selected, **B**, **I** and **&lt;/&gt;** insert the pair of markers with the cursor between them. **H** and the two list buttons toggle: if every selected line already starts with the prefix, it is removed.

## Preview

Forms and in-place editing on detail pages have **Write** and **Preview** buttons. **Preview** sends the current text to the server, which renders it the same way the detail page does: Markdown, then mentions, then shortcodes. Nothing is saved.

- The preview shows in a sandboxed frame that runs no scripts, so interactive shortcode widgets do not work there. When editing a saved Resource, Note, Group, Category, Resource Category or Note Type, the preview says so.
- Links in the preview open in a new tab.
- On a create form there is no saved entity yet, so shortcodes stay as literal text. On the six entity types whose Descriptions run shortcodes (Resource, Note, Group, Category, Resource Category, Note Type), the preview says "Shortcodes render after saving."
- Tag, Query, Relation, Relation Type and Template Partial Descriptions never run shortcodes, on the detail page or in the preview.

Text blocks have no Preview button. Click **Done** to leave edit mode and see the rendered blocks.

## Autocomplete

**Mentions.** Type `@` at the start of a line or after a space, then at least two characters, to search Resources, Notes, Groups and Tags. Choosing a result inserts a mention such as `@[group:12:Projects]`. See [Mentions](../features/mentions.md).

**Shortcodes.** On Resource, Note, Group, Category, Resource Category and Note Type Descriptions, and in text blocks, the editor also completes shortcode names and attributes, and shows help when you hover over a shortcode. See [Shortcodes](../features/shortcodes.md).

## Keyboard shortcuts

| Shortcut | Action |
|----------|--------|
| Ctrl/Cmd + B | Bold |
| Ctrl/Cmd + I | Italic |
| Ctrl/Cmd + Shift + K | Link |
| Ctrl/Cmd + K | Global search, also from inside the editor |
| Tab | Move focus out of the editor. Tab does not indent. |
| Arrow Up / Arrow Down, Enter | Move through and choose an autocomplete suggestion |
| Escape | Close the autocomplete popup |

### Editing in place

| Shortcut or action | Result |
|--------------------|--------|
| Double-click the Description | Start editing |
| Ctrl/Cmd + Enter, or **Save** | Save, then refresh the page (on a note card, the list) |
| Click outside the editor | Save, then refresh the page (on a note card, the list). A click on a link starts the save and follows the link without waiting for it. |
| Tab past the **Save** and **Cancel** buttons | Save without refreshing. The saved text shows as plain text until the page is next loaded. |
| Escape | Cancel and discard changes. If an autocomplete popup is open, the first Escape closes it instead. |
| **Cancel** | Cancel and discard changes |

Moving focus to the toolbar, the Write and Preview buttons, the Save and Cancel buttons, or an autocomplete popup counts as staying in the editor. If the text is unchanged, saving just closes the editor. If a save fails, the editor stays open with your text and shows the error.

## Text blocks

In edit mode, each text block has the toolbar and autocomplete described above. Changes save automatically 500 ms after you stop typing, and immediately when focus leaves the block.

Outside edit mode, text blocks are rendered on the server, like a Description on a detail page, so shortcodes in them run. After you edit a block and click **Done**, the block shows its previous rendering, dimmed and inactive, until the new rendering arrives.

A block you have interacted with since leaving edit mode is not re-rendered in the background, so a draft in a widget inside it is not lost. A block that contains an embedded frame (an `<iframe>`) is never re-rendered in the background, because the page cannot see what you do inside the frame. Such a block shows its newer rendering when you edit it or reload the page.

The first text block also writes the Note's Description. See [Description Field Synchronization](./managing-notes.md#description-field-synchronization).
