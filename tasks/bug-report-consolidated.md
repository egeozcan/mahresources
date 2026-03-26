# Consolidated Bug Report

**Date**: 2026-03-26
**Total bugs**: 9 (4 major, 3 minor, 2 cosmetic)

---

## Group A: Bulk Operation Input Validation (5 bugs - major)

All bulk endpoints (addTags, removeTags, addMeta, delete) across all entity types silently return `200 {"ok":true}` when required parameters are missing or reference nonexistent entities. This is a systemic pattern needing a unified fix.

| Bug | Endpoint Pattern | Missing Param | Severity |
|-----|-----------------|---------------|----------|
| BUG-2-02 | */addMeta | No IDs | major |
| BUG-2-03 | */addMeta | Nonexistent IDs | major |
| BUG-2-04 | */addTags, removeTags, addMeta, delete | No IDs | major |
| BUG-2-05 | */addTags, removeTags | No TagID | major |
| BUG-2-06 | */addTags | Nonexistent TagID | minor |

**Root cause**: Bulk handlers execute on empty sets without validating required inputs.

---

## Group B: Error Message Quality (3 bugs - minor/cosmetic)

Raw internal errors are exposed to users instead of friendly messages.

| Bug | Scenario | Current Error | Severity |
|-----|---------|---------------|----------|
| BUG-1-02 | Non-numeric entity ID (/note?id=abc) | `schema: error converting value for "id"` | cosmetic |
| BUG-1-03 | Duplicate tag name via API | `UNIQUE constraint failed: tags.name` | minor |
| BUG-2-01 | Template .json route errors | Leaks adminMenu, config in JSON | minor |

---

## Group C: URL Parameter Case Sensitivity (1 bug - minor)

| Bug | Scenario | Severity |
|-----|---------|----------|
| BUG-1-01 | Filter inputs don't populate from lowercase URL params | minor |

**Root cause**: Templates check `queryValues.Name.0` (uppercase) but lowercase URL params don't match.
