# Phase 5: Template Robustness and Consistency

Plan: `docs/plans/category-template-robustness-phase5.md`

## Work item 1 — Visible failure markers (shortcodes package) ✅ DONE
- [x] `shortcodes/markers.go`: `shortcodeErrorMarker` (inline span, escaped), `shortcodeComment`
- [x] processor.go depth cap → content as-is + `<!-- mr:shortcode depth limit reached -->` (only if shortcodes remain)
- [x] processor.go `[mrql]` nil executor → `<!-- mr:mrql unavailable in this context -->`
- [x] processor.go plugin renderer nil → `<!-- mr:plugin unavailable in this context -->`
- [x] processor.go plugin renderer error → marker `⚠ plugin:foo:bar` (error in title)
- [x] processor.go malformed plugin name → marker (defensive; parser regex makes it unreachable via Process — no test)
- [x] conditional_handler.go non-block `[conditional]` → marker
- [x] Updated stale raw-leak tests + new table-driven marker/comment tests
- Lint coverage: unclosed `[conditional]` already flagged by BlockRequired rule; plugin runtime errors are not statically lintable

## Work item 2 — Per-page MRQL query budget ✅ DONE
- [x] config `MRQLPageQueryBudget`, flag `-mrql-page-query-budget`, env `MRQL_PAGE_QUERY_BUDGET`, default 200, 0 disables
- [x] runtime setting `KeyMRQLPageQueryBudget` (0–100000, AllowZero) mirroring `mrql_default_limit`
- [x] exported accessor `appCtx.MRQLPageQueryBudget()`
- [x] `shortcodes/query_budget.go`: QueryBudget + BudgetedExecutor (cache dedup, count misses, clone to avoid alias), one-shot MarkExceeded
- [x] `BuildQueryExecutor` uses BudgetedExecutor + one-warning-per-page log (entity type mrql)
- [x] attach budget in shortcode_tag.go + custom_css_tag.go
- [x] unit tests (limit, cache-hit-no-increment, clone isolation, disabled, key distinctness)
- [x] E2E `mrql-page-query-budget.spec.ts` (per-card trips budget; dedup does not) — PASS
- [x] CLAUDE.md config table + docs-site advanced.md + runtime-settings.md + shortcodes.md notes
- Fixed setting-count tests (12→13): runtime_setting_spec_test.go, admin_settings_test.go

## Work item 3 — CustomAvatar semantics (docs only) ✅ DONE
- [x] createResourceCategory.tpl → "...resources keep their thumbnail"
- [x] createCategory.tpl / createNoteType.tpl already accurate ("Replaces the default initials avatar")
- [x] docs-site custom-templates.md per-carrier clarification
- [x] no model/template changes (explicit non-goal honored)

## Verification
- [x] `go test --tags 'json1 fts5' ./...` — green
- [x] rebuild `./mahresources` — OK
- [x] E2E browser + CLI (`test:with-server:all`) — 1668 passed, 1 known-flaky (lightbox), 0 unexpected after fixing 2 setting-count assertions (admin-settings.spec.ts, cli/admin-settings-list.spec.ts)
- [x] Postgres suites (`json1 fts5 postgres` mrql + api_tests) — green

## Review
- Work item 1: raw shortcode leaks replaced by inline `shortcode-error` markers (plugin errors, unclosed [conditional]) and `<!-- mr:… -->` comments (depth cap, absent executor/renderer). Unblocks Phase 6 share rendering.
- Work item 2: per-page inline-MRQL query budget via a context-threaded QueryBudget + BudgetedExecutor (cache dedup, count misses, clone-on-store/lookup to avoid aliasing). Config flag + runtime setting + accessor + one warning/page. Default 200 (default page size 50 × 3-query summary = 150 < 200; only trips genuinely heavy pages).
- Work item 3: docs-only. Resource-category CustomAvatar description clarified (kept its per-carrier semantics); docs-site updated. No model/template changes.
