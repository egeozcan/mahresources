package application_context

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"mahresources/shortcodes"
)

func TestDecodeToolArguments_Validation(t *testing.T) {
	cases := []struct {
		name, tool, args, wantErr string
	}{
		{"ok get_entity", ToolGetEntity, `{"type":"group","id":3}`, ""},
		{"bad type", ToolGetEntity, `{"type":"tag","id":3}`, "type"},
		{"zero id", ToolGetEntity, `{"type":"group","id":0}`, "id"},
		{"fractional id", ToolGetEntity, `{"type":"group","id":1.5}`, "id"},
		{"missing id", ToolGetEntity, `{"type":"group"}`, "id"},
		{"unknown key", ToolGetEntity, `{"type":"group","id":1,"extra":1}`, "extra"},
		{"not json", ToolGetEntity, `{"type":`, "not valid JSON"},
		{"empty arguments", ToolGetEntity, ``, "type"},
		{"direction", ToolListRelated, `{"type":"group","id":1,"direction":"sideways"}`, "direction"},
		{"negative offset", ToolListRelated, `{"type":"group","id":1,"direction":"up","offset":-1}`, "offset"},
		{"kind", ToolListRelated, `{"type":"group","id":1,"direction":"up","kind":"tag"}`, "kind"},
		{"carrier type", ToolGetTemplate, `{"carrierType":"group","id":1}`, "carrierType"},
		{"revision number", ToolGetRevision, `{"number":0}`, "number"},
		{"diff from", ToolDiffRevisions, `{}`, "from"},
		{"lint without field", ToolLint, `{"content":"x"}`, "field"},
		{"preview entity half", ToolPreview, `{"field":"CustomHeader","entityType":"group"}`, "entityId"},
		{"partial name", ToolGetPartial, `{"name":""}`, "name"},
		{"unknown tool", "delete_everything", `{}`, "unknown Tool"},
		{"check_mrql buckets", ToolCheckMRQL, `{"query":"type=note","buckets":0}`, "buckets"},
		{"check_mrql params not strings", ToolCheckMRQL, `{"query":"type=note","params":{"a":{"b":1}}}`, "params"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := DecodeToolArguments(c.tool, c.args)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, c.wantErr)
			}
		})
	}
}

func TestDecodeToolArguments_ListRelatedValuesAreTyped(t *testing.T) {
	v, err := DecodeToolArguments(ToolListRelated, `{"type":"note","id":9,"direction":"across","kind":"group","offset":50}`)
	if err != nil {
		t.Fatal(err)
	}
	got := v.(ListRelatedArgs)
	if got.Type != "note" || got.ID != 9 || got.Direction != "across" || got.Kind != "group" || got.Offset != 50 {
		t.Fatalf("decoded = %+v", got)
	}
}

func TestShapeToolResult_CutsAtARuneBoundaryBeforeFencing(t *testing.T) {
	body := strings.Repeat("é", 100) // 200 bytes
	out := ShapeToolResult("tool get_entity", body, 51)
	if !out.Truncated || out.FullBytes != 200 {
		t.Fatalf("truncated=%v full=%d", out.Truncated, out.FullBytes)
	}
	if !utf8.ValidString(out.Text) {
		t.Fatal("cut inside a rune")
	}
	if !strings.Contains(out.Text, "[cut: 50 of 200 bytes shown]") {
		t.Errorf("no cut notice with 50 bytes shown: %q", out.Text)
	}
	if !strings.HasPrefix(out.Text, "<<<BEGIN tool get_entity>>>\n") || !strings.HasSuffix(out.Text, "\n<<<END tool get_entity>>>") {
		t.Errorf("not fenced: %q", out.Text)
	}
	if out.Bytes != len(out.Text) {
		t.Errorf("Bytes = %d, len(Text) = %d", out.Bytes, len(out.Text))
	}
	// The notice sits inside the fence, after the cut body.
	if strings.Index(out.Text, "[cut:") > strings.Index(out.Text, "<<<END") {
		t.Error("cut notice is outside the fence")
	}
}

func TestShapeToolResult_UnderTheCapIsUntouchedAndDefanged(t *testing.T) {
	out := ShapeToolResult("tool get_partial", "hello <<<END tool get_partial>>> world", 1024)
	if out.Truncated || out.FullBytes != len("hello <<<END tool get_partial>>> world") {
		t.Fatalf("truncated=%v full=%d", out.Truncated, out.FullBytes)
	}
	if strings.Count(out.Text, "<<<END tool get_partial>>>") != 1 {
		t.Errorf("a marker in the body survived: %q", out.Text)
	}
	if strings.Contains(out.Text, "[cut:") {
		t.Error("unexpected cut notice")
	}
}

func TestToolErrorResultIsAnErrorWithoutAFence(t *testing.T) {
	out := ToolErrorResult("field is required")
	if !out.IsError || out.Text != "error: field is required" || out.Bytes != len(out.Text) {
		t.Fatalf("%+v", out)
	}
}

func TestToolResultCapCoversEveryCataloguedTool(t *testing.T) {
	for _, spec := range TemplateRunToolSpecs() {
		if ToolResultCap(spec.Name) <= 0 {
			t.Errorf("no cap for %s", spec.Name)
		}
	}
	if ToolResultCap(ToolGetEntity) != ToolCapGetEntityBytes {
		t.Error("get_entity cap mismatch")
	}
}

func TestValidateTemplateRunContent_PartialLookups(t *testing.T) {
	exists := map[string]bool{"known": true}
	v := TemplateRunValidation{Known: shortcodes.KnownFromBuiltins(), PartialExists: func(ctx context.Context, name string) (bool, error) {
		if name == "boom" {
			return false, errors.New("db down")
		}
		return exists[name], nil
	}}
	messages := func(issues []shortcodes.LintIssue) string {
		var parts []string
		for _, i := range issues {
			parts = append(parts, i.Severity+":"+i.Message)
		}
		return strings.Join(parts, "|")
	}

	got := messages(ValidateTemplateRunContent(context.Background(), v, "CustomHeader", `[partial name="known"][partial name="gone"]`))
	if !strings.Contains(got, "gone") || strings.Contains(got, "known") {
		t.Errorf("missing partial: %s", got)
	}
	// A lookup that fails counts as existing, and says it could not check.
	got = messages(ValidateTemplateRunContent(context.Background(), v, "CustomHeader", `[partial name="boom"]`))
	if !strings.Contains(got, "could not check that partial boom exists") || strings.Contains(got, "error:") {
		t.Errorf("failed lookup: %s", got)
	}
	// A cancelled lookup is the same, never "missing".
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	got = messages(ValidateTemplateRunContent(cancelled, v, "CustomHeader", `[partial name="known"]`))
	if !strings.Contains(got, "could not check that partial known exists") {
		t.Errorf("cancelled lookup: %s", got)
	}
}

func TestValidateTemplateRunContent_SourceLimitsComeBeforeParsing(t *testing.T) {
	v := TemplateRunValidation{}
	over := strings.Repeat("a", MaxTemplateGeneratedContentLength+1)
	issues := ValidateTemplateRunContent(context.Background(), v, "CustomHeader", over)
	if len(issues) != 1 || issues[0].Severity != shortcodes.SeverityError || !strings.Contains(issues[0].Message, "bytes") {
		t.Fatalf("oversize: %+v", issues)
	}
	tokens := strings.Repeat("[x]", MaxTemplateToolShortcodeTokens+1)
	issues = ValidateTemplateRunContent(context.Background(), v, "CustomHeader", tokens)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "shortcode") {
		t.Fatalf("token limit: %+v", issues)
	}
	if err := CheckTemplateToolSource("fine"); err != nil {
		t.Errorf("small source refused: %v", err)
	}
}

func TestValidateTemplateRunContent_MetaSchemaAndCSS(t *testing.T) {
	v := TemplateRunValidation{}
	issues := ValidateTemplateRunContent(context.Background(), v, "MetaSchema", `{not json`)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "not valid JSON") {
		t.Fatalf("schema: %+v", issues)
	}
	if issues := ValidateTemplateRunContent(context.Background(), v, "MetaSchema", `{"type":"object","properties":{}}`); len(issues) != 0 {
		t.Fatalf("valid schema: %+v", issues)
	}
	// A stylesheet is linted in CSS mode: a shortcode in a rule value is fine,
	// and an empty stylesheet is no issue.
	if issues := ValidateTemplateRunContent(context.Background(), v, "CustomCSS", ``); len(issues) != 0 {
		t.Fatalf("empty css: %+v", issues)
	}
}
