# b4-kinds final report: INTERIM (wound down)

Branch `jobs-qa/b4-kinds`, base bdafcb51, **HEAD a18ab492**. The worktree is clean and nothing is running.

## Issues

- **K1: fixed.** The review lived only in page memory. Now `/admin/import?job=<handle>` restores the review from the stored plan (the upload writes the handle into the address), follows a parse that is still running, and for an applied import shows the report, the created groups and the outcome. The outcome comes only from an apply Job the viewer can see, following its Retries. Otherwise the page says the outcome is not available, never "completed". The parse publishes an entity output `{"importReview":handle}` ("View import review"), offered only on a parse Job and only while its plan or report is on disk. The hook in job_output_access.go and job_output_destination.go is its own commit (39b811b7). The apply publishes its first created group as an entity output. Every read goes through the authorized import and Job routes, and another user gets the same 404 as for an unknown handle.
  Tests: job_import_review_output_test.go, job_import_group_output_test.go, import_job_title_test.go (owner reopens, stranger gets identical 404s), adminImport.test.ts (8 cases), e2e admin-import/import-resume.spec.ts.
- **K2: fixed** (option A). Titles are now "Export of <root> [and N more groups]", "Import of <file>", "Apply import of <file>" and "Clusters for <reduction>". Names are bounded to 80 characters with an ellipsis, and the whole title stays within the ceiling (job_titles.go). The rule is stated in the comments and in job-system.md.
  Title audit: no reader outside owner or admin. after_job_* carries only a download's Name or a plugin label, the summary export is aggregates only, no log line has a title, and legacy rows carry no title. Tests: job_titles_test.go, import_job_title_test.go.
- **K3: fixed.** The plan counted GUID-matched resources as hash collisions. Now `resource_guid_matches` is counted separately, `resource_hash_matches` covers only resources no GUID claims, and the review and CLI name the deciding policy. Test: groupio/import_conflicts_test.go (plan predicts the apply).
- **K4: known limit, plus an alignment.** The hashes are luminance-only (measured in k4-measure.txt), and this is documented in resource-reduction.md and image-similarity.md. The reduction now applies the aHash guard through `mrql.SimilarPairPredicate`, shared with the similar list and SIMILAR TO. Tests on SQLite and Postgres.
- **K5: fixed.** The form submits with fetch and stays on the page, opens the Jobs panel on the new Jobs, and shows a status notice with any refusals. The label names the Jobs panel. Tests: resourceUploadBackground.test.ts, e2e jobs/background-download-form.spec.ts; job-center.spec.ts is updated.
- **Follow-ups done:**
  - /v1/jobs/queue managed entries now carry canonicalJobId, and legacy rows state times in UTC (bf0f3d3f).
  - GET on POST-only /v1/jobs/{verb} now gets the wrong-method answer (0b17dc27).
  - A legacy deferred row whose account read fails is deferred rather than failed (86db4045). The outage log already named no caller on the base.

## pi rounds

| Round | Bytes | Blocking | Changes |
|---|---|---|---|
| 1 | 2485 | 1 | Unknown apply outcome no longer shown as success, non-404 reads reported, role=alert (7edeac95) |
| 2 | 2919 | 2 | Per-import state reset on upload and resume, full Retry walk, non-404 Job read throws, PG twin (6a46fa4c) |
| 3 | not run | n/a | Codex usage limit, then wind-down; the prompt is ready in pi-prompt-3.txt and lists a18ab492 |

Trend: 1 then 2, and both round-2 P1s were in the page-state code round 1 touched. No findings are unhandled. Round 2's P2s are fixed too. Nothing was declined.

## Gates

- Go ./... EXIT=0 on 6a46fa4c (go-full-3.log).
- E2E all on 6a46fa4c: 2320 passed, 1 flaky (ws10:75, baseline) (e2e-all-2.log).
- vitest: 1695 passed on 6a46fa4c.
- PG Go on 6a46fa4c: only TestAClusteringRunIsTitledByItsReduction failed, a TempDir race. a18ab492 (test-only) makes the title tests wait for their Jobs, and they then passed 3 times on SQLite and PG. The full PG run was not repeated at a18ab492.

## Follow-ups (not done)

1. Colour signature for near-identical matching: a mean or 2x2 colour column in image_hashes, a guard, a runtime threshold and a backfill. Hue-only pairs within pHash 10 differ by 89-173 in mean RGB, re-encodes by 1.0 or less, and brightness, gamma or saturation edits by 17-41.
2. Router-wide 405 with Allow, and HEAD for GET routes. gorilla mux drops a method mismatch once a later route accepts that method.
3. A reduction Recompute has no lineage link to the previous compute.
4. The PG api_tests harness wires no runtime settings, so similarity thresholds are 0 there.

## Merge notes

- job_output_access.go and job_output_destination.go (b4-detail's files): additive branches only, in 39b811b7.
- e2e/tests/jobs/job-center.spec.ts: the background-download test no longer expects a redirect.
- `SubmitImportParse` gained a `fileName` parameter.
- `ManagedJobOptions.CanonicalJobID` is new.
- No adapter's Commands() list changed.
- public/dist was rebuilt; after merging, rebuild JS then CSS.

## Lessons

- A page-state section moved out of an `x-if` gains a lifetime of its own. Reset it with the data it describes, or the next item on the same page inherits it.
- A result file proves only that something ran, not how it ended: a partial apply writes one too. Read the outcome from the Job, and say "unknown" when the viewer cannot see the Job.
