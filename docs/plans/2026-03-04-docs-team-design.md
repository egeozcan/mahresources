# Documentation Team Design

## Goal

Bring docs-site documentation to 100% accuracy and coverage with zero AI-slop, using a coordinated agent team.

## Audience

Both end users/self-hosters and developers/contributors, with clear separation between user and developer content.

## Team Composition (8 agents)

| Role | Count | Agent Type | Purpose |
|------|-------|-----------|---------|
| Conductor | 1 | general-purpose | Orchestrate pipeline, assign work, final approval |
| Technical Summarizer | 2 | Explore | Read codebase, produce feature specs |
| Doc Checker | 2 | Explore | Compare specs vs docs, flag gaps/inaccuracies |
| Writing Coach | 1 | general-purpose | Establish style guide, review all writer output |
| Writer | 2 | general-purpose | Create/rewrite docs under coach direction |
| Hallucination Checker | 1 | Explore | Verify every claim against actual code |

Summarizers, Checkers, and Hallucination Checker are read-only (Explore agents). Writers and Coach need file write access (general-purpose).

## Pipeline

### Phase 1: Style Guide
Coach reads all 40 existing docs, identifies best-written ones, produces a style guide covering tone, structure, terminology, and anti-patterns.

### Phase 2: Feature Extraction
Two summarizers work in parallel:
- **Summarizer A**: Core entities (Resources, Notes, Groups, Tags, Categories, Series), relationships, metadata, search/filtering
- **Summarizer B**: Plugin system, download queue, job system, versioning, image similarity, configuration, deployment

Each produces structured feature specs.

### Phase 3: Gap Analysis
Two checkers work in parallel:
- **Checker A**: Specs from Summarizer A vs concepts/, user-guide/, api/ docs
- **Checker B**: Specs from Summarizer B vs features/, configuration/, deployment/ docs

Each produces: missing docs, inaccurate content, outdated info, AI-slop flags.

### Phase 4: Writing
Writers work in parallel on different doc batches. Coach reviews each doc before finalization. Revision cycles until coach approves.

### Phase 5: Verification
Hallucination checker verifies every finalized doc against actual code: endpoints exist, flags are real, examples work, descriptions match behavior.

### Phase 6: Final Review
Conductor spot-checks, updates sidebars.ts, merges everything.

## Scope

### New docs (7)
1. `features/plugin-system.md` - Plugin architecture, Lua APIs, creating plugins
2. `features/plugin-actions.md` - Action system, form parameters, async jobs
3. `concepts/series.md` - Series entity, usage patterns
4. `concepts/note-blocks.md` - Block types, reordering, custom blocks
5. `features/job-system.md` - Unified async job system
6. `features/meta-schemas.md` - JSON schema validation per category
7. `api/plugins.md` - Plugin API endpoints

### Existing docs audit (40)
All existing docs reviewed for accuracy, AI-slop, consistency with style guide. Rewritten where needed.

### Supporting artifacts
- Style guide (internal, for writers)
- Updated sidebars.ts

## AI-Slop Definition

Zero tolerance for:
- Filler phrases ("In this section, we will explore...")
- Hedging ("It's worth noting that...")
- Generic transitions ("Let's dive into...")
- Unnecessary adverbs ("simply", "easily", "just")
- Restating what the reader already knows
- Vague claims without specifics

Required: direct, concrete, specific language. Every sentence earns its place.

## Coverage Policy

Document everything in the codebase, including experimental features (marked with warnings).
