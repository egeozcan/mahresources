package application_context

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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

// SummarizeTemplatePartial summarises what a partial's content reads from the
// entity that includes it, as the runtime reads it: the path= values per
// shortcode, the fields a [conditional] tests, the item fields an [each] body
// reads, the other shortcodes it uses and the partials it includes, each in
// sorted order.
func SummarizeTemplatePartial(name, description, content string) TemplatePartialSummary {
	use := collectTemplateShortcodeUse(content)
	var parts []string
	for _, sc := range sortedKeys(keySet(use.paths)) {
		parts = append(parts, sc+" path="+strings.Join(sortedKeys(use.paths[sc]), ", "))
	}
	if len(use.fields) > 0 {
		parts = append(parts, "conditional field="+strings.Join(sortedKeys(use.fields), ", "))
	}
	if len(use.itemPaths) > 0 {
		parts = append(parts, "item path="+strings.Join(sortedKeys(use.itemPaths), ", ")+" (inside each)")
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
	paths     map[string]map[string]bool // shortcode name -> entity path= values
	fields    map[string]bool            // entity fields a [conditional] tests
	itemPaths map[string]bool            // [item] paths at the top of an [each] body
	others    map[string]bool            // shortcodes used without reading a path
	partials  map[string]bool            // [partial] names included
}

func (u templateShortcodeUse) addPath(shortcode, path string) {
	if u.paths[shortcode] == nil {
		u.paths[shortcode] = map[string]bool{}
	}
	u.paths[shortcode][path] = true
}

// collectTemplateShortcodeUse walks content the way rendering does
// (ParseWithBlocks leaves block bodies as raw text):
//   - a [conditional]'s condition sets are read like evaluateConditionSet
//     reads them, numbered conditions included, for the opening tag and every
//     [elseif] guard, and every branch body is walked;
//   - an [each] body's top-level [item] tokens read the element, not the
//     entity (markItems); an [item] anywhere else renders empty;
//   - an [mrql] body's per-result template renders against each result, so
//     only its [header], [footer] and [else] are walked;
//   - other block bodies render against the entity and are walked.
func collectTemplateShortcodeUse(content string) templateShortcodeUse {
	use := templateShortcodeUse{paths: map[string]map[string]bool{}, fields: map[string]bool{}, itemPaths: map[string]bool{}, others: map[string]bool{}, partials: map[string]bool{}}
	conditions := func(attrs map[string]string) {
		for i := 1; ; i++ {
			suffix := ""
			if i >= 2 {
				suffix = strconv.Itoa(i)
				if attrs["mrql"+suffix] == "" && attrs["field"+suffix] == "" && attrs["path"+suffix] == "" {
					return
				}
			}
			// The source the runtime uses: mrql, then field, then path.
			switch {
			case attrs["mrql"+suffix] != "":
				use.others["mrql"] = true
			case attrs["field"+suffix] != "":
				use.fields[attrs["field"+suffix]] = true
			case attrs["path"+suffix] != "":
				use.addPath("conditional", attrs["path"+suffix])
			}
		}
	}
	var walk func(text string, eachBody bool)
	walk = func(text string, eachBody bool) {
		for _, sc := range shortcodes.ParseWithBlocks(text) {
			switch sc.Name {
			case "partial":
				if name := sc.Attrs["name"]; name != "" {
					use.partials[name] = true
				}
			case "item":
				if eachBody && sc.Attrs["path"] != "" {
					use.itemPaths[sc.Attrs["path"]] = true
				}
			case "conditional":
				conditions(sc.Attrs)
				for i, branch := range shortcodes.SplitBranches(sc.InnerContent) {
					if i > 0 && !branch.IsElse {
						conditions(branch.Attrs)
					}
					walk(branch.Content, false)
				}
			case "each":
				if sc.Attrs["path"] != "" {
					use.addPath("each", sc.Attrs["path"])
				}
				itemBranch, elseBranch := shortcodes.SplitElse(sc.InnerContent)
				walk(itemBranch, true)
				walk(elseBranch, false)
			case "mrql":
				// The per-result template renders against each result; the
				// header, footer and [else] render against this entity.
				use.others["mrql"] = true
				if sc.IsBlock {
					for _, slot := range shortcodes.MRQLParentBoundSlots(sc.InnerContent) {
						walk(slot, false)
					}
				}
			default:
				if path := sc.Attrs["path"]; path != "" {
					use.addPath(sc.Name, path)
				} else {
					use.others[sc.Name] = true
				}
				if sc.IsBlock {
					walk(sc.InnerContent, false)
				}
			}
		}
	}
	walk(content, false)
	return use
}

func keySet[V any](m map[string]V) map[string]bool {
	set := make(map[string]bool, len(m))
	for k := range m {
		set[k] = true
	}
	return set
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
// which is more than len(partials) when the handler capped the list. Every
// name that fits under the block's cap is listed first; the summary budget is
// then spent, in name order, on the room left, so a long description never
// pushes a name out.
func partialLines(partials []TemplatePartialSummary, total int) []string {
	if total < len(partials) {
		total = len(partials)
	}
	if total == 0 {
		return []string{"No template partials exist; do not use [partial]."}
	}
	sorted := append([]TemplatePartialSummary(nil), partials...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	var named []TemplatePartialSummary
	var entries []string
	for _, p := range sorted {
		if len(p.Name) <= templatePartialMaxNameBytes {
			named = append(named, p)
			entries = append(entries, "- "+p.Name)
		}
	}
	// Partials skipped or never loaded are left out as surely as those the cap
	// drops.
	unlisted := total - len(entries)
	build := func(kept []string, omitted int) []string {
		return partialBlock(kept, unlisted+omitted)
	}
	_, indexes := fitSection(entries, templatePartialBlockCap, build)
	kept := make([]string, 0, len(indexes))
	keptPartials := make([]TemplatePartialSummary, 0, len(indexes))
	for _, i := range indexes {
		kept = append(kept, entries[i])
		keptPartials = append(keptPartials, named[i])
	}
	omitted := len(entries) - len(kept)
	spent := 0
	for i, p := range keptPartials {
		var summary []string
		if description := collapseSpace(p.Description); description != "" {
			summary = append(summary, description)
		}
		if p.Reads != "" {
			summary = append(summary, "(reads "+p.Reads+")")
		}
		if len(summary) == 0 {
			continue
		}
		text := ": " + strings.Join(summary, " ")
		if spent+len(text) > templatePartialSummaryBudget {
			continue
		}
		candidate := append([]string(nil), kept...)
		candidate[i] += text
		if sectionBytes(build(candidate, omitted)) <= templatePartialBlockCap {
			kept = candidate
			spent += len(text)
		}
	}
	return build(kept, omitted)
}

// partialBlock is the partials section for the listed entries, omitted
// naming how many partials exist but are not listed.
func partialBlock(entries []string, omitted int) []string {
	rules := "[partial] takes no parameters: a partial renders against the entity that includes it, so one that reads [meta] renders empty in CustomListHeader and CustomListFooter, which bind the category."
	if omitted > 0 {
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
