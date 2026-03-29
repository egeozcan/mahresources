# MRQL Owner Traversal & Multi-Level Chaining Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `owner` traversal field to resources/notes and support multi-level FK chaining (e.g., `owner.parent.tags = "y"`) across the MRQL system.

**Architecture:** Extract a generalized FK traversal helper (`fkStep`) from the existing parent/children code. The parser accepts up to 5-part dotted fields. The validator classifies each part as root/intermediate/leaf. The translator builds nested subqueries by walking the chain outside-in. Owner is a new FieldRelation on resources and notes pointing to `owner_id → groups`.

**Tech Stack:** Go (MRQL package), Playwright (E2E), Docusaurus (docs-site), Pongo2 templates (inline docs)

---

## File Map

| File | Responsibility |
|------|---------------|
| `mrql/fields.go` | Add `owner` field definition to resourceFields and noteFields |
| `mrql/parser.go` | Allow up to 5-part dotted fields (remove 2-part cap) |
| `mrql/validator.go` | Chain validation: root/intermediate/leaf classification |
| `mrql/translator.go` | `fkStep` helper, refactor parent/children, add owner routing |
| `mrql/completer.go` | Suggest `owner` for resources/notes, traversal subfields after dots |
| `mrql/translator_test.go` | Seed data with owner relationships, test owner + chained traversal |
| `mrql/validator_test.go` | Test valid/invalid chain validation |
| `mrql/completer_test.go` | Test owner completion suggestions |
| `mrql/parser_test.go` | Test multi-part field parsing |
| `e2e/tests/cli/cli-mrql.spec.ts` | CLI E2E test for owner traversal queries |
| `docs-site/docs/features/mrql.md` | Document owner field, multi-level traversal |
| `templates/mrql.tpl` | Update inline docs panel |

---

### Task 1: Add owner field definitions

**Files:**
- Modify: `mrql/fields.go:33-50`

- [ ] **Step 1: Add owner to resourceFields and noteFields**

In `mrql/fields.go`, add the `owner` field to both slices:

```go
// resourceFields are fields only available on the Resource entity.
var resourceFields = []FieldDef{
	{Name: "groups", Type: FieldRelation, Column: "groups"},
	{Name: "group", Type: FieldRelation, Column: "groups"}, // alias
	{Name: "owner", Type: FieldRelation, Column: "owner_id"},
	{Name: "category", Type: FieldNumber, Column: "resource_category_id"},
	{Name: "contentType", Type: FieldString, Column: "content_type"},
	{Name: "fileSize", Type: FieldNumber, Column: "file_size"},
	{Name: "width", Type: FieldNumber, Column: "width"},
	{Name: "height", Type: FieldNumber, Column: "height"},
	{Name: "originalName", Type: FieldString, Column: "original_name"},
	{Name: "hash", Type: FieldString, Column: "hash"},
}

// noteFields are fields only available on the Note entity.
var noteFields = []FieldDef{
	{Name: "groups", Type: FieldRelation, Column: "groups"},
	{Name: "group", Type: FieldRelation, Column: "groups"}, // alias
	{Name: "owner", Type: FieldRelation, Column: "owner_id"},
	{Name: "noteType", Type: FieldNumber, Column: "note_type_id"},
}
```

- [ ] **Step 2: Verify build**

Run: `go build --tags 'json1 fts5' ./mrql/...`
Expected: compiles clean

- [ ] **Step 3: Run existing tests pass**

Run: `go test --tags 'json1 fts5' ./mrql/...`
Expected: all pass (owner field is defined but not yet used in traversal)

- [ ] **Step 4: Commit**

```
git add mrql/fields.go
git commit -m "feat(mrql): add owner field to resource and note field definitions"
```

---

### Task 2: Allow multi-part dotted fields in the parser

**Files:**
- Modify: `mrql/parser.go:275-314`
- Test: `mrql/parser_test.go`

- [ ] **Step 1: Write the failing test for 3-part fields**

Add to `mrql/parser_test.go`:

```go
// Test: 3-part dotted field parses correctly
func TestParserMultiPartField(t *testing.T) {
	q := mustParse(t, `owner.parent.name = "test"`)
	comp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatal("expected ComparisonExpr")
	}
	if len(comp.Field.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(comp.Field.Parts))
	}
	if comp.Field.Parts[0].Value != "owner" {
		t.Fatalf("expected part[0] = owner, got %q", comp.Field.Parts[0].Value)
	}
	if comp.Field.Parts[1].Value != "parent" {
		t.Fatalf("expected part[1] = parent, got %q", comp.Field.Parts[1].Value)
	}
	if comp.Field.Parts[2].Value != "name" {
		t.Fatalf("expected part[2] = name, got %q", comp.Field.Parts[2].Value)
	}
}

// Test: 5-part field parses correctly (max depth)
func TestParserMaxDepthField(t *testing.T) {
	q := mustParse(t, `owner.parent.parent.parent.name = "test"`)
	comp := q.Where.(*ComparisonExpr)
	if len(comp.Field.Parts) != 5 {
		t.Fatalf("expected 5 parts, got %d", len(comp.Field.Parts))
	}
}

// Test: 6-part field is rejected
func TestParserTooDeepFieldRejected(t *testing.T) {
	_, err := Parse(`a.b.c.d.e.f = "test"`)
	if err == nil {
		t.Fatal("expected error for 6-part field, got nil")
	}
	if !strings.Contains(err.Error(), "too deep") {
		t.Fatalf("expected 'too deep' error, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./mrql/ -run TestParserMultiPartField -v`
Expected: FAIL (parser rejects 3-part fields)

- [ ] **Step 3: Modify parseField to accept up to 5 parts**

Replace the `parseField` method in `mrql/parser.go` (lines 275-314):

```go
// maxFieldParts is the maximum number of parts in a dotted field expression.
// e.g., owner.parent.parent.parent.name = 5 parts (4 traversal + 1 leaf).
const maxFieldParts = 5

// parseField reads a field name: IDENT (. IDENT)* with up to maxFieldParts parts.
func (p *parser) parseField() (*FieldExpr, error) {
	tok := p.lexer.Next()
	if tok.Type != TokenIdentifier && tok.Type != TokenKwType {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected field name (identifier), got %q", tok.Value),
			Pos:     tok.Pos,
			Length:  tok.Length,
		}
	}

	parts := []Token{tok}

	for p.lexer.Peek().Type == TokenDot {
		if len(parts) >= maxFieldParts {
			dotTok := p.lexer.Peek()
			return nil, &ParseError{
				Message: fmt.Sprintf("traversal chain too deep (max %d parts)", maxFieldParts),
				Pos:     dotTok.Pos,
				Length:  dotTok.Length,
			}
		}
		p.lexer.Next() // consume '.'

		nextTok := p.lexer.Next()
		if nextTok.Type != TokenIdentifier && nextTok.Type != TokenKwType {
			return nil, &ParseError{
				Message: fmt.Sprintf("expected identifier after '.', got %q", nextTok.Value),
				Pos:     nextTok.Pos,
				Length:  nextTok.Length,
			}
		}
		parts = append(parts, nextTok)
	}

	return &FieldExpr{Parts: parts}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/ -run TestParser -v`
Expected: all parser tests pass (including the new multi-part ones, and the old `TestParserParentNameField`)

- [ ] **Step 5: Commit**

```
git add mrql/parser.go mrql/parser_test.go
git commit -m "feat(mrql): allow up to 5-part dotted fields in parser"
```

---

### Task 3: Multi-level chain validation

**Files:**
- Modify: `mrql/validator.go:387-473`
- Test: `mrql/validator_test.go`

- [ ] **Step 1: Write failing tests for chain validation**

Add to `mrql/validator_test.go`:

```go
func TestValidatorOwnerTraversal(t *testing.T) {
	// Valid owner queries on resource
	validQueries := []string{
		`type = resource AND owner = "MyGroup"`,
		`type = resource AND owner ~ "Project*"`,
		`type = resource AND owner.name = "test"`,
		`type = resource AND owner.tags = "urgent"`,
		`type = resource AND owner.category = "3"`,
		`type = resource AND owner.parent.name = "Acme"`,
		`type = resource AND owner.parent.tags = "active"`,
		`type = resource AND owner.children.name ~ "Q*"`,
		`type = note AND owner = "MyGroup"`,
		`type = note AND owner.parent.name = "test"`,
		// Existing parent/children chaining on groups
		`type = group AND parent.parent.name = "Root"`,
		`type = group AND parent.parent.tags = "org"`,
		`type = group AND children.parent.name = "X"`,
	}
	for _, q := range validQueries {
		t.Run(q, func(t *testing.T) {
			ast, err := Parse(q)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if err := Validate(ast); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})
	}
}

func TestValidatorOwnerTraversalInvalid(t *testing.T) {
	cases := []struct {
		query string
		errContains string
	}{
		// owner not valid on groups
		{`type = group AND owner = "test"`, "unknown"},
		// owner as intermediate not valid
		{`type = resource AND owner.owner.name = "test"`, "not valid as intermediate"},
		// groups not a traversal field
		{`type = resource AND owner.groups.name = "test"`, "not a traversal field"},
		// meta as leaf in chain not supported
		{`type = resource AND owner.parent.meta = "test"`, "not supported"},
		// owner on unspecified entity type
		{`owner = "test"`, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			ast, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			err = Validate(ast)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("expected error containing %q, got: %v", tc.errContains, err)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./mrql/ -run TestValidatorOwner -v`
Expected: FAIL (validator doesn't recognize owner or multi-level chains)

- [ ] **Step 3: Rewrite validateFieldExpr for chain support**

Replace `validateFieldExpr` in `mrql/validator.go` (starting at line 387):

```go
// traversalRoots lists which entity types allow which root traversal fields.
var traversalRoots = map[string][]EntityType{
	"parent":   {EntityGroup},
	"children": {EntityGroup},
	"owner":    {EntityResource, EntityNote},
}

// traversalIntermediates are fields valid as middle steps in a chain.
// After the first step you're always in groups context, so only
// parent/children are valid intermediates.
var traversalIntermediates = map[string]bool{
	"parent":   true,
	"children": true,
}

// isTraversalRoot returns true if fieldName is a valid traversal root for the entity type.
func isTraversalRoot(fieldName string, entityType EntityType) bool {
	allowedTypes, ok := traversalRoots[fieldName]
	if !ok {
		return false
	}
	for _, et := range allowedTypes {
		if et == entityType {
			return true
		}
	}
	return false
}

// validateFieldExpr checks that the referenced field (or traversal chain) is valid
// for the given entity type.
func validateFieldExpr(f *FieldExpr, entityType EntityType) error {
	if len(f.Parts) == 0 {
		return nil
	}

	firstName := f.Parts[0].Value

	// "type" is always a valid pseudo-field for entity type filtering.
	if firstName == "type" && len(f.Parts) == 1 {
		return nil
	}

	// Single-part: simple field lookup
	if len(f.Parts) == 1 {
		_, ok := LookupField(entityType, firstName)
		if !ok {
			return &ValidationError{
				Message: fmt.Sprintf("unknown or invalid field %q for entity type %s", firstName, entityType),
				Pos:     f.Pos(),
				Length:  len(firstName),
			}
		}
		return nil
	}

	// 2-part "meta.*" is always valid
	if firstName == "meta" && len(f.Parts) == 2 {
		return nil
	}

	// Multi-part: validate as traversal chain
	return validateTraversalChain(f, entityType)
}

// validateTraversalChain validates a dotted field chain like owner.parent.tags.
// Chain structure: root . [intermediate...] . leaf
func validateTraversalChain(f *FieldExpr, entityType EntityType) error {
	parts := f.Parts
	root := parts[0].Value
	leaf := parts[len(parts)-1].Value

	// Validate root
	if !isTraversalRoot(root, entityType) {
		// Check if it's a traversal field on a different entity type
		if _, ok := traversalRoots[root]; ok {
			return &ValidationError{
				Message: fmt.Sprintf("field %q: %s traversal is not valid for %s entities", f.Name(), root, entityType),
				Pos:     f.Pos(),
				Length:  len(f.Name()),
			}
		}
		return &ValidationError{
			Message: fmt.Sprintf("unknown field prefix %q in %q", root, f.Name()),
			Pos:     f.Pos(),
			Length:  len(f.Name()),
		}
	}

	// Validate intermediates (parts[1] through parts[len-2])
	for i := 1; i < len(parts)-1; i++ {
		name := parts[i].Value
		if !traversalIntermediates[name] {
			if name == "owner" {
				return &ValidationError{
					Message: fmt.Sprintf("%q is not valid as intermediate traversal field (only parent/children); owner is only valid as root", name),
					Pos:     parts[i].Pos,
					Length:  len(name),
				}
			}
			if name == "meta" {
				return &ValidationError{
					Message: fmt.Sprintf("meta fields are not supported in traversal chains"),
					Pos:     parts[i].Pos,
					Length:  len(name),
				}
			}
			return &ValidationError{
				Message: fmt.Sprintf("%q is not a traversal field; only parent and children can be used as intermediate steps", name),
				Pos:     parts[i].Pos,
				Length:  len(name),
			}
		}
	}

	// Validate leaf: must be a valid group field (since all traversals target groups)
	if leaf == "meta" {
		return &ValidationError{
			Message: fmt.Sprintf("meta fields are not supported as traversal leaf (would need %s.meta.<key>)", f.Name()),
			Pos:     parts[len(parts)-1].Pos,
			Length:  len(leaf),
		}
	}

	subFd, ok := LookupField(EntityGroup, leaf)
	if !ok && !IsCommonField(leaf) {
		return &ValidationError{
			Message: fmt.Sprintf("unknown field %q for traversal; valid fields: name, description, tags, category, id, created, updated", leaf),
			Pos:     parts[len(parts)-1].Pos,
			Length:  len(leaf),
		}
	}

	// Only tags is supported as a relation leaf (not parent/children/groups)
	if ok || IsCommonField(leaf) {
		if !ok {
			subFd, _ = LookupField(EntityGroup, leaf)
		}
		if subFd.Type == FieldRelation && leaf != "tags" {
			return &ValidationError{
				Message: fmt.Sprintf("field %q is not supported as traversal leaf; only tags, name, category, and other scalar fields are valid", leaf),
				Pos:     parts[len(parts)-1].Pos,
				Length:  len(leaf),
			}
		}
	}

	return nil
}
```

Also update the references in `validateNode` and `validateValueType` that check for 2-part traversals. In `validateNode` (the `InExpr` case around line 214), update the traversal check:

```go
// In the InExpr case, replace the existing 2-part check:
if len(n.Field.Parts) >= 2 {
	prefix := n.Field.Parts[0].Value
	if isTraversalRoot(prefix, entityType) || traversalIntermediates[prefix] {
		return &ValidationError{
			Message: fmt.Sprintf("traversal fields do not support IN operator; use = or != instead"),
			Pos:     n.Field.Pos(),
			Length:  len(n.Field.Name()),
		}
	}
}
```

In the `IsExpr` case (around line 240), update to handle multi-part traversals:

```go
// Replace the existing 2-part traversal IS check with:
if len(n.Field.Parts) >= 2 {
	root := n.Field.Parts[0].Value
	if isTraversalRoot(root, entityType) || traversalIntermediates[root] {
		if !n.IsNull {
			return &ValidationError{
				Message: fmt.Sprintf("traversal fields do not support IS EMPTY; use direct IS EMPTY or field = \"...\" instead"),
				Pos:     n.Field.Pos(),
				Length:  len(n.Field.Name()),
			}
		}
		// IS NULL on multi-part chains: only supported for 2-part with scalar leaf
		leaf := n.Field.Parts[len(n.Field.Parts)-1].Value
		subFd, ok := LookupField(EntityGroup, leaf)
		if ok && subFd.Type == FieldRelation {
			return &ValidationError{
				Message: fmt.Sprintf("IS NULL is not supported on relation traversal subfields; use = for tag comparisons"),
				Pos:     n.Field.Pos(),
				Length:  len(n.Field.Name()),
			}
		}
		// Multi-level IS NULL (>2 parts) is not supported for simplicity
		if len(n.Field.Parts) > 2 {
			return &ValidationError{
				Message: fmt.Sprintf("IS NULL/IS NOT NULL is only supported on single-level traversals (e.g., parent.name IS NULL)"),
				Pos:     n.Field.Pos(),
				Length:  len(n.Field.Name()),
			}
		}
	}
}
```

In `validateValueType` (around line 339), update the traversal skip:

```go
// Replace the existing 2-part traversal check with:
if len(field.Parts) >= 2 {
	prefix := field.Parts[0].Value
	if isTraversalRoot(prefix, entityType) || traversalIntermediates[prefix] {
		return nil // traversal value types validated by translator
	}
}
```

In `validateSortable` (around line 291), update to reject all traversal ORDER BY:

```go
// Replace the existing 2-part check with:
if len(f.Parts) >= 2 {
	prefix := f.Parts[0].Value
	if prefix == "meta" {
		return nil // meta.X is sortable
	}
	return &ValidationError{
		Message: fmt.Sprintf("cannot ORDER BY %s: traversal fields are not sortable", f.Name()),
		Pos:     f.Pos(),
		Length:  len(f.Name()),
	}
}
```

In the `ComparisonExpr` case of `validateNode` (around line 181), update the relation operator check to handle multi-part traversals where root is `owner`:

```go
// Replace the existing relation operator validation:
if !isTypeField(n.Field) {
	fieldName := n.Field.Parts[0].Value
	// For multi-part traversals, operator validation is handled by the translator
	if len(n.Field.Parts) == 1 {
		fd, ok := LookupField(entityType, fieldName)
		if ok && fd.Type == FieldRelation {
			switch n.Operator.Type {
			case TokenEq, TokenNeq, TokenLike, TokenNotLike:
				// supported
			default:
				return &ValidationError{
					Message: fmt.Sprintf("field %q is a relation and only supports =, !=, ~, !~ operators", fieldName),
					Pos:     n.Operator.Pos,
					Length:  n.Operator.Length,
				}
			}
		}
	}
}
```

Also update the bare parent/children IN check (around line 225) to include owner:

```go
if len(n.Field.Parts) == 1 {
	fieldName := n.Field.Parts[0].Value
	if fieldName == "parent" || fieldName == "children" || fieldName == "owner" {
		return &ValidationError{
			Message: fmt.Sprintf("%s does not support IN operator; use %s = \"...\" instead", fieldName, fieldName),
			Pos:     n.Field.Pos(),
			Length:  len(fieldName),
		}
	}
}
```

And update the IS NULL relation check (around line 268) to include owner:

```go
if n.IsNull && len(n.Field.Parts) == 1 {
	fieldName := n.Field.Parts[0].Value
	fd, ok := LookupField(entityType, fieldName)
	if ok && fd.Type == FieldRelation && fieldName != "parent" && fieldName != "children" && fieldName != "owner" {
		return &ValidationError{
			Message: fmt.Sprintf("use \"%s IS EMPTY\" instead of \"%s IS NULL\" for relation fields", fieldName, fieldName),
			Pos:     n.Field.Pos(),
			Length:  len(fieldName),
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/ -run TestValidator -v`
Expected: all validator tests pass

- [ ] **Step 5: Run all MRQL tests for regressions**

Run: `go test --tags 'json1 fts5' ./mrql/...`
Expected: all pass

- [ ] **Step 6: Commit**

```
git add mrql/validator.go mrql/validator_test.go
git commit -m "feat(mrql): chain validation for owner and multi-level traversal"
```

---

### Task 4: Generalized FK traversal helper and translator refactoring

**Files:**
- Modify: `mrql/translator.go`
- Test: `mrql/translator_test.go`

This is the core task. Extract a generalized FK traversal helper, refactor existing parent/children code to use it, and add owner routing.

- [ ] **Step 1: Write failing tests for owner traversal**

Add to `mrql/translator_test.go`. First, update `setupTestDB` to seed owner relationships:

After the existing resource creation block (around line 136), add owner relationships:

```go
// Set owner_id on resources: resource 1 owned by Vacation (group 1), resource 3 owned by Work (group 2)
db.Model(&testResource{}).Where("id = ?", 1).Update("owner_id", 1)
db.Model(&testResource{}).Where("id = ?", 3).Update("owner_id", 2)

// Set owner_id on notes: note 1 owned by Vacation (group 1), note 2 owned by Work (group 2)
db.Model(&testNote{}).Where("id = ?", 1).Update("owner_id", 1)
db.Model(&testNote{}).Where("id = ?", 2).Update("owner_id", 2)

// Add more group_tags for traversal testing: group 2 (Work) has tag "document"
db.Exec("INSERT INTO group_tags (group_id, tag_id) VALUES (2, 3)")
```

Then add the test functions:

```go
// TestTranslateOwnerDirect tests owner = "name" on resources.
func TestTranslateOwnerDirect(t *testing.T) {
	db := setupTestDB(t)

	// Resource 1 is owned by "Vacation"
	result := parseAndTranslate(t, `owner = "Vacation"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected [sunset.jpg], got %v", namesOfResources(resources))
	}
}

// TestTranslateOwnerTags tests owner.tags = "tagname" on resources.
func TestTranslateOwnerTags(t *testing.T) {
	db := setupTestDB(t)

	// Resource 1 owned by Vacation (has tag "photo"), Resource 3 owned by Work (has tag "document")
	result := parseAndTranslate(t, `owner.tags = "photo"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected [sunset.jpg], got %v", namesOfResources(resources))
	}
}

// TestTranslateOwnerName tests owner.name = "value" on resources.
func TestTranslateOwnerName(t *testing.T) {
	db := setupTestDB(t)

	result := parseAndTranslate(t, `owner.name = "Work"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "report.pdf" {
		t.Fatalf("expected [report.pdf], got %v", namesOfResources(resources))
	}
}

// TestTranslateOwnerParentChain tests owner.parent.name = "value" (multi-level).
func TestTranslateOwnerParentChain(t *testing.T) {
	db := setupTestDB(t)

	// Resource 3 owned by Work, whose parent is Vacation
	result := parseAndTranslate(t, `owner.parent.name = "Vacation"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "report.pdf" {
		t.Fatalf("expected [report.pdf], got %v", namesOfResources(resources))
	}
}

// TestTranslateOwnerParentTags tests owner.parent.tags multi-level chaining.
func TestTranslateOwnerParentTags(t *testing.T) {
	db := setupTestDB(t)

	// Resource 3 owned by Work, Work's parent is Vacation, Vacation has tag "photo"
	result := parseAndTranslate(t, `owner.parent.tags = "photo"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "report.pdf" {
		t.Fatalf("expected [report.pdf], got %v", namesOfResources(resources))
	}
}

// TestTranslateParentParentChain tests parent.parent.name on groups.
func TestTranslateParentParentChain(t *testing.T) {
	db := setupTestDB(t)

	// Sub-Work (ID=4) -> parent Work (ID=2) -> parent Vacation (ID=1)
	result := parseAndTranslate(t, `parent.parent.name = "Vacation"`, EntityGroup, db)
	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "Sub-Work" {
		t.Fatalf("expected [Sub-Work], got %v", namesOfGroups(groups))
	}
}

// TestTranslateOwnerNegationNull tests owner != includes resources with no owner.
func TestTranslateOwnerNegationNull(t *testing.T) {
	db := setupTestDB(t)

	// Resources 2 and 4 have no owner; resource 3 is owned by "Work"
	// owner != "Work" should return resources 1, 2, 4 (not 3)
	result := parseAndTranslate(t, `owner != "Work"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	names := namesOfResources(resources)
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources, got %d: %v", len(resources), names)
	}
}

// TestTranslateNoteOwner tests owner traversal on notes.
func TestTranslateNoteOwner(t *testing.T) {
	db := setupTestDB(t)

	result := parseAndTranslate(t, `owner.tags = "document"`, EntityNote, db)
	var notes []testNote
	if err := result.Find(&notes).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(notes) != 1 || notes[0].Name != "Todo list" {
		t.Fatalf("expected [Todo list], got %v", namesOfNotes(notes))
	}
}
```

You'll also need a `namesOfResources` helper if it doesn't exist:

```go
func namesOfResources(resources []testResource) []string {
	names := make([]string, len(resources))
	for i, r := range resources {
		names[i] = r.Name
	}
	return names
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./mrql/ -run TestTranslateOwner -v`
Expected: FAIL (translator doesn't handle owner or multi-level chains)

- [ ] **Step 3: Implement the generalized FK traversal helper**

Add the following to `mrql/translator.go`. Place it after the existing `translateTraversalTagComparison` function (around line 626):

```go
// fkStep describes one step in a FK traversal chain.
type fkStep struct {
	fkExpr    string // source FK expression, e.g. "resources.owner_id"
	selectCol string // what to SELECT: "t0.id" (forward) or "t0.owner_id" (reverse/children)
	alias     string // subquery table alias
}

// traversalFieldNames that indicate a valid traversal step.
var traversalFieldNames = map[string]bool{
	"owner":    true,
	"parent":   true,
	"children": true,
}

// buildFKStep creates an fkStep for a given traversal field name.
// outerRef is the qualified column reference from the outer query (e.g., "resources.owner_id" or "t0.owner_id").
// idx is used to generate unique aliases (t0, t1, t2...).
func buildFKStep(fieldName string, outerRef string, idx int) fkStep {
	alias := fmt.Sprintf("_t%d", idx)
	switch fieldName {
	case "children":
		// Reverse FK: children of a group are groups whose owner_id = group.id
		return fkStep{
			fkExpr:    outerRef,
			selectCol: alias + ".owner_id",
			alias:     alias,
		}
	default:
		// Forward FK (owner, parent): follow owner_id to groups.id
		return fkStep{
			fkExpr:    outerRef,
			selectCol: alias + ".id",
			alias:     alias,
		}
	}
}

// buildTraversalChain constructs the list of fkSteps for a multi-part traversal.
// parts[0] is the root (owner/parent/children), parts[1..n-2] are intermediates, parts[n-1] is the leaf.
// The returned steps cover parts[0] through parts[n-2]; the leaf is handled separately.
func (tc *translateContext) buildTraversalChain(parts []Token) []fkStep {
	var steps []fkStep

	for i := 0; i < len(parts)-1; i++ {
		fieldName := parts[i].Value
		var outerRef string

		if i == 0 {
			// First step: FK from the source entity table
			switch fieldName {
			case "children":
				outerRef = tc.tableName + ".id"
			default: // owner, parent
				outerRef = tc.tableName + ".owner_id"
			}
		} else {
			// Subsequent steps: FK from the previous step's alias
			prevAlias := steps[i-1].alias
			switch fieldName {
			case "children":
				outerRef = prevAlias + ".id"
			default: // parent
				outerRef = prevAlias + ".owner_id"
			}
		}

		steps = append(steps, buildFKStep(fieldName, outerRef, i))
	}

	return steps
}

// translateFKChainScalar generates nested subqueries for a chained traversal
// ending in a scalar field comparison.
func (tc *translateContext) translateFKChainScalar(db *gorm.DB, steps []fkStep, leafCol string, op Token, val interface{}, leafFd FieldDef) (*gorm.DB, error) {
	// Build the innermost WHERE clause for the leaf field
	innerAlias := steps[len(steps)-1].alias
	innerWhere, innerVal := tc.buildScalarClause(innerAlias+"."+leafCol, op, val, leafFd)

	// Children steps need an extra IS NOT NULL filter on owner_id
	sql, vals := tc.wrapChainSubqueries(steps, innerWhere, []interface{}{innerVal})

	// Apply negation NULL handling on outermost step only
	isNegated := op.Type == TokenNeq || op.Type == TokenNotLike
	if isNegated {
		sql = "(" + sql + " OR " + steps[0].fkExpr + " IS NULL)"
	}

	db = db.Where(sql, vals...)
	return db, nil
}

// translateFKChainTag generates nested subqueries for a chained traversal
// ending in a tags comparison.
func (tc *translateContext) translateFKChainTag(db *gorm.DB, steps []fkStep, op Token, val interface{}) (*gorm.DB, error) {
	isNegated := op.Type == TokenNeq || op.Type == TokenNotLike
	isLike := op.Type == TokenLike || op.Type == TokenNotLike

	var tagMatchClause string
	var tagMatchVal interface{}

	if isLike {
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		tagMatchClause = "LOWER(t.name) " + likeOp + " LOWER(?) ESCAPE '\\'"
		tagMatchVal = likePattern
	} else {
		tagMatchClause = "LOWER(t.name) = LOWER(?)"
		tagMatchVal = val
	}

	inOrNotIn := "IN"
	if isNegated {
		inOrNotIn = "NOT IN"
	}

	// The innermost subquery: find group IDs that have (or don't have) the tag
	innerAlias := steps[len(steps)-1].alias
	innerWhere := fmt.Sprintf("%s.id %s (SELECT gt.group_id FROM group_tags gt JOIN tags t ON t.id = gt.tag_id WHERE %s)",
		innerAlias, inOrNotIn, tagMatchClause)

	sql, vals := tc.wrapChainSubqueries(steps, innerWhere, []interface{}{tagMatchVal})

	if isNegated {
		sql = "(" + sql + " OR " + steps[0].fkExpr + " IS NULL)"
	}

	db = db.Where(sql, vals...)
	return db, nil
}

// wrapChainSubqueries wraps the innermost WHERE clause in nested subqueries
// for each step, from inside out.
func (tc *translateContext) wrapChainSubqueries(steps []fkStep, innerWhere string, innerVals []interface{}) (string, []interface{}) {
	// Start from the innermost step and wrap outward
	currentWhere := innerWhere
	currentVals := innerVals

	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]

		// Children steps: add owner_id IS NOT NULL filter
		childFilter := ""
		if i < len(steps)-1 {
			// This is an intermediate step — check if the NEXT step's field was children
			// Actually we detect children by selectCol containing "owner_id"
		}
		if strings.HasSuffix(step.selectCol, ".owner_id") {
			// This is a children (reverse) step
			childFilter = step.alias + ".owner_id IS NOT NULL AND "
		}

		if i == 0 {
			// Outermost: use the source FK
			currentWhere = fmt.Sprintf("%s IN (SELECT %s FROM groups %s WHERE %s%s)",
				step.fkExpr, step.selectCol, step.alias, childFilter, currentWhere)
		} else {
			// Intermediate: the previous step's reference
			prevStep := steps[i-1]
			_ = prevStep // FK expr is already baked into step.fkExpr
			currentWhere = fmt.Sprintf("%s IN (SELECT %s FROM groups %s WHERE %s%s)",
				step.fkExpr, step.selectCol, step.alias, childFilter, currentWhere)
		}
	}

	return currentWhere, currentVals
}

// buildScalarClause builds a WHERE clause fragment for a scalar field comparison.
func (tc *translateContext) buildScalarClause(qualifiedCol string, op Token, val interface{}, fd FieldDef) (string, interface{}) {
	sqlOp := tc.sqlOperator(op)

	if op.Type == TokenLike || op.Type == TokenNotLike {
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		if op.Type == TokenNotLike {
			likeOp = "NOT " + likeOp
		}
		return qualifiedCol + " " + likeOp + " ? ESCAPE '\\'", likePattern
	}

	if fd.Type == FieldString && (op.Type == TokenEq || op.Type == TokenNeq) {
		return "LOWER(" + qualifiedCol + ") " + sqlOp + " LOWER(?)", val
	}

	return qualifiedCol + " " + sqlOp + " ?", val
}
```

- [ ] **Step 4: Update translateComparisonExpr to route traversal chains**

Replace the existing 2-part traversal routing in `translateComparisonExpr` (around line 212-218):

```go
	// Handle traversal chains: owner.X, parent.X, children.X, owner.parent.X, etc.
	if len(expr.Field.Parts) >= 2 {
		root := expr.Field.Parts[0].Value
		if traversalFieldNames[root] {
			return tc.translateChainedComparison(db, expr)
		}
	}
```

Add the new routing function:

```go
// translateChainedComparison handles any multi-part traversal (owner.X, parent.X,
// parent.parent.X, owner.parent.tags, etc.) using the generalized FK chain builder.
func (tc *translateContext) translateChainedComparison(db *gorm.DB, expr *ComparisonExpr) (*gorm.DB, error) {
	parts := expr.Field.Parts
	leaf := parts[len(parts)-1].Value

	// Look up the leaf field on the group entity
	subFd, ok := LookupField(EntityGroup, leaf)
	if !ok && !IsCommonField(leaf) {
		return nil, &TranslateError{
			Message: fmt.Sprintf("unknown field %q for traversal", leaf),
			Pos:     parts[len(parts)-1].Pos,
		}
	}
	if IsCommonField(leaf) && !ok {
		subFd, _ = LookupField(EntityGroup, leaf)
	}

	val, err := tc.resolveValue(expr.Value, subFd)
	if err != nil {
		return nil, err
	}

	steps := tc.buildTraversalChain(parts)

	// Route based on leaf type
	if subFd.Type == FieldRelation && subFd.Column == "tags" {
		return tc.translateFKChainTag(db, steps, expr.Operator, val)
	}

	return tc.translateFKChainScalar(db, steps, subFd.Column, expr.Operator, val, subFd)
}
```

- [ ] **Step 5: Update translateRelationComparison to route owner**

In `translateRelationComparison` (around line 270), add the `owner_id` case:

```go
case "owner_id":
	// Direct owner comparison: owner = "name"
	// Use the chain builder with a single step for consistency
	steps := tc.buildTraversalChain([]Token{
		{Value: "owner"},
		{Value: "name"}, // synthetic leaf — we're matching by name
	})
	return tc.translateFKChainScalar(db, steps, "name", op, val, FieldDef{Name: "name", Type: FieldString, Column: "name"})
```

- [ ] **Step 6: Update translateIsExpr for owner and multi-part traversals**

In `translateIsExpr` (around line 920-929), update the traversal IS NULL routing:

```go
	// Handle traversal IS NULL / IS NOT NULL (only for 2-part chains)
	if len(expr.Field.Parts) == 2 && expr.IsNull {
		root := expr.Field.Parts[0].Value
		if traversalFieldNames[root] {
			return tc.translateTraversalIsNull(db, expr, root)
		}
	}
```

Update `translateTraversalIsNull` to use the entity's table name instead of hardcoded "groups":

```go
func (tc *translateContext) translateTraversalIsNull(db *gorm.DB, expr *IsExpr, root string) (*gorm.DB, error) {
	subField := expr.Field.Parts[1].Value

	subFd, ok := LookupField(EntityGroup, subField)
	if !ok && !IsCommonField(subField) {
		return nil, &TranslateError{
			Message: fmt.Sprintf("unknown field %q for %s traversal", subField, root),
			Pos:     expr.Field.Parts[1].Pos,
		}
	}
	if IsCommonField(subField) {
		subFd, _ = LookupField(EntityGroup, subField)
	}

	col := subFd.Column
	fkCol := tc.tableName + ".owner_id"

	if root == "children" {
		// children.X IS NULL → has some child where X is null (or no children)
		if expr.Negated {
			db = db.Where(
				fmt.Sprintf("%s.id IN (SELECT c.owner_id FROM groups c WHERE c.%s IS NOT NULL AND c.owner_id IS NOT NULL)", tc.tableName, col),
			)
		} else {
			db = db.Where(
				fmt.Sprintf("(%s.id IN (SELECT c.owner_id FROM groups c WHERE c.%s IS NULL AND c.owner_id IS NOT NULL) OR %s.id NOT IN (SELECT owner_id FROM groups WHERE owner_id IS NOT NULL))", tc.tableName, col, tc.tableName),
			)
		}
	} else {
		// parent/owner: X IS NULL → target exists but X is null, OR no target
		if expr.Negated {
			db = db.Where(
				fmt.Sprintf("%s IN (SELECT p.id FROM groups p WHERE p.%s IS NOT NULL)", fkCol, col),
			)
		} else {
			db = db.Where(
				fmt.Sprintf("(%s IN (SELECT p.id FROM groups p WHERE p.%s IS NULL) OR %s IS NULL)", fkCol, col, fkCol),
			)
		}
	}

	return db, nil
}
```

- [ ] **Step 7: Remove old functions that are now replaced by the chain builder**

Delete `translateParentComparison`, `translateChildrenComparison`, `translateTraversalComparison`, and `translateTraversalTagComparison`. Their logic is now handled by `translateChainedComparison` → `buildTraversalChain` → `translateFKChainScalar`/`translateFKChainTag`.

Also update `translateRelationComparison` to route `parent_id` and `children` through the chain builder:

```go
case "parent_id":
	steps := tc.buildTraversalChain([]Token{
		{Value: "parent"},
		{Value: "name"},
	})
	return tc.translateFKChainScalar(db, steps, "name", op, val, FieldDef{Name: "name", Type: FieldString, Column: "name"})
case "children":
	steps := tc.buildTraversalChain([]Token{
		{Value: "children"},
		{Value: "name"},
	})
	return tc.translateFKChainScalar(db, steps, "name", op, val, FieldDef{Name: "name", Type: FieldString, Column: "name"})
```

- [ ] **Step 8: Run all tests**

Run: `go test --tags 'json1 fts5' ./mrql/...`
Expected: all pass including new owner tests and all existing parent/children tests

- [ ] **Step 9: Run full Go test suite**

Run: `go test --tags 'json1 fts5' ./...`
Expected: all pass

- [ ] **Step 10: Commit**

```
git add mrql/translator.go mrql/translator_test.go
git commit -m "feat(mrql): generalized FK traversal with owner and multi-level chaining"
```

---

### Task 5: Completer updates

**Files:**
- Modify: `mrql/completer.go`
- Test: `mrql/completer_test.go`

- [ ] **Step 1: Write failing tests**

Add to `mrql/completer_test.go`:

```go
func TestCompleterOwnerDot(t *testing.T) {
	suggestions := Complete(`type = "resource" AND owner.`, 28)
	hasName := false
	hasTags := false
	for _, s := range suggestions {
		if s.Value == "name" { hasName = true }
		if s.Value == "tags" { hasTags = true }
	}
	if !hasName || !hasTags {
		t.Fatalf("after owner., expected name and tags in suggestions; got %v", suggestions)
	}
}

func TestCompleterOwnerFieldSuggestion(t *testing.T) {
	suggestions := Complete(`type = "resource" AND `, 22)
	hasOwner := false
	for _, s := range suggestions {
		if s.Value == "owner" { hasOwner = true }
	}
	if !hasOwner {
		t.Fatalf("expected owner in field suggestions for resource; got %v", suggestions)
	}
}

func TestCompleterOwnerParentDot(t *testing.T) {
	suggestions := Complete(`type = "resource" AND owner.parent.`, 35)
	hasName := false
	for _, s := range suggestions {
		if s.Value == "name" { hasName = true }
	}
	if !hasName {
		t.Fatalf("after owner.parent., expected name in suggestions; got %v", suggestions)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./mrql/ -run TestCompleterOwner -v`
Expected: FAIL

- [ ] **Step 3: Update completer for owner and multi-level traversal**

In `mrql/completer.go`, update `suggestionsForContext` (around line 219). Replace the dot handler:

```go
	// After a dot — context depends on what's before the dot.
	if last.Type == TokenDot && len(tokens) >= 2 {
		prev := tokens[len(tokens)-2]
		switch prev.Value {
		case "parent", "children", "owner":
			return traversalSubFieldSuggestions(entityType)
		default:
			return metaSubFieldSuggestions
		}
	}
```

Also update `traversalSubFieldSuggestions` to include `parent` and `children` as sub-suggestions (for chaining):

```go
func traversalSubFieldSuggestions(entityType EntityType) []Suggestion {
	var suggestions []Suggestion
	// Common fields valid on groups
	for name := range commonIndex {
		if name == "tags" {
			suggestions = append(suggestions, Suggestion{Value: name, Type: "field", Label: "group tag"})
		} else {
			suggestions = append(suggestions, Suggestion{Value: name, Type: "field"})
		}
	}
	// Group-specific scalar fields (category)
	for name, fd := range groupIndex {
		if fd.Type != FieldRelation {
			suggestions = append(suggestions, Suggestion{Value: name, Type: "field"})
		}
	}
	// Traversal intermediates for chaining
	suggestions = append(suggestions, Suggestion{Value: "parent", Type: "field", Label: "parent group traversal"})
	suggestions = append(suggestions, Suggestion{Value: "children", Type: "field", Label: "child groups traversal"})
	return suggestions
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./mrql/ -run TestCompleter -v`
Expected: all pass

- [ ] **Step 5: Commit**

```
git add mrql/completer.go mrql/completer_test.go
git commit -m "feat(mrql): completer suggests owner field and chained traversal subfields"
```

---

### Task 6: Update documentation

**Files:**
- Modify: `docs-site/docs/features/mrql.md`
- Modify: `templates/mrql.tpl`

- [ ] **Step 1: Update docs-site field tables**

In `docs-site/docs/features/mrql.md`, add `owner` to resource-only and note-only field tables:

Resource-only fields table — add row:
```
| `owner` | relation | Owner group (match by name, supports traversal) |
```

Note-only fields table — add row:
```
| `owner` | relation | Owner group (match by name, supports traversal) |
```

- [ ] **Step 2: Add Traversal section to docs-site**

Replace the existing Traversal section (around line 262) with an expanded version:

```markdown
## Traversal

MRQL supports filtering by properties of related groups through dotted field paths. Traversal works on:

- **Resources and notes:** `owner` accesses the owner group
- **Groups:** `parent` accesses the parent group, `children` accesses child groups

### Single-Level Traversal

```
type = resource AND owner.name = "Project Alpha"
type = resource AND owner.tags = "active"
type = resource AND owner.category = "3"
type = group AND parent.name = "Acme Corp"
type = group AND children.name ~ "Q*"
```

### Multi-Level Traversal

Chain traversal fields to reach groups further up or down the hierarchy. After the first step, you're always in group context, so `parent` and `children` are the valid intermediate steps:

```
type = resource AND owner.parent.name = "Acme Corp"
type = resource AND owner.parent.tags = "active"
type = note AND owner.children.name ~ "Sprint*"
type = group AND parent.parent.name = "Root"
type = group AND parent.parent.tags = "org-level"
```

Maximum traversal depth is 5 parts (4 traversal steps + 1 leaf field).

### Valid Traversal Subfields

At the end of a traversal chain, you can access any group field:

- **Scalar:** `name`, `description`, `category`, `id`, `created`, `updated`
- **Relation:** `tags` (match by tag name)

`meta.*` fields are not supported in traversal chains.

Traversal fields follow the same operators as regular fields. Traversal deeper than 5 levels is not supported.
```

- [ ] **Step 3: Add owner examples to the cookbook**

Add to the Examples Cookbook section:

```markdown
### Resources owned by a specific group

```​
type = resource AND owner = "Project Alpha"
```​

### Resources whose owner has a specific tag

```​
type = resource AND tags = "photo" AND owner.tags = "active"
```​

### Resources whose owner's parent matches

```​
type = resource AND owner.parent.name = "Acme Corp"
```​

### Groups with deeply nested parent

```​
type = group AND parent.parent.name = "Root Organization"
```​
```

- [ ] **Step 4: Update inline docs in mrql.tpl**

In `templates/mrql.tpl`, add owner to the Resource Fields section:

```html
<div>
    <h3 class="font-semibold text-stone-700">Resource Fields</h3>
    <p class="text-xs">
        <code class="bg-stone-200 px-1 rounded">groups</code>,
        <code class="bg-stone-200 px-1 rounded">owner</code>,
        <code class="bg-stone-200 px-1 rounded">category</code>,
        <code class="bg-stone-200 px-1 rounded">contentType</code>,
        <code class="bg-stone-200 px-1 rounded">fileSize</code>,
        <code class="bg-stone-200 px-1 rounded">width</code>,
        <code class="bg-stone-200 px-1 rounded">height</code>,
        <code class="bg-stone-200 px-1 rounded">originalName</code>,
        <code class="bg-stone-200 px-1 rounded">hash</code>
    </p>
</div>
```

Update Note Fields similarly:

```html
<div>
    <h3 class="font-semibold text-stone-700">Note Fields</h3>
    <p class="text-xs">
        <code class="bg-stone-200 px-1 rounded">groups</code>,
        <code class="bg-stone-200 px-1 rounded">owner</code>,
        <code class="bg-stone-200 px-1 rounded">noteType</code>
    </p>
</div>
```

Add a traversal section after the wildcards section:

```html
<div>
    <h3 class="font-semibold text-stone-700">Traversal</h3>
    <p class="text-xs">Use dotted paths to filter by related group properties:
    <code class="bg-stone-200 px-1 rounded">owner.tags = "x"</code>,
    <code class="bg-stone-200 px-1 rounded">owner.parent.name = "y"</code>,
    <code class="bg-stone-200 px-1 rounded">parent.parent.tags = "z"</code>.
    Chain up to 5 levels deep.</p>
</div>
```

- [ ] **Step 5: Commit**

```
git add docs-site/docs/features/mrql.md templates/mrql.tpl
git commit -m "docs(mrql): document owner traversal and multi-level chaining"
```

---

### Task 7: E2E tests

**Files:**
- Modify: `e2e/tests/cli/cli-mrql.spec.ts`

- [ ] **Step 1: Add CLI E2E tests for owner traversal**

Add a new describe block to `e2e/tests/cli/cli-mrql.spec.ts`:

```typescript
test.describe('MRQL owner traversal', () => {
  let categoryId: number;
  let parentGroupId: number;
  let childGroupId: number;
  let resourceId: number;
  let tagId: number;

  test.beforeAll(async ({ request, baseURL }) => {
    // Create a tag
    const tagResp = await request.post(`${baseURL}/v1/tag`, {
      form: { Name: 'owner-test-tag' },
    });
    tagId = (await tagResp.json()).ID;

    // Create parent group
    const parentResp = await request.post(`${baseURL}/v1/group`, {
      form: { Name: 'OwnerTestParent' },
    });
    parentGroupId = (await parentResp.json()).ID;

    // Tag the parent group
    await request.post(`${baseURL}/v1/groups/addTags`, {
      form: { ids: String(parentGroupId), tagIds: String(tagId) },
    });

    // Create child group owned by parent
    const childResp = await request.post(`${baseURL}/v1/group`, {
      form: { Name: 'OwnerTestChild', OwnerID: String(parentGroupId) },
    });
    childGroupId = (await childResp.json()).ID;

    // Create a resource owned by the child group
    const fs = await import('fs');
    const path = await import('path');
    const testFilePath = path.join(__dirname, '../../test-assets/sample-image-34.png');
    const fileBuffer = fs.readFileSync(testFilePath);
    const resourceResp = await request.post(`${baseURL}/v1/resource`, {
      multipart: {
        file: { name: 'owner-test.png', mimeType: 'image/png', buffer: fileBuffer },
        Name: 'OwnerTestResource',
        OwnerID: String(childGroupId),
      },
    });
    resourceId = (await resourceResp.json()).ID;
  });

  test('owner = "name" finds resources by owner name', async ({ cli }) => {
    const result = cli.run('mrql', 'type = resource AND owner = "OwnerTestChild"', '--json');
    expect(result.exitCode).toBe(0);
    const parsed = JSON.parse(result.stdout);
    expect(parsed.resources?.length).toBeGreaterThanOrEqual(1);
    const names = parsed.resources.map((r: any) => r.Name);
    expect(names).toContain('OwnerTestResource');
  });

  test('owner.parent.name chains through hierarchy', async ({ cli }) => {
    const result = cli.run('mrql', 'type = resource AND owner.parent.name = "OwnerTestParent"', '--json');
    expect(result.exitCode).toBe(0);
    const parsed = JSON.parse(result.stdout);
    expect(parsed.resources?.length).toBeGreaterThanOrEqual(1);
    const names = parsed.resources.map((r: any) => r.Name);
    expect(names).toContain('OwnerTestResource');
  });

  test('owner.parent.tags chains to parent tags', async ({ cli }) => {
    const result = cli.run('mrql', 'type = resource AND owner.parent.tags = "owner-test-tag"', '--json');
    expect(result.exitCode).toBe(0);
    const parsed = JSON.parse(result.stdout);
    expect(parsed.resources?.length).toBeGreaterThanOrEqual(1);
    const names = parsed.resources.map((r: any) => r.Name);
    expect(names).toContain('OwnerTestResource');
  });

  test.afterAll(async ({ request, baseURL }) => {
    try {
      if (resourceId) await request.post(`${baseURL}/v1/resource/delete`, { form: { id: String(resourceId) } });
      if (childGroupId) await request.post(`${baseURL}/v1/group/delete`, { form: { id: String(childGroupId) } });
      if (parentGroupId) await request.post(`${baseURL}/v1/group/delete`, { form: { id: String(parentGroupId) } });
      if (tagId) await request.post(`${baseURL}/v1/tag/delete`, { form: { id: String(tagId) } });
    } catch { /* cleanup best-effort */ }
  });
});
```

- [ ] **Step 2: Build and run E2E tests**

Run: `cd /Users/egecan/Code/mahresources && npm run build && cd e2e && npm run test:with-server:all`
Expected: all pass

- [ ] **Step 3: Commit**

```
git add e2e/tests/cli/cli-mrql.spec.ts
git commit -m "test(mrql): E2E tests for owner traversal and multi-level chaining"
```

---

### Task 8: Final verification

- [ ] **Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: all pass

- [ ] **Step 2: Build the application**

Run: `npm run build`
Expected: builds clean

- [ ] **Step 3: Run all E2E tests**

Run: `cd e2e && npm run test:with-server:all`
Expected: all pass

- [ ] **Step 4: Final commit (if any remaining changes)**

Verify `git status` is clean or commit any remaining files.
