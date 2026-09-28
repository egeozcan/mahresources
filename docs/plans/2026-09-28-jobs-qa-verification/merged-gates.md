# Final merged gate evidence

Production source: `67bb52f31e6600f463eb46047cbfe498d64b7241`.
Browser test correction: `ff897d59e66ca902de2e40f6ded183f0d95c2691`.
Later changes in the closure commit are documentation/archive changes only.
All commands below ran in the batch-4 integration worktree. The coordinator
owned the broad sessions and collected their actual exit codes; a log footer
alone was not used as the subprocess verdict. All listed gates completed.

Raw log directory (outside the retired worktree):

```text
/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-integration-tmp/resume-20260928
```

## Commands

```sh
go test --tags 'json1 fts5' ./... -count=1
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... ./application_context/... ./jobs/... -count=1
npx vitest run
npm run build-js
npm run build-css
./scripts/css-scan-test.sh
go build --tags 'json1 fts5'
make openapi
# cwd e2e
npm run test:with-server -- tests/jobs/background-download-form.spec.ts tests/jobs/job-center.spec.ts --project=default
npm run test:with-server:all
# A new tagged Go binary was built before this harness, which can reuse it.
npm run test:with-server:postgres
```

JS preceded CSS, and CSS scan completed before Go tests could create transient
test templates. OpenAPI was regenerated again after backend gates; YAML had no
diff. The Pin check used the actual merged drawer with a held same-version
older detail read, actual Pin command, then release: pinned state and Unpin
survive. The timeline check ran ten selected humanizer/timeline cases. The
clustering overlay constructed an actual PostgreSQL ownership fixture, asserted
the dialect, ran the L6 Reduction, verified title/success, and used follower-aware
cleanup. These controlled checks also exited zero.

## Preserved results

| Log | Result | SHA-256 of raw log |
|---|---|---|
| `final-go.log` | EXIT=0 | `18040028db8c4b0ca123c985089aa4f6d60bb835651d23167095c3900dd03105` |
| `final-go-pg.log` | EXIT=0 | `c68b68e100323c7bee2527c51d65a23644f2eca1db9ed10623caa8a6f0f46b85` |
| `final-vitest.log` | Test Files  107 passed (107); Tests  1859 passed (1859); EXIT=0 | `6ad42415ece263b7f8e82292b815dffa4403be4a638af7166f16f22c9f682c3a` |
| `final-build-js.log` | EXIT=0 | `91a508df8a16f7d6ace57ce92f217506867ebf554d890d8b197171bb0eb42893` |
| `final-build-css.log` | EXIT=0 | `2edc80a84702af87ed9a161a69df7bf8a983d304af3667629e1c6ec46de551b0` |
| `final-css-scan.log` | EXIT=0 | `2ddc4401cb502b98bb264d58f96906dceef45ed748ca611d6b7a8293e664a2bb` |
| `final-build-go.log` | EXIT=0 | `0e606818852cc6d4e60902e87575d34ac32240a5d6906a692c9213d2c43e035c` |
| `final-openapi-recheck.log` | EXIT=0 | `5b5c21d9aff05bc29bbad17d1d8ca327c37b5036b5441ae55cc02ad15723f396` |
| `final-pin-held-detail.log` | EXIT=0 | `3ff47ead8db1b2a4b72b3cc62e9db5ee5f6c8f9ff838ef3376c66b9931ed167a` |
| `final-timeline-wording.log` | Test Files  1 passed (1); Tests  10 passed | 13 skipped (23); EXIT=0 | `9bc51315d8bb53658ad01b50b354c9824a33f502ccc8946839455985f63cf6ef` |
| `final-clustering-title-pg.log` | EXIT=0 | `9514aca5462fa945696750c7da892a92857061dabde4c0c2385f89ad7b4ce8a9` |
| `merged-label-repair-focused.log` | 26 passed (16.5s); EXIT=0 | `7a77aeb052da80fba811dd6bd0634fc1afce0a75ecd61ca6d743ff37ff4563e8` |
| `final-before-pg-browser-build.log` | EXIT=0 | `0e606818852cc6d4e60902e87575d34ac32240a5d6906a692c9213d2c43e035c` |
| `final-e2e-all-labels.log` | 5 skipped; 2367 passed (6.2m); EXIT=0 | `79cf0283d8885f8c572c4582e54adfa6df23db75eb915155da2245a4ea50df5c` |
| `final-e2e-postgres.log` | 4 skipped; 2368 passed (6.0m); EXIT=0 | `45254a0d9dd46fbd28783dee00e365500ad4bff17c272149628b3891575a5a08` |

The final ALL run is **2,367 passed / five skipped / no retries**. The final
PostgreSQL run is **2,368 passed / four skipped / no retries**. The explicit
390 px long owner/category guard passed in both. Exact saved Playwright verdicts
are [ALL](final-e2e-all-labels.last-run.json) and
[PostgreSQL](final-e2e-postgres.last-run.json); both contain `status=passed` and
`failedTests=[]`.

## Earlier merged failure and scope

`final-e2e-all.log` on the same production source failed two assertions on all
three attempts: the background-form test expected a host title instead of its
filename, and Back-navigation setup expected the raw Kind instead of “Plugin
action”. Preserved DOM snapshots and the title/filter producers established
stale tests. The test-only correction retains exact filename article, canonical
`plugin-action` query value, form/notice/focus and Back/pinned checks. The 26
focused cases and complete fresh suites above pass. This is a proved assertion
mismatch, separate from earlier unexplained browser retries. The failed-run
screenshots/video/contexts and verdict remain under
`browser-artifacts-67bb-first-all` in the raw directory.

An optional E2E TypeScript-project check attempted by the correction worker
reported missing Node ambient names across E2E files (`process`, `path`,
`__dirname`). It is not one of the handoff gates above and was not established
as a baseline by a base-head control. This record does not relabel that check as
passing. The corrected specs execute in the focused and complete browser gates.

The raw scratch logs and probes are retained outside retired checkouts but are
not a reboot-safe archive. This committed result table, saved final browser
verdicts, final review reports and permanent regressions preserve the durable
conclusions. Full production/review history remains reachable by retained lane
branches and the integration merge.

## Closing record checks

After the completion checklist edit, `go test --tags 'json1 fts5'
./internal/arch -count=1` passed, EXIT=0. It verifies the preserved legacy
findings ledger and architecture rules. Raw log `final-archive-ledger.log`,
SHA-256 `777132b069954df52822b6de44b7d15aa12cb0e4e2a75f4122a558d396dda940`.

The final HTML was inspected in the in-app browser: 94 rendered issues; totals
92 fixed and two documented; the Documented filter contains exactly A11 and K4;
status options contain no remaining Batch 4 group. Desktop screenshot inspection
and page-width measurement found no horizontal overflow. The two final status
cells use two equal columns. This artifact check is separate from the explicit
390 px application-card regression in both full browser suites.
