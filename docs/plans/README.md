# Plans & design docs

Design and implementation notes, one pair per feature. They are a historical
record of intent, not living documentation — when a plan and the code disagree,
the code wins. For how the system is actually put together see
[../architecture/](../architecture/) and the root [CLAUDE.md](../../CLAUDE.md).

## Conventions

- `YYYY-MM-DD-<slug>-design.md` — what we decided to build and why.
- `YYYY-MM-DD-<slug>-impl.md` — how it was built, usually with a task checklist.
- A slug with no suffix is a standalone plan with no separate design doc.
- The date is when the work started, not when it shipped.

## Active

- [2026-07-08-schema-editor-scalar-meta-edit-bug.md](2026-07-08-schema-editor-scalar-meta-edit-bug.md)
- [2026-07-26-headless-selector-core-refactor.md](2026-07-26-headless-selector-core-refactor.md)
- [2026-07-27-headless-selector-handover.md](2026-07-27-headless-selector-handover.md)

## Archive

Shipped or abandoned work, kept for the reasoning. 187 documents.

### MRQL (16)

- `2026-04-13` **block-mrql-shortcode** — [design](archive/2026-04-13-block-mrql-shortcode-design.md) [impl](archive/2026-04-13-block-mrql-shortcode-impl.md)
- `2026-04-22` **bug-backlog-c12-mrql-polish** — [plan](archive/2026-04-22-bug-backlog-c12-mrql-polish.md)
- `2026-07-04` **category-template-mrql-phase4** — [plan](archive/2026-07-04-category-template-mrql-phase4.md)
- `2026-03-30` **mrql-group-by** — [design](archive/2026-03-30-mrql-group-by-design.md) [impl](archive/2026-03-30-mrql-group-by-impl.md)
- `2026-06-27` **mrql-nlp-generation** — [design](archive/2026-06-27-mrql-nlp-generation-design.md) [impl](archive/2026-06-27-mrql-nlp-generation-impl.md)
- `2026-03-29` **mrql-owner-traversal** — [design](archive/2026-03-29-mrql-owner-traversal-design.md) [impl](archive/2026-03-29-mrql-owner-traversal-impl.md)
- `2026-03-28` **mrql-query-language** — [design](archive/2026-03-28-mrql-query-language-design.md) [impl](archive/2026-03-28-mrql-query-language-impl.md)
- `2026-04-11` **mrql-scope-filter** — [design](archive/2026-04-11-mrql-scope-filter-design.md) [impl](archive/2026-04-11-mrql-scope-filter-impl.md)
- `2026-04-09` **mrql-shortcodes** — [design](archive/2026-04-09-mrql-shortcodes-design.md) [impl](archive/2026-04-09-mrql-shortcodes-impl.md)

### Selectors & pickers (9)

- `2026-01-29` **gallery-resource-picker** — [design](archive/2026-01-29-gallery-resource-picker-design.md) [impl](archive/2026-01-29-gallery-resource-picker-impl.md)
- `2026-01-30` **generic-entity-picker** — [design](archive/2026-01-30-generic-entity-picker-design.md) [impl](archive/2026-01-30-generic-entity-picker-impl.md)
- `2026-03-22` **multi-tag-quick-slots** — [design](archive/2026-03-22-multi-tag-quick-slots-design.md) [impl](archive/2026-03-22-multi-tag-quick-slots-impl.md)
- `2026-03-23` **quick-slot-expansion** — [design](archive/2026-03-23-quick-slot-expansion-design.md) [impl](archive/2026-03-23-quick-slot-expansion-impl.md)
- `2026-07-01` **tier2-chip-input** — [plan](archive/2026-07-01-tier2-chip-input.md)

### Schema & metadata display (15)

- `2026-04-22` **bug-backlog-c15-schema-block-editor** — [plan](archive/2026-04-22-bug-backlog-c15-schema-block-editor.md)
- `2026-04-05` **display-renderers** — [design](archive/2026-04-05-display-renderers-design.md) [impl](archive/2026-04-05-display-renderers-impl.md)
- `2026-04-04` **labeled-enum** — [design](archive/2026-04-04-labeled-enum-design.md)
- `2026-04-04` **labeled-enums** — [plan](archive/2026-04-04-labeled-enums.md)
- `2026-04-04` **meta-subpath** — [design](archive/2026-04-04-meta-subpath-design.md) [impl](archive/2026-04-04-meta-subpath-impl.md)
- `2026-04-06` **metadata-display-remake** — [design](archive/2026-04-06-metadata-display-remake-design.md) [impl](archive/2026-04-06-metadata-display-remake-impl.md)
- `2026-03-31` **schema-driven-search-fields** — [design](archive/2026-03-31-schema-driven-search-fields-design.md) [impl](archive/2026-03-31-schema-driven-search-fields-impl.md)
- `2026-04-01` **schema-editor** — [design](archive/2026-04-01-schema-editor-design.md) [impl](archive/2026-04-01-schema-editor-impl.md)
- `2026-04-04` **schema-metadata-display** — [design](archive/2026-04-04-schema-metadata-display-design.md) [impl](archive/2026-04-04-schema-metadata-display-impl.md)

### Category templates & shortcodes (11)

- `2026-04-12` **block-shortcodes** — [design](archive/2026-04-12-block-shortcodes-design.md) [impl](archive/2026-04-12-block-shortcodes-impl.md)
- `2026-04-08` **category-section-config** — [design](archive/2026-04-08-category-section-config-design.md) [impl](archive/2026-04-08-category-section-config-impl.md)
- `2026-07-04` **category-template-authoring-phase1** — [plan](archive/2026-07-04-category-template-authoring-phase1.md)
- `2026-07-04` **category-template-composition-phase3** — [plan](archive/2026-07-04-category-template-composition-phase3.md)
- `2026-07-04` **category-template-language-phase2** — [plan](archive/2026-07-04-category-template-language-phase2.md)
- `2026-07-04` **category-template-robustness-phase5** — [plan](archive/2026-07-04-category-template-robustness-phase5.md)
- `2026-07-04` **category-template-surfaces-phase6** — [plan](archive/2026-07-04-category-template-surfaces-phase6.md)
- `2026-04-05` **shortcode-system** — [design](archive/2026-04-05-shortcode-system-design.md) [impl](archive/2026-04-05-shortcode-system-impl.md)

### Resources & media (18)

- `2026-04-06` **auto-detect-resource-category** — [design](archive/2026-04-06-auto-detect-resource-category-design.md) [impl](archive/2026-04-06-auto-detect-resource-category-impl.md)
- `2026-04-22` **bug-backlog-c3-image-hashing** — [plan](archive/2026-04-22-bug-backlog-c3-image-hashing.md)
- `2026-01-27` **image-similarity** — [design](archive/2026-01-27-image-similarity-design.md) [impl](archive/2026-01-27-image-similarity-impl.md)
- `2026-07-02` **image-similarity-v2-plan** — [plan](archive/2026-07-02-image-similarity-v2-plan.md)
- `2026-01-28` **lightbox-fullscreen-zoom** — [design](archive/2026-01-28-lightbox-fullscreen-zoom-design.md) [impl](archive/2026-01-28-lightbox-fullscreen-zoom-impl.md)
- `2026-01-25` **office-document-previews** — [design](archive/2026-01-25-office-document-previews-design.md)
- `2026-02-19` **paste-upload** — [design](archive/2026-02-19-paste-upload-design.md) [impl](archive/2026-02-19-paste-upload-impl.md)
- `2026-07-01` **paste-upload-group** — [plan](archive/2026-07-01-paste-upload-group.md)
- `2026-02-07` **resource-categories** — [plan](archive/2026-02-07-resource-categories.md)
- `2026-02-26` **resource-details-redesign** — [plan](archive/2026-02-26-resource-details-redesign.md)
- `2026-01-24` **resource-versioning** — [design](archive/2026-01-24-resource-versioning-design.md) [impl](archive/2026-01-24-resource-versioning-impl.md)
- `2026-01-26` **version-compare-ui** — [design](archive/2026-01-26-version-compare-ui-design.md) [plan](archive/2026-01-26-version-compare-ui.md)

### Notes, blocks & timeline (14)

- `2026-03-12` **at-mentions** — [design](archive/2026-03-12-at-mentions-design.md) [impl](archive/2026-03-12-at-mentions-impl.md)
- `2026-01-29` **block-editor** — [design](archive/2026-01-29-block-editor-design.md) [impl](archive/2026-01-29-block-editor-impl.md)
- `2026-04-22` **bug-backlog-c6-block-editor-a11y** — [plan](archive/2026-04-22-bug-backlog-c6-block-editor-a11y.md)
- `2026-01-30` **calendar-block** — [design](archive/2026-01-30-calendar-block-design.md)
- `2026-03-07` **note-bulk-actions** — [design](archive/2026-03-07-note-bulk-actions-design.md) [impl](archive/2026-03-07-note-bulk-actions-impl.md)
- `2026-01-29` **note-sharing** — [design](archive/2026-01-29-note-sharing-design.md) [impl](archive/2026-01-29-note-sharing-impl.md)
- `2026-04-09` **note-type-feature-parity** — [design](archive/2026-04-09-note-type-feature-parity-design.md) [impl](archive/2026-04-09-note-type-feature-parity-impl.md)
- `2026-03-22` **timeline-view** — [design](archive/2026-03-22-timeline-view-design.md) [impl](archive/2026-03-22-timeline-view-impl.md)

### Groups, export & import (12)

- `2026-04-22` **bug-backlog-c11-import-ux** — [plan](archive/2026-04-22-bug-backlog-c11-import-ux.md)
- `2026-04-22` **bug-backlog-c16-group-ux** — [plan](archive/2026-04-22-bug-backlog-c16-group-ux.md)
- `2026-03-21` **compare-merge-resources** — [design](archive/2026-03-21-compare-merge-resources-design.md) [impl](archive/2026-03-21-compare-merge-resources-impl.md)
- `2026-04-13` **entity-guid** — [design](archive/2026-04-13-entity-guid-design.md) [impl](archive/2026-04-13-entity-guid-impl.md)
- `2026-04-11` **group-export-import** — [design](archive/2026-04-11-group-export-import-design.md)
- `2026-04-11` **group-export-plan-a** — [plan](archive/2026-04-11-group-export-plan-a.md)
- `2026-04-13` **related-entity-export-import** — [design](archive/2026-04-13-related-entity-export-import-design.md) [impl](archive/2026-04-13-related-entity-export-import-impl.md)
- `2026-02-19` **tag-merge** — [design](archive/2026-02-19-tag-merge-design.md) [impl](archive/2026-02-19-tag-merge-impl.md)

### Auth, admin & sharing (8)

- `2026-03-22` **admin-overview** — [design](archive/2026-03-22-admin-overview-design.md) [impl](archive/2026-03-22-admin-overview-impl.md)
- `2026-07-01` **auth-audit** — [plan](archive/2026-07-01-auth-audit.md)
- `2026-04-22` **bug-backlog-c8-share-allowlist** — [plan](archive/2026-04-22-bug-backlog-c8-share-allowlist.md)
- `2026-04-22` **bug-backlog-c9-share-surface** — [plan](archive/2026-04-22-bug-backlog-c9-share-surface.md)
- `2026-07-01` **root-admin-invariant** — [plan](archive/2026-07-01-root-admin-invariant.md)
- `2026-01-27` **user-documentation** — [design](archive/2026-01-27-user-documentation-design.md) [impl](archive/2026-01-27-user-documentation-impl.md)

### Plugins (20)

- `2026-04-10` **data-views-extended-sources** — [design](archive/2026-04-10-data-views-extended-sources-design.md) [impl](archive/2026-04-10-data-views-extended-sources-impl.md)
- `2026-02-26` **lua-plugin-system** — [design](archive/2026-02-26-lua-plugin-system-design.md) [impl](archive/2026-02-26-lua-plugin-system-impl.md)
- `2026-04-26` **multi-resource-plugin-inputs** — [design](archive/2026-04-26-multi-resource-plugin-inputs-design.md) [impl](archive/2026-04-26-multi-resource-plugin-inputs-impl.md)
- `2026-03-03` **plugin-actions** — [design](archive/2026-03-03-plugin-actions-design.md) [impl](archive/2026-03-03-plugin-actions-impl.md)
- `2026-03-02` **plugin-activation-settings** — [design](archive/2026-03-02-plugin-activation-settings-design.md) [impl](archive/2026-03-02-plugin-activation-settings-impl.md)
- `2026-03-06` **plugin-api-endpoints** — [design](archive/2026-03-06-plugin-api-endpoints-design.md) [impl](archive/2026-03-06-plugin-api-endpoints-impl.md)
- `2026-03-07` **plugin-block-types** — [design](archive/2026-03-07-plugin-block-types-design.md) [impl](archive/2026-03-07-plugin-block-types-impl.md)
- `2026-03-04` **plugin-entity-crud** — [design](archive/2026-03-04-plugin-entity-crud-design.md) [impl](archive/2026-03-04-plugin-entity-crud-impl.md)
- `2026-03-05` **plugin-kv-store** — [design](archive/2026-03-05-plugin-kv-store-design.md) [impl](archive/2026-03-05-plugin-kv-store-impl.md)
- `2026-03-02` **plugin-menus-pages** — [design](archive/2026-03-02-plugin-menus-pages-design.md) [impl](archive/2026-03-02-plugin-menus-pages-impl.md)

### CLI (mr) (11)

- `2026-03-14` **cli-e2e-tests** — [design](archive/2026-03-14-cli-e2e-tests-design.md) [impl](archive/2026-03-14-cli-e2e-tests-impl.md)
- `2026-04-10` **cli-feature-parity** — [plan](archive/2026-04-10-cli-feature-parity.md)
- `2026-03-14` **mr-cli** — [design](archive/2026-03-14-mr-cli-design.md) [impl](archive/2026-03-14-mr-cli-impl.md)
- `2026-04-14` **mr-cli-docs** — [design](archive/2026-04-14-mr-cli-docs-design.md) [impl](archive/2026-04-14-mr-cli-docs-impl.md)
- `2026-04-15` **mr-cli-docs-phase3** — [design](archive/2026-04-15-mr-cli-docs-phase3-design.md) [impl](archive/2026-04-15-mr-cli-docs-phase3-impl.md)
- `2026-04-15` **mr-cli-docs-phase4** — [design](archive/2026-04-15-mr-cli-docs-phase4-design.md) [impl](archive/2026-04-15-mr-cli-docs-phase4-impl.md)

### Tagging (11)

- `2026-02-11` **context-aware-popular-tags** — [design](archive/2026-02-11-context-aware-popular-tags-design.md)
- `2026-02-28` **quick-tag-panel** — [design](archive/2026-02-28-quick-tag-panel-design.md) [impl](archive/2026-02-28-quick-tag-panel-impl.md)
- `2026-03-01` **tag-editor-relocation** — [design](archive/2026-03-01-tag-editor-relocation-design.md) [plan](archive/2026-03-01-tag-editor-relocation.md)
- `2026-07-01` **tagging-plans-index** — [plan](archive/2026-07-01-tagging-plans-index.md)
- `2026-07-01` **tier0-foundation** — [plan](archive/2026-07-01-tier0-foundation.md)
- `2026-07-01` **tier1-batch-pipeline** — [plan](archive/2026-07-01-tier1-batch-pipeline.md)
- `2026-07-01` **tier2-bottom-tag-dock** — [plan](archive/2026-07-01-tier2-bottom-tag-dock.md)
- `2026-07-01` **tier3-suggested-tags** — [plan](archive/2026-07-01-tier3-suggested-tags.md)
- `2026-07-01` **tier3-tag-untagged** — [plan](archive/2026-07-01-tier3-tag-untagged.md)

### Bug backlogs & audits (23)

- `2026-07-01` **adversarial-review** — [plan](archive/2026-07-01-adversarial-review.md)
- `2026-07-01` **adversarial-review-2** — [plan](archive/2026-07-01-adversarial-review-2.md)
- `2026-07-01` **adversarial-review-3** — [plan](archive/2026-07-01-adversarial-review-3.md)
- `2026-02-20` **audit-next-steps** — [plan](archive/2026-02-20-audit-next-steps.md)
- `2026-02-17` **audit-p0-p2** — [design](archive/2026-02-17-audit-p0-p2-design.md) [impl](archive/2026-02-17-audit-p0-p2-impl.md)
- `2026-02-20` **audit2-next-steps** — [plan](archive/2026-02-20-audit2-next-steps.md)
- `2026-04-22` **bug-backlog-c1-error-hygiene** — [plan](archive/2026-04-22-bug-backlog-c1-error-hygiene.md)
- `2026-04-22` **bug-backlog-c10-jobs-ui-polish** — [plan](archive/2026-04-22-bug-backlog-c10-jobs-ui-polish.md)
- `2026-04-22` **bug-backlog-c13-cosmetic-cleanup** — [plan](archive/2026-04-22-bug-backlog-c13-cosmetic-cleanup.md)
- `2026-04-22` **bug-backlog-c14-ingestion-safety** — [plan](archive/2026-04-22-bug-backlog-c14-ingestion-safety.md)
- `2026-04-22` **bug-backlog-c17-a11y-batch-3** — [plan](archive/2026-04-22-bug-backlog-c17-a11y-batch-3.md)
- `2026-04-22` **bug-backlog-c18-obs-search-docs** — [plan](archive/2026-04-22-bug-backlog-c18-obs-search-docs.md)
- `2026-04-22` **bug-backlog-c2-form-ux** — [plan](archive/2026-04-22-bug-backlog-c2-form-ux.md)
- `2026-04-22` **bug-backlog-c4-deletion-cascade** — [plan](archive/2026-04-22-bug-backlog-c4-deletion-cascade.md)
- `2026-04-22` **bug-backlog-c5-jobs-ui-a11y** — [plan](archive/2026-04-22-bug-backlog-c5-jobs-ui-a11y.md)
- `2026-04-22` **bug-backlog-c7-alt-fs** — [plan](archive/2026-04-22-bug-backlog-c7-alt-fs.md)
- `2026-04-22` **bug-backlog-master** — [plan](archive/2026-04-22-bug-backlog-master.md)
- `2026-04-22` **bug-backlog-triage** — [design](archive/2026-04-22-bug-backlog-triage-design.md)
- `2026-04-22` **bughunt-batch-c9-c18** — [design](archive/2026-04-22-bughunt-batch-c9-c18-design.md)
- `2026-07-01` **entity-detail-audit** — [plan](archive/2026-07-01-entity-detail-audit.md)
- `2026-02-20` **kan-audit-bugfixes** — [plan](archive/2026-02-20-kan-audit-bugfixes.md)
- `2026-07-01` **review-followup** — [plan](archive/2026-07-01-review-followup.md)

### Docs, testing & performance (9)

- `2026-03-07` **docs-perfection** — [design](archive/2026-03-07-docs-perfection-design.md) [impl](archive/2026-03-07-docs-perfection-impl.md)
- `2026-03-08` **docs-perfection-v3** — [design](archive/2026-03-08-docs-perfection-v3-design.md) [impl](archive/2026-03-08-docs-perfection-v3-impl.md)
- `2026-07-01` **docs-refresh** — [plan](archive/2026-07-01-docs-refresh.md)
- `2026-03-04` **docs-team** — [design](archive/2026-03-04-docs-team-design.md) [impl](archive/2026-03-04-docs-team-impl.md)
- `2026-03-29` **postgres-testcontainers** — [design](archive/2026-03-29-postgres-testcontainers-design.md) [impl](archive/2026-03-29-postgres-testcontainers-impl.md)

### Other (10)

- `2026-03-02` **dashboard** — [design](archive/2026-03-02-dashboard-design.md) [impl](archive/2026-03-02-dashboard-impl.md)
- `2026-03-10` **design-language-migration** — [design](archive/2026-03-10-design-language-migration-design.md) [plan](archive/2026-03-10-design-language-migration.md)
- `2026-02-17` **maintainability-cleanup** — [design](archive/2026-02-17-maintainability-cleanup-design.md) [impl](archive/2026-02-17-maintainability-cleanup-impl.md)
- `2026-04-22` **runtime-settings** — [design](archive/2026-04-22-runtime-settings-design.md) [impl](archive/2026-04-22-runtime-settings-impl.md)
- `2026-02-14` **series-entity** — [design](archive/2026-02-14-series-entity-design.md) [impl](archive/2026-02-14-series-entity-impl.md)

