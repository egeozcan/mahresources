# b4-list independent Standards + Spec review, round 5

**No P0 or P1 remains in this review.** The round-4 drawer defect is repaired at the incoming-snapshot ownership boundary, and the independent direct-detail and opened-reader-queue reproductions now pass. I reviewed the complete `bdafcb51d5fc4348877ca468338b29d4c1fa0170..0eb3402981d971e7a8f2d99214731d11578f734d` change on `jobs-qa/b4-list`, including the original List issues, all routed follow-ups, earlier repairs and declines, and both shared Flakes dependencies. This is not a review of only the last commit. No new actionable P2 or P3 was established either; the previously accepted limits remain identified below.

The frozen checkout matched the exact HEAD and tree `adb43d77a937963a3a92ecaa36fc4c5e2908ce3b` and was clean at the start and finish. The full diff contains 73 files, 4,998 insertions and 267 deletions; `git diff --check` passed. The latest commit changes the drawer consumer, its tests, the shared-helper test and the built JavaScript. Its source tree is identical to the pre-amend `8b9ba968` tree; the amend changed commit metadata only.

This is the requested independent **gpt-6-sol xhigh** review with both Standards and Spec lenses. I read the complete refreshed `review-prompt-r5.txt`, including the coordinator's 08:30 CEST completed-gate update, before reviewing. The user's model and delegation instructions override the historical pi workflow in the common brief and lessons. No pi/ask-pi, extra agents, source/Git/generated changes, builds, servers, broad/browser duplicate suites, merges or pushes were used. All new evidence is in the sibling `../b4-list-tmp/resume-20260928` directory. All owned test sessions and controlled reads finished; component probes destroy their components, cancel clocks/refresh timers and close happy-dom before returning.

## Severity rubric

> Tag every finding P0, P1, P2 or P3, by consequence, not by how likely the interleaving is.
> P0 = data loss or corruption, a security or access-control hole, a crash or a server that cannot
> start, or work that runs twice or is lost for good. P1 = broken: a listed issue that is not
> actually fixed at its root, a regression of existing behaviour, a user-facing path that fails or
> gives a wrong result, or an accessibility barrier that blocks a task. P2 = wrong or confusing but
> survivable: a workaround exists, it occurs only under an injected infrastructure fault (a stalled
> or failing DB call), missing or weak tests, or docs that disagree with the code. P3 = polish:
> wording, style, naming, or a pre-existing problem this change neither causes nor worsens.
> For each finding give file:line, a concrete failing scenario, and a suggested fix. End your
> answer with exactly one line: `BLOCKING COUNT: N`, where N is the number of P0 and P1 findings.

## Standards review

The standards/spec reading includes the worktree's `CLAUDE.md`, the Jobs sections of `docs/lessons.md`, the original implementation handoff, `../../brief-common.md`, `../../issues-b4-list.md`, `../../extra-b4-list.md`, the original List interim/final reports, all five lane reports and the retained earlier review packets/reports. The earlier complete production-diff reading is carried forward from rounds 3/4 and was rechecked against this frozen revision; current source, the complete commit list and diff, affected tests, all progress consumer assignments and the shared dependency changes were examined again. Historical reports supply context and decisions, not proof that the current source works.

The boundary remains principal → request-local `jobs.Deps` → service work. No request DB handle or principal is stored on the Service. `Access.Implicit` comes from the server principal, not a body or query parameter. Visibility predicates still surround the whole OR search expression; terms and escaped LIKE patterns are bound parameters. Neither replay ciphertext nor protected diagnostic references became searchable. Only the decoded final filename segment, under the extension rule, is projected into a download title/summary; query, fragment, userinfo and other path segments remain outside that projection.

Successor admission, replay/chain locking, lineage and the ancestor notification remain inside the existing transaction. The notification contains only the accepted command name; it does not disclose a hidden successor ID. The shared read-only suppression is present in Timeline, PublishedEvents and PublishedEventHead. Bulk requests retain per-target advertisement/version checks inside the transaction, rather than trusting the UI's intersection of selected commands. Concurrent Retry still admits one successor. The focused concurrency, advertisement and visibility tests passed.

Summary reads use the existing read-only transaction options, which avoid turning an aggregate scan into SQLite's `BEGIN IMMEDIATE` writer lock. Export acceptance seals only supported dimensions and resolves owner=me to the submitting account. Execution and output-open access recheck the current principal/data scope, including admin demotion and historical artifacts lacking a scope. Canonical summary encoding validates input first, preserves number spellings and member order, and applies the stored-size ceiling after encoding. The safe text prefilter is used only when it cannot hide a matching decoded value; numeric and escape cases still walk the document.

The latest drawer repair reuses the existing `mergeJobSnapshot` rule rather than adding another partial clock call or moving a progress version into the whole Job lifecycle. Server detail metadata and client progress remain governed by their separate versions/epochs. A source that does not own `progressVersion` cannot borrow it from the held row before comparison. Commands, outputs and lineage remain in fenced detail storage; pins and dismissal retain the preference-epoch guard. The direct and queue probes check those consumers as well as the visible bar, so the fix is not accepted solely on a helper assertion.

Alpine selectable-card state is declared on each nested component; removed cards unregister and the observer prunes current membership. Selection announcements use the actual entity noun, preserving ordinary/MRQL entity behavior. Job result text identifies the titled target/action and uses safe bindings and encoded links. Live progress updates visible text and accessible progressbar attributes together and is not announced every tick. State changes share the drawer ledger, including the failure reason and page handover. The header observer disconnects on destroy and reuses the menu's keyboard/focus behavior. SSR and live indeterminate fills both use `motion-safe:animate-pulse`.

The public SSR progress attribute remains restricted to phase, completed, total, unit, message, rate, ETA and estimated-ETA status; its RFC3339Nano timestamp is separate. It excludes identity/principal data, replay/diagnostics, metrics, series and the client-only marker. Escaping is covered by the focused provider render tests. The instant parser checks calendar/offset fields and compares seconds plus integer nanoseconds; display/freshness calculations retain millisecond arithmetic where that precision is sufficient.

Docs/OpenAPI and the mirrored CLI export help describe the actual search fields, ASCII case behavior on SQLite, measured scan cost, export filter/range metadata, ancestor notification, stream promotion semantics, auth-off attribution and the capped completion-refresh limit. The latest fix changes internal reconciliation without creating a new public API contract. Root's current JS→CSS build, Go build and all-browser gate validate asset generation/registration; this reviewer did not rebuild them. No independent Standards-only defect was established.

## Complete original-scope Spec coverage

“Ran” here means my fresh focused execution at frozen `0eb34029`. Source/spec inspection, retained scale evidence and root-owned broad/browser/PG gates are identified separately. No browser geometry or PostgreSQL execution is claimed as my own run.

| Issue | What I read and independently ran | Assessment |
|---|---|---|
| **J1: downloads distinguishable/findable** | Read the download title and summary codec, decoded last URL segment/extension rule, chosen-name precedence and byte ceiling; legacy URL translation; `jobs/query.go` value walk/prefilter; replay canonicalization and JSONB mapping. Read SQLite/PG search tests and million-row harness. Ran `TestADownloadIsTitledByTheFileItFetches`, sanitized value/syntax/secret/diagnostic and canonical-summary tests, legacy filename/host tests, plus the UUID/numeric collision overlay. | Fixed at the specified root. The URL filename remains searchable beside a chosen title; keys and punctuation alone do not match summary syntax. Numeric, escaped and historical SQLite encodings take the sound value-walk path. Title/query-token expectations follow the explicit filename rule. |
| **J2: readable summaries** | Read `jobSummaryPresentation`, full non-object decoding, nested object/array/flag/number handling, order/labels and card Details rendering. Ran `TestJobSummaryReadsAsFieldsNotJSON` and `TestAJobCardListsItsSummaryAsFields`; inspected browser card assertions. | Fixed. Strings render as text; structured values render as escaped labelled fields rather than the serialized JSON block. |
| **J9: filter behavior and auth-off attribution** | Read reset/pageshow/bfcache behavior, `ParseFilter` trimming, Origin checkboxes, auth-gated Owner/Actor fields, bare-local-date/legacy translation, principal-to-access and successor ownership. Ran frontend filter cases, provider choices/form tests, trim/date/legacy cases, implicit-principal/Retry tests and the owned-source attribution overlay. Inspected account E2E. | Fixed. Back restores this page's form; the request term is trimmed; Origin uses known choices. Auth-off successors have owner/actor nil/nil even when the source keeps owner 7. Retry/Continue/Repeat use the same helper; stored-account sources remain intentional exceptions. |
| **J10: summary and export** | Read the on-demand panel, generation/pending fences, same-filter query encoding, owner=me sealing, unsealable-dimension refusal, range title/description/CSV/JSON and service scope. Ran component races/pending checks; the non-anchored focused `TestSummaryExport` prefix and API refusal/demotion tests; summary counts/range/visibility/provider tests; exact UTC/Berlin wire probes and actual service day membership. | Fixed. Export metadata records the same supported filter and range. The end of the selected local day is inclusive through its final nanosecond, including 23/25-hour days. The pre-existing API requirement for an explicit export range longer than 90 days remains; short wire fixtures check serialization, not acceptance. |
| **U9: current bulk membership, Cancel/Retry** | Read nested declared Alpine fields, observer/destroy/unregister paths and actual morph, all changed Kind advertisements and transaction revalidation. Ran both bulk selection files, the real nested-Alpine regression, Job List bulk tests, multi-download Cancel/Retry, concurrent successor admission and in-transaction advertisement recheck. Inspected removal/Retry/MRQL browser specs. | Fixed. Removed cards leave selection. Bulk commands require the selected Jobs' bulk advertisement, then each target is independently rechecked. Refused targets do not stop other valid targets. Deferred Download now remains single because its warning changes the meaning. |
| **U10: reachable header controls** | Read available-width calculation, sibling badge/control measurements, gap/padding subtraction, links width cache, observer lifetime, collapse CSS and existing menu button/focus behavior. Inspected the browser regression at narrow desktop widths and restoration. | No blocker found. Geometry is measured when adjacent content changes. Root's complete no-retry browser gate includes this regression; I did not duplicate a physical browser/layout run. |
| **U14: useful bulk outcome names and noun** | Read per-target reports, action/title/link binding, selected noun and shared default. Ran list result/confirmation and selection-announcement tests; inspected browser outcomes. | Fixed. Results identify a titled Job/action rather than a bare UUID/key, and other entity lists retain their nouns. |
| **L7: /jobs progress and shared consumers** | Read list frame/reconcile/clock/teardown, public snapshots, shared version/instant/frame/fetch helpers, API polling and every Center/Panel assignment/cache path. Ran all eight clock cases, actual refresher/Alpine morph entrypoints, actual service/API precision, all three consumers, direct detail plus opened queue, and the new independent cache/preference matrix. | Fixed. Original list hydration and nanosecond ordering defects remain repaired; the round-4 drawer regression is repaired too. The latest source retains newer held progress without borrowing its version for a fetched source. The Detail lane's timeline/paging scope remains separate. |
| **X5: one lifecycle announcement with reason** | Read `jobAnnouncements`, list before/after handover, detail-page sites, drawer ledger/scope/catch-up handling, previous-version and news dedup. Ran the complete focused list/center/panel tests, including capped Jobs, page-first/drawer-first, previously heard states, My jobs/Everyone's and catch-up fallbacks; inspected cap=1 auth E2E. | Fixed at the shared ledger. Pages hand over both old and new cards. The drawer follows even a capped-out Job; an out-of-scope Job returns to its page. Failure reasons survive and progress/retried notifications are silent. |
| **X8: reduced motion** | Read SSR indeterminate fill and live class changes. Ran `TestAnIndeterminateJobCardHonoursReducedMotion` and progress component cases; inspected browser reduced-motion assertions. | Fixed in SSR and live updates. Shared forced-color structure is retained; broader Numbers visual changes remain separate integration scope. |
| **F1: resource completion outside drawer Finished cap** | Read `trackResourceCompletion`, bounded groups, the routed follow-up and download-queue docs. | Accepted documented limit. A completion outside the group cap may need the next refresh/reload; a newest successful download in a burst can reread the complete resource list. This is not claimed as a code fix. |
| **F2: another account retries a failed Job** | Read successor transaction/ancestor event, unchanged lifecycle version, no successor identity, all three read-only suppression sites and linked-job access. Ran ancestor notification/read-only tests and drawer silent-removal coverage; inspected auth E2E. | Fixed. Authorized ancestor viewers are prompted to reread without learning a hidden successor's identity or hearing the failure again. Read-only viewers intentionally do not get that existence signal. |
| **F3: hidden-event cursor rescanning** | Read allocator-head-before-events, full versus short page handling, marker/cursor order, head/query errors and reset/resume contract. Ran short-page, full-page, failed-head, hidden-row, reset/retention and post-promotion handler/API tests; inspected PG twins and docs. | Fixed under the accepted promotion policy. Only a successful short page advances to a successfully read allocator head, after the marker. Full pages drain; a failed read invents no head. Events published after promotion arrive. |
| **Flakes 876c16d3 / List 59dd755e: fixture lifetime** | Read fixture tracking, global/describe teardown order and production destroy fences/timer cancellation; read preserved normal/delay/master/branch attribution evidence. Ran the complete current drawer file among the 361 focused frontend tests. | Appropriate test-only fix. Panels are destroyed before timer/global restoration. The original request-count failure is independently attributed to a prior fixture and is separate from the product detail race. |
| **Flakes 109c2a5 / List d9a788a6: expired shutdown drain** | Read claimed/settled atomic states, settlement/ownership sites, drain admission and `ctx.Err()` after select, active-command classification, Executor and native runner quiescence contracts, deterministic descendant test and recovery. Independently ran original timeout/recovery and new settled-claimed deadline tests. | No new blocker. Ever-claimed work remains recoverable after an expired drain even if the callback has settled. The real-group test proves recovery kills the descendant before terminal classification. Root's repeat and broad gates remain separate evidence. |

## Latest repair: actual production boundary and consequences

The old finding is preserved in full in `review-r4.md` and `review-r4-consumers-final.log`. It was one P1 class, reproduced by both the direct `loadDetail` and the opened drawer's `loadStaleDetails`/`startDetailReader` queue. A successful version-3 detail response inherited `progressVersion:4` from the held row before comparison; its later clock replaced a newer version-4 frame, and the poisoned timestamp then rejected a valid later frame and caught-up server snapshot.

The current `src/components/jobPanel.js:1047` announcement read and `:1053` row installation now call `mergeJobSnapshot` with the source snapshot before bounded/fetched progress reconciliation. `jobCenter.js:1097-1105` defines the existing ownership rule: an incoming source lacking its own marker clears an inherited one before comparison; a higher lifecycle version also clears fields that a new snapshot omits when empty. `jobPanel.js:1686` uses the same helper for acknowledgement/read announcements, and `:1698` applies it before `mergeFetchedProgress` for the detail cache. A raw successful `loadDetail` cache entry at `:1044` remains the server's actual copy, guarded by lifecycle and preference epoch; the visible row then merges progress independently. This is correct separation, not an incomplete repair.

I reran the **unmodified original round-4 consumer probe** against this HEAD, preserving the historical failed log. The fresh `review-r5-consumers.log` exits 0. I also regenerated the service/API fixture and ran `review-r5-consumers.mjs`, a copy whose only fixture-path change selects the fresh API data; `review-r5-consumers-fresh-api.log` exits 0. Both use production components and controlled real entry functions, not a copied merge implementation.

The formerly failing successful-read sequence now produces:

1. Lifecycle-version-3 row starts the actual detail read; the response carries 20/1000 at `10:00:03Z`, no client marker, pin/phase and a version-3 Cancel advertisement.
2. A version-4 frame reports 900/1000 at `09:59:59.999999999Z` while the read is in flight.
3. Resolving the detail leaves the row at **900/1000, 90%, lifecycle 3, marker 4**. The raw detail cache is lifecycle 3 with completed 20 and no marker; commands still advertise `jobVersion:3`, and the source pin/phase install.
4. `resultSource` reads the row's progress, so the visible/accessibility consumer also reports 90%. The template's percent and aria value use this same result.
5. A valid version-4 frame at `10:00:00Z` advances to **910**; the caught-up version-4 snapshot at `10:00:01Z` advances to **920**, lifecycle becomes 4 and the marker clears.
6. The opened-reader-queue variant independently produces 900/marker 4 after the delayed response, then finishes with **readers=0 and inFlight=0**. It does not pass by fencing away an unresolved response at teardown.

`review-r5-snapshot-boundaries.mjs` is a further independent matrix, with results in `review-r5-snapshot-boundaries.log`, EXIT 0. It exercises the actual `applyStreamSnapshot`, `refreshJobPreference` and `loadDetail` consumers and checks both row/cache plus control/result reads:

| Additional challenge | Observed behavior |
|---|---|
| Successful same-lifecycle acknowledgement/cache copy with later server clock, held marker from a higher frame | Row and cache retain completed 900/marker 4; source pin, phase, command label, output label and lineage still install. |
| Higher lifecycle-5 snapshot with an earlier clock | Row/cache adopt completed 50 and lifecycle 5; marker and omitted phase/failure/control intent clear. Commands remain version 5. |
| Actual preference reread returns lifecycle 3 with later clock | Raw detail remains source-owned and marker-free; row/result retain 900/marker 4; pin installs and commands remain version 3. |
| Preference epoch advances while a read is pending | Stale pin metadata does not install; held progress remains 900 and invalidated detail cache is absent. The owned promise resolves before teardown. |
| Actual detail read receives newer lifecycle, then a lower lifecycle with a future clock | The newer source installs; the lower version returns failed and cannot replace lifecycle-5 controls/cache. |
| Equal-lifecycle cache and row copies one nanosecond older/newer | Older copy is rejected in both; newer copy is adopted in both. Higher lifecycle still wins with an earlier clock. |
| Source explicitly owns a progress marker | Its own marker is retained and ordered as incoming evidence. A server source lacking a marker first clears the borrowed marker, then newer held progress/marker may be retained by the comparator. |

There is no evidence that the fix promotes the frame's version into lifecycle state, abandons useful source metadata, invalidates command versions or bypasses preference fencing. Those were practical alternatives that would have hidden the old bar defect but broken other consumers; the independent assertions guard against them.

## All progress entry and cache paths audited

I used `rg` for every row/detail assignment, raw snapshot spread, marker and progress merge call in List, Center and Panel, including surrounding unchanged code. The rule is source-owned ordering metadata before comparison, and retained client marker only after determining that held progress is newer.

| Entry or assignment | Source path and assessment |
|---|---|
| List init, actual refresher completion and initialize/reapply wrappers | All use `reconcileProgressCards`. Required clock cases and actual new/queued/removed-returning membership morphs pass. |
| List frame | Rendered/held base is compared using shared helpers before `applyProgressFrame`. Higher frame with earlier clock wins; lower copies do not rewind it; server catch-up clears the marker. |
| List clock and destruction | One clock is derived from reconciled running snapshots; removal/nonrunning prunes held entries and the unneeded interval stops. A card without a public snapshot seeds no fabricated progress clock. |
| Center initial load | Initial server detail/list/cache is installed before following starts. There is no held frame marker to borrow in that first-load boundary. Subsequent reconciliation uses fenced common helpers. |
| Center frame | Shared frame result is installed consistently into detail, cache and row. Fresh API fraction and version/marker cases pass. |
| Center fetched detail, preferences and command answers | `applyStreamSnapshot` uses `mergeJobSnapshot` then `mergeFetchedProgress` with lifecycle/preference fences. Lower source/later-clock copies retain held higher progress while legitimate metadata is independently governed. |
| Center stream reducer/snapshot copies | Reducer uses source ownership helper; result rows use fetched-progress reconciliation. Canonical production SSE is event-only and reaches fenced reads. Pre-existing direct snapshot-test-only behavior is not promoted into a new product finding. |
| Panel frame, bounded lists and actual bounded group refresh | Shared frame/fetched helpers govern incoming progress. Actual group read, marker retention, lower source and lifecycle catch-up pass in the consumer probe. |
| Panel reducer/event/acknowledgement and upsert | Reducer construction uses `mergeJobSnapshot`; bounded rows apply shared progress comparison. Acknowledgement/cache construction now uses the helper before progress merge. Fresh row/cache matrix passes. |
| Panel detail, direct and opened queue | `loadDetail` keeps raw source detail in cache and uses source ownership before bounded row comparison. Both original production entry reproductions now pass, including future frames and catch-up snapshots. |
| Panel preference reread, including epoch change | Raw detail cache is source-owned, then common snapshot application compares rows/cache. Actual reread and pending epoch invalidation pass; stale metadata is not restored. |
| Panel reveal absent row, command-bearing row-as-detail and removal/reset | Reveal adds only an absent row. A row already containing commands is retained as its own detail; its marker belongs to that same retained source. Removal/scope reset/delete do not install server progress. Destroy cancels owned work and fences reads. |
| Panel `resultSource`, commands/outputs/lineage and display | Row overlays cached detail for visible progress; commands/outputs/lineage come from fenced detail. Actual result and controls assertions pass despite deliberately older raw cached progress. |
| Remaining raw spreads | List spreads for title/state retain held progress intentionally; command/notice formatting and bulk read metadata are not fetched-progress installation. Center notice spread formats a result. Panel `resultSource` makes the deliberate row-over-detail view. None was an unguarded incoming server-progress boundary. |

This audit covers the class, rather than accepting the latest two new calls in isolation. Series behavior is also preserved: older/equal/unknown fetched progress retains the whole held snapshot; newer data without series retains comparable held series; stream points replace an equal-time point or extend/compact the bounded series; unit changes clear incomparable data. Focused `jobProgress` tests and consumer checks pass. The marker orders progress without advancing the whole Job's version/state.

## Original eight clock cases, real morphs and full-precision time

`review-r5-progress-core.log` reruns the preserved independent source probe. All original required cases pass:

| Original contract | Result |
|---|---|
| Initial running SSR snapshot starts a clock; ETA counts down and stale rate/estimated ETA disappear without another frame | Pass |
| Equal version/equal timestamp keeps held complete progress and its clock | Pass |
| Same version, strictly newer SSR timestamp adopts bar/rate/ETA | Pass |
| Same version, older/equal SSR snapshot cannot rewind | Pass |
| Higher server lifecycle version wins even with earlier executor timestamp | Pass |
| After higher-version adoption, lower-version refresh cannot revive old progress | Pass |
| Older frames are ignored | Pass |
| Removed/nonrunning cards are pruned and the unneeded interval stops | Pass |

`review-r5-progress-entrypoints.log` reruns `jobList` with the real `createJobListRefresher`, morph utility and Alpine. It covers a newly inserted running card whose sole frame preceded insertion, queued→running without another frame, and removal followed by reappearance without another frame. Each has one held snapshot and a live interval after actual DOM replacement, then loses stale stats after 11 virtual seconds. This is consumer/membership evidence, beyond an isolated reconcile helper. The missing-public-snapshot control retains no fabricated snapshot/clock.

Fresh `jobs.Service.UpdateProgress` and the actual API `jobProgressFrame` produce `.000001Z`, `.0001Z` and `.0009Z` at the same lifecycle version, with completed 50, 100 and 900 respectively. `review-r5-service-boundaries.log`, `review-r5-service-snapshots.json`, `review-r5-api-precision.log` and `review-r5-api-frames.json` retain the source/formatter proof. The fresh consumer copy uses those actual API frames successfully in all three consumer paths. The preserved real-morph script explicitly uses the retained round-3 API fixture; it is not silently described as loading the newly generated file.

The real default clock also produced two consecutive committed reports at `2026-09-28T06:33:26.516507Z` and `.516664Z`, **157 microseconds apart**, both version 1. `review-r5-real-clock-snapshots.json` preserves them. Full-precision ordering is therefore meaningful at an actual service cadence, not just at synthetic timestamps.

The comparator challenge covers variable fraction widths, one-nanosecond older/newer order, second boundaries, offset-normalized equal instants, version-before-clock precedence, and the client marker. Valid leap day/years 0000 and 0099 pass; impossible days, hour/minute/second overflow, malformed offsets and more than nine fractional digits are rejected. A valid equal-version instant outranks an absent/malformed one. Both unknown instants yield no time ordering: fetched/SSR copies retain held progress, while an equal-version unknown-time frame may update. Equal valid instants do not rewind. This preserves the source policy stated in the packet and does not use lexicographic timestamps or `Date.parse` milliseconds for ordering. The previously corrected explicit-undefined harness fixture is retained; no new parser failure occurred.

## Export/day boundaries, attribution, access and performance

The exact wire probes exercise production `jobSummary.exportSummary` and record both URL and JSON body in `review-r5-export-wire-utc.log` and `review-r5-export-wire-berlin.log`. The filter remains `ownerId=7`, remote-download and dismissed=false. UTC ends at the final nanosecond before midnight. Berlin spring sends `2026-03-28T23:00:00.000Z` through `2026-03-29T21:59:59.999999999Z`; fall sends `2026-10-24T22:00:00.000Z` through `2026-10-25T22:59:59.999999999Z`. Invalid local end dates are refused. Local calendar arithmetic produces 23/25 hours instead of assuming a fixed day length.

The independent service overlay inserts each selected day's first instant, a fractional instant in its last second, its final nanosecond and the following midnight. Actual inclusive `SummaryRange` membership is exactly three for UTC, Berlin spring and Berlin fall, excluding the following midnight. Acceptance/scope tests separately cover the long-range requirement, format/refusal, visibility, legacy artifacts and post-demotion output access. The short wire probe is not described as a successful short-range export enqueue.

The auth-off overlay retries a source owned by account 7 as the implicit administrator. The source remains owner 7; the successor is owner/actor nil/nil. Current authentication docs explicitly state both halves, matching `ownerReference` rather than implying inherited successor ownership. Retry/Continue/Repeat share that helper. Stored-account schedule/deferred behavior remains intentional. Owner/Actor UI gating uses authentication presence, not just an administrator flag.

Search was challenged against keys versus values, punctuation, nested data, exponent/numeric renderings, quotes/backslashes, Go-escaped `<`, `>`, `&`, controls, U+2028/U+2029, canonical/current and historical SQLite encodings, plus protected secret/diagnostic exclusions. A valid unrelated UUID may match a numeric term; the overlay preserves that legitimate identity result instead of assuming exactly one row. PG JSONB plus strict recursive path remains dialect-specific and is covered by root's fresh PostgreSQL gate. No access predicate was moved into the search OR or bypassed by the summary walk.

The million-row harness was read but not rerun. Preserved alternated fastest-of-five before/after measurements are the accepted scope evidence: SQLite needle 241→361 ms, key-like scheme 154→913 ms, numeric 252→856 ms; PG 1358→1372 ms, 1245→3193 ms, numeric 1301→2394 ms. Docs state the up-to-about-one/three-second scans. The faster key query matched the JSON field name in every row, which is the wrong result being removed. A numeric text prefilter is unsound for engine-expanded exponent/rounded number renderings. I found no new scale regression beyond this explicitly accepted cost. Fresh focused correctness runs are not claimed as scale measurements.

## Shared shutdown and fixture dependencies

The shutdown source was independently assessed rather than accepted only because Flakes reviewed it. Claimed and unclaimed settlement states are distinct, so `wasEverClaimed` stays true after the claimed callback settles. Claim/settle compare-and-swap ownership remains exclusive and accounting occurs once. Admission closes under the existing mutex before drain; checking `ctx.Err()` after select catches a simultaneously-ready expired deadline even when workersDone wins select. After expiry, ever-claimed active commands leave durable recovery evidence instead of shutdown publishing terminal state on callback count alone. Unclaimed cleanup retains its previous ownership path.

The Executor contract requires group quiescence and drained output before return; the real native runner keeps Execute live until parent, pipes and group are done. The deterministic test expires the drain, permits the managed callback to settle before classification, proves the owned descendant is alive while the durable row remains nonterminal, then proves startup recovery kills that exact group before Interrupted classification. My current focused Go run executes that test and the original running-group timeout test successfully. Root separately ran the original unmodified timeout test 300/300 on the same source (`../b4-flakes-tmp/resume-20260928/root-shutdown-original-300.log`, 27.974 s, EXIT 0); that is root evidence. No new work-twice, lost-work or fabricated-persistence-verdict path was established.

The fixture repair remains confined to `jobPanel.test.ts`. Every constructed fixture is destroyed before real timers/globals are restored, including describe-level cleanup ordering. Production destruction cancels scheduled refresh/retry/clock/live-region work and fences future work. Preserved controlled 1.8-second master/branch delay traces identified the old extra requests as an earlier fixture's scheduled panel refresh. Normal and controlled-delay drawer runs passed 188 tests after cleanup; the current focused drawer file passes its 191 cases within the fresh 361-test check. This is distinct from the round-4 product defect, whose successful detail response completes and is asserted before destruction.

## Previously declined, with evidence

- **F3 access promotion sends newly visible older history:** retained as an intentional live-stream policy. Advancing beyond hidden history prevents old outcomes arriving after catch-up as new announcements; lists remain the record. `TestCanonicalJobSSEPromotionSendsWhatIsPublishedAfterIt` freshly passes, and source/docs describe post-promotion publications. Reconnect continues using the browser's last delivered event ID. No contrary new evidence warrants calling this lost durable history.
- **Million-row value-search cost:** retained as the measured documented tradeoff above. It removes incorrect key/punctuation matches and preserves numeric/escape correctness. No new measurement disproves the accepted limit.
- **F1 completion beyond capped Finished list:** retained as a documented limit, not reclassified as fixed code. The next reread/reload shows the resource; a newest completion may reread the whole resource list.
- **Read-only ancestor Retry notification:** retained as the explicit access policy. All three read APIs suppress the existence signal; authorized writable viewers still receive the safe ancestor event. Tests and docs agree.
- **Auth-off source attribution:** the earlier docs discrepancy is repaired. The owned-source overlay and current docs both show source owner retained and successor owner/actor cleared; no inherited-successor assumption is accepted.
- **Numeric fixture exactly one match:** rejected because an independently generated valid UUID can legitimately match the term. The adversarial overlay confirms the intended search behavior while keeping that identity match valid.
- **Canonical snapshot-test-only paths versus production SSE:** retained distinction. Production SSE carries event-only messages; actual detail/preferences/reducer reads are tested at their real boundaries. A pre-existing test-only direct snapshot behavior is not a new defect caused by this diff.
- **Initial JS request-count failure as a List product regression:** rejected with preserved independent master/branch delay/earlier-panel trace evidence. Fixture cleanup fixes its owner; the repaired drawer P1 was separately reproduced in product code and remains separately verified.
- **Earlier PM focus-spec retry as baseline or fixed:** neither conclusion is established. It was a setup POST timeout before focus assertions with unproved cause. The current no-retry gate validates this candidate but is not causal proof of that earlier timeout.

No new decline was used to lower a demonstrated P0/P1. All old confirmed blocking findings were checked as repairs, and the latest finding's direct production consequences now pass.

## Fresh independent verification and root gate provenance

All reviewer-owned commands ran in the frozen List worktree with saved logs and their own exit status. `review-r5-commands.txt` records the exact commands; synthetic Go overlay test destinations leave tracked source untouched. Every yielded session was resumed to completion. The repository's standalone-Node module-type warning is recorded in the DOM logs; it is not a test/product failure and source was not changed to remove it.

| Reviewer-owned focused check | Evidence | Result |
|---|---|---|
| jobProgress, jobList, jobCenter, jobPanel, bulkSelection, bulkSelectionNested | `review-r5-focused-vitest.log` | 6 files / 361 tests passed, EXIT 0 |
| Original-scope cases across jobs, application_context, jobview, template providers, API handlers, API tests, server and plugin_commands; includes export prefix cases and both shutdown tests | `review-r5-focused-go.log` | All 8 package results passed, EXIT 0 |
| Service real clock, nanosecond storage, inclusive UTC/DST membership, implicit successor attribution and UUID numeric collision overlay | `review-r5-service-boundaries.log`, `review-r5-overlay.json`, test/JSON files | All selected tests/subtests passed, EXIT 0 |
| Actual API formatter fixture | `review-r5-api-precision.log`, `review-r5-api-frames.json` | Passed, EXIT 0 |
| All original eight clock cases and ordering controls | `review-r5-progress-core.log`, preserved `review-r3-progress-probe.mjs` | Passed, EXIT 0 |
| Actual refresher/Alpine morph membership and stream callbacks | `review-r5-progress-entrypoints.log`, preserved `review-r3-progress-entrypoints.mjs` | Passed, EXIT 0 |
| Original R4 actual consumer reproduction, unchanged | `review-r5-consumers.log`, preserved `review-r4-consumers.mjs` | All scenario groups passed, EXIT 0; old failed log retained |
| Actual consumer copy using freshly produced API frames | `review-r5-consumers-fresh-api.log`, `review-r5-consumers.mjs` | All scenario groups passed, EXIT 0; readers drained |
| Additional actual cache/preference/detail source-ownership and lifecycle/epoch matrix | `review-r5-snapshot-boundaries.mjs`, `.log` | All 6 groups passed, EXIT 0; components/promises finished |
| Exact export URL/JSON in UTC and Berlin | `review-r5-export-wire-utc.log`, `review-r5-export-wire-berlin.log`, preserved `review-r3-export-wire.mjs` | Passed, EXIT 0 |
| Complete diff whitespace, frozen HEAD/tree and clean status | `git diff --check`, `git rev-parse HEAD HEAD^{tree}`, `git status --porcelain` | Passed; exact candidate; empty status |

The focused Go regex deliberately uses the `SummaryExport` prefix without the previous round's accidental ending anchor; the actual export tests execute in this run. The earlier round's separate explicit-prefix coverage remains preserved but is not substituted for this execution. The fresh real-clock snapshots use the current default service clock; the nanosecond fixture uses controlled service times, then the actual API formatter, then actual JS consumers.

Root-owned full gates at the exact current candidate were inspected, not duplicated:

| Gate | Root evidence | Observed result |
|---|---|---|
| Whole Go/SQLite `./...` | `go-full-r5.log` | EXIT 0 |
| Requested PostgreSQL Go trees | `go-pg-r5.log` | EXIT 0; MRQL 7.291 s, API 163.526 s, application_context 367.979 s, jobs 20.535 s |
| Whole JavaScript | `vitest-r5-full.log` | 100 files / 1,732 tests passed, EXIT 0 |
| JS then CSS builds | `build-js-r5.log`, `build-css-r5.log` | Both EXIT 0 |
| Go build | `go-build-root-r5.log` | EXIT 0 |
| All-browser suite | `e2e-all-r5.log` | 2,337 passed, 5 skipped, **no retries**, EXIT 0 |
| Saved browser verdict | `e2e-all-r5.last-run.json` | status passed, failedTests=[] |
| Root independent actual consumers | `root-r5-consumers.log` | All scenarios passed, EXIT 0 |

The coordinator also reports the reversible integration merge retained both `jobName` and `mergeJobSnapshot` at its import conflict, rebuilt JS→CSS, and passed 331 tests across the three merged consumers (`../b4-integration-tmp/resume-20260928/list-r5-merge-focused.log`). That is root integration work, not this reviewer's execution or a replacement for this full lane review. Root owns the final merged integration checks, including the browser/PG-browser and explicit long-name guard.

Historical failures remain explicit: the pre-shutdown-fix `go-full-r4.log` and repeat both failed the original descendant-live check at the one-second deadline. The source repair is now in the complete diff and freshly tested. Pre-cleanup `vitest-r4-full.log` reported 1,726 pass / one two-versus-five request-count failure; its repeat passed 1,727. The earlier `vitest-final.log` failure was independently reproduced on master and branch with the 1.8-second earlier-fixture trace; normal/delay cleanup evidence and the current full suite follow the test dependency. R4's exact final browser gate had a passing retry in PM spec 923, a ten-second task-create POST timeout at test line 930 during setup before focus checks; its cause remains unproved. R5's clean run does not rewrite that history or establish why it happened.

## Convergence and verdict

Blocking-count trend is **8 → 2 → 2 → 1 → 0** across rounds 1-5. The round-4 P1 was introduced as a missed consumer of the round-3 shared progress-version repair; round 5 corrects the common incoming-snapshot ownership rule and audits every remaining construction/consumer path. No new blocking finding introduced by that latest fix was established. Earlier list morph and submillisecond defects remain independently controlled, rather than assumed fixed from the new drawer tests alone.

This is the first zero-blocking round, meeting the user's authorized lane stop rule before the cap of eight. No tracked repair is made by the reviewer. The reviewed clean candidate is ready for the coordinator's remaining integration work, with the accepted limits and historical gate signals recorded above.

BLOCKING COUNT: 0
