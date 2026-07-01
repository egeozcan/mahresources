# PR #55 review follow-up (post-Group-B/C/D)

Five findings from the review — all fixed + regression-tested, committed `ae5b3c88`,
pushed. Go suite green (SQLite + Postgres); staticcheck/gofmt clean; auth + download
E2E green; `mr auth login` smoke-tested end-to-end against an auth-on server.

- [x] **#3 P1 CLI login CSRF (REGRESSION from Group B)** — `cmd/mr/commands/auth.go`
      `loginAndMintToken`: cookie login then POST /v1/account/tokens w/o CSRF header → 403 with auth on.
      Fix: GET /v1/auth/me for csrfToken, send `X-CSRF-Token` on the mint POST.
- [x] **#1 P1 Dashboard scope bypass** — `application_context/dashboard_context.go:65`
      `GetRecentActivity` raw SQL bypasses GORM scope callback. Apply `subtreeScopeIDs()` to
      resources/notes/groups sub-selects (tags are global labels, left visible). Called on scoped ctx.
- [x] **#2 P1 Queued download scope bypass** — `server/routes.go:582` + `download_queue_handlers.go`
      Worker creates resources unscoped; scoped user can set out-of-subtree OwnerId/Groups, or create a
      top-level group via GroupName. Fix: wrap submit routes with `scopedAPI`; for scoped principals
      require visible OwnerId, visible Groups, and reject GroupName (no subtree-safe placement).
- [x] **#4 P2 Download job mutation ownership** — `download_queue_handlers.go` cancel/pause/resume/retry
      call manager directly. Fix: 404 if the job isn't visible to the principal (owner/admin).
- [x] **#5 P1 Import lifecycle ownership** — `import_api_handlers.go` plan/result/apply/delete
      ignore the parse job's owner. Fix: 404 when the job is known and not visible to the principal.
