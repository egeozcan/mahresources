---
sidebar_position: 4
title: Template Revisions
---

# Template Revisions

A **Template Revision** is one saved state of a Category, Resource Category or Note Type: all of its template slots and their CSS, plus its Meta JSON Schema and section configuration. Every save that changes any of those records a new revision, so you can list a carrier's history, compare any two states, and bring an old state back.

Categories, Resource Categories and Note Types are called Template Carriers here. A revision's content never changes after it is recorded. Only its label can be edited.

## What a revision holds

A revision stores the whole tracked set of its carrier at that moment, not one slot.

Every carrier tracks these fields: `CustomHeader`, `CustomDetailFooter`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`, `CustomHoverCard`, `CustomListHeader`, `CustomListFooter`, `CustomMRQLResult` and `CustomEntityPickerResult`, each with its `*CSS` sibling, plus `CustomCSS`, `MetaSchema` and `SectionConfig`. Each carrier adds its own:

| Carrier | Also tracked |
|---------|--------------|
| Category | `CustomOwnEntities` with its CSS |
| Resource Category | `CustomPreview`, `CustomLightbox` and `CustomCell`, each with its CSS, and `AutoDetectRules` |
| Note Type | `ApplyTemplatesToShares` |

Not tracked: `Name`, `Description` and `MetadataIndexes`. Renaming a carrier or editing its description records nothing. Slot names are listed in [Custom Templates](./custom-templates.md#template-locations).

Each revision also carries:

- a number, counted per carrier (#1, #2, ...), and a global row ID that the API returns as `id`
- a timestamp and the user who made the change
- an optional label of at most 200 characters
- a **source**, described below
- an **applied** list naming a preset, a generated draft, a copied template or a loaded revision, when the save used one
- the slots that changed compared with the previous revision. A slot and its `*CSS` sibling count as one slot. For revision #1 these are the slots that have content.

## When a revision is recorded

| Event | Result |
|-------|--------|
| A carrier is created | Revision #1, source `create`, even when every template is empty |
| A save changes a tracked field | The next number |
| A save changes no tracked field | Nothing |
| A restore | The next number, source `restore`, even when the content equals the current state |
| A group import creates the carrier | Revision #1, source `import` |
| A group import reuses an existing carrier | Nothing |
| A carrier that has no revisions is saved with a change | Revision #1 holding the state before the save (source `baseline`), then the new revision |

A save changes a tracked field when the canonical form of that field differs. Text slots compare as written. `SectionConfig` is compared after the server resolves it, so an empty value, `{}`, a sparse config and a full config that only restates the defaults are the same. `MetaSchema` and `AutoDetectRules` compare as pretty-printed JSON with sorted keys, so reordering keys is not a change. When the text does not parse as JSON, the raw text is compared.

A `baseline` revision carries the timestamp and user of the save that triggered it, not those of the carrier's original author.

### Carriers created without a revision

Three creation paths write no revision. Each makes a carrier with empty or fixed templates, so nothing is lost, and the first edit records a `baseline` and then the new revision:

- the seeded Categories, Resource Categories and Note Types
- the Category that a remote download creates by name
- the default Resource Category

### Sources

The source records where a save came from. It is provenance for reading the history. It is never used to decide what a caller may do.

| Source | Meaning |
|--------|---------|
| `create` | The carrier was created |
| `form` | The request accepts `text/html`, which a browser form submit does |
| `cli` | The `mr` CLI sent the change, identified by the `X-Mahresources-Client: mr` header it adds to every request |
| `api` | Any other API client |
| `plugin` | A plugin changed the carrier through `mah.db` |
| `import` | A group import created the carrier |
| `restore` | A restore recorded it |
| `baseline` | The state before the first recorded change of an older carrier |

Because `form` is decided by the `Accept` header, a script that posts with `Accept: text/html` is recorded as `form`.

### Applied entries

The edit form sends a hidden `revisionApplied` field so a revision can say how its content was produced. The server keeps only these forms and drops everything else:

| Entry | Meaning |
|-------|---------|
| `preset:<id>` | A preset was applied |
| `generated` | AI generation filled a slot |
| `copied:<carrierType>:<id>` | A template was copied from another carrier |
| `loaded:<number>` | A whole revision was loaded into the editor |
| `loaded:<number>:<slot>` | One slot of a revision was loaded |

## History page

Each carrier has a History page, linked from its edit and display pages:

| Carrier | Page |
|---------|------|
| Category | `/category/revisions?id=<id>` |
| Resource Category | `/resourceCategory/revisions?id=<id>` |
| Note Type | `/noteType/revisions?id=<id>` |

The page lists the revisions with their number, source, changed slots, user, time and label. From it you can compare revisions, edit a label, and load a revision into the editor.

## Comparing revisions

A diff compares two revisions, or one revision against the carrier's current state, which is the default. It is grouped by slot and shows only the slots and fields that changed. Each changed field is a line diff, shown unified by default with a side-by-side option. The server computes the diff.

`MetaSchema`, `SectionConfig` and `AutoDetectRules` are diffed as pretty-printed JSON with sorted keys. A value that does not parse is diffed as raw text.

The CLI prints the same diff as a unified diff with three lines of context:

```bash
mr categories revisions diff --id 7 --from 2
mr categories revisions diff --id 7 --from 2 --to 5
```

## Restoring

There are two ways to bring an old state back.

**Load into editor** is the way the UI restores. It fills the edit form with the whole revision, or with one slot, without saving. The slot's CSS comes along by default and can be unticked. You review the form and save it like any edit, and the saved revision lists `loaded:<number>` or `loaded:<number>:<slot>` in its applied entries. Loading a revision replaces the form's section configuration with the server-resolved view, not a merge with what the form held.

**Restore** through the API or CLI saves directly:

```bash
# The whole of revision 3
mr categories revisions restore 3 --id 7

# Only CustomHeader, keeping the current header CSS
mr categories revisions restore 3 --id 7 --slot CustomHeader --no-css
```

A restore:

- records a new revision with source `restore` and never removes later revisions
- replaces only the named slot when `slot` is given, using the primary field name such as `CustomHeader`. The CSS sibling is replaced with it unless `includeCss` is `false`.
- sets a slot that the old revision did not have, because it was added to the product later, to empty
- runs the normal save validation. A revision whose content no longer validates, for example an old malformed `MetaSchema`, is refused with an error naming the field, and nothing is changed or recorded. To repair it, load the revision into the editor, fix the field and save.
- writes an [activity log](./activity-log.md) entry whose details hold `fromRevision` and `toRevision`

## Labels

A label is free text of up to 200 characters, for marking a state such as "before redesign". It can be set, changed or cleared at any time and is the only mutable part of a revision. Setting an empty label clears it.

```bash
mr categories revisions label 3 "before redesign" --id 7
mr categories revisions label 3 --clear --id 7
```

## Permissions

Viewing history and diffs, labelling and restoring all need the permission to edit the carrier itself, because a revision can contain content that its author later removed on purpose:

| Carrier | Required role |
|---------|---------------|
| Category | admin |
| Resource Category | admin |
| Note Type | editor or admin |

The same applies to the History page and to its `.json` and `.body` variants. A caller without the role gets `403`. When [authentication](./authentication.md) is off, every request has full access. See [the four roles](./authentication.md#the-four-roles).

## Retention and deletion

Every revision is kept. There is no pruning. Deleting a carrier deletes its revisions in the same transaction.

## Import and export

Group export archives do not carry revisions. On import, a carrier that the import creates starts with revision #1 from source `import`. A carrier the import reuses is left alone and gets no revision.

## Plugins

Plugins have no API for reading or restoring revisions. A save a plugin makes through `mah.db` is recorded like any other, with source `plugin`.

## API

All routes answer JSON, take the carrier ID as the `id` query parameter, and are registered in the OpenAPI spec. They sit under `/v1/category`, `/v1/resourceCategory` and `/v1/noteType`.

| Request | Purpose |
|---------|---------|
| `GET <prefix>/revisions?id=` | List, newest first |
| `GET <prefix>/revisions/{number}?id=` | One revision with its `snapshot` (the stored values) and `editor` (the values as the edit form loads them) |
| `GET <prefix>/revisions/diff?id=&from=&to=` | Slot-grouped line diff. `to` omitted means the current state |
| `POST <prefix>/revisions/{number}/label?id=` | Body `{"label": "..."}` |
| `POST <prefix>/revisions/{number}/restore?id=` | Optional body `{"slot": "CustomHeader", "includeCss": true}` |

Errors are `400` for an unknown slot, an overlong label, a revision that no longer validates or a missing parameter, `403` for the role, and `404` for an unknown carrier or revision number.

## CLI

The same operations are available as `revisions` subcommands of `mr categories`, `mr resource-categories` and `mr note-types`. Each takes the carrier ID with `--id`.

| Command | Reference |
|---------|-----------|
| `list` | [categories](../cli/categories/revisions/list.md), [resource-categories](../cli/resource-categories/revisions/list.md), [note-types](../cli/note-types/revisions/list.md) |
| `show <number>` | [categories](../cli/categories/revisions/show.md), [resource-categories](../cli/resource-categories/revisions/show.md), [note-types](../cli/note-types/revisions/show.md) |
| `diff --from N [--to M]` | [categories](../cli/categories/revisions/diff.md), [resource-categories](../cli/resource-categories/revisions/diff.md), [note-types](../cli/note-types/revisions/diff.md) |
| `restore <number> [--slot X] [--no-css]` | [categories](../cli/categories/revisions/restore.md), [resource-categories](../cli/resource-categories/revisions/restore.md), [note-types](../cli/note-types/revisions/restore.md) |
| `label <number> <text>` or `--clear` | [categories](../cli/categories/revisions/label.md), [resource-categories](../cli/resource-categories/revisions/label.md), [note-types](../cli/note-types/revisions/label.md) |
