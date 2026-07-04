package shortcodes

// BlockCapability describes whether a shortcode may or must use the paired
// block form ([name]...[/name]) versus the self-closing inline form ([name ...]).
type BlockCapability string

const (
	// BlockNo — inline-only. A closing tag is an error (e.g. [meta], [property]).
	BlockNo BlockCapability = "no"
	// BlockOptional — may be inline or block (e.g. [mrql]).
	BlockOptional BlockCapability = "optional"
	// BlockRequired — must be a block; the inline form renders nothing
	// (e.g. [conditional]).
	BlockRequired BlockCapability = "required"
)

// DocAttr describes a single shortcode attribute for documentation, linting,
// and autocomplete.
type DocAttr struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description"`
	// Enum lists the closed set of valid values, when the attribute accepts a
	// fixed vocabulary (used for attr-value autocomplete). Empty means open.
	Enum []string `json:"enum,omitempty"`
	// Wildcard marks a prefix-matched attribute family, e.g. Name "param-"
	// matches "param-tag", "param-since", etc.
	Wildcard bool `json:"wildcard,omitempty"`
}

// DocExample is a usage example for a shortcode.
type DocExample struct {
	Title string `json:"title,omitempty"`
	Code  string `json:"code"`
	Notes string `json:"notes,omitempty"`
}

// BuiltinDoc documents one built-in shortcode (meta, property, mrql, conditional).
type BuiltinDoc struct {
	Name        string          `json:"name"`
	Syntax      string          `json:"syntax"`
	Description string          `json:"description"`
	IsBlock     BlockCapability `json:"isBlock"`
	Attrs       []DocAttr       `json:"attrs"`
	Examples    []DocExample    `json:"examples"`
}

// BuiltinDocs returns machine-readable documentation for the four built-in
// shortcodes. It is the single source of truth for both the docs endpoint and
// the linter's KnownShortcodes, so lint rules stay in sync with the docs.
func BuiltinDocs() []BuiltinDoc {
	return []BuiltinDoc{
		{
			Name:        "meta",
			Syntax:      `[meta path="..."]`,
			Description: "Renders a value from the entity's Meta JSON at a dot-notation path. Optionally editable inline.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Required: true, Description: "Dot-notation path into the entity Meta, e.g. cooking.time."},
				{Name: "editable", Type: "boolean", Default: "false", Description: "When true, renders an inline editor for the value.", Enum: []string{"true", "false"}},
				{Name: "hide-empty", Type: "boolean", Default: "false", Description: "When true, renders nothing if the value is empty.", Enum: []string{"true", "false"}},
			},
			Examples: []DocExample{
				{Title: "Show a meta value", Code: `[meta path="cooking.time"]`},
				{Title: "Editable field", Code: `[meta path="rating" editable="true"]`},
			},
		},
		{
			Name:        "property",
			Syntax:      `[property path="..."]`,
			Description: "Renders a scalar property of the entity itself (a struct field such as Name or Description), HTML-escaped by default.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Required: true, Description: "Entity field name, e.g. Name or Description."},
				{Name: "raw", Type: "boolean", Default: "false", Description: "When true, output is not HTML-escaped.", Enum: []string{"true", "false"}},
			},
			Examples: []DocExample{
				{Title: "Entity name", Code: `[property path="Name"]`},
			},
		},
		{
			Name:        "mrql",
			Syntax:      `[mrql query="..."]`,
			Description: "Runs an MRQL query and renders the results. Provide either an inline query or a saved query name. Optionally wrap a custom item template as a block.",
			IsBlock:     BlockOptional,
			Attrs: []DocAttr{
				{Name: "query", Type: "string", Required: false, Description: "Inline MRQL expression. Required unless saved is set."},
				{Name: "saved", Type: "string", Required: false, Description: "Name of a saved MRQL query. Required unless query is set."},
				{Name: "format", Type: "enum", Default: "auto", Description: "Result layout. Auto-resolves to custom templates when available.", Enum: []string{"table", "list", "compact", "custom"}},
				{Name: "limit", Type: "number", Default: "20", Description: "Maximum number of results."},
				{Name: "buckets", Type: "number", Default: "5", Description: "Bucket count for grouped (GROUP BY) results."},
				{Name: "scope", Type: "enum", Default: "entity", Description: "Group subtree to scope results to.", Enum: []string{"entity", "parent", "root", "global"}},
				{Name: "param-", Type: "string", Wildcard: true, Description: "Binds an MRQL $name placeholder, e.g. param-tag=\"x\" fills $tag."},
			},
			Examples: []DocExample{
				{Title: "Inline query", Code: `[mrql query="resources where tag = 'draft'" format="list"]`},
				{Title: "Saved query with params", Code: `[mrql saved="recent" param-since="-7d"]`},
				{Title: "Custom item template (block)", Code: "[mrql query=\"resources limit 3\"]\n  <div>[property path=\"Name\"]</div>\n[/mrql]"},
			},
		},
		{
			Name:        "conditional",
			Syntax:      `[conditional path="..." eq="..."]...[/conditional]`,
			Description: "Renders its inner content only when a condition holds. Test a meta path, an entity field, or an MRQL result count against one operator. Supports an [else] divider inside the block.",
			IsBlock:     BlockRequired,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Required: false, Description: "Meta path to test. Provide one of path / field / mrql."},
				{Name: "field", Type: "string", Required: false, Description: "Entity field name to test. Provide one of path / field / mrql."},
				{Name: "mrql", Type: "string", Required: false, Description: "MRQL query whose result count (or aggregate) is tested. Provide one of path / field / mrql."},
				{Name: "eq", Type: "string", Required: false, Description: "Condition: value equals this."},
				{Name: "neq", Type: "string", Required: false, Description: "Condition: value does not equal this."},
				{Name: "gt", Type: "number", Required: false, Description: "Condition: value is greater than this."},
				{Name: "lt", Type: "number", Required: false, Description: "Condition: value is less than this."},
				{Name: "contains", Type: "string", Required: false, Description: "Condition: value contains this substring."},
				{Name: "empty", Type: "boolean", Required: false, Description: `Condition: value is empty or missing. Write empty="true".`, Enum: []string{"true"}},
				{Name: "not-empty", Type: "boolean", Required: false, Description: `Condition: value is present and non-empty. Write not-empty="true".`, Enum: []string{"true"}},
				{Name: "aggregate", Type: "string", Required: false, Description: "Column name to read from an aggregated MRQL result."},
				{Name: "scope", Type: "enum", Default: "entity", Description: "Group subtree for the mrql condition.", Enum: []string{"entity", "parent", "root", "global"}},
				{Name: "limit", Type: "number", Default: "20", Description: "Result limit for the mrql condition."},
				{Name: "buckets", Type: "number", Default: "5", Description: "Bucket count for a grouped mrql condition."},
				{Name: "param-", Type: "string", Wildcard: true, Description: "Binds an MRQL $name placeholder for the mrql condition."},
			},
			Examples: []DocExample{
				{Title: "Show when field set", Code: "[conditional path=\"rating\" not-empty=\"true\"]\n  Rated: [meta path=\"rating\"]\n[/conditional]"},
				{Title: "With else branch", Code: "[conditional field=\"Name\" eq=\"Draft\"]\n  Draft\n[else]\n  Published\n[/conditional]"},
			},
		},
	}
}
