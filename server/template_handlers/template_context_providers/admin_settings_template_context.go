package template_context_providers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/flosch/pongo2/v4"

	"mahresources/application_context"
)

// AdminSettingsContextProvider builds the context for /admin/settings.
// Returns the settings grouped by SettingGroup (preserves the display order
// from RuntimeSettings.List), plus a read-only snapshot of boot-only settings
// for the reference section.
func AdminSettingsContextProvider(ctx AdminSettingsPageContext) func(r *http.Request) pongo2.Context {
	return func(r *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(r)

		views := ctx.Settings().List()
		groups := groupSettings(views)

		return pongo2.Context{
			"pageTitle":       "Settings",
			"hideSidebar":     true,
			"settingsByGroup": groups,
			"bootOnly":        bootOnlyFields(ctx.Configuration(), !ctx.ShareServerListening()),
		}.Update(baseContext)
	}
}

// ShortDuration renders a duration the way /admin/settings renders one.
//
// Finding 104: /admin/export printed `time.Duration.String()` — "24h0m0s" — for
// the very setting /admin/settings shows as "24h", because the settings page
// formats it in the browser (`nanosToShort` in templates/adminSettings.tpl) and
// the export page did not format it at all. This is the Go twin of that
// function, deliberately rule-for-rule: change one and change the other.
func ShortDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	ms := d.Milliseconds()
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	s := ms / 1000
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	if m < 60 {
		if rem := s % 60; rem != 0 {
			return fmt.Sprintf("%dm%ds", m, rem)
		}
		return fmt.Sprintf("%dm", m)
	}
	h := m / 60
	if rem := m % 60; rem != 0 {
		return fmt.Sprintf("%dh%dm", h, rem)
	}
	return fmt.Sprintf("%dh", h)
}

// settingsGroupView clusters one display group of settings for the template.
// Group stays the machine key — it is the section's `id` and what
// `aria-labelledby` points at — while Label is what a reader sees.
type settingsGroupView struct {
	Group string
	Label string
	Items []application_context.SettingView
}

// settingsGroupLabels gives each config group a human title. Finding 112: the
// template printed the raw key with `text-transform: capitalize`, which cannot
// reach an underscore, so the section above the remote-download settings read
// "Remote_downloads". A map rather than a string transform because "MRQL" and
// "Docs" are not what any general rule would produce from "mrql" and "docs".
var settingsGroupLabels = map[application_context.SettingGroup]string{
	application_context.GroupUploads:         "Uploads",
	application_context.GroupQueries:         "Queries",
	application_context.GroupRemoteDownloads: "Remote downloads",
	application_context.GroupSharing:         "Sharing",
	application_context.GroupDocs:            "Docs",
	application_context.GroupDeduplication:   "Deduplication",
	application_context.GroupExports:         "Exports",
}

// settingsGroupLabel falls back to the key itself, so a group added without a
// label still renders something rather than an empty heading.
func settingsGroupLabel(g application_context.SettingGroup) string {
	if label, ok := settingsGroupLabels[g]; ok {
		return label
	}
	return string(g)
}

// groupSettings clusters views by Group while preserving their order in the list.
// Relies on RuntimeSettings.List returning views ordered by (Group, Key).
func groupSettings(views []application_context.SettingView) []settingsGroupView {
	if len(views) == 0 {
		return nil
	}
	newGroup := func(v application_context.SettingView) settingsGroupView {
		return settingsGroupView{
			Group: string(v.Group),
			Label: settingsGroupLabel(v.Group),
			Items: []application_context.SettingView{v},
		}
	}
	groups := make([]settingsGroupView, 0, 6)
	cur := newGroup(views[0])
	for _, v := range views[1:] {
		if string(v.Group) == cur.Group {
			cur.Items = append(cur.Items, v)
			continue
		}
		groups = append(groups, cur)
		cur = newGroup(v)
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
func bootOnlyFields(cfg *application_context.MahresourcesConfig, shareFailed bool) []bootOnlyField {
	if cfg == nil {
		return nil
	}
	// Finding 51: this table said "Share port 8383" while nothing was listening on
	// 8383, because a bind failure was logged and dropped. The value alone cannot
	// tell an operator that, so the row says which it is.
	sharePort := cfg.SharePort
	switch {
	case sharePort == "":
		sharePort = "not configured"
	case shareFailed:
		sharePort += " (NOT serving — the share server failed to start)"
	}
	return []bootOnlyField{
		{Label: "DB type", Value: cfg.DbType},
		{Label: "Bind address", Value: cfg.BindAddress},
		{Label: "File save path", Value: cfg.FileSavePath},
		{Label: "Ephemeral mode", Value: boolStr(cfg.EphemeralMode || cfg.MemoryDB || cfg.MemoryFS)},
		{Label: "Share port", Value: sharePort},
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
