package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"

	"mahresources/application_context"
)

// AdminSettingsContextProvider builds the context for /admin/settings.
// Returns the settings grouped by SettingGroup (preserves the display order
// from RuntimeSettings.List), plus a read-only snapshot of boot-only settings
// for the reference section.
func AdminSettingsContextProvider(ctx *application_context.MahresourcesContext) func(r *http.Request) pongo2.Context {
	return func(r *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(r)

		views := ctx.Settings().List()
		groups := groupSettings(views)

		return pongo2.Context{
			"pageTitle":       "Settings",
			"hideSidebar":     true,
			"settingsByGroup": groups,
			"bootOnly":        bootOnlyFields(ctx.Config),
		}.Update(baseContext)
	}
}

// settingsGroupView clusters one display group of settings for the template.
type settingsGroupView struct {
	Group string
	Items []application_context.SettingView
}

// groupSettings clusters views by Group while preserving their order in the list.
// Relies on RuntimeSettings.List returning views ordered by (Group, Key).
func groupSettings(views []application_context.SettingView) []settingsGroupView {
	if len(views) == 0 {
		return nil
	}
	groups := make([]settingsGroupView, 0, 6)
	cur := settingsGroupView{Group: string(views[0].Group), Items: []application_context.SettingView{views[0]}}
	for _, v := range views[1:] {
		if string(v.Group) == cur.Group {
			cur.Items = append(cur.Items, v)
			continue
		}
		groups = append(groups, cur)
		cur = settingsGroupView{Group: string(v.Group), Items: []application_context.SettingView{v}}
	}
	groups = append(groups, cur)
	return groups
}

// bootOnlyField is a label/value pair for the "Requires restart" reference table.
type bootOnlyField struct {
	Label string
	Value string
}

// bootOnlyFields returns a read-only snapshot of restart-only settings, shown
// in the collapsible "Requires restart" reference section.
func bootOnlyFields(cfg *application_context.MahresourcesConfig) []bootOnlyField {
	if cfg == nil {
		return nil
	}
	return []bootOnlyField{
		{Label: "DB type", Value: cfg.DbType},
		{Label: "Bind address", Value: cfg.BindAddress},
		{Label: "File save path", Value: cfg.FileSavePath},
		{Label: "Ephemeral mode", Value: boolStr(cfg.EphemeralMode || cfg.MemoryDB || cfg.MemoryFS)},
		{Label: "Share port", Value: cfg.SharePort},
		{Label: "FTS enabled", Value: boolStr(!cfg.SkipFTS)},
		{Label: "Plugin path", Value: cfg.PluginPath},
	}
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
