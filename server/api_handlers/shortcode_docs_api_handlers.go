package api_handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/mrql"
	"mahresources/plugin_system"
	"mahresources/server/http_utils"
	"mahresources/shortcodes"
)

// shortcodeDocAttrResponse is the JSON shape of one documented attribute.
type shortcodeDocAttrResponse struct {
	Name        string   `json:"name"`
	Type        string   `json:"type,omitempty"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Wildcard    bool     `json:"wildcard,omitempty"`
}

// shortcodeDocExampleResponse is the JSON shape of one usage example.
type shortcodeDocExampleResponse struct {
	Title string `json:"title,omitempty"`
	Code  string `json:"code"`
	Notes string `json:"notes,omitempty"`
}

// shortcodeDocResponse is one entry in the /v1/shortcodes/docs response: a
// built-in or plugin shortcode described uniformly for the editor tooling.
type shortcodeDocResponse struct {
	Name        string                        `json:"name"`    // "meta" or "plugin:foo:badge"
	Syntax      string                        `json:"syntax"`  // one-line usage hint
	Description string                        `json:"description"`
	IsBlock     string                        `json:"isBlock"` // "no" | "optional" | "required"
	Source      string                        `json:"source"`  // "builtin" | "plugin"
	Attrs       []shortcodeDocAttrResponse    `json:"attrs"`
	Examples    []shortcodeDocExampleResponse `json:"examples"`
}

// GetShortcodeDocsHandler handles GET /v1/shortcodes/docs — a machine-readable
// catalogue of the built-in shortcodes plus every shortcode registered by
// an enabled plugin. It powers editor lint, autocomplete, and hover docs.
func GetShortcodeDocsHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		docs := make([]shortcodeDocResponse, 0, 8)

		for _, d := range shortcodes.BuiltinDocs() {
			docs = append(docs, shortcodeDocResponse{
				Name:        d.Name,
				Syntax:      d.Syntax,
				Description: d.Description,
				IsBlock:     string(d.IsBlock),
				Source:      "builtin",
				Attrs:       convertBuiltinAttrs(d.Attrs),
				Examples:    convertBuiltinExamples(d.Examples),
			})
		}

		if pm := ctx.PluginManager(); pm != nil {
			for _, sc := range pm.AllShortcodeDocs() {
				docs = append(docs, shortcodeDocResponse{
					Name:        sc.FullName,
					Syntax:      pluginShortcodeSyntax(sc),
					Description: sc.Description,
					// Plugin shortcodes declare no block capability; the render
					// context carries is_block, so both forms are accepted.
					IsBlock:  string(shortcodes.BlockOptional),
					Source:   "plugin",
					Attrs:    convertPluginAttrs(sc.Attrs),
					Examples: convertPluginExamples(sc.Examples),
				})
			}
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(docs)
	}
}

func convertBuiltinAttrs(attrs []shortcodes.DocAttr) []shortcodeDocAttrResponse {
	out := make([]shortcodeDocAttrResponse, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, shortcodeDocAttrResponse{
			Name:        a.Name,
			Type:        a.Type,
			Required:    a.Required,
			Default:     a.Default,
			Description: a.Description,
			Enum:        a.Enum,
			Wildcard:    a.Wildcard,
		})
	}
	return out
}

func convertBuiltinExamples(examples []shortcodes.DocExample) []shortcodeDocExampleResponse {
	out := make([]shortcodeDocExampleResponse, 0, len(examples))
	for _, e := range examples {
		out = append(out, shortcodeDocExampleResponse{Title: e.Title, Code: e.Code, Notes: e.Notes})
	}
	return out
}

func convertPluginAttrs(attrs []plugin_system.ShortcodeDocAttr) []shortcodeDocAttrResponse {
	out := make([]shortcodeDocAttrResponse, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, shortcodeDocAttrResponse{
			Name:        a.Name,
			Type:        a.Type,
			Required:    a.Required,
			Default:     a.Default,
			Description: a.Description,
		})
	}
	return out
}

func convertPluginExamples(examples []plugin_system.ShortcodeDocExample) []shortcodeDocExampleResponse {
	out := make([]shortcodeDocExampleResponse, 0, len(examples))
	for _, e := range examples {
		out = append(out, shortcodeDocExampleResponse{Title: e.Title, Code: e.Code, Notes: e.Notes})
	}
	return out
}

type shortcodeLintRequest struct {
	Content string `json:"content" schema:"content"`
}

type shortcodeLintResponse struct {
	Issues []shortcodes.LintIssue `json:"issues"`
}

// GetShortcodeLintHandler handles POST /v1/shortcodes/lint — pure-parse linting
// of shortcode markup. It never executes shortcodes, plugin code, or the DB;
// only the MRQL parser is invoked to syntax-check query attributes. Listed in
// isReadViaPost so read-only principals may lint while authoring templates.
func GetShortcodeLintHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req shortcodeLintRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		known := buildKnownShortcodes(ctx)
		validateMRQL := func(query string) error {
			_, err := mrql.Parse(query)
			return err
		}

		issues := shortcodes.Lint(req.Content, shortcodes.LintOptions{
			Known:        known,
			ValidateMRQL: validateMRQL,
		})
		if issues == nil {
			issues = []shortcodes.LintIssue{}
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(shortcodeLintResponse{Issues: issues})
	}
}

// buildKnownShortcodes assembles the linter catalogue from the built-in registry
// plus every enabled plugin shortcode. A plugin shortcode is treated as
// "documented" (attribute checks enabled) only when it declares attributes.
func buildKnownShortcodes(ctx *application_context.MahresourcesContext) shortcodes.KnownShortcodes {
	known := shortcodes.KnownFromBuiltins()

	pm := ctx.PluginManager()
	if pm == nil {
		return known
	}
	for _, sc := range pm.AllShortcodeDocs() {
		attrs := make(map[string]shortcodes.DocAttr, len(sc.Attrs))
		for _, a := range sc.Attrs {
			attrs[a.Name] = shortcodes.DocAttr{
				Name:        a.Name,
				Type:        a.Type,
				Required:    a.Required,
				Default:     a.Default,
				Description: a.Description,
			}
		}
		known[sc.FullName] = shortcodes.KnownShortcode{
			Name:       sc.FullName,
			Block:      shortcodes.BlockOptional,
			Attrs:      attrs,
			Documented: len(sc.Attrs) > 0,
		}
	}
	return known
}

// pluginShortcodeSyntax builds a one-line usage hint, marking required attrs.
func pluginShortcodeSyntax(sc plugin_system.PluginShortcodeInfo) string {
	syntax := "[" + sc.FullName
	for _, a := range sc.Attrs {
		if a.Required {
			syntax += fmt.Sprintf(` %s="…"`, a.Name)
		}
	}
	return syntax + "]"
}
