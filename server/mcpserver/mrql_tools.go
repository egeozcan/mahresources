package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mahresources/skills"
)

// MRQLFacts are the deployment facts a model needs to write MRQL that runs
// here, not merely MRQL that parses.
type MRQLFacts struct {
	// Postgres enables the regex operators (~*, !~*).
	Postgres bool
	// FullText reports whether full-text search is available (ORDER BY RANK).
	FullText bool
	// DefaultLimit is the LIMIT a query without one runs with.
	DefaultLimit int
	// Rules are the compact syntax rules and example mappings the NL→MRQL
	// generator is given (application_context.MRQLLanguageReference).
	Rules []string
}

// mrqlReferenceDroppedSections are sections of the skill's language reference
// about surfaces a model on MCP does not use: the CLI, the web UI and the
// HTML rendering of results.
var mrqlReferenceDroppedSections = map[string]bool{
	"CLI Invocation":        true,
	"CLI Reference":         true,
	"List-Page Filter Bar":  true,
	"MRQL in Global Search": true,
	"Rendering":             true,
	"See Also":              true,
}

// mrqlLanguageForMCP is the skill's language reference without its generated
// header comment and the sections named in mrqlReferenceDroppedSections.
func mrqlLanguageForMCP() string {
	src := skills.MRQLLanguage
	if strings.HasPrefix(src, "<!--") {
		if i := strings.Index(src, "-->"); i >= 0 {
			src = src[i+len("-->"):]
		}
	}
	var out []string
	inFence, dropping := false, false
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		} else if !inFence && strings.HasPrefix(line, "## ") {
			dropping = mrqlReferenceDroppedSections[strings.TrimSpace(strings.TrimPrefix(line, "## "))]
		}
		if !dropping {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

func (c *caller) mrqlReference(args map[string]any) *mcp.CallToolResult {
	var b strings.Builder
	if c.h.opts.MRQL != nil {
		f := c.h.opts.MRQL()
		b.WriteString("# MRQL on this server\n\n")
		if f.Postgres {
			b.WriteString("- Database: PostgreSQL. The regex operators ~* and !~* are available.\n")
		} else {
			b.WriteString("- Database: SQLite. The regex operators ~* and !~* parse and validate, but fail when the query runs; use ~ patterns instead.\n")
		}
		if f.FullText {
			b.WriteString("- Full-text search is available: TEXT ~ \"words\" uses the index, and ORDER BY RANK works.\n")
		} else {
			b.WriteString("- Full-text search is not available: ORDER BY RANK fails.\n")
		}
		if f.DefaultLimit > 0 {
			fmt.Fprintf(&b, "- A query without LIMIT runs with LIMIT %d: at most %d results, or %d items per bucket in a bucketed GROUP BY. Write an explicit LIMIT.\n", f.DefaultLimit, f.DefaultLimit, f.DefaultLimit)
		}
		b.WriteString("- Local names (tags, categories, note types, meta keys, saved queries) come from mrql_vocabulary. category and noteType take numeric IDs.\n\n")
		if len(f.Rules) > 0 {
			b.WriteString("# Rules\n\n")
			for _, r := range f.Rules {
				b.WriteString("- " + r + "\n")
			}
			b.WriteString("\n")
		}
	}
	switch topic := stringArg(args, "topic"); topic {
	case "", "language":
		b.WriteString(mrqlLanguageForMCP())
	case "recipes":
		b.WriteString("The recipes below are written as `mr` CLI commands. `mr mrql '<query>'` is mrql_query with that query; " +
			"`--param name=value` is params {\"name\": \"value\"}; `mr mrql run <name>` is run_saved_query.\n\n")
		b.WriteString(skills.MRQLRecipes)
	default:
		return toolError(fmt.Sprintf("topic must be language or recipes, not %q", topic))
	}
	return textResult(b.String())
}

// vocabularySection is one list mrql_vocabulary can return.
type vocabularySection struct {
	opID string
	// args are the operation's fixed arguments.
	args func(toolArgs map[string]any) map[string]any
	// metaKeys marks a response of [{"key": ...}].
	metaKeys bool
	// savedQueries marks the saved-query list, whose rows carry the query.
	savedQueries bool
	// usedAs says how MRQL refers to the section's values.
	usedAs string
}

var vocabularySections = map[string]vocabularySection{
	"tags": {opID: "listTags", usedAs: `tags = "<name>"`, args: func(a map[string]any) map[string]any {
		if n := stringArg(a, "tagName"); n != "" {
			return map[string]any{"Name": n}
		}
		return map[string]any{}
	}},
	"categories":         {opID: "listCategories", usedAs: "type = group AND category = <id>"},
	"resourceCategories": {opID: "listResourceCategories", usedAs: "type = resource AND category = <id>"},
	"noteTypes":          {opID: "getNoteTypes", usedAs: "type = note AND noteType = <id>"},
	"resourceMetaKeys":   {opID: "getResourceMetaKeys", metaKeys: true, usedAs: "type = resource AND meta.<key> ..."},
	"noteMetaKeys":       {opID: "getNoteMetaKeys", metaKeys: true, usedAs: "type = note AND meta.<key> ..."},
	"groupMetaKeys":      {opID: "getGroupMetaKeys", metaKeys: true, usedAs: "type = group AND meta.<key> ..."},
	"savedQueries":       {opID: "listSavedMRQLQueries", savedQueries: true, usedAs: "run_saved_query with id or name"},
}

// vocabularyPages bounds a paginated section at vocabularyPages × 50 rows.
const vocabularyPages = 4

func vocabularySectionNames() []any {
	var names []string
	for n := range vocabularySections {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]any, len(names))
	for i, n := range names {
		out[i] = n
	}
	return out
}

// mrqlVocabulary collects the caller's local names for MRQL, each section
// through its own REST operation as the caller, so roles and group scope
// apply as they do to the REST API.
func (c *caller) mrqlVocabulary(args map[string]any) *mcp.CallToolResult {
	var want []string
	switch v := args["sections"].(type) {
	case nil:
		for _, n := range vocabularySectionNames() {
			want = append(want, n.(string))
		}
	case []any:
		for _, x := range v {
			s, _ := x.(string)
			if _, ok := vocabularySections[s]; !ok {
				return toolError(fmt.Sprintf("unknown section %q; sections are %v", s, vocabularySectionNames()))
			}
			want = append(want, s)
		}
	default:
		return toolError(`"sections" must be an array of section names`)
	}

	out := map[string]any{}
	notes := map[string]string{}
	for _, name := range want {
		sec := vocabularySections[name]
		op, ok := c.h.catalog.Get(sec.opID)
		if !ok {
			notes[name] = "not available"
			continue
		}
		rows, more, err := c.vocabularyRows(op, sec, args)
		if err != nil {
			notes[name] = err.Error()
			continue
		}
		out[name] = rows
		usage := "MRQL: " + sec.usedAs
		if more {
			usage += fmt.Sprintf("; only the first %d are listed", len(rows))
			if name == "tags" {
				usage += ", pass tagName to filter"
			}
		}
		notes[name] = usage
	}
	out["notes"] = notes
	return jsonResult(out)
}

// vocabularyRows fetches one section. more reports that rows stopped at the
// page bound with a full page.
func (c *caller) vocabularyRows(op *Operation, sec vocabularySection, toolArgs map[string]any) ([]any, bool, error) {
	base := map[string]any{}
	if sec.args != nil {
		base = sec.args(toolArgs)
	}
	if sec.savedQueries {
		base["all"] = json.Number("1")
	}
	paginated := !sec.metaKeys && !sec.savedQueries
	pages := 1
	if paginated {
		pages = vocabularyPages
	}
	var rows []any
	for page := 1; page <= pages; page++ {
		a := map[string]any{}
		for k, v := range base {
			a[k] = v
		}
		if paginated {
			a["page"] = json.Number(fmt.Sprint(page))
		}
		resp, err := dispatch(c.h.opts.Dispatch, c.readOnly(), op, a, 2<<20)
		if err != nil {
			return nil, false, err
		}
		if resp.Truncated {
			return nil, false, fmt.Errorf("response too large; use %s through call_operation", op.ID)
		}
		if resp.Status >= 300 {
			if resp.Status == http.StatusForbidden || resp.Status == http.StatusUnauthorized {
				return nil, false, fmt.Errorf("not permitted (HTTP %d)", resp.Status)
			}
			return nil, false, fmt.Errorf("HTTP %d", resp.Status)
		}
		var items []map[string]any
		if err := json.Unmarshal(resp.Body, &items); err != nil {
			return nil, false, fmt.Errorf("unexpected response from %s", op.ID)
		}
		for _, it := range items {
			rows = append(rows, vocabularyRow(sec, it))
		}
		if !paginated || len(items) < 50 {
			return rows, false, nil
		}
	}
	return rows, true, nil
}

func vocabularyRow(sec vocabularySection, it map[string]any) any {
	field := func(keys ...string) any {
		for _, k := range keys {
			if v, ok := it[k]; ok {
				return v
			}
		}
		return nil
	}
	if sec.metaKeys {
		return field("key", "Key")
	}
	row := map[string]any{"id": field("ID", "id"), "name": field("Name", "name")}
	if sec.savedQueries {
		row["query"] = field("query", "Query")
		if p := field("params"); p != nil {
			row["params"] = p
		}
		if d, _ := field("description", "Description").(string); d != "" {
			row["description"] = clip(d, 200)
		}
	}
	return row
}

// buildMRQLTools returns the MRQL tools: running, validating and saving
// queries, plus the reference and the caller's vocabulary to write them from.
func buildMRQLTools(c *Catalog) ([]namedTool, error) {
	ops := map[string]*Operation{}
	for _, id := range []string{"executeMRQL", "validateMRQL", "listSavedMRQLQueries", "runSavedMRQLQuery", "createSavedMRQLQuery"} {
		op, ok := c.Get(id)
		if !ok {
			return nil, fmt.Errorf("MRQL tools: operation %s is not exposed", id)
		}
		ops[id] = op
	}
	paramsSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"description":          `Values for the query's $name placeholders, e.g. {"tag": "photo"} for tags = $tag`,
	}
	pageProps := func() map[string]any {
		return map[string]any{
			"params":  paramsSchema,
			"limit":   map[string]any{"type": "integer", "description": "Total results, or items per bucket for a bucketed GROUP BY. Overrides the query's LIMIT."},
			"buckets": map[string]any{"type": "integer", "description": "Buckets per page (bucketed GROUP BY only)"},
			"page":    map[string]any{"type": "integer", "description": "1-based page"},
			"offset":  map[string]any{"type": "integer", "description": "Bucket offset for cursor paging (bucketed GROUP BY); wins over page"},
		}
	}
	pass := func(op *Operation) func(map[string]any) (*Operation, map[string]any, error) {
		return func(args map[string]any) (*Operation, map[string]any, error) { return op, args, nil }
	}
	ro := annotationsFor(ReadOnly, false, http.MethodGet)

	queryProps := pageProps()
	queryProps["query"] = map[string]any{"type": "string", "description": "The MRQL query"}
	runProps := pageProps()
	runProps["id"] = map[string]any{"type": "integer", "description": "Saved query ID"}
	runProps["name"] = map[string]any{"type": "string", "description": "Saved query name (used when id is absent or not found)"}

	return []namedTool{
		{
			tool: &mcp.Tool{
				Name:  "mrql_query",
				Title: "Run an MRQL query",
				Description: "Run a Mahresources Query Language query and return matching resources, notes or groups, or GROUP BY rows/buckets. " +
					"Shape: [type = resource|note|group AND] <conditions> [SCOPE <group>] [GROUP BY ...] [ORDER BY ...] [LIMIT n] [OFFSET n]. " +
					"Example: type = resource AND tags = \"photo\" AND created > -30d ORDER BY created DESC LIMIT 20. " +
					"Strings are double-quoted; ~ is a case-insensitive wildcard match (name ~ \"report*\"); there is no LIKE, HAS or CONTAINS. " +
					"category and noteType take numeric IDs. Read mrql_reference before writing a non-trivial query, " +
					"and look up local tag, category and meta key names with mrql_vocabulary.",
				InputSchema: map[string]any{"type": "object", "properties": queryProps, "required": []any{"query"}},
				Annotations: ro,
			},
			resolve: pass(ops["executeMRQL"]),
		},
		{
			tool: &mcp.Tool{
				Name:  "mrql_validate",
				Title: "Validate an MRQL query",
				Description: "Parse and validate an MRQL query without running it. Returns valid, errors with positions, and the $name placeholders it needs. " +
					"Validation does not check the database dialect: a regex (~*) query validates on SQLite but fails when run.",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{
					"query": map[string]any{"type": "string"},
				}, "required": []any{"query"}},
				Annotations: ro,
			},
			resolve: pass(ops["validateMRQL"]),
		},
		{
			tool: &mcp.Tool{
				Name:  "mrql_reference",
				Title: "MRQL language reference",
				Description: "The MRQL language reference: query shape, operators, every field per entity type, metadata, relation counts, " +
					"relative dates, SCOPE, GROUP BY/HAVING, hierarchy traversal, similarity, ordering and $params, plus what this server supports. " +
					"topic \"recipes\" returns task-shaped example queries instead.",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{
					"topic": map[string]any{"type": "string", "enum": []any{"language", "recipes"}, "description": "language (default) or recipes"},
				}},
				Annotations: ro,
			},
			handle: func(_ context.Context, c *caller, _ *mcp.CallToolRequest, args map[string]any) *mcp.CallToolResult {
				return c.mrqlReference(args)
			},
		},
		{
			tool: &mcp.Tool{
				Name:  "mrql_vocabulary",
				Title: "MRQL vocabulary",
				Description: "The names an MRQL query refers to on this server, as the caller sees them: tags, group categories and resource categories " +
					"(with the numeric IDs category = <id> needs), note types (noteType = <id>), the meta keys in use per entity type, and saved queries. " +
					"Paginated lists stop at 200 rows; pass tagName to find tags by name.",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{
					"sections": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": vocabularySectionNames()},
						"description": "Sections to return; all when omitted"},
					"tagName": map[string]any{"type": "string", "description": "Only tags whose name contains this"},
				}},
				Annotations: ro,
			},
			handle: func(_ context.Context, c *caller, _ *mcp.CallToolRequest, args map[string]any) *mcp.CallToolResult {
				return c.mrqlVocabulary(args)
			},
		},
		{
			tool: &mcp.Tool{
				Name:        "list_saved_queries",
				Title:       "List saved MRQL queries",
				Description: "List saved MRQL queries with their query text and $name placeholders, or fetch one with id. Pass all=1 for every query.",
				InputSchema: ops["listSavedMRQLQueries"].InputSchema,
				Annotations: ro,
			},
			resolve: pass(ops["listSavedMRQLQueries"]),
		},
		{
			tool: &mcp.Tool{
				Name:        "run_saved_query",
				Title:       "Run a saved MRQL query",
				Description: "Run a saved MRQL query by id or name. params binds its $name placeholders (list_saved_queries shows which it needs).",
				InputSchema: map[string]any{"type": "object", "properties": runProps},
				Annotations: ro,
			},
			resolve: pass(ops["runSavedMRQLQuery"]),
		},
		{
			tool: &mcp.Tool{
				Name:        "save_mrql_query",
				Title:       "Save an MRQL query",
				Description: "Save an MRQL query under a unique name so it can be run again, from here or the web UI. Validate it first with mrql_validate.",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{
					"name":        map[string]any{"type": "string"},
					"query":       map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
				}, "required": []any{"name", "query"}},
				Annotations: annotationsFor(ops["createSavedMRQLQuery"].Safety, false, http.MethodPost),
			},
			resolve: pass(ops["createSavedMRQLQuery"]),
		},
	}, nil
}
