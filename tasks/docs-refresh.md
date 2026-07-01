# Docs-site refresh — catch up on May–June 2026 features

## Context
- docs-site last had a *content* push in Mar–Apr 2026. Zero docs commits in May, only 4 in June.
- Several user-facing features landed in that gap and were never documented (the auth commit only added CLI command pages, no narrative).

## Confirmed gaps (feature landed -> docs missing)
1. Authentication & RBAC (ab6d826b, 2026-06-23) — MAJOR. Only CLI pages exist. No feature page, no config flags.
2. fal.ai built-in plugin models (311ae7ae 05-10, e07f4d91, 77412a54) — built-in-plugins.md lists only `upscale`.
3. Custom thumbnail upload (0de0650c, 2026-05-23) — thumbnail-generation.md covers only auto-generation.
4. Image crop + video trim as new versions (8cacfe93; video trim 2026-06-13) — managing-resources.md covers only Rotate.
5. CustomCSS slot (2f80c42b, 2026-06-21) — custom-templates.md lists other slots but not CustomCSS.
6. Note type detail surfaces (36a3ec55, 627ba3f2, 2026-06-27) — notes-using-a-type + type's own config now shown.
7. Versioning: auto-select both on Compare (2026-05-05) — minor.

Already documented (skip): MRQL natural-language generation (80c87a59), auth CLI commands.

## Team (non-overlapping file ownership)
- [ ] Agent A — Auth/RBAC: NEW features/authentication.md; edit configuration/overview.md, sidebars.ts, intro.md, deployment/reverse-proxy.md
- [ ] Agent B — Resource editing/versioning: edit user-guide/managing-resources.md, features/thumbnail-generation.md, features/versioning.md
- [ ] Agent C — Plugins/templates/note-type config: edit features/built-in-plugins.md, features/custom-templates.md, features/meta-schemas.md

## Verify after
- [ ] `cd docs-site && npm run build` passes (no broken links / MDX errors)
- [ ] New page registered in sidebar
- [ ] git status review of all changed files

## Review
Done. 3 agents, non-overlapping files, all claims verified against source.

Changed (11 files):
- NEW features/authentication.md — full RBAC page (roles, scoping, sessions, CSRF, rate-limiting, flag table)
- configuration/overview.md, intro.md, deployment/reverse-proxy.md, sidebars.ts — auth flags + cross-links + sidebar registration
- features/built-in-plugins.md — fal.ai: 6 actions, all upscale/restore/edit/generate models, output mode, multi-image
- features/thumbnail-generation.md + user-guide/managing-resources.md — custom thumbnail upload; crop; video trim
- features/versioning.md — edits-that-create-versions; auto-select-both-on-compare
- features/custom-templates.md + features/meta-schemas.md — CustomCSS slot; note-type detail surfaces

Verified against source:
- auth routes /login /logout /account /admin/users /v1/auth/me, models/user_model.go ScopeGroupId
- fal.ai model lists + 6 action ids in plugins/fal-ai/plugin.lua (incl. polish)
- /v1/resource/preview POST/DELETE, multipart field "thumbnail" (resource_api_handlers.go:582)
- 1920px / JPEG-85 in resource_custom_thumbnail_context.go; crop /v1/resources/crop; trim /v1/resources/trim

`cd docs-site && npm run build` -> SUCCESS (onBrokenLinks:'throw', so all internal links resolve).
Not committed (per workflow — commit only when asked). build/ is gitignored.
