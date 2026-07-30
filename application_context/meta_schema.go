package application_context

import (
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Findings 17/93: a Category's, Note Type's or Resource Category's Meta JSON
// Schema was accepted and persisted verbatim whatever it contained. `{ not valid
// json ][` saved with no lint marker, no client validation and no server
// rejection, and every entity form in that category then silently fell back to
// the freeform meta editor with nothing to say why. SectionConfig, the field
// declared right beside it, is stored as types.JSON and so has always been
// parse-checked — the gap was local to MetaSchema.
//
// Validation lives here rather than in each context file because four entry
// points write the field (Category create/update, Note Type create/update,
// Resource Category create/update, and the generic CRUD writers), and a rule
// enforced in three of four places is not a rule.

// ValidateMetaSchema checks that a Meta JSON Schema is usable: it must parse as
// JSON and compile as a JSON Schema. An empty or whitespace-only value is valid
// and means "no schema" — that is how a carrier with a freeform meta editor is
// spelled.
//
// The returned error quotes the parser's own message, which is what the editor
// shows the author, so a rejection says where the problem is rather than that
// there is one.
func ValidateMetaSchema(schemaJSON string) error {
	if strings.TrimSpace(schemaJSON) == "" {
		return nil
	}

	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(schemaJSON))
	if err != nil {
		return fmt.Errorf("meta JSON schema is not valid JSON: %w", err)
	}

	c := jsonschema.NewCompiler()
	const id = "mem://meta-schema.json"
	if err := c.AddResource(id, doc); err != nil {
		return fmt.Errorf("meta JSON schema is not a valid JSON Schema: %w", err)
	}
	if _, err := c.Compile(id); err != nil {
		return fmt.Errorf("meta JSON schema is not a valid JSON Schema: %w", err)
	}
	return nil
}
