# Reduction review: outsider-winner deadlock + long-name overflow

## Diagnosis

### Bug 1 — huge filename leaks outside the Cluster card under reduced zoom

`public/index.css` renders the header meta line with `.card-meta-item { white-space: nowrap; }`
inside the flex card header. A Resource whose *name* is a long unbroken URL (a remote
download keeps its source URL as the name) gives the "Winner:" meta item content wider
than the card; flex `min-width: auto` + `nowrap` means the item cannot shrink, so it
overflows the card border. Zooming out narrows the card but not the nowrap content, so
the leak grows.

### Bug 2 — external ("Outside the Extent") winner cannot be ejected or demoted: catch-22

In `application_context/resource_reduction_override.go`:

- `promoteMember` refused any promotion while the current Winner was out-of-Extent
  (`ErrReductionWouldDemoteOutsider`), with the message "…eject it instead".
- Ejecting the Winner is refused (`ErrReductionEjectWinner`), and the template hides
  the Eject button on the Winner.

So with an outsider Winner there was **no legal move**: you cannot eject it (it is the
Winner), and you cannot promote past it (refused). The two in-Extent duplicates could
never be merged. The refusal's own advice ("eject it instead") pointed at a door that
did not exist.

## Plan

- [x] TDD: rewrite `TestPromotingPastAnOutOfExtentWinnerIsRefused` to assert the new
      behaviour (promote succeeds; the outsider is auto-ejected with a dedicated
      reason; it is out of `LoserIDs()`), and add a restore-refusal test. Ran red
      (undefined `EjectReasonOutsiderDemoted` / `ErrReductionRestoreOutsider`).
- [x] `models/resource_reduction_model.go`: add `EjectReasonOutsiderDemoted = "outsider-demoted"`.
- [x] `promoteMember`: instead of refusing, auto-eject every un-ejected out-of-Extent
      member (in practice the Winner) with `EjectReasonOutsiderDemoted`; removed
      `ErrReductionWouldDemoteOutsider`.
- [x] Restore guard: refuse restoring an out-of-Extent member
      (`ErrReductionRestoreOutsider`, 400 in `error_status.go`) — restoring one would
      make it a Loser, which the "may never lose" rule forbids.
- [x] Template: "Put back" button only for in-Extent members.
- [x] CSS: scoped to `.reduction-cluster`, let the meta item shrink and break the
      Winner link with `overflow-wrap: anywhere` (the `.card-title` precedent, Finding 148).

## Review

Verified by automated tests and against a live ephemeral server:

- `go test --tags 'json1 fts5' ./...` — all green (after restoring `docs/todo.md`,
  which is a tracked findings ledger that `internal/arch/findings_coverage_test.go`
  parses; the plan first written there was moved to this file).
- Live scenario on an ephemeral server: three resources sharing one hash, the
  outsider (long-URL name) winning on "created first", extent = the two locals.
  - Promoting a local member past the outsider winner now succeeds; the outsider
    flips to Ejected + Outside the Extent, is out of the Losers, and gets no
    "Put back" button; the other local stays a mergeable Loser.
- CSS verified load-bearing in the same DOM: with the rules, the meta item stays
  inside the card; with them neutralised (injected `!important` overrides), the item
  exceeds the card's right edge by 1443px.
- `go test --tags 'json1 fts5' ./...` full suite green; `resource-reduction.spec.ts`
  8/8; a11y suite 204/204.
- One round of self-correction: the first version of the promote-past-outsider test
  used a one-member Extent (LoserIDs empty) and an assertion written for the two-member
  case; the full-suite run caught it because the earlier `-run 'Reduction'` filter
  never matched the test's name. The test now mirrors the reported scenario — two
  in-Extent copies under an outsider winner — and asserts the other copy stays
  mergeable.

## GPT-5.6-sol review round-trip

- Round 1 found one MAJOR: `rejustifyAgainstWinner` relabelled an auto-ejected
  outsider's reason to `no-pair-to-winner` when the new Winner had no pair to it; the
  promotion after that (Winner paired with the outsider) then read the relabel as "a
  previous promotion's lapse" and restored the outsider into a Loser — the one path
  by which an apply could destroy a Resource outside the Extent. Fixed:
  `rejustifyAgainstWinner` now skips every non-Winner out-of-Extent member before any
  pair-based relabelling or restoration. Regression test
  `TestAutoEjectedOutsiderStaysEjectedAcrossPromotions` reproduces the
  three-promotion/pair-removal sequence (failed red before the fix).
- Round 1 MINOR (redundant Eject button on an already-ejected outsider) fixed: the
  Eject branch now requires `not member.Ejected`. Deliberately not taken: exposing
  "Make Winner" for an ejected outsider — the server accepts it safely, but the UI
  deliberately does not offer re-instating an outsider as Winner.
- Round 2: no MAJOR findings, no MINOR findings, no regressions. Merge verdict: OK.
