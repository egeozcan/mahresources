package commands

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

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
	{"custom-header-css", "CustomHeaderCSS", "Global CSS for CustomHeader"},
	{"custom-detail-footer", "CustomDetailFooter", "Rendered at the bottom of the {member} detail page, below every built-in section"},
	{"custom-detail-footer-css", "CustomDetailFooterCSS", "Global CSS for CustomDetailFooter"},
	{"custom-sidebar", "CustomSidebar", "Rendered in the {member} detail page sidebar"},
	{"custom-sidebar-css", "CustomSidebarCSS", "Global CSS for CustomSidebar"},
	{"custom-summary", "CustomSummary", "Rendered on {member} cards in list views, below the title"},
	{"custom-summary-css", "CustomSummaryCSS", "Global CSS for CustomSummary"},
	{"custom-avatar", "CustomAvatar", "Replaces the default avatar on {member} cards"},
	{"custom-avatar-css", "CustomAvatarCSS", "Global CSS for CustomAvatar"},
	{"custom-hover-card", "CustomHoverCard", "Rendered in the hover card for a {member} link; falls back to --custom-summary when unset"},
	{"custom-hover-card-css", "CustomHoverCardCSS", "Global CSS for CustomHoverCard"},
	{"custom-list-header", "CustomListHeader", "Rendered above {member} list pages filtered to exactly this {carrier}, against the {carrier} itself"},
	{"custom-list-header-css", "CustomListHeaderCSS", "Global CSS for CustomListHeader"},
	{"custom-list-footer", "CustomListFooter", "Rendered below {member} list pages filtered to exactly this {carrier}, against the {carrier} itself"},
	{"custom-list-footer-css", "CustomListFooterCSS", "Global CSS for CustomListFooter"},
	{"custom-mrql-result", "CustomMRQLResult", "Template for rendering {member}s of this {carrier} in MRQL results"},
	{"custom-mrql-result-css", "CustomMRQLResultCSS", "Global CSS for CustomMRQLResult"},
	{"custom-entity-picker-result", "CustomEntityPickerResult", "Template for {member} content inside an entity picker result"},
	{"custom-entity-picker-result-css", "CustomEntityPickerResultCSS", "Global CSS for CustomEntityPickerResult"},
	{"custom-css", "CustomCSS", "CSS injected as a <style> block on the {member} detail page and its list pages"},
}

// carrierOnlyCustomSlots hold slots whose render surface exists for a single
// carrier: there is no group or note table view, and no note lightbox.
var carrierOnlyCustomSlots = map[string][]customSlotFlag{
	"group": {
		{"custom-own-entities", "CustomOwnEntities", "Replaces the body of the group detail page's Own Entities section"},
		{"custom-own-entities-css", "CustomOwnEntitiesCSS", "Global CSS for CustomOwnEntities"},
	},
	"resource": {
		{"custom-preview", "CustomPreview", "Rendered above the built-in preview image, for file types it cannot show"},
		{"custom-preview-css", "CustomPreviewCSS", "Global CSS for CustomPreview"},
		{"custom-lightbox", "CustomLightbox", "Rendered in the lightbox details panel; falls back to --custom-sidebar when unset"},
		{"custom-lightbox-css", "CustomLightboxCSS", "Global CSS for CustomLightbox"},
		{"custom-cell", "CustomCell", "Rendered as one extra cell per row in the resources details table"},
		{"custom-cell-css", "CustomCellCSS", "Global CSS for CustomCell"},
	},
	"note": nil,
}

// customSlotFlags holds the registered flag targets for one command.
type customSlotFlags struct {
	cmd    *cobra.Command
	slots  []customSlotFlag
	values []*string
	files  map[int]*string
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
	f := &customSlotFlags{cmd: cmd, slots: slots, values: make([]*string, len(slots)), files: make(map[int]*string)}
	for i, s := range slots {
		f.values[i] = cmd.Flags().String(s.Flag, "", words.Replace(s.Usage))
		if s.Field == "CustomEntityPickerResult" || s.Field == "CustomEntityPickerResultCSS" {
			f.files[i] = cmd.Flags().String(s.Flag+"-file", "", "Read --"+s.Flag+" from a UTF-8 file (mutually exclusive with the inline flag)")
			cmd.MarkFlagsMutuallyExclusive(s.Flag, s.Flag+"-file")
		}
	}
	return f
}

// each yields the slots to copy. changedOnly picks the semantics: create
// commands take the non-empty flags, because an unset flag there means "leave it
// at the server default"; update commands take the explicitly-passed ones,
// because passing an empty string is how a slot is cleared and an absent flag
// must leave the stored value alone.
func (f *customSlotFlags) each(changedOnly bool, set func(field, value string)) error {
	for i, s := range f.slots {
		if file, ok := f.files[i]; ok && f.cmd.Flags().Changed(s.Flag+"-file") {
			data, err := os.ReadFile(*file)
			if err != nil {
				return fmt.Errorf("read --%s-file: %w", s.Flag, err)
			}
			if !utf8.Valid(data) {
				return fmt.Errorf("--%s-file must contain UTF-8 text", s.Flag)
			}
			set(s.Field, string(data))
			continue
		}
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
	return nil
}

// Creates omit unset flags; partial edits include explicitly empty values.
func (f *customSlotFlags) applySet(body map[string]string) error {
	return f.each(false, func(k, v string) { body[k] = v })
}

func (f *customSlotFlags) applyChangedAny(body map[string]any) error {
	return f.each(true, func(k, v string) { body[k] = v })
}
