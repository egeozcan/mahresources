package application_context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"mahresources/models"
)

// Sample budgets for the generation prompt.
const (
	// MaxTemplateSamples is how many members the handler samples.
	MaxTemplateSamples = 5
	// maxTemplateRawSamples is how many whole samples follow the summary.
	maxTemplateRawSamples = 3
	// templateSampleSummaryBudget and templateSampleBudget bound the summary
	// and the whole block, raw samples included.
	templateSampleSummaryBudget = 3 << 10
	templateSampleBudget        = 10 << 10
	// maxTemplateSampleMetaPaths caps the Meta paths the summary lists.
	maxTemplateSampleMetaPaths = 40

	templateSampleStringBytes      = 120
	templateSampleDescriptionBytes = 160
	templateSampleListItems        = 5
	templateSampleMaxDepth         = 4
	templateSampleTags             = 8
	templateSampleRelations        = 5
)

// TemplateSample is one member entity projected for the generation prompt.
// Keys are the entity's [property] names, plus Meta and a few counts.
type TemplateSample map[string]any

// ProjectTemplateSample projects an explicit allow-list of a member entity's
// fields. It never marshals the model, so fields that are not listed, the
// share token, file locations and the hash among them, cannot reach a prompt.
// full adds the relations, which only the first sample carries.
func ProjectTemplateSample(entity any, full bool) TemplateSample {
	s := TemplateSample{}
	switch e := entity.(type) {
	case *models.Group:
		s["Name"] = e.Name
		putSampleDescription(s, e.Description)
		s["HasURL"] = e.URL != nil
		putSampleOwner(s, e.Owner)
		putSampleTags(s, e.Tags)
		putSampleMeta(s, e.Meta)
		if full {
			putSampleRelations(s, e.Relationships)
		}
	case *models.Resource:
		s["Name"] = e.Name
		s["ContentType"] = e.ContentType
		s["Width"] = e.Width
		s["Height"] = e.Height
		s["FileSize"] = e.FileSize
		putSampleDescription(s, e.Description)
		putSampleTags(s, e.Tags)
		putSampleOwner(s, e.Owner)
		putSampleMeta(s, e.Meta)
		if full {
			names := make([]string, 0, len(e.Groups))
			for _, g := range sortedSampleGroups(e.Groups) {
				names = append(names, g.Name)
			}
			s["Groups"] = capSampleNames(names, templateSampleRelations, "groups")
			s["NoteCount"] = len(e.Notes)
		}
	case *models.Note:
		s["Name"] = e.Name
		putSampleDescription(s, e.Description)
		putSampleDate(s, "StartDate", e.StartDate)
		putSampleDate(s, "EndDate", e.EndDate)
		putSampleTags(s, e.Tags)
		putSampleOwner(s, e.Owner)
		putSampleMeta(s, e.Meta)
		if full {
			s["ResourceCount"] = len(e.Resources)
			s["GroupCount"] = len(e.Groups)
		}
	}
	return s
}

func putSampleDescription(s TemplateSample, description string) {
	if description != "" {
		s["Description"] = clipSampleString(description, templateSampleDescriptionBytes)
	}
}

func putSampleOwner(s TemplateSample, owner *models.Group) {
	if owner != nil {
		s["Owner"] = owner.Name
	}
}

func putSampleDate(s TemplateSample, key string, t *time.Time) {
	if t != nil {
		s[key] = t.UTC().Format(time.RFC3339)
	}
}

// putSampleTags lists tag names by name, then ID, so the projection does not
// depend on load order.
func putSampleTags(s TemplateSample, tags []*models.Tag) {
	if len(tags) == 0 {
		return
	}
	sorted := append([]*models.Tag(nil), tags...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].ID < sorted[j].ID
	})
	names := make([]string, 0, len(sorted))
	for _, tag := range sorted {
		names = append(names, tag.Name)
	}
	s["Tags"] = capSampleNames(names, templateSampleTags, "tags")
}

// putSampleRelations lists a group's outgoing relations, by type name, then
// target name, then ID.
func putSampleRelations(s TemplateSample, relations []*models.GroupRelation) {
	type relation struct {
		Type, Group string
		id          uint
	}
	var rows []relation
	for _, r := range relations {
		if r == nil || r.ToGroup == nil {
			continue
		}
		row := relation{Group: r.ToGroup.Name, id: r.ID}
		if r.RelationType != nil {
			row.Type = r.RelationType.Name
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Type != rows[j].Type {
			return rows[i].Type < rows[j].Type
		}
		if rows[i].Group != rows[j].Group {
			return rows[i].Group < rows[j].Group
		}
		return rows[i].id < rows[j].id
	})
	out := []map[string]string{}
	for i, row := range rows {
		if i == templateSampleRelations {
			break
		}
		out = append(out, map[string]string{"Type": row.Type, "Group": row.Group})
	}
	s["Relations"] = out
	if len(rows) > templateSampleRelations {
		s["RelationCount"] = len(rows)
	}
}

func sortedSampleGroups(groups []*models.Group) []*models.Group {
	sorted := append([]*models.Group(nil), groups...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}

// capSampleNames keeps the first n names and, past them, a count of all.
func capSampleNames(names []string, n int, noun string) []string {
	if len(names) <= n {
		return names
	}
	return append(names[:n:n], fmt.Sprintf("…(%d %s)", len(names), noun))
}

func putSampleMeta(s TemplateSample, raw []byte) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var meta any
	if err := decoder.Decode(&meta); err != nil {
		return
	}
	if object, ok := meta.(map[string]any); ok && len(object) > 0 {
		s["Meta"] = clipSampleValue(object, 1)
	}
}

// clipSampleValue shortens long strings and arrays and stops at
// templateSampleMaxDepth, so one entity cannot fill the prompt.
func clipSampleValue(v any, depth int) any {
	switch value := v.(type) {
	case string:
		return clipSampleString(value, templateSampleStringBytes)
	case map[string]any:
		if depth > templateSampleMaxDepth {
			return "…"
		}
		out := make(map[string]any, len(value))
		for k, child := range value {
			out[k] = clipSampleValue(child, depth+1)
		}
		return out
	case []any:
		if depth > templateSampleMaxDepth {
			return "…"
		}
		n := min(len(value), templateSampleListItems)
		out := make([]any, 0, n+1)
		for _, child := range value[:n] {
			out = append(out, clipSampleValue(child, depth+1))
		}
		if len(value) > templateSampleListItems {
			out = append(out, fmt.Sprintf("…(%d items)", len(value)))
		}
		return out
	default:
		return v
	}
}

// clipSampleString keeps at most limit bytes, cut on a rune boundary, and
// says how long the whole string was.
func clipSampleString(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("…(%d chars)", utf8.RuneCountInString(s))
}

// templateSampleStat is what the summary says about one field or Meta path.
type templateSampleStat struct {
	present int
	types   map[string]bool
	strings map[string]int
	enumOK  bool
}

// DescribeTemplateSamples is the samples block: a summary of every field and
// Meta path across the samples, then up to maxTemplateRawSamples whole
// samples, fenced together. It iterates in sorted key order, so unchanged
// samples always give the same bytes.
func DescribeTemplateSamples(samples []TemplateSample, target string) []string {
	if len(samples) == 0 {
		return nil
	}
	fields := map[string]*templateSampleStat{}
	metaPaths := map[string]*templateSampleStat{}
	for _, sample := range samples {
		for key, value := range sample {
			if key == "Meta" {
				meta, _ := value.(map[string]any)
				for metaKey, metaValue := range meta {
					observeSampleValue(metaPaths, "meta."+metaKey, metaValue)
					if nested, ok := metaValue.(map[string]any); ok {
						for nestedKey, nestedValue := range nested {
							observeSampleValue(metaPaths, "meta."+metaKey+"."+nestedKey, nestedValue)
						}
					}
				}
				continue
			}
			observeSampleValue(fields, key, value)
		}
	}

	var summary []string
	used := 0
	add := func(line string) bool {
		if used+len(line)+1 > templateSampleSummaryBudget {
			return false
		}
		summary = append(summary, line)
		used += len(line) + 1
		return true
	}
	add(fmt.Sprintf("Summary of %d sample(s): field (samples it is present in): types", len(samples)))
	for _, key := range sortedStatKeys(fields) {
		add(describeSampleStat(key, fields[key], len(samples)))
	}
	paths := sortedStatKeys(metaPaths)
	listed := 0
	for _, path := range paths {
		if listed == maxTemplateSampleMetaPaths || !add(describeSampleStat(path, metaPaths[path], len(samples))) {
			break
		}
		listed++
	}
	if listed < len(paths) {
		summary = append(summary, fmt.Sprintf("%d more Meta paths are not listed.", len(paths)-listed))
	}

	body := strings.Join(summary, "\n")
	raw := []string{}
	for i, sample := range samples {
		if i == maxTemplateRawSamples {
			break
		}
		encoded, err := json.Marshal(sample)
		if err != nil {
			continue
		}
		line := fmt.Sprintf("Sample %d: %s", i+1, encoded)
		if len(body)+len("\nRaw samples:")+len(strings.Join(raw, "\n"))+len(line)+2 > templateSampleBudget {
			break
		}
		raw = append(raw, line)
	}
	if len(raw) > 0 {
		body += "\nRaw samples:\n" + strings.Join(raw, "\n")
	}

	intro := "Here are real members of this category, summarised and then shown whole, for reference only: do not hard-code their values. Fields present in fewer than all samples are optional, so guard optional fields (for example with [conditional])."
	if target == TemplateTargetMetaSchema {
		intro += " Fields present in samples but missing from the schema are candidates to add."
	}
	return append([]string{intro}, referenceBlock("samples", body)...)
}

func observeSampleValue(stats map[string]*templateSampleStat, key string, value any) {
	stat := stats[key]
	if stat == nil {
		stat = &templateSampleStat{types: map[string]bool{}, strings: map[string]int{}, enumOK: true}
		stats[key] = stat
	}
	stat.present++
	stat.types[sampleValueType(value)] = true
	if s, ok := value.(string); ok && len(s) <= 40 {
		stat.strings[s]++
	} else {
		stat.enumOK = false
	}
}

func sampleValueType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number, int, int64, uint, float64:
		return "number"
	case map[string]any:
		return "object"
	default:
		// []any and []string, the projected tag and group lists among them.
		return "array"
	}
}

// describeSampleStat is one summary line. Short strings that repeat, with few
// distinct values, look like an enum and list the values seen.
func describeSampleStat(key string, stat *templateSampleStat, total int) string {
	types := make([]string, 0, len(stat.types))
	for t := range stat.types {
		types = append(types, t)
	}
	sort.Strings(types)
	line := fmt.Sprintf("- %s (%d of %d): %s", key, stat.present, total, strings.Join(types, "|"))
	if stat.enumOK && len(stat.strings) <= 6 && len(stat.strings) < stat.present {
		values := make([]string, 0, len(stat.strings))
		for v := range stat.strings {
			values = append(values, v)
		}
		sort.Slice(values, func(i, j int) bool {
			if stat.strings[values[i]] != stat.strings[values[j]] {
				return stat.strings[values[i]] > stat.strings[values[j]]
			}
			return values[i] < values[j]
		})
		seen := make([]string, 0, len(values))
		for _, v := range values {
			seen = append(seen, strconv.Quote(v)+"×"+strconv.Itoa(stat.strings[v]))
		}
		line += "; values seen: " + strings.Join(seen, ", ")
	}
	return line
}

func sortedStatKeys(stats map[string]*templateSampleStat) []string {
	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
