# Lightbox Tagging Improvements — Plan Index

Six implementation plans to improve the tagging experience inside the custom Alpine lightbox.
Grounded in a recon pass plus UX, UI, and frontend-feasibility reviews. The lightbox is a custom
store (not baguetteBox); tagging already exists (Quick Tag panel, autocompleter, 9-slot numpads,
recents, optimistic writes, keyboard model), so every plan improves the existing surface.

## Plans

| Tier | File | Items | Layer | Effort |
|------|------|-------|-------|--------|
| 0 | [tier0-foundation.md](tier0-foundation.md) | Stop-blanking+prefetch; autocomplete scaling; duplicate-tag handling | FE + BE | S-M |
| 1 | [tier1-batch-pipeline.md](tier1-batch-pipeline.md) | Carry-forward (R); auto-advance flow mode; global undo | FE | S |
| 2 | [tier2-bottom-tag-dock.md](tier2-bottom-tag-dock.md) | Re-home chip input to a slim in-flow dock; demote numpad to expander | FE | M-L |
| 2 | [tier2-chip-input.md](tier2-chip-input.md) | Comma/space commit; backspace-removes-last; "Create X" row; pending state + tagpop/shake | FE | S-M |
| 3 | [tier3-suggested-tags.md](tier3-suggested-tags.md) | Context-aware suggested-tags endpoint + one-tap row | FE + BE | M |
| 3 | [tier3-tag-untagged.md](tier3-tag-untagged.md) | "Untagged" search predicate + "Tag untagged" launcher | FE + BE | M-L |

## Recommended build sequence

1. **Tier 0, Item 1 (stop-blanking + prefetch)** — load-bearing scale fix; everything else inherits the per-image flicker today. Pure frontend.
2. **Tier 0, Items 2-3 (autocomplete index/lean endpoint, duplicate handling)** — backend; needed before million-resource deployments feel the typeahead.
3. **Tier 1 (carry-forward, auto-advance, undo)** — cheap frontend wins that define the high-volume pipeline. Ship Item 4 first; Items 5-6 share one refactor and land together.
4. **Tier 3 suggested-tags** — marquee feature; the perceptual-hash data it needs already exists and is indexed.
5. **Tier 2 chip-input niceties** — small, independent, benefits all autocompleter forms.
6. **Tier 2 bottom-tag-dock** — biggest churn (moves substantial markup + updates ~12 specs); do last unless the heavy panel is the top complaint.

## Cross-plan decisions and gotchas captured in the plans

- **Tier 0 Item 3 changes a contract**: rewriting duplicate-tag behavior breaks `duplicate_tag_friendly_error_test.go`, which asserts the current 4xx. Intentional; the test is updated as part of the change.
- **Tier 1 undo collides with `z`**: `Cmd/Ctrl+Z` must be guarded with `!metaKey && !ctrlKey` so it does not also trigger the `z` tab-switch. Flow-mode announcements thread one combined message through `announcePosition` to avoid the 50ms latest-wins live-region clobber.
- **Tier 2 dock — no teleport**: the relocated slots popover must stay a DOM descendant of `[data-quick-tag-panel]`; `focusTagEditor()`, click-outside-collapse, and the `0`-key focus poll all rely on `querySelector` containment.
- **Tier 2 chip-input — protect the shared component**: comma always commits; space-commit is opt-in (`commitOnSpace`, default off) to keep multi-word tag names typeable; pending/animation visuals stay lightbox-only by riding the existing thenables so non-standalone forms are untouched.
- **Tier 3 untagged — engine-neutral SQL**: a correlated `NOT EXISTS` (mirrors the existing `ShowDhashZero` block), index-served by the `resource_tags` composite PK, composes with RBAC scope as just another WHERE.
- **Tier 3 suggested — fail-closed RBAC**: `scopedAPI`-wrapped, gated on a scoped `GetResource(id)`; both similarity and group queries are subtree-confined, so a confined principal only sees in-subtree-derived suggestions. Ranking: `0.6*normSimilar + 0.4*normGroup`, exclude already-applied, cap 8.

## Verification (per the plans)

- Frontend-only tiers (1, 2): `npm run build-js` then `cd e2e && npm run test:with-server:all`.
- Backend-touching tiers (0, 3): `go test --tags 'json1 fts5' ./...` and Postgres `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`, plus the E2E suites above and `:postgres`.
- All plans are TDD-ordered: the failing test is written before the change.
