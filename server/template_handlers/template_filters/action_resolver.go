package template_filters

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/plugin_system"
	"mahresources/shortcodes"
)

// ActionEntityData extracts the fields an action's filters match on
// (content_type, category_id, note_type_id) from a loaded entity. The detail
// page's action menu and the [action] shortcode both read it, so a button and
// the menu entry beside it cannot disagree about an entity. Pointer and value
// forms are accepted, because a template may be handed either. An entity it
// does not recognise yields an empty map, which fails every declared filter.
func ActionEntityData(entity any, entityType string) map[string]any {
	data := map[string]any{}
	switch entityType {
	case "resource":
		var r *models.Resource
		switch e := entity.(type) {
		case *models.Resource:
			r = e
		case models.Resource:
			r = &e
		}
		if r != nil {
			data["content_type"] = r.ContentType
			// A resource's category lives under "category_id", the key the
			// filter predicate reads for every entity type. Without it an
			// action filtered on category_ids was never offered on a resource
			// detail page. The column is NOT NULL, so only a zero-valued
			// struct lands here with no category — and 0 is not a real id.
			if r.ResourceCategoryId != 0 {
				data["category_id"] = r.ResourceCategoryId
			}
		}
	case "group":
		var g *models.Group
		switch e := entity.(type) {
		case *models.Group:
			g = e
		case models.Group:
			g = &e
		}
		if g != nil && g.CategoryId != nil {
			data["category_id"] = *g.CategoryId
		}
	case "note":
		var n *models.Note
		switch e := entity.(type) {
		case *models.Note:
			n = e
		case models.Note:
			n = &e
		}
		if n != nil && n.NoteTypeId != nil {
			data["note_type_id"] = *n.NoteTypeId
		}
	}
	return data
}

// BuildActionResolver returns the [action] resolver for one render. access is
// the run endpoint's own rule for this request (auth.PluginActionAccessFor, or
// the same predicate built from the page's _pluginAccess), so a button is drawn
// only where the click would pass the endpoint's plugin check.
//
// It reads nothing from the database: FindAction is an in-memory lookup that
// takes and releases the registry's read lock on its own (the resolver holds
// no other plugin-manager lock while it runs), the filter data comes from
// columns already on the loaded entity, and param validation is pure. The DB
// checks (entity_ref targets, filters against the stored row, subtree scope)
// run on the click, in the run endpoint.
func BuildActionResolver(pm *plugin_system.PluginManager, access auth.PluginAccess) shortcodes.ActionResolver {
	return func(plugin, name string, attrs map[string]string, ctx shortcodes.MetaShortcodeContext) (shortcodes.ActionButton, error) {
		if pm == nil || access == nil || !access(plugin) {
			return shortcodes.ActionButton{}, shortcodes.ErrActionUnavailable
		}
		action, _, err := pm.FindAction(plugin, name)
		if err != nil {
			return shortcodes.ActionButton{}, shortcodes.ErrActionUnavailable
		}
		// Carrier contexts (category, resource_category, note_type) never match
		// an action's entity, so list headers get no button.
		if action.Entity != ctx.EntityType || ctx.EntityID == 0 {
			return shortcodes.ActionButton{}, shortcodes.ErrActionUnavailable
		}
		var entityData map[string]any
		if plugin_system.ActionHasEntityFilters(action) {
			entityData = ActionEntityData(ctx.Entity, ctx.EntityType)
			if !plugin_system.ActionMatchesEntityData(action, entityData) {
				return shortcodes.ActionButton{}, shortcodes.ErrActionUnavailable
			}
		}
		params, err := actionTemplateParams(action, attrs, ctx.EntityID)
		if err != nil {
			return shortcodes.ActionButton{}, err
		}
		for _, p := range action.Params {
			// Without an override the trigger ref inherits the action filter,
			// which the same entity has already passed above.
			if p.Filters == nil || !actionParamUsesTrigger(p, action.Entity) {
				continue
			}
			if _, visible := params[p.Name]; !visible {
				continue
			}
			if entityData == nil {
				entityData = ActionEntityData(ctx.Entity, ctx.EntityType)
			}
			if !actionTriggerMatchesParamFilter(p, entityData) {
				return shortcodes.ActionButton{}, shortcodes.ErrActionUnavailable
			}
		}
		if err := validateActionTemplateParams(action, params); err != nil {
			return shortcodes.ActionButton{}, err
		}
		return shortcodes.ActionButton{Label: action.Label, Confirm: action.Confirm, Async: action.Async, Params: params}, nil
	}
}

func actionParamUsesTrigger(p plugin_system.ActionParam, entityType string) bool {
	def, _ := p.Default.(string)
	return p.Type == "entity_ref" && p.Entity == entityType && (def == "trigger" || def == "both")
}

func actionTriggerMatchesParamFilter(p plugin_system.ActionParam, entityData map[string]any) bool {
	// EntityRefReader checks only the fields that belong to the reference's
	// entity type, unlike the action's primary filter predicate.
	var filter plugin_system.ActionFilter
	switch p.Entity {
	case "resource":
		filter.ContentTypes = p.Filters.ContentTypes
		filter.CategoryIDs = p.Filters.CategoryIDs
	case "group":
		filter.CategoryIDs = p.Filters.CategoryIDs
	case "note":
		filter.NoteTypeIDs = p.Filters.NoteTypeIDs
	}
	return plugin_system.ActionMatchesEntityData(plugin_system.ActionRegistration{Filters: filter}, entityData)
}

// actionTemplateParams resolves defaults the way the action modal does, lays
// the template's param-<name> values on top, and strips hidden params. The
// visible trigger references are checked before validating the final values.
func actionTemplateParams(action plugin_system.ActionRegistration, attrs map[string]string, entityID uint) (map[string]any, error) {
	declared := make(map[string]plugin_system.ActionParam, len(action.Params))
	params := make(map[string]any, len(action.Params))
	for _, p := range action.Params {
		declared[p.Name] = p
		switch p.Type {
		case "info":
			// Help text with no value.
		case "entity_ref":
			var ids []any
			if actionParamUsesTrigger(p, action.Entity) {
				ids = append(ids, float64(entityID))
			}
			switch {
			case p.Multi && ids == nil:
				params[p.Name] = []any{}
			case p.Multi:
				params[p.Name] = ids
			case ids == nil:
				params[p.Name] = nil
			default:
				params[p.Name] = ids[0]
			}
		default:
			switch {
			case p.Default != nil:
				params[p.Name] = p.Default
			case p.Type == "boolean":
				params[p.Name] = false
			default:
				params[p.Name] = ""
			}
		}
	}

	// Sorted so the first error reported for a template is always the same one.
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p, ok := declared[name]
		if !ok {
			return nil, fmt.Errorf("%s: no such parameter", name)
		}
		value, err := coerceActionParam(p, attrs[name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		params[name] = value
	}

	plugin_system.StripHiddenParams(action, params)
	return params, nil
}

func validateActionTemplateParams(action plugin_system.ActionRegistration, params map[string]any) error {
	for _, p := range action.Params {
		def, _ := p.Default.(string)
		if _, visible := params[p.Name]; visible && p.Type == "entity_ref" && def == "selection" && p.Required {
			return fmt.Errorf("%s: needs a bulk selection and cannot run from a template", p.Name)
		}
	}
	if errs := plugin_system.ValidateActionParams(action, params); len(errs) > 0 {
		return fmt.Errorf("%s: %s", errs[0].Field, errs[0].Message)
	}
	return nil
}

// coerceActionParam turns one literal attribute value into the JSON type the
// run endpoint expects for p. Select values are checked against the options
// by ValidateActionParams afterwards.
func coerceActionParam(p plugin_system.ActionParam, raw string) (any, error) {
	switch p.Type {
	case "entity_ref", "info":
		return nil, errors.New("cannot be set from a template")
	case "number":
		f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, errors.New("must be a number")
		}
		return f, nil
	case "boolean":
		switch strings.TrimSpace(raw) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, errors.New("must be true or false")
	default:
		// text, textarea, hidden and select take the string as written.
		return raw, nil
	}
}
