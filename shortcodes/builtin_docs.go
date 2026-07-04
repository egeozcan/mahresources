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

// BuiltinDocs returns machine-readable documentation for the seven built-in
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
				{Name: "default", Type: "string", Required: false, Description: "Text rendered in place of the empty state when the value is missing. Ignored when hide-empty is set (hide wins)."},
			},
			Examples: []DocExample{
				{Title: "Show a meta value", Code: `[meta path="cooking.time"]`},
				{Title: "Editable field", Code: `[meta path="rating" editable="true"]`},
				{Title: "Fallback for missing value", Code: `[meta path="rating" default="Unrated"]`},
			},
		},
		{
			Name:        "property",
			Syntax:      `[property path="..."]`,
			Description: "Renders a scalar property of the entity itself (a struct field such as Name or Description), HTML-escaped by default. The path may traverse preloaded related structs and slices with dot notation (e.g. Owner.Name, Tags.0.Name); it never triggers DB loads, so related structs render only where the page already loaded them.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Required: true, Description: "Entity field name or dot path, e.g. Name, Owner.Name, or Tags.0.Name."},
				{Name: "raw", Type: "boolean", Default: "false", Description: "When true, output is not HTML-escaped.", Enum: []string{"true", "false"}},
				{Name: "default", Type: "string", Required: false, Description: "Text rendered when the resolved value is empty."},
				{Name: "format", Type: "enum", Required: false, Description: "Post-processes the value. date/datetime/time format time fields; filesize humanizes integer byte counts. Unknown values and non-matching types pass through unchanged.", Enum: []string{"date", "datetime", "time", "filesize"}},
				{Name: "layout", Type: "string", Required: false, Description: "Custom Go time layout for time fields (e.g. Jan 2, 2006). Wins over format."},
			},
			Examples: []DocExample{
				{Title: "Entity name", Code: `[property path="Name"]`},
				{Title: "Related field", Code: `[property path="Owner.Name" default="Unassigned"]`},
				{Title: "Formatted date", Code: `[property path="CreatedAt" format="date"]`},
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
			Description: "Renders its inner content only when a condition holds. Test a meta path, an entity field, or an MRQL result count against one or more operators. Every operator present must pass (use combine=\"any\" for OR). Add numbered-suffix sources (path2, eq2, …) for extra conditions. Supports [elseif …] and [else] dividers inside the block; the first matching branch renders.",
			IsBlock:     BlockRequired,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Required: false, Description: "Meta path to test. Provide one of path / field / mrql."},
				{Name: "field", Type: "string", Required: false, Description: "Entity field name to test. Provide one of path / field / mrql."},
				{Name: "mrql", Type: "string", Required: false, Description: "MRQL query whose result count (or aggregate) is tested. Provide one of path / field / mrql."},
				{Name: "eq", Type: "string", Required: false, Description: "Condition: value equals this."},
				{Name: "neq", Type: "string", Required: false, Description: "Condition: value does not equal this."},
				{Name: "gt", Type: "number", Required: false, Description: "Condition: value is greater than this."},
				{Name: "lt", Type: "number", Required: false, Description: "Condition: value is less than this."},
				{Name: "gte", Type: "number", Required: false, Description: "Condition: value is greater than or equal to this."},
				{Name: "lte", Type: "number", Required: false, Description: "Condition: value is less than or equal to this."},
				{Name: "in", Type: "string", Required: false, Description: `Condition: value equals one of a comma-separated list, e.g. in="a,b,c".`},
				{Name: "contains", Type: "string", Required: false, Description: "Condition: value contains this substring."},
				{Name: "matches", Type: "string", Required: false, Description: "Condition: value matches this Go regular expression. An invalid pattern evaluates to false."},
				{Name: "empty", Type: "boolean", Required: false, Description: `Condition: value is empty or missing. Write empty="true".`, Enum: []string{"true"}},
				{Name: "not-empty", Type: "boolean", Required: false, Description: `Condition: value is present and non-empty. Write not-empty="true".`, Enum: []string{"true"}},
				{Name: "combine", Type: "enum", Default: "all", Description: "How to fold multiple operators and numbered-suffix conditions: all (AND) or any (OR).", Enum: []string{"all", "any"}},
				{Name: "aggregate", Type: "string", Required: false, Description: "Column name to read from an aggregated MRQL result."},
				{Name: "scope", Type: "enum", Default: "entity", Description: "Group subtree for the mrql condition.", Enum: []string{"entity", "parent", "root", "global"}},
				{Name: "limit", Type: "number", Default: "20", Description: "Result limit for the mrql condition."},
				{Name: "buckets", Type: "number", Default: "5", Description: "Bucket count for a grouped mrql condition."},
				{Name: "param-", Type: "string", Wildcard: true, Description: "Binds an MRQL $name placeholder for the mrql condition."},
			},
			Examples: []DocExample{
				{Title: "Show when field set", Code: "[conditional path=\"rating\" not-empty=\"true\"]\n  Rated: [meta path=\"rating\"]\n[/conditional]"},
				{Title: "Numeric range (AND)", Code: "[conditional path=\"score\" gte=\"1\" lte=\"10\"]\n  In range\n[/conditional]"},
				{Title: "elseif chain", Code: "[conditional path=\"tier\" eq=\"gold\"]\n  Gold\n[elseif path=\"tier\" eq=\"silver\"]\n  Silver\n[else]\n  Basic\n[/conditional]"},
				{Title: "With else branch", Code: "[conditional field=\"Name\" eq=\"Draft\"]\n  Draft\n[else]\n  Published\n[/conditional]"},
			},
		},
		{
			Name:        "link",
			Syntax:      `[link to="..."]` + " or " + `[link to="..."]inner[/link]`,
			Description: "Resolves a detail-page URL for the entity or a related target. Inline, it renders just the URL (write <a href=\"[link]\">…</a>); as a block, it wraps its processed inner content in an anchor. When the target cannot be resolved, the inline form renders nothing and the block form renders its inner content without a link.",
			IsBlock:     BlockOptional,
			Attrs: []DocAttr{
				{Name: "to", Type: "enum", Default: "self", Description: "Link target: self (this entity's page), owner (its group), root (top of the ownership chain), or category (its category/type page).", Enum: []string{"self", "owner", "root", "category"}},
			},
			Examples: []DocExample{
				{Title: "Inline URL in an anchor", Code: `<a href="[link]" class="underline">Open</a>`},
				{Title: "Block anchor", Code: "[link to=\"owner\"]Back to group[/link]"},
				{Title: "Category page", Code: `[link to="category"]View type[/link]`},
			},
		},
		{
			Name:        "each",
			Syntax:      `[each path="..."]...[item ...]...[/each]`,
			Description: "Iterates an array value from the entity's Meta JSON, rendering its inner content once per element. Reference the current element with [item] inside the block. A non-array or empty value renders the [else] branch (nothing when there is no [else]). Inner [meta]/[conditional]/[mrql] shortcodes run against the parent entity, not the element.",
			IsBlock:     BlockRequired,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Required: true, Description: "Dot-notation path to an array in the entity Meta, e.g. ingredients."},
				{Name: "limit", Type: "number", Default: "100", Description: "Maximum number of elements to render."},
			},
			Examples: []DocExample{
				{Title: "List a scalar array", Code: "[each path=\"tags\"]\n  <span>[item]</span>\n[/each]"},
				{Title: "Objects with fields and empty state", Code: "[each path=\"ingredients\"]\n  <li>[item path=\"name\"] — [item path=\"qty\" default=\"?\"]</li>\n[else]\n  <p>No ingredients.</p>\n[/each]"},
				{Title: "Numbered list", Code: "[each path=\"steps\"]\n  <p>[item index=\"true\"]. [item]</p>\n[/each]"},
			},
		},
		{
			Name:        "item",
			Syntax:      `[item path="..."]`,
			Description: "Renders the current element inside an [each] block: the element itself (scalar) or a dot-path into it (object). Uses the same format/layout/default helpers as [property] and is HTML-escaped unless raw is set. Outside an [each] block it renders nothing.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Required: false, Description: "Dot-path into the current element when it is an object, e.g. name. Omit to render a scalar element directly."},
				{Name: "index", Type: "boolean", Default: "false", Description: "When true, renders the element's 1-based position instead of its value.", Enum: []string{"true", "false"}},
				{Name: "format", Type: "enum", Required: false, Description: "Post-processes the value (date/datetime/time for time values, filesize for byte counts). Unknown values and non-matching types pass through unchanged.", Enum: []string{"date", "datetime", "time", "filesize"}},
				{Name: "layout", Type: "string", Required: false, Description: "Custom Go time layout for time values (e.g. Jan 2, 2006). Wins over format."},
				{Name: "default", Type: "string", Required: false, Description: "Text rendered when the resolved value is empty."},
				{Name: "raw", Type: "boolean", Default: "false", Description: "When true, output is not HTML-escaped.", Enum: []string{"true", "false"}},
			},
			Examples: []DocExample{
				{Title: "Scalar element", Code: `[item]`},
				{Title: "Object field with fallback", Code: `[item path="qty" default="?"]`},
				{Title: "1-based position", Code: `[item index="true"]`},
			},
		},
		{
			Name:        "partial",
			Syntax:      `[partial name="..."]`,
			Description: "Expands a reusable template partial by name, rendered with the current entity context so the partial's own shortcodes resolve against the carrier entity. An unknown name renders an HTML comment. Partials are managed under Template Partials.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "name", Type: "string", Required: true, Description: "Kebab-case name of the partial to expand, e.g. status-badge."},
			},
			Examples: []DocExample{
				{Title: "Include a partial", Code: `[partial name="status-badge"]`},
			},
		},
	}
}
