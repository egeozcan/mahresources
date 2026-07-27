# Headless Selector Core Refactor

Source: `docs/plans/2026-07-26-headless-selector-core-refactor.md`

## Approved execution corrections

- [x] Review Tasks 18–20 as one green batch, with queue foundations implemented before token/confirmation consumers.
- [x] Move virtual-create navigation coverage from Task 14 into the creation batch.
- [x] Treat a non-creatable multi-tag selector as the ordinary multi-entity profile with tag-specific source/presentation configuration.
- [x] Include browser + CLI E2E and PostgreSQL verification when Docker is available.
- [x] Execute autonomously in nine reviewable phase batches.

## Execution batches

- [x] Batch 1 — behavior contract (Commits 1–4)
  - [x] Atomic single-selection replacement
  - [x] Rapid-query stale-result handling
  - [x] Create confirmation/current-query gating
  - [x] Dynamic relation-selector parameters
- [x] Batch 2 — implicit integrations (Commits 5–10)
  - [x] Scoped selector registry
  - [x] Legacy registration
  - [x] MRQL read migration
  - [x] MRQL write migration
  - [x] Chip removals through commands
  - [x] External replacement through reset commands
- [x] Batch 3 — headless foundations (Commits 11–17)
  - [x] Types/source/in-memory adapter
  - [x] Lifecycle/snapshots/subscriptions
  - [x] Selection/removal/replacement
  - [x] Open/navigation
  - [x] Search orchestration
  - [x] Debounced source
  - [x] HTTP source
- [x] Batch 4 — creation and contract completion (Commits 18–21)
  - [x] Creation queue foundation
  - [x] Create candidates/token precedence
  - [x] Confirmation
  - [x] Unified queue and contract flows
- [ ] Batch 5 — Alpine compatibility delegation (Commits 22–28)
- [ ] Batch 6 — explicit profiles (Commits 29–31)
- [ ] Batch 7 — direct caller migrations (Commits 32–37)
- [ ] Batch 8 — shared form migrations (Commits 38–42)
- [ ] Batch 9 — compatibility removal/docs/final verification (Commits 43–46)

## Verification

- [ ] Frontend unit tests and build pass.
- [ ] Focused selector/MRQL/lightbox/relations Playwright tests pass.
- [ ] Full Go suite passes with `json1 fts5`.
- [ ] Browser and CLI E2E pass together.
- [ ] PostgreSQL Go and E2E checks pass when Docker is available.
- [ ] DeepSeek and GPT-5.6-Terra browser validation completed after significant integrations.
- [ ] Fresh final code review completed.

## Review

Pending implementation.
