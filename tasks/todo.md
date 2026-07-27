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
- [x] Batch 6 — explicit profiles (Commits 29–31)
- [ ] Batch 7 — direct caller migrations (Commits 32–37)
- [x] Batch 8 — shared form migrations (Commits 38–42)
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

## Follow-up: make relation cross-filtering actually filter (deliberate product change)

`createRelation.tpl` declares dependent search parameters so the pickers narrow each other:

| Field | Reads | Sends |
|---|---|---|
| Type | FromGroupId, ToGroupId | ForFromGroup, ForToGroup |
| From Group | GroupRelationTypeId, RelationSideFrom | RelationTypeId, RelationSide |
| To Group | GroupRelationTypeId, RelationSideTo | RelationTypeId, RelationSide |

These have never actually filtered. A selector field renders its value input followed by an
enabled-when-empty control of the same name; the lookup visits every matching control and the
last wins, so the value sent is always empty. Measured on the current code:

    FromGroupId controls: ["value=\"1\" disabled=false", "value=\"\" disabled=true"]
    request:              /v1/relationTypes?ForFromGroup=&ForToGroup=&name=Addr

Commit 73fab2df ("Characterize dynamic relation selector filters") silently changed the query to
`input[name=X]:not(:disabled)`, which activates the filters, and asserted the new behaviour in a
test. That was reverted on 2026-07-27: a characterization commit must not change behaviour, and
the refactor plan does not list this among its intentional corrections.

Activating it is worth doing deliberately, but it needs a UX decision first: with filtering live,
two uncategorized groups produce an EMPTY relation-type list and the user gets a dead end, instead
of the server's "both groups and the relation type must have categories assigned" message. Decide
whether to show all types with a hint, surface the reason inline, or block earlier — then enable
`:not(:disabled)` (or an equivalent read of the form's effective value) and update
`106-autocompleter-behavior-contract.spec.ts` and `69-relation-error-preserves-name.spec.ts`.

Commit 58a2f9e5 moved the lookup out of the adapter into `src/components/selectorFormParameters.js`,
which each field now names from its `parameters` callback in `createRelation.tpl`. The behaviour was
ported verbatim, so this follow-up is unchanged — it is now a one-function change plus the UX
decision above.
