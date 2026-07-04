# Phase 3: Template Composition and Reuse

Plan: `docs/plans/category-template-composition-phase3.md`

## Work item 1 — `[each]` iteration over meta arrays  ✅ DONE
- [x] Parser: add `each` (block) + `item` (inline) to shortcodePattern/closingTagPattern
- [x] `shortcodes/each_handler.go`: RenderEachShortcode + item substitution (skip nested each)
- [x] processor.go: dispatch `each`/`item`
- [x] builtin_docs.go: `each` + `item` doc entries
- [x] lint.go: `[item]` outside `[each]` warning + typo names + each path completion (JS)
- [x] Tests: each_handler_test.go + lint tests + docs API tests (pass)

## Work item 2 — `[partial]` reusable snippets  ✅ DONE
- [x] Model `TemplatePartial` + AutoMigrate + stampedModels() + CLAUDE.md count (14→15)
- [x] Resolver via request-context (WithPartialResolver) — DEVIATION from struct refactor:
      threaded through reqCtx (WithMRQLCache pattern, plan item 7) — no signature/test churn,
      DB-free shortcodes pkg. Injected at all 5 render sites.
- [x] processor.go: `partial` case, BuildPartialResolver + per-render cache
- [x] Parser: add `partial` (inline); builtin_docs + lint (self-reference via PartialName)
- [x] CRUD: model/query/scope/context/interfaces/handlers, routes /v1/templatePartial(s),
      OpenAPI, template pages (list/create/display), context providers, nav + stats card
- [x] Authorization: isTaxonomyPath prefix (admin-only writes, reads open)
- [x] Tests: partial_handler_test, lint self-ref, API CRUD + kebab reject + unique, authz matrix — all green
- [ ] docs-site "reusing templates" page (deferred to docs pass)

## Work item 3 — Template-set duplication and export/import  ✅ DONE
- [x] Single-item slots exposed via list endpoints (full objects) — no new GET needed
- [x] `templateBundle` Alpine component + `templateBundleTools.tpl` (all 3 forms)
- [x] "Copy from…" (same + cross carrier), Export (bundle JSON v1), Import (carrier + schemaVersion checks)
- [x] Cross-carrier fills shared fields only; SectionConfig same-carrier via Alpine.$data

## Work item 4 — Starter template gallery  ✅ DONE
- [x] 4 preset bundles server/template_presets/*.json (go:embed) — lint-clean regression fixture
- [x] GET /v1/templatePresets + OpenAPI
- [x] "start from preset" picker → same client import path
- [x] E2E 97-template-composition-phase3: each/partial/unknown-partial/preset-import — 4 pass
- [x] presets_test.go: parse + carrier + shortcode-lint every slot

## Verification
- [x] Go unit (json1 fts5) — full suite green; rebuilt ./mahresources
- [x] E2E: 97-phase3 (4), carrier forms 02/03/21 (32), shortcodes (30), lint/autocomplete/template-authoring (7)
- [x] a11y: 01-a11y-pages (177, incl. new templatePartial list/create + modified forms)
- [x] CLI E2E: admin/stats + related (22) green
- [x] Postgres suite (MRQL + api_tests) green — fixed PG migration list gap
- [x] docs-site: shortcodes.md (each/item/partial) + custom-templates.md (Reusing templates); OpenAPI drift green (no committed spec)

## Migration-list gaps fixed (stampedModels + GetDataStats count reach every test DB)
- user_context_test, user_admin_guard_test, created_by_stamp_test, admin_context_test,
  api_test_utils (SetupTestEnv), pg_test_helper_test — all add &models.TemplatePartial{}

## Notable deviations / follow-ups
- Partial resolver threaded via reqCtx (WithPartialResolver) instead of the planned Handlers
  struct — same goal, far less churn, matches WithMRQLCache pattern (plan item 7).
- PartialName self-reference lint capability exists + tested; not yet wired to a per-partial
  preview endpoint (partials use the generic code editor). Runtime recursion is bounded anyway.
- CLI commands for partials: intentionally not added (plan lists as optional follow-up).
