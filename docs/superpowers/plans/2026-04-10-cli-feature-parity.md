# CLI Feature Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the `mr` CLI tool up to date with all recently added server features (edit-meta, new category/note-type fields, relation-type/series edit-name, MRQL render flag) and ensure all new commands have clear help text.

**Architecture:** Each gap is a small addition following existing CLI patterns: Cobra subcommands with `PostForm` for entity editors, `Post` for JSON bodies, and `--flag` for new fields. Each task adds the CLI command, updates the E2E test file, and verifies.

**Tech Stack:** Go (Cobra CLI), Playwright (E2E tests), existing `client.Client` HTTP methods.

---

### Task 1: Add `edit-meta` subcommand to notes, groups, and resources

The API exposes `POST /v1/{entity}/editMeta` which accepts form fields `id` (query), `path`, and `value` for deep-merge editing of a single meta field.

**Files:**
- Modify: `cmd/mr/commands/notes.go:29-43` (add subcommand to NewNoteCmd)
- Modify: `cmd/mr/commands/groups.go:37-53` (add subcommand to NewGroupCmd)
- Modify: `cmd/mr/commands/resources.go:77-104` (add subcommand to NewResourceCmd)
- Test: `e2e/tests/cli/cli-notes.spec.ts`
- Test: `e2e/tests/cli/cli-groups.spec.ts`
- Test: `e2e/tests/cli/cli-resources.spec.ts`

- [ ] **Step 1: Add `edit-meta` subcommand to notes.go**

Add to `NewNoteCmd` after line 40 (`newNoteEditDescriptionCmd`):

```go
cmd.AddCommand(newNoteEditMetaCmd(c, opts))
```

Add the function after `newNoteEditDescriptionCmd`:

```go
func newNoteEditMetaCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-meta <id> <path> <value>",
		Short: "Edit a single metadata field by JSON path",
		Long: `Edit a single metadata field using deep-merge-by-path.

The path is a dot-separated JSON path (e.g., "address.city") and the value
is a JSON literal (e.g., '"Berlin"', '42', '{"nested":"obj"}').

Examples:
  mr note edit-meta 5 status '"active"'
  mr note edit-meta 5 address.city '"Berlin"'
  mr note edit-meta 5 scores '[1,2,3]'`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("path", args[1])
			form.Set("value", args[2])

			var raw json.RawMessage
			if err := c.PostForm("/v1/note/editMeta", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note metadata updated successfully.")
			}
			return nil
		},
	}
}
```

- [ ] **Step 2: Add `edit-meta` subcommand to groups.go**

Add to `NewGroupCmd` after line 49 (`newGroupEditDescriptionCmd`):

```go
cmd.AddCommand(newGroupEditMetaCmd(c, opts))
```

Add the function after `newGroupEditDescriptionCmd`:

```go
func newGroupEditMetaCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-meta <id> <path> <value>",
		Short: "Edit a single metadata field by JSON path",
		Long: `Edit a single metadata field using deep-merge-by-path.

The path is a dot-separated JSON path (e.g., "address.city") and the value
is a JSON literal (e.g., '"Berlin"', '42', '{"nested":"obj"}').

Examples:
  mr group edit-meta 5 status '"active"'
  mr group edit-meta 5 address.city '"Berlin"'
  mr group edit-meta 5 scores '[1,2,3]'`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("path", args[1])
			form.Set("value", args[2])

			var raw json.RawMessage
			if err := c.PostForm("/v1/group/editMeta", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Group metadata updated successfully.")
			}
			return nil
		},
	}
}
```

- [ ] **Step 3: Add `edit-meta` subcommand to resources.go**

Add to `NewResourceCmd` after line 88 (`newResourceEditDescriptionCmd`):

```go
cmd.AddCommand(newResourceEditMetaCmd(c, opts))
```

Add the function after `newResourceEditDescriptionCmd`:

```go
func newResourceEditMetaCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-meta <id> <path> <value>",
		Short: "Edit a single metadata field by JSON path",
		Long: `Edit a single metadata field using deep-merge-by-path.

The path is a dot-separated JSON path (e.g., "address.city") and the value
is a JSON literal (e.g., '"Berlin"', '42', '{"nested":"obj"}').

Examples:
  mr resource edit-meta 5 status '"active"'
  mr resource edit-meta 5 address.city '"Berlin"'
  mr resource edit-meta 5 scores '[1,2,3]'`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("path", args[1])
			form.Set("value", args[2])

			var raw json.RawMessage
			if err := c.PostForm("/v1/resource/editMeta", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resource metadata updated successfully.")
			}
			return nil
		},
	}
}
```

- [ ] **Step 4: Add E2E tests for edit-meta commands**

Add to `e2e/tests/cli/cli-notes.spec.ts` a new test block:

```typescript
test.describe('Note edit-meta', () => {
  const suffix = Date.now();
  let noteId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const note = cli.runJson<Note>('note', 'create', '--name', `meta-note-${suffix}`, '--meta', '{"existing":"value"}');
    noteId = note.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note', 'delete', String(noteId));
  });

  test('edit-meta sets a new field', async ({ cli }) => {
    cli.runOrFail('note', 'edit-meta', String(noteId), 'newField', '"hello"');

    const note = cli.runJson<any>('note', 'get', String(noteId));
    const meta = typeof note.Meta === 'string' ? JSON.parse(note.Meta) : note.Meta;
    expect(meta.newField).toBe('hello');
    expect(meta.existing).toBe('value');
  });
});
```

Add to `e2e/tests/cli/cli-groups.spec.ts` a new test block:

```typescript
test.describe('Group edit-meta', () => {
  const suffix = Date.now();
  let groupId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const group = cli.runJson<Group>('group', 'create', '--name', `meta-group-${suffix}`, '--meta', '{"existing":"value"}');
    groupId = group.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('group', 'delete', String(groupId));
  });

  test('edit-meta sets a new field', async ({ cli }) => {
    cli.runOrFail('group', 'edit-meta', String(groupId), 'newField', '"hello"');

    const group = cli.runJson<any>('group', 'get', String(groupId));
    const meta = typeof group.Meta === 'string' ? JSON.parse(group.Meta) : group.Meta;
    expect(meta.newField).toBe('hello');
    expect(meta.existing).toBe('value');
  });
});
```

Add a similar block to `e2e/tests/cli/cli-resources.spec.ts` (resource must be uploaded first via `from-url` or `upload`, so use whatever pattern that file already uses for creating resources with meta).

- [ ] **Step 5: Build and verify**

Run: `go build --tags 'json1 fts5' ./cmd/mr/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add cmd/mr/commands/notes.go cmd/mr/commands/groups.go cmd/mr/commands/resources.go e2e/tests/cli/cli-notes.spec.ts e2e/tests/cli/cli-groups.spec.ts e2e/tests/cli/cli-resources.spec.ts
git commit -m "feat(cli): add edit-meta subcommand for notes, groups, and resources"
```

---

### Task 2: Add `edit-name` and `edit-description` subcommands to relation-type

The API exposes `POST /v1/relationType/editName` and `POST /v1/relationType/editDescription` using the same generic edit-entity-name/description handlers as other entities.

**Files:**
- Modify: `cmd/mr/commands/relation_types.go:28-39` (add subcommands to NewRelationTypeCmd)
- Test: `e2e/tests/cli/cli-relation-types.spec.ts`

- [ ] **Step 1: Add edit-name and edit-description subcommands**

Add to `NewRelationTypeCmd` after line 36 (`newRelationTypeDeleteCmd`):

```go
cmd.AddCommand(newRelationTypeEditNameCmd(c, opts))
cmd.AddCommand(newRelationTypeEditDescriptionCmd(c, opts))
```

Add the functions:

```go
func newRelationTypeEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-name <id> <new-name>",
		Short: "Edit a relation type's name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Name", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/relationType/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Relation type name updated successfully.")
			}
			return nil
		},
	}
}

func newRelationTypeEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-description <id> <new-description>",
		Short: "Edit a relation type's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Description", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/relationType/editDescription", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Relation type description updated successfully.")
			}
			return nil
		},
	}
}
```

- [ ] **Step 2: Add E2E tests**

Add to `e2e/tests/cli/cli-relation-types.spec.ts`:

```typescript
test.describe('Relation type edit-name / edit-description', () => {
  const suffix = Date.now();
  let rtId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const rt = cli.runJson<RelationType>('relation-type', 'create', '--name', `rt-${suffix}`, '--description', `desc-${suffix}`);
    rtId = rt.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('relation-type', 'delete', String(rtId));
  });

  test('edit-name updates the relation type name', async ({ cli }) => {
    const newName = `rt-${suffix}-renamed`;
    cli.runOrFail('relation-type', 'edit-name', String(rtId), newName);

    const list = cli.runJson<RelationType[]>('relation-types', 'list');
    const match = list.find(rt => rt.ID === rtId);
    expect(match).toBeDefined();
    expect(match!.Name).toBe(newName);
  });

  test('edit-description updates the relation type description', async ({ cli }) => {
    const newDesc = `desc-${suffix}-updated`;
    cli.runOrFail('relation-type', 'edit-description', String(rtId), newDesc);

    const list = cli.runJson<RelationType[]>('relation-types', 'list');
    const match = list.find(rt => rt.ID === rtId);
    expect(match).toBeDefined();
    expect(match!.Description).toBe(newDesc);
  });
});
```

- [ ] **Step 3: Build and verify**

Run: `go build --tags 'json1 fts5' ./cmd/mr/`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/commands/relation_types.go e2e/tests/cli/cli-relation-types.spec.ts
git commit -m "feat(cli): add edit-name and edit-description to relation-type command"
```

---

### Task 3: Add `edit-name` subcommand to series

The API exposes `POST /v1/series/editName` using the generic edit-entity-name handler.

**Files:**
- Modify: `cmd/mr/commands/series.go:27-41` (add subcommand to NewSeriesCmd)
- Test: `e2e/tests/cli/cli-series.spec.ts`

- [ ] **Step 1: Add edit-name subcommand**

Add to `NewSeriesCmd` after line 37 (`newSeriesDeleteCmd`):

```go
seriesCmd.AddCommand(newSeriesEditNameCmd(c, opts))
```

Add the function:

```go
func newSeriesEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-name <id> <new-name>",
		Short: "Edit a series name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Name", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/series/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Series name updated successfully.")
			}
			return nil
		},
	}
}
```

- [ ] **Step 2: Add E2E test**

Add to `e2e/tests/cli/cli-series.spec.ts` (in the existing CRUD lifecycle block, or as a new block):

```typescript
test.describe('Series edit-name subcommand', () => {
  const suffix = Date.now();
  let seriesId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const series = cli.runJson<Series>('series', 'create', '--name', `editname-series-${suffix}`);
    seriesId = series.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('series', 'delete', String(seriesId));
  });

  test('edit-name updates the series name', async ({ cli }) => {
    const newName = `editname-series-${suffix}-renamed`;
    cli.runOrFail('series', 'edit-name', String(seriesId), newName);

    const series = cli.runJson<Series>('series', 'get', String(seriesId));
    expect(series.Name).toBe(newName);
  });
});
```

- [ ] **Step 3: Build and verify**

Run: `go build --tags 'json1 fts5' ./cmd/mr/`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/commands/series.go e2e/tests/cli/cli-series.spec.ts
git commit -m "feat(cli): add edit-name subcommand to series command"
```

---

### Task 4: Add MetaSchema, SectionConfig, and CustomMRQLResult flags to note-type create/edit

The NoteType model now has three additional text fields. The create and edit commands need flags for them.

**Files:**
- Modify: `cmd/mr/commands/note_types.go:83-135` (create) and `cmd/mr/commands/note_types.go:137-189` (edit)
- Test: `e2e/tests/cli/cli-note-types.spec.ts`

- [ ] **Step 1: Add flags to `newNoteTypeCreateCmd`**

Change the var declaration at line 84:

```go
var name, description, customHeader, customSidebar, customSummary, customAvatar, metaSchema, sectionConfig, customMRQLResult string
```

Add to the body map (after `CustomAvatar` block):

```go
if metaSchema != "" {
    body["MetaSchema"] = metaSchema
}
if sectionConfig != "" {
    body["SectionConfig"] = sectionConfig
}
if customMRQLResult != "" {
    body["CustomMRQLResult"] = customMRQLResult
}
```

Add flag registrations after the `custom-avatar` flag:

```go
cmd.Flags().StringVar(&metaSchema, "meta-schema", "", "JSON Schema defining the metadata structure for notes of this type")
cmd.Flags().StringVar(&sectionConfig, "section-config", "", "JSON controlling which sections are visible on note detail pages")
cmd.Flags().StringVar(&customMRQLResult, "custom-mrql-result", "", "Pongo2 template for rendering notes of this type in MRQL results")
```

- [ ] **Step 2: Add flags to `newNoteTypeEditCmd`**

Change the var declaration at line 139:

```go
var name, description, customHeader, customSidebar, customSummary, customAvatar, metaSchema, sectionConfig, customMRQLResult string
```

Add to the body map (after `CustomAvatar` block):

```go
if cmd.Flags().Changed("meta-schema") {
    body["MetaSchema"] = metaSchema
}
if cmd.Flags().Changed("section-config") {
    body["SectionConfig"] = sectionConfig
}
if cmd.Flags().Changed("custom-mrql-result") {
    body["CustomMRQLResult"] = customMRQLResult
}
```

Add flag registrations after the `custom-avatar` flag:

```go
cmd.Flags().StringVar(&metaSchema, "meta-schema", "", "JSON Schema defining the metadata structure for notes of this type")
cmd.Flags().StringVar(&sectionConfig, "section-config", "", "JSON controlling which sections are visible on note detail pages")
cmd.Flags().StringVar(&customMRQLResult, "custom-mrql-result", "", "Pongo2 template for rendering notes of this type in MRQL results")
```

- [ ] **Step 3: Add E2E test**

Add to `e2e/tests/cli/cli-note-types.spec.ts`:

```typescript
test.describe('NoteType create with new fields', () => {
  const suffix = Date.now();
  let ntId: number;

  test.afterAll(() => {
    const cli = createCliRunner();
    if (ntId) cli.run('note-type', 'delete', String(ntId));
  });

  test('create note type with meta-schema and section-config', async ({ cli }) => {
    const schema = '{"type":"object","properties":{"status":{"type":"string"}}}';
    const sectionConfig = '{"resources":false}';
    const nt = cli.runJson<NoteType>('note-type', 'create',
      '--name', `nt-fields-${suffix}`,
      '--meta-schema', schema,
      '--section-config', sectionConfig
    );
    expect(nt.ID).toBeGreaterThan(0);
    ntId = nt.ID;
  });
});
```

- [ ] **Step 4: Build and verify**

Run: `go build --tags 'json1 fts5' ./cmd/mr/`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add cmd/mr/commands/note_types.go e2e/tests/cli/cli-note-types.spec.ts
git commit -m "feat(cli): add meta-schema, section-config, custom-mrql-result flags to note-type"
```

---

### Task 5: Add SectionConfig and CustomMRQLResult flags to category and resource-category create

Both Category and ResourceCategory gained `SectionConfig` and `CustomMRQLResult` fields. The CLI `create` commands need to support them.

**Files:**
- Modify: `cmd/mr/commands/categories.go:82-138`
- Modify: `cmd/mr/commands/resource_categories.go:82-138`
- Test: `e2e/tests/cli/cli-categories.spec.ts`
- Test: `e2e/tests/cli/cli-resource-categories.spec.ts`

- [ ] **Step 1: Add flags to `newCategoryCreateCmd`**

Change the var declaration at line 83:

```go
var name, description, customHeader, customSidebar, customSummary, customAvatar, metaSchema, sectionConfig, customMRQLResult string
```

Add to the body map (after `MetaSchema` block):

```go
if sectionConfig != "" {
    body["SectionConfig"] = sectionConfig
}
if customMRQLResult != "" {
    body["CustomMRQLResult"] = customMRQLResult
}
```

Add flag registrations after the `meta-schema` flag:

```go
cmd.Flags().StringVar(&sectionConfig, "section-config", "", "JSON controlling which sections are visible on group detail pages for this category")
cmd.Flags().StringVar(&customMRQLResult, "custom-mrql-result", "", "Pongo2 template for rendering groups of this category in MRQL results")
```

- [ ] **Step 2: Add flags to `newResourceCategoryCreateCmd`**

Same changes as Step 1, but in `resource_categories.go`. Change the var declaration at line 83:

```go
var name, description, customHeader, customSidebar, customSummary, customAvatar, metaSchema, sectionConfig, customMRQLResult string
```

Add to the body map (after `MetaSchema` block):

```go
if sectionConfig != "" {
    body["SectionConfig"] = sectionConfig
}
if customMRQLResult != "" {
    body["CustomMRQLResult"] = customMRQLResult
}
```

Add flag registrations after the `meta-schema` flag:

```go
cmd.Flags().StringVar(&sectionConfig, "section-config", "", "JSON controlling which sections are visible on resource detail pages for this category")
cmd.Flags().StringVar(&customMRQLResult, "custom-mrql-result", "", "Pongo2 template for rendering resources of this category in MRQL results")
```

- [ ] **Step 3: Add E2E tests**

Add to `e2e/tests/cli/cli-categories.spec.ts`:

```typescript
test.describe('Category create with new fields', () => {
  const suffix = Date.now();
  let catId: number;

  test.afterAll(() => {
    const cli = createCliRunner();
    if (catId) cli.run('category', 'delete', String(catId));
  });

  test('create category with section-config and custom-mrql-result', async ({ cli }) => {
    const sectionConfig = '{"resources":false}';
    const customMRQL = '<div>{{ entity.Name }}</div>';
    const cat = cli.runJson<Category>('category', 'create',
      '--name', `cat-fields-${suffix}`,
      '--section-config', sectionConfig,
      '--custom-mrql-result', customMRQL
    );
    expect(cat.ID).toBeGreaterThan(0);
    catId = cat.ID;
  });
});
```

Add a similar test block to `e2e/tests/cli/cli-resource-categories.spec.ts`.

- [ ] **Step 4: Build and verify**

Run: `go build --tags 'json1 fts5' ./cmd/mr/`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add cmd/mr/commands/categories.go cmd/mr/commands/resource_categories.go e2e/tests/cli/cli-categories.spec.ts e2e/tests/cli/cli-resource-categories.spec.ts
git commit -m "feat(cli): add section-config and custom-mrql-result flags to category and resource-category"
```

---

### Task 6: Add `--render` flag to MRQL command

The API supports `?render=1` query parameter on both `/v1/mrql` and `/v1/mrql/saved/run` to trigger server-side template rendering via `CustomMRQLResult`.

**Files:**
- Modify: `cmd/mr/commands/mrql.go:62-149` (main command)
- Modify: `cmd/mr/commands/mrql.go:230-276` (run subcommand)
- Test: `e2e/tests/cli/cli-mrql.spec.ts`

- [ ] **Step 1: Add `--render` flag to main mrql command**

Add a `render` bool var alongside the existing vars at line 64:

```go
var (
    fileFlag string
    limit    int
    buckets  int
    offset   int
    render   bool
)
```

Update the Long description to document the flag (add after the existing examples):

```
Rendering:
  mr mrql --render 'type = resource AND tags = "photo"'
  The --render flag requests server-side template rendering using CustomMRQLResult
  templates. Results include a renderedHTML field when a template is configured.
```

In the RunE body, after building the body map (around line 127), add:

```go
q := url.Values(nil)
if render {
    q = url.Values{}
    q.Set("render", "1")
}
```

And update the Post call to pass `q` instead of `nil`:

```go
if err := c.Post("/v1/mrql", q, body, &raw); err != nil {
```

Register the flag after line 141:

```go
mrqlCmd.Flags().BoolVar(&render, "render", false, "Request server-side template rendering via CustomMRQLResult")
```

- [ ] **Step 2: Add `--render` flag to `run` subcommand**

Add a `render` bool var to `newMRQLRunCmd` alongside existing vars (line 232):

```go
var (
    limit   int
    buckets int
    offset  int
    render  bool
)
```

In the RunE body, add after offset handling:

```go
if render {
    q.Set("render", "1")
}
```

Register the flag after line 273:

```go
cmd.Flags().BoolVar(&render, "render", false, "Request server-side template rendering via CustomMRQLResult")
```

- [ ] **Step 3: Build and verify**

Run: `go build --tags 'json1 fts5' ./cmd/mr/`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/commands/mrql.go
git commit -m "feat(cli): add --render flag to mrql command and run subcommand"
```

---

### Task 7: Run E2E tests

- [ ] **Step 1: Build the full application**

Run: `npm run build`

- [ ] **Step 2: Run CLI E2E tests**

Run: `cd e2e && npm run test:with-server:cli`
Expected: All tests pass

- [ ] **Step 3: Fix any failures, commit fixes**

If tests fail, diagnose and fix. Then:

```bash
git add -A
git commit -m "fix(cli): address E2E test failures from feature parity update"
```
