# Batch 4 baseline (master bdafcb51, the batch 3 integration run)

Gates on the merged batch 3 tree (logs: $SP/integ-logs/*-b3r1.*):
- vitest: 99 files, 1682 tests passed.
- Go (json1 fts5, ./...): EXIT=0.
- Postgres Go (mrql, api_tests, application_context, jobs): EXIT=0.
- E2E all (browser + CLI): 2282 passed, 3 failed; E2E Postgres: 2283 passed, 1 failed, 1 flaky (phase3-sweeps mobile overflow on /resources, passed on retry).

The failures are pre-existing and depend on the time of day. The run was at 00:10–00:25 CEST, when the local date is a day ahead of UTC. They fail identically on master 54fb4533 at that hour, and they passed in every run before midnight:
- e2e/tests/plugins/plugin-project-management.spec.ts:268 (overdue semantics).
- cli-doctest: the `mr groups timeline`, `mr queries timeline` and `mr resources timeline` examples, on both the plain and the auth-enabled server.
b4-flakes owns them. Anyone else who sees them between local and UTC midnight: note it and move on.

Other intermittent flakes seen during batch 3 (b4-flakes owns them): calendar-event-modal-a11y :61 and :85, compare-page-teardown :553, ws10-global-chrome :75, phase3-sweeps mobile overflow on /resources, and PG TestAReductionComputeAcceptsADurableJobAndPublishesItsReduction (a TempDir cleanup race under load).
