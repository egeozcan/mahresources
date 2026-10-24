package templateeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mahresources/shortcodes"
)

func reportWith(results ...CaseResult) RunReport {
	r := RunReport{SchemaVersion: ReportSchemaVersion, CorpusVersion: CorpusVersion, FixtureChecksum: FixtureChecksum(), Results: results}
	r.Summary = Summarize(r.Results)
	return r
}

// result builds a run whose checks agree with pass, as Compare requires.
func result(id string, kind Kind, carrier string, pass bool) CaseResult {
	checks := Checks{Kind: kind, HTTPStatus: 200, SchemaProperties: 1}
	if !pass {
		checks.HTTPStatus = 502
	}
	return CaseResult{Case: id, Kind: kind, Carrier: carrier, Pass: pass, Checks: checks}
}

func TestCompareRefusesIncompatibleReports(t *testing.T) {
	base := reportWith(result("a", KindPair, "group", true))
	for name, mutate := range map[string]func(*RunReport){
		"corpus":  func(r *RunReport) { r.CorpusVersion = "other" },
		"fixture": func(r *RunReport) { r.FixtureChecksum = "other" },
		"schema":  func(r *RunReport) { r.SchemaVersion = ReportSchemaVersion + 1 },
		"cases":   func(r *RunReport) { r.Results = append(r.Results, result("b", KindPair, "group", true)) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := reportWith(base.Results...)
			mutate(&candidate)
			got := Compare(base, candidate)
			if got.Compatible || len(got.Errors) == 0 {
				t.Fatalf("incompatible %s accepted: %#v", name, got)
			}
		})
	}
}

func TestCompareFlagsKindRegressionsOverTenPoints(t *testing.T) {
	var baseResults, candResults []CaseResult
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		baseResults = append(baseResults, result(id, KindPair, "group", true), result(id+"m", KindMetaSchema, "group", true))
		// Pair drops 20 points, metaschema 10: only the first is a regression.
		candResults = append(candResults, result(id, KindPair, "group", i >= 2), result(id+"m", KindMetaSchema, "group", i >= 1))
	}
	got := Compare(reportWith(baseResults...), reportWith(candResults...))
	if !got.Compatible {
		t.Fatalf("compatible reports refused: %v", got.Errors)
	}
	if len(got.Regressions) != 1 || !strings.Contains(got.Regressions[0], "pair") {
		t.Fatalf("regressions = %v", got.Regressions)
	}
	if !strings.Contains(FormatComparison(got), "pair") {
		t.Fatalf("comparison text lacks the pair row:\n%s", FormatComparison(got))
	}
}

func TestSummarizeGroupsByKindAndCarrierWithCacheRatio(t *testing.T) {
	r := result("a", KindPair, "group", true)
	r.Calls = []Call{{Usage: Usage{Prompt: 100, CacheHit: 75, CacheMiss: 25}, LatencyMS: 10}}
	s := Summarize([]CaseResult{r, result("b", KindPair, "note", false)})
	all := s.Group("all")
	pair := s.Group("pair")
	pairGroup := s.Group("pair/group")
	if all == nil || pair == nil || pairGroup == nil {
		t.Fatalf("missing groups: %#v", s)
	}
	if pair.Runs != 2 || pair.Passed != 1 || pair.PassRate != 0.5 {
		t.Fatalf("pair summary = %#v", pair)
	}
	if pairGroup.CacheHitRatio != 0.75 {
		t.Fatalf("cache ratio = %v", pairGroup.CacheHitRatio)
	}
	if !strings.Contains(FormatSummary(s), "pair/group") {
		t.Fatalf("summary text:\n%s", FormatSummary(s))
	}
}

func TestSummarizeCountsRepairs(t *testing.T) {
	applied := result("a", KindPair, "group", true)
	applied.Checks.RepairAttempted, applied.Checks.Repaired = true, true
	rejected := result("b", KindPair, "group", true)
	rejected.Checks.RepairAttempted = true
	s := Summarize([]CaseResult{applied, rejected, result("c", KindPair, "group", true), result("d", KindPair, "group", true)})
	pair := s.Group("pair")
	if pair.RepairAttemptedRate != 0.5 || pair.RepairedRate != 0.25 {
		t.Fatalf("repair rates = %v / %v", pair.RepairAttemptedRate, pair.RepairedRate)
	}
	if !strings.Contains(FormatSummary(s), "repaired") {
		t.Fatalf("summary text lacks the repair column:\n%s", FormatSummary(s))
	}
}

func TestAggregateOnlyDropsDraftsAndRawContent(t *testing.T) {
	r := result("a", KindPair, "group", true)
	r.Draft = &Draft{Content: "secret draft"}
	r.Previews = map[string]Preview{"CustomHeader": {HTML: "x"}}
	r.Details = &Details{ProviderError: "x"}
	r.Calls = []Call{{Content: "raw", Usage: Usage{Prompt: 3}}}
	report := reportWith(r)
	got := AggregateOnly(report)
	if got.Results[0].Draft != nil || got.Results[0].Previews != nil || got.Results[0].Details != nil || got.Results[0].Calls[0].Content != "" {
		t.Fatalf("aggregate kept per-run detail: %#v", got.Results[0])
	}
	if got.Results[0].Calls[0].Usage.Prompt != 3 || report.Results[0].Calls[0].Content != "raw" || report.Results[0].Draft == nil {
		t.Fatal("aggregate lost usage or mutated its input")
	}
}

func TestWriteAndReadReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "run.json")
	r := result("a", KindPair, "group", true)
	r.Calls = []Call{{Usage: Usage{CacheHit: 1, CacheMiss: 2}, LatencyMS: 7}}
	report := reportWith(r, result("b", KindSlot, "note", false), result("b", KindSlot, "note", true))
	if err := WriteJSONAtomic(path, report); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.FixtureChecksum != report.FixtureChecksum || len(loaded.Results) != 3 {
		t.Fatalf("loaded %#v", loaded)
	}
	// The stored summary must survive the JSON round trip exactly, or Compare
	// would refuse every report read from disk.
	if got := Compare(loaded, loaded); !got.Compatible {
		t.Fatalf("a report read back from disk cannot be compared: %v", got.Errors)
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".run.json-*.tmp")); len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
	if err := os.WriteFile(path, []byte(`{"schemaVersion":99}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(path); err == nil {
		t.Fatal("an unknown schema version was read")
	}
}

func TestCorpusShape(t *testing.T) {
	cases := Corpus()
	seen := map[string]bool{}
	adversarial := 0
	for _, c := range cases {
		if seen[c.ID] {
			t.Fatalf("duplicate case id %q", c.ID)
		}
		seen[c.ID] = true
		if c.Adversarial {
			adversarial++
		}
		if _, ok := FixtureForCase(c); !ok {
			t.Fatalf("case %q has no fixture", c.ID)
		}
		if c.Kind == KindMetaSchema && len(c.ExpectFields) == 0 {
			t.Fatalf("metaschema case %q has no ExpectFields, so {} could pass", c.ID)
		}
		if c.Kind == KindBundle && len(c.ExpectSlots) == 0 {
			t.Fatalf("bundle case %q has no ExpectSlots, so an empty bundle could pass", c.ID)
		}
		if c.Kind == KindPair && !containsString(BundleSlots(c.Carrier), c.Slot+"CSS") {
			t.Fatalf("pair case %q names a slot %q with no companion on %s", c.ID, c.Slot, c.Carrier)
		}
	}
	if len(cases) < 28 || adversarial < 2 {
		t.Fatalf("corpus has %d cases, %d adversarial", len(cases), adversarial)
	}
	if _, err := FilterCases(cases, []string{"nope"}); err == nil {
		t.Fatal("an unknown case id was accepted")
	}
}

func TestFixtureChecksumIsStable(t *testing.T) {
	if FixtureChecksum() != FixtureChecksum() || len(FixtureChecksum()) != 64 {
		t.Fatalf("checksum %q", FixtureChecksum())
	}
}

func TestCompareRefusesUnevenRepeats(t *testing.T) {
	base := reportWith(result("a", KindPair, "group", false), result("b", KindPair, "group", true))
	// Three passing repeats of b and one failing a would read as a 25-point gain.
	candidate := reportWith(result("a", KindPair, "group", false), result("b", KindPair, "group", true), result("b", KindPair, "group", true), result("b", KindPair, "group", true))
	if got := Compare(base, candidate); got.Compatible {
		t.Fatalf("uneven repeats compared: %#v", got)
	}
}

func TestCompareRecomputesSummariesAndRefusesStaleOnes(t *testing.T) {
	base := reportWith(result("a", KindPair, "group", true), result("b", KindPair, "group", true))
	candidate := reportWith(result("a", KindPair, "group", false), result("b", KindPair, "group", false))

	missing := candidate
	missing.Summary = Summary{}
	if got := Compare(base, missing); got.Compatible || len(got.Regressions) != 0 {
		t.Fatalf("a report with no summary was compared: %#v", got)
	}

	tampered := candidate
	tampered.Summary = base.Summary
	if got := Compare(base, tampered); got.Compatible {
		t.Fatalf("a report whose summary disagrees with its results was compared: %#v", got)
	}

	got := Compare(base, candidate)
	if !got.Compatible || len(got.Regressions) != 1 {
		t.Fatalf("the honest regression was not reported: %#v", got)
	}
}

func TestBundleRequestSendsTheBrowsersWholeFieldMap(t *testing.T) {
	c := Case{ID: "b", Kind: KindBundle, Carrier: "note", Prompt: "x", Current: map[string]string{"CustomHeader": "<p>h</p>"}}
	body := c.Request(3, 4)
	var content map[string]string
	if err := json.Unmarshal([]byte(body["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	// templateBundle.js sends every SLOT_FIELDS entry, including fields the
	// carrier has no surface for.
	if len(content) != 29 || content["CustomPreview"] != "" || content["CustomOwnEntities"] != "" || content["CustomHeader"] != "<p>h</p>" {
		t.Fatalf("bundle content = %v", content)
	}
	if _, ok := content["CustomPreview"]; !ok {
		t.Fatal("a cross-carrier field is missing")
	}
	f, _ := FixtureForCase(c)
	if body["metaSchema"] != f.MetaSchema {
		t.Fatalf("the bundle request does not carry the editor's schema: %v", body["metaSchema"])
	}
}

func TestCompareRefusesAStoredPassThatDisagreesWithItsChecks(t *testing.T) {
	base := reportWith(result("a", KindPair, "group", true))
	lying := result("a", KindPair, "group", true)
	lying.Checks.LintErrors = 1 // Checks.Pass() is false
	candidate := reportWith(lying)
	if got := Compare(base, candidate); got.Compatible {
		t.Fatalf("a result whose Pass disagrees with its checks was compared: %#v", got)
	}
}

// TestAggregateKeepsNoFreeText plants a secret in every free-text field a full
// report can hold, then walks the aggregate's JSON: every string must sit under
// a known config key, be a known enum value, or be a corpus case ID.
func TestAggregateKeepsNoFreeText(t *testing.T) {
	const secret = "sk-planted-0123456789"
	c := Corpus()[0]
	checks := Checks{Kind: c.Kind, HTTPStatus: 502, MissingSlots: []string{"CustomHeader"}, RenderEmptySlots: []string{"CustomHeader"},
		LeftoverShortcodeText: []string{"CustomHeader"}, MissingExpectFields: []string{"isbn"}, ExecutableMarkup: []string{"script"}, RenderedExecutableMarkup: []string{"embed"}}
	r := CaseResult{Case: c.ID, Kind: c.Kind, Carrier: c.Carrier, Repeat: 1, Pass: checks.Pass(), Checks: checks,
		Details:  &Details{ProviderError: secret, SchemaCompileError: secret, UnknownShortcodes: []string{secret}},
		Draft:    &Draft{Error: secret, Content: secret, Explanation: secret, Slots: map[string]string{secret: secret}, Issues: []shortcodes.LintIssue{{Message: secret}}},
		Previews: map[string]Preview{secret: {HTML: secret, Error: secret, Issues: []shortcodes.LintIssue{{Message: secret}}}},
		Calls:    []Call{{Model: "deepseek-flash", ErrorClass: ErrorClassHTTPStatus, FinishReason: "stop", Content: secret}},
	}
	report := reportWith(r)
	report.Commit, report.Model, report.Timeout, report.StartedAt, report.FinishedAt = "abc123+dirty", "deepseek-flash", "20s", "2026-10-24T00:00:00Z", "2026-10-24T00:01:00Z"

	encoded, err := json.Marshal(AggregateOnly(report))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "planted") {
		t.Fatalf("the aggregate carries planted text: %s", encoded)
	}
	ids := map[string]bool{}
	expectFields := map[string]bool{}
	for _, cc := range Corpus() {
		ids[cc.ID] = true
		for _, f := range cc.ExpectFields {
			expectFields[f] = true
		}
	}
	slots := map[string]bool{}
	for _, s := range browserBundleFields {
		slots[s] = true
	}
	flags := map[string]bool{}
	for _, f := range flagOrder {
		flags[f] = true
	}
	enum := func(values ...string) map[string]bool {
		m := map[string]bool{}
		for _, v := range values {
			m[v] = true
		}
		return m
	}
	allowed := map[string]func(string) bool{
		"case":    func(v string) bool { return ids[v] },
		"kind":    func(v string) bool { return enum("slot", "pair", "bundle", "metaschema")[v] },
		"carrier": func(v string) bool { return enum("group", "resource", "note")[v] },
		"errorClass": func(v string) bool {
			return enum(ErrorClassTimeout, ErrorClassHTTPStatus, ErrorClassTransport, ErrorClassDecode)[v]
		},
		"finishReason":             func(v string) bool { return knownFinishReasons[v] },
		"missingSlots":             func(v string) bool { return slots[v] },
		"renderEmptySlots":         func(v string) bool { return slots[v] },
		"leftoverShortcodeText":    func(v string) bool { return slots[v] },
		"missingExpectFields":      func(v string) bool { return expectFields[v] },
		"executableMarkup":         func(v string) bool { return flags[v] },
		"renderedExecutableMarkup": func(v string) bool { return flags[v] },
		"key":                      func(v string) bool { return v == "all" || strings.Count(v, "/") <= 1 },
		// Config the harness sets, never text from upstream.
		"corpusVersion": func(string) bool { return true }, "fixtureChecksum": func(string) bool { return true },
		"commit": func(string) bool { return true }, "startedAt": func(string) bool { return true },
		"finishedAt": func(string) bool { return true }, "model": func(string) bool { return true }, "timeout": func(string) bool { return true },
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatal(err)
	}
	var walk func(key string, n any)
	walk = func(key string, n any) {
		switch x := n.(type) {
		case string:
			check, ok := allowed[key]
			if !ok || !check(x) {
				t.Errorf("free text in the aggregate under %q: %q", key, x)
			}
		case []any:
			for _, e := range x {
				walk(key, e)
			}
		case map[string]any:
			for k, e := range x {
				walk(k, e)
			}
		}
	}
	walk("", generic)
}

func TestRequestContentMatchesJSONStringify(t *testing.T) {
	pair := Case{ID: "p", Kind: KindPair, Carrier: "group", Slot: "CustomHeader", Prompt: "x",
		Current: map[string]string{"CustomHeader": `<p class="a">&</p>`}}
	if got := pair.Request(1, 2)["content"]; got != `{"CustomHeader":"<p class=\"a\">&</p>","CustomHeaderCSS":""}` {
		t.Fatalf("pair content = %s", got)
	}
	bundle := Case{ID: "b", Kind: KindBundle, Carrier: "note", Prompt: "x", Current: map[string]string{"CustomCSS": "a>b{}"}}
	content := bundle.Request(1, 2)["content"].(string)
	if !strings.HasPrefix(content, `{"CustomHeader":"","CustomHeaderCSS":"","CustomDetailFooter":""`) || !strings.HasSuffix(content, `"CustomCSS":"a>b{}"}`) {
		t.Fatalf("bundle content is not in SLOT_FIELDS order or escapes HTML: %s", content)
	}
}
