# Adversarial review round 2 — feature/user-accounts-rbac

Four more P1s (relation-scoping theme not fully covered in round 1, plus CLI creds).
All fixed + regression-tested; verified SQLite + Postgres.

- [x] **Group detail leaks cross-scope relations** — `group_crud_context.go:370` GetGroup
      preloaded Relationships/BackRelations unfiltered. Now a scoped principal's preload confines each
      relation's far endpoint (to_group_id / from_group_id) to the subtree; fail-closed. Test:
      TestScopedUser_GroupDetailRelationsConfined.
- [x] **Clone creates relations to external groups** — `group_bulk_context.go:416,429` DuplicateGroup
      copied all out/in relations. Now skips relations whose far endpoint is outside the subtree (scoped
      only). Test: TestScopedUser_CloneSkipsExternalRelations.
- [x] **Merge raw-SQL relation transfer bypasses scope** — `group_bulk_context.go:81` Exec transfers.
      Now appends `AND to_group_id IN ?` / `AND from_group_id IN ?` for scoped principals so a loser's
      relation to an external group is not re-pointed at the winner. Test:
      TestScopedUser_MergeSkipsExternalRelations.
  - [x] **Follow-up (round 2b): merge also transferred the OTHER raw join tables unscoped** —
        `group_related_groups` (both directions), `groups_related_notes`, `groups_related_resources`
        were still copied wholesale, so a scoped user merging an in-scope loser could recreate a winner
        association to an external group/note/resource. Now every owner-scoped join table confines the
        far endpoint to the subtree (groups by id, notes/resources by owner_id) — no-op for
        unscoped/admin, fail-closed when the subtree is unresolvable. Extended
        TestScopedUser_MergeSkipsExternalRelations to cover all three join tables and added
        TestAdminMergeTransfersAllAssociations as an over-filtering backstop. Verified SQLite + Postgres.
- [x] **CLI credentials not origin-bound** — `client/client.go` stored one global token sent to any
      --server. Now stored as an origin→token JSON map; ResolveToken(baseURL)/StoreToken(baseURL,...)/
      ClearToken(baseURL) key on normalized origin. MR_TOKEN env stays an explicit global override.
      Tests: client_test.go (TestTokenOriginBinding, TestTokenEnvOverride) + end-to-end login smoke.

Verified: full Go suite green (SQLite + Postgres); staticcheck/gofmt clean; full browser+CLI E2E
running; origin-bound `mr auth login` smoke-tested.
