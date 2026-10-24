//go:build templateeval && json1 && fts5

package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/internal/templateeval"
	"mahresources/shortcodes"
)

// The quality eval for the template generator. It calls the real DeepSeek
// provider and costs money; see benchmarks/template-generation/README.md.
//
//	DEEPSEEK_API_KEY=... go test --tags 'json1 fts5 templateeval' ./server/api_tests \
//	  -run TestTemplateGenerationEval -count=1 -timeout 60m -v
func TestTemplateGenerationEval(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Fatal("DEEPSEEK_API_KEY is not set. The template eval calls the real provider; see benchmarks/template-generation/README.md.")
	}
	repeats := 3
	if raw := os.Getenv("TEMPLATE_EVAL_REPEATS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			t.Fatalf("invalid TEMPLATE_EVAL_REPEATS=%q", raw)
		}
		repeats = n
	}
	var ids []string
	if raw := strings.TrimSpace(os.Getenv("TEMPLATE_EVAL_CASES")); raw != "" {
		ids = strings.Split(raw, ",")
	}
	cases, err := templateeval.FilterCases(templateeval.Corpus(), ids)
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("TEMPLATE_EVAL_OUT")
	if out == "" {
		out = filepath.Join(repoRoot, "benchmarks", "template-generation", "results", "eval-"+time.Now().UTC().Format("20060102T150405Z")+".json")
	}

	// The provider configuration main.go reads (DEEPSEEK_MODEL, DEEPSEEK_TIMEOUT
	// and their defaults), wired the way main.go wires it, with the recorder as
	// the HTTP transport.
	model := os.Getenv("DEEPSEEK_MODEL")
	if model == "" {
		model = application_context.DefaultDeepSeekMRQLGenerationModel
	}
	timeout := application_context.DefaultDeepSeekMRQLGenerationTimeout
	if raw := os.Getenv("DEEPSEEK_TIMEOUT"); raw != "" {
		if timeout, err = time.ParseDuration(raw); err != nil {
			t.Fatalf("invalid DEEPSEEK_TIMEOUT=%q: %v", raw, err)
		}
	}

	tc := SetupTestEnv(t)
	seeded := seedTemplateEvalFixtures(t, tc)
	// There is no dry-run limiter yet (phase 1f adds one); the generation limiter
	// is the only one to lift.
	tc.AppCtx.SetTemplateGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(math.MaxInt, time.Minute))
	recorder := templateeval.NewRecorder(nil, apiKey)
	provider := application_context.NewDeepSeekTemplateDraftProvider(
		application_context.DefaultDeepSeekChatCompletionsURL,
		apiKey,
		model,
		&http.Client{Transport: recorder},
	)
	tc.AppCtx.SetTemplateGenerator(application_context.NewTemplateGenerator(provider, application_context.TemplateGenerationConfig{
		APIKey:  apiKey,
		Model:   model,
		Timeout: timeout,
	}))
	// SetupTestEnv runs no plugin manager, so the server lints against the
	// built-ins alone, and so does the scorer.
	known := shortcodes.KnownFromBuiltins()

	report := templateeval.RunReport{
		SchemaVersion:   templateeval.ReportSchemaVersion,
		CorpusVersion:   templateeval.CorpusVersion,
		FixtureChecksum: templateeval.FixtureChecksum(),
		Commit:          evalCommit(repoRoot),
		StartedAt:       time.Now().UTC().Format(time.RFC3339),
		Model:           model,
		Timeout:         timeout.String(),
		Repeats:         repeats,
	}
	for _, c := range cases {
		fixture, _ := templateeval.FixtureForCase(c)
		ids := seeded[fixture.Key]
		for repeat := 1; repeat <= repeats; repeat++ {
			_ = recorder.Take()
			start := time.Now()
			draft := evalGenerate(tc, c, ids.category, ids.sample)
			calls := recorder.Take()
			previews := evalPreviews(tc, c, draft, ids.category, ids.sample)
			checks := templateeval.Score(c, draft, calls, previews, known)
			result := templateeval.CaseResult{
				Case: c.ID, Kind: c.Kind, Carrier: c.Carrier, Repeat: repeat,
				DurationMS: time.Since(start).Milliseconds(),
				Pass:       checks.Pass(), Checks: checks, Calls: calls,
				Draft: &draft, Previews: previews,
			}
			report.Results = append(report.Results, result)
			t.Logf("%s #%d: pass=%t status=%d calls=%d %dms", c.ID, repeat, result.Pass, draft.Status, len(calls), result.DurationMS)
		}
	}
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	report.Summary = templateeval.Summarize(report.Results)
	// The recorder already redacts what it captures; this covers the drafts,
	// previews and error text that reached the report by other routes.
	report = templateeval.RedactReport(report, apiKey)

	if err := templateeval.WriteJSONAtomic(out, report); err != nil {
		t.Fatal(err)
	}
	aggregate := strings.TrimSuffix(out, ".json") + ".aggregate.json"
	if err := templateeval.WriteJSONAtomic(aggregate, templateeval.AggregateOnly(report)); err != nil {
		t.Fatal(err)
	}
	t.Logf("report: %s\naggregate: %s\n%s", out, aggregate, templateeval.FormatSummary(report.Summary))
}

// TestTemplateGenerationEvalCompare compares two saved reports. It makes no
// network calls.
func TestTemplateGenerationEvalCompare(t *testing.T) {
	basePath, candidatePath := os.Getenv("TEMPLATE_EVAL_BASELINE"), os.Getenv("TEMPLATE_EVAL_CANDIDATE")
	if basePath == "" && candidatePath == "" {
		t.Skip("set TEMPLATE_EVAL_BASELINE and TEMPLATE_EVAL_CANDIDATE to compare two reports")
	}
	if basePath == "" || candidatePath == "" {
		t.Fatal("TEMPLATE_EVAL_BASELINE and TEMPLATE_EVAL_CANDIDATE must both be set")
	}
	base, err := templateeval.ReadReport(basePath)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := templateeval.ReadReport(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	cmp := templateeval.Compare(base, candidate)
	t.Logf("\n%s", templateeval.FormatComparison(cmp))
	if !cmp.Compatible {
		t.Fatalf("the reports cannot be compared: %s", strings.Join(cmp.Errors, "; "))
	}
	for _, r := range cmp.Regressions {
		t.Errorf("regression: %s", r)
	}
}

type evalFixtureIDs struct {
	category uint
	sample   uint // Members[0]
}

// seedTemplateEvalFixtures creates every fixture through the public API.
func seedTemplateEvalFixtures(t *testing.T, tc *TestContext) map[string]evalFixtureIDs {
	t.Helper()
	seeded := map[string]evalFixtureIDs{}
	images := 0
	upload := func(fields map[string]string, notes []uint, width, height int) uint {
		images++
		if width == 0 {
			width, height = 20+images, 20
		}
		return evalUploadResource(t, tc, fields, notes, createTestPNG(t, width, height))
	}
	for _, f := range templateeval.Fixtures() {
		categoryPath := map[string]string{"group": "/v1/category", "resource": "/v1/resourceCategory", "note": "/v1/note/noteType"}[f.Carrier]
		category := evalPostJSON(t, tc, categoryPath, map[string]any{"Name": f.Name, "Description": f.Description, "MetaSchema": f.MetaSchema})
		container := evalPostJSON(t, tc, "/v1/group", map[string]any{"Name": f.Container})
		ids := evalFixtureIDs{category: category}
		for i, m := range f.Members {
			meta, _ := json.Marshal(m.Meta)
			var member uint
			switch f.Carrier {
			case "group":
				member = evalPostJSON(t, tc, "/v1/group", map[string]any{"Name": m.Name, "Description": m.Description, "Meta": string(meta), "CategoryId": category, "OwnerId": container})
				for _, child := range m.Children {
					switch child.Kind {
					case "group":
						evalPostJSON(t, tc, "/v1/group", map[string]any{"Name": child.Name, "OwnerId": member})
					case "note":
						evalPostJSON(t, tc, "/v1/note", map[string]any{"Name": child.Name, "OwnerId": member})
					case "resource":
						upload(map[string]string{"Name": child.Name, "OwnerId": fmt.Sprint(member)}, nil, child.Width, child.Height)
					}
				}
			case "resource":
				member = upload(map[string]string{"Name": m.Name, "Description": m.Description, "Meta": string(meta),
					"ResourceCategoryId": fmt.Sprint(category), "OwnerId": fmt.Sprint(container)}, nil, m.Width, m.Height)
				for _, child := range m.Children {
					evalPostJSON(t, tc, "/v1/note", map[string]any{"Name": child.Name, "OwnerId": container, "Resources": []uint{member}})
				}
			case "note":
				member = evalPostJSON(t, tc, "/v1/note", map[string]any{"Name": m.Name, "Description": m.Description, "Meta": string(meta), "NoteTypeId": category, "OwnerId": container})
				for _, child := range m.Children {
					upload(map[string]string{"Name": child.Name, "OwnerId": fmt.Sprint(container)}, []uint{member}, child.Width, child.Height)
				}
			}
			if i == 0 {
				ids.sample = member
			}
		}
		seeded[f.Key] = ids
	}
	return seeded
}

func evalRequest(tc *TestContext, path string, body any) *httptest.ResponseRecorder {
	encoded, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return rr
}

func evalPostJSON(t *testing.T, tc *TestContext, path string, body map[string]any) uint {
	t.Helper()
	rr := evalRequest(tc, path, body)
	var created struct{ ID uint }
	if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &created) != nil || created.ID == 0 {
		t.Fatalf("seed %s %v: %d %s", path, body["Name"], rr.Code, rr.Body.String())
	}
	return created.ID
}

func evalUploadResource(t *testing.T, tc *TestContext, fields map[string]string, notes []uint, png []byte) uint {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("resource", fields["Name"])
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(png)
	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}
	for _, id := range notes {
		_ = writer.WriteField("Notes", fmt.Sprint(id))
	}
	_ = writer.Close()
	req, _ := http.NewRequest(http.MethodPost, "/v1/resource", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	var created []struct{ ID uint }
	if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &created) != nil || len(created) != 1 {
		t.Fatalf("seed resource %s: %d %s", fields["Name"], rr.Code, rr.Body.String())
	}
	return created[0].ID
}

func evalGenerate(tc *TestContext, c templateeval.Case, categoryID, entityID uint) templateeval.Draft {
	rr := evalRequest(tc, "/v1/"+templateeval.CarrierRoute(c.Carrier)+"/generateTemplate", c.Request(categoryID, entityID))
	draft := templateeval.Draft{Status: rr.Code}
	if rr.Code != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(rr.Body.Bytes(), &failure) == nil && failure.Error != "" {
			draft.Error = failure.Error
		} else {
			draft.Error = strings.TrimSpace(rr.Body.String())
		}
		return draft
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &draft); err != nil {
		draft.Error = "undecodable response: " + err.Error()
	}
	draft.Status = rr.Code
	return draft
}

// evalPreviews renders each returned HTML slot with its companion CSS, the way
// the editor's live preview does.
func evalPreviews(tc *TestContext, c templateeval.Case, d templateeval.Draft, categoryID, entityID uint) map[string]templateeval.Preview {
	if d.Status != http.StatusOK || len(d.Slots) == 0 {
		return nil
	}
	previews := map[string]templateeval.Preview{}
	for slot, html := range d.Slots {
		if strings.HasSuffix(slot, "CSS") || strings.TrimSpace(html) == "" {
			continue
		}
		body := map[string]any{"content": html, "css": d.Slots[slot+"CSS"], "slot": slot, "categoryId": categoryID}
		if templateeval.RendersAgainstCarrier(slot) {
			body["carrier"] = true
		} else {
			body["entityId"] = entityID
		}
		rr := evalRequest(tc, "/v1/"+templateeval.CarrierRoute(c.Carrier)+"/previewTemplate", body)
		preview := templateeval.Preview{Status: rr.Code}
		if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &preview) != nil {
			preview.Error = strings.TrimSpace(rr.Body.String())
		}
		preview.Status = rr.Code
		previews[slot] = preview
	}
	return previews
}

// evalCommit names the code under test, marking a dirty tree.
func evalCommit(repoRoot string) string {
	head, err := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	commit := strings.TrimSpace(string(head))
	if status, err := exec.Command("git", "-C", repoRoot, "status", "--porcelain", "--untracked-files=no").Output(); err == nil && len(bytes.TrimSpace(status)) > 0 {
		commit += "+dirty"
	}
	return commit
}
