# Handover — Headless Selector Core Refactor

> **Date:** 2026-07-27, ~13:20 UTC
> **Branch:** `refactor/headless-selector-core`
> **Worktree:** `/Users/egecan/Code/mahresources/.worktrees/headless-selector-core`
> **Plan:** `docs/plans/2026-07-26-headless-selector-core-refactor.md`
> **Ledger:** `/Users/egecan/Code/mahresources/.superpowers/sdd/2026-07-26-headless-selector-core-refactor/progress.md`
> **Report dir:** Same path as ledger — batch reports are `batch-*-report.md`
> **Server:** Ephemeral mahresources running at `http://127.0.0.1:8291` (start this session's first task with `cd /Users/egecan/Code/mahresources/.worktrees/headless-selector-core && npm run build && ./mahresources -ephemeral -bind-address=:8291 -max-db-connections=2` if not found)

## Project overview

We are refactoring the shared Alpine `autocompleter` into a framework-independent headless selector module (`src/selector/`), with explicit profiles, atomic selection-change contracts, and a scoped registry for MRQL integration. The refactor is structured in 46 numbered commits across 9 phase batches.

## What is complete (Phases 1–6)

All of Phases 1–6 are done and passed independent spec reviews:

| Phase | Commits | What |
|-------|---------|------|
| **Phase 1** (Commits 1–4) | `3dc2aae9..73fab2df` | Browser behavior contract (replacement, stale queries, create confirmation, dynamic relation params) |
| **Phase 2** (Commits 5–10) | `f66f0b1d..90938c36` | Scoped selector registry, MRQL read/write migration to registry, chip removal through commands, external sync through reset |
| **Phase 3** (Commits 11–17) | `7bd64b98..df692a98` | Headless core TypeScript: types, in-memory source, lifecycle, snapshots, atomic selection, navigation, search orchestration, debounce decorator, HTTP source adapter |
| **Phase 4** (Commits 18–21) | `f046b3bf..13954fb7` | Creation queue, typed-token precedence, create-candidate gating, create-confirmation state, typed creation outcomes, destruction abort/discard, contract end-to-end flows |
| **Phase 5** (Commits 22–28) | `56fed24e..b80e9e31` | Compatibility adapter extraction, config normalization, core delegation of selection → navigation → search → creation → lifecycle/teardown. Fixes for stale Enter, close-during-search, silent announcement, inline partial match |
| **Phase 6** (Commits 29–31) | `aa1f7606..f1962aab` | Explicit profiles (single/multi entity, general creatable/series, lean tag field with usage sort/delimiter, tag-editor with optimistic association persistence, external-reset rollback guard) |

## What is in progress (Phase 7)

Batch 7A (Commits 32–34): **IMPLEMENTED, FINAL REVIEW PENDING**

- Commits: `bad5d89d`, `24ca626e`, `13c21559`, `cddfc633`
- Compare selectors, block-editor query selector, paste-upload selectors migrated to explicit profiles.
- **Fix Round 1** (commit `cddfc633`) added compare resource-navigation browser coverage.
- **Spec compliance: PASS, Task quality: CHANGES REQUIRED by last review – Fix Round 1 completed but final scoped re-review has NOT been run.**
- Tests: 107 unit, 61 focused browser – all passing (`npm run test:unit && npm run build-js`).

**Action: Run scoped re-review of Batch 7A Fix Round 1 before proceeding.**

### Unresolved intercom message from a prior subagent

An orphan intercom message from a prior session exists on target `subagent-worker-f1e36ab4-1` asking:

> "Commit 17's required public HTTP adapter configuration/API is not specified by the brief or prior contracts. I can implement a conservative `HttpSelectorSource` + `HttpSelectorSourceConfig` (`searchUrl`, optional `createUrl`, `mapOption`, optional dynamic `parameters`, injected `fetch`, query key `name`, create POST JSON `{Name, ...parameters}`), matching existing dropdown behavior. Please approve or provide expected API names/shape."

**Resolution:** Commit 17 was already implemented (ffd16f4c) and reviewed – this was a stale question from an earlier agent. The HTTP source at `src/selector/httpSelectorSource.ts` follows exactly that API shape. Ignore/expire the intercom message.

## What remains (Phase 7B through Phase 9)

### Phase 7B (Commits 35–37): Lightbox/picker migrations

- **Commit 35:** Entity-picker filter selectors → dynamic entity-selector profile. Runtime endpoints, single/multi policy, filter-store updates, close-time reset, unregister on close.
- **Commit 36:** Lightbox quick-slot selectors → non-creatable multi-tag profile. Usage sorting, exclusion of configured slot tags, autofocus, Escape, slot persistence (Batch 5 adapter already handles Escape/location).
- **Commit 37:** Lightbox tag editor → tag-editor profile with resource association adapter. Optimistic detail-cache updates, suggested-tag invalidation, recent-tag tracking, failure announcements, navigation-safe external reset. Remove legacy pending/failure tracking after migration.

Brief already written at: `batch-7b-brief.md`

### Phase 8 (Commits 38–42): Shared form surface

- **Commit 38:** Teach shared form partial to accept explicit profiles with stable metadata while retaining legacy compatibility.
- **Commit 39:** Migrate single-value shared fields (owner, category, resource-category, note-type, relation-type, merge-winner).
- **Commit 40:** Migrate non-tag multi-value shared fields (group, note, resource, category, relation-type filters).
- **Commit 41:** Migrate shared tag pickers/fields to non-creatable multi-entity or tag-field profiles. Standardize on lean suggestion source.
- **Commit 42:** Migrate dynamic relation fields to profile parameter callbacks; remove legacy filter parsing.

### Phase 9 (Commits 43–46): Compatibility removal and final verification

- **Commit 43:** Migrate or document remaining aggregate selection-event consumers.
- **Commit 44:** Remove legacy configuration options, writable raw selected-results exposure, and state aliases.
- **Commit 45:** Delete superseded implementation (legacy search, queue, selection, watcher logic). Remove tests that only cover obsolete internals.
- **Commit 46:** Document profiles/source/registry contracts; run formatting, type checking, unit tests, full builds, **Go tests**, **full browser+CLI E2E suite**, and **PostgreSQL suite** when Docker is available.

## Test discipline

- Each commit (or green batch for Commits 18–20) must pass: `npm run test:unit`, `npx tsc --noEmit`, `npm run build-js`, focused Playwright coverage for affected callers.
- No skipped tests; no `test.skip` stubs.
- After significant integration points (end of Phase 2, 5, 7, 8, 9): run `cd e2e && npm run test:with-server:all` and, when Docker is available, `cd e2e && npm run test:with-server:postgres`.
- The plan calls for DeepSeek and GPT-5.6-Terra browser validation after significant changes. These are informative but not blocking.
- E2E tests need a built server binary (`npm run build`) and an ephemeral server. Use `-max-db-connections=2` to reduce SQLite lock contention.

## Key files to know

| File | Role |
|------|------|
| `src/selector/types.ts` | Public selector contracts: state, commands, changes, options, keys |
| `src/selector/selectorCore.ts` | Headless core: lifecycle, selection, navigation, search, creation queue |
| `src/selector/entityFieldProfiles.ts` | Single/multi/creatable/tag field profiles |
| `src/selector/tagEditorProfile.ts` | Optimistic tag association profile |
| `src/selector/httpSelectorSource.ts` | Production HTTP search/creation source |
| `src/selector/selectorRegistry.ts` | Form-scoped selector registry |
| `src/components/legacyAutocompleterAdapter.js` | Alpine compatibility adapter (delegates to core) |
| `src/components/legacyAutocompleterConfig.ts` | Pure config/normalization |
| `src/components/dropdown.js` | Legacy Alpine component (directs Alpine registration and event binding) |
| `src/components/dropdownRegistry.test.ts` | Registry/adapter integration tests |
| `src/selector/selectorCore.test.ts` | Core module interface tests |
| `src/selector/index.ts` | Public module exports |

## Post-handover checklist

- [ ] Run Batch 7A scoped re-review (reviewer subagent with diff `cddfc633..13c21559`)
- [ ] If clean: Batch 7B (Commits 35–37) via Terra worker
- [ ] Batch 8 (Commits 38–42) via Terra worker
- [ ] Batch 9 (Commits 43–44 compatibility removal + 45 deletion + 46 final verification)
- [ ] Full browser + CLI E2E sweep (`cd e2e && npm run test:with-server:all`)
- [ ] PostgreSQL sweep if Docker available (`cd e2e && npm run test:with-server:postgres`)
- [ ] Final whole-branch code review
- [ ] Delete plan workspace (`rm -rf /Users/egecan/Code/mahresources/.superpowers/sdd/2026-07-26-headless-selector-core-refactor/`)
- [ ] Delete worktree (`git worktree remove /Users/egecan/Code/mahresources/.worktrees/headless-selector-core`)
- [ ] Merge into master
