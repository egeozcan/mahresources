# Strategy 1: Extract Common Utilities

**Complexity:** Low
**Impact:** Medium
**Risk:** Low
**Effort:** ~2-3 days

## Goal

Eliminate scattered utility duplication without changing architecture. This strategy creates reusable helper functions for patterns that are repeated across multiple files.

## Problem Statement

The codebase has several utility patterns duplicated across files:

1. **Database dialect handling** (7+ occurrences):
   ```go
   likeOperator := "LIKE"
   if db.Config.Dialector.Name() == "postgres" {
       likeOperator = "ILIKE"
   }
   ```

2. **Date range filtering** (3+ occurrences in scopes)
3. **Sort column validation** (4+ occurrences with same regex)
4. **Transaction handling with panic recovery** (only 5/116 methods)
5. **Association slice building** (27+ occurrences)

## Proposed Changes

### 1. Database Dialect Helper

**New file:** `models/db_utils.go`

```go
package models

import "gorm.io/gorm"

// GetLikeOperator returns the appropriate LIKE operator for the database dialect.
// PostgreSQL uses ILIKE for case-insensitive matching.
func GetLikeOperator(db *gorm.DB) string {
    if db.Config.Dialector.Name() == "postgres" {
        return "ILIKE"
    }
    return "LIKE"
}

// sortColumnMatcher validates sort column strings to prevent SQL injection
var sortColumnMatcher = regexp.MustCompile(`^(meta->>?'[a-z_]+'|[a-z_]+)(\s(desc|asc))?$`)

// ValidateSortColumn checks if a sort column string is safe to use
func ValidateSortColumn(sortBy string) bool {
    return sortBy != "" && sortColumnMatcher.MatchString(sortBy)
}
```

### 2. Common Database Scopes

**New file:** `models/database_scopes/common_scopes.go`

```go
package database_scopes

import "gorm.io/gorm"

// ApplyDateRangeFilter adds created_at date range filtering to a query
func ApplyDateRangeFilter(column, before, after string) func(*gorm.DB) *gorm.DB {
    return func(db *gorm.DB) *gorm.DB {
        if before != "" {
            db = db.Where(column+" <= ?", before)
        }
        if after != "" {
            db = db.Where(column+" >= ?", after)
        }
        return db
    }
}

// ApplyPagination adds LIMIT and OFFSET to a query
func ApplyPagination(offset, limit int) func(*gorm.DB) *gorm.DB {
    return func(db *gorm.DB) *gorm.DB {
        return db.Offset(offset).Limit(limit)
    }
}

// ApplySort adds ORDER BY if the sort column is valid
func ApplySort(sortBy string) func(*gorm.DB) *gorm.DB {
    return func(db *gorm.DB) *gorm.DB {
        if models.ValidateSortColumn(sortBy) {
            return db.Order(sortBy)
        }
        return db
    }
}
```

### 3. Transaction Helper

**New file:** `application_context/tx_helper.go`

```go
package application_context

import "gorm.io/gorm"

// WithTx executes a function within a database transaction with proper
// panic recovery and automatic rollback on error.
func (ctx *MahresourcesContext) WithTx(fn func(*gorm.DB) error) error {
    tx := ctx.db.Begin()
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            panic(r) // re-panic after rollback
        }
    }()

    if err := fn(tx); err != nil {
        tx.Rollback()
        return err
    }

    return tx.Commit().Error
}

// WithTxResult executes a function within a transaction and returns a result
func WithTxResult[T any](ctx *MahresourcesContext, fn func(*gorm.DB) (T, error)) (T, error) {
    var result T
    tx := ctx.db.Begin()
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            panic(r)
        }
    }()

    var err error
    result, err = fn(tx)
    if err != nil {
        tx.Rollback()
        return result, err
    }

    return result, tx.Commit().Error
}
```

### 4. Association Builder Helper

**New file:** `application_context/assoc_helper.go`

```go
package application_context

import "mahresources/models"

// BuildTagSlice creates a slice of Tag models from a slice of IDs
func BuildTagSlice(ids []uint) []models.Tag {
    tags := make([]models.Tag, len(ids))
    for i, id := range ids {
        tags[i] = models.Tag{ID: id}
    }
    return tags
}

// BuildGroupSlice creates a slice of Group models from a slice of IDs
func BuildGroupSlice(ids []uint) []models.Group {
    groups := make([]models.Group, len(ids))
    for i, id := range ids {
        groups[i] = models.Group{ID: id}
    }
    return groups
}

// BuildResourceSlice creates a slice of Resource models from a slice of IDs
func BuildResourceSlice(ids []uint) []models.Resource {
    resources := make([]models.Resource, len(ids))
    for i, id := range ids {
        resources[i] = models.Resource{ID: id}
    }
    return resources
}

// Generic version (Go 1.18+)
func BuildIDSlice[T any](ids []uint, factory func(uint) T) []T {
    result := make([]T, len(ids))
    for i, id := range ids {
        result[i] = factory(id)
    }
    return result
}
```

## Files to Modify

### Scope Files (use new helpers)

| File | Changes |
|------|---------|
| `models/database_scopes/tag_scope.go` | Replace inline LIKE/date logic with helpers |
| `models/database_scopes/category_scope.go` | Replace inline LIKE logic |
| `models/database_scopes/note_scope.go` | Replace inline LIKE/date/sort logic |
| `models/database_scopes/resource_scope.go` | Replace inline LIKE/date/sort logic |
| `models/database_scopes/group_scope.go` | Replace inline LIKE/date/sort logic |
| `models/database_scopes/query_scope.go` | Replace inline LIKE logic |
| `models/database_scopes/relation_scope.go` | Replace inline LIKE logic |

### Context Files (use transaction helper)

| File | Changes |
|------|---------|
| `application_context/note_context.go` | Use `WithTxResult` for CreateOrUpdateNote |
| `application_context/group_context.go` | Use `WithTxResult` for CreateGroup, UpdateGroup |
| `application_context/resource_context.go` | Use `WithTxResult` for EditResource |
| `application_context/relation_context.go` | Use `WithTxResult` for AddRelation |

### Context Files (use association helpers)

| File | Changes |
|------|---------|
| `application_context/note_context.go` | Use BuildTagSlice, BuildGroupSlice, BuildResourceSlice |
| `application_context/group_context.go` | Use BuildTagSlice |
| `application_context/resource_context.go` | Use BuildTagSlice, BuildGroupSlice |

## Example Refactoring

### Before (note_context.go)

```go
func (ctx *MahresourcesContext) CreateOrUpdateNote(noteQuery *query_models.NoteEditor) (*models.Note, error) {
    tx := ctx.db.Begin()
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
        }
    }()

    // ... validation ...

    if err := tx.Create(&note).Error; err != nil {
        tx.Rollback()
        return nil, err
    }

    if len(noteQuery.Tags) > 0 {
        tags := make([]models.Tag, len(noteQuery.Tags))
        for i, v := range noteQuery.Tags {
            tags[i] = models.Tag{ID: v}
        }
        if err := tx.Model(&note).Association("Tags").Append(&tags); err != nil {
            tx.Rollback()
            return nil, err
        }
    }

    // ... more associations ...

    return &note, tx.Commit().Error
}
```

### After

```go
func (ctx *MahresourcesContext) CreateOrUpdateNote(noteQuery *query_models.NoteEditor) (*models.Note, error) {
    return WithTxResult(ctx, func(tx *gorm.DB) (*models.Note, error) {
        // ... validation ...

        if err := tx.Create(&note).Error; err != nil {
            return nil, err
        }

        if len(noteQuery.Tags) > 0 {
            tags := BuildTagSlice(noteQuery.Tags)
            if err := tx.Model(&note).Association("Tags").Append(&tags); err != nil {
                return nil, err
            }
        }

        // ... more associations ...

        return &note, nil
    })
}
```

## Testing

1. **Unit tests for new helpers:**
   - `models/db_utils_test.go`
   - `models/database_scopes/common_scopes_test.go`
   - `application_context/tx_helper_test.go`

2. **Existing E2E tests** should pass without modification

3. **Run full test suite:**
   ```bash
   go test ./...
   cd e2e && npm test
   ```

## Success Metrics

- [ ] All 7 LIKE operator duplications replaced
- [ ] All 4 sort validation duplications replaced
- [ ] All 3 date range filtering duplications replaced
- [ ] Transaction helper used in all complex create/update methods
- [ ] All tests passing
- [ ] ~400 lines of duplicated code removed
