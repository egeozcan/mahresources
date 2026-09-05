package template_filters

import (
	"context"
	"fmt"
	"reflect"

	"github.com/flosch/pongo2/v4"
	"mahresources/plugin_system"
)

type pluginSlotNode struct {
	slotName string
}

func (node *pluginSlotNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	pmVal, ok := ctx.Public["_pluginManager"]
	if !ok || pmVal == nil {
		return nil
	}
	pm, ok := pmVal.(*plugin_system.PluginManager)
	if !ok || pm == nil {
		return nil
	}

	// Injections run plugin Lua against the unscoped DB handle, and six of these
	// slots live in the base layout, so they fire on every page a group-confined
	// principal is allowed to read. Skip them for such principals.
	//
	// Unlike the shortcode tags, this one has no request context of its own, so
	// it reads the one the enricher published. Absence fails closed: every site
	// that publishes _pluginManager publishes _requestContext beside it, so a
	// missing context means an unrecognised render path, not a legitimate one.
	var reqCtx context.Context
	if v, ok := ctx.Public["_requestContext"]; ok && v != nil {
		if rc, ok := v.(context.Context); ok {
			reqCtx = rc
		}
	}
	// Per-plugin now: an operator may mark individual plugins reachable by
	// group-limited users, and a slot renders several plugins' injections at
	// once, so the decision cannot be made for the request as a whole. Absence
	// of the predicate fails closed for the same reason the missing request
	// context does — it means an unrecognised render path.
	access := pluginAccessFromContext(ctx, reqCtx)

	slotCtx := make(map[string]any)
	if path, ok := ctx.Public["currentPath"].(string); ok {
		slotCtx["path"] = path
	}

	// Pass entity data for detail pages
	for _, key := range []string{"resource", "note", "group", "tag", "category"} {
		if entity, ok := ctx.Public[key]; ok && entity != nil {
			slotCtx["entity_key"] = key
			v := reflect.ValueOf(entity)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}
			if v.Kind() == reflect.Struct {
				if idField := v.FieldByName("ID"); idField.IsValid() && idField.Kind() == reflect.Uint {
					slotCtx["entity_id"] = float64(idField.Uint())
				}
				for _, taxonomy := range []string{"NoteType", "Category", "ResourceCategory"} {
					field := v.FieldByName(taxonomy + "Id")
					if field.IsValid() && field.Kind() == reflect.Ptr && !field.IsNil() {
						field = field.Elem()
					}
					if !field.IsValid() || field.Kind() != reflect.Uint {
						continue
					}
					slotCtx["entity_type_id"] = float64(field.Uint())
					association := v.FieldByName(taxonomy)
					if association.IsValid() && association.Kind() == reflect.Ptr && !association.IsNil() {
						association = association.Elem()
					}
					if association.IsValid() && association.Kind() == reflect.Struct {
						name := association.FieldByName("Name")
						if name.IsValid() && name.Kind() == reflect.String {
							slotCtx["entity_type_name"] = name.String()
						}
					}
					break
				}
			}
			break
		}
	}

	// The same context the gate above was decided from: an abandoned page
	// stops its injections instead of holding each plugin's VM lock, and
	// identical MRQL queries across slots collapse to one execution.
	html := pm.RenderSlot(reqCtx, node.slotName, slotCtx, access)
	if html != "" {
		if _, err := writer.WriteString(html); err != nil {
			return ctx.Error(fmt.Sprintf("plugin_slot: write error: %s", err), nil)
		}
	}
	return nil
}

func pluginSlotTagParser(doc *pongo2.Parser, start *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	slotNameToken := arguments.MatchType(pongo2.TokenString)
	if slotNameToken == nil {
		return nil, arguments.Error("plugin_slot tag requires a string argument", nil)
	}
	return &pluginSlotNode{slotName: slotNameToken.Val}, nil
}

func init() {
	if err := pongo2.RegisterTag("plugin_slot", pluginSlotTagParser); err != nil {
		fmt.Println("error when registering plugin_slot tag:", err)
	}
}
