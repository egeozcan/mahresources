# b4-flakes: the date-boundary failures and the remaining flakes

Root-cause each one. A retry or a longer timeout is not a fix unless the cause is shown to be timing the test can't control. Measure before and after, on base and branch, under the same load.

1. **MUST FIX: date-boundary failures.** Between local midnight and UTC midnight (00:00 to 02:00 CEST here), these fail on master every time, and pass at other hours:
   - `e2e/tests/plugins/plugin-project-management.spec.ts:268` ("effective status and overdue semantics agree across native and board surfaces"): no task summary reads "overdue".
   - cli-doctest (`e2e/tests/cli/cli-doctest.spec.ts`, both servers): the `mr groups timeline`, `mr queries timeline` and `mr resources timeline` examples.
   Likely a local-versus-UTC date comparison (see the memory note: local-offset text makes RFC 3339 bounds compare lexicographically on SQLite). Decide for each whether the product or the test is wrong; a product bug gets a product fix. Reproduce at any hour by running the server and the tests with a TZ whose date differs from UTC (for example `TZ=Pacific/Kiritimati` or `TZ=Etc/GMT-14`), and add that as a regression guard.
2. `calendar-event-modal-a11y` :61 (PG E2E) and :85 (E2E all).
3. `compare-page-teardown` :553, `ws10-global-chrome` :75, and `phase3-sweeps` mobile overflow on /resources (PG, passed on retry).
4. PG `TestAReductionComputeAcceptsADurableJobAndPublishesItsReduction`: a TempDir cleanup race under load (10/10 alone).
