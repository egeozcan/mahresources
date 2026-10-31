# Template Revisions for Categories, Resource Categories and Note Types

Goal: every save that changes a Template Carrier's templates records a Template
Revision. A user can list a carrier's revisions, diff any two (or one against the
current state), load a whole revision or one slot of it into the edit form, and
restore through the API or CLI.

Terms are in `CONTEXT.md` § Category Templates. The storage decision is
`docs/adr/0008-template-revisions-are-json-snapshots-in-one-table.md`.

## Decisions (settled in the grilling session, 2026-10-31)

1. A revision is a snapshot of the carrier's **whole tracked set**, not a per-slot history.
2. **Tracked fields**: every `Custom*` slot and its `*CSS` sibling, `CustomCSS`,
   `MetaSchema`, `SectionConfig`, `ApplyTemplatesToShares` (Note Type), and
   `AutoDetectRules` (Resource Category). **Not tracked**: `Name`, `Description`,
   `MetadataIndexes`.
3. **When a revision is recorded**: automatically on every save that changes a tracked field. A save that changes no tracked field records nothing. A revision
   carries an optional label of at most 200 characters that the user can set, change or clear later. The label is the only
   mutable part of a revision.
4. **Restore moves forward**: it records a new revision ("restored from #N") and never
   removes later ones.
5. **Retention**: keep every revision. No pruning.
6. **Metadata per revision**:
   - per-carrier number (#1, #2, ...) plus a global row ID for the API
   - timestamp
   - user (`CreatedByUserId`)
   - label
   - source: `create`, `form`, `api`, `cli`, `plugin`, `import`, `restore` or `baseline`
   - an **applied** list (see 11)
   - the names of the slots that changed compared with the previous revision, computed at write time
7. **Lazy baseline**: when a carrier with no revisions is saved, the pre-save state is
   first recorded as revision #1 with source `baseline`, then the new state as #2. There is no
   backfill migration.
8. **Create** records revision #1 with source `create`, even when every template is empty.
   Exempt creation paths record nothing and fall under the lazy baseline instead. They are seeds
   (`models/seed/initial_data.go`), the Category a remote download creates by name
   (`resource_upload_context.go:350`), and the default Resource Category's raw-SQL
   insert (`main.go:1215`). Each creates a carrier with empty or fixed templates, so
   no history is lost.

   Recording branches, in order:

   | Case | Records |
   |---|---|
   | create | #1 `create`, always |
   | restore | always records a revision, even if identical to current (it is an explicit act) |
   | save, carrier has no revisions, tracked fields unchanged | nothing |
   | save, carrier has no revisions, changed | #1 `baseline` (pre-save state) + #2 |
   | save, has revisions, unchanged | nothing |
   | save, has revisions, changed | next number |

   "No revisions" only ever means an exempt or pre-feature carrier, because every
   non-exempt create writes #1.
9. **Delete** removes the carrier's revisions in the same transaction.
10. **Diff**: any two revisions, or a revision against the current state (the default).
    The view is grouped by slot and hides unchanged slots. Each changed slot shows a line diff, unified by default with a side-by-side toggle.
    `MetaSchema`, `SectionConfig` and `AutoDetectRules` are diffed as canonical,
    pretty-printed JSON with sorted keys. The server computes the diff and the UI renders it.
11. **Attributing form saves**: presets, AI generation, "copy from existing" and
    "load revision" each append an entry to a hidden `revisionApplied` form field.
    The server keeps only entries it recognises (`preset:<id>`, `generated`,
    `copied:<carrierType>:<id>`, `loaded:<revisionNumber>[:<slot>]`) and records
    them with source `form`.
12. **Restore in the UI is "Load into editor"**: the whole revision, or one slot (whose CSS comes along by default and can be unticked),
    fills the edit form unsaved, and the user saves. A direct server-side restore
    endpoint exists for the API and CLI.
13. **Missing keys**: a snapshot with no key for a slot that was added later restores that slot as empty.
14. **Permissions**: viewing history and diffs, labelling, and restoring all need the carrier's
    edit permission. Today that is `requireTaxonomyRole` (admin) for Category and Resource Category, and
    `requireEditorRole` for Note Type.
15. **Group import**: a carrier the import creates gets #1 with source `import`. A
    reused carrier gets nothing. Export archives do not carry revisions.
16. **Audit log**: a restore writes a log entry whose Details hold `{fromRevision,
    toRevision}`. The existing log line for ordinary saves is unchanged.
17. **Surfaces**:
    - a History page per carrier, linked from its edit and display pages
    - REST: list, get, diff, label and restore, all registered in OpenAPI
    - CLI: `mr categories|resource-categories|note-types revisions list|show|diff|restore|label`
    - no plugin Lua API
18. **Out of scope**: a stale-form check for concurrent edits. Revisions already make an overwritten save
    recoverable. Possible follow-up.

## Write paths (surveyed 2026-10-31)

| Path | Funnel | Covered by |
|---|---|---|
| Edit form, `/v1/category`, `/v1/resourceCategory`, note type POST | handler pre-fill → shared function | the shared function |
| `mr ... edit` / `create` | same endpoints | the shared function |
| Plugin `Create*` / `Update*` / `Patch*` (`plugin_db_adapter.go`) | shared function | the shared function |
| Group import (`groupio/apply_import.go:515-760`) | direct `db.Create` | explicit `import` revision |
| Preset / generate / copy apply | client-side form fill only | the form save + `revisionApplied` |
| Seeds, download-created Category, default Resource Category | direct `db.Create` / `Save` / raw SQL | exempt; lazy baseline on first edit (decision 8) |
| Inline name / description edits (`basic_entity_context.go:48,69`) | untracked columns only | not a revision write |

The shared functions are `UpdateCategory` / `CreateCategory`
(`application_context/category_context.go`), `UpdateResourceCategory` /
`CreateResourceCategory` (`resource_category_context.go`) and
`CreateOrUpdateNoteType` (`note_context.go`). `CategoryCRUD()`'s writer is
registered only for the list route (`server/routes.go:804`), so it is not a write
path.

The source (`form` / `api` / `cli` / `plugin`) has to reach the shared function.
Carry it on the context, and stamp it at each ingress:

| Ingress | Source | How |
|---|---|---|
| HTTP | `form` / `api` / `cli` | survives `WithRequest` and `WithTransaction` (`context.go:1361-1387, 1557-1564`). `form` when the request accepts HTML (the same test `RedirectIfHTMLAccepted` uses); `cli` when the advisory `X-Mahresources-Client: mr` header is present (the `mr` client must start sending it, `cmd/mr/client/client.go`); otherwise `api` |
| Plugins | `plugin` | `boundContext` builds a fresh context through `WithPrincipal` (`plugin_db_adapter.go:56-70`), so it must set the source itself after binding, on both the actor and the actor-zero branches |
| Restore, import | `restore`, `import` | the caller sets it explicitly |

The source is advisory provenance, never an authorization input.

## Work items

### 1. Model and storage
- [x] `models.TemplateRevision`:
  - `ID`, `CreatedAt`, `CreatedByUserId`
  - `CarrierType` (`category` | `resourceCategory` | `noteType`), `CarrierID`, `Number`
  - `Snapshot types.JSON`
  - `Source`, `Applied types.JSON`, `ChangedSlots types.JSON`
  - `Label` (size 200)
  - `RestoredFrom *uint`
  - unique index on (`CarrierType`, `CarrierID`, `Number`)
- [x] Add to both `AutoMigrate` lists in `main.go` (562 and 1109).
- [x] One place that says which fields each carrier type tracks. Write a drift guard test that fails when
  a `Custom*` field is added to a carrier model but missing from the tracked set
  (follow the slot drift guards from the template slot expansion).
- [x] **Carrier lock protocol**, shared by save, restore and delete:
  1. Open one transaction. The shared functions read the carrier outside any transaction today
     (`category_context.go:171`, `resource_category_context.go:127`,
     `note_context.go:512`) and save on `ctx.db`, so each must be moved inside one.
  2. Lock the carrier row first (`SELECT ... FOR UPDATE` on Postgres; on SQLite the server
     driver's `BEGIN IMMEDIATE` already serialises writers, see
     `docs/architecture/sqlite-transactions.md`).
  3. Read the pre-save state under that lock. Diff it, assign `MAX(number)+1`, write the carrier
     and the revision, commit.

  Under the lock, numbering cannot collide. The unique index is a backstop that fails
  the whole transaction; there is no in-transaction retry, because a Postgres
  unique violation aborts the transaction.
- [x] Every write inside goes through the transaction's handle, never `ctx.db` or a
  logger built from the outer context (`sqlite-transactions.md` second bullet).

### 2. Recording
- [x] `recordTemplateRevision(tx, kind, carrierType, before, after, source, applied)`
  dispatches on `kind` **before** any no-op check, exactly as the table in decision 8 says:
  - `create` / `import`: write #1 unconditionally. There is no baseline and no no-op check. Empty templates
    are a valid create (`category_context.go:88-122`).
    `CreateOrUpdateNoteType` serves both, so it must pass `create` or `update` from
    whether the row existed.
  - `restore`: write the next number unconditionally, with `RestoredFrom`.
  - `update`: compare the tracked fields; if none changed, return. Otherwise, if the carrier
    has no revisions, write the `baseline` (pre-save state) first, then the new revision.
- [x] **Snapshots store raw values; comparison uses a canonical view.**
  - The snapshot keeps each field exactly as stored, malformed or not. Old MetaSchema can be
    invalid (`meta_schema.go:11-17`), and import copies MetaSchema and AutoDetectRules
    verbatim (`groupio/apply_import.go:571, 669, 762, 768`).
  - The canonical view used for change detection and diff is:
    - text slots as-is
    - `SectionConfig` through `Resolve{Group,Resource,Note}SectionConfig`
      (`models/section_config.go`) and re-marshalled, so NULL (which scans as `"null"`,
      `models/types/json.go:43-46`), `{}`, a sparse config and a full config that only
      restates defaults all compare equal
    - `MetaSchema` / `AutoDetectRules` pretty-printed with sorted keys when they parse, and
      the raw text when they do not
  - Canonicalisation never fails a save. Repairing an invalid MetaSchema must record
    baseline (invalid, raw) + new revision.
- [x] Call it from the three shared functions, inside their existing transaction
  (wrap a save in one if it has none).
- [x] Group import: write `import` revision #1 for each carrier it creates, with
  the carrier insert and its revision in one transaction per carrier. Schema-definition
  creation has no enclosing transaction today (`groupio/apply_import.go:86, 608-610,
  699-701`). If the revision write failed after the carrier committed, a retry would
  reuse the carrier (`:594-605, 696-698`) and never record it. Test a failure between
  the two inserts.
- [x] Carrier delete: delete its revisions in the delete transaction (all three
  delete functions).

### 3. Diff and restore
- [x] Server-side line diff. Check `go.mod` for an existing diff library before adding one.
- [x] `RestoreTemplateRevision(carrierType, carrierID, number, slot *string)` builds
  the target state (whole snapshot, or current state with one slot plus its CSS replaced)
  and saves it with source `restore`, under the carrier lock, with the same permission
  check and validation as the shared function. It must not go through the shared
  function's field mapping unchanged, because there an empty `SectionConfig` means
  "keep current" (`category_context.go:206`, `resource_category_context.go:179`,
  `note_context.go:551`), so a revision with no section config could never be
  restored. Factor the shared function into "map editor input to a target state" (with
  the keep-current rule) and "persist a target state" (exact), and have restore call
  only the second.
- [x] Restore still runs the ordinary save validation (MetaSchema,
  `meta_schema.go:31-50`; Resource Category detection rules,
  `resource_category_context.go:135-140`). It never bypasses it. A revision whose raw content no longer
  validates (old malformed MetaSchema, for example) is refused with an error naming the
  field, and records nothing. The user repairs it by loading the revision into the editor,
  fixing the field and saving. Test that a refused restore writes no revision and
  changes no field.
- [x] **Plugin hook boundaries.** Run before-hooks before the carrier transaction
  opens, and after-hooks only after it commits. `WithTransaction` only swaps the db
  handle (`context.go:1557-1564`), and ordinary hook calls have no transaction binding
  (`:1193-1205`), so a hook that writes while the carrier transaction holds SQLite's
  writer lock would block on it. Keep the existing deferral inside a plugin
  transaction (`deferredHooks`, `:1257-1269`) unchanged.
- [x] Restore audit log entry with `{fromRevision, toRevision}`.

### 4. REST + OpenAPI
- [x] `GET .../revisions` (list), `GET .../revisions/{n}`, `GET .../revisions/diff?from=&to=`
  (`to` defaults to current), `POST .../revisions/{n}/label`, `POST .../revisions/{n}/restore`
  (optional `slot`), for each of the three carrier types. Match the existing route
  naming (`/v1/category/...`) and register every route in OpenAPI.
- [x] Permission check on every route, per decision 14, **including the GETs**. The
  path gates give taxonomy and editor GETs only `capRead` (`server/authz_policy.go:87-103`), and
  edit pages are deliberately readable by lower roles (`:59-62`). So prefix coverage
  does not protect revision list, get or diff, or the History page. Give them explicit
  edit-capability gates in the policy and in the handler.

### 5. UI
- [ ] History page per carrier: revision list (decision 6 columns), choose two to diff
  (default: selected vs current), the slot-grouped diff, label editing, and "Load into
  editor" for the whole revision or for one slot.
- [ ] Link it from the edit and display pages of all three carrier types.
- [ ] Load into editor: open the edit form pre-filled with the target state, unsaved,
  and append `loaded:<n>[:<slot>]` to `revisionApplied`. Build it on
  `src/components/templateBundle.js`'s editor setters, but not on `applyBundle`
  as it stands. That function loads only slots, MetaSchema and SectionConfig
  (`:314-339`), skips an empty SectionConfig and merges keys (`:191-205`). A revision
  load must:
  - set every tracked field, including `ApplyTemplatesToShares` and `AutoDetectRules`
  - set SectionConfig from the **server-resolved** view, which the revision endpoint returns
    (`Resolve*SectionConfig`, re-marshalled), and never from the raw snapshot. The JS merge
    (`src/components/sectionConfigForm.js:24-33`) does not behave like the Go resolvers:
    - `{"ownEntities":[],"tags":false}` resolves to all defaults in Go
      (`models/section_config.go:183-185`), but JS keeps `tags=false`.
    - `{"ownEntities":null}` survives the JS merge and breaks `config.ownEntities.state`
      (`templates/partials/sectionConfigForm.tpl:61`).

    Replace the whole control state with the resolved view; do not merge it into the current config.

  The hidden input then posts that resolved config. It renders exactly what the
  snapshot rendered, because it is the resolver's own output, and it compares equal
  under the canonical view. So no "clear" marker is needed, and later checkbox edits
  are ordinary edits.
- [ ] Presets, generation and copy append their `revisionApplied` entries.

### 6. CLI
- [x] `revisions list|show|diff|restore|label` under `categories`, `resource-categories`
  and `note-types`, plus their help files.

### 7. Docs
- [x] docs-site page for template revisions (under `docs-site/docs/features/`, next to
  `custom-templates.md`) and a link from `custom-templates.md`.
- [x] CLI reference pages for the new subcommands.
- [x] Link from the in-app category-template docs to the History page.

## Verification

Go unit tests (SQLite and Postgres):
- a no-op save writes nothing
- a first save on an old carrier writes baseline + new
- create writes #1
- each write path (form handler, plugin `Patch*`, import) records with the right source
- a reused carrier on import writes nothing
- delete removes revisions
- concurrent saves get distinct numbers
- restoring a snapshot missing a slot clears it
- one-slot restore leaves the other slots current
- JSON fields diff without key-order noise
- the drift guard
- explicit role assertions, not "same as the edit routes":
  - admin, editor, unscoped user, scoped user and guest (`models/role.go:14-17`), each against
    Category, Resource Category and Note Type
  - for list, get, diff, label, restore and the History page
  - including the HTML, `.json` and `.body` variants (`server/routes.go:538-545`)
- a lock-protocol test: two concurrent changing saves on Postgres produce two revisions
  whose snapshots chain correctly (#n+1's baseline is #n's content)
- seed, download-created and default carriers get a `baseline` on first edit
- recording counts and sources:
  - empty create → exactly one `create`
  - non-empty create → exactly one `create`
  - identical restore → one `restore`
  - Note Type create vs update through `CreateOrUpdateNoteType`
- SectionConfig NULL, `{}`, sparse and defaults-only all compare equal; a revision with
  no section config, loaded into the editor and saved, renders the same as the snapshot
- section config `{"ownEntities":[],"tags":false}` and `{"ownEntities":null}` load
  into the editor as the resolver's output and save without changing what renders
- invalid MetaSchema before, valid after: the save succeeds, records baseline + new, and the diff
  falls back to text
- an import failure between carrier and revision inserts leaves neither

E2E (Playwright):
- edit a category twice, open History, see three revisions with changed slot names
- diff two revisions
- load one slot into the editor, save, see a `form` revision with `loaded:` applied
- apply a preset and save, see `preset:` in applied
- same for one Resource Category and one Note Type

CLI doctests for each new subcommand.

Run the full suite (Go unit, E2E browser, CLI) on SQLite and Postgres before calling this done.

## API contract (lane 1)

All routes take the carrier's ID as the query parameter `id` and answer JSON. Every one needs the
carrier's edit capability, GETs included: admin for Category and Resource Category, editor for
Note Type (403 otherwise, with `{"error": "insufficient permissions"}`).

| Carrier | `carrierType` | API prefix | History page (reserved, UI lane adds the template) |
|---|---|---|---|
| Category | `category` | `/v1/category` | `/category/revisions?id=` |
| Resource Category | `resourceCategory` | `/v1/resourceCategory` | `/resourceCategory/revisions?id=` |
| Note Type | `noteType` | `/v1/noteType` | `/noteType/revisions?id=` |

The `.json` and `.body` variants of a History page path are gated the same way. The table lives in
`server/template_revision_carriers.go`.

| Method and path (under the prefix) | Purpose | Body or query |
|---|---|---|
| `GET /revisions?id=` | list, newest first | none |
| `GET /revisions/{number}?id=` | one revision with content | none |
| `GET /revisions/diff?id=&from=&to=` | slot-grouped line diff; `to` omitted means the current state | `from` required |
| `POST /revisions/{number}/label?id=` | set, change or clear the label (at most 200 characters) | `{"label": "..."}` |
| `POST /revisions/{number}/restore?id=` | restore the whole revision, or one slot | `{"slot": "CustomHeader", "includeCss": true}`, both optional |

Errors: 403 role (declared in OpenAPI on every route), 400 invalid (unknown slot, overlong label, a revision that no longer validates, missing `id` or
`from`), 403 role, 404 unknown carrier or revision number.

Saving still goes through the existing endpoints. The edit form (and `mr`) post `revisionApplied`
alongside the other fields: repeated values or one comma-separated value, for example
`preset:dashboard`, `generated`, `copied:category:12`, `loaded:3`, `loaded:3:CustomHeader`. Anything
else is dropped. A save is recorded with source `form` only when the request's `Accept` header contains
`text/html`, which a normal browser form submit sends; a `fetch` that sends `Accept: application/json`
is recorded as `api` unless it also sends `X-Mahresources-Client: mr`.

### Summary (list items, label and restore responses)

```json
{
  "id": 41, "number": 3, "carrierType": "category", "carrierId": 7,
  "createdAt": "2026-10-31T12:00:00Z", "createdByUserId": 2,
  "label": "before redesign",
  "source": "form",
  "applied": ["preset:dashboard", "loaded:1:CustomHeader"],
  "changedSlots": ["CustomHeader", "CustomSidebar"],
  "restoredFrom": 1
}
```

`source` is one of `create`, `form`, `api`, `cli`, `plugin`, `import`, `restore`, `baseline`.
`restoredFrom` (only for `restore`) is the revision **number** restored. `changedSlots` are slot names
(a slot and its `*CSS` sibling count as one), computed against the state before the save; for `#1` they
are the slots that have content. A `baseline` revision's `createdAt` and `createdByUserId` are those of
the save that triggered it, not of the original author.

### Detail (`GET /revisions/{number}`)

The summary fields plus:

- `snapshot`: the raw stored value of every tracked field the revision recorded, keyed by Go field name
  (strings, and a bool for `ApplyTemplatesToShares`). `SectionConfig` is the raw text. A key the revision
  lacks is a slot added later.
- `editor`: the value of every tracked field as the edit form must load it. Missing keys are empty
  strings or false, and `SectionConfig` is the Go resolver's output as a JSON string. **Load the editor
  from `editor`, never from `snapshot`**, and replace the section config control state wholesale.

### Diff (`GET /revisions/diff`)

```json
{
  "carrierType": "category", "carrierId": 7, "from": 1, "to": 3,
  "slots": [
    { "slot": "CustomHeader",
      "fields": [
        { "field": "CustomHeader",
          "hunks": [
            { "op": "equal",  "lines": ["<div>"] },
            { "op": "delete", "lines": ["old"] },
            { "op": "insert", "lines": ["new"] } ] },
        { "field": "CustomHeaderCSS", "hunks": [ ... ] } ] } ]
}
```

`to` is absent when the right side is the current state. Unchanged slots and fields are left out. `op` is
`equal`, `insert` or `delete`; consecutive lines with one op share a hunk. `MetaSchema`,
`SectionConfig` and `AutoDetectRules` are diffed as sorted-key pretty-printed JSON, or as raw text when
they do not parse. The valid slot names per carrier type are `models.TemplateSlots(carrierType)`; slot
names for `slot` in a restore are the primary field names (`CustomHeader`, not `CustomHeaderCSS`).

## Deviations (lane 1)

- **AutoMigrate lists.** `main.go` has one entity list (line 562); the one at 1109 migrates the Job
  tables. `TemplateRevision` is in the entity list, in `server/api_tests` (SQLite and Postgres harnesses)
  and in the 45 test files that carry their own list, because every carrier create, update and delete
  now touches the table.
- **Snapshot, applied and changed-slots columns are plain `string` fields holding JSON, migrated as
  `text`.** `types.JSON.GormDBDataType` returns JSONB on Postgres whatever the `gorm:"type:text"` tag says,
  and JSONB reorders keys and renormalises numbers; the snapshot should keep the exact bytes.
  `TestRevision_SnapshotColumnsMigrateAsTextOnPostgres` asserts the column type.
- **`restoredFrom` stores the revision number**, not the global row ID, since the UI shows "restored
  from #N".
- **Note Type revision routes sit under `/v1/noteType/revisions`.** `/v1/note/noteType` is the
  create/edit/delete prefix; both are editor paths, so the gate is the same.
- **Restore fires the Category update hooks** (`before_category_update`, `after_category_update`). The
  before-hook can veto; edits it makes to name or description are ignored, since restore touches neither.
- **`DeleteNoteType` is now one transaction.** It used to issue the note update and the delete as two
  statements; the carrier lock and the revision delete needed an enclosing transaction.
- **`mr` always installs its transport**, so `X-Mahresources-Client: mr` is sent even with no token.
- **The arch gate test is unchanged.** The revision operations carry neither "Categor" nor "NoteType"
  in their names, so it never matches them; `TestRevision_ReadsAndWritesNeedTheCarriersEditCapability`
  and `TestTemplateRevisionRoleMatrix` pin the guards instead.

## Review

(to fill in after implementation)

## Deviations (lane 3)

- **Command group.** The `revisions` group hangs off the plural commands (`mr categories|resource-categories|note-types revisions ...`), as decision 17 says, not the singular `category` commands where `edit` lives. The carrier ID is `--id` on every leaf, matching `edit`; the revision number is positional.
- **Label argument.** `label <number> [text]` takes the text as an optional second argument and requires exactly one of text and `--clear`.
- **`--no-css` without `--slot`** is refused client-side, since the server only reads `includeCss` with a slot.
- **`diff` text output** renders the server's full-file hunks as a unified diff with three lines of context and `@@` headers, per field. `--json` prints the server response unchanged.
- **`show` text output** prints the metadata, then each non-empty stored field under a `--- Field ---` header. `--json` prints the response, including `editor`.
- **Role denial is not in the CLI E2E.** The `cli` Playwright project runs an auth-disabled server, so a non-admin token cannot be exercised there. Role assertions stay in the Go API tests of lane 1.
- **In-app docs.** The History link in the Reference panels was dropped: lane 2 puts a "View template history" link atop each edit form and a "History" action on display pages. The docs-site link from `custom-templates.md` stays.
- **docs-site page text for the History page** (columns, compare, label, Load into editor) follows decisions 10 and 12 and the lane 2 template; recheck it against the final UI.
