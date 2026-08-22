package commands

import (
	"strings"

	"github.com/spf13/cobra"
)

// One table, three carriers. Registering the --custom-* flags from a shared
// definition is what keeps `mr category create`, `mr resource-category create`
// and `mr note-type create/update` in step: a slot added to the models shows up
// on every command whose carrier has it, and a slot whose render surface exists
// for one carrier only is never offered on the other two. Before this table the
// three commands each listed their flags by hand and had already drifted --
// CustomListHeader shipped with no flag on any of them.

type customSlotFlag struct {
	Flag  string // --custom-header
	Field string // CustomHeader, the API field name
	// Usage carries two placeholders rather than printf verbs, so a string that
	// needs neither is not a formatting bug waiting to print %!(EXTRA ...):
	// {member} is the entity the carrier governs ("group", "resource", "note"),
	// {carrier} is the carrier itself ("category", "resource category", "note type").
	Usage string
}

// customSlotOrder mirrors the order the slots appear on the edit forms.
var sharedCustomSlots = []customSlotFlag{
	{"custom-header", "CustomHeader", "Rendered at the top of the {member} detail page"},
	{"custom-detail-footer", "CustomDetailFooter", "Rendered at the bottom of the {member} detail page, below every built-in section"},
	{"custom-sidebar", "CustomSidebar", "Rendered in the {member} detail page sidebar"},
	{"custom-summary", "CustomSummary", "Rendered on {member} cards in list views, below the title"},
	{"custom-avatar", "CustomAvatar", "Replaces the default avatar on {member} cards"},
	{"custom-hover-card", "CustomHoverCard", "Rendered in the hover card for a {member} link; falls back to --custom-summary when unset"},
	{"custom-list-header", "CustomListHeader", "Rendered above {member} list pages filtered to exactly this {carrier}, against the {carrier} itself"},
	{"custom-list-footer", "CustomListFooter", "Rendered below {member} list pages filtered to exactly this {carrier}, against the {carrier} itself"},
	{"custom-mrql-result", "CustomMRQLResult", "Template for rendering {member}s of this {carrier} in MRQL results"},
	{"custom-css", "CustomCSS", "CSS injected as a <style> block on the {member} detail page and its list pages"},
}

// carrierOnlyCustomSlots hold slots whose render surface exists for a single
// carrier: there is no group or note table view, and no note lightbox.
var carrierOnlyCustomSlots = map[string][]customSlotFlag{
	"group": {
		{"custom-own-entities", "CustomOwnEntities", "Replaces the body of the group detail page's Own Entities section"},
	},
	"resource": {
		{"custom-preview", "CustomPreview", "Rendered above the built-in preview image, for file types it cannot show"},
		{"custom-lightbox", "CustomLightbox", "Rendered in the lightbox details panel; falls back to --custom-sidebar when unset"},
		{"custom-cell", "CustomCell", "Rendered as one extra cell per row in the resources details table"},
	},
	"note": nil,
}

// customSlotFlags holds the registered flag targets for one command.
type customSlotFlags struct {
	cmd    *cobra.Command
	slots  []customSlotFlag
	values []*string
}

// carrierNouns maps the member noun to the carrier's own name.
var carrierNouns = map[string]string{
	"group":    "category",
	"resource": "resource category",
	"note":     "note type",
}

// registerCustomSlotFlags declares every --custom-* flag the carrier has.
// member is the entity the carrier governs: "group", "resource" or "note".
func registerCustomSlotFlags(cmd *cobra.Command, member string) *customSlotFlags {
	slots := append(append([]customSlotFlag{}, sharedCustomSlots...), carrierOnlyCustomSlots[member]...)
	words := strings.NewReplacer("{member}", member, "{carrier}", carrierNouns[member])
	f := &customSlotFlags{cmd: cmd, slots: slots, values: make([]*string, len(slots))}
	for i, s := range slots {
		f.values[i] = cmd.Flags().String(s.Flag, "", words.Replace(s.Usage))
	}
	return f
}

// each yields the slots to copy. changedOnly picks the semantics: create
// commands take the non-empty flags, because an unset flag there means "leave it
// at the server default"; update commands take the explicitly-passed ones,
// because passing an empty string is how a slot is cleared and an absent flag
// must leave the stored value alone.
func (f *customSlotFlags) each(changedOnly bool, set func(field, value string)) {
	for i, s := range f.slots {
		if changedOnly {
			if f.cmd.Flags().Changed(s.Flag) {
				set(s.Field, *f.values[i])
			}
			continue
		}
		if *f.values[i] != "" {
			set(s.Field, *f.values[i])
		}
	}
}

// The four wrappers exist because the request bodies in this package are
// map[string]string in some commands and map[string]any in others.
func (f *customSlotFlags) applySet(body map[string]string) {
	f.each(false, func(k, v string) { body[k] = v })
}

func (f *customSlotFlags) applySetAny(body map[string]any) {
	f.each(false, func(k, v string) { body[k] = v })
}

func (f *customSlotFlags) applyChanged(body map[string]string) {
	f.each(true, func(k, v string) { body[k] = v })
}

func (f *customSlotFlags) applyChangedAny(body map[string]any) {
	f.each(true, func(k, v string) { body[k] = v })
}
