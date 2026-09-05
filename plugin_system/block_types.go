package plugin_system

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"mahresources/models/block_types"

	"github.com/santhosh-tekuri/jsonschema/v6"
	lua "github.com/yuin/gopher-lua"
)

// Compile-time check: PluginBlockType implements block_types.BlockType.
var _ block_types.BlockType = (*PluginBlockType)(nil)

// BlockTypeFilter restricts which notes/categories can use a plugin block type.
//
// NoteTypeIDs matches the note's own type. CategoryIDs matches the Category of
// the note's owning Group — a note has no category of its own, so "available
// only inside the Recipes category" means "owned by a group in that category".
type BlockTypeFilter struct {
	NoteTypeIDs []uint `json:"note_type_ids,omitempty"`
	CategoryIDs []uint `json:"category_ids,omitempty"`
}

// Allows reports whether this filter admits a note with the given note type and
// owning-group category. Both are pointers because either can be unset, and an
// unset value cannot satisfy a filter that names specific ids.
//
// Filters are AND-joined: a block type declaring both must match both. An empty
// filter admits everything.
//
// This is the single predicate behind both halves of the feature — the write
// path that refuses a disallowed block, and the picker that does not offer it.
// They were allowed to drift before: category_ids was parsed, stored and
// shipped to the browser while nothing read it, and the picker listed every
// type regardless, so a filtered type was offered on every note and failed only
// after the click.
func (f BlockTypeFilter) Allows(noteTypeID, ownerCategoryID *uint) bool {
	if len(f.NoteTypeIDs) > 0 {
		if noteTypeID == nil || !containsID(f.NoteTypeIDs, *noteTypeID) {
			return false
		}
	}
	if len(f.CategoryIDs) > 0 {
		if ownerCategoryID == nil || !containsID(f.CategoryIDs, *ownerCategoryID) {
			return false
		}
	}
	return true
}

func containsID(ids []uint, id uint) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

// PluginBlockType implements the block_types.BlockType interface for plugin-defined
// block types. It uses JSON Schema for content/state validation instead of Go struct
// unmarshalling.
type PluginBlockType struct {
	PluginName    string
	TypeName      string // full namespaced: plugin:<pluginName>:<type>
	Label         string
	Icon          string
	Description   string
	Scripts       []string // ordered URLs within this plugin's public assets
	contentSchema *jsonschema.Schema
	stateSchema   *jsonschema.Schema
	DefContent    json.RawMessage
	DefState      json.RawMessage
	Filters       BlockTypeFilter
	RenderView    *lua.LFunction
	RenderEdit    *lua.LFunction
	State         *lua.LState // Lua VM for rendering
}

// PluginBlockTypeConfig holds construction parameters for NewPluginBlockType.
type PluginBlockTypeConfig struct {
	PluginName    string
	TypeName      string
	Label         string
	Icon          string
	Description   string
	Scripts       []string // paths relative to this plugin's public directory
	ContentSchema string   // JSON Schema string; empty means accept all
	StateSchema   string   // JSON Schema string; empty means accept all
	DefContent    json.RawMessage
	DefState      json.RawMessage
	Filters       BlockTypeFilter
	RenderView    *lua.LFunction
	RenderEdit    *lua.LFunction
	State         *lua.LState
}

// NewPluginBlockType creates a PluginBlockType, compiling any JSON Schemas at
// construction time. Returns an error if a schema is invalid.
func NewPluginBlockType(cfg PluginBlockTypeConfig) (*PluginBlockType, error) {
	var scripts []string
	for _, script := range cfg.Scripts {
		if !validBlockScriptPath.MatchString(script) || path.Clean(script) != script || strings.HasPrefix(script, "/") || strings.Contains(script, "../") {
			return nil, fmt.Errorf("invalid block script %q: expected a relative .js path within the plugin public directory", script)
		}
		scripts = append(scripts, "/plugins/"+cfg.PluginName+"/public/"+script)
	}
	bt := &PluginBlockType{
		PluginName:  cfg.PluginName,
		TypeName:    cfg.TypeName,
		Label:       cfg.Label,
		Icon:        cfg.Icon,
		Description: cfg.Description,
		Scripts:     scripts,
		DefContent:  cfg.DefContent,
		DefState:    cfg.DefState,
		Filters:     cfg.Filters,
		RenderView:  cfg.RenderView,
		RenderEdit:  cfg.RenderEdit,
		State:       cfg.State,
	}

	if cfg.ContentSchema != "" {
		schema, err := compileSchema(cfg.TypeName+"/content", cfg.ContentSchema)
		if err != nil {
			return nil, fmt.Errorf("invalid content schema for %s: %w", cfg.TypeName, err)
		}
		bt.contentSchema = schema
	}

	if cfg.StateSchema != "" {
		schema, err := compileSchema(cfg.TypeName+"/state", cfg.StateSchema)
		if err != nil {
			return nil, fmt.Errorf("invalid state schema for %s: %w", cfg.TypeName, err)
		}
		bt.stateSchema = schema
	}

	return bt, nil
}

// compileSchema parses a JSON Schema string and compiles it. The id is used as
// the resource identifier for the compiler.
func compileSchema(id string, schemaJSON string) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("parsing schema JSON: %w", err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		return nil, fmt.Errorf("adding schema resource: %w", err)
	}

	schema, err := c.Compile(id)
	if err != nil {
		return nil, fmt.Errorf("compiling schema: %w", err)
	}

	return schema, nil
}

// Type returns the unique block type identifier (e.g., "plugin:myplugin:custom-block").
func (bt *PluginBlockType) Type() string {
	return bt.TypeName
}

// ValidateContent validates content JSON against the content schema.
// If no content schema is set, all content is accepted.
func (bt *PluginBlockType) ValidateContent(content json.RawMessage) error {
	return bt.validateAgainstSchema(bt.contentSchema, content)
}

// ValidateState validates state JSON against the state schema.
// If no state schema is set, all state is accepted.
func (bt *PluginBlockType) ValidateState(state json.RawMessage) error {
	return bt.validateAgainstSchema(bt.stateSchema, state)
}

// DefaultContent returns the default content for new blocks of this type.
func (bt *PluginBlockType) DefaultContent() json.RawMessage {
	return bt.DefContent
}

// DefaultState returns the default state for new blocks of this type.
func (bt *PluginBlockType) DefaultState() json.RawMessage {
	return bt.DefState
}

// validateAgainstSchema validates a JSON raw message against a compiled schema.
// Returns nil if schema is nil (accept all).
func (bt *PluginBlockType) validateAgainstSchema(schema *jsonschema.Schema, data json.RawMessage) error {
	if schema == nil {
		return nil
	}

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if err := schema.Validate(v); err != nil {
		return err
	}

	return nil
}

var validBlockTypeName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,49}$`)
var validBlockScriptPath = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_./-]*\.js$`)

// parseBlockTypeTable parses a Lua table from mah.block_type({...}) into a PluginBlockType.
// Required fields: type, label, render_view, render_edit.
// Optional fields: icon, description, scripts, content_schema, state_schema, default_content, default_state, filters.
func parseBlockTypeTable(L *lua.LState, tbl *lua.LTable, pluginName string) (*PluginBlockType, error) {
	cfg := PluginBlockTypeConfig{
		PluginName: pluginName,
	}

	// Required: type (must be a string)
	if v := tbl.RawGetString("type"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'type'")
	} else if str, ok := v.(lua.LString); !ok {
		return nil, fmt.Errorf("'type' must be a string, got %s", v.Type())
	} else {
		raw := string(str)
		if !validBlockTypeName.MatchString(raw) {
			return nil, fmt.Errorf("invalid type name %q: must match [a-z][a-z0-9-]{0,49}", raw)
		}
		cfg.TypeName = "plugin:" + pluginName + ":" + raw
	}

	// Required: label
	if v := tbl.RawGetString("label"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'label'")
	} else {
		cfg.Label = v.String()
	}

	// Required: render_view (must be a function)
	var renderView *lua.LFunction
	if v := tbl.RawGetString("render_view"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'render_view'")
	} else if fn, ok := v.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("'render_view' must be a function")
	} else {
		renderView = fn
	}

	// Required: render_edit (must be a function)
	var renderEdit *lua.LFunction
	if v := tbl.RawGetString("render_edit"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'render_edit'")
	} else if fn, ok := v.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("'render_edit' must be a function")
	} else {
		renderEdit = fn
	}

	// Optional: icon
	if v := tbl.RawGetString("icon"); v != lua.LNil {
		cfg.Icon = v.String()
	}

	// Optional: description
	if v := tbl.RawGetString("description"); v != lua.LNil {
		cfg.Description = v.String()
	}

	// Optional: scripts, loaded in order before block HTML becomes interactive.
	if v := tbl.RawGetString("scripts"); v != lua.LNil {
		list, ok := v.(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("'scripts' must be an array of relative .js paths")
		}
		var parseErr error
		list.ForEach(func(key, value lua.LValue) {
			index, indexed := key.(lua.LNumber)
			script, isString := value.(lua.LString)
			if !indexed || index != lua.LNumber(len(cfg.Scripts)+1) || !isString {
				parseErr = fmt.Errorf("'scripts' must be an array of relative .js paths")
				return
			}
			cfg.Scripts = append(cfg.Scripts, string(script))
		})
		if parseErr != nil {
			return nil, parseErr
		}
	}

	// Optional: content_schema (Lua table -> JSON)
	if v := tbl.RawGetString("content_schema"); v != lua.LNil {
		if schemaTbl, ok := v.(*lua.LTable); ok {
			goVal := luaValueToGo(schemaTbl)
			jsonBytes, err := json.Marshal(goVal)
			if err != nil {
				return nil, fmt.Errorf("marshalling content_schema: %w", err)
			}
			cfg.ContentSchema = string(jsonBytes)
		}
	}

	// Optional: state_schema (Lua table -> JSON)
	if v := tbl.RawGetString("state_schema"); v != lua.LNil {
		if schemaTbl, ok := v.(*lua.LTable); ok {
			goVal := luaValueToGo(schemaTbl)
			jsonBytes, err := json.Marshal(goVal)
			if err != nil {
				return nil, fmt.Errorf("marshalling state_schema: %w", err)
			}
			cfg.StateSchema = string(jsonBytes)
		}
	}

	// Optional: default_content (Lua table -> JSON)
	if v := tbl.RawGetString("default_content"); v != lua.LNil {
		if dcTbl, ok := v.(*lua.LTable); ok {
			goVal := luaValueToGo(dcTbl)
			jsonBytes, err := json.Marshal(goVal)
			if err != nil {
				return nil, fmt.Errorf("marshalling default_content: %w", err)
			}
			cfg.DefContent = json.RawMessage(jsonBytes)
		}
	}

	// Optional: default_state (Lua table -> JSON)
	if v := tbl.RawGetString("default_state"); v != lua.LNil {
		if dsTbl, ok := v.(*lua.LTable); ok {
			goVal := luaValueToGo(dsTbl)
			jsonBytes, err := json.Marshal(goVal)
			if err != nil {
				return nil, fmt.Errorf("marshalling default_state: %w", err)
			}
			cfg.DefState = json.RawMessage(jsonBytes)
		}
	}

	// Optional: filters
	//
	// Ids are validated rather than cast: a filter states where a block type may
	// be used, and uint(2.9) quietly naming category 2 offers it somewhere the
	// author did not choose.
	if v := tbl.RawGetString("filters"); v != lua.LNil {
		filtersTbl, ok := v.(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("'filters' must be a table, got %s", v.Type())
		}
		{
			// note_type_ids
			if ni := filtersTbl.RawGetString("note_type_ids"); ni != lua.LNil {
				niTbl, ok := ni.(*lua.LTable)
				if !ok {
					return nil, fmt.Errorf("filters.note_type_ids must be an array of ids, got %s", ni.Type())
				}
				ids, err := collectFilterIDs(cfg.Filters.NoteTypeIDs, niTbl, "filters.note_type_ids")
				if err != nil {
					return nil, err
				}
				cfg.Filters.NoteTypeIDs = ids
			}
			// category_ids
			if ci := filtersTbl.RawGetString("category_ids"); ci != lua.LNil {
				ciTbl, ok := ci.(*lua.LTable)
				if !ok {
					return nil, fmt.Errorf("filters.category_ids must be an array of ids, got %s", ci.Type())
				}
				ids, err := collectFilterIDs(cfg.Filters.CategoryIDs, ciTbl, "filters.category_ids")
				if err != nil {
					return nil, err
				}
				cfg.Filters.CategoryIDs = ids
			}
		}
	}

	pbt, err := NewPluginBlockType(cfg)
	if err != nil {
		return nil, err
	}

	pbt.RenderView = renderView
	pbt.RenderEdit = renderEdit

	return pbt, nil
}
