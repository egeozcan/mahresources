# Bug-Backlog Cleanup — Master Orchestration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to execute each cluster plan one at a time. Each cluster is a separate worktree + PR. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 13 Major+Medium bugs in `tasks/bug-hunt-log.md` as 8 merged PRs.

**Architecture:** File-location clustered bug fixes. Each cluster runs in its own git worktree off `master`. TDD per bug with 3× determinism checks. Self-merge on green. Autonomous end-to-end.

**Tech Stack:** Go 1.21+ (build tags `json1 fts5`), Gorilla Mux, Pongo2, Alpine.js, Playwright E2E, SQLite + Postgres.

**Spec reference:** `docs/superpowers/specs/2026-04-22-bug-backlog-triage-design.md`

---

## Cluster Index (execution order)

| # | Plan file | Bugs | Dependency |
|---|---|---|---|
| 1 | `2026-04-22-bug-backlog-c1-error-hygiene.md` | BH-P05, BH-019 | none |
| 2 | `2026-04-22-bug-backlog-c2-form-ux.md` | BH-006, BH-009 | none |
| 3 | `2026-04-22-bug-backlog-c3-image-hashing.md` | BH-011 → BH-018 | internal: BH-011 before BH-018 |
| 4 | `2026-04-22-bug-backlog-c4-deletion-cascade.md` | BH-020, BH-024 | none |
| 5 | `2026-04-22-bug-backlog-c5-jobs-ui-a11y.md` | BH-025, BH-026, BH-028 | none |
| 6 | `2026-04-22-bug-backlog-c6-block-editor-a11y.md` | BH-027 | none |
| 7 | `2026-04-22-bug-backlog-c7-alt-fs.md` | BH-023 | none |
| 8 | `2026-04-22-bug-backlog-c8-share-allowlist.md` | BH-031 | none |

Execute in the order listed. Each cluster ends with a merge to `master`.

---

## Pre-flight (before Cluster 1)

- [ ] **Step 1: Confirm clean master baseline**

```bash
cd /Users/egecan/Code/mahresources
git fetch origin
git checkout master
git pull --ff-only
git status --short  # only pre-existing untracked tasks/ and modified config files allowed
```

Expected: `master` is up-to-date. Any uncommitted changes in user's working copy stay untouched.

- [ ] **Step 2: Confirm full test suite is green on baseline**

```bash
go test --tags 'json1 fts5' ./...
```

Expected: PASS. If any pre-existing failures, STOP and escalate — cluster tests can't be trusted on a red baseline. Do NOT "fix while you're at it" — that's out of scope and must be escalated.

- [ ] **Step 3: Confirm Docker is running (for final Postgres gate)**

```bash
docker info | head -5
```

Expected: Docker Desktop / daemon responding. If not, note that final Postgres gate will be deferred, but clusters can still proceed.

---

## Per-cluster execution template (applied to each of C1–C8)

For **each** cluster plan in order:

- [ ] **Step A: Create isolated worktree**

Use `superpowers:using-git-worktrees` skill. Branch name matches the cluster's plan (e.g. `bugfix/c1-error-hygiene`). Worktree path under `../mahresources-worktrees/<branch>`.

- [ ] **Step B: Execute the cluster plan**

Work through every task in the cluster plan's file. Each task has its own TDD cycle (fail 3×, fix, pass 3×).

- [ ] **Step C: Cluster-level test gate**

```bash
cd <worktree>
go test --tags 'json1 fts5' ./...
```

Expected: PASS. If fail, diagnose and fix within the cluster (do NOT proceed to next cluster).

Additionally run the cluster's targeted E2E surface:

```bash
cd <worktree>/e2e
npm run test:with-server -- --grep "<cluster-tag>"
```

- [ ] **Step D: Rebase on latest master, run full suite**

```bash
cd <worktree>
git fetch origin
git rebase origin/master
go test --tags 'json1 fts5' ./...
cd e2e && npm run test:with-server:all
cd ../ && go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1
cd e2e && npm run test:with-server:postgres
```

Expected: ALL PASS.

- [ ] **Step E: Open PR, verify all gates in PR body, self-merge**

```bash
cd <worktree>
gh pr create --title "fix(area): BH-<IDs> — <short>" --body "$(cat <<'EOF'
Closes BH-<id> ...
[see each cluster plan for full body]
EOF
)"
gh pr merge --merge --delete-branch
```

- [ ] **Step F: Update bug-hunt-log and clean up**

Move merged BH-IDs from the active section to the "Fixed / closed pre-existing" section of `tasks/bug-hunt-log.md` with merge SHA + date. Commit directly to master:

```bash
git checkout master
git pull
# edit tasks/bug-hunt-log.md
git add tasks/bug-hunt-log.md
git commit -m "chore(bughunt): mark BH-<IDs> fixed (<cluster>, merged <sha>)"
git push
```

Then delete the worktree:

```bash
git worktree remove ../mahresources-worktrees/bugfix/c<N>-<slug>
```

- [ ] **Step G: Proceed to next cluster**

No user check-in. Start Step A for the next cluster.

---

## Final gate (after Cluster 8 merges)

- [ ] **Step 1: Checkout fresh master**

```bash
cd /Users/egecan/Code/mahresources
git checkout master
git pull --ff-only
```

- [ ] **Step 2: Full regression sweep**

```bash
go test --tags 'json1 fts5' ./...
cd e2e && npm run test:with-server:all
cd .. && go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1
cd e2e && npm run test:with-server:postgres
```

Expected: ALL PASS on new `master`.

- [ ] **Step 3: Completion report**

Write summary to `tasks/bug-backlog-cleanup-report.md` covering:

- 8 merged PR SHAs and URLs
- All 13 BH-IDs closed (with merge SHA per bug)
- Total new tests added (count by type: Go unit, Go API, Playwright E2E)
- Any tests that flaked and were deflaked (with root cause)
- Any bugs escalated (should be zero in nominal case)
- Any new bugs discovered and logged (with BH-IDs for unverified entries)
- Total wall-clock elapsed

Commit report to master, announce completion to user.

---

## Rollback procedure (escalation-only)

If a cluster PR is merged and subsequent regression sweep finds a regression introduced by that cluster:

- [ ] **Step 1: Revert the PR merge**

```bash
git checkout master
git revert -m 1 <merge-sha>
git commit -m "revert: <cluster name> due to <regression>"
git push
```

- [ ] **Step 2: Reopen the bugs**

Move affected BH-IDs back to the active section of `tasks/bug-hunt-log.md` with a note referencing the revert.

- [ ] **Step 3: Escalate to user**

This constitutes a Section 8.2 escalation in the design spec. Stop and ask the user for guidance before re-attempting.

---

## Inventory of tests to be added

Running total across all clusters (rough pre-count; refine in each plan):

- **C1:** 1 Go API test (BH-P05 extension) + 6 Go unit tests (BH-019, one per entity type) = **7 tests**
- **C2:** 6 Playwright specs (BH-006, one per entity form) + 3 Go API tests (BH-006 JSON-path safety) + 3 Playwright specs (BH-009, required / pattern / type-mismatch) = **12 tests**
- **C3:** 2 Go API tests (BH-011: rejected bad image, valid image accepted) + 2 Go unit tests (BH-018: solid-color rejection, near-dupe acceptance) = **4 tests**
- **C4:** 4 Go API tests (BH-020, one per block type) + 1 Go API test (BH-024 err-translator) + 1 migration test = **6 tests**
- **C5:** 3 Playwright specs (BH-025 reload, BH-026 completed-export link, BH-028 panel dialog + progressbar ARIA) = **3 tests**
- **C6:** 1 Playwright spec (axe-core sweep of block editor) + 4 targeted assertions = **1 test file, 4 assertion blocks**
- **C7:** 2 Go API tests (BH-023 multipart PathName, export/import round-trip) + 1 Playwright spec (storage select visible) = **3 tests**
- **C8:** 1 Go API test (BH-031 block-type allowlist) = **1 test**

**Grand total: ~36 tests added.**

---

## Notes on autonomy

- Every cluster's plan file says "execute autonomously" and references the design spec's § 8 autonomy contract.
- If a subagent hits a hard-stop condition (see design § 8.2), it pauses and escalates — not the orchestrator.
- The orchestrator (this plan) simply sequences the clusters and gates the final regression sweep.
