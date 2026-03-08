# Doc Checker B Report: Features, API, Config, Deployment

**Summary**: 26 KEEP, 6 EDIT, 0 REWRITE. 10 total issues across 32 files. Zero AI-slop found.

---

## features/versioning.md — KEEP (0 issues)

## features/image-similarity.md — KEEP (0 issues)

## features/saved-queries.md — KEEP (0 issues)
CodeMirror 6 editor with SQL syntax highlighting and schema autocompletion documented at line 125.

## features/custom-templates.md — KEEP (0 issues)

## features/meta-schemas.md — KEEP (0 issues)

## features/note-sharing.md — KEEP (0 issues)
Interactive shared note features documented at lines 117-143 including todo toggling and calendar events.

## features/download-queue.md — KEEP (0 issues)

## features/job-system.md — EDIT (2 issues)

**INACCURATE:**
1. Line 14: Download job ID format claims "4-byte random hex (8 chars)" but ground truth says "Random 16-char hex"

**MISSING:**
1. Missing user jobs from `mah.start_job` as a third job source. Doc only mentions download queue jobs and async plugin action jobs.

## features/activity-log.md — EDIT (1 issue)

**INACCURATE:**
1. Line 22: Details field type described as `string` but ground truth says `types.JSON`. Should be described as JSON/object.

## features/thumbnail-generation.md — KEEP (0 issues)

## features/custom-block-types.md — EDIT (2 issues)

**INACCURATE:**
1. Lines 13-22: Built-in block types list does not include `code` blocks, which ground truth lists as a built-in type.

**MISSING:**
1. No mention of plugin block types (`plugin:<plugin-name>:<type>`) as a mechanism in the overview section.

## features/entity-picker.md — EDIT (3 issues)

**INACCURATE:**
1. Line 32: `entityType` parameter documented as only supporting `'resource'` or `'group'`, but ground truth says picker supports resources, notes, groups, tags, categories.

**MISSING:**
1. Notes, tags, categories not documented as supported entity types.
2. Batch selection mode not mentioned.

## features/plugin-system.md — KEEP (0 issues)
Purge-data endpoint present at line 119.

## features/plugin-actions.md — KEEP (0 issues)

## features/plugin-hooks.md — EDIT (1 issue)

**INACCURATE:**
1. Line 65: Claims "28 lifecycle hooks" but the table at lines 67-73 shows 5 entity types x 6 hooks = 30 hooks.

## features/plugin-lua-api.md — KEEP (0 issues)
All known gaps verified as present: full CRUD for all entity types, relationship functions, mah.kv.*, mah.log(), mah.start_job().

## api/overview.md — KEEP (0 issues)
OpenAPI validator command documented at lines 182-188.

## api/resources.md — KEEP (0 issues)

## api/notes.md — EDIT (1 issue)

**MISSING:**
1. Missing bulk operations: POST /v1/notes/addTags, removeTags, addGroups, addMeta, delete.

## api/groups.md — KEEP (0 issues)

## api/plugins.md — KEEP (0 issues)
Purge-data endpoint present at lines 98-114.

## api/other-endpoints.md — KEEP (0 issues)

## configuration/overview.md — KEEP (0 issues)

## configuration/database.md — KEEP (0 issues)

## configuration/storage.md — KEEP (0 issues)

## configuration/advanced.md — KEEP (0 issues)
All config flags verified present: thumbnail worker, video thumbnail, plugin, share server flags all documented.

## deployment/docker.md — KEEP (0 issues)

## deployment/systemd.md — KEEP (0 issues)

## deployment/reverse-proxy.md — KEEP (0 issues)

## deployment/public-sharing.md — KEEP (0 issues)

## deployment/backups.md — KEEP (0 issues)

## troubleshooting.md — KEEP (0 issues)

---

## Summary Table

| File | Priority | Issue Count | Biggest Issue |
|------|----------|-------------|---------------|
| features/versioning.md | KEEP | 0 | -- |
| features/image-similarity.md | KEEP | 0 | -- |
| features/saved-queries.md | KEEP | 0 | -- |
| features/custom-templates.md | KEEP | 0 | -- |
| features/meta-schemas.md | KEEP | 0 | -- |
| features/note-sharing.md | KEEP | 0 | -- |
| features/download-queue.md | KEEP | 0 | -- |
| features/job-system.md | EDIT | 2 | Download job ID format wrong; missing mah.start_job user jobs |
| features/activity-log.md | EDIT | 1 | Details field type "string" should be JSON |
| features/thumbnail-generation.md | KEEP | 0 | -- |
| features/custom-block-types.md | EDIT | 2 | Missing `code` block type; missing plugin block types mention |
| features/entity-picker.md | EDIT | 3 | Only documents resource/group; notes/tags/categories also supported |
| features/plugin-system.md | KEEP | 0 | -- |
| features/plugin-actions.md | KEEP | 0 | -- |
| features/plugin-hooks.md | EDIT | 1 | "28 lifecycle hooks" should be 30 |
| features/plugin-lua-api.md | KEEP | 0 | -- |
| api/overview.md | KEEP | 0 | -- |
| api/resources.md | KEEP | 0 | -- |
| api/notes.md | EDIT | 1 | Missing bulk operations |
| api/groups.md | KEEP | 0 | -- |
| api/plugins.md | KEEP | 0 | -- |
| api/other-endpoints.md | KEEP | 0 | -- |
| configuration/overview.md | KEEP | 0 | -- |
| configuration/database.md | KEEP | 0 | -- |
| configuration/storage.md | KEEP | 0 | -- |
| configuration/advanced.md | KEEP | 0 | -- |
| deployment/docker.md | KEEP | 0 | -- |
| deployment/systemd.md | KEEP | 0 | -- |
| deployment/reverse-proxy.md | KEEP | 0 | -- |
| deployment/public-sharing.md | KEEP | 0 | -- |
| deployment/backups.md | KEEP | 0 | -- |
| troubleshooting.md | KEEP | 0 | -- |
