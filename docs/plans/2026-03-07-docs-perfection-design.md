# Documentation Perfection: Agent Team Design

## Goal

Make the docs under `docs-site/` perfect and up-to-date with zero AI-slop. Fix accuracy, fill coverage gaps, add contextual screenshots, and clean up writing — all in parallel using a coordinated agent team.

## Current State

- 48 markdown doc files in `docs-site/docs/`
- Zero screenshots/images
- Writing quality generally good (0-3/10 AI-slop), ~5-6 files have mild generic phrasing
- App has ~108 API endpoints, 55+ template pages, 40+ frontend components
- Most features documented, but cross-referencing needed to find gaps

## Agent Team

### Agents (7 total)

| Agent | Role | Edits Docs? |
|-------|------|-------------|
| Technical Summarizer A | Ground truth for entities & CRUD (Resources, Notes, Groups, Tags, Categories, Series, Relations, Queries) | No — report only |
| Technical Summarizer B | Ground truth for advanced features & plugins (versioning, similarity, blocks, search, bulk ops, sharing, plugins, etc.) | No — report only |
| Doc Checker A | Cross-reference concepts/getting-started/user-guide docs against ground truth | No — report only |
| Doc Checker B | Cross-reference features/api/config/deployment docs against ground truth | No — report only |
| Writing Coach | Edit all docs: fix accuracy, fill gaps, remove slop, maintain tone | Yes |
| Screenshot Agent | Build app, seed content, capture contextual screenshots via Playwright, add to docs | Yes (image refs only) |
| Conductor | Dispatch, collect, resolve conflicts, final review | Yes (final pass) |

### Execution Phases

**Phase 1 — Ground Truth + App Setup (Parallel)**
- Summarizer A reads entity code, produces ground truth report
- Summarizer B reads feature code, produces ground truth report
- Screenshot Agent builds app, starts ephemeral server, seeds realistic content

**Phase 2 — Doc Checking (Parallel, depends on Phase 1 summarizers)**
- Doc Checker A compares concepts/getting-started/user-guide docs against reports
- Doc Checker B compares features/api/config/deployment docs against reports
- Screenshot Agent navigates pages, captures screenshots

**Phase 3 — Fixes (Parallel, depends on Phase 2 checkers)**
- Writing Coach applies all fixes from checker reports
- Screenshot Agent adds image references to docs

**Phase 4 — Final Review (Sequential)**
- Conductor reviews all changes for consistency
- Verifies screenshots render, cross-references work, sidebar is accurate
- Commits result

### Conflict Resolution
- Writing Coach owns all text edits
- Screenshot Agent only adds image references — never rewrites text
- If both touch same file: Writing Coach first, Screenshot Agent follows

## Content Seeding Plan

Create realistic data in ephemeral instance via API:

- 3-4 Groups with hierarchy (e.g., "Photography Projects" > "Landscapes")
- 8-10 Resources (images, PDF, text file) with tags and group associations
- 3-4 Notes with different block types (text, heading, todos, gallery, references)
- 5-6 Tags (e.g., "landscape", "draft", "reviewed", "important", "archived")
- 2 Categories (e.g., "Documents", "Media")
- 1 Saved Query, 1 Series, 1 Group Relation

## Screenshot Plan (~15-20 captures)

| Doc | Screenshot | What it shows |
|-----|-----------|---------------|
| intro.md | dashboard.png | Populated dashboard |
| navigation.md | grid-view.png | Resource grid with images |
| navigation.md | global-search.png | Search modal with results |
| managing-resources.md | resource-detail.png | Resource with tags, groups, meta |
| managing-resources.md | upload-form.png | Create resource page |
| managing-notes.md | note-blocks.png | Note with text, todos, gallery |
| organizing-with-groups.md | group-tree.png | Hierarchical tree view |
| organizing-with-groups.md | group-detail.png | Group with owned items |
| bulk-operations.md | bulk-selection.png | Items selected with bulk toolbar |
| search.md | search-results.png | Filtered resource list |
| versioning.md | version-compare.png | Side-by-side comparison |
| image-similarity.md | similar-images.png | Similarity matches |
| saved-queries.md | query-editor.png | SQL editor with results |
| plugin-system.md | plugin-management.png | Plugin list page |
| entity-picker.md | entity-picker.png | Picker modal open |
| download-queue.md | download-cockpit.png | Queue with statuses |
| activity-log.md | activity-log.png | Log entries |

**Specs:** 1200px width, PNG, light mode, saved to `docs-site/static/img/`

## Writing Coach Rules

### Fix
- Inaccurate descriptions (doesn't match code)
- Missing features (exist in code, not in docs)
- AI-slop phrases ("seamlessly", "leverages", "robust", "comprehensive", generic openings)
- Filler sentences (restate heading, add no info)
- Vague descriptions (replace with specifics)

### Don't Fix
- Docs that are already good
- Don't add boilerplate admonitions
- Don't restructure working docs
- Don't change accurate terminology

### Tone
- "You" for the reader
- Short sentences, lead with action
- Code examples over prose
- State limitations directly
- No exclamation marks, no enthusiasm

### New Doc Threshold
Feature gets its own page if it has its own UI page or 3+ API endpoints. Otherwise fold into existing page.

## Screenshot Technology

Use Playwright (already set up in `e2e/`) rather than Chrome automation. More reliable, scriptable, and consistent rendering.
