# Job Center list on the standard list layout

`/jobs` rendered nothing on the server: `JobCenterListContextProvider` passed no
data and hid the sidebar, and `jobCenter.js` fetched `/v1/jobs`, split rows into
Overview sections, drew its own filter form, cursor "Load more", selection and
bulk bar. Every other list renders its rows on the server into
`section.list-container`, filters through a GET form in the sidebar, pages with
`pagination.tpl` and selects through `selectableItem` + `bulkSelection`. This
plan moves `/jobs` onto that layout.

Decisions (grilling session, 2026-09-24):

- Server-rendered rows; no-JS readable. The `/job` detail page, the header jobs
  panel and the legacy `/downloads` redirect are out of scope.
- The Overview is gone. Its groupings become sidebar quick filters with counts:
  Needs attention, Active, Finished, Pinned by me (CONTEXT.md: Active Job,
  Finished Job, Needs Attention).
- Counts are unwindowed and match the rows the list shows with that slice chosen.
  A state group's count applies every current filter except State; Pinned by
  me's applies every one except Pinned, State included, because the quick
  filters combine: Pinned while Needs attention is chosen is the pinned Jobs
  that need attention.
- Default list: Jobs the viewer has not dismissed, newest accepted first. Pinned
  Jobs are not floated.
- Typed filters. The existing query parameter names are kept so the legacy
  translator, the panel links and bookmarks keep working. `view=all` is ignored.
- Keyset Previous/Next inside the standard `pagination.tpl` nav, without page
  numbers: offsets drift on a list that changes while it is read.
- Live refresh: one SSE connection. Debounced refetch of the current URL, then
  morph the list, the quick filters and the pagination nav.
- Bulk: standard selection plus one `job` entry in the `listviews` bulk-action
  catalog (ADR 0004). A `jobCommands` component offers the commands every
  selected Job advertises and reports per-Job outcomes. Commands are read
  lazily per selected Job, as today; advertising them for every rendered row
  would cost an adapter call per row on every render and refresh.
- Included: saved searches, `listEmpty`. Excluded: MRQL bar, custom list
  header/footer, Mass Edit, plugin list slots, the display-mode switcher.

## Tasks

- [x] `jobs.Service`: reverse keyset (`ListBefore`), `Page.Prev`, and an
      unwindowed `CountByState` sharing `applyFilter` + command filter. Go tests first.
- [x] Shared job query parsing and presentation (`server/jobview`): filter
      parsing, list cursor codec, result link for a succeeded Job. `api_handlers`
      delegates to it; template providers cannot import `api_handlers`.
- [x] Context provider: parse filters (default `dismissed=false`), cursor or
      `before`, list, quick-filter counts and links, Kind and command options,
      keyset pagination, result links.
- [x] Templates: `listJobs.tpl` on the standard blocks, `partials/job.tpl` card,
      sidebar form from `/partials/form/*`, `bulkActions/jobCommands.tpl`.
- [x] `listviews`: saved-search view `/jobs`, bulk catalog entry for `job`.
- [x] JS: `jobList.js` (live refresh + bulk commands). Strip list-only code from
      `jobCenter.js` (the detail page and the panel still use it).
- [x] E2E: server-rendered rows, filters, quick-filter counts,
      Previous/Next, live refresh, bulk command, result link, a11y, auth visibility.
- [x] docs-site: user guide (the job-system page described nothing the change removed).
- [x] Build; Go, vitest, browser + CLI E2E, Postgres.

## Review

- Filter controls: Kind is a checkbox group rather than a select, because the
  filter takes several Kinds and a single select would drop a multi-Kind URL.
- `dismissed=any` is a page-only value. The page's default list is the
  undismissed one, so a missing parameter cannot also mean "any" as it does on
  the API.
- The sidebar form submits empty fields; the page drops them before parsing,
  since the API parser refuses `command=` and an empty origin.
- Acceptance bounds use `datetime-local` inputs, not the shared date input: a
  bookmark or a legacy link can name an instant, and a date input widened it
  to a whole day on the next submit (pi review). The shared parser, and
  therefore `/v1/jobs`, also accepts server-local `YYYY-MM-DD`,
  `YYYY-MM-DDTHH:MM` and `YYYY-MM-DDTHH:MM:SS`, each naming its whole day,
  minute or second, both ends inclusive.
- The card draws a progress bar only for Active or succeeded Jobs, or when the
  Job carries progress data. The old card drew "Working" on every failed Job.
- Live-refresh announcements compare each row's state before and after the
  morph, because a v2 stream message often names a Job without a snapshot.
- Rendering reads openable outputs per succeeded row for the result link:
  measured 13ms for a page of 45 succeeded Jobs against 7ms for none, so it is
  not batched.

### pi review, round 1 (gpt-6-astra)

Eight findings, all confirmed and fixed: stream changes replayed during
catch-up now reconcile once at the boundary; the bulk bar's cached commands
are keyed on the row's pin state as well as its version; an emptied page keeps
a Previous link; bulk outcomes survive the refresh their command triggers; a
superseded read no longer leaves the bar loading; a failed bulk request shows
its reason; every origin stays in the form; acceptance bounds keep their time
of day.

### pi review, round 2 (gpt-6-sol)

Four findings, all confirmed and fixed. The datetime inputs were read in the
server's zone; `jobFilterTimes` now shows the bounds in the reader's zone and
submits an edited bound as an RFC 3339 instant covering its whole minute, with
the server-local reading left as the no-JavaScript fallback. An untouched
bound is resubmitted as the exact instant it arrived as, so a bookmark no
longer widens to the minute or loses fractional seconds. The command filter's
keys moved beside the selectors (`application_context.JobCommandFilterKeys`),
gaining the plugin-command `inspect` and `retry-import`, and a key the URL
names that the list lacks is kept. The bulk bar offers nothing, and runs
nothing, while a selected row is newer than its cached detail.

### pi review, round 3 (gpt-6-sol)

Four findings, all confirmed and fixed. An edited end bound is written to the
last nanosecond of its unit, as the server reads one, rather than the last
millisecond. A failed live refetch retries after five seconds instead of
leaving the page stale until the next event. A Kind the URL names that no
adapter registers any more stays offered, as a command key does. Card times
are localized to the reader's zone after load and after each refresh, so the
list, its filter inputs and the detail page agree on the clock; the server's
rendering remains the no-JavaScript fallback.

### pi review, round 4 (gpt-6-sol)

Two findings. An active quick filter's count describes its slice while its
link clears it, as a tag chip's does; that is the intended behaviour, and the
wording that claimed "the rows the link opens" was what was wrong — corrected
in the code comment, the user guide and this plan. Card times without
JavaScript mixed zones (Accepted in UTC, Started and Finished server-local);
all three are now formatted the same way.

### pi review, round 5 (gpt-6-sol)

Two findings, both confirmed and fixed. A bulk command whose refresh removed
every selected card hid the bar and its results with it; the summary is now
also shown in a visible notice on the page. A refetch redirected to the login
page was taken for the list and stripped the quick filters and pagination; a
redirect now stops live updates with a visible "reload" status, and a page
without the list is a failed refresh that retries.

### pi review, round 6 (gpt-6-sol)

Two findings. A confirmed bulk command posted the ids captured before the
dialog opened, although a live refresh could remove a card while it was open;
the selection is now rechecked after confirmation and a changed one is refused.
Declined: "Clear filters was dropped". No standard list has a clear control in
its sidebar, and matching them is the point of the change; the empty state's
"Clear filters" link (`listEmpty.tpl`) serves /jobs as it serves every list.

### pi review, round 7 (gpt-6-sol)

Two findings, both confirmed and fixed. The post-confirmation refusal was only
shown in the bulk bar, which hides when the refresh empties the selection; it
now also reaches the page notice. A failed detail read's error outlived the
selection that caused it; each sync now starts by clearing it.

### pi review, round 8 (gpt-6-sol)

Three findings. A steady stream of events restarted the refresh debounce each
time and could postpone every refresh; the first event now schedules one
refresh and later ones join it. An emptied later page fell through to the
shared empty state, which read the page position as no filter and said "No
jobs yet"; it now says the page is empty and points to Previous. Declined:
"Pinned by me stays constrained by State". The quick filters combine, and the
count reports the combination the link opens; the plan's wording, which said
the Pinned count omitted State, was what was wrong, and is corrected above.

### pi review, round 9 (gpt-6-sol)

One finding, confirmed and fixed: a bulk request that failed while a refresh
emptied the selection reported only inside the hidden bar. It was the third
finding of one shape (rounds 5, 7, 9), so rather than patch the path it named,
every result of a command now goes through one `report()` that writes the bar,
the live region and the page notice together.

### pi review, round 10 (gpt-6-sol)

One finding, declined: without JavaScript, resubmitting the form reads an
exact RFC 3339 bound back at the precision the datetime input shows, widening
it by under a minute. The no-JavaScript form is the documented server-local
fallback; the JavaScript path returns an untouched bound exactly. Closing it
would put a shown-value and an exact-value hidden field beside every bound in
every submitted URL, for a browser without JavaScript.

## Flakes seen while verifying (separate change, not this one)

- `server/api_tests` import-bridge tests (`TestAnImportApplyWaitingForCapacityAnswersItsLegacyId`,
  `TestImportParseRouteAcceptsADurableJob`): `database table is locked`
  (SQLITE_LOCKED) from the harness's `cache=shared` in-memory DSN, which
  `busy_timeout` does not cover. Reproduced 4/15 on a clean worktree of
  4dea04ff, so it predates this change.
- `application_context` `TestAQueueBackedSubmissionsClaimKeepsEveryOtherProcessOut`
  and `download_queue` `TestClaimRetry_ClearsThePreviousAttemptsReport`: failed
  once each during a whole-repo run under heavy concurrent load; 5/5 and 10/10
  in isolation.

### pi review, round 11 (gpt-6-sol)

No findings. With round 10 (one finding, declined), two consecutive clean
rounds.
