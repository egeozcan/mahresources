# b4-list final report: INTERIM (wound down)

Branch `jobs-qa/b4-list`, HEAD `21a105e1`. Base is `bdafcb51`. The worktree is clean. Logs are in `$SP/lanes/b4-list-tmp/`.

## Per issue

- **J1: fixed.**
  - Cause: every download without a chosen name was titled "Download from host", and search matched the summary's JSON text.
  - Fix, title: the last URL segment is used only when it names a file (a stem plus a 1 to 10 character alphanumeric extension). The file is kept in `summary.file` even when a name was chosen.
  - Fix, search: search walks the summary's values, with a text prefilter only where it is sound. Summaries are stored canonically.
    - Older rows on SQLite that hold a `\/` or a `\u` escape are always walked (b94be535).
    - On PG, `summary` is jsonb, so it is already sound there.
  - Fix, legacy link: an old `/downloads?URL=` link searches for the URL's file name, or its host.
  - Tests:
    - `TestADownloadIsTitledByTheFileItFetches` covers long and JWT-like tokens.
    - `TestListSearchMatchesSummaryValuesNotItsSyntax`, on SQLite and PG, covers escapes, numbers and an older-release row.
    - Legacy URL tests and job-center E2E.
- **J2: fixed.** A summary shows as text, or as labelled `dt`/`dd` fields.
- **J9: fixed.**
  - Back no longer restores the previous page's checkboxes (autocomplete off, plus a pageshow reset).
  - A search term's spaces are trimmed.
  - Origin is now a set of checkboxes.
  - With auth off, a successor Job records no owner (`Access.Implicit`), and the Owner and Actor filters are hidden.
  - `/downloads` date ranges now cover whole days.
- **J10: fixed.**
  - The summary panel and the export form were added. Export is offered only for filters the export can seal; otherwise the page says why.
  - The export records its range and filter in the title, CSV and JSON.
  - The 91d message was already fixed on base, pinned by `job_api_refusals_test.go:51`.
- **U9: fixed.**
  - Cause: `selectableItem` assigned fields it did not declare, so in nested Alpine they landed on the outer scope and every card's checkbox pointed at the last card.
  - Cancel and Retry are now `Bulk: true` in every adapter. "Download now" stays single-Job.
- **U10: fixed.** The navbar folds its links behind the menu button when they don't fit (ResizeObserver).
- **U14: fixed.** Bulk results name Jobs by title with links, and the selection uses the list's own noun.
- **L7 (/jobs half): fixed.** Progress frames patch the cards and time left counts down. A frame older than the drawn card is ignored.
- **X5: fixed.**
  - There is one shared ledger. A page hands each change, with the card as it was before, to the drawer's `hearFromPage`, which says it once. The ledger dedupes by Job, state and version.
  - The drawer hands back Jobs it does not follow, or cannot say yet because its stream is still catching up; the page says those with the failure reason.
  - Tests: vitest, and `auth/job-announcements.spec.ts` on both scopes with the drawer's Finished group capped at one. The capped cases are red on the pre-fix source.
- **X8: fixed.** Motion-safe pulse.
- **F1:** recorded as a documented known limit (the comment on `trackResourceCompletion`, and docs).
- **F2: fixed.** A `retried` event on the ancestor Job. Read-only viewers do not see it.
- **F3: fixed.**
  - The cursor moves to the allocator head after the caught-up marker.
  - A viewer promoted while the stream is open is sent only what is published after the promotion. This is by design and accepted by you; it is pinned by `TestCanonicalJobSSEPromotionSendsWhatIsPublishedAfterIt` and explained in the code comment and job-system.md.

## pi rounds

| Round | Blocking | Result |
|---|---|---|
| 1 | 8 | 7 fixed, 1 declined (F3 promotion) |
| 2 | not run | Codex usage limit; the prompt is ready at `pi-prompt-2.txt` (needs its commit list refreshed from HEAD before rerunning) |

Trend: only one round has run. No findings are open that I know of.

## Declined

- **F3 promotion.** Sending the pre-promotion history as live events would announce old outcomes as news. The lists are the record.
- **Numeric-term prefilter.** SQLite rounds reals to 15 significant digits and renders large integers in exponent form, so a sound text test isn't cheap. You accepted the timings.

## Gates on 0486ab89

The commits after it change only docs.

| Gate | Log | Result |
|---|---|---|
| Go, SQLite | `go-all-3.log` | EXIT 0 |
| Go, Postgres | `go-pg-2.log` | EXIT 0 |
| E2E all | `e2e-all-2.log` | 2337 passed, 5 skipped |
| E2E, Postgres | `e2e-pg-1.log` | 2338 passed, 4 skipped |
| vitest | `vitest-3.log` | 1709 passed (3c84583e; no JS changed after it) |

Search timings at 1M Jobs, fastest of 5, before/after:

| Term | SQLite (ms) | PG (ms) |
|---|---|---|
| needle | 241 / 361 | 1358 / 1372 |
| scheme | 154 / 913 | 1245 / 3193 |
| numeric | 252 / 856 | 1301 / 2394 |

## Docs changed

- `job-system.md`: announcements, the widened stream, search timing.
- `download-queue.md`, `managing-resources.md`, `authentication.md`.
- CLAUDE.md: no-auth attribution.
- `jobs_summary_export.md` and the generated CLI page.

## Follow-ups and outside-lane notes

- **Nested-Alpine audit.** Undeclared fields assigned in `init()` land on the outer scope. Components to check: `confirmAction`, `resourceUpload`, `mrqlBar`, `codeEditor`, `blockEditor`, `globalSearch`. The `/mrql` cards are nested inside `mrqlEditor`; an E2E was added.
- `jobs/list_page_test.go` and `list_page_pg_test.go` are not gofmt-clean on base.
- **Lessons:**
  - The git stash is shared across worktrees.
  - The E2E harness rebuilds the bundle, so a red check must revert the source.
  - Heredocs rewrite `\u` escapes.
  - `types.JSON` is jsonb on PG despite the `type:json` tag.

## Merge notes

- `job.tpl` progress data hooks overlap with b4-numbers.
- In `jobListContextProvider`, my auth-off condition gates both the account options and `nameJobRowOwners`.
- The shared `jobAnnouncements.js` API is now `followJobAnnouncements({hear})` and `tellDrawerOfJobs`.
