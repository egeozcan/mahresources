# Category template visibility on cards

## Success criterion

The existing Section Visibility controls suppress matching built-in sections on
group, resource and note cards while custom summaries and avatars still render.
Mixed-category lists and cards nested inside detail pages resolve each card's own
configuration. Missing, empty, partial and invalid configuration retain the
existing resolver defaults.

## Scope and tradeoffs

The user chose to extend existing controls to list/card views. Reuse SectionConfig
and its existing boolean keys; do not add template directives or a separate card
configuration. This intentionally makes an existing false setting affect cards
as well as detail pages. Custom slots remain independent of built-in visibility.

| Card | Existing controls to apply |
| --- | --- |
| Group | description, tags, timestamps |
| Resource | description, tags, owner, fileSize, categoryLink, previewImage |
| Note | content (description preview/editor), tags |

Resource CustomAvatar remains visible when categoryLink is false. Resource tag
editing follows tags visibility, but plugin actions remain available. Category
settings never hide selection controls, titles, relation descriptions, or plugin
actions. Detail-only controls keep their existing detail behavior. All callers
of the shared card partials inherit the change, including nested cards and the
group text/resource simple list views. Tables, timeline rows, hover cards,
lightbox rendering, custom MRQL templates and public shares retain their current
separate rendering contracts.

## Implementation and verification

- [x] Record baseline Go and full browser/CLI E2E results before changing code.
- [x] Add regression tests proving built-in sections disappear and replacement
      CustomSummary content survives; verify defaults and per-card isolation.
      Run them against the existing implementation and observe failure.
- [x] Expose the existing section resolvers through a small Pongo2 filter and use
      local card configuration in the three shared partials.
- [x] Load missing carrier associations for nested cards using batched preloads
      on their existing read queries. The nested-card regression exposed missing
      ResourceCategory on group-owned resources; inspect the other detail-card
      readers for the same gap. Do not add per-card database lookups.
- [x] Update Section Visibility help and custom-template documentation with the
      card mapping and a description replacement example.
- [x] Verify focused tests, the full Go suite and browser/CLI E2E suite, CSS scan,
      and Postgres Go/E2E tests using ephemeral servers.
- [x] Review the final diff, record results, and complete this checklist.
- [x] Run fresh subagent standards and specification reviews after implementation,
      fixing and repeating as needed until a review finds zero P0/P1 issues.

## Review

Implementation reviewed against every caller of the three shared card partials.
Carrier preloads now cover category, tag, series, relations and entity detail
pages, including both similarity-query branches. They use the caller's existing
DB handle and association scope; existing collection limits remain in place.

Observed red/green evidence: native card descriptions remained despite false
visibility settings; missing nested carriers also caused custom summaries to
disappear. Eight browser regressions and seven reader regression subtests now
pass. CSS scan checks and the application build pass. Final verification:

- `go test --tags 'json1 fts5' ./...`: passed.
- `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`: passed.
- `npm run test:with-server:all`: 2,462 passed, 5 skipped.
- Full Postgres browser/CLI E2E: 2,461 passed, 2 passed on retry, 4 skipped.
  The two timing failures were examined: mass edit applied before asynchronous
  tag creation selected the operation, and the video focus test tabbed before
  Alpine's delayed trap activation. The tests now await those real prerequisites
  while preserving their existing behavior assertions. Repeated Postgres checks
  passed without retries: 18 mass-edit cases and 10 video-focus cases. Both
  modified specs also passed on SQLite: 17 cases, no retries.
- Eight new card E2E cases passed on both SQLite and Postgres; seven new reader
  regression subtests passed on SQLite.
- `./scripts/css-scan-test.sh` and `git diff --check`: passed.

The working tree's pre-existing scheduled-task lock deletion and video-tools
plan changes were preserved. No dependencies or schema changes were required.

Post-implementation review round 1 used two independent subagents, comparing
the feature's tracked edits and new files against
`ce387f52e8e1806e670034b443e42c447a292853`. The standards review and the
specification/correctness review each reported no findings: P0 0, P1 0, P2 0,
P3 0. Both traced the shared-card callers and checked per-card configuration,
carrier availability, query scope/limits, retained custom content and regression
coverage. The requested zero-P0/P1 review gate is satisfied.
