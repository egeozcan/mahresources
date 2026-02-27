package template_filters

import (
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
				if idField := v.FieldByName("ID"); idField.IsValid() {
					slotCtx["entity_id"] = float64(idField.Uint())
				}
			}
			break
		}
	}

	html := pm.RenderSlot(node.slotName, slotCtx)
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
