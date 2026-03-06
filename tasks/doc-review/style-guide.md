# Mahresources Documentation Style Guide

This guide defines the writing standards for all Mahresources documentation. It was derived from analysis of the 50 existing doc files and codifies the patterns that work well while eliminating the few that do not.

The target style is **structured tutorial**, modeled after Django's documentation: direct, concrete, example-driven.

---

## 1. Voice & Tone

**Person and tense.** Write in second person ("you"), present tense. Address the reader directly.

**Imperative for instructions.** Use bare imperatives for steps and commands:

- YES: "Run the server with ephemeral mode."
- NO: "You should run the server with ephemeral mode."
- NO: "You can run the server with ephemeral mode."

**Direct and concrete.** Every claim must be backed by an example, a code block, or a table. Do not describe a capability without demonstrating it.

- YES: "Filter resources by content type: `GET /v1/resources?ContentType=image/jpeg`"
- NO: "The API supports filtering by content type."

**No hedging unless genuinely uncertain.** Remove "may", "might", "could potentially" unless the behavior is actually nondeterministic or platform-dependent. Legitimate uses: "You may need to update the Dockerfile" (because it depends on the user's version).

**No cheerfulness or salesmanship.** The docs describe what the software does. They do not promote it.

**State facts, not feelings.** Do not tell the reader how they will feel ("you'll love this feature") or what they find easy ("this is straightforward"). Describe the mechanism and let them decide.

---

## 2. Banned Phrases

### Core banned list

| Banned | Replacement |
|--------|-------------|
| "It's important to note that..." | State the fact directly. |
| "In order to..." | "To..." |
| "This allows you to..." | Show what it does with an example. |
| "Seamlessly" | Delete. Describe the integration mechanism. |
| "Robust" | Delete. Name the specific reliability property. |
| "Powerful" | Delete. Show what it does. |
| "Elegant" | Delete. Show the code. |
| "Intuitive" | Delete. Provide the instructions. |
| "With ease" / "Effortlessly" / "Simply" | Delete. If it were simple, you would not need docs. |
| "Leverage" / "Utilize" | "Use" |
| "Comprehensive" / "Extensive" (self-describing) | Name the specific things covered. |
| "Under the hood" | Describe the mechanism by name. |
| "Out of the box" | "By default" |
| "Best practices" (unnamed) | Name the specific practices. |
| "And much more!" / "...and more" | List the things or stop. |
| "Feel free to..." | Use imperative: "Do X." |
| "Please note that..." | State the fact. Use a `:::warning` admonition for genuinely important warnings. |
| "As mentioned earlier/above/below" | Link to the section or repeat the essential fact. |
| "It should be noted" | State the fact. |
| "A wide range/variety of" | Name the things. |
| "Designed to" | Delete. Describe what it does, not its intent. |
| "Provides a" / "Provides the ability to" | Show the capability with an example. |
| "Enables you to" / "Makes it easy to" | Use imperative or show the example. |
| "Take advantage of" | "Use" |
| "A number of" | State the count or list the items. |
| "Whether you...or..." (false dichotomy opener) | Pick the relevant case and write for it. |
| "You can easily" / "You'll be able to" | Imperative: "Do X." |
| "Basically" / "Essentially" | Delete. Say the actual thing. |
| "Straightforward" | Delete. Provide the instructions. |
| "It is worth noting" / "Worth mentioning" | State the fact. |
| "Importantly" | Delete. If it is important, the reader will understand from context or from a `:::warning` admonition. |
| "In this section" / "In this page" | Delete. The heading already orients the reader. |
| "Let's" / "We'll" | Use second person: "you" and imperative verbs. |

### Phrases found in current docs that need correction

| Found in file | Phrase | Fix |
|---------------|--------|-----|
| `configuration/overview.md` | "This allows you to override `.env` settings for specific runs." | "Command-line flags override `.env` settings." |
| `features/custom-block-types.md` | Section heading "Validation Best Practices" | Rename to "Validation Rules" or list the specific rules without the heading. |

---

## 3. Page Structure Templates

### Concept Page Template

Use for pages that explain what something is: entities, data models, architectural components.

```markdown
# Title

One sentence: what this is and when you need it.

## Core Idea

2-3 paragraphs explaining the concept. Include at least one code block, diagram, or table
before the third paragraph.

## Properties

Table of fields/properties with types and descriptions.

## Behavior

Specific behaviors: creation, deletion cascades, constraints. Use tables for 4+ items.

## Relationships

How this entity connects to others. Use a tree diagram or table.

## Related Pages

- [Page title](../path/to/page.md) -- one-line description of what the reader finds there
```

**Current docs that follow this pattern well:** `concepts/resources.md`, `concepts/notes.md`, `concepts/groups.md`, `concepts/relationships.md`.

### How-To Page Template

Use for pages that walk the reader through a task: setup, configuration, workflows.

```markdown
# Title

One sentence: what you will accomplish.

## Prerequisites

What you need before starting. Bullet list or table.

## Step 1: [Action]

Numbered steps with code examples. Each step has:
- One imperative instruction
- A code block showing the command or UI action
- An optional note about what happens

## Step 2: [Action]

Continue with numbered steps.

## Verification

How to confirm the task succeeded. Include a command or expected output.

## Related Pages

- [Page title](../path/to/page.md) -- one-line description
```

**Current docs that follow this pattern well:** `getting-started/quick-start.md`, `getting-started/first-steps.md`, `deployment/systemd.md`.

### API Reference Page Template

Use for pages documenting HTTP endpoints.

```markdown
# Title

One sentence: what this API covers.

## Endpoint Name

### METHOD /v1/path

One sentence describing what it does.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name`    | string | Yes    | What it controls |

**Request:**

```bash
curl -X POST http://localhost:8181/v1/path \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"key": "value"}'
```

**Response:**

```json
{
  "ID": 1,
  "Name": "example"
}
```
```

**Current docs that follow this pattern well:** `api/resources.md`, `api/notes.md`, `api/groups.md`.

---

## 4. Code Example Rules

### Every configuration option: three formats

Show CLI flag, environment variable, and a usage example together:

```markdown
### FFmpeg Path

| Flag | Env Variable | Default |
|------|-------------|---------|
| `-ffmpeg-path` | `FFMPEG_PATH` | auto-detect |

```bash
./mahresources -ffmpeg-path=/usr/bin/ffmpeg -db-type=SQLITE -db-dsn=./db.sqlite -file-save-path=./files
```

Or in `.env`:

```bash
FFMPEG_PATH=/usr/bin/ffmpeg
```
```

### Every API endpoint: curl request and JSON response

No endpoint is documented without both a request example and a response example. The request must be a runnable curl command. The response must be valid JSON.

### Every concept: at least one concrete example

Do not explain a concept abstractly for more than two paragraphs without a code block, table, or diagram.

### Use Mahresources defaults in examples

- Port: `8181`
- Database: SQLite with `./mahresources.db`
- File path: `./files`
- Bind address: `:8181` or `127.0.0.1:8181`
- Share port (when needed): `8383`

### No placeholder values

- NO: `your-database.db`, `your-value-here`, `<insert-token>`
- YES: `mahresources.db`, `sk-abc123`, `a1b2c3d4e5f6g7h8`

Use realistic but obviously fake values. API keys start with a recognizable prefix (`sk-abc123`). Tokens are hex strings. Paths use common Linux conventions (`/opt/mahresources/`).

### Code block language tags

Always specify the language: `bash`, `sql`, `json`, `lua`, `go`, `html`, `javascript`, `yaml`, `nginx`, `ini`, `caddyfile`, `dockerfile`.

For generic endpoint signatures without a specific language, use bare fenced blocks (no language tag):

```
GET /v1/resources?ContentType=image/jpeg
```

---

## 5. Terminology Canon

Use these exact terms. Do not substitute synonyms.

### Entities

| Canonical Term | Do NOT Use | Notes |
|---------------|-----------|-------|
| **resource** | file, asset, upload, attachment | A resource is the entity. The *file* is its stored content. "Upload a file to create a resource." |
| **note** | document, article, post, entry | Text content entity. |
| **group** | collection, folder, directory, container | Hierarchical organizer. Groups *contain* entities. |
| **tag** | label, keyword, marker | Flat cross-cutting label. |
| **category** | type (when referring to group categories) | Defines the type of a Group. Categories apply to Groups only. |
| **resource category** | resource type (when referring to the entity) | Defines the type of a Resource. |
| **note type** | note category | Defines the type of a Note. |
| **relation** | connection, link (when referring to the Group Relation entity) | Typed, directed edge between two Groups. |
| **relation type** | relationship type, link type | Defines the kind of relation. |
| **query** | saved query, stored query, report | The entity name is "Query". Capitalize when referring to the entity. |
| **series** | sequence, collection (when referring to the Series entity) | Groups Resources with shared metadata. |
| **note block** / **block** | section, widget, component (in note context) | Structured content unit within a Note. |
| **version** | revision, history entry | A snapshot of a Resource's file at a point in time. |
| **plugin** | extension, add-on, module | Lua-based extension. |
| **action** (plugin) | operation, command (in plugin context) | Plugin-contributed operation on entities. |
| **hook** (plugin) | callback, trigger, event handler | Plugin function that fires before/after operations. |
| **injection** (plugin) | slot, widget (in plugin context) | HTML injected into named page slots. |
| **log entry** | audit record, event | Activity log record. |
| **preview** / **thumbnail** | thumb, icon (when referring to generated images) | Use "thumbnail" for the small image; "preview" for the endpoint. |

### Relationships

| Canonical Term | Do NOT Use | Notes |
|---------------|-----------|-------|
| **owner** | parent (except in Group hierarchy context) | The Group that owns an entity. "The resource's owner is Group 5." |
| **owned by** | belongs to, child of (for ownership) | The ownership relation. |
| **related to** | associated with, linked to | Many-to-many connections. |
| **parent group** / **child group** | parent/child (when specifically about Group hierarchy) | Use for Group-to-Group ownership only. |

### Operations

| Canonical Term | Do NOT Use | Notes |
|---------------|-----------|-------|
| **merge** | combine, consolidate | Combining entities into a winner. |
| **clone** | copy, duplicate | Creating a copy of a Group. |
| **bulk operation** | batch operation, mass operation | Operating on multiple entities. |
| **winner** / **loser** | target/source, keep/discard (in merge context) | The winner survives; losers are deleted. |

### Technical terms

| Canonical Term | Do NOT Use | Notes |
|---------------|-----------|-------|
| **perceptual hash** | phash (except as shorthand after first use) | The visual fingerprint of an image. |
| **Hamming distance** | similarity score (imprecise) | The number of differing bits between two hashes. |
| **content-addressable storage** | hash-based storage | Files stored by their SHA1 hash. |
| **FTS5** | full-text search engine (when being specific about SQLite) | SQLite's full-text search extension. |
| **meta** / **metadata** | custom fields, properties (in JSON metadata context) | The JSON `meta` field on entities. |
| **MetaQuery** | meta filter, metadata search | The query parameter for filtering by metadata. |
| **share token** | share link, public URL (when referring to the token itself) | The 32-character hex token. |
| **share server** | public server (when being specific) | The separate HTTP server for shared notes. |

### UI elements

| Canonical Term | Do NOT Use | Notes |
|---------------|-----------|-------|
| **global search** | universal search, search bar | Cmd/Ctrl+K search. |
| **lightbox** | image viewer, gallery viewer | The full-screen image/video viewer. |
| **bulk editor** | bulk action bar, selection toolbar | The inline editor that appears on selection. |
| **Download Cockpit** | download manager, download panel | The floating download status UI. |
| **entity picker** | selector modal, chooser | The modal for selecting entities in blocks. |

---

## 6. Length Guidelines

### Page opener

**1 sentence maximum.** State what the page covers and move on.

- YES: "Groups are hierarchical containers that organize Resources, Notes, and other Groups."
- NO: A paragraph explaining why groups exist, who uses them, and what problems they solve.

### Concept explanation before first code example

**2-4 paragraphs maximum.** If you have not shown code, a table, or a diagram within 4 paragraphs, you have waited too long.

### Maximum paragraph length

**4 sentences.** Break longer paragraphs into two, or convert the information to a list or table.

### Tables over prose

Use a table when listing **4 or more items** with parallel structure (properties, parameters, options, comparisons). The existing docs do this well -- maintain the pattern.

### Admonitions

Use Docusaurus admonitions sparingly and only for content the reader must not miss:

- `:::danger` -- Security warnings, data loss risks, breaking changes
- `:::warning` -- Cascade deletes, irreversible operations, common mistakes
- `:::caution` -- Version-specific issues, known inconsistencies
- `:::tip` -- Shortcuts, non-obvious configurations
- `:::note` -- Technical context that clarifies but is not essential

Do not use admonitions for routine information. If the reader needs to know it to complete the task, put it in the main text.

---

## 7. Before/After Examples

### Example 1: Filler phrasing

**BEFORE** (from `configuration/overview.md`, line 35):
> Command-line flags take precedence over environment variables. This allows you to override `.env` settings for specific runs.

**AFTER:**
> Command-line flags take precedence over environment variables, so a flag overrides the same setting from `.env`.

**WHY:** "This allows you to" is banned filler. The rewrite folds the two sentences into one and eliminates the indirect phrasing.

---

### Example 2: Abstract section heading

**BEFORE** (from `features/custom-block-types.md`, line 467):
> ## Validation Best Practices
>
> 1. **Validate required fields** - Return clear error messages for missing data
> 2. **Validate data types** - Ensure numbers are in valid ranges, strings are not too long

**AFTER:**
> ## Validation Rules
>
> 1. **Validate required fields.** Return clear error messages for missing data.
> 2. **Validate data types.** Reject numbers outside valid ranges and strings that exceed length limits.

**WHY:** "Best practices" is banned when the practices are named -- rename the heading to what the section actually contains. "Ensure" is vague; "Reject" is the concrete action.

---

### Example 3: Vague feature list in intro

**BEFORE** (from `intro.md`, lines 26-27):
> - **Plugin system** - Extend Mahresources with plugins that hook into CRUD events and run background actions.

**AFTER:**
> - **Plugin system** -- Lua plugins that intercept create/update/delete operations, add custom pages, and run background jobs.

**WHY:** "Extend Mahresources with" is indirect. The rewrite names the language (Lua) and lists the three concrete capabilities instead of using the vague "hook into CRUD events."

---

### Example 4: Passive setup instruction

**BEFORE** (from `troubleshooting.md`, lines 13-14):
> **Solutions:**
> - Reduce the number of database connections using `-max-db-connections=2`

**AFTER:**
> **Fix:**
> - Set `-max-db-connections=2` to limit concurrent writes.

**WHY:** "Reduce the number of database connections using" is wordy. The rewrite is imperative ("Set") and adds the reason ("to limit concurrent writes") in fewer words.

---

### Example 5: Unnecessary orienting clause

**BEFORE** (from `features/image-similarity.md`, lines 148-149):
> Merging is permanent. The merged resources are deleted. Make sure you have selected the correct resource to keep.

**AFTER:**
> Merging is permanent -- the merged resources are deleted. Verify that the winner resource is the one you intend to keep.

**WHY:** "Make sure" is weaker than the imperative "Verify." The em dash tightens the first two sentences. "The correct resource to keep" becomes "the winner resource" using canonical terminology.

---

## Appendix: Checklist for New Pages

Before merging a new doc page, verify:

- [ ] Page opens with exactly one sentence
- [ ] First code example appears within 4 paragraphs
- [ ] No paragraph exceeds 4 sentences
- [ ] All configuration options show CLI flag, env var, and example
- [ ] All API endpoints show curl request and JSON response
- [ ] No banned phrases from Section 2
- [ ] All entity names match the Terminology Canon in Section 5
- [ ] Tables used for lists of 4+ parallel items
- [ ] Admonitions used only for warnings and dangers, not routine info
- [ ] Code blocks have language tags
- [ ] No placeholder values -- all examples use realistic defaults
- [ ] Page ends with Related Pages section linking to 2-4 related docs
