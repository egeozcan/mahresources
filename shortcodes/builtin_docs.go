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
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
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

// BuiltinDoc documents one built-in shortcode (meta, datetime, pills,
// property, calc, mrql, conditional, link, each, item, partial, lazy, details,
// reload, using, inspect, adjacent, similar, calendar, comment).
type BuiltinDoc struct {
	Name        string          `json:"name"`
	Syntax      string          `json:"syntax"`
	Description string          `json:"description"`
	IsBlock     BlockCapability `json:"isBlock"`
	Attrs       []DocAttr       `json:"attrs"`
	Examples    []DocExample    `json:"examples"`
}

// BuiltinDocs returns machine-readable documentation for the built-in
// shortcodes. It is the single source of truth for both the docs endpoint and
// the linter's KnownShortcodes, so lint rules stay in sync with the docs.
func BuiltinDocs() []BuiltinDoc {
	return []BuiltinDoc{
		{
			Name:        "meta",
			Syntax:      `[meta path="..."]`,
			Description: "Renders a value from the entity's Meta JSON at a dot-notation path. Optionally editable inline. By default it renders a <meta-shortcode> element, which cannot go inside an HTML attribute — use inline=\"true\" there to emit the bare value instead.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Required: true, Description: "Dot-notation path into the entity Meta, e.g. cooking.time."},
				{Name: "inline", Type: "boolean", Default: "false", Description: "When true, renders the bare value instead of the element — the only form usable inside an HTML attribute. Output is HTML-escaped unless raw is set, which keeps a value containing a quote inside a QUOTED attribute and nothing more — it does not make a javascript: URL safe in href, survive an on* handler, or contain a value in an unquoted attribute. The linter warns on those four shapes. format/layout/default/hide-empty apply as they do on [item]; editable is ignored, there being no widget to edit. What is emitted is the stored value, not the element's rendering of it: the display rules the element applies do not apply here, so a labeled enum renders its stored value and not its label, a boolean renders true/false and not Yes/No, and an x-display renderer does not run. That is what the mode is for, since the reason to reach for it is an attribute, a URL or a script: data-status=\"[meta path='status' inline='true']\" has to carry the key those were written against, not a label an operator can rename in the schema. Use the element form where you want the label.", Enum: []string{"true", "false"}},
				{Name: "editable", Type: "boolean", Default: "false", Description: "When true, renders an inline editor for the value. Ignored when inline is set.", Enum: []string{"true", "false"}},
				{Name: "raw", Type: "boolean", Default: "false", Description: "With inline, skips HTML escaping — same meaning as on [property] and [item]. Do not use inside an attribute: an unescaped quote closes it early. No effect without inline.", Enum: []string{"true", "false"}},
				{Name: "format", Type: "enum", Required: false, Description: "With inline, post-processes the value like [property]: date/datetime/time for timestamps (including RFC3339 strings, which is how a time lands in Meta), filesize for byte counts. No effect without inline.", Enum: []string{"date", "datetime", "time", "filesize"}},
				{Name: "layout", Type: "string", Required: false, Description: "With inline, a custom Go time layout (e.g. Jan 2, 2006). Wins over format. No effect without inline."},
				{Name: "hide-empty", Type: "boolean", Default: "false", Description: "When true, renders nothing if the value is empty.", Enum: []string{"true", "false"}},
				{Name: "default", Type: "string", Required: false, Description: "Text rendered in place of the empty state when the value is missing. Ignored when hide-empty is set (hide wins)."},
			},
			Examples: []DocExample{
				{Title: "Show a meta value", Code: `[meta path="cooking.time"]`},
				{Title: "Editable field", Code: `[meta path="rating" editable="true"]`},
				{Title: "Fallback for missing value", Code: `[meta path="rating" default="Unrated"]`},
				{Title: "Inside an HTML attribute", Code: `<a href="/x/[meta path='slug' inline='true']" title="[meta path='blurb' inline='true']">Open</a>`},
				{Title: "Formatted inline value", Code: `[meta path="published" inline="true" format="date"]`},
			},
		},
		{
			Name:        "datetime",
			Syntax:      `[datetime path="..."]`,
			Description: "Displays and optionally edits a date/time Meta field. Preserves its stored date/time format and timezone offset; invalid values display unchanged and the editor starts from the field's schema default (or empty).",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Required: true, Description: "Dot-notation path into the entity Meta."},
				{Name: "editable", Type: "boolean", Default: "false", Description: "Enable the date/time picker with Save and Cancel.", Enum: []string{"true", "false"}},
				{Name: "layout", Type: "string", Description: "Custom display layout using Go tokens, e.g. January 2, 2006 at 15:04. Defaults to the stored format."},
				{Name: "input-layout", Type: "string", Description: "Custom storage layout for parsing and saving, e.g. 02/01/2006 15:04. Otherwise detects ISO dates, times and timestamps, then uses the schema format."},
				{Name: "hide-empty", Type: "boolean", Default: "false", Description: "Hide empty values outside edit mode.", Enum: []string{"true", "false"}},
				{Name: "default", Type: "string", Description: "Display text for empty values. Editor defaults come from the field schema."},
			},
			Examples: []DocExample{
				{Title: "Display a date/time", Code: `[datetime path="event.start"]`},
				{Title: "Edit a date/time", Code: `[datetime path="event.start" editable="true"]`},
				{Title: "Custom display", Code: `[datetime path="event.start" layout="January 2, 2006 at 15:04"]`},
				{Title: "Custom storage", Code: `[datetime path="deadline" editable="true" input-layout="02/01/2006"]`},
			},
		},
		{
			Name:        "pills",
			Syntax:      `[pills path="..." editable="true"]`,
			Description: "Displays a Meta field as a pill selector. Choices come from the field's enum or labeled oneOf schema unless options supplies a manual JSON array. The selected entry uses its x-color as a background tint and dark text color, defaulting to purple. Selecting a choice saves immediately when editable is true. Values retain their JSON types; public shares are read-only.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Required: true, Description: "Dot-notation path into the entity Meta."},
				{Name: "editable", Type: "boolean", Default: "false", Description: "Save immediately when a pill is selected.", Enum: []string{"true", "false"}},
				{Name: "options", Type: "string", Description: "Manual JSON array of scalar values or {value, label} objects. Overrides schema options. Values may be strings, numbers, booleans or null."},
				{Name: "hide-empty", Type: "boolean", Default: "false", Description: "Hide the selector when the stored value is empty. Leave false to allow setting an empty field.", Enum: []string{"true", "false"}},
			},
			Examples: []DocExample{
				{Title: "Schema enum", Code: `[pills path="priority" editable="true"]`},
				{Title: "Manual choices", Code: `[pills path="priority" editable="true" options='["Low","Medium","High"]']`},
				{Title: "Labels and typed values", Code: `[pills path="priority" editable="true" options='[{"value":1,"label":"Low"},{"value":2,"label":"Medium"},{"value":3,"label":"High"}]']`},
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
			Name:        "excerpt",
			Syntax:      `[excerpt words="30"]`,
			Description: "Renders a short plain-text preview of the entity's Description, another string field (field=) or a Meta string (path=). Markdown, HTML, mentions (their names are kept), template comments and shortcodes are stripped without running anything; the text becomes one line, is cut to words= words (and at most 12 characters per word) with an ellipsis, and the result, default= included, is always HTML-escaped. A source longer than 16 KiB is read only to that point, so its excerpt always ends with an ellipsis. Without more= it emits a bare value, usable inside a quoted attribute; with more= a cut text is followed by a link to the entity's page, so do not use more= inside an attribute. It never loads data: use it inside [using] or an [mrql] block to preview a related entity.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "words", Type: "number", Default: "30", Description: "Maximum number of words, a whole number from 1 to 200. Anything else falls back to 30; above 200 is clamped to 200."},
				{Name: "field", Type: "string", Default: "Description", Description: "One string field of this entity, e.g. Name or OriginalName. No dot paths: preview a related entity through [using]. Credential fields stay empty unless the reader may see them, as with [property]. Cannot be combined with path."},
				{Name: "path", Type: "string", Description: "Dot-notation path to a string in the entity Meta, e.g. recipe.intro. Any other JSON type renders the default. Cannot be combined with field."},
				{Name: "more", Type: "string", Description: "Visible label of a link to the entity's detail page, shown only when the text was cut. Its accessible name adds the entity's name (\"Read more: Sunset\"). Left out inside a [reload] button face and on the public share page, which serves no detail pages."},
				{Name: "default", Type: "string", Description: "Text rendered, HTML-escaped, when there is no text to preview, including a source that is empty once stripped."},
			},
			Examples: []DocExample{
				{Title: "Description preview", Code: `[excerpt]`},
				{Title: "With a link", Code: `[excerpt words="20" more="Read more"]`},
				{Title: "A Meta string", Code: `[excerpt path="recipe.intro" words="15" default="No intro"]`},
				{Title: "Related notes", Code: `[mrql query="type = note ORDER BY updated DESC" limit="5"]<p><b>[property path="Name"]</b> [excerpt words="12"]</p>[/mrql]`},
				{Title: "Inside an attribute", Code: `<span title="[excerpt words='25']">[property path="Name"]</span>`, Notes: "Only without more=, which adds a link."},
			},
		},
		{
			Name:        "calc",
			Syntax:      `[calc expr="..."]`,
			Description: "Computes a number from this entity's Meta (meta.<path>), its numeric fields (property.<Path>) and, at the top level of an [each] block, the current element (item or item.<path>), with + - * / % and parentheses and the functions min, max, abs, round(x[, digits]) and coalesce(a, b, …). It renders the bare number, so it works in text, table cells and attributes such as <meter value=\"…\">. A Meta string counts only when it is wholly a decimal number of at most 64 bytes; a missing or non-numeric operand, division by zero or overflow gives no value, which renders the default unless coalesce catches it. A syntax error, an unknown name or a bad decimals/format renders an error marker. It never runs a query. Only Meta keys of ASCII letters, digits and _ that do not start with a digit can be referenced. Limits: 512 bytes, 128 tokens, 32 levels of nesting, 32 references.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "expr", Type: "string", Required: true, Description: "The expression, e.g. meta.qty * meta.unit_price. Names are case-sensitive; numbers are plain decimals with no exponent. An item operand has no value outside [each] or inside another block within it."},
				{Name: "decimals", Type: "number", Description: "A whole number from 0 to 10: round half away from zero to exactly that many fraction digits, keeping trailing zeros (10 gives 10.00). Unset, the result is rounded to 10 digits and trailing zeros are dropped."},
				{Name: "format", Type: "enum", Default: "number", Description: "number is a plain decimal (1234567.89); grouped puts a comma between every three integer digits (1,234,567.89, not locale-aware); percent multiplies by 100 and appends % (0.256 gives 25.6%).", Enum: []string{"number", "grouped", "percent"}},
				{Name: "default", Type: "string", Description: "Text rendered, HTML-escaped, when the expression has no numeric result. Empty by default. Prefer words such as \"not set\" over a lone dash where the reader needs to know why the value is missing."},
			},
			Examples: []DocExample{
				{Title: "Line total", Code: `[calc expr="meta.qty * meta.unit_price" decimals="2"]`},
				{Title: "Optional discount", Code: `[calc expr="meta.qty * meta.unit_price * (1 - coalesce(meta.discount, 0))" decimals="2" default="not set"]`},
				{Title: "Inside [each]", Code: "<table>\n  <caption>Invoice lines</caption>\n  <tr><th scope=\"col\">Item</th><th scope=\"col\">Total</th></tr>\n  [each path=\"lines\"]\n  <tr><td>[item path=\"name\"]</td><td>[calc expr=\"item.qty * item.price\" decimals=\"2\"]</td></tr>\n  [/each]\n</table>"},
				{Title: "Percent in a meter", Code: `<meter min="0" max="1" value="[calc expr='meta.spent / meta.budget' default='0']" aria-label="Budget used">[calc expr='meta.spent / meta.budget' format='percent' decimals='0' default='unknown']</meter>`, Notes: "The aria-label names the meter and the fallback text carries the value, so the number is not conveyed by the bar alone."},
			},
		},
		{
			Name:        "mrql",
			Syntax:      `[mrql query="..."]`,
			Description: "Runs an MRQL query and renders the results. Provide either an inline query or a saved query name. Set value= for a single inline scalar, or wrap a block body for a custom item template with optional [header]/[footer]/[else] slots. [else] is a divider with no closing tag: write [else]…[/mrql], never [else]…[/else].",
			IsBlock:     BlockOptional,
			Attrs: []DocAttr{
				{Name: "query", Type: "string", Required: false, Description: "Inline MRQL expression. Required unless saved is set."},
				{Name: "saved", Type: "string", Required: false, Description: "Name of a saved MRQL query. Required unless query is set."},
				{Name: "value", Type: "string", Required: false, Description: "Inline scalar mode: renders a single escaped value with no wrapper. Use \"count\" for the result count (returned rows, capped by limit) or a column name from an aggregated result. Conflicts with a block body."},
				{Name: "format", Type: "enum", Default: "auto", Description: "Result layout, or (with value=) the scalar's format like [property]: date/datetime/time/filesize. Auto-resolves to custom templates when available.", Enum: []string{"table", "list", "compact", "custom"}},
				{Name: "layout", Type: "string", Required: false, Description: "Custom Go time layout for a value= time scalar (e.g. Jan 2, 2006). Wins over format."},
				{Name: "limit", Type: "number", Default: "20", Description: "Maximum number of results."},
				{Name: "buckets", Type: "number", Default: "5", Description: "Bucket count for grouped (GROUP BY) results."},
				{Name: "scope", Type: "enum", Default: "entity", Description: "Group subtree to scope results to: entity (the current entity's group), parent, root, global, or a numeric group ID.", Enum: []string{"entity", "parent", "root", "global"}},
				{Name: "link-all", Type: "boolean", Default: "false", Description: "Appends a default \"View all →\" link (after results, before a custom [footer]) to the /mrql page for this query. In [header]/[footer]/[else] slots, the {link-all} placeholder expands to the bare URL.", Enum: []string{"true", "false"}},
				{Name: "param-", Type: "string", Wildcard: true, Description: "Binds an MRQL $name placeholder, e.g. param-tag=\"x\" fills $tag."},
			},
			Examples: []DocExample{
				{Title: "Inline query", Code: `[mrql query='type = resource AND tags = "draft"' format="list"]`},
				{Title: "Inline count value", Code: `[mrql query='type = resource' value="count"]`, Notes: "Counts returned rows, capped by limit. For a true total use an aggregated count() query or the {total} slot placeholder."},
				{Title: "Saved query with params", Code: `[mrql saved="recent" param-since="-7d"]`},
				{Title: "Custom item template (block)", Code: "[mrql query='type = resource LIMIT 3']\n  <div>[property path=\"Name\"]</div>\n[/mrql]"},
				{Title: "Header / footer / empty slots", Code: "[mrql query='type = note AND tags = \"todo\"' link-all=\"true\"]\n  [header]<h4>TODO ({count} of {total})</h4>[/header]\n  <li>[property path=\"Name\"]</li>\n[else]\n  <p>Nothing to do 🎉</p>\n[/mrql]", Notes: "[header]/[footer] render once around results; [else] is the whole empty-state output. {count} = returned rows, {total} = true total (its presence triggers the count query)."},
			},
		},
		{
			Name:        "conditional",
			Syntax:      `[conditional path="..." eq="..."]...[/conditional]`,
			Description: "Renders its inner content only when a condition holds. Test a meta path, an entity field, or an MRQL result count against one or more operators. Every operator present must pass (use combine=\"any\" for OR). Add numbered-suffix sources (path2, eq2, …) for extra conditions. Supports [elseif …] and [else] dividers inside the block; the first matching branch renders. [elseif] and [else] are dividers with no closing tag: never write [/elseif] or [/else], they render as literal text.",
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
				{Name: "matches", Type: "string", Required: false, Description: "Condition: value matches this Go regular expression. An invalid pattern, or one past the regular expression allowance, evaluates to false."},
				{Name: "empty", Type: "boolean", Required: false, Description: `Condition: value is empty or missing. Write empty="true".`, Enum: []string{"true"}},
				{Name: "not-empty", Type: "boolean", Required: false, Description: `Condition: value is present and non-empty. Write not-empty="true".`, Enum: []string{"true"}},
				{Name: "combine", Type: "enum", Default: "all", Description: "How to fold multiple operators and numbered-suffix conditions: all (AND) or any (OR).", Enum: []string{"all", "any"}},
				{Name: "aggregate", Type: "string", Required: false, Description: "Column name to read from an aggregated MRQL result."},
				{Name: "scope", Type: "enum", Default: "entity", Description: "Group subtree for the mrql condition: entity (the current entity's group), parent, root, global, or a numeric group ID.", Enum: []string{"entity", "parent", "root", "global"}},
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
				{Title: "Inline URL in an anchor", Code: `<a href="[link]">Open</a>`},
				{Title: "Block anchor", Code: "[link to=\"owner\"]Back to group[/link]"},
				{Title: "Category page", Code: `[link to="category"]View type[/link]`},
			},
		},
		{
			Name:        "each",
			Syntax:      `[each path="..."]...[item ...]...[/each]`,
			Description: "Iterates an array value from the entity's Meta JSON, rendering its inner content once per element. Reference the current element with [item] inside the block. A non-array or empty value renders the [else] branch (nothing when there is no [else]); [else] is a divider with no closing tag, so end the block with [/each], never [/else]. Inner [meta]/[conditional]/[mrql] shortcodes run against the parent entity, not the element.",
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
		{
			Name:        "lazy",
			Syntax:      `[lazy]...[/lazy]`,
			Description: "Defers its inner content: on a display page the body is rendered on the server only when the block scrolls into view, keeping expensive shortcodes (especially [mrql]) out of the initial page render. Ideal for per-card slots on long lists. Off the display page (share pages, live preview) the body renders inline. The body renders against the entity, so an [item] from an enclosing [each] is not available inside it.",
			IsBlock:     BlockRequired,
			Attrs:       []DocAttr{},
			Examples: []DocExample{
				{Title: "Defer an expensive query", Code: "[lazy]\n  [mrql query='type = \"resource\"' format=\"list\"]\n[/lazy]"},
			},
		},
		{
			Name:        "details",
			Syntax:      `[details summary="..."]...[/details]`,
			Description: "A disclosure (like the HTML details/summary element) whose inner content is rendered on the server only the first time it is opened. Keyboard- and screen-reader-accessible. Off the display page (share pages, live preview) it renders as a plain collapsible <details> with the body inline. The body renders against the entity, so an [item] from an enclosing [each] is not available inside it.",
			IsBlock:     BlockRequired,
			Attrs: []DocAttr{
				{Name: "summary", Type: "string", Default: "Details", Description: "The always-visible label that toggles the disclosure."},
				{Name: "open", Type: "boolean", Default: "false", Description: "When true, the disclosure starts expanded (and loads its content immediately).", Enum: []string{"true", "false"}},
			},
			Examples: []DocExample{
				{Title: "Collapsible section", Code: "[details summary=\"Nutrition\"]\n  [meta path=\"calories\"] kcal\n[/details]"},
				{Title: "Start expanded", Code: "[details summary=\"Notes\" open=\"true\"]\n  [property path=\"Description\"]\n[/details]"},
			},
		},
		{
			Name:        "tabs",
			Syntax:      `[tabs label="..."][tab label="..."]...[/tab][/tabs]`,
			Description: "Alternative sections in one constant-height area, as an accessible tab set. Each [tab label=\"…\" lazy=\"true|false\"]…[/tab] directly inside the block is one tab; the first is selected when the page loads. label is the tab's visible text and accessible name (blank shows \"Tab N\"). An eager tab renders with the page, hidden or not, so put lazy=\"true\" on every tab after the first that holds [mrql], a gallery or a plugin view: a lazy tab is a [lazy] block, rendered on the server only when the tab is first shown (a lazy first tab loads right after the page). A lazy tab's body renders against the entity, so an [item] from an enclosing [each] is not available inside it. Keyboard: Left and Right arrows (wrapping), Home and End move between tabs, and Enter or Space opens the focused one; Tab moves into the open panel. Without JavaScript every tab shows, stacked under its label. A [tab] inside a nested block such as [conditional] is not a tab of this block, so tabs cannot be added conditionally: put the [conditional] inside a tab. A [tabs] inside a tab works. Off the display page (share pages, live preview, list headers) lazy tabs render inline.",
			IsBlock:     BlockRequired,
			Attrs: []DocAttr{
				{Name: "label", Type: "string", Description: "The accessible name of the tab list. Set it whenever a page has more than one [tabs]."},
			},
			Examples: []DocExample{
				{Title: "Two views", Code: "[tabs label=\"Recipe\"]\n  [tab label=\"Ingredients\"]<ul>[each path=\"ingredients\"]<li>[item]</li>[/each]</ul>[/tab]\n  [tab label=\"Steps\"][each path=\"steps\"]<p>[item]</p>[/each][/tab]\n[/tabs]"},
				{Title: "Lazy query tabs", Code: "[tabs label=\"Project views\"]\n  [tab label=\"Overview\"][property path=\"Description\"][/tab]\n  [tab label=\"Open tasks\" lazy=\"true\"][mrql query='type = note AND tags = \"todo\"' format=\"list\"][/tab]\n  [tab label=\"Files\" lazy=\"true\"][mrql query='type = resource' format=\"list\"][/tab]\n[/tabs]", Notes: "The two query tabs run nothing until a reader opens them."},
			},
		},
		{
			Name:        "reload",
			Syntax:      `[reload]`,
			Description: "A button that re-renders content on the server without navigating. It reloads the innermost [lazy], [details] or [autorefresh] block it sits inside. Outside those it reloads the whole custom-content slot it was written in (header, sidebar, summary or avatar). Where neither can be re-rendered on its own (share pages, live preview) it reloads the page. Self-closing it renders a circular-arrow icon button; wrap a block body to use that content as the button face instead. Keep the body to text or icons: a button may not contain links or other interactive elements.",
			IsBlock:     BlockOptional,
			Attrs: []DocAttr{
				{Name: "label", Type: "string", Default: "Reload", Description: "Accessible name for the button. The default applies whenever the button face carries no text of its own (the icon, or a body that is only an image or an svg), and that case also gets it as a tooltip. When the button has visible text, keep that text inside this value."},
			},
			Examples: []DocExample{
				{Title: "Icon button", Code: `[reload]`},
				{Title: "Refresh one deferred block", Code: "[lazy]\n  [reload label=\"Refresh open tasks\"]\n  [mrql query='type = \"note\"' format=\"list\"]\n[/lazy]"},
				{Title: "Labelled button", Code: `[reload]Refresh[/reload]`},
			},
		},
		{
			Name:        "autorefresh",
			Syntax:      `[autorefresh]...[/autorefresh]`,
			Description: "Re-renders its body on the server whenever an inline editor saves, without reloading the page. It refreshes after a [meta], [pills] or [datetime] with editable=\"true\" saves this entity anywhere on the page, or saves any entity from inside the block (an editor in an [mrql] result list, say). An [action] button for this entity, or inside the block, refreshes it the same way when its run succeeds. Edits made within a quarter of a second of each other cause one refresh. Wrap only what depends on the edited values: everything in the body is replaced, so an editor inside it closes and loses unsaved input when another edit lands. A [reload] inside it reloads this block. On share pages and in the live preview the body renders once and does not refresh. An [item] from an enclosing [each] is not available inside it.",
			IsBlock:     BlockRequired,
			Attrs: []DocAttr{
				{Name: "on", Type: "enum", Default: "edit", Description: "What triggers a refresh. edit is the only trigger.", Enum: []string{"edit"}},
			},
			Examples: []DocExample{
				{Title: "A badge that follows an editable field", Code: "[pills path=\"status\" editable=\"true\"]\n[autorefresh]\n  [conditional path=\"status\" eq=\"done\"]<span class=\"badge\">Done</span>[/conditional]\n[/autorefresh]"},
				{Title: "Bare values in attributes", Code: "[autorefresh]\n  <span data-status=\"[meta path='status' inline='true']\">[meta path=\"status\" inline=\"true\"]</span>\n[/autorefresh]", Notes: "A value written with inline=\"true\" is plain text, so it never updates on its own after an edit."},
			},
		},
		{
			Name:        "action",
			Syntax:      `[action plugin="..." name="..."]`,
			Description: "A button that runs a registered plugin action (mah.action) on this entity, with the permissions of whoever clicks it, through the same endpoint as the actions menu. It shows the action's confirmation first when there is one, reports progress and the result next to the button and to screen readers, and when the run succeeds it refreshes the [autorefresh] blocks for this entity and the one it sits in. The page is never reloaded. Any registered action whose entity type matches works, whatever its placement; one that needs a bulk selection does not. It renders nothing (an HTML comment) for viewers who cannot run the action: guests and other read-only accounts, group-limited accounts the plugin is not opened to, an entity the action's filters exclude, and a missing or disabled plugin or action. It also renders nothing in a Description or note text block when login is on, on share pages, in the live preview, in hover cards and in MRQL results rendered by the API. Wrap the content the action changes in [autorefresh], with the button first in that block or outside it.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "plugin", Type: "string", Required: true, Description: "The plugin's name, as in [plugin:<name>:…]."},
				{Name: "name", Type: "string", Required: true, Description: "The action's id, as registered with mah.action({ id = … }) (ids are lowercase)."},
				{Name: "label", Type: "string", Description: "The button text, which is also its accessible name. Blank means the action's own label."},
				{Name: "confirm", Type: "string", Description: "Confirmation question asked before the run, replacing the action's own. Blank keeps the action's confirmation; a template cannot remove one."},
				{Name: "param-", Type: "string", Wildcard: true, Description: "Sets an action parameter, e.g. param-scale=\"2\". The parameter must be declared by the action; number takes a number, boolean true or false, select one of its options, and entity_ref and info parameters cannot be set. Unset parameters take the action's defaults, and an entity_ref that defaults to the trigger gets this entity. Values are literal: shortcodes inside them are not expanded."},
			},
			Examples: []DocExample{
				{Title: "A button beside the value it changes", Code: "[autorefresh]\n  [action plugin=\"project-management\" name=\"pm-set-status\" label=\"Mark done\" param-status=\"done\"]\n  Status: [meta path=\"status\"]\n[/autorefresh]", Notes: "For the PM Task note type of the bundled project-management plugin. The button comes first in the block, so focus returns to it after the refresh."},
				{Title: "With a parameter and a confirmation", Code: `[action plugin="fal-ai" name="upscale" label="Upscale (Topaz)" confirm="Upscale this image on fal.ai? This is billed." param-model="topaz"]`, Notes: "The bundled fal-ai plugin's upscale action, which applies to images only; it runs as a Job and the button reports when it finishes."},
				{Title: "One button per result row", Code: "[mrql query='type = note AND tags = \"triage\"']\n  <div>[property path=\"Name\"] [action plugin=\"project-management\" name=\"pm-set-status\" label=\"Start\" param-status=\"in_progress\"]</div>\n[/mrql]", Notes: "Each row's button runs the action on that row's note; rows the action's filters exclude get no button."},
			},
		},
		{
			Name:        "using",
			Syntax:      `[using type="..." id="..."]...[else]...[/using]`,
			Description: "Renders its body with another group, resource or note as the entity, so every shortcode inside ([property], [meta], [conditional], [each], [link], [mrql scope=…]) reads that entity instead of this one. Name the target with exactly one of id, id-path, id-prop, rel or query. When there is no target, or it is one you cannot see, the [else] branch renders against this entity (nothing when there is no [else]); [else] is a divider with no closing tag, so end the block with [/using]. Each block runs one query per render. An [item] from an enclosing [each] is not available inside it, and the target's category CustomCSS is not added to the page.",
			IsBlock:     BlockRequired,
			Attrs: []DocAttr{
				{Name: "type", Type: "enum", Required: false, Description: "Entity type of the target. Required with id, id-path and id-prop; not used with rel (always a group) or query (the query names its type).", Enum: []string{"group", "resource", "note"}},
				{Name: "id", Type: "number", Required: false, Description: "The target's ID, a positive whole number."},
				{Name: "id-path", Type: "string", Required: false, Description: "Dot-path into this entity's Meta holding the target's ID (a number or a numeric string), e.g. links.client."},
				{Name: "id-prop", Type: "string", Required: false, Description: "Field of this entity holding the target's ID, e.g. OwnerId."},
				{Name: "rel", Type: "enum", Required: false, Description: "A group related to this entity: owner (the group that owns it) or root (the top of its ownership chain). Same meaning as [link to=].", Enum: []string{"owner", "root"}},
				{Name: "query", Type: "string", Required: false, Description: "MRQL query whose first result is the target. Name the entity type, and add ORDER BY so the first result is a stable one. GROUP BY is not allowed."},
				{Name: "scope", Type: "enum", Default: "entity", Description: "With query only: group subtree to scope the query to, as on [mrql]: entity, parent, root, global, or a numeric group ID. Lookups by ID ignore it.", Enum: []string{"entity", "parent", "root", "global"}},
				{Name: "param-", Type: "string", Wildcard: true, Description: "With query only: binds an MRQL $name placeholder, e.g. param-name=\"Index\" binds $name."},
			},
			Examples: []DocExample{
				{Title: "The owning group", Code: "[using rel=\"owner\"]\n  <h3>[link][property path=\"Name\"][/link]</h3>\n  Status: [meta path=\"status\" inline=\"true\"]\n[else]\n  <p>No owner.</p>\n[/using]"},
				{Title: "An ID stored in Meta", Code: "[using type=\"group\" id-path=\"client\"]\n  Client: [property path=\"Name\"]\n[/using]"},
				{Title: "First result of a query", Code: "[using query='type = note AND name = $name ORDER BY created DESC' param-name=\"Index\"]\n  [property path=\"Description\"]\n[/using]"},
			},
		},
		{
			Name:        "inspect",
			Syntax:      `[inspect path="..."]`,
			Description: "An author aid for the template editor's live preview: a collapsed, keyboard-operable panel listing what this point of the template can read. It shows every Meta path with its JSON type, a sample value and a ready-to-copy shortcode; the entity's own fields for [property] (associations such as Owner or Tags are named but never read, and credential fields are left out); and, at the top level of an [each] body, the first element's [item] paths. Inside [mrql], [using] or a CustomMRQLResult it shows the switched entity. It renders only in the live preview: on display pages, share pages, deferred bodies, query results and every other surface it renders nothing at all, so a template saved with it left in is harmless. It runs no query. Limits: 200 rows per panel, 10 panels per preview, samples cut at 80 characters, Meta walked 6 levels deep.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "path", Type: "string", Description: "A Meta dot-path, written as for [meta path=]. Limits the Meta section to the subtree at that path; the other sections are unaffected. Empty lists all of Meta."},
			},
			Examples: []DocExample{
				{Title: "Everything in context", Code: `[inspect]`},
				{Title: "One Meta subtree", Code: `[inspect path="project"]`},
				{Title: "Inside a loop", Code: "[each path=\"steps\"]\n  [inspect]\n  [item path=\"title\"]\n[/each]", Notes: "Only the first element gets a panel. An [inspect] nested in another block inside the loop shows the entity, as [item] renders empty there."},
			},
		},
		{
			Name:        "adjacent",
			Syntax:      `[adjacent]`,
			Description: "Previous and next links to this entity's siblings, in creation order (created_at, then id, so the order is total and editing a row never moves it). With series=\"true\" a resource steps through the members of its Series, whatever their owner or category. Otherwise siblings are the entities of the same type with the same owner (or, for an ownerless entity, the other ownerless ones) and the same category, resource category or note type (a note or group with none pairs with the others that have none). It renders a <nav> landmark with a rel=\"prev\" and a rel=\"next\" link; a side with no sibling is left out, and with neither it renders nothing. Siblings are read as the viewer, so a group-confined user only ever sees siblings inside their subtree, and an entity the viewer cannot see renders nothing. Each [adjacent] costs one unit of the page query budget and is bounded by the MRQL query timeout. It renders on display pages (their slots and list cards), deferred bodies and the template and description previews; elsewhere (MRQL render API, share pages, the lightbox, hover cards, the entity picker, generation-run tools) it renders an \"unavailable\" comment. Put it in CustomHeader or CustomSidebar rather than a per-card slot.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "series", Type: "boolean", Default: "false", Description: "Resources only. true takes the siblings from the resource's Series; a resource with no Series renders nothing. On a group or note, true renders an error marker.", Enum: []string{"true", "false"}},
				{Name: "label", Type: "string", Default: "Previous and next", Description: "The accessible name (aria-label) of the <nav> landmark. A name already used on the page gets a number, e.g. \"Previous and next (2)\"; a label of your own reads better."},
			},
			Examples: []DocExample{
				{Title: "Owner and category siblings", Code: `[adjacent]`},
				{Title: "Pages of a Series", Code: `[adjacent series="true" label="Pages"]`},
			},
		},
		{
			Name:        "similar",
			Syntax:      `[similar]`,
			Description: "Thumbnails of the resources that look like this one, from the perceptual-hash pairs the hash worker stores, nearest first. Each thumbnail links to its resource and shows a tier badge with the distance: Duplicate for a distance of 2 or less (a near-certain duplicate), Similar above that, the same cut-off as the resource page's Similar Resources panel. Its alt text gives the name, tier and distance, and a Compare link under it opens the side-by-side compare page. It works only on a resource; to show the lookalikes of another resource, such as a group's cover, wrap it in [using type=\"resource\" …]. With no stored pairs within the distance it renders nothing; it never hashes images at render time. Lookalikes are read as the viewer, so a group-confined user only sees resources inside their subtree. Each [similar] costs one unit of the page query budget. It renders on display pages, deferred bodies, the lightbox and the template previews; on share pages it renders an \"unavailable\" comment. Put it in CustomSidebar, CustomLightbox or CustomDetailFooter rather than a per-card slot.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "max-distance", Type: "number", Description: "The largest perceptual distance shown, a whole number from 0 to 11. Unset, the hash similarity threshold from the settings applies, the one the Similar Resources panel uses."},
				{Name: "limit", Type: "number", Default: "8", Description: "The most thumbnails shown, a whole number from 1 to 24."},
				{Name: "size", Type: "enum", Default: "md", Description: "Thumbnail edge: sm is 48 px, md 96 px, lg 200 px.", Enum: []string{"sm", "md", "lg"}},
				{Name: "label", Type: "string", Default: "Similar resources", Description: "The visible caption, which is also the strip's accessible name. Blank means the default."},
			},
			Examples: []DocExample{
				{Title: "Lookalikes of this resource", Code: `[similar]`},
				{Title: "Only near-certain duplicates", Code: `[similar max-distance="2" limit="12" size="sm" label="Possible duplicates"]`},
				{Title: "A group's cover image", Code: `[using type="resource" id-path="cover"][similar label="Lookalikes of the cover"][/using]`, Notes: "The strip belongs to the cover resource; a cover the viewer cannot see renders nothing."},
			},
		},
		{
			Name:        "calendar",
			Syntax:      `[calendar mrql='type = "note"' date="meta.due"]`,
			Description: "One month as an accessible grid, Monday first, of the entities an MRQL filter matches, placed by a date field. Each day shows its count, links to the day's full list on the MRQL page, and lists up to 3 linked titles followed by a \"+k more\" link. Prev/Next buttons swap the month in place on display pages, cards, the lightbox and deferred bodies; share pages, the live preview and list headers show the month without them. A month of up to 100 entities costs one query of the page query budget and its titles and counts come from the same rows; a denser month costs two and shows counts only, with a note. A day is a date on the server's local clock; a Meta date counts only when it is a string starting YYYY-MM-DD (a JSON-Schema date or an RFC3339 date-time), and its day is the date as written. Counts are read as the viewer, so a group-confined user only sees their subtree. Put it in CustomSidebar or CustomHeader rather than a per-card slot, where every card runs its own queries.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "mrql", Type: "string", Required: true, Description: "An MRQL filter naming exactly one entity type through a top-level type = …. No GROUP BY, ORDER BY, LIMIT, OFFSET, SCOPE or $parameters: the calendar adds its own month range and order."},
				{Name: "date", Type: "string", Default: "created", Description: "The date field that places an entity on a day: created, updated, or meta.<path> (letters, digits and underscores, dot-separated)."},
				{Name: "month", Type: "string", Default: "current", Description: "current (the server's current month) or YYYY-MM, year 1000 to 9999."},
				{Name: "scope", Type: "string", Description: "Group scope, as on [mrql]: global, parent, root or a group ID. Unset, or any other value, is the entity's own group."},
			},
			Examples: []DocExample{
				{Title: "Notes by due date", Code: `[calendar mrql='type = "note"' date="meta.due"]`, Notes: "Notes whose meta.due is not a YYYY-MM-DD string are left out."},
				{Title: "Uploads in one month, everywhere", Code: `[calendar mrql='type = "resource"' date="created" month="2026-03" scope="global"]`},
			},
		},
		{
			Name:        "pivot",
			Syntax:      `[pivot query="..." rows="..." cols="..."]`,
			Description: "A cross-tab of two fields over the entities an MRQL filter matches: one row per value of rows, one column per value of cols, a count (or a sum of field) in each cell, and row, column and grand totals. It renders an accessible <table> with a caption and row and column headers, in a scroll region named by the caption. Values that differ only in ASCII case share a row or column, as MRQL's = treats them; so do two related entities with the same name. A row or column of a multi-valued relation (tags, groups, notes, resources, children) counts an entity once per value, so the totals across it are left out. In count mode each cell and total links to the MRQL page filtered to exactly what it counted; sum pivots, date buckets, the (none) bucket and truncated tables have no links. The most frequent max-rows row values and max-cols column values are shown, and a note says how many more there are. One aggregated query reads at most 5000 combinations, most items first; past that the table says it is incomplete. Counts are read as the viewer, so a group-confined user only sees their subtree. Each [pivot] costs one unit of the page query budget. On share pages it renders an \"unavailable\" comment. Put it in CustomHeader or CustomSidebar rather than a per-card slot.",
			IsBlock:     BlockNo,
			Attrs: []DocAttr{
				{Name: "query", Type: "string", Required: true, Description: "An MRQL filter naming exactly one entity type through a top-level type = …. No GROUP BY, ORDER BY, LIMIT, OFFSET, SCOPE or $parameters: the pivot adds its own grouping and order."},
				{Name: "rows", Type: "string", Required: true, Description: "The row field: meta.<path>, a scalar field such as name or contentType, owner, groups, tags (or parent and children on groups), or a date bucket of a timestamp such as created.month or created.year. Not a bare timestamp, not an ID field (id, category, noteType) and not a traversal such as owner.name."},
				{Name: "cols", Type: "string", Required: true, Description: "The column field, with the same forms as rows. It must be a different field (group and groups are the same field)."},
				{Name: "value", Type: "enum", Default: "count", Description: "What each cell holds: count (entities) or sum (of field).", Enum: []string{"count", "sum"}},
				{Name: "field", Type: "string", Description: "With value=\"sum\", the field to add up: meta.<path>, or fileSize, width or height on resources. Numbers add up; how a Meta string or boolean counts differs between SQLite and PostgreSQL (see the shortcodes docs)."},
				{Name: "scope", Type: "enum", Default: "entity", Description: "Group subtree to count in, as on [mrql]: entity (the current entity's group), parent, root, global, or a numeric group ID.", Enum: []string{"entity", "parent", "root", "global"}},
				{Name: "max-rows", Type: "number", Default: "20", Description: "How many row values are shown, most items first; at most 50."},
				{Name: "max-cols", Type: "number", Default: "10", Description: "How many column values are shown, most items first; at most 20."},
				{Name: "rows-label", Type: "string", Description: "The heading of the row-label column, also used in the caption and link names. Unset or blank, the rows field without meta."},
				{Name: "cols-label", Type: "string", Description: "The name of the column field in the caption and link names. Unset or blank, the cols field without meta."},
				{Name: "caption", Type: "string", Description: "The table caption, which also names the scroll region. Unset or blank: \"Count by <rows> and <cols>\" or \"Sum of <field> by <rows> and <cols>\"."},
			},
			Examples: []DocExample{
				{Title: "Resources by type and year", Code: `[pivot query="type = resource" rows="contentType" cols="created.year"]`},
				{Title: "Open work by owner", Code: `[pivot query="type = note AND meta.status IS NOT NULL" rows="meta.status" cols="owner" caption="Open work by owner"]`},
				{Title: "Bytes by type and year", Code: `[pivot query="type = resource" rows="contentType" cols="created.year" value="sum" field="fileSize"]`},
			},
		},
		{
			Name:        "comment",
			Syntax:      `[comment]...[/comment]`,
			Description: "Comments out part of a template or Description. Everything between [comment] and [/comment] is removed on the server before any other shortcode is read, so nothing inside runs and none of it is rendered. An HTML comment is different: the shortcodes inside it still run and the comment is sent to the browser. Comments nest. An unclosed [comment] hides everything after it, and a [/comment] with no opener is removed; the linter reports both. The opener accepts any attributes, such as reason=\"…\", and ignores them; it ends at its first ], so a value cannot contain one, and it must fit on one line. There is no way to write a literal [comment] on the page.",
			IsBlock:     BlockRequired,
			Attrs:       []DocAttr{},
			Examples: []DocExample{
				{Title: "Hide a section", Code: "[comment]\n  <h3>Related</h3>\n  [mrql query=\"type = note\"]\n[/comment]"},
				{Title: "Say why", Code: "[comment reason=\"old sidebar\"]<aside>[meta path=\"legacy\"]</aside>[/comment]"},
			},
		},
	}
}
