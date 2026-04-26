package plugin_system

import (
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// ActionParam describes a single parameter for a plugin action.
//
// ShowWhen, when set, gates the param's visibility in the action modal:
// the param renders only if every key in the map equals the live form
// value for that key (AND-joined equality). Hidden params are also
// skipped during required-field validation. The map is plumbed verbatim
// to the frontend, which interprets it identically.
type ActionParam struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"` // text, textarea, number, select, boolean, hidden, info
	Label       string         `json:"label"`
	Required    bool           `json:"required"`
	Default     any            `json:"default,omitempty"`
	Options     []string       `json:"options,omitempty"`
	Min         *float64       `json:"min,omitempty"`
	Max         *float64       `json:"max,omitempty"`
	Step        *float64       `json:"step,omitempty"`
	ShowWhen    map[string]any `json:"show_when,omitempty"`
	Description string         `json:"description,omitempty"`
	Entity      string         `json:"entity,omitempty"`  // "resource" | "note" | "group" — required when Type=="entity_ref"
	Multi       bool           `json:"multi,omitempty"`   // false → single ID; true → array of IDs
	Filters     *ActionFilter  `json:"filters,omitempty"` // nil = inherit action.Filters
}

// ActionFilter restricts which entities an action applies to.
type ActionFilter struct {
	ContentTypes []string `json:"content_types,omitempty"`
	CategoryIDs  []uint   `json:"category_ids,omitempty"`
	NoteTypeIDs  []uint   `json:"note_type_ids,omitempty"`
}

// ActionRegistration represents a plugin-contributed action.
type ActionRegistration struct {
	PluginName  string         `json:"plugin_name"`
	ID          string         `json:"id"`
	Label       string         `json:"label"`
	Description string         `json:"description,omitempty"`
	Icon        string         `json:"icon,omitempty"`
	Entity      string         `json:"entity"` // resource, note, group
	Placement   []string       `json:"placement"`
	Filters     ActionFilter   `json:"filters,omitempty"` // omitempty works because nil slices keep ActionFilter at zero value; avoid initializing empty slices
	Params      []ActionParam  `json:"params,omitempty"`
	Async       bool           `json:"async,omitempty"`
	Confirm     string         `json:"confirm,omitempty"`
	BulkMax     int            `json:"bulk_max,omitempty"`
	Handler     *lua.LFunction `json:"-"`
}

// parseActionTable parses a Lua table into an ActionRegistration.
// Required fields: id, label, entity (must be resource/note/group), handler (must be a function).
// Defaults placement to ["detail"] if empty.
func parseActionTable(L *lua.LState, tbl *lua.LTable, pluginName string) (*ActionRegistration, error) {
	a := &ActionRegistration{
		PluginName: pluginName,
	}

	// Required: id (normalized to lowercase to avoid case-sensitive collisions)
	if v := tbl.RawGetString("id"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'id'")
	} else {
		a.ID = strings.ToLower(v.String())
	}

	// Required: label
	if v := tbl.RawGetString("label"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'label'")
	} else {
		a.Label = v.String()
	}

	// Required: entity
	if v := tbl.RawGetString("entity"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'entity'")
	} else {
		entity := v.String()
		if entity != "resource" && entity != "note" && entity != "group" {
			return nil, fmt.Errorf("entity must be 'resource', 'note', or 'group', got %q", entity)
		}
		a.Entity = entity
	}

	// Required: handler (must be a function)
	if v := tbl.RawGetString("handler"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'handler'")
	} else if fn, ok := v.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("'handler' must be a function")
	} else {
		a.Handler = fn
	}

	// Optional: description
	if v := tbl.RawGetString("description"); v != lua.LNil {
		a.Description = v.String()
	}

	// Optional: icon
	if v := tbl.RawGetString("icon"); v != lua.LNil {
		a.Icon = v.String()
	}

	// Optional: async
	if v, ok := tbl.RawGetString("async").(lua.LBool); ok {
		a.Async = bool(v)
	}

	// Optional: confirm
	if v := tbl.RawGetString("confirm"); v != lua.LNil {
		a.Confirm = v.String()
	}

	// Optional: bulk_max
	if v, ok := tbl.RawGetString("bulk_max").(lua.LNumber); ok {
		a.BulkMax = int(v)
	}

	// Optional: placement (array of strings)
	if v := tbl.RawGetString("placement"); v != lua.LNil {
		if placeTbl, ok := v.(*lua.LTable); ok {
			placeTbl.ForEach(func(_, val lua.LValue) {
				if s, ok := val.(lua.LString); ok {
					a.Placement = append(a.Placement, string(s))
				}
			})
		}
	}
	if len(a.Placement) == 0 {
		a.Placement = []string{"detail"}
	}

	// Optional: filters
	if v := tbl.RawGetString("filters"); v != lua.LNil {
		if filtersTbl, ok := v.(*lua.LTable); ok {
			a.Filters = parseFiltersTable(filtersTbl)
		}
	}

	// Optional: params (array of tables)
	if v := tbl.RawGetString("params"); v != lua.LNil {
		if paramsTbl, ok := v.(*lua.LTable); ok {
			paramsTbl.ForEach(func(_, val lua.LValue) {
				if pTbl, ok := val.(*lua.LTable); ok {
					p := ActionParam{}

					if n := pTbl.RawGetString("name"); n != lua.LNil {
						p.Name = n.String()
					}
					if t := pTbl.RawGetString("type"); t != lua.LNil {
						p.Type = t.String()
					}
					if l := pTbl.RawGetString("label"); l != lua.LNil {
						p.Label = l.String()
					}
					if r, ok := pTbl.RawGetString("required").(lua.LBool); ok {
						p.Required = bool(r)
					}
					if d := pTbl.RawGetString("default"); d != lua.LNil {
						p.Default = luaValueToGo(d)
					}
					if o := pTbl.RawGetString("options"); o != lua.LNil {
						if oTbl, ok := o.(*lua.LTable); ok {
							oTbl.ForEach(func(_, v lua.LValue) {
								if s, ok := v.(lua.LString); ok {
									p.Options = append(p.Options, string(s))
								}
							})
						}
					}
					if m, ok := pTbl.RawGetString("min").(lua.LNumber); ok {
						f := float64(m)
						p.Min = &f
					}
					if m, ok := pTbl.RawGetString("max").(lua.LNumber); ok {
						f := float64(m)
						p.Max = &f
					}
					if s, ok := pTbl.RawGetString("step").(lua.LNumber); ok {
						f := float64(s)
						p.Step = &f
					}
					if sw := pTbl.RawGetString("show_when"); sw != lua.LNil {
						if swTbl, ok := sw.(*lua.LTable); ok {
							p.ShowWhen = luaTableToGoMap(swTbl)
						}
					}
					if d := pTbl.RawGetString("description"); d != lua.LNil {
						p.Description = d.String()
					}
					if e := pTbl.RawGetString("entity"); e != lua.LNil {
						p.Entity = e.String()
					}
					if m, ok := pTbl.RawGetString("multi").(lua.LBool); ok {
						p.Multi = bool(m)
					}
					if f := pTbl.RawGetString("filters"); f != lua.LNil {
						if fTbl, ok := f.(*lua.LTable); ok {
							af := parseFiltersTable(fTbl)
							p.Filters = &af
						}
					}

					a.Params = append(a.Params, p)
				}
			})
		}
	}

	// Validate entity_ref params.
	for i, p := range a.Params {
		if p.Type == "entity_ref" {
			if p.Entity == "" {
				return nil, fmt.Errorf("param %q: type 'entity_ref' requires 'entity' field", p.Name)
			}
			if p.Entity != "resource" && p.Entity != "note" && p.Entity != "group" {
				return nil, fmt.Errorf("param %q: entity must be 'resource', 'note', or 'group', got %q", p.Name, p.Entity)
			}
			// Default for `default` field is "trigger" when omitted.
			if p.Default == nil {
				a.Params[i].Default = "trigger"
			}
			// Validate `default` value.
			if d, ok := a.Params[i].Default.(string); ok {
				if d != "trigger" && d != "selection" && d != "both" && d != "" {
					return nil, fmt.Errorf("param %q: default must be 'trigger', 'selection', 'both', or '', got %q", p.Name, d)
				}
				if d == "both" && !p.Multi {
					return nil, fmt.Errorf("param %q: default 'both' requires multi=true", p.Name)
				}
			} else {
				return nil, fmt.Errorf("param %q: default must be a string for entity_ref", p.Name)
			}
		}
	}

	return a, nil
}

// parseFiltersTable parses a Lua table into an ActionFilter struct.
// Used for both action-level and per-param filter parsing.
func parseFiltersTable(tbl *lua.LTable) ActionFilter {
	var f ActionFilter
	// content_types
	if ct := tbl.RawGetString("content_types"); ct != lua.LNil {
		if ctTbl, ok := ct.(*lua.LTable); ok {
			ctTbl.ForEach(func(_, val lua.LValue) {
				if s, ok := val.(lua.LString); ok {
					f.ContentTypes = append(f.ContentTypes, string(s))
				}
			})
		}
	}
	// category_ids
	if ci := tbl.RawGetString("category_ids"); ci != lua.LNil {
		if ciTbl, ok := ci.(*lua.LTable); ok {
			ciTbl.ForEach(func(_, val lua.LValue) {
				if n, ok := val.(lua.LNumber); ok {
					f.CategoryIDs = append(f.CategoryIDs, uint(n))
				}
			})
		}
	}
	// note_type_ids
	if ni := tbl.RawGetString("note_type_ids"); ni != lua.LNil {
		if niTbl, ok := ni.(*lua.LTable); ok {
			niTbl.ForEach(func(_, val lua.LValue) {
				if n, ok := val.(lua.LNumber); ok {
					f.NoteTypeIDs = append(f.NoteTypeIDs, uint(n))
				}
			})
		}
	}
	return f
}

// GetActions returns all actions matching the given entity type.
// If entityData is non-nil, actions are further filtered by their filter criteria.
func (pm *PluginManager) GetActions(entity string, entityData map[string]any) []ActionRegistration {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var result []ActionRegistration
	for _, actions := range pm.actions {
		for _, a := range actions {
			if a.Entity != entity {
				continue
			}
			if entityData != nil && !actionMatchesFilters(a, entityData) {
				continue
			}
			result = append(result, a)
		}
	}
	return result
}

// GetActionsForPlacement returns actions matching entity, placement, and optional filters.
func (pm *PluginManager) GetActionsForPlacement(entity string, placement string, entityData map[string]any) []ActionRegistration {
	actions := pm.GetActions(entity, entityData)
	var result []ActionRegistration
	for _, a := range actions {
		for _, p := range a.Placement {
			if p == placement {
				result = append(result, a)
				break
			}
		}
	}
	return result
}

// actionMatchesFilters checks whether an action's filters match the given entity data.
// Empty filter fields match everything. entityData keys:
//   - "content_type" (string) — matched against Filters.ContentTypes
//   - "category_id" (uint) — matched against Filters.CategoryIDs
//   - "note_type_id" (uint) — matched against Filters.NoteTypeIDs
func actionMatchesFilters(a ActionRegistration, entityData map[string]any) bool {
	// Check content_type filter
	if len(a.Filters.ContentTypes) > 0 {
		ct, ok := entityData["content_type"].(string)
		if !ok || ct == "" {
			return false
		}
		found := false
		for _, allowed := range a.Filters.ContentTypes {
			if ct == allowed {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check category_id filter
	if len(a.Filters.CategoryIDs) > 0 {
		cid, ok := entityData["category_id"].(uint)
		if !ok {
			return false
		}
		found := false
		for _, allowed := range a.Filters.CategoryIDs {
			if cid == allowed {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check note_type_id filter
	if len(a.Filters.NoteTypeIDs) > 0 {
		nid, ok := entityData["note_type_id"].(uint)
		if !ok {
			return false
		}
		found := false
		for _, allowed := range a.Filters.NoteTypeIDs {
			if nid == allowed {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
