package template_context_providers

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/listviews"
	"mahresources/plugin_system"
)

func bulkActionDeclarations(entity string, pluginValue *pongo2.Value) []listviews.BulkAction {
	plugins, _ := pluginValue.Interface().([]plugin_system.ActionRegistration)
	actions := listviews.BulkActions(entity)
	for i := range actions {
		if actions[i].Component != "" {
			actions[i].Component = "/partials/bulkActions/" + actions[i].Component + ".tpl"
		}
	}
	for _, plugin := range plugins {
		if plugin.Entity != entity {
			continue
		}
		actions = append(actions, listviews.BulkAction{
			ID: "plugin:" + plugin.PluginName + ":" + plugin.ID, Label: plugin.Label,
			Component: "/partials/bulkActions/plugin.tpl", Min: 1, Max: plugin.BulkMax, Plugin: plugin,
			Entities: []string{plugin.Entity},
			Filters:  listviews.BulkActionFilter{ContentTypes: plugin.Filters.ContentTypes, CategoryIDs: plugin.Filters.CategoryIDs, NoteTypeIDs: plugin.Filters.NoteTypeIDs},
		})
	}
	return actions
}
