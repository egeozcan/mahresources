# Meta Subpath Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable dotted subpath navigation in MRQL meta fields (e.g., `meta.a.b.c = 1`) across all meta usage sites.

**Architecture:** The parser already splits `meta.a.b.c` into multiple parts. We bump `maxFieldParts` from 5 to 8, add two shared helpers (`metaJsonExpr` and `metaJsonTextExpr`) that build dialect-specific JSON extraction SQL from subpath segments, update the validator to allow multi-part meta leaves in traversal chains, and replace all 10 inline meta-key-to-SQL call sites with calls to the shared helpers.

**Tech Stack:** Go, GORM, SQLite (json_extract), PostgreSQL (chained `->` / `->>` operators)

---

### Task 1: Bump maxFieldParts and add parser tests

**Files:**
- Modify: `mrql/parser.go:293`
- Modify: `mrql/parser_test.go`

- [ ] **Step 1: Write failing parser tests for deeper chains**

Add to `mrql/parser_test.go`:

```go
func TestParser_MetaSubpathParsing(t *testing.T) {
	t.Run("meta.a.b parses to 3 parts", func(t *testing.T) {
		q, err := Parse(`meta.a.b = 1`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		comp := q.Where.(*ComparisonExpr)
		if len(comp.Field.Parts) != 3 {
			t.Errorf("expected 3 parts, got %d", len(comp.Field.Parts))
		}
		if comp.Field.Name() != "meta.a.b" {
			t.Errorf("expected 'meta.a.b', got %q", comp.Field.Name())
		}
	})

	t.Run("meta.a.b.c.d parses to 5 parts", func(t *testing.T) {
		q, err := Parse(`meta.a.b.c.d = "x"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		comp := q.Where.(*ComparisonExpr)
		if len(comp.Field.Parts) != 5 {
			t.Errorf("expected 5 parts, got %d", len(comp.Field.Parts))
		}
	})

	t.Run("8-part chain parses successfully", func(t *testing.T) {
		// parent.parent.parent.parent.meta.a.b.c = 8 parts
		q, err := Parse(`parent.parent.parent.parent.meta.a.b.c = 1`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		comp := q.Where.(*ComparisonExpr)
		if len(comp.Field.Parts) != 8 {
			t.Errorf("expected 8 parts, got %d", len(comp.Field.Parts))
		}
	})

	t.Run("9-part chain rejected", func(t *testing.T) {
		_, err := Parse(`parent.parent.parent.parent.parent.meta.a.b.c = 1`)
		if err == nil {
			t.Fatal("expected error for 9-part chain, got nil")
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestParser_MetaSubpathParsing -v`
Expected: The 8-part test fails with "traversal chain too deep (max 5 parts)". The 3-part and 5-part tests already pass (within old limit).

- [ ] **Step 3: Bump maxFieldParts from 5 to 8**

In `mrql/parser.go`, change line 293:

```go
const maxFieldParts = 8
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestParser_MetaSubpathParsing -v`
Expected: All 4 tests pass.

- [ ] **Step 5: Commit**

```bash
git add mrql/parser.go mrql/parser_test.go
git commit -m "feat(mrql): bump maxFieldParts to 8 for meta subpath support"
```

---

### Task 2: Add shared meta JSON extraction helpers

**Files:**
- Modify: `mrql/translator.go` (add helpers near existing `isValidMetaKey` at line ~793)

- [ ] **Step 1: Write tests for the new helpers**

Add to `mrql/translator_test.go`:

```go
func TestMetaJsonExpr(t *testing.T) {
	tests := []struct {
		name       string
		segments   []string
		tableName  string
		isPostgres bool
		wantExpr   string
		wantText   string
	}{
		{
			name:       "single key sqlite",
			segments:   []string{"rating"},
			tableName:  "resources",
			isPostgres: false,
			wantExpr:   "json_extract(resources.meta, '$.rating')",
			wantText:   "json_extract(resources.meta, '$.rating')",
		},
		{
			name:       "single key postgres",
			segments:   []string{"rating"},
			tableName:  "resources",
			isPostgres: true,
			wantExpr:   "resources.meta->>'rating'",
			wantText:   "resources.meta->>'rating'",
		},
		{
			name:       "two-level subpath sqlite",
			segments:   []string{"a", "b"},
			tableName:  "resources",
			isPostgres: false,
			wantExpr:   "json_extract(resources.meta, '$.a.b')",
			wantText:   "json_extract(resources.meta, '$.a.b')",
		},
		{
			name:       "two-level subpath postgres",
			segments:   []string{"a", "b"},
			tableName:  "resources",
			isPostgres: true,
			wantExpr:   "resources.meta->'a'->>'b'",
			wantText:   "resources.meta->'a'->>'b'",
		},
		{
			name:       "three-level subpath postgres",
			segments:   []string{"a", "b", "c"},
			tableName:  "notes",
			isPostgres: true,
			wantExpr:   "notes.meta->'a'->'b'->>'c'",
			wantText:   "notes.meta->'a'->'b'->>'c'",
		},
		{
			name:       "three-level subpath sqlite",
			segments:   []string{"a", "b", "c"},
			tableName:  "groups",
			isPostgres: false,
			wantExpr:   "json_extract(groups.meta, '$.a.b.c')",
			wantText:   "json_extract(groups.meta, '$.a.b.c')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := &translateContext{tableName: tt.tableName}
			if tt.isPostgres {
				tc.dialect = "postgres"
			}
			gotExpr := tc.metaJsonExpr(tt.segments)
			if gotExpr != tt.wantExpr {
				t.Errorf("metaJsonExpr: got %q, want %q", gotExpr, tt.wantExpr)
			}
			gotText := tc.metaJsonTextExpr(tt.segments)
			if gotText != tt.wantText {
				t.Errorf("metaJsonTextExpr: got %q, want %q", gotText, tt.wantText)
			}
		})
	}
}

func TestValidateMetaSegments(t *testing.T) {
	tests := []struct {
		name     string
		segments []string
		wantErr  bool
	}{
		{"single valid", []string{"rating"}, false},
		{"multi valid", []string{"a", "b", "c"}, false},
		{"underscores", []string{"config_v2", "host"}, false},
		{"hyphen invalid", []string{"a-b", "c"}, true},
		{"space invalid", []string{"a b"}, true},
		{"empty segment invalid", []string{"a", ""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMetaSegments(tt.segments)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMetaSegments(%v) err=%v, wantErr=%v", tt.segments, err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./mrql/... -run "TestMetaJsonExpr|TestValidateMetaSegments" -v`
Expected: Compilation error — functions don't exist yet.

- [ ] **Step 3: Implement the helpers**

Add to `mrql/translator.go` after the existing `isValidMetaKey` function (around line 800):

```go
// validateMetaSegments checks that each segment in a meta subpath is safe for
// interpolation into JSON extraction paths.
func validateMetaSegments(segments []string) error {
	for _, seg := range segments {
		if !isValidMetaKey(seg) {
			return fmt.Errorf("invalid meta key segment %q: must contain only alphanumeric characters and underscores", seg)
		}
	}
	return nil
}

// metaSubpathSegments extracts the subpath segments from a meta field name.
// "meta.a.b.c" → ["a", "b", "c"]
func metaSubpathSegments(fieldName string) []string {
	return strings.Split(strings.TrimPrefix(fieldName, "meta."), ".")
}

// metaJsonExpr builds the JSON extraction expression for a meta subpath.
// On SQLite: json_extract(table.meta, '$.a.b.c')
// On Postgres: table.meta->'a'->'b'->>'c'  (text extraction on final key)
func (tc *translateContext) metaJsonExpr(segments []string) string {
	return tc.metaJsonExprOn(tc.tableName, segments)
}

// metaJsonExprOn builds the JSON extraction expression using a specific table alias.
func (tc *translateContext) metaJsonExprOn(alias string, segments []string) string {
	if tc.isPostgres() {
		return pgJsonTextPath(alias, segments)
	}
	return sqliteJsonPath(alias, segments)
}

// metaJsonTextExpr returns a text-typed JSON extraction expression.
// On SQLite this is the same as metaJsonExpr (json_extract returns native types).
// On Postgres this is the same as metaJsonExpr (final ->> returns text).
func (tc *translateContext) metaJsonTextExpr(segments []string) string {
	return tc.metaJsonExpr(segments)
}

// metaJsonTextExprOn returns a text-typed JSON extraction expression for a specific alias.
func (tc *translateContext) metaJsonTextExprOn(alias string, segments []string) string {
	return tc.metaJsonExprOn(alias, segments)
}

// metaNumericExpr builds a safe numeric cast expression for a meta subpath.
// On Postgres: CASE WHEN expr ~ numericPattern THEN expr::numeric ELSE NULL END
// On SQLite: json_extract (returns native types, caller adds json_type filter)
func (tc *translateContext) metaNumericExpr(segments []string) string {
	return tc.metaNumericExprOn(tc.tableName, segments)
}

// metaNumericExprOn builds a safe numeric cast expression using a specific table alias.
func (tc *translateContext) metaNumericExprOn(alias string, segments []string) string {
	if tc.isPostgres() {
		textExpr := pgJsonTextPath(alias, segments)
		return fmt.Sprintf(
			"CASE WHEN %s ~ '^-{0,1}[0-9]+(\\.[0-9]+){0,1}$' THEN (%s)::numeric ELSE NULL END",
			textExpr, textExpr,
		)
	}
	return sqliteJsonPath(alias, segments)
}

// metaTypeFilter returns a SQL WHERE clause that filters to rows where the
// meta subpath holds a numeric JSON type. Only needed for SQLite.
// Returns empty string on Postgres (numeric cast handles non-numeric via CASE).
func (tc *translateContext) metaTypeFilterOn(alias string, segments []string) string {
	if tc.isPostgres() {
		return ""
	}
	path := "$." + strings.Join(segments, ".")
	return fmt.Sprintf("json_type(%s.meta, '%s') IN ('integer', 'real')", alias, path)
}

// pgJsonTextPath builds Postgres chained arrow JSON path: table.meta->'a'->'b'->>'c'
func pgJsonTextPath(alias string, segments []string) string {
	if len(segments) == 1 {
		return fmt.Sprintf("%s.meta->>'%s'", alias, segments[0])
	}
	var b strings.Builder
	b.WriteString(alias)
	b.WriteString(".meta")
	for i, seg := range segments {
		if i == len(segments)-1 {
			b.WriteString("->>'" + seg + "'")
		} else {
			b.WriteString("->'" + seg + "'")
		}
	}
	return b.String()
}

// sqliteJsonPath builds SQLite json_extract path: json_extract(table.meta, '$.a.b.c')
func sqliteJsonPath(alias string, segments []string) string {
	path := "$." + strings.Join(segments, ".")
	return fmt.Sprintf("json_extract(%s.meta, '%s')", alias, path)
}
```

Note: You need to check the `translateContext` struct for its dialect field. Look at the `isPostgres()` method to see how dialect is stored. The test setup must set `tc.dialect = "postgres"` for Postgres tests. If the field name differs, adjust accordingly.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/... -run "TestMetaJsonExpr|TestValidateMetaSegments" -v`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add mrql/translator.go mrql/translator_test.go
git commit -m "feat(mrql): add shared meta JSON extraction helpers for subpath support"
```

---

### Task 3: Update validator for multi-part meta leaves in traversal chains

**Files:**
- Modify: `mrql/validator.go:554`
- Modify: `mrql/validator_test.go`

- [ ] **Step 1: Write failing validator tests**

Add to `mrql/validator_test.go`:

```go
func TestValidate_MetaSubpathFields(t *testing.T) {
	valid := []struct {
		name  string
		input string
	}{
		{"meta.a.b on resource", `type = "resource" AND meta.a.b = 1`},
		{"meta.a.b.c on note", `type = "note" AND meta.a.b.c = "x"`},
		{"meta.a.b on group", `type = "group" AND meta.a.b = 1`},
		{"owner.meta.a.b on resource", `type = "resource" AND owner.meta.a.b = 1`},
		{"parent.meta.a.b on group", `type = "group" AND parent.meta.a.b = 1`},
		{"parent.parent.meta.a.b.c on group", `type = "group" AND parent.parent.meta.a.b.c = "x"`},
		{"owner.parent.meta.a.b on resource", `type = "resource" AND owner.parent.meta.a.b = 1`},
		{"order by meta.a.b", `type = "resource" ORDER BY meta.a.b`},
		{"group by meta.a.b", `type = "resource" GROUP BY meta.a.b COUNT()`},
		{"group by owner.meta.a.b", `type = "resource" GROUP BY owner.meta.a.b COUNT()`},
	}
	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if err := Validate(q); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify some fail**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestValidate_MetaSubpathFields -v`
Expected: Direct meta tests (`meta.a.b`) pass (validator already allows `prefix == "meta"`). Traversal tests (`owner.meta.a.b`) fail because `validateTraversalChain` only allows exactly one part after `meta`.

- [ ] **Step 3: Fix the validator**

In `mrql/validator.go`, change the meta leaf detection in `validateTraversalChain` (around line 554). Replace:

```go
		// meta.key leaf: the rest of the chain is a meta field reference, not traversal
		if part == "meta" && i == len(f.Parts)-2 {
			// owner.meta.abc → valid (meta.abc is the leaf on the target group)
			return nil
		}
```

With:

```go
		// meta subpath leaf: once we see "meta", everything after it is the JSON
		// subpath — stop validating intermediates. Handles owner.meta.a.b.c etc.
		if part == "meta" && i < len(f.Parts)-1 {
			return nil
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestValidate_MetaSubpathFields -v`
Expected: All tests pass.

- [ ] **Step 5: Run full validator test suite to check for regressions**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestValidate -v`
Expected: All existing validator tests still pass.

- [ ] **Step 6: Commit**

```bash
git add mrql/validator.go mrql/validator_test.go
git commit -m "feat(mrql): allow multi-part meta subpaths in traversal chain validation"
```

---

### Task 4: Update translateMetaComparison to use helpers

**Files:**
- Modify: `mrql/translator.go` (function `translateMetaComparison` at line ~813)
- Modify: `mrql/translator_comprehensive_test.go`

- [ ] **Step 1: Write failing integration tests for meta subpath comparisons**

Add to `mrql/translator_comprehensive_test.go`. First, update `setupTestDB` in `mrql/translator_test.go` to add a resource with nested meta. Find the resources seed data (around line 128) and add nested meta to the existing `sunset.jpg` entry. Change:

```go
{ID: 1, Name: "sunset.jpg", OriginalName: "sunset.jpg", ContentType: "image/jpeg", FileSize: 1024000, Width: 1920, Height: 1080, CreatedAt: now, UpdatedAt: now, Meta: `{"rating":5}`},
```

To:

```go
{ID: 1, Name: "sunset.jpg", OriginalName: "sunset.jpg", ContentType: "image/jpeg", FileSize: 1024000, Width: 1920, Height: 1080, CreatedAt: now, UpdatedAt: now, Meta: `{"rating":5,"location":{"country":"spain","city":"barcelona","coords":{"lat":41.3851}}}`},
```

Also update the Vacation group meta (around line 116). Change:

```go
{ID: 1, Name: "Vacation", Meta: `{"region":"europe","priority":3}`},
```

To:

```go
{ID: 1, Name: "Vacation", Meta: `{"region":"europe","priority":3,"settings":{"visibility":"public","nested":{"deep":"value"}}}`},
```

Also update the "Meeting notes" note meta (around line 140). Change:

```go
{ID: 1, Name: "Meeting notes", CreatedAt: now, UpdatedAt: now, Meta: `{"priority":"high"}`},
```

To:

```go
{ID: 1, Name: "Meeting notes", CreatedAt: now, UpdatedAt: now, Meta: `{"priority":"high","details":{"room":"A1","floor":2}}`},
```

Then add the test to `mrql/translator_comprehensive_test.go`:

```go
func TestComprehensive_MetaSubpathComparison(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		// Resource: meta.location.country = "spain"
		{"resource meta.location.country eq", `type = "resource" AND meta.location.country = "spain"`, EntityResource, 1, []string{"sunset.jpg"}},
		// Resource: meta.location.city = "barcelona"
		{"resource meta.location.city eq", `type = "resource" AND meta.location.city = "barcelona"`, EntityResource, 1, []string{"sunset.jpg"}},
		// Resource: meta.location.coords.lat > 40 (3-level numeric)
		{"resource meta.location.coords.lat gt", `type = "resource" AND meta.location.coords.lat > 40`, EntityResource, 1, []string{"sunset.jpg"}},
		// Resource: meta.location.coords.lat < 40 (should match nothing)
		{"resource meta.location.coords.lat lt", `type = "resource" AND meta.location.coords.lat < 40`, EntityResource, 0, nil},
		// Resource: meta.location.country != "spain"
		{"resource meta.location.country neq", `type = "resource" AND meta.location.country != "spain"`, EntityResource, 0, nil},
		// Resource: meta.location.city ~ "bar*"
		{"resource meta.location.city like", `type = "resource" AND meta.location.city ~ "bar*"`, EntityResource, 1, []string{"sunset.jpg"}},
		// Group: meta.settings.visibility = "public"
		{"group meta.settings.visibility eq", `type = "group" AND meta.settings.visibility = "public"`, EntityGroup, 1, []string{"Vacation"}},
		// Group: meta.settings.nested.deep = "value" (3-level)
		{"group meta.settings.nested.deep eq", `type = "group" AND meta.settings.nested.deep = "value"`, EntityGroup, 1, []string{"Vacation"}},
		// Note: meta.details.room = "A1"
		{"note meta.details.room eq", `type = "note" AND meta.details.room = "A1"`, EntityNote, 1, []string{"Meeting notes"}},
		// Note: meta.details.floor = 2
		{"note meta.details.floor eq", `type = "note" AND meta.details.floor = 2`, EntityNote, 1, []string{"Meeting notes"}},
		// Case-insensitive: meta.location.country = "SPAIN"
		{"resource meta subpath case insensitive", `type = "resource" AND meta.location.country = "SPAIN"`, EntityResource, 1, []string{"sunset.jpg"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(notes), namesOfNotes(notes))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfNotes(notes), tt.wantNames)
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(groups), namesOfGroups(groups))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfGroups(groups), tt.wantNames)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestComprehensive_MetaSubpathComparison -v`
Expected: Fails with `translate error: invalid meta key "location.country": must contain only alphanumeric characters and underscores`

- [ ] **Step 3: Update translateMetaComparison to use helpers**

Replace the `translateMetaComparison` function body in `mrql/translator.go`. The new version:

```go
// translateMetaComparison handles meta.key and meta.a.b.c comparisons using json_extract.
func (tc *translateContext) translateMetaComparison(db *gorm.DB, fd FieldDef, op Token, val interface{}) (*gorm.DB, error) {
	segments := metaSubpathSegments(fd.Name)

	if err := validateMetaSegments(segments); err != nil {
		return nil, &TranslateError{Message: err.Error(), Pos: 0}
	}

	isNumericVal := isNumericValue(val)
	var jsonExpr string
	if isNumericVal {
		jsonExpr = tc.metaNumericExpr(segments)
		if filter := tc.metaTypeFilterOn(tc.tableName, segments); filter != "" {
			db = db.Where(filter)
		}
	} else {
		jsonExpr = tc.metaJsonExpr(segments)
	}

	sqlOp := tc.sqlOperator(op)

	if op.Type == TokenLike || op.Type == TokenNotLike {
		textExpr := tc.metaJsonTextExpr(segments)
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		if op.Type == TokenNotLike {
			likeOp = "NOT " + likeOp
		}
		db = db.Where(textExpr+" "+likeOp+" ? ESCAPE '\\'", likePattern)
		return db, nil
	}

	if !isNumericVal && (op.Type == TokenEq || op.Type == TokenNeq) {
		db = db.Where("LOWER("+jsonExpr+") "+sqlOp+" LOWER(?)", val)
		return db, nil
	}

	db = db.Where(jsonExpr+" "+sqlOp+" ?", val)
	return db, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestComprehensive_MetaSubpathComparison -v`
Expected: All tests pass.

- [ ] **Step 5: Run existing meta tests to check for regressions**

Run: `go test --tags 'json1 fts5' ./mrql/... -run "TestComprehensive_Meta|TestTranslateMetaField" -v`
Expected: All existing meta tests still pass.

- [ ] **Step 6: Commit**

```bash
git add mrql/translator.go mrql/translator_test.go mrql/translator_comprehensive_test.go
git commit -m "feat(mrql): update translateMetaComparison to support subpaths"
```

---

### Task 5: Update translateChainedMetaComparison and its dispatcher

**Files:**
- Modify: `mrql/translator.go` (functions `translateChainedComparison` at line ~306 and `translateChainedMetaComparison` at line ~344)
- Modify: `mrql/translator_comprehensive_test.go`

- [ ] **Step 1: Write failing tests for traversal + subpath**

Add to `mrql/translator_comprehensive_test.go`:

```go
func TestComprehensive_TraversalMetaSubpath(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		// owner.meta.settings.visibility = "public" — sunset.jpg owned by Vacation
		{"owner.meta subpath string", `type = "resource" AND owner.meta.settings.visibility = "public"`, EntityResource, 1, []string{"sunset.jpg"}},
		// owner.meta.settings.nested.deep = "value"
		{"owner.meta deep subpath", `type = "resource" AND owner.meta.settings.nested.deep = "value"`, EntityResource, 1, []string{"sunset.jpg"}},
		// parent.meta.settings.visibility on groups — Work's parent is Vacation
		{"parent.meta subpath", `type = "group" AND parent.meta.settings.visibility = "public"`, EntityGroup, 2, []string{"Work", "Photos"}},
		// owner.parent.meta.settings.visibility — report.pdf owned by Work, Work's parent is Vacation
		{"owner.parent.meta subpath", `type = "resource" AND owner.parent.meta.settings.visibility = "public"`, EntityResource, 1, []string{"report.pdf"}},
		// case-insensitive
		{"owner.meta subpath case insensitive", `type = "resource" AND owner.meta.settings.visibility = "PUBLIC"`, EntityResource, 1, []string{"sunset.jpg"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(groups), namesOfGroups(groups))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfGroups(groups), tt.wantNames)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestComprehensive_TraversalMetaSubpath -v`
Expected: Fails — the dispatcher at line 313 checks `parts[len(parts)-2].Value == "meta"` which only matches single-key meta. With `owner.meta.a.b` (4 parts), `parts[2].Value` is `"a"`, not `"meta"`.

- [ ] **Step 3: Update the dispatcher to find "meta" anywhere in the chain**

In `translateChainedComparison` (around line 313), replace:

```go
	// Handle meta.key leaf: owner.meta.region, owner.parent.meta.key
	// Detected when second-to-last part is "meta".
	if len(parts) >= 3 && parts[len(parts)-2].Value == "meta" {
		return tc.translateChainedMetaComparison(db, expr)
	}
```

With:

```go
	// Handle meta subpath leaf: owner.meta.region, owner.meta.a.b.c
	// Detected when any part (after root) is "meta" — everything after it is the subpath.
	for i := 1; i < len(parts); i++ {
		if parts[i].Value == "meta" {
			return tc.translateChainedMetaComparison(db, expr)
		}
	}
```

- [ ] **Step 4: Update translateChainedMetaComparison to handle subpaths**

Replace the function body:

```go
func (tc *translateContext) translateChainedMetaComparison(db *gorm.DB, expr *ComparisonExpr) (*gorm.DB, error) {
	parts := expr.Field.Parts

	// Find the "meta" part in the chain
	metaIdx := -1
	for i := 1; i < len(parts); i++ {
		if parts[i].Value == "meta" {
			metaIdx = i
			break
		}
	}

	// Extract subpath segments (everything after "meta")
	segments := make([]string, 0, len(parts)-metaIdx-1)
	for i := metaIdx + 1; i < len(parts); i++ {
		segments = append(segments, parts[i].Value)
	}

	if err := validateMetaSegments(segments); err != nil {
		return nil, &TranslateError{Message: err.Error(), Pos: parts[metaIdx+1].Pos}
	}

	// Build FK chain for everything before "meta" (e.g., [owner] or [owner, parent]).
	// Append a dummy leaf so buildTraversalChain processes all traversal steps.
	chainParts := make([]Token, 0, metaIdx+1)
	chainParts = append(chainParts, parts[:metaIdx]...)
	chainParts = append(chainParts, Token{Value: "_meta_leaf"})
	steps := tc.buildTraversalChain(chainParts)

	// Resolve the comparison value
	metaFd := FieldDef{Name: "meta." + strings.Join(segments, "."), Type: FieldMeta, Column: "meta." + strings.Join(segments, ".")}
	val, err := tc.resolveValue(expr.Value, metaFd)
	if err != nil {
		return nil, err
	}

	innerAlias := steps[len(steps)-1].alias
	isNumericVal := isNumericValue(val)
	var jsonExpr string
	var numericFilter string
	if isNumericVal {
		jsonExpr = tc.metaNumericExprOn(innerAlias, segments)
		numericFilter = tc.metaTypeFilterOn(innerAlias, segments)
	} else {
		jsonExpr = tc.metaJsonExprOn(innerAlias, segments)
	}

	textExpr := tc.metaJsonTextExprOn(innerAlias, segments)
	innerWhere, innerVal := tc.buildMetaClauseV2(jsonExpr, textExpr, expr.Operator, val, isNumericVal)
	if numericFilter != "" {
		innerWhere = numericFilter + " AND " + innerWhere
	}

	isNegated := expr.Operator.Type == TokenNeq || expr.Operator.Type == TokenNotLike
	isChildrenRoot := tc.isChildrenStep(steps[0])

	if isNegated && isChildrenRoot {
		positiveOp := tc.flipOperator(expr.Operator)
		posWhere, posVal := tc.buildMetaClauseV2(jsonExpr, textExpr, positiveOp, val, isNumericVal)
		if numericFilter != "" {
			posWhere = numericFilter + " AND " + posWhere
		}
		sql, vals := tc.wrapChainSubqueries(steps, posWhere, []interface{}{posVal})
		sql = strings.Replace(sql, steps[0].fkExpr+" IN ", steps[0].fkExpr+" NOT IN ", 1)
		sql = "(" + sql + " OR " + tc.negatedNullClause(steps[0]) + ")"
		db = db.Where(sql, vals...)
		return db, nil
	}

	sql, vals := tc.wrapChainSubqueries(steps, innerWhere, []interface{}{innerVal})
	if isNegated {
		sql = "(" + sql + " OR " + tc.negatedNullClause(steps[0]) + ")"
	}
	db = db.Where(sql, vals...)
	return db, nil
}
```

Also add a new `buildMetaClauseV2` that takes explicit text expression (replacing the old `buildMetaClause` which computed it from alias+key). Add this near the existing `buildMetaClause`:

```go
// buildMetaClauseV2 builds a WHERE clause for a meta JSON comparison.
// Unlike buildMetaClause, it receives pre-built JSON and text expressions
// so it works with both single keys and subpaths.
func (tc *translateContext) buildMetaClauseV2(jsonExpr string, textExpr string, op Token, val interface{}, isNumericVal bool) (string, interface{}) {
	if op.Type == TokenLike || op.Type == TokenNotLike {
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		if op.Type == TokenNotLike {
			likeOp = "NOT " + likeOp
		}
		return textExpr + " " + likeOp + " ? ESCAPE '\\'", likePattern
	}

	sqlOp := tc.sqlOperator(op)

	if !isNumericVal && (op.Type == TokenEq || op.Type == TokenNeq) {
		return "LOWER(" + jsonExpr + ") " + sqlOp + " LOWER(?)", val
	}

	return jsonExpr + " " + sqlOp + " ?", val
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestComprehensive_TraversalMetaSubpath -v`
Expected: All tests pass.

- [ ] **Step 6: Run existing traversal meta tests for regressions**

Run: `go test --tags 'json1 fts5' ./mrql/... -run "TestBugfix_TraversalMeta|TestComprehensive_TraversalMeta" -v`
Expected: All pass.

- [ ] **Step 7: Commit**

```bash
git add mrql/translator.go mrql/translator_comprehensive_test.go
git commit -m "feat(mrql): update chained meta comparison for subpath support"
```

---

### Task 6: Update translateInExpr for meta subpaths

**Files:**
- Modify: `mrql/translator.go` (function `translateInExpr` at line ~910)
- Modify: `mrql/translator_comprehensive_test.go`

- [ ] **Step 1: Write failing tests**

Add to `mrql/translator_comprehensive_test.go`:

```go
func TestComprehensive_MetaSubpathIn(t *testing.T) {
	db := setupTestDB(t)

	// meta.location.country in ("spain", "france") — sunset.jpg has spain
	result := parseAndTranslate(t, `type = "resource" AND meta.location.country in ("spain", "france")`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d (names: %v)", len(resources), namesOfResources(resources))
	}
	assertNames(t, namesOfResources(resources), []string{"sunset.jpg"})

	// NOT IN
	result2 := parseAndTranslate(t, `type = "resource" AND meta.location.country not in ("spain")`, EntityResource, db)
	var resources2 []testResource
	if err := result2.Find(&resources2).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	// Other resources have no location.country, so LOWER(json_extract) returns NULL which is excluded from NOT IN
	if len(resources2) != 0 {
		t.Fatalf("expected 0 resources, got %d (names: %v)", len(resources2), namesOfResources(resources2))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestComprehensive_MetaSubpathIn -v`
Expected: Fails with "invalid meta key" error.

- [ ] **Step 3: Update translateInExpr**

In `mrql/translator.go`, in the `translateInExpr` function's meta handling block (around line 910), replace:

```go
	// Handle meta fields — need json_extract, not qualifiedColumn
	if fd.Type == FieldMeta {
		key := strings.TrimPrefix(fd.Name, "meta.")
		if !isValidMetaKey(key) {
			return nil, &TranslateError{Message: fmt.Sprintf("invalid meta key %q", key)}
		}
		var jsonExpr string
		if tc.isPostgres() {
			jsonExpr = fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
		} else {
			jsonExpr = fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
		}
```

With:

```go
	// Handle meta fields — need json_extract, not qualifiedColumn
	if fd.Type == FieldMeta {
		segments := metaSubpathSegments(fd.Name)
		if err := validateMetaSegments(segments); err != nil {
			return nil, &TranslateError{Message: err.Error()}
		}
		jsonExpr := tc.metaJsonExpr(segments)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/... -run "TestComprehensive_MetaSubpathIn|TestComprehensive_MetaInExpr|TestComprehensive_MetaInCase" -v`
Expected: All pass (new and existing).

- [ ] **Step 5: Commit**

```bash
git add mrql/translator.go mrql/translator_comprehensive_test.go
git commit -m "feat(mrql): update translateInExpr for meta subpaths"
```

---

### Task 7: Update translateIsExpr for meta subpaths (IS NULL / IS EMPTY)

**Files:**
- Modify: `mrql/translator.go` (function `translateIsExpr` at line ~1068)
- Modify: `mrql/translator_comprehensive_test.go`

- [ ] **Step 1: Write failing tests**

Add to `mrql/translator_comprehensive_test.go`:

```go
func TestComprehensive_MetaSubpathIsNull(t *testing.T) {
	db := setupTestDB(t)

	// meta.location.country IS NULL — resources without nested location.country
	result := parseAndTranslate(t, `type = "resource" AND meta.location.country is null`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	// sunset.jpg has location.country, the other 3 don't
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources with null meta.location.country, got %d (names: %v)", len(resources), namesOfResources(resources))
	}

	// meta.location.country IS NOT NULL
	result2 := parseAndTranslate(t, `type = "resource" AND meta.location.country is not null`, EntityResource, db)
	var resources2 []testResource
	if err := result2.Find(&resources2).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources2) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources2))
	}
	assertNames(t, namesOfResources(resources2), []string{"sunset.jpg"})
}

func TestComprehensive_MetaSubpathIsEmpty(t *testing.T) {
	db := setupTestDB(t)

	// meta.location.country IS EMPTY — resources without location.country (null or "")
	result := parseAndTranslate(t, `type = "resource" AND meta.location.country is empty`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources with empty meta.location.country, got %d (names: %v)", len(resources), namesOfResources(resources))
	}

	// meta.location.country IS NOT EMPTY
	result2 := parseAndTranslate(t, `type = "resource" AND meta.location.country is not empty`, EntityResource, db)
	var resources2 []testResource
	if err := result2.Find(&resources2).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources2) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources2))
	}
	assertNames(t, namesOfResources(resources2), []string{"sunset.jpg"})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./mrql/... -run "TestComprehensive_MetaSubpathIsNull|TestComprehensive_MetaSubpathIsEmpty" -v`
Expected: Fails because `translateIsExpr` uses `strings.TrimPrefix` and builds SQL inline with the full dotted key.

- [ ] **Step 3: Update translateIsExpr for IS NULL and IS EMPTY**

In `mrql/translator.go`, in the `translateIsExpr` function, replace both meta column extraction blocks. For IS NULL (around line 1070):

```go
		var column string
		if fd.Type == FieldMeta {
			key := strings.TrimPrefix(fd.Name, "meta.")
			if tc.isPostgres() {
				column = fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
			} else {
				column = fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
			}
```

Replace with:

```go
		var column string
		if fd.Type == FieldMeta {
			segments := metaSubpathSegments(fd.Name)
			column = tc.metaJsonExpr(segments)
```

For IS EMPTY (around line 1096), apply the same replacement:

```go
		var column string
		if fd.Type == FieldMeta {
			key := strings.TrimPrefix(fd.Name, "meta.")
			if tc.isPostgres() {
				column = fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
			} else {
				column = fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
			}
```

Replace with:

```go
		var column string
		if fd.Type == FieldMeta {
			segments := metaSubpathSegments(fd.Name)
			column = tc.metaJsonExpr(segments)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/... -run "TestComprehensive_MetaSubpathIsNull|TestComprehensive_MetaSubpathIsEmpty|TestComprehensive_MetaIsNull|TestComprehensive_MetaIsNotNull|TestComprehensive_MetaIsEmpty|TestComprehensive_MetaIsNotEmpty" -v`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add mrql/translator.go mrql/translator_comprehensive_test.go
git commit -m "feat(mrql): update translateIsExpr for meta subpaths"
```

---

### Task 8: Update resolveOrderByColumn, groupByFieldExprs, groupByTraversalJoins, and resolveAggregateColumn

**Files:**
- Modify: `mrql/translator.go` (four functions)
- Modify: `mrql/translator_comprehensive_test.go`

- [ ] **Step 1: Write failing tests**

Add to `mrql/translator_comprehensive_test.go`:

```go
func TestComprehensive_MetaSubpathOrderBy(t *testing.T) {
	db := setupTestDB(t)

	// ORDER BY meta.location.country — should not error
	result := parseAndTranslate(t, `type = "resource" ORDER BY meta.location.country`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) == 0 {
		t.Fatal("expected resources, got none")
	}
}

func TestComprehensive_MetaSubpathGroupBy(t *testing.T) {
	db := setupTestDB(t)

	// GROUP BY meta.settings.visibility on groups — Vacation has "public"
	q, err := Parse(`type = "group" GROUP BY meta.settings.visibility COUNT()`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	q.EntityType = EntityGroup
	if err := Validate(q); err != nil {
		t.Fatalf("validation error: %v", err)
	}
	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Fatal("expected rows, got none")
	}
}

func TestComprehensive_MetaSubpathGroupByTraversal(t *testing.T) {
	db := setupTestDB(t)

	// GROUP BY owner.meta.settings.visibility on resources
	q, err := Parse(`type = "resource" GROUP BY owner.meta.settings.visibility COUNT()`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validation error: %v", err)
	}
	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Fatal("expected rows, got none")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./mrql/... -run "TestComprehensive_MetaSubpathOrderBy|TestComprehensive_MetaSubpathGroupBy" -v`
Expected: ORDER BY silently falls back to `tableName.meta` (because `isValidMetaKey` returns false for dotted key). GROUP BY generates wrong SQL.

- [ ] **Step 3: Update resolveOrderByColumn**

In `mrql/translator.go`, replace the meta handling in `resolveOrderByColumn` (around line 1359):

```go
	if strings.HasPrefix(fieldName, "meta.") {
		key := strings.TrimPrefix(fieldName, "meta.")
		if !isValidMetaKey(key) {
			return tc.tableName + ".meta"
		}
		if tc.isPostgres() {
			return fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
		}
		return fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
	}
```

With:

```go
	if strings.HasPrefix(fieldName, "meta.") {
		segments := metaSubpathSegments(fieldName)
		return tc.metaJsonExpr(segments)
	}
```

- [ ] **Step 4: Update groupByFieldExprs**

In `mrql/translator.go`, replace the meta handling in `groupByFieldExprs` (around line 1604):

```go
	if strings.HasPrefix(fieldName, "meta.") {
		key := strings.TrimPrefix(fieldName, "meta.")
		if tc.isPostgres() {
			expr := fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
			return expr, expr
		}
		expr := fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
		return expr, expr
	}
```

With:

```go
	if strings.HasPrefix(fieldName, "meta.") {
		segments := metaSubpathSegments(fieldName)
		expr := tc.metaJsonExpr(segments)
		return expr, expr
	}
```

- [ ] **Step 5: Update groupByTraversalJoins**

In `mrql/translator.go`, in `groupByTraversalJoins` (around line 1720), replace the meta leaf detection:

```go
	if len(parts) >= 3 && parts[len(parts)-2].Value == "meta" {
		metaKey := leaf
		// Build chain JOINs for everything before "meta"
		lastAlias := tc.groupByBuildChainJoins(&db, parts[:len(parts)-2], fieldIdx)
		if tc.isPostgres() {
			return db, fmt.Sprintf("%s.meta->>'%s'", lastAlias, metaKey)
		}
		return db, fmt.Sprintf("json_extract(%s.meta, '$.%s')", lastAlias, metaKey)
	}
```

With:

```go
	// Handle meta subpath leaf: owner.meta.a.b, owner.parent.meta.x.y.z
	// Find "meta" in the parts and treat everything after as subpath segments.
	for i := 1; i < len(parts); i++ {
		if parts[i].Value == "meta" {
			segments := make([]string, 0, len(parts)-i-1)
			for j := i + 1; j < len(parts); j++ {
				segments = append(segments, parts[j].Value)
			}
			lastAlias := tc.groupByBuildChainJoins(&db, parts[:i], fieldIdx)
			return db, tc.metaJsonExprOn(lastAlias, segments)
		}
	}
```

- [ ] **Step 6: Update resolveAggregateColumn**

In `mrql/translator.go`, replace the meta handling in `resolveAggregateColumn` (around line 1801):

```go
	if strings.HasPrefix(fieldName, "meta.") {
		key := strings.TrimPrefix(fieldName, "meta.")
		if tc.isPostgres() {
			if numericCast {
				return fmt.Sprintf(
					"CASE WHEN %s.meta->>'%s' ~ '^-{0,1}[0-9]+(\\.[0-9]+){0,1}$' THEN (%s.meta->>'%s')::numeric ELSE NULL END",
					tc.tableName, key, tc.tableName, key,
				)
			}
			return fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
		}
		return fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
	}
```

With:

```go
	if strings.HasPrefix(fieldName, "meta.") {
		segments := metaSubpathSegments(fieldName)
		if numericCast {
			return tc.metaNumericExpr(segments)
		}
		return tc.metaJsonExpr(segments)
	}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/... -run "TestComprehensive_MetaSubpathOrderBy|TestComprehensive_MetaSubpathGroupBy" -v`
Expected: All pass.

- [ ] **Step 8: Run full test suite for regressions**

Run: `go test --tags 'json1 fts5' ./mrql/... -v`
Expected: All tests pass.

- [ ] **Step 9: Commit**

```bash
git add mrql/translator.go mrql/translator_comprehensive_test.go
git commit -m "feat(mrql): update ORDER BY, GROUP BY, and aggregates for meta subpaths"
```

---

### Task 9: Update buildMetaClause and clean up old code

**Files:**
- Modify: `mrql/translator.go`

- [ ] **Step 1: Check if old buildMetaClause is still used**

Search for callers of `buildMetaClause` (the old version, not `buildMetaClauseV2`). If `translateChainedMetaComparison` was the only caller and it now uses `buildMetaClauseV2`, the old `buildMetaClause` can be removed.

Run: `grep -n 'buildMetaClause[^V]' mrql/translator.go`

If there are no remaining callers, delete `buildMetaClause` and rename `buildMetaClauseV2` to `buildMetaClause`.

- [ ] **Step 2: Run all tests**

Run: `go test --tags 'json1 fts5' ./mrql/... -v`
Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add mrql/translator.go
git commit -m "refactor(mrql): clean up old buildMetaClause after subpath migration"
```

---

### Task 10: Segment validation error tests

**Files:**
- Modify: `mrql/translator_comprehensive_test.go`

- [ ] **Step 1: Write tests that verify invalid segments are rejected**

Add to `mrql/translator_comprehensive_test.go`:

```go
func TestComprehensive_MetaSubpathInvalidSegments(t *testing.T) {
	db := setupTestDB(t)

	invalidQueries := []struct {
		name  string
		query string
	}{
		{"hyphen in segment", `type = "resource" AND meta.a-b.c = 1`},
	}

	for _, tt := range invalidQueries {
		t.Run(tt.name, func(t *testing.T) {
			// The lexer will reject "a-b" since '-' is not part of an identifier.
			// The parser should produce an error, or if it somehow parses,
			// the translator should reject it.
			q, err := Parse(tt.query)
			if err != nil {
				// Parser correctly rejected it
				return
			}
			q.EntityType = EntityResource
			if err := Validate(q); err != nil {
				// Validator correctly rejected it
				return
			}
			_, err = Translate(q, db)
			if err == nil {
				t.Error("expected error for invalid meta segment, got nil")
			}
		})
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test --tags 'json1 fts5' ./mrql/... -run TestComprehensive_MetaSubpathInvalidSegments -v`
Expected: Pass (the lexer or parser will reject `a-b` since `-` terminates the identifier token).

- [ ] **Step 3: Commit**

```bash
git add mrql/translator_comprehensive_test.go
git commit -m "test(mrql): add meta subpath segment validation tests"
```

---

### Task 11: Full regression + Postgres test run

**Files:** None (testing only)

- [ ] **Step 1: Run all Go unit tests**

Run: `go test --tags 'json1 fts5' ./... -count=1`
Expected: All pass.

- [ ] **Step 2: Run E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: All pass.

- [ ] **Step 3: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
Expected: All pass.

- [ ] **Step 4: Run E2E Postgres tests**

Run: `cd e2e && npm run test:with-server:postgres`
Expected: All pass.
