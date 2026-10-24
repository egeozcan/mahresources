package application_context

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"mahresources/shortcodes"
)

// Partial-block budgets for the generation prompt. Summaries (description and
// reads) share one budget; past it a partial is listed by name. The whole
// block, names included, has a hard cap, and past that the block says how many
// partials it left out, so the model is never told an incomplete list is
// complete.
const (
	templatePartialSummaryBudget = 16 << 10
	templatePartialBlockCap      = 24 << 10
	// templatePartialMaxNameBytes skips a partial whose name is longer: names
	// have no length bound, and one huge name could fill the block.
	templatePartialMaxNameBytes = 128
)

// TemplatePartialSummary describes one partial for the generation prompt.
// [partial] takes no parameters and renders against the including entity, so
// what a partial reads is its usage contract.
type TemplatePartialSummary struct {
	Name        string
	Description string
	// Reads is what the partial's content reads and uses, from
	// SummarizeTemplatePartial; "" when it uses no shortcode.
	Reads string
}

// SummarizeTemplatePartial summarises a partial's content: the path= values
// it reads per shortcode, the other shortcodes it uses, and the partials it
// includes, each in sorted order.
func SummarizeTemplatePartial(name, description, content string) TemplatePartialSummary {
	use := collectTemplateShortcodeUse(content)
	var parts []string
	shortcodeNames := make([]string, 0, len(use.paths))
	for sc := range use.paths {
		shortcodeNames = append(shortcodeNames, sc)
	}
	sort.Strings(shortcodeNames)
	for _, sc := range shortcodeNames {
		parts = append(parts, sc+" path="+strings.Join(sortedKeys(use.paths[sc]), ", "))
	}
	if len(use.others) > 0 {
		parts = append(parts, "uses "+strings.Join(sortedKeys(use.others), ", "))
	}
	if len(use.partials) > 0 {
		parts = append(parts, "includes partials "+strings.Join(sortedKeys(use.partials), ", "))
	}
	return TemplatePartialSummary{Name: name, Description: description, Reads: strings.Join(parts, "; ")}
}

// templateShortcodeUse is what a piece of template content reads and uses.
type templateShortcodeUse struct {
	paths    map[string]map[string]bool // shortcode name -> path= values
	others   map[string]bool            // shortcodes used without a path=
	partials map[string]bool            // [partial] names included
}

// collectTemplateShortcodeUse walks content's shortcodes, descending into
// block bodies, which ParseWithBlocks leaves as raw text.
func collectTemplateShortcodeUse(content string) templateShortcodeUse {
	use := templateShortcodeUse{paths: map[string]map[string]bool{}, others: map[string]bool{}, partials: map[string]bool{}}
	var walk func(string)
	walk = func(text string) {
		for _, sc := range shortcodes.ParseWithBlocks(text) {
			switch {
			case sc.Name == "partial":
				if name := sc.Attrs["name"]; name != "" {
					use.partials[name] = true
				}
			case sc.Attrs["path"] != "":
				if use.paths[sc.Name] == nil {
					use.paths[sc.Name] = map[string]bool{}
				}
				use.paths[sc.Name][sc.Attrs["path"]] = true
			default:
				use.others[sc.Name] = true
			}
			if sc.IsBlock {
				walk(sc.InnerContent)
			}
		}
	}
	walk(content)
	return use
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// collapseSpace puts free text on one line.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// partialLines lists the partials in name order, so the prefix does not depend
// on the order the catalogue was read in. total is how many partials exist,
// which is more than len(partials) when the handler capped the list.
func partialLines(partials []TemplatePartialSummary, total int) []string {
	if total < len(partials) {
		total = len(partials)
	}
	if total == 0 {
		return []string{"No template partials exist; do not use [partial]."}
	}
	sorted := append([]TemplatePartialSummary(nil), partials...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var entries []string
	blockBytes, summaryBytes := 0, 0
	for _, p := range sorted {
		if len(p.Name) > templatePartialMaxNameBytes {
			continue
		}
		entry := "- " + p.Name
		var summary []string
		if description := collapseSpace(p.Description); description != "" {
			summary = append(summary, description)
		}
		if p.Reads != "" {
			summary = append(summary, "(reads "+p.Reads+")")
		}
		if len(summary) > 0 {
			text := ": " + strings.Join(summary, " ")
			if summaryBytes+len(text) <= templatePartialSummaryBudget && blockBytes+len(entry)+len(text)+1 <= templatePartialBlockCap {
				entry += text
				summaryBytes += len(text)
			}
		}
		if blockBytes+len(entry)+1 > templatePartialBlockCap {
			break
		}
		entries = append(entries, entry)
		blockBytes += len(entry) + 1
	}
	rules := "[partial] takes no parameters: a partial renders against the entity that includes it, so one that reads [meta] renders empty in CustomListHeader and CustomListFooter, which bind the category."
	if omitted := total - len(entries); omitted > 0 {
		lines := append([]string{"Some of the template partials (not all are listed); use [partial name=\"...\"] only with a name listed in the partials block. " + rules}, referenceBlock("partials", strings.Join(entries, "\n"))...)
		return append(lines, fmt.Sprintf("%d more partials exist; use [partial] only with a name listed here.", omitted))
	}
	return append([]string{"Only these template partials exist; use [partial name=\"...\"] only with a name listed in the partials block. " + rules}, referenceBlock("partials", strings.Join(entries, "\n"))...)
}

// includedPartialLines names the partials the current content already
// includes. It is a tail line, so the prefix stays the same whatever the
// content is.
func includedPartialLines(in TemplateGenerationInput) []string {
	if strings.TrimSpace(in.CurrentContent) == "" {
		return nil
	}
	contents := []string{in.CurrentContent}
	// A bundle's current content is a JSON map of slot to content; read each
	// value, since the JSON escaping would hide the attributes' quotes.
	if in.Target == TemplateTargetBundle {
		var slots map[string]string
		if err := json.Unmarshal([]byte(in.CurrentContent), &slots); err == nil {
			contents = contents[:0]
			for _, content := range slots {
				contents = append(contents, content)
			}
		}
	}
	names := map[string]bool{}
	for _, content := range contents {
		for name := range collectTemplateShortcodeUse(content).partials {
			if len(name) <= templatePartialMaxNameBytes {
				names[name] = true
			}
		}
	}
	if len(names) == 0 {
		return nil
	}
	return append([]string{"This content already includes these partials:"}, referenceBlock("included partials", strings.Join(sortedKeys(names), ", "))...)
}
