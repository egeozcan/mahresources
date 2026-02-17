# Maintainability Triage and Low-Hanging Cleanup Design

## Context

Audit of the codebase found 40 staticcheck findings, error-shadowing bugs in template context providers, copy/paste correctness issues, incorrect content negotiation, and significant code duplication. All Go tests pass. No DB schema changes needed.

## Execution Strategy

Sequential commits by item (Approach A). Items 1-5 first (quick fixes), then 6-8 (medium effort), then 9-10 (tooling/frontend). Run `go test ./...` after each item.

## Items

### 1. Fix Silent Error Shadowing in Create Providers

Files: `category_template_context.go`, `tag_template_context.go`, `query_template_context.go`, `resource_category_template_context.go`

Check decode error before proceeding. If decode fails, return error context. If `query.ID` is zero, skip entity fetch (create page, not edit).

### 2. Fix Copy/Paste Bugs

- `resource_template_context.go:278`: fix `/resouce` to `/resource`
- `group_template_context.go:62-66`: remove duplicate dead `if err != nil` block

### 3. Content Negotiation Fix

`render_template.go:49`: change `request.Header.Get("Content-type")` to `request.Header.Get("Accept")`. Keep `.json` suffix check. Clean switch, no fallback to old header.

### 4. JSON Charset Tolerance

`api_handlers.go:41`: change `contentTypeHeader == constants.JSON` to `strings.HasPrefix(contentTypeHeader, constants.JSON)`. Mirrors pattern already used for form content types.

### 5. Mock and Test Quality

`mock_group_context.go`: replace `panic("implement me")` with zero-value returns (nil error for reads, sentinel error for writes).

`group_template_context_test.go`: rewrite to assert on returned `pongo2.Context` fields. Remove `fmt.Println` lines.

### 6. Error Handling Consolidation

Remove `fmt.Println(err)` noise before `addErrContext` calls across all providers. The `addErrContext` helper already surfaces errors to templates.

For `_, _ :=` patterns on optional supplementary data loads: keep as-is (intentional graceful degradation), add brief comment where intent isn't obvious. No new helper function needed.

### 7. De-duplication

**Handler factory** (`handler_factory.go`): extract generic `createOrUpdateHandler[Creator, Entity]` that takes decode type, create/update functions, entity name, and ID extractor. Four handlers become thin wrappers.

**Template providers**: extract generic `entityEditContextProvider` for the create/edit pattern shared by category, tag, and query providers. List providers stay separate (too entity-specific).

### 8. Staticcheck Backlog

Priority order: SA4006 (unused assignments) > U1000 (dead code) > SA1019 (deprecated APIs like `ioutil`) > S1028 (simplify error formatting). Straight deletion for unused code, no compatibility shims. Goal: zero high-signal findings.

### 9. Frontend Hotspots

**schemaForm.js**: split ~840-line monolith into `src/components/schemaForm/` directory with type-specific modules. Entry point dispatches to per-type render functions.

**blockCalendar.js**: batch multi-select operations into one `saveContent` + one `fetchEvents(true)` instead of N calls.

### 10. CI and Build Hygiene

**`.github/workflows/ci.yml`**: `go test`, `staticcheck` (fail on SA4006/U1000/SA1019/S1028), optional route/OpenAPI parity warning.

**`.dockerignore`**: exclude `node_modules/`, `.git/`, `e2e/`, `*.db`.

## Public API Changes

- Template endpoints will honor `Accept: application/json` instead of `Content-type` (documented behavior, previously broken).
- All other changes are internal-only.

## Test Scenarios

1. Malformed `id` query on create pages returns error context instead of silent fallthrough.
2. `Accept: application/json` returns JSON body for non-`.json` URL; HTML remains default.
3. JSON body with `Content-Type: application/json; charset=utf-8` parses correctly.
4. Resource breadcrumb link uses `/resource?id=...` (regression test for typo fix).
5. Group template context test asserts on returned context fields.
6. Calendar multi-select triggers exactly one refresh fetch.
7. `go test ./...` passes and staticcheck high-signal findings are zero.
