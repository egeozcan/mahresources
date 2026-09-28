# b4-detail — independent final review, round 4

Reviewed HEAD **014fccc58487133081e24bd87fcc68294f9ea3f4** against **bdafcb51** using git diff bdafcb51..HEAD and the eleven-commit log. The worktree was clean before and after the review.

Worktree: /private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-detail
Evidence directory: /private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-detail-tmp/resume-20260928

Read the full supplied lane-input prompt, its issue list, severity rubric and prior declines; the matching handoff lane report and brief-common.md; worktree CLAUDE.md (layering, request-bound DB handles, accessibility, generated assets and verification); and the relevant Jobs lessons at the end of docs/lessons.md. I used the code-review skill's Standards and Spec axes directly. I did not use pi, ask-pi or subagents, and did not change tracked/generated files, branches or Git state. List search, progress-number semantics and vocabulary that other lanes replace remain integration review responsibilities.

**Verdict: no confirmed P0 or P1. One P2 test-coverage finding, below.** This is based on source review, focused checks, explicit deferred-read probes and inspection of the supplied gate logs. I did not rerun a full browser or broad Go suite.

## Findings

### P2 — Preserve the timeline's generation and teardown cases in the committed tests

**Location:** src/components/jobDetail.test.ts:173 and :178; the guarded production code is src/components/jobCenter.js:826 and :846–855.

The committed timeline tests exercise paging, a read bound, live wake-ups, catch-up, read failure and phase removal, but no test starts a second load or destroys the component while an event-page response is pending. The shared test helper generally resolves each mocked request immediately, and the file has no delayed-response generation/teardown case.

**Concrete coverage gap:** load a Job, hold its first event-page response, then load that Job again before releasing the first response. The replacement timeline must fetch afterSequence=0 rather than retain events from the old generation. Likewise, destroying the page with both a response and another wake-up pending must append no events and start no queued request. Removing the generation check or teardown guard would not be caught by the current committed timeline tests.

**Consequence/severity:** missing regression coverage for a new concurrency mechanism, hence P2 by the supplied rubric. I do **not** assert a current normal-operation failure: all three deterministic scratch probes pass at this HEAD.

**Suggested fix:** port the deferred-response cases in review-r4-timeline-probe.mjs into jobDetail.test.ts. Assert a maximum of one active event read, cursor progression [0, 1] for coalesced wake-ups, [0, 0] after a reload, and no applied response or second read after destroy. This is a local test follow-up, not a production fix.

**Evidence:** review-r4-timeline-probe.mjs and review-r4-timeline-probe-vite.log (three PASS lines). The initial direct Node invocation could not import the app's bare JSON module under Node 25; that is recorded in review-r4-timeline-probe.log. Running the unchanged probe through Vite's SSR loader succeeded with exit 0. The loader failure was a probe-harness issue, not a lane defect.

## Spec: per-issue review and evidence

### J4 — Detail times, durations, retention and consistent format

**Read:** templates/displayJob.tpl:59–80 and its timeline/output time bindings; src/utils/localTime.js; src/components/jobCenter.js:271–318 and :1205–1213; server/template_handlers/template_context_providers/job_template_context.go:108–123; jobs/service.go:1264–1328; src/utils/localTime.test.ts; the times tests in src/components/jobDetail.test.ts; e2e/tests/jobs/job-detail-page.spec.ts:59.

**Ran:** both localTime.test.ts tests and all jobDetail.test.ts cases as part of the 93-test focused JS run; TestJobRowTimesShareOneZone in the focused Go run.

**Conclusion:** fixed in this scope. The detail page exposes accepted/scheduled/started/resumed/finished/history-expiry instants, labels the reader's zone, formats times in the cards' year-month-day/24-hour form and uses seconds where needed. Cumulative state durations convert Go nanoseconds and add elapsed time only for the state currently entered. This matches the service's banking rule. Pinned-history wording distinguishes retained history from independently expiring files. Drawer row ages were not reclassified into this lane.

### J5 — File naming and finished-export result links

**Read:** application_context/job_output_access.go:347–452; application_context/job_export_adapter.go's OpenJobOutput; jobs/short_id.go; server/api_handlers/job_output_handlers.go:27–77; server/jobview/result.go; resultOutput/outputLinkLabel in src/components/jobCenter.js and their drawer wrappers; application_context/job_output_filename_test.go; server/api_tests/export_job_bridge_test.go:466; the Go result-link and JS file-output tests; the finished-export E2E case.

**Ran:** TestAJobOutputFileIsNamedByItsLabelTimeAndStoredExtension, TestAGroupExportArchiveDownloadsUnderAFileNameWithItsExtension, all TestResultLinkFor* tests, TestShortIDIsTheIdsLastEightLettersAndDigits, JS file-output tests, and the existing TestOpenJobOutputRechecksJobAndOutputAuthorization.

**Conclusion:** fixed. Both generic and group-export file openers use the new label/time/Job-tail/extension name, including .tar.gz. The Go and JS result selection retain entity precedence, then historical plugin summary destinations, then an available/unexpired artifact for succeeded Jobs. The drawer already renders that selected result. The output endpoint reauthorizes the file and encodes Content-Disposition with mime.FormatMediaType; this change does not introduce a raw-header or path-writing sink. Existing root-path checks still reject absolute/traversal/backslash/NUL references.

### J8 — Recoverable bad filters and missing/malformed Job pages

**Read:** job_template_context.go:190–265, :446–477 and :869–957; application_context/job_compatibility.go:39–84; application_context/job_context.go:178; jobs/service.go:284; server/routes.go:508–523 and scopedCtx in server/request_scope.go; error_surfaces.go; render_template.go; templates/listJobs.tpl; the provider and HTTP page tests.

**Ran:** TestTheJobPageAnswersForTheJobItNames, TestAnUnusableJobCenterFilterKeepsTheFilterForm, TestAnUnknownJobIsANotFoundPageWithAWayBack, TestALegacyHandleOpensTheJobItNames, TestAJobPageWhoseReadFailedStillRenders, TestJobListRefusesAnUnreadableFilter, TestJobListRefusalNamesOnlyTheFilterProblem, TestARefusedFilterKeepsAnAdministratorsOwnerChoice, and TestHandleResolutionRechecksVisibilityAtTheFacade.

**Conclusion:** fixed. The HTML route builds its reader from the request principal. Missing or invisible canonical IDs and unresolvable handles become 404 recovery pages; missing ID becomes 400. Slash-containing IDs never reach the client API routing error because the server handles that page request first. A visible legacy handle redirects only after an authorized read, with query escaping. Other read errors render the recoverable client page rather than being called absence. Refused filters return 400 on the Job Center with the form/clear link, retain administrator Owner/Actor options and do not instantiate the list's live component. _statusKeepsPage is an internal boolean and is excluded from JSON projection.

### X7 — Visible output label is contained in its accessible name

**Read:** outputLinkLabel/outputLinkAccessibleLabel/resultAccessibleLabel in src/components/jobCenter.js; output/result bindings in displayJob.tpl and jobPanel.tpl; server/jobview/result.go; the file-output name cases in jobCenter.test.ts.

**Ran:** the JS output accessible-name cases for artifact/report/log/external-link/future type, plus Go result-link tests.

**Conclusion:** fixed. Accessible names begin with the displayed action words, and list/drawer names append the Job context after those words. The former visible “Open output” versus accessible “Open Exported archive” mismatch is removed.

### X9 — Distinct document titles and one h1

**Read:** job_template_context.go:894–957; jobs/short_id.go and tests; jobCenter.js:242–268 and :1197–1204; displayJob.tpl; layouts/base.tpl:6; partials/title.tpl; commandFocusTarget and focusOn; provider, HTTP-page and JS title tests; adapted E2E helper and selectors.

**Ran:** title/short-ID/heading recovery JS tests, TestAJobPageIsTitledByItsJob, TestShortIDIsTheIdsLastEightLettersAndDigits and TestTheJobPageAnswersForTheJobItNames.

**Conclusion:** fixed. The server title and client title rule agree, including state, site title and ID tail. A successful client recovery updates the layout heading too. The page's h1 is the layout's title heading; removed detail markup does not create a second one. Command focus fallback reaches that real heading using the existing focus helper. Template values are escaped and client heading/title writes use text, not HTML. I found no new evidence that rebuts the prior P2 ruling for an eight-character title collision; the full ID remains visible in displayJob.tpl:26.

### X10 — Narrow/short drawer reflow

**Read:** public/index.css:182–199; jobPanel.tpl title, kind, owner, progress/metric/graph labels and responsive grids; e2e/tests/accessibility/job-drawer-small-viewport.spec.ts.

**Ran:** no new browser layout run. Inspected the supplied e2e-all-2.log entries for all three small-viewport cases, each passed, including 320x180 scrolling, 320px wrapping and the axe structure check. Confirmed required break-words/grid-cols-1/sm:grid-cols-2 classes exist in committed public/tailwind.css.

**Conclusion:** fixed on the inspected source and existing browser evidence. At short heights the drawer itself scrolls, its list no longer consumes an unusable flex sliver and group headings stop sticking. Narrow labels wrap and metrics/graphs use one column. I did not claim a fresh visual/assistive-technology run.

### X11 — Drawer semantics and internal wording

**Read:** jobPanel.tpl:26–105 and footer; displayJob.tpl actions/timeline; keepCommandFocus/commandFocusTarget; changed job-center-a11y and small-viewport specs and command-selector migrations.

**Ran:** focused JS template assertions. Inspected passed axe/small-viewport results in the supplied E2E gate; no fresh browser axe run.

**Conclusion:** fixed. Drawer header/footer are ordinary divs, the role-less list div no longer has aria-label, actions are named “Job actions”, focus lookup uses data-job-commands, and the visible “Earlier events are not announced again” note is gone. The specs no longer suppress the two landmark rules.

### X12 — Shortcut refusals belong to the blocking dialog

**Read:** src/utils/modality.js:59–106; globalSearch.js toggle and jobPanel.js toggle/openFromEvent; public/index.css:201–228; dialogRefusal.test.ts; dialog-shortcut-refusal.spec.ts; actual Search/Jobs x-if templates and representative retained dialog/focus implementations.

**Ran:** all dialogRefusal.test.ts cases (creation/repeat, ten-second expiry, focus leaving, both shortcut directions). Inspected the supplied passed browser refusal and 320x180 bounding-box cases.

**Conclusion:** fixed for the specified Search/Jobs flows. Both sides use the same helper, place a visible role=status region inside the blocking dialog and retain existing focus. The region is established before text is written and repeated notices cancel earlier timers. It is fixed at the viewport bottom, removes itself on focus leaving or timeout, and the specified dialogs are removed by x-if on close. No outside live-region announcement remains on these refusal paths.

### U11 — Related Jobs identify their relation and attempt

**Read:** jobs/types.go Lineage; jobs/query.go:1345–1397; job_handlers.go's response types/conversion; jobCenter.js:331–380; displayJob.tpl:162–181; Go relation/visibility and JS lineage cases; same-pair E2E case; openapi schema generation/diff.

**Ran:** TestLineageNamesOnlyVisibleRelatives, TestLineageSaysHowEachRelativeIsRelated, TestJobDetailSaysHowEachRelatedJobIsRelated, JS lineage cases and TestGeneratePartialSchema* including embedded fields.

**Conclusion:** fixed. The link arrays are appended index-for-index only after the related Job's visibility is established. A pair linked by Retry and Repeat keeps both relations in the API and distinct id:relation keys in the template. Each entry exposes readable relation, state, acceptance time to the second and an ID tail in the link text. Parent/child direction and partial-success Continue wording match the service model. No hidden Job is exposed by the added relation.

### L7 detail half — Live and paged timeline; settled phase

**Read:** jobCenter.js:798–906, :1055–1075 and :1105–1168; GetJobTimelineHandler in job_event_handlers.go; jobs/query.go:1232–1250; displayJob.tpl:184–216; jobDetail.test.ts; the live/bounded E2E cases.

**Ran:** every timeline/phase/catch-up/error test in jobDetail.test.ts; TestTimelineReturnsTheJobsOrderedBoundedTimeline; the three deferred-read scratch probes. Inspected passed live and pagination E2E results.

**Conclusion:** fixed, with the P2 committed-test follow-up above. Reads use an exclusive sequence cursor, pages of 200, a bound of 1,000 and an explicit continuation. Stream wake-ups for this Job and stream catch-up both trigger serialized following reads; another Job does not. Reload generations and destruction reject old replies in the probe. The base's snapshot replacement/phase-clearing rule is preserved and explicitly pinned by a test. Failed timeline reads offer Try again; event types are never blank, including later-release types.

### Search focus follow-up

**Read:** globalSearch.tpl:76; generated focus:ring-amber-600 CSS; dialog-shortcut-refusal.spec.ts:40; CLAUDE.md's forced-colors rule.

**Ran:** no fresh computed-style browser measurement; inspected its passed E2E entry in the supplied gate.

**Conclusion:** fixed. The inline box-shadow suppression and focus:ring-0 are removed, a visible focus ring is applied, and focus:outline-hidden remains as the forced-colors-compatible outline fallback. No outline-none was introduced.

## Standards and cross-cutting review

- **Access/security:** new server reads and legacy resolution remain principal-scoped; an unauthorized handle cannot provide a canonical redirect. Existing output visibility is rechecked at open. Added relation data does not bypass far-endpoint visibility. Focused facade/output/hidden-read tests passed.
- **Data/concurrency:** no new execution/claim/mutation logic or SQL was added; lineage now retains link identity without changing the query predicate. Timeline request serialization, catch-up reconciliation, generation replacement and teardown were challenged with controlled pending responses. No confirmed normal-operation loss, duplicate execution or corrupt stored data was found.
- **Scale/reactivity:** single-ID page reads and bounded event pages do not scan the deployment's Job population. Keyed timeline and lineage entries retain identity; the per-second time display updates small fixed sets of time rows. The added response/schema types preserve existing Job snapshot fields rather than replacing them with relation-only objects.
- **Standards:** request-bound handles and layering remain intact; comments explain the rule rather than QA/review history; git diff --check passed. Repeated Go/JS display rules are intentional cross-runtime counterparts with shared test cases. I found no actionable new Fowler smell warranting a separate finding.
- **Docs/assets:** inspected changed job-system.md and managing-resources.md sections, openapi.yaml and the embedded-field partial-schema generator. Documentation agrees with filenames, title/heading, relations, paging/bound, retention and refusal behavior. No conflicting Jobs rule in CLAUDE.md was found. New frontend behavior strings are present in public/dist/main.js and responsive/focus classes exist in public/tailwind.css. I did not rebuild or alter generated assets.
- **Prior declines:** no renewed title-collision P1, no requirement to put an ID into every h1, and no superseded list/numbers/vocabulary finding. No blocker was inferred solely from an injected dependency failure.

**Axes:** Standards: one P2 (committed concurrency regression coverage); Spec: zero findings. Neither axis has a P0/P1.

## Checks actually executed here

Focused JS, exit 0, **4 files / 93 tests passed**:

    npx --no-install vitest run src/components/jobDetail.test.ts src/components/jobCenter.test.ts src/components/dialogRefusal.test.ts src/utils/localTime.test.ts

Log: review-r4-js-focused.log.

Focused Go, exit 0, **all seven selected packages passed**:

    go test --tags 'json1 fts5' ./application_context ./jobs ./server/api_tests ./server/api_handlers ./server/jobview ./server/openapi ./server/template_handlers/template_context_providers -run 'Test(AJobOutputFileIsNamedByItsLabelTimeAndStoredExtension|TheJobPageAnswersForTheJobItNames|AnUnusableJobCenterFilterKeepsTheFilterForm|AGroupExportArchiveDownloadsUnderAFileNameWithItsExtension|ShortIDIsTheIdsLastEightLettersAndDigits|TimelineReturnsTheJobsOrderedBoundedTimeline|LineageNamesOnlyVisibleRelatives|LineageSaysHowEachRelativeIsRelated|JobDetailSaysHowEachRelatedJobIsRelated|JobDetailHidesInvisibleIDBeforeReadingRelatedData|ResultLinkFor.*|GeneratePartialSchema.*|AJobPageIsTitledByItsJob|AnUnknownJobIsANotFoundPageWithAWayBack|ALegacyHandleOpensTheJobItNames|AJobPageWhoseReadFailedStillRenders|JobListRefusesAnUnreadableFilter|JobListRefusalNamesOnlyTheFilterProblem|ARefusedFilterKeepsAnAdministratorsOwnerChoice|JobRowTimesShareOneZone)$' -count=1

Log: review-r4-go-focused.log.

Additional authorization-focused Go, exit 0, both selected packages passed:

    go test --tags 'json1 fts5' ./application_context ./jobs -run '^Test(HandleResolutionRechecksVisibilityAtTheFacade|OpenJobOutputRechecksJobAndOutputAuthorization|VisibilityHidesAJobFromEveryOtherReadPath)$' -count=1

Log: review-r4-auth-focused.log.

Deferred timeline probe: three controlled-promise cases passed under Vite's SSR module loader, exit 0. Evidence: review-r4-timeline-probe.mjs and review-r4-timeline-probe-vite.log. Its initial plain-Node import failure is retained separately as described above. No browser/server instance or full suite was started for the probe.

Read-only repository checks: git diff --check bdafcb51..HEAD, git status --porcelain=v1 and git rev-parse HEAD. The diff check passed; status stayed empty and HEAD stayed pinned.

## Supplied gate evidence inspected, not rerun

The lane's earlier logs end in EXIT=0: go-full-4.log; vitest-4.log (**102 files / 1711 tests passed**); and e2e-all-2.log (**2330 passed, 5 skipped**, no failed tests). The current e2e/test-results/.last-run.json says passed with an empty failedTests list. I checked individual passed E2E entries for the detail page, downloads, missing IDs, same-pair relations, paging/live timeline, invalid-filter recovery, refusal reflow and narrow drawer.

The exact-HEAD Postgres gate at resume-20260928/go-pg.log ends in **EXIT=0** and reports mrql, server/api_tests, application_context, application_context/validation and jobs green. The supplied interim handoff report's “not run” PG statement is superseded by this log, as the coordinator stated.

BLOCKING COUNT: 0

