# Architecture Improvement Backlog

This backlog contains strategies for improving the maintainability of the mahresources codebase. Each strategy is documented in its own subfolder with detailed implementation guidance.

## Current State

The codebase has solid foundational architecture with clear layering:
- **application_context/** - Business logic (3,496 LOC, 116+ methods)
- **server/** - HTTP handlers (5,744 LOC, 60+ handlers)
- **models/** - GORM models and database scopes

### Key Issues Identified

| Issue | Impact | Occurrences |
|-------|--------|-------------|
| Duplicated CRUD logic across entities | High | ~400+ lines |
| Nearly identical handler patterns | High | 60+ handlers |
| Inconsistent dependency injection | Medium | 40+ routes |
| Monolithic context files | Medium | 3 files (1,570+ LOC each) |
| Scattered database dialect handling | Low | 7+ duplications |
| Inconsistent transaction handling | Medium | Only 5/116 methods have proper panic recovery |

## Strategies Overview

| # | Strategy | Complexity | Impact | Risk | Effort | Status |
|---|----------|------------|--------|------|--------|--------|
| 1 | [Extract Common Utilities](./01-extract-utilities/) | Low | Medium | Low | ~2-3 days | ✅ Complete |
| 2 | [Generic CRUD Operations](./02-generic-crud/) | Medium | High | Medium | ~1 week | 🔶 Partial |
| 3 | [Handler Middleware & Factories](./03-handler-middleware/) | Medium | High | Medium | ~1 week | 🔶 Partial |
| 4 | [Split Monolithic Context Files](./04-split-context-files/) | Medium | Medium | Low | ~2-3 days | ✅ Complete |
| 5 | [Consistent DI with Interface Expansion](./05-consistent-di/) | Medium-High | Medium | Medium | ~1 week | ⬜ Not Started |
| 6 | [Repository Pattern Extraction](./06-repository-pattern/) | High | Very High | High | ~2-3 weeks | ⬜ Not Started |
| 7 | [Event-Driven Side Effects](./07-event-driven/) | High | High | High | ~2-3 weeks | ⬜ Not Started |

### Implementation Progress

**Phase 1: Quick Wins** - ✅ COMPLETE (Commit: bb29541)
- Strategy 1: Extracted utilities to `db_utils.go` and `associations.go`
- Strategy 4: Split `resource_context.go` → 4 files, `group_context.go` → 2 files

**Phase 2: Core Improvements** - 🔶 IN PROGRESS (Commit: 9438ff9)
- Strategy 2: Generic CRUD implemented for Tag, Category, Query entities
- Strategy 3: Handler factory and middleware added; Tag, Category, Query use generic handlers
- Remaining: Group, Note, Resource entities still use entity-specific code (complex relationships)

## Recommended Implementation Order

### Phase 1: Quick Wins ✅ COMPLETE
**Strategies 1 + 4** - Extract utilities and split large files
- ✅ `db_utils.go`: GetLikeOperator, SortColumnMatcher, ValidateSortColumn, ApplyDateRange
- ✅ `associations.go`: Generic BuildAssociationSlice and BuildAssociationSlicePtr
- ✅ `resource_context.go` split into: `resource_crud_context.go`, `resource_upload_context.go`, `resource_media_context.go`, `resource_bulk_context.go`
- ✅ `group_context.go` split into: `group_crud_context.go`, `group_bulk_context.go`

### Phase 2: Core Improvements 🔶 IN PROGRESS
**Strategies 2 + 3** - Implement generic CRUD and handler factories
- ✅ `generic_crud.go`: CRUDReader and CRUDWriter with scope adapters
- ✅ `crud_factories.go`: Entity factories on MahresourcesContext
- ✅ `handler_factory.go`: CRUDHandlerFactory for standard HTTP handlers
- ✅ `middleware.go`: Request parsing and response handling utilities
- ✅ Tag, Category, Query entities use generic CRUD and handler factory
- ⬜ Group, Note, Resource: Still use entity-specific code (complex relationships justify custom code)

### Phase 3: Consistency ⬜ NOT STARTED
**Strategy 5** - Fix DI inconsistencies
- Improves testability
- ~1 week effort
- Can be done independently

### Phase 4: Major Refactor (Optional) ⬜ NOT STARTED
**Strategies 6 or 7** - Repository pattern or event-driven architecture
- Only if business needs justify the effort
- High risk, requires comprehensive testing
- ~2-3 weeks effort each

## Dependencies Between Strategies

```
Strategy 1 (Extract Utilities)
    ↓
Strategy 4 (Split Context Files)
    ↓
Strategy 2 (Generic CRUD) ←→ Strategy 3 (Handler Middleware)
    ↓
Strategy 5 (Consistent DI)
    ↓
Strategy 6 (Repository Pattern) OR Strategy 7 (Event-Driven)
```

## How to Use This Backlog

1. **Review each strategy** in its subfolder to understand scope and impact
2. **Pick strategies** based on current priorities and available time
3. **Create branches** for each strategy: `refactor/strategy-01-extract-utilities`
4. **Test thoroughly** using existing E2E tests before merging
5. **Update this document** as strategies are completed
