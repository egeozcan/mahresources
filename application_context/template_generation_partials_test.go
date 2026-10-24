package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"mahresources/shortcodes"
)

func TestSummarizeTemplatePartialReadsThroughNesting(t *testing.T) {
	content := `<div>[property path="Name"]
[conditional path="status" eq="done"]
  [lazy][meta path="rating"] [mrql query="type = note" limit="3"][/lazy]
  [partial name="badge"]
[else][meta path="status"][/conditional]
[each path="links"][item path="url"][/each]
[partial name="avatar"]</div>`
	got := SummarizeTemplatePartial("card", "A card.", content)
	want := "conditional path=status; each path=links; meta path=rating, status; property path=Name; item path=url (inside each); uses lazy, mrql; includes partials avatar, badge"
	if got.Reads != want {
		t.Fatalf("reads:\n got %q\nwant %q", got.Reads, want)
	}
	if got.Name != "card" || got.Description != "A card." {
		t.Fatalf("summary %+v", got)
	}
	if empty := SummarizeTemplatePartial("plain", "", "<hr>"); empty.Reads != "" {
		t.Fatalf("a partial with no shortcode reads %q", empty.Reads)
	}
}

func TestPartialLinesListSummariesInNameOrder(t *testing.T) {
	lines := strings.Join(partialLines([]TemplatePartialSummary{
		{Name: "card", Description: "Shows a\ncard.", Reads: "meta path=rating"},
		{Name: "badge"},
	}, 2), "\n")
	if !strings.Contains(lines, "- badge\n- card: Shows a card. (reads meta path=rating)") {
		t.Fatalf("partials block:\n%s", lines)
	}
	for _, want := range []string{"Only these template partials exist", "takes no parameters", "CustomListHeader and CustomListFooter"} {
		if !strings.Contains(lines, want) {
			t.Errorf("block is missing %q", want)
		}
	}
	if none := strings.Join(partialLines(nil, 0), "\n"); !strings.Contains(none, "No template partials exist") {
		t.Fatalf("empty catalogue: %s", none)
	}
}

// TestPartialLinesBudgetKeepsEveryName fills the summary budget: every name
// is still listed, later ones without their summary, in name order.
func TestPartialLinesBudgetKeepsEveryName(t *testing.T) {
	var partials []TemplatePartialSummary
	for i := 0; i < 40; i++ {
		partials = append(partials, TemplatePartialSummary{Name: fmt.Sprintf("p%02d", i), Description: strings.Repeat("d", 1000)})
	}
	block := strings.Join(partialLines(partials, len(partials)), "\n")
	for _, p := range partials {
		if !strings.Contains(block, "- "+p.Name) {
			t.Fatalf("%s is missing", p.Name)
		}
	}
	if !strings.Contains(block, "- p00: ddd") || !strings.Contains(block, "- p39\n") {
		t.Fatalf("want the first summarised and the last name only:\n%.300s", block)
	}
	if n := strings.Count(block, strings.Repeat("d", 1000)); n == 0 || n > templatePartialSummaryBudget/1000 {
		t.Fatal("summaries exceed their budget")
	}
	if strings.Contains(block, "more partials exist") {
		t.Fatal("a complete list says partials are missing")
	}
}

// TestPartialLinesCapSaysTheListIsIncomplete covers names that alone pass the
// hard cap, a capped catalogue and an overlong name: the block never claims to
// be complete.
func TestPartialLinesCapSaysTheListIsIncomplete(t *testing.T) {
	var partials []TemplatePartialSummary
	for i := 0; i < 200; i++ {
		partials = append(partials, TemplatePartialSummary{Name: fmt.Sprintf("p%03d-%s", i, strings.Repeat("n", 120))})
	}
	partials = append(partials, TemplatePartialSummary{Name: strings.Repeat("x", templatePartialMaxNameBytes+1)})
	lines := partialLines(partials, 250)
	block := strings.Join(lines, "\n")
	if len(block) > templatePartialBlockCap {
		t.Fatalf("block is %d bytes", len(block))
	}
	listed := strings.Count(block, "\n- p")
	if listed == 0 || listed >= 200 {
		t.Fatalf("listed %d names", listed)
	}
	if want := fmt.Sprintf("%d more partials exist; use [partial] only with a name listed here.", 250-listed); lines[len(lines)-1] != want {
		t.Fatalf("last line %q, want %q", lines[len(lines)-1], want)
	}
	if strings.Contains(block, "Only these template partials exist") || strings.Contains(block, "xxxx") {
		t.Fatalf("an incomplete list claims completeness or keeps the overlong name")
	}
	if capped := strings.Join(partialLines([]TemplatePartialSummary{{Name: "card"}}, 3), "\n"); !strings.Contains(capped, "2 more partials exist") {
		t.Fatalf("a capped catalogue: %s", capped)
	}
}

func TestPromptNamesPartialsTheContentAlreadyIncludes(t *testing.T) {
	pair := slotInput()
	pair.Target, pair.Slot, pair.BundleSlots = TemplateTargetBundle, "", []string{"CustomHeader", "CustomHeaderCSS"}
	pair.CurrentContent = `{"CustomHeader":"[conditional path=\"x\"][partial name=\"badge\"][/conditional][partial name=\"avatar\"]","CustomHeaderCSS":""}`
	pair.Partials = []TemplatePartialSummary{{Name: "avatar"}, {Name: "badge"}, {Name: "card"}}
	_, user, _ := buildTemplateGenerationPrompt(pair, "make it")
	prefix := buildTemplateReferencePrefix(pair)
	tail := user[len(prefix):]
	if !strings.Contains(tail, "<<<BEGIN included partials>>>\navatar, badge\n<<<END included partials>>>") {
		t.Fatalf("tail does not name the included partials:\n%s", tail)
	}
	pair.CurrentContent = `{"CustomHeader":"<h1>x</h1>"}`
	if _, user, _ := buildTemplateGenerationPrompt(pair, "make it"); strings.Contains(user, "included partials") {
		t.Fatal("content with no partial still lists included partials")
	}
}

// TestSummarizeTemplatePartialReadsWhatTheRuntimeReads: the reads follow the
// runtime's own handling. Numbered conditions and [elseif] guards are read;
// an [item] is an item field only at the top of an [each] body (one nested in
// a block renders empty), and an [mrql] body renders against each result,
// not against the including entity.
func TestSummarizeTemplatePartialReadsWhatTheRuntimeReads(t *testing.T) {
	render := func(content, meta string) string {
		return shortcodes.Process(context.Background(), content, shortcodes.MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: json.RawMessage(meta)}, nil, nil)
	}
	// The runtime reads path2 and the [elseif] guard's own paths and fields.
	numbered := `[conditional path="status" eq="a" path2="secret" eq2="b"]X[elseif path="other" eq="c"]Y[/conditional]`
	if render(numbered, `{"status":"a","secret":"b"}`) != "X" || render(numbered, `{"status":"a","secret":"z"}`) != "" || render(numbered, `{"other":"c"}`) != "Y" {
		t.Fatal("the runtime no longer reads path2 and the [elseif] guard; update this test")
	}
	// An [item] inside a block within [each] renders empty.
	nested := `[each path="links"][conditional path="z" eq="y"][item path="url"][/conditional]|[item path="title"][/each]`
	if got := render(nested, `{"z":"y","links":[{"url":"U","title":"T"}]}`); strings.Contains(got, "U") || !strings.Contains(got, "T") {
		t.Fatalf("the runtime's [item] handling changed; update this test: %q", got)
	}

	content := numbered + `[conditional path="a" eq="1" field2="Name" eq2="x"]A[elseif path="b" eq="2" mrql2="type = note" gt2="0"]B[/conditional]` +
		nested + `[mrql query="type = note"][meta path="inresult"][/mrql]`
	got := SummarizeTemplatePartial("p", "", content).Reads
	want := "conditional path=a, b, other, secret, status, z; each path=links; conditional field=Name; item path=title (inside each); uses mrql"
	if got != want {
		t.Fatalf("reads:\n got %q\nwant %q", got, want)
	}
}

// TestPartialLinesKeepEveryNameThatFits: a long description never pushes a
// name out of the list while the names alone fit under the cap.
func TestPartialLinesKeepEveryNameThatFits(t *testing.T) {
	var partials []TemplatePartialSummary
	for i := 0; i < 200; i++ {
		partials = append(partials, TemplatePartialSummary{Name: fmt.Sprintf("p%03d-%s", i, strings.Repeat("n", 95))})
	}
	partials[0].Description = strings.Repeat("d", 16000)
	partials[1].Description = "short"
	block := strings.Join(partialLines(partials, len(partials)), "\n")
	for _, p := range partials {
		if !strings.Contains(block, "- "+p.Name) {
			t.Fatalf("%s is missing although every name fits", p.Name[:4])
		}
	}
	if len(block) > templatePartialBlockCap || strings.Contains(block, "more partials exist") {
		t.Fatalf("block is %d bytes", len(block))
	}
	if !strings.Contains(block, ": short") {
		t.Error("a summary that fits after the names was dropped")
	}
}
