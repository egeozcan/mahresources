# Doc Checker A Report: Intro, Concepts, Getting Started, User Guide

**Summary**: 10 KEEP, 8 EDIT, 0 REWRITE. 13 total issues across 18 files. Zero AI-slop found.

---

## intro.md — EDIT (4 issues)

**INACCURATE:**
1. Block types list is wrong — missing several built-in types and doesn't mention plugin-defined block types.

**MISSING:**
1. Plugin KV store (`mah.kv.*`) not mentioned in feature highlights.
2. Plugin entity CRUD (`mah.db.create_*`, `mah.db.update_*`, etc.) not mentioned — docs imply read-only access.
3. Paste upload feature not mentioned anywhere.

## concepts/overview.md — EDIT (1 issue)

**MISSING:**
1. Note bulk operations not documented in the overview of entity capabilities. Resources and Groups show bulk ops but Notes are omitted despite having addTags, removeTags, addGroups, addMeta, delete bulk endpoints.

## concepts/resources.md — EDIT (1 issue)

**INACCURATE:**
1. Image similarity hashing described as "AHash" (average hash) but ground truth confirms the implementation uses DHash (difference hash).

## concepts/notes.md — KEEP (0 issues)

## concepts/note-blocks.md — EDIT (2 issues)

**INACCURATE:**
1. Built-in block types list is wrong/incomplete — does not match the full set of built-in block types from ground truth.

**MISSING:**
1. Plugin-defined custom block types (`plugin:<plugin-name>:<type>`) not mentioned as a mechanism for extending block types.

## concepts/groups.md — KEEP (0 issues)

## concepts/tags-categories.md — KEEP (0 issues)

## concepts/relationships.md — KEEP (0 issues)

## concepts/series.md — KEEP (0 issues)

## getting-started/installation.md — KEEP (0 issues)

## getting-started/quick-start.md — KEEP (0 issues)

## getting-started/first-steps.md — KEEP (0 issues)

## user-guide/navigation.md — EDIT (1 issue)

**INACCURATE:**
1. Max search results limit stated as 200 but ground truth says the default/max is 50.

## user-guide/managing-resources.md — KEEP (0 issues)

## user-guide/managing-notes.md — EDIT (1 issue)

**INACCURATE:**
1. Block types listed when describing note block editing are wrong — does not match the full set of built-in block types from ground truth.

## user-guide/organizing-with-groups.md — KEEP (0 issues)

## user-guide/search.md — EDIT (1 issue)

**INACCURATE:**
1. Max search results limit stated as 200 but ground truth says the default/max is 50.

## user-guide/bulk-operations.md — EDIT (1 issue)

**MISSING:**
1. Notes entity missing from bulk operations documentation. Notes support addTags, removeTags, addGroups, addMeta, and delete bulk operations but only Resources and Groups are documented.

---

## Summary Table

| File | Priority | Issue Count | Biggest Issue |
|------|----------|-------------|---------------|
| intro.md | EDIT | 4 | Missing plugin KV/CRUD/paste upload; wrong block types |
| concepts/overview.md | EDIT | 1 | Missing Note bulk ops |
| concepts/resources.md | EDIT | 1 | AHash should be DHash |
| concepts/notes.md | KEEP | 0 | -- |
| concepts/note-blocks.md | EDIT | 2 | Wrong block type list; missing plugin block types |
| concepts/groups.md | KEEP | 0 | -- |
| concepts/tags-categories.md | KEEP | 0 | -- |
| concepts/relationships.md | KEEP | 0 | -- |
| concepts/series.md | KEEP | 0 | -- |
| getting-started/installation.md | KEEP | 0 | -- |
| getting-started/quick-start.md | KEEP | 0 | -- |
| getting-started/first-steps.md | KEEP | 0 | -- |
| user-guide/navigation.md | EDIT | 1 | Max search limit 200 should be 50 |
| user-guide/managing-resources.md | KEEP | 0 | -- |
| user-guide/managing-notes.md | EDIT | 1 | Wrong block types listed |
| user-guide/organizing-with-groups.md | KEEP | 0 | -- |
| user-guide/search.md | EDIT | 1 | Max search limit 200 should be 50 |
| user-guide/bulk-operations.md | EDIT | 1 | Missing Notes entity |
