package mrql

import (
	"strings"
	"testing"
)

func TestValidator(t *testing.T) {
	t.Run("valid_resource_fields", func(t *testing.T) {
		queries := []string{
			`name = "foo"`,
			`contentType = "image/png"`,
			`fileSize > 100`,
			`width > 0`,
			`height > 0`,
			`originalName ~ "foo"`,
			`hash = "abc"`,
			`tags IN ("foo", "bar")`,
			`groups = "mygroup"`,
			`category = "photos"`,
			`created > -7d`,
			`updated = NOW()`,
			`id = 1`,
			`description ~ "test"`,
			`meta.rating = 5`,
			`meta.custom_key = "value"`,
			`type = "resource" AND contentType = "image/png"`,
		}
		for _, q := range queries {
			t.Run(q, func(t *testing.T) {
				ast, err := Parse(q)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				ast.EntityType = EntityResource
				if err := Validate(ast); err != nil {
					t.Errorf("expected valid, got error: %v", err)
				}
			})
		}
	})

	t.Run("valid_note_fields", func(t *testing.T) {
		queries := []string{
			`name = "foo"`,
			`noteType = "personal"`,
			`tags IN ("foo")`,
			`groups = "mygroup"`,
			`created > -30d`,
			`id = 42`,
			`meta.priority = "high"`,
			`type = "note" AND noteType = "work"`,
		}
		for _, q := range queries {
			t.Run(q, func(t *testing.T) {
				ast, err := Parse(q)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				ast.EntityType = EntityNote
				if err := Validate(ast); err != nil {
					t.Errorf("expected valid, got error: %v", err)
				}
			})
		}
	})

	t.Run("valid_group_fields", func(t *testing.T) {
		queries := []string{
			`name = "foo"`,
			`category = "albums"`,
			`parent.name = "root"`,
			`children IS EMPTY`,
			`tags IN ("foo")`,
			`created > -7d`,
			`id = 1`,
			`meta.color = "blue"`,
			`type = "group" AND parent.name = "root"`,
		}
		for _, q := range queries {
			t.Run(q, func(t *testing.T) {
				ast, err := Parse(q)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				ast.EntityType = EntityGroup
				if err := Validate(ast); err != nil {
					t.Errorf("expected valid, got error: %v", err)
				}
			})
		}
	})

	t.Run("invalid_field_name", func(t *testing.T) {
		cases := []struct {
			query string
			field string
		}{
			{`foobar = "x"`, "foobar"},
			{`unknownField > 0`, "unknownField"},
			{`blah ~ "test"`, "blah"},
		}
		for _, tc := range cases {
			t.Run(tc.query, func(t *testing.T) {
				ast, err := Parse(tc.query)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				ast.EntityType = EntityResource
				err = Validate(ast)
				if err == nil {
					t.Errorf("expected error for unknown field %q, got nil", tc.field)
					return
				}
				if !strings.Contains(err.Error(), tc.field) {
					t.Errorf("expected error to mention field %q, got: %v", tc.field, err)
				}
			})
		}
	})

	t.Run("field_not_valid_for_entity", func(t *testing.T) {
		cases := []struct {
			query      string
			entityType EntityType
			badField   string
		}{
			// contentType is resource-only
			{`contentType = "image/png"`, EntityNote, "contentType"},
			{`contentType = "image/png"`, EntityGroup, "contentType"},
			// noteType is note-only
			{`noteType = "personal"`, EntityResource, "noteType"},
			{`noteType = "personal"`, EntityGroup, "noteType"},
			// fileSize is resource-only
			{`fileSize > 100`, EntityNote, "fileSize"},
			// width/height are resource-only
			{`width > 0`, EntityGroup, "width"},
			{`height > 0`, EntityNote, "height"},
			// parent/children are group-only
			{`parent.name = "root"`, EntityResource, "parent"},
			{`parent.name = "root"`, EntityNote, "parent"},
			{`children IS EMPTY`, EntityResource, "children"},
		}
		for _, tc := range cases {
			t.Run(tc.query, func(t *testing.T) {
				ast, err := Parse(tc.query)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				ast.EntityType = tc.entityType
				err = Validate(ast)
				if err == nil {
					t.Errorf("expected error for field %q on %v entity, got nil", tc.badField, tc.entityType)
					return
				}
				if !strings.Contains(err.Error(), tc.badField) {
					t.Errorf("expected error to mention field %q, got: %v", tc.badField, err)
				}
			})
		}
	})

	t.Run("traversal_on_non_group", func(t *testing.T) {
		cases := []struct {
			query      string
			entityType EntityType
		}{
			{`parent.name = "root"`, EntityResource},
			{`parent.name = "root"`, EntityNote},
			{`children IS EMPTY`, EntityResource},
		}
		for _, tc := range cases {
			t.Run(tc.query, func(t *testing.T) {
				ast, err := Parse(tc.query)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				ast.EntityType = tc.entityType
				err = Validate(ast)
				if err == nil {
					t.Errorf("expected traversal error for %q on %v, got nil", tc.query, tc.entityType)
				}
			})
		}
	})

	t.Run("invalid_entity_type_value", func(t *testing.T) {
		cases := []string{
			`type = "foobar"`,
			`type = "invalid"`,
		}
		for _, q := range cases {
			t.Run(q, func(t *testing.T) {
				ast, err := Parse(q)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				// entity type unspecified; validator should catch invalid type value
				err = Validate(ast)
				if err == nil {
					t.Errorf("expected error for invalid entity type value in %q, got nil", q)
				}
			})
		}
	})

	t.Run("valid_entity_type_comparisons", func(t *testing.T) {
		cases := []string{
			`type = "resource"`,
			`type = "note"`,
			`type = "group"`,
			`type = "RESOURCE"`,   // case-insensitive
			`type = "Note"`,       // mixed case
			`type = "GROUP"`,      // uppercase
		}
		for _, q := range cases {
			t.Run(q, func(t *testing.T) {
				ast, err := Parse(q)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				if err := Validate(ast); err != nil {
					t.Errorf("expected valid, got: %v", err)
				}
			})
		}
	})

	t.Run("cross_entity_allows_common_fields", func(t *testing.T) {
		// No entity type set — only common fields should be allowed
		queries := []string{
			`name = "foo"`,
			`description ~ "test"`,
			`created > -7d`,
			`updated = NOW()`,
			`tags IN ("foo")`,
			`id = 1`,
			`meta.key = "val"`,
		}
		for _, q := range queries {
			t.Run(q, func(t *testing.T) {
				ast, err := Parse(q)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				// EntityType left as zero value (EntityUnspecified)
				if err := Validate(ast); err != nil {
					t.Errorf("expected valid cross-entity query, got: %v", err)
				}
			})
		}
	})

	t.Run("cross_entity_rejects_entity_specific_fields", func(t *testing.T) {
		cases := []struct {
			query    string
			badField string
		}{
			{`contentType = "image/png"`, "contentType"},
			{`noteType = "personal"`, "noteType"},
			{`fileSize > 100`, "fileSize"},
			{`parent.name = "root"`, "parent"},
			{`children IS EMPTY`, "children"},
		}
		for _, tc := range cases {
			t.Run(tc.query, func(t *testing.T) {
				ast, err := Parse(tc.query)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				// EntityType unspecified
				err = Validate(ast)
				if err == nil {
					t.Errorf("expected error for entity-specific field %q in cross-entity query, got nil", tc.badField)
					return
				}
				if !strings.Contains(err.Error(), tc.badField) {
					t.Errorf("expected error to mention %q, got: %v", tc.badField, err)
				}
			})
		}
	})

	t.Run("extract_entity_type", func(t *testing.T) {
		cases := []struct {
			query    string
			expected EntityType
		}{
			{`type = "resource"`, EntityResource},
			{`type = "note"`, EntityNote},
			{`type = "group"`, EntityGroup},
			{`type = "RESOURCE"`, EntityResource},
			{`type = "Note"`, EntityNote},
			{`name = "foo"`, EntityUnspecified},
			{`type = "resource" AND name = "foo"`, EntityResource},
			{`name = "foo" AND type = "group"`, EntityGroup},
		}
		for _, tc := range cases {
			t.Run(tc.query, func(t *testing.T) {
				ast, err := Parse(tc.query)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				got := ExtractEntityType(ast)
				if got != tc.expected {
					t.Errorf("expected entity type %v, got %v", tc.expected, got)
				}
			})
		}
	})

	t.Run("meta_fields_always_valid", func(t *testing.T) {
		entityTypes := []EntityType{EntityResource, EntityNote, EntityGroup, EntityUnspecified}
		queries := []string{
			`meta.rating = 5`,
			`meta.custom_key = "value"`,
			`meta.someKey IS EMPTY`,
		}
		for _, et := range entityTypes {
			for _, q := range queries {
				t.Run(q, func(t *testing.T) {
					ast, err := Parse(q)
					if err != nil {
						t.Fatalf("parse error: %v", err)
					}
					ast.EntityType = et
					if err := Validate(ast); err != nil {
						t.Errorf("meta field should always be valid, got: %v", err)
					}
				})
			}
		}
	})

	t.Run("group_alias_fields", func(t *testing.T) {
		// "group" should be accepted as alias for "groups"
		queries := []string{
			`group = "mygroup"`,
		}
		for _, q := range queries {
			t.Run(q, func(t *testing.T) {
				ast, err := Parse(q)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}
				ast.EntityType = EntityResource
				if err := Validate(ast); err != nil {
					t.Errorf("expected valid, got: %v", err)
				}
			})
		}
	})

	t.Run("nil_where_query", func(t *testing.T) {
		// A query with no WHERE clause should be valid
		ast, err := Parse(`ORDER BY name ASC`)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := Validate(ast); err != nil {
			t.Errorf("expected valid query with no WHERE, got: %v", err)
		}
	})
}

func TestValidatorOwnerTraversal(t *testing.T) {
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
		query       string
		errContains string
	}{
		{`type = group AND owner = "test"`, "unknown"},
		{`type = resource AND owner.owner.name = "test"`, "not valid as intermediate"},
		{`type = resource AND owner.groups.name = "test"`, "not a traversal field"},
		{`type = resource AND owner.parent.meta = "test"`, "not supported"},
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

func TestValidate_GroupByRequiresEntityType(t *testing.T) {
	q, err := Parse(`name ~ "test" GROUP BY name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = Validate(q)
	if err == nil {
		t.Fatal("expected validation error: GROUP BY requires entity type")
	}
	if !strings.Contains(err.Error(), "GROUP BY requires an explicit entity type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_GroupByValidScalarField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidate_GroupByMetaField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY meta.source COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidate_GroupByRelationField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY tags COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidate_GroupByAcceptsTraversal(t *testing.T) {
	tests := []string{
		`type = "resource" GROUP BY owner.name COUNT()`,
		`type = "resource" GROUP BY owner.category COUNT()`,
		`type = "group" GROUP BY parent.name COUNT()`,
		`type = "resource" GROUP BY owner.tags COUNT()`,
		`type = "resource" GROUP BY owner.parent.name COUNT()`,
	}
	for _, input := range tests {
		q, err := Parse(input)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		if err := Validate(q); err != nil {
			t.Errorf("expected valid for %q, got: %v", input, err)
		}
	}
}

func TestValidate_GroupByRejectsInvalidTraversal(t *testing.T) {
	// parent.name is not valid on resources (parent is group-only)
	q, err := Parse(`type = "resource" GROUP BY parent.name COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Fatal("expected error for parent traversal on resource")
	}
}

func TestValidate_GroupByRejectsUnknownField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY fakeField COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Fatal("expected validation error for unknown field")
	}
}

func TestValidate_SumRequiresNumericField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType SUM(name)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = Validate(q)
	if err == nil {
		t.Fatal("expected validation error: SUM on string field")
	}
	if !strings.Contains(err.Error(), "numeric") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_AvgRequiresNumericField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType AVG(description)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Fatal("expected validation error: AVG on string field")
	}
}

func TestValidate_MinAllowsDateTimeField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType MIN(created)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid (MIN on datetime), got: %v", err)
	}
}

func TestValidate_MaxAllowsNumberField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType MAX(fileSize)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid (MAX on number), got: %v", err)
	}
}

func TestValidate_SumAllowsMetaField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType SUM(meta.size)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid (SUM on meta), got: %v", err)
	}
}

func TestValidate_AggregateFieldMustExist(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType SUM(bogus)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err == nil {
		t.Fatal("expected validation error for unknown aggregate field")
	}
}

func TestValidate_GroupByOrderByAggregateKey(t *testing.T) {
	// In aggregated mode, ORDER BY can reference output keys like "count"
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() ORDER BY count DESC`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid ORDER BY on aggregate key, got: %v", err)
	}
}

func TestValidate_GroupByOrderByGroupField(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() ORDER BY contentType ASC`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Errorf("expected valid ORDER BY on group field, got: %v", err)
	}
}
