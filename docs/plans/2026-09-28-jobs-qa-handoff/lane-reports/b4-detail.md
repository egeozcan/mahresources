# b4-detail final report — INTERIM (wound down)

Branch `jobs-qa/b4-detail`, base bdafcb51, HEAD **014fccc5**. The worktree is clean and every change is committed (11 commits).

## Per issue

- **J4, fixed on the detail page.**
  - Root cause: the page rendered three fields only.
  - Fix: a new "Times" section lists accepted, scheduled, started, last resumed and finished, each with a relative age. It shows the time spent queued, running, paused and blocked, counting the current state from `stateEnteredAt`, and when the history is kept until (with a note when the Job is pinned). Times use `src/utils/localTime.js`, the /jobs cards' format, and the section names the reader's zone. The timeline, output expiry and scheduled text use the same format.
  - Tests: `src/components/jobDetail.test.ts` and `e2e/tests/jobs/job-detail-page.spec.ts`.
  - Not mine: the drawer row time was ruled b4-numbers'.
- **J5, fixed.**
  - Root cause: `safeJobOutputFilename` used the display label and dropped the extension.
  - Fix: `jobOutputFilename` builds label slug, UTC publish time, `jobs.ShortID` of the Job, then the stored extension (`.tar.gz` stays whole). A succeeded Job with no entity output but an available, unexpired artifact now offers it as its result, "Download exported archive". One rule covers both sides: `server/jobview/result.go` for the /jobs card and `jobCenter.js resultOutput` for the drawer.
  - Tests: `job_output_filename_test.go`, `TestAGroupExportArchiveDownloadsUnderAFileNameWithItsExtension`, `TestResultLinkForOffersAFinishedExportsFile`, the JS "file outputs" block, and E2E.
- **J8, fixed.**
  - `/job`: the server reads the Job as the viewer. Not found gives a 404 page with "Back to Job Center". A legacy queue or plugin-action handle redirects to its canonical Job. Any other read error still renders the page. Tests: `job_detail_page_test.go` and E2E.
  - `/jobs`: a bad filter answers 400 on the page itself, with the form, an inline message and "clear all filters" (`_statusKeepsPage` in `render_template.go`). The admin Owner/Actor selects are drawn before the filter is read. Tests: provider tests, `TestAnUnusableJobCenterFilterKeepsTheFilterForm` and E2E.
- **X7, fixed.** Every output link's accessible name starts with its visible words; entities and files are named in the visible text.
- **X9, fixed.** The title is `<title> (<State>) - Job <shortid>`, kept current by `syncDocumentTitle`. The page has one h1, the layout's, which also recovers after a failed server read.
- **X10, fixed.** Drawer titles and labels wrap, metrics use one column below 640 px, and below 30rem height the whole drawer scrolls.
- **X11, fixed.**
  - The drawer's header and footer are now divs.
  - The unnamed div lost its label.
  - "Advertised …" became "Job actions", and selectors use data attributes.
  - The visible screen-reader note is removed.
  - The axe landmark exemptions are removed from the tests.
- **X12, fixed.** `refuseOverModal` puts a visible status inside the blocking dialog, pinned to the bottom of the viewport. It is used by both the drawer and search. It leaves after 10 s or when focus leaves the dialog.
- **U11, fixed.** Each lineage entry carries its link (`AncestorLinks`/`SuccessorLinks`, API `relation`). Entries say "Retry of", "Continuation of", "Repeat of" and their reverses, plus state, acceptance time to the second and the short id.
- **L7 detail half, fixed.** The timeline reads pages of 200 up to 1,000, then offers "Show later events". It follows stream events for this Job and reads again when the stream catches up, one read at a time behind a generation fence. The phase clearing was already fixed on the base; a test now pins it. Event types read as words, and failed reads offer Try again.
- **Search focus follow-up, fixed.** The search input shows a ring over `outline-hidden`.

## Gates (HEAD 014fccc5)

- Go: `go-full-4.log` EXIT=0.
- vitest: `vitest-4.log`, 1711 passed.
- E2E all: `e2e-all-2.log`, 2330 passed and 0 failed; `.last-run.json` says passed.
- Postgres: the subset for jobs, api_tests and application_context was started and then stopped by the wind-down, so it has **not been run**. `jobs/query.go` changed Go logic only, not SQL.
- Earlier run (e2e-all-1): the compare `:456` flake was already reported to you (125/125 alone on this branch). `plugin_commands TestShutdownTimeoutLeavesRunningGroupForRecovery` failed once under load and passed 3/3 alone.

## pi rounds

| Round | Output | P0/P1 | Handled |
|---|---|---|---|
| 1 | 4.4 KB | 5 | All fixed (short id, per-entry links, fixed refusal, heading recovery) |
| 2 | 3.9 KB | 3 | All fixed (catch-up timeline read, lineage keys, admin selects) plus the P2 (timeline retry) |
| 3 | 2.7 KB | 1 | P1 declined, ruled P2 by you; P2 fixed (refusal on focus leave) |
| 4 | not run | — | The first attempt hit the Codex usage limit; the rerun was stopped by the wind-down. The prompt is ready at `pi-prompt-4.txt` |

The trend is 5, 3, 1, and each new finding was in code or scenarios added by earlier rounds. No findings from round 3 are left unhandled.

## Declined

- **Short-id title collisions (round 3 P1).** A collision needs the same title, the same state and the same last 32 random bits: 1 in 4.3e9 for any given pair. The full id is shown under the heading, and you ruled it P2.
- **Short id in the h1 (round 1, partly).** The id, state and lineage sit directly under the heading.

## Docs

- `docs-site/docs/features/job-system.md`: new "The Job page" section, the drawer's refusal and small viewport, and the export result link and API `relation`.
- `docs-site/docs/user-guide/managing-resources.md`.
- `openapi.yaml` regenerated. The partial-schema generator now reads embedded structs' fields.

## Merge notes

- `/job` now 404s for ids the server does not hold. Any spec that mocks the API for invented ids must use `e2e/helpers/job-page.ts gotoMockedJobPage`; 9 specs were moved to it.
- `displayJob.tpl` changes that overlap other lanes:
  - I removed the header h1, moved "Scheduled for" into Times, and added the `outputCountText` plural.
  - b4-numbers touched the state pill, kind, "Started from" and "Failure type".
  - b4-list's X5 touches three `sayLifecycle` sites in `jobCenter.js`; none of my hunks are there.
- `jobListContextProvider`: the admin options block moved above the filter read (3f983fe6). This overlaps b4-list's J9 changes in that function.
- b4-kinds' K1 output branch is clear of `jobOutputFilename`.
- The "retried" event from b4-list's F2 renders as "Retried" through `timelineEventLabel`.

## Lessons

- A server-side read on a page that specs used to mock through the browser API breaks every spec that invented ids. Budget a helper for that before choosing the design.
- A stream that starts at the head needs every derived view (timeline, not only the snapshot) re-read at catch-up.

## Outside my lane

- The compare reduced-motion flake (b4-flakes).
- `jobs/list_page_test.go` and `list_page_pg_test.go` are not gofmt-clean on base.
- ETA wording "about under 1 s left" and raw byte counts in progressbar names (L3-16 items 6 and 7) belong to b4-numbers.
