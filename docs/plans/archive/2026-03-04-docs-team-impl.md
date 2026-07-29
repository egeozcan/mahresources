# Documentation Team Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Bring docs-site to 100% accuracy and coverage with zero AI-slop using a coordinated 8-agent team.

**Architecture:** Assembly line pipeline — Summarizers extract features from code, Checkers compare against existing docs, Coach establishes style, Writers create/rewrite under Coach supervision, Hallucination Checker verifies against code. Conductor orchestrates all handoffs.

**Tech Stack:** Claude Code teams (TeamCreate + Agent + SendMessage), Docusaurus docs-site, mahresources Go codebase

---

## Shared Context

**Docs-site root:** `/Users/egecan/Code/mahresources/docs-site`
**Docs directory:** `/Users/egecan/Code/mahresources/docs-site/docs`
**Codebase root:** `/Users/egecan/Code/mahresources`
**Working artifacts:** `/Users/egecan/Code/mahresources/docs-site/.work/` (intermediate files, not committed)

**Existing docs (40 files):**

| Section | Files | Path prefix |
|---------|-------|-------------|
| Getting Started | installation, quick-start, first-steps | getting-started/ |
| Core Concepts | overview, resources, notes, groups, tags-categories, relationships | concepts/ |
| User Guide | navigation, managing-resources, managing-notes, organizing-with-groups, search, bulk-operations | user-guide/ |
| Configuration | overview, database, storage, advanced | configuration/ |
| Advanced Features | versioning, image-similarity, saved-queries, custom-templates, note-sharing, download-queue, activity-log, custom-block-types, entity-picker | features/ |
| API Reference | overview, resources, notes, groups, other-endpoints | api/ |
| Deployment | docker, systemd, reverse-proxy, public-sharing, backups | deployment/ |
| Other | intro, troubleshooting | root |

**New docs to create (7):**
1. `features/plugin-system.md`
2. `features/plugin-actions.md`
3. `concepts/series.md`
4. `concepts/note-blocks.md`
5. `features/job-system.md`
6. `features/meta-schemas.md`
7. `api/plugins.md`

---

## Task 1: Setup — Create Team and Working Directory

**Step 1: Create the working directory**

```bash
mkdir -p /Users/egecan/Code/mahresources/docs-site/.work
```

**Step 2: Create the team**

Use TeamCreate:
```
team_name: "docs-team"
description: "Documentation audit, rewrite, and expansion team"
```

**Step 3: Create all phase tasks in the task list**

Create these tasks with dependencies (use TaskCreate + TaskUpdate for blockedBy):

| ID | Task | Owner | Blocked By |
|----|------|-------|------------|
| 1 | Phase 1: Create style guide | coach | — |
| 2 | Phase 2a: Summarize core entities | summarizer-a | — |
| 3 | Phase 2b: Summarize plugins & advanced features | summarizer-b | — |
| 4 | Phase 3a: Gap analysis — entities | checker-a | 1, 2 |
| 5 | Phase 3b: Gap analysis — plugins & features | checker-b | 1, 3 |
| 6 | Phase 4a: Write/rewrite batch A | writer-a | 4, 5 |
| 7 | Phase 4b: Write/rewrite batch B | writer-b | 4, 5 |
| 8 | Phase 5: Hallucination check all docs | hallucination-checker | 6, 7 |
| 9 | Phase 6: Final review + sidebars + commit | conductor | 8 |

---

## Task 2: Phase 1 — Coach Creates Style Guide

**Spawn the coach agent:**

```
name: "coach"
subagent_type: general-purpose
team_name: "docs-team"
```

**Prompt for coach:**

```
You are the Writing Coach for the mahresources documentation team.

YOUR TASK: Read all existing documentation and create a style guide.

STEP 1: Read every .md file in /Users/egecan/Code/mahresources/docs-site/docs/ (all 40 files).

STEP 2: Identify the 3-5 best-written docs — those with the clearest language, most useful structure, and best examples.

STEP 3: Identify the 3-5 worst-written docs — those with AI-slop, vague language, filler, or poor structure.

STEP 4: Write a style guide to /Users/egecan/Code/mahresources/docs-site/.work/style-guide.md covering:

A. TONE & VOICE
- What tone the best docs use (with quotes as examples)
- What to avoid (with anti-examples from the worst docs)

B. STRUCTURE TEMPLATES
- Standard doc structure for concept pages
- Standard doc structure for feature pages
- Standard doc structure for API reference pages
- Standard doc structure for how-to/guide pages

C. TERMINOLOGY
- Canonical names for all entities (Resource, Note, Group, Tag, Category, etc.)
- Canonical names for features (perceptual hashing NOT "image fingerprinting", etc.)
- Words/phrases to NEVER use (AI-slop list)

D. AI-SLOP BLACKLIST (be exhaustive)
- Filler phrases: "In this section, we will explore...", "This guide will walk you through..."
- Hedging: "It's worth noting that...", "You may want to consider..."
- Generic transitions: "Let's dive into...", "Now that we've covered..."
- Unnecessary adverbs: "simply", "easily", "just", "seamlessly"
- Vague claims: "powerful", "robust", "comprehensive", "flexible"
- Restating the obvious
- Any pattern you find in the worst docs

E. FORMATTING RULES
- When to use tables vs lists vs prose
- Code block conventions (language tags, realistic examples)
- Heading hierarchy rules
- Link conventions

STEP 5: Send a message to "conductor" with a summary of your findings and confirm the style guide is written.
```

**Wait for coach to complete.** Read the style guide to verify quality.

---

## Task 3: Phase 2a — Summarizer A Extracts Core Entity Features

**Can run in parallel with Task 2.**

**Spawn summarizer-a:**

```
name: "summarizer-a"
subagent_type: general-purpose
team_name: "docs-team"
```

**Prompt for summarizer-a:**

```
You are Technical Summarizer A for the mahresources documentation team.

YOUR TASK: Read the mahresources codebase and produce comprehensive feature specs for CORE ENTITIES.

Your scope:
- Resources (models/resource_model.go, application_context/resource_context.go, server/api_handlers/, server/template_handlers/)
- Notes (models/note_model.go, application_context/note_context.go, note blocks system)
- Groups (models/group_model.go, application_context/group_context.go, group relations)
- Tags (models/tag_model.go, application_context/tag_context.go)
- Categories (models/category_model.go, models/resource_category_model.go)
- Series (models/series_model.go, application_context/series_context.go)
- Relationships (many-to-many tables, ownership)
- Metadata system (JSON meta fields, meta schema validation, meta key discovery)
- Search & filtering (database_scopes/, full-text search, query parameters)
- Note blocks (models/note_block_model.go, block types, reordering)

FOR EACH FEATURE, document:
1. What it does (user-facing behavior)
2. All API endpoints (method, path, parameters, response shape)
3. All configuration flags/env vars that affect it
4. Edge cases and limitations
5. How it relates to other features

Write your complete specs to /Users/egecan/Code/mahresources/docs-site/.work/spec-entities.md

Be exhaustive. Read the actual Go code — don't guess. Include exact parameter names, types, and defaults from the code.

When done, send a message to "conductor" confirming completion and listing any features you found that weren't expected.
```

---

## Task 4: Phase 2b — Summarizer B Extracts Plugin & Advanced Features

**Can run in parallel with Tasks 2 and 3.**

**Spawn summarizer-b:**

```
name: "summarizer-b"
subagent_type: general-purpose
team_name: "docs-team"
```

**Prompt for summarizer-b:**

```
You are Technical Summarizer B for the mahresources documentation team.

YOUR TASK: Read the mahresources codebase and produce comprehensive feature specs for PLUGINS & ADVANCED FEATURES.

Your scope:
- Plugin system (plugin_manager/, plugin discovery, lifecycle, Lua VM)
- Plugin APIs (database API, HTTP API, JSON API, settings API)
- Plugin actions (action registration, form parameters, filters, async execution)
- Plugin hooks (lifecycle hooks, template injections, custom pages, menu items)
- Download queue (download manager, job states, SSE progress, pause/resume/cancel/retry)
- Job system (unified async jobs for downloads + plugin actions)
- Resource versioning (version CRUD, restore, compare, cleanup, deduplication)
- Image similarity (perceptual hashing: DHash/AHash, Hamming distance, background worker, LRU cache)
- Configuration system (all flags, env vars, ephemeral mode, seed-db, seed-fs, copy-on-write, alt filesystems)
- Deployment (how the binary runs, static file serving, template loading)
- Activity logging (audit log, log levels, actions, entity tracking)
- Custom templates (Pongo2 template system, custom headers/sidebars)
- Note sharing (share tokens, public access)
- Entity picker (UI component for selecting entities)
- Saved queries (SQL query storage and execution)
- Thumbnail generation (image, video via ffmpeg, office docs via LibreOffice)

FOR EACH FEATURE, document:
1. What it does (user-facing behavior)
2. All API endpoints (method, path, parameters, response shape)
3. All configuration flags/env vars that affect it
4. Edge cases and limitations
5. How it relates to other features

Write your complete specs to /Users/egecan/Code/mahresources/docs-site/.work/spec-plugins.md

Be exhaustive. Read the actual Go code — don't guess. Include exact parameter names, types, and defaults.

When done, send a message to "conductor" confirming completion and listing any features you found that weren't expected.
```

---

## Task 5: Phase 3a — Checker A Analyzes Entity Doc Gaps

**Blocked by: Tasks 2 (style guide) and 3 (entity specs).**

**Spawn checker-a:**

```
name: "checker-a"
subagent_type: general-purpose
team_name: "docs-team"
```

**Prompt for checker-a:**

```
You are Doc Checker A for the mahresources documentation team.

YOUR TASK: Compare the technical specs against existing documentation and produce a gap/quality report.

INPUTS (read these files first):
1. Style guide: /Users/egecan/Code/mahresources/docs-site/.work/style-guide.md
2. Entity specs: /Users/egecan/Code/mahresources/docs-site/.work/spec-entities.md

DOCS TO CHECK (read every one):
- /Users/egecan/Code/mahresources/docs-site/docs/intro.md
- /Users/egecan/Code/mahresources/docs-site/docs/concepts/*.md (all 6)
- /Users/egecan/Code/mahresources/docs-site/docs/user-guide/*.md (all 6)
- /Users/egecan/Code/mahresources/docs-site/docs/api/overview.md
- /Users/egecan/Code/mahresources/docs-site/docs/api/resources.md
- /Users/egecan/Code/mahresources/docs-site/docs/api/notes.md
- /Users/egecan/Code/mahresources/docs-site/docs/api/groups.md
- /Users/egecan/Code/mahresources/docs-site/docs/api/other-endpoints.md
- /Users/egecan/Code/mahresources/docs-site/docs/getting-started/*.md (all 3)

FOR EACH DOC, produce:

A. ACCURACY CHECK
- List every technical claim and whether it matches the spec
- Flag any endpoints, parameters, or behaviors that are wrong or outdated
- Flag any features described that don't exist in the spec

B. COMPLETENESS CHECK
- List features from the spec that this doc SHOULD cover but doesn't
- Note any API endpoints missing from API docs

C. AI-SLOP CHECK
- Quote every line that violates the style guide
- Categorize each violation (filler, hedging, vague, adverb, etc.)

D. MISSING DOCS
- List features from the spec that have NO doc at all
- For each, suggest which section it belongs in

E. REWRITE PRIORITY
- Rate each doc: KEEP (good), EDIT (minor fixes), REWRITE (major issues)
- Brief justification for each rating

Write your complete report to /Users/egecan/Code/mahresources/docs-site/.work/gaps-entities.md

When done, send a message to "conductor" with a summary: how many docs need rewrite vs edit vs keep, and the biggest gaps found.
```

---

## Task 6: Phase 3b — Checker B Analyzes Plugin & Feature Doc Gaps

**Blocked by: Tasks 2 (style guide) and 4 (plugin specs).**

**Spawn checker-b:**

```
name: "checker-b"
subagent_type: general-purpose
team_name: "docs-team"
```

**Prompt for checker-b:**

```
You are Doc Checker B for the mahresources documentation team.

YOUR TASK: Compare the technical specs against existing documentation and produce a gap/quality report.

INPUTS (read these files first):
1. Style guide: /Users/egecan/Code/mahresources/docs-site/.work/style-guide.md
2. Plugin specs: /Users/egecan/Code/mahresources/docs-site/.work/spec-plugins.md

DOCS TO CHECK (read every one):
- /Users/egecan/Code/mahresources/docs-site/docs/features/*.md (all 9 files)
- /Users/egecan/Code/mahresources/docs-site/docs/configuration/*.md (all 4)
- /Users/egecan/Code/mahresources/docs-site/docs/deployment/*.md (all 5)
- /Users/egecan/Code/mahresources/docs-site/docs/troubleshooting.md

FOR EACH DOC, produce:

A. ACCURACY CHECK
- List every technical claim and whether it matches the spec
- Flag any endpoints, parameters, or behaviors that are wrong or outdated
- Flag any features described that don't exist in the spec

B. COMPLETENESS CHECK
- List features from the spec that this doc SHOULD cover but doesn't
- Note any API endpoints missing

C. AI-SLOP CHECK
- Quote every line that violates the style guide
- Categorize each violation (filler, hedging, vague, adverb, etc.)

D. MISSING DOCS
- List features from the spec that have NO doc at all (especially the plugin system)
- For each, suggest which section it belongs in and outline what it should cover

E. REWRITE PRIORITY
- Rate each doc: KEEP (good), EDIT (minor fixes), REWRITE (major issues)
- Brief justification for each rating

Write your complete report to /Users/egecan/Code/mahresources/docs-site/.work/gaps-plugins.md

When done, send a message to "conductor" with a summary: how many docs need rewrite vs edit vs keep, and the biggest gaps found.
```

---

## Task 7: Phase 4 Prep — Conductor Creates Writing Assignments

**Blocked by: Tasks 5 and 6 (both checker reports).**

**Step 1:** Read both gap reports:
- `/Users/egecan/Code/mahresources/docs-site/.work/gaps-entities.md`
- `/Users/egecan/Code/mahresources/docs-site/.work/gaps-plugins.md`

**Step 2:** Split all docs into two balanced batches for the two writers:

**Writer A batch (entity-focused):**
- All docs rated REWRITE or EDIT from checker-a report
- New docs: `concepts/series.md`, `concepts/note-blocks.md`, `features/meta-schemas.md`
- Getting started docs (if they need work)
- intro.md

**Writer B batch (plugin/feature-focused):**
- All docs rated REWRITE or EDIT from checker-b report
- New docs: `features/plugin-system.md`, `features/plugin-actions.md`, `features/job-system.md`, `api/plugins.md`
- Troubleshooting.md

**Step 3:** Write assignment files:
- `/Users/egecan/Code/mahresources/docs-site/.work/assignments-a.md` — Writer A's doc list with specific instructions per doc (from checker report)
- `/Users/egecan/Code/mahresources/docs-site/.work/assignments-b.md` — Writer B's doc list with specific instructions per doc

**Step 4:** Update team tasks — unblock writer tasks.

---

## Task 8: Phase 4a — Writer A Creates/Rewrites Entity Docs

**Blocked by: Task 7 (assignments ready).**

**Spawn writer-a:**

```
name: "writer-a"
subagent_type: general-purpose
team_name: "docs-team"
```

**Prompt for writer-a:**

```
You are Writer A for the mahresources documentation team.

YOUR TASK: Write and rewrite documentation according to your assignment.

READ THESE FIRST:
1. Style guide: /Users/egecan/Code/mahresources/docs-site/.work/style-guide.md
2. Your assignments: /Users/egecan/Code/mahresources/docs-site/.work/assignments-a.md
3. Entity specs: /Users/egecan/Code/mahresources/docs-site/.work/spec-entities.md

RULES:
- Follow the style guide exactly. Zero AI-slop.
- Every technical claim must come from the spec. Don't invent features.
- For REWRITE docs: read the original, understand what it was trying to say, then rewrite from scratch using the spec as source of truth.
- For EDIT docs: make only the changes noted in the assignment. Don't rewrite what's already good.
- For NEW docs: follow the structure template from the style guide for that doc type.
- Use realistic, working code examples. Don't use placeholder values like "example.com" — use mahresources-appropriate examples.
- Every doc must have proper Docusaurus frontmatter (sidebar_position, title).

WORKFLOW:
For each doc in your assignment:
1. Read the spec section for that feature
2. Read the existing doc (if editing/rewriting)
3. Write/rewrite the doc
4. Send a message to "coach" with the file path for review
5. Wait for coach's response. If revision needed, fix and resend.
6. Move to next doc after coach approves.

When all docs are complete, send a message to "conductor" confirming all assigned docs are done.
```

---

## Task 9: Phase 4b — Writer B Creates/Rewrites Plugin & Feature Docs

**Blocked by: Task 7 (assignments ready).**

**Spawn writer-b:**

```
name: "writer-b"
subagent_type: general-purpose
team_name: "docs-team"
```

**Prompt for writer-b:**

```
You are Writer B for the mahresources documentation team.

YOUR TASK: Write and rewrite documentation according to your assignment.

READ THESE FIRST:
1. Style guide: /Users/egecan/Code/mahresources/docs-site/.work/style-guide.md
2. Your assignments: /Users/egecan/Code/mahresources/docs-site/.work/assignments-b.md
3. Plugin specs: /Users/egecan/Code/mahresources/docs-site/.work/spec-plugins.md

RULES:
- Follow the style guide exactly. Zero AI-slop.
- Every technical claim must come from the spec. Don't invent features.
- For REWRITE docs: read the original, understand what it was trying to say, then rewrite from scratch using the spec as source of truth.
- For EDIT docs: make only the changes noted in the assignment. Don't rewrite what's already good.
- For NEW docs: follow the structure template from the style guide for that doc type.
- Use realistic, working code examples.
- Every doc must have proper Docusaurus frontmatter (sidebar_position, title).
- Plugin docs are NEW — there's no existing content. Build from the spec only.

WORKFLOW:
For each doc in your assignment:
1. Read the spec section for that feature
2. Read the existing doc (if editing/rewriting)
3. Write/rewrite the doc
4. Send a message to "coach" with the file path for review
5. Wait for coach's response. If revision needed, fix and resend.
6. Move to next doc after coach approves.

When all docs are complete, send a message to "conductor" confirming all assigned docs are done.
```

---

## Task 10: Phase 4 — Coach Reviews All Writer Output

**The coach (already spawned in Task 2) stays active through Phase 4.**

**Send this to coach when writers start:**

```
Writers are now active. Your role shifts to REVIEWER.

For each doc a writer sends you:
1. Read the doc at the file path they provide
2. Read the corresponding section of the style guide
3. Check for:
   - AI-slop violations (any phrase on the blacklist)
   - Tone consistency with the style guide
   - Structure matching the template for that doc type
   - Vague or unsubstantiated claims
   - Missing content that the checker report flagged
   - Formatting rule violations
4. If the doc passes: reply "APPROVED: [filepath]"
5. If the doc needs work: reply with specific line-by-line feedback. Quote the problem lines and say exactly what to change.

Be strict. Zero AI-slop means zero. Don't approve docs with "just one small issue" — send them back.

When both writers report all docs complete and approved, send a message to "conductor" confirming Phase 4 is done.
```

---

## Task 11: Phase 5 — Hallucination Checker Verifies All Docs

**Blocked by: Tasks 8 and 9 (both writers done).**

**Spawn hallucination-checker:**

```
name: "hallucination-checker"
subagent_type: general-purpose
team_name: "docs-team"
```

**Prompt for hallucination-checker:**

```
You are the Hallucination Checker for the mahresources documentation team.

YOUR TASK: Verify every technical claim in every doc against the actual codebase.

READ FIRST:
- Entity specs: /Users/egecan/Code/mahresources/docs-site/.work/spec-entities.md
- Plugin specs: /Users/egecan/Code/mahresources/docs-site/.work/spec-plugins.md

THEN CHECK EVERY .md FILE in /Users/egecan/Code/mahresources/docs-site/docs/

For each doc, verify against the ACTUAL CODEBASE (not just the specs — read the Go code):

1. API ENDPOINTS: For every endpoint mentioned, grep the codebase to confirm it exists.
   - Check: server/routes*.go, server/api_handlers/, server/template_handlers/
   - Verify: method (GET/POST/DELETE), exact path, parameter names

2. CONFIGURATION FLAGS: For every flag or env var mentioned, confirm it exists.
   - Check: main.go, application_context/context.go for flag definitions
   - Verify: flag name, env var name, default value, description

3. CODE EXAMPLES: For every code snippet, verify it would work.
   - curl commands: correct URL patterns, parameter names, JSON field names
   - Go code: correct types, function signatures, import paths
   - Config examples: correct syntax, valid values

4. FEATURE DESCRIPTIONS: For every behavioral claim, verify in code.
   - "Resources support X" — find where X is implemented
   - "The default is Y" — find where Y is set
   - "When Z happens, A occurs" — find the code path

5. MODEL FIELDS: For every field mentioned, verify it exists in the model.
   - Check: models/*_model.go

PRODUCE A REPORT at /Users/egecan/Code/mahresources/docs-site/.work/verification-report.md:

For each doc file:
- VERIFIED: Claims confirmed against code
- HALLUCINATION: Claims that don't match code (with evidence)
- UNVERIFIABLE: Claims that couldn't be confirmed (code too complex or ambiguous)

For each HALLUCINATION, include:
- The doc file and line
- What the doc says
- What the code actually does
- The source file and line in the codebase

When done, send a message to "conductor" with a summary: total claims checked, hallucinations found, unverifiable claims.
```

---

## Task 12: Phase 5b — Fix Hallucinations

**If hallucinations found:**

**Step 1:** Read the verification report.

**Step 2:** For each hallucination, determine which writer owns that doc.

**Step 3:** Send the writer a message with the specific fix needed:

```
HALLUCINATION FIX NEEDED in [filepath]:
Line: [quote the wrong line]
Code says: [what the code actually does]
Source: [codebase file:line]
Fix: [exact correction to make]
```

**Step 4:** Writer fixes, coach re-reviews the fix.

**Step 5:** If hallucination-checker found many issues, re-run verification on fixed docs.

---

## Task 13: Phase 6 — Final Review + Sidebars + Commit

**Blocked by: Task 11 (verification complete) and Task 12 (fixes applied).**

**Step 1: Update sidebars.ts**

Add new docs to the sidebar. The updated sidebars.ts should include:

```typescript
// In Core Concepts:
'concepts/series',
'concepts/note-blocks',

// In Advanced Features:
'features/plugin-system',
'features/plugin-actions',
'features/job-system',
'features/meta-schemas',

// In API Reference:
'api/plugins',
```

**Step 2: Verify docs build**

```bash
cd /Users/egecan/Code/mahresources/docs-site && npm run build
```

Fix any broken links or build errors.

**Step 3: Spot-check 5-10 docs**

Read a random sample of docs — both rewritten and new. Verify:
- No AI-slop
- Technical accuracy (quick check against code)
- Consistent style
- Working links

**Step 4: Clean up working directory**

```bash
rm -rf /Users/egecan/Code/mahresources/docs-site/.work/
```

**Step 5: Shutdown team**

Send shutdown_request to all active teammates. Wait for confirmations.

Use TeamDelete to clean up.

**Step 6: Commit**

```bash
cd /Users/egecan/Code/mahresources
git add docs-site/docs/ docs-site/sidebars.ts
git commit -m "docs: comprehensive audit, rewrite, and expansion of documentation

- Audit all 40 existing docs for accuracy, AI-slop, and consistency
- Rewrite docs that needed improvement per style guide
- Add 7 new docs: plugin system, plugin actions, series, note blocks,
  job system, meta schemas, plugin API
- Update sidebars with new navigation entries
- Verify all technical claims against codebase"
```

---

## Concurrency Map

```
Time →

Phase 1:  [====== Coach: style guide ======]
Phase 2:  [=== Summarizer A ===][=== Summarizer B ===]  (parallel with Phase 1)
Phase 3:         (wait for 1+2)  [== Checker A ==][== Checker B ==]  (parallel)
Phase 4:               (wait for 3)  [=== Writer A ===][=== Writer B ===]  (parallel, coach reviews)
Phase 5:                                    (wait for 4)  [= Hallucination Check =]
Phase 6:                                                       (wait for 5)  [= Final =]
```

Phases 1 and 2 run in parallel. Everything else is sequential with dependencies.
