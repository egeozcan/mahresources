package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c2h5oh/datasize"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/types"
)

// Privacy mode, end to end against the real wire adapters: a scripted provider
// behind a capturing http.RoundTripper, a real database, and the whole handler.

type privacyFixture struct {
	tc                        *TestContext
	cat                       *models.Category
	root, child, target, html *models.Group
	resource                  *models.Resource
	note                      *models.Note
	protected, readable       []string
	bodies                    [][]byte
	mu                        sync.Mutex
	script                    func(n int, req map[string]any) map[string]any
	calls                     int
	repair                    bool
	inject                    func(n int) string // text appended to the n-th request's last message, to model a leak below the masker
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func chatToolCalls(calls ...map[string]string) map[string]any {
	var tcs []any
	for _, c := range calls {
		tcs = append(tcs, map[string]any{"id": c["id"], "type": "function", "function": map[string]string{"name": c["name"], "arguments": c["args"]}})
	}
	return map[string]any{
		"choices": []any{map[string]any{"finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "content": nil, "tool_calls": tcs}}},
		"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 20},
	}
}

func call(id, name string, args any) map[string]string {
	raw, _ := json.Marshal(args)
	return map[string]string{"id": id, "name": name, "args": string(raw)}
}

func newPrivacyFixture(t *testing.T) *privacyFixture {
	t.Helper()
	tc := SetupTestEnv(t)
	f := &privacyFixture{tc: tc}
	create := func(v any) {
		t.Helper()
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.cat = &models.Category{
		Name: "Projects READABLE_CATEGORY_NAME", Description: "PROT_CATEGORY_DESC",
		MetaSchema:   `{"type":"object","properties":{"READABLE_META_KEY":{"type":"string"},"status":{"enum":["open","closed"]},"list":{"type":"array"}}}`,
		CustomHeader: `<h1>[meta path="READABLE_META_KEY"]</h1> READABLE_TEMPLATE_SOURCE`,
		// A custom per-result template: [mrql] renders each group through it.
		CustomMRQLResult: `<i class="cm">[property path="Name"]</i>`,
	}
	create(f.cat)
	f.root = &models.Group{Name: "PROT_OWNER_NAME", Description: "PROT_OWNER_DESC"}
	create(f.root)
	f.target = &models.Group{Name: "PROT_RELATION_TARGET"}
	create(f.target)
	u := types.URL{}
	_ = u.Scan("https://prot-host.example/PROT_URL_PATH")
	f.child = &models.Group{
		Name: "PROT_GROUP_NAME", Description: "PROT_GROUP_DESC \"quoted\" naïve", URL: &u,
		OwnerId: &f.root.ID, CategoryId: &f.cat.ID,
		Meta: types.JSON(`{"READABLE_META_KEY":"PROT_META_VALUE","status":"open","num":734928601,"list":["PROT_LIST_ITEM_A","PROT_LIST_ITEM_B"]}`),
	}
	create(f.child)
	// Protected names that equal text a render makes from a preserved number: the
	// FileSize itself and its humanised form.
	for _, name := range []string{"PROT_TAG_NAME", "734928601", datasize.ByteSize(734928601).HumanReadable()} {
		tag := &models.Tag{Name: name}
		create(tag)
		if err := tc.DB.Model(f.child).Association("Tags").Append(tag); err != nil {
			t.Fatal(err)
		}
	}
	relType := &models.GroupRelationType{Name: "READABLE_RELTYPE"}
	create(relType)
	create(&models.GroupRelation{FromGroupId: &f.child.ID, ToGroupId: &f.target.ID, RelationTypeId: &relType.ID})
	f.resource = &models.Resource{
		Name: "PROT_RES_NAME", OriginalName: "PROT_ORIGINAL_NAME", Hash: "PROT_HASH_VALUE", HashType: "SHA1",
		Location: "PROT_LOCATION/x", ContentType: `image/png; name="PROT_CT_PARAM"`, Description: "PROT_RES_DESC",
		OwnerId: &f.child.ID, ResourceCategoryId: 1, FileSize: 734928601,
	}
	create(f.resource)
	token := "PROTSHARETOKEN0123456789abcdef12"
	f.note = &models.Note{Name: "PROT_NOTE_NAME", OwnerId: &f.child.ID, ShareToken: &token}
	create(f.note)
	create(&models.NoteBlock{NoteID: f.note.ID, Type: "text", Position: "a", Content: types.JSON(`{"text":"PROT_BLOCK_TEXT"}`), State: types.JSON(`{}`)})
	create(&models.TemplatePartial{Name: "READABLE_partial", Description: "PROT_PARTIAL_DESC", Content: `<i>READABLE_PARTIAL_SOURCE [partial name="READABLE_inner"]</i>`})
	create(&models.TemplatePartial{Name: "READABLE_inner", Description: "inner", Content: `<u>READABLE_INNER_SOURCE [property path="Name"]</u>`})
	for i, label := range []string{"PROT_REV_LABEL", ""} {
		create(&models.TemplateRevision{CarrierType: models.TemplateCarrierCategory, CarrierID: f.cat.ID, Number: uint(i + 1), Source: "form",
			Snapshot: fmt.Sprintf(`{"CustomHeader":"READABLE_REV_SOURCE_%d"}`, i+1), Applied: `[]`, ChangedSlots: `["CustomHeader"]`, Label: label})
	}
	f.html = &models.Group{Name: `PROT_HTML <i>&"q"</i>`, OwnerId: &f.root.ID, CategoryId: &f.cat.ID}
	create(f.html)
	f.protected = []string{
		"PROT_OWNER_NAME", "PROT_OWNER_DESC", "PROT_RELATION_TARGET", "PROT_GROUP_NAME", "PROT_GROUP_DESC", "PROT_URL_PATH", "prot-host.example",
		"PROT_META_VALUE", "PROT_LIST_ITEM_A", "PROT_LIST_ITEM_B", "PROT_TAG_NAME", "PROT_RES_NAME", "PROT_ORIGINAL_NAME", "PROT_HASH_VALUE",
		"PROT_LOCATION", "PROT_CT_PARAM", "PROT_RES_DESC", "PROTSHARETOKEN", "PROT_NOTE_NAME", "PROT_BLOCK_TEXT", "PROT_PARTIAL_DESC",
		"PROT_REV_LABEL", "PROT_CATEGORY_DESC", "PROT_HTML",
	}
	f.readable = []string{"READABLE_CATEGORY_NAME", "READABLE_META_KEY", "READABLE_TEMPLATE_SOURCE", "READABLE_RELTYPE", "READABLE_partial", "READABLE_PARTIAL_SOURCE", "READABLE_INNER_SOURCE", "READABLE_REV_SOURCE_1", "READABLE_USER_REQUEST"}
	return f
}

// install wires the real DeepSeek chat adapter behind a capturing RoundTripper.
func (f *privacyFixture) install(t *testing.T, mode string) {
	t.Helper()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.bodies = append(f.bodies, raw)
		f.calls++
		n := f.calls
		f.mu.Unlock()
		var req map[string]any
		_ = json.Unmarshal(raw, &req)
		resp := f.script(n, req)
		encoded, _ := json.Marshal(resp)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(encoded)), Request: r}, nil
	})}
	f.tc.AppCtx.InstallAIGeneration(application_context.TemplateGenerationConfig{RepairDisabled: !f.repair}, client)
	update := map[string]any{"values": map[string]string{
		"ai_deepseek_api_key": "test-key", "ai_template_provider": "deepseek", "ai_template_deepseek_model": "ds-model",
		"ai_template_generation_mode": mode,
	}}
	if rec := f.tc.MakeRequest(http.MethodPut, "/v1/admin/settings", update); rec.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body.String())
	}
}

func (f *privacyFixture) allBodies() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b strings.Builder
	for _, body := range f.bodies {
		b.Write(body)
		b.WriteByte('\n')
	}
	return b.String()
}

func (f *privacyFixture) requestBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"target": "slot", "slot": "CustomCSS", "mode": "css", "prompt": "READABLE_USER_REQUEST: make it blue",
		"categoryId": f.cat.ID, "entityId": f.child.ID, "previewMRQL": true,
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

// toolResults maps a call ID to its result text across every captured request.
func (f *privacyFixture) toolResults() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for _, raw := range f.bodies {
		var req struct {
			Messages []struct {
				Role       string `json:"role"`
				ToolCallID string `json:"tool_call_id"`
				Content    string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(raw, &req)
		for _, m := range req.Messages {
			if m.Role == "tool" {
				out[m.ToolCallID] = m.Content
			}
		}
	}
	return out
}

func (f *privacyFixture) generate(t *testing.T, path string, extra map[string]any) *httpResult {
	t.Helper()
	hash := disclose(t, f.tc, path, f.requestBody(extra))
	rec := f.tc.MakeRequest(http.MethodPost, path, withHash(f.requestBody(extra), hash))
	return &httpResult{code: rec.Code, body: rec.Body.Bytes()}
}

type httpResult struct {
	code int
	body []byte
}

func harnessScript(f *privacyFixture) func(n int, req map[string]any) map[string]any {
	childID, resID, noteID, catID := f.child.ID, f.resource.ID, f.note.ID, f.cat.ID
	preview := func(id, content string, entityType string, entityID uint) map[string]string {
		args := map[string]any{"field": "CustomHeader", "content": content, "entityType": entityType, "entityId": entityID}
		return call(id, "preview", args)
	}
	return func(n int, req map[string]any) map[string]any {
		switch n {
		case 1:
			return chatToolCalls(
				call("t1", "get_entity", map[string]any{"type": "group", "id": childID}),
				call("t2", "list_related", map[string]any{"type": "group", "id": childID, "direction": "down"}),
				call("t3", "get_template", map[string]any{"carrierType": "category", "id": catID}),
				call("t4", "list_carriers", map[string]any{"carrierType": "category"}),
				call("t5", "list_members", map[string]any{"carrierType": "category", "id": catID}),
				call("t6", "list_revisions", map[string]any{}),
			)
		case 2:
			return chatToolCalls(
				call("u1", "get_revision", map[string]any{"number": 1}),
				call("u2", "diff_revisions", map[string]any{"from": 1, "to": 2}),
				call("u3", "get_partial", map[string]any{"name": "READABLE_partial"}),
				call("u4", "check_mrql", map[string]any{"query": "type = group"}),
				call("u5", "lint", map[string]any{"field": "CustomHeader", "content": `[property path="Name"]`}),
				call("u6", "get_shortcode_docs", map[string]any{"name": "property"}),
			)
		case 3:
			return chatToolCalls(
				preview("p1", `<h1>[property path="Name"]</h1><p>[property path="Description"]</p><a>[property path="URL"]</a><b>[property path="Owner.Name"]</b>[meta path="READABLE_META_KEY"]`, "group", childID),
				preview("p2", `[property path="Hash"]|[property path="Location"]|[property path="OriginalName"]|[property path="ContentType"]|[property path="Name"]|[property path="FileSize" format="filesize"]`, "resource", resID),
				preview("p3", `[property path="ShareToken"]|[property path="Name"]`, "note", noteID),
				preview("p4", `[each path="list"]<li>[item]</li>[/each][conditional path="READABLE_META_KEY" not-empty="true"]READABLE_YES[else]READABLE_NO[/conditional]`, "group", childID),
				preview("p5", `[mrql query='type = group' limit="3"]`, "group", childID),
				preview("p6", `[mrql saved="anything"]`, "group", childID),
			)
		case 4:
			return chatToolCalls(
				preview("q1", `[mrql query='type = nonsense']`, "group", childID),
				preview("q2", `[partial name="READABLE_partial"]`, "group", childID),
				preview("q3", `[using type="group" id="`+fmt.Sprint(f.root.ID)+`"][property path="Name"][/using]`, "group", childID),
				call("q4", "get_revision", map[string]any{"number": 1, "field": "CustomHeader"}),
			)
		case 5:
			// The same aggregate and bucketed queries twice, in HTML and in CSS, so
			// the second render of each reads the query cache's shared containers.
			both := `[mrql query='type = group GROUP BY name COUNT()'][mrql query='type = group GROUP BY name'][mrql query='type = group GROUP BY name COUNT()'][mrql query='type = group GROUP BY name']`
			return chatToolCalls(
				call("r1", "preview", map[string]any{"field": "CustomHeader", "content": both, "css": both, "entityType": "group", "entityId": childID}),
				call("r2", "preview", map[string]any{"field": "CustomHeader", "content": `[mrql query='type = group']`, "entityType": "group", "entityId": childID}),
				call("r3", "preview", map[string]any{"field": "CustomHeader", "content": `<h2>[property path="Name"]</h2>`, "entityType": "group", "entityId": f.html.ID}),
			)
		default:
			return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomCSS": ".a{color:blue}"}, "done": true}))
		}
	}
}

// Criterion 1: no protected marker reaches the provider, every readable one
// does, every scripted Tool call succeeds, and the legend maps the Placeholders
// back to their values.
func TestPrivacyModeHarnessRunNeverSendsEntityValues(t *testing.T) {
	f := newPrivacyFixture(t)
	f.script = harnessScript(f)
	f.install(t, "harness")

	res := f.generate(t, "/v1/category/generateTemplate", nil)
	if res.code != http.StatusOK {
		t.Fatalf("run: %d %s", res.code, res.body)
	}
	bodies := f.allBodies()
	for _, marker := range f.protected {
		for _, form := range []string{marker, strings.ReplaceAll(marker, `"`, `\"`)} {
			if strings.Contains(bodies, form) {
				t.Errorf("protected marker %q reached the provider", marker)
			}
		}
	}
	for _, marker := range f.readable {
		if !strings.Contains(bodies, marker) {
			t.Errorf("readable marker %q never reached the provider", marker)
		}
	}
	results := f.toolResults()
	for _, id := range []string{"t1", "t2", "t3", "t4", "t5", "t6", "u1", "u2", "u3", "u4", "u5", "u6", "p1", "p2", "p3", "p4", "p5", "p6", "q1", "q2", "q3", "q4", "r1", "r2", "r3"} {
		text, ok := results[id]
		if !ok {
			t.Errorf("no result for call %s", id)
			continue
		}
		if strings.HasPrefix(text, "error:") {
			t.Errorf("scripted call %s failed instead of succeeding: %s", id, text)
		}
	}
	assertContains := func(id string, parts ...string) {
		t.Helper()
		for _, part := range parts {
			if !strings.Contains(results[id], part) {
				t.Errorf("result %s lacks %q: %s", id, part, results[id])
			}
		}
	}
	// Per-render positive assertions.
	assertContains("p1", "⟦", "<h1>⟦")
	assertContains("p2", "⟦", datasize.ByteSize(734928601).HumanReadable())
	assertContains("p3", "⟦")
	assertContains("p4", "⟦", "READABLE_YES")
	assertContains("p5", "mrql-results")
	assertContains("r2", "mrql-results", `class=\"cm\"`)
	assertContains("p6", "saved queries are not available in Privacy mode")
	assertContains("q1", "the query failed; details are hidden in Privacy mode; use check_mrql to check its syntax")
	assertContains("q2", "READABLE_PARTIAL_SOURCE", "READABLE_INNER_SOURCE", "⟦")
	assertContains("q3", "⟦")
	if strings.Contains(results["p4"], "READABLE_NO") {
		t.Error("the [conditional] took the wrong branch")
	}
	for _, id := range []string{"p1", "p2", "p3", "p4", "p5", "q2", "q3"} {
		if strings.Contains(results[id], "not found") || strings.Contains(results[id], "unavailable") {
			t.Errorf("render %s hid a failure: %s", id, results[id])
		}
	}

	var out struct {
		StopReason string `json:"stopReason"`
		Legend     []struct {
			Placeholder string `json:"placeholder"`
			Value       string `json:"value"`
		} `json:"legend"`
	}
	if err := json.Unmarshal(res.body, &out); err != nil {
		t.Fatal(err)
	}
	if out.StopReason != "done" {
		t.Fatalf("stop reason %q: %s", out.StopReason, res.body)
	}
	values := map[string]string{}
	for _, e := range out.Legend {
		values[e.Value] = e.Placeholder
		if !strings.Contains(bodies, e.Placeholder) && !strings.HasPrefix(e.Placeholder, "https://") {
			// not every Placeholder need have been sent, but most are
			continue
		}
	}
	for _, want := range []string{"PROT_GROUP_NAME", "PROT_RES_NAME", "PROT_META_VALUE", "PROT_REV_LABEL"} {
		if _, ok := values[want]; !ok {
			t.Errorf("the legend lacks %q", want)
		}
	}
	// Positive controls: each render names the intended entity's own Placeholders,
	// not just any Placeholder.
	mustShow := func(id string, wants ...string) {
		t.Helper()
		for _, want := range wants {
			ph, ok := values[want]
			if !ok {
				t.Errorf("render %s: the legend has no entry for %q", id, want)
				continue
			}
			if !strings.Contains(results[id], ph) {
				t.Errorf("render %s does not show the Placeholder of %q (%s): %s", id, want, ph, results[id])
			}
		}
	}
	mustShow("p1", "PROT_GROUP_NAME", "PROT_OWNER_NAME")
	mustShow("p2", "PROT_HASH_VALUE", "PROT_LOCATION/x", "PROT_ORIGINAL_NAME", "PROT_RES_NAME")
	mustShow("p3", "PROTSHARETOKEN0123456789abcdef12", "PROT_NOTE_NAME")
	mustShow("p4", "PROT_LIST_ITEM_A", "PROT_LIST_ITEM_B")
	mustShow("r2", "PROT_GROUP_NAME")
	mustShow("q3", "PROT_OWNER_NAME")
	mustShow("r3", `PROT_HTML <i>&"q"</i>`)
	// A protected Name with HTML-sensitive characters, rendered through a
	// preview: neither the raw nor the HTML-escaped real value may be sent.
	for _, form := range []string{`PROT_HTML <i>&"q"</i>`, html.EscapeString(`PROT_HTML <i>&"q"</i>`), `PROT_HTML &lt;i&gt;`} {
		if strings.Contains(bodies, form) {
			t.Errorf("the transformed protected value %q reached the provider", form)
		}
	}
	for _, id := range []string{"p1", "p2", "p3", "p4", "p5", "q2", "q3", "r1", "r2"} {
		if strings.Contains(results[id], "mrql-error") || strings.Contains(results[id], "shortcode-error") {
			t.Errorf("render %s shows an error where it must succeed: %s", id, results[id])
		}
	}
	// The aggregate and the bucketed query, twice in HTML and twice in CSS: stable
	// Placeholders for the bucket keys, and nothing raw.
	r1 := results["r1"]
	if phName, ok := values["PROT_GROUP_NAME"]; !ok || strings.Count(r1, phName) < 4 {
		t.Errorf("r1: the bucket and row keys must repeat as the same Placeholder in HTML and CSS: %s", r1)
	}
}

// leakingGenerator wraps a real generator (real wire adapter, capturing
// transport) and models a leak below the masking layer: the masker knows a
// value, and it still reaches the request.
type leakingGenerator struct {
	inner   application_context.TemplateGenerator
	mode    string
	mutate  func(in *application_context.TemplateGenerationInput)
	perform func(in *application_context.TemplateGenerationInput)
}

func (l *leakingGenerator) PrepareTemplateGeneration() (application_context.TemplateGenerationPrep, error) {
	return application_context.TemplateGenerationPrep{GenerationMode: l.mode, Generator: l}, nil
}

func (l *leakingGenerator) DiscloseTemplate(ctx context.Context, in application_context.TemplateGenerationInput, prompt string) (*application_context.TemplateDisclosure, error) {
	if l.mutate != nil {
		l.mutate(&in)
	}
	return l.inner.DiscloseTemplate(ctx, in, prompt)
}

func (l *leakingGenerator) GenerateTemplate(ctx context.Context, in application_context.TemplateGenerationInput, prompt, hash string) (*application_context.TemplateGenerationResult, error) {
	if l.mutate != nil {
		l.mutate(&in)
	}
	if l.perform != nil {
		l.perform(&in)
	}
	return l.inner.GenerateTemplate(ctx, in, prompt, hash)
}

// leakExecutor appends a value the masker knows to every lint result.
type leakExecutor struct {
	application_context.TemplateToolExecutor
	leak    string
	privacy *application_context.TemplatePrivacy
}

func (e *leakExecutor) Execute(ctx context.Context, call application_context.TemplateToolInvocation) (application_context.TemplateToolOutput, error) {
	out, err := e.TemplateToolExecutor.Execute(ctx, call)
	if call.Name == application_context.ToolLint {
		e.privacy.Text(e.leak)
		out.Text += "\n" + e.leak
	}
	return out, err
}

func (f *privacyFixture) transportClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.bodies = append(f.bodies, raw)
		f.calls++
		n := f.calls
		f.mu.Unlock()
		var req map[string]any
		_ = json.Unmarshal(raw, &req)
		encoded, _ := json.Marshal(f.script(n, req))
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(encoded)), Request: r}, nil
	})}
}

// Criterion 3, harness mode: a leak injected below the masking layer stops the
// run before that request is sent, with stop reason privacy_guard.
func TestPrivacyModeHarnessGuardStopsALeakBelowTheMasker(t *testing.T) {
	f := newPrivacyFixture(t)
	leak := "LEAKVALUE \"quote\"\nnewline — ünï"
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return chatToolCalls(call("t1", "lint", map[string]any{"field": "CustomHeader", "content": "x"}))
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomCSS": ".a{}"}, "done": true}))
	}
	f.install(t, "harness")
	conv := application_context.NewGenerationConversationProvider("deepseek", "test-key", "ds-model", f.transportClient())
	inner := application_context.NewTemplateHarnessGenerator(conv, application_context.TemplateGenerationConfig{
		APIKey: "k", Model: "ds-model", Harness: &application_context.TemplateHarnessSettings{MaxTurns: 5},
	})
	f.tc.AppCtx.SetTemplateGenerator(&leakingGenerator{inner: inner, mode: application_context.TemplateGenerationModeHarness,
		perform: func(in *application_context.TemplateGenerationInput) {
			run := *in.Run
			run.Executor = &leakExecutor{TemplateToolExecutor: run.Executor, leak: leak, privacy: in.Privacy}
			in.Run = &run
		}})
	f.mu.Lock()
	f.bodies, f.calls = nil, 0
	f.mu.Unlock()

	res := f.generate(t, "/v1/category/generateTemplate", nil)
	if res.code != http.StatusInternalServerError {
		t.Fatalf("status %d: %s", res.code, res.body)
	}
	var out struct {
		Error      string `json:"error"`
		StopReason string `json:"stopReason"`
		Transcript []struct {
			Turn   int    `json:"turn"`
			Kind   string `json:"kind"`
			CallID string `json:"callId"`
			Sent   bool   `json:"sent"`
		} `json:"transcript"`
		Legend []struct{ Value string } `json:"legend"`
	}
	_ = json.Unmarshal(res.body, &out)
	if out.Error != "Privacy mode stopped a request that contained an entity value. Nothing was sent" || out.StopReason != "privacy_guard" {
		t.Fatalf("body: %s", res.body)
	}
	if strings.Contains(f.allBodies(), "ünï") || f.calls != 1 {
		t.Fatalf("the leak was sent (calls=%d)", f.calls)
	}
	// The Tool result the blocked request would have carried was made in Turn 1;
	// find it by its call ID, not by Turn.
	foundResult := false
	for _, rec := range out.Transcript {
		switch {
		case rec.Kind == "tool_result" && rec.CallID == "t1":
			foundResult = true
			if rec.Sent {
				t.Errorf("the result carried by the blocked request claims it was sent: %+v", rec)
			}
		case rec.Turn == 2 && rec.Sent:
			t.Errorf("a refused Turn's record claims it was sent: %+v", rec)
		case rec.Turn == 1 && (rec.Kind == "seed" || rec.Kind == "assistant") && !rec.Sent:
			t.Errorf("an earlier record that really was sent must stay sent: %+v", rec)
		}
	}
	if !foundResult {
		t.Error("the transcript has no result for call t1")
	}
	found := false
	for _, e := range out.Legend {
		found = found || strings.HasPrefix(e.Value, "LEAKVALUE")
	}
	if !found {
		t.Errorf("the error carries the legend with the transcript: %s", res.body)
	}
}

// Criterion 4: the administrator lock and the plugin refusal.
func TestPrivacyModeRefusals(t *testing.T) {
	f := newPrivacyFixture(t)
	f.script = harnessScript(f)
	f.install(t, "harness")
	path := "/v1/category/generateTemplate"
	rec := f.tc.MakeRequest(http.MethodPost, path, f.requestBody(map[string]any{"dryRun": true, "previewPlugins": true}))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "plugin previews are not available in Privacy mode") {
		t.Fatalf("privacy plus plugin previews: %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.tc.MakeRequest(http.MethodPost, path, f.requestBody(map[string]any{"dryRun": true, "privacyOff": true, "previewPlugins": true})); rec.Code != http.StatusOK {
		t.Fatalf("privacy off allows plugin previews: %d %s", rec.Code, rec.Body.String())
	}
	lock := map[string]any{"values": map[string]string{"ai_template_privacy_mode": "always"}}
	if rec := f.tc.MakeRequest(http.MethodPut, "/v1/admin/settings", lock); rec.Code != http.StatusOK {
		t.Fatalf("lock: %d %s", rec.Code, rec.Body.String())
	}
	rec = f.tc.MakeRequest(http.MethodPost, path, f.requestBody(map[string]any{"dryRun": true, "privacyOff": true}))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Privacy mode is required on this server") {
		t.Fatalf("locked, privacy off, dry run: %d %s", rec.Code, rec.Body.String())
	}
	rec = f.tc.MakeRequest(http.MethodPost, path, f.requestBody(map[string]any{"dryRun": true}))
	var d struct {
		Privacy struct{ On, Locked bool } `json:"privacy"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &d)
	if rec.Code != http.StatusOK || !d.Privacy.On || !d.Privacy.Locked {
		t.Fatalf("locked disclosure: %d %s", rec.Code, rec.Body.String())
	}
}

// The hash covers the flag: approving with privacy on and sending with it off is a 409.
func TestPrivacyModeHashCoversTheFlag(t *testing.T) {
	f := newPrivacyFixture(t)
	f.script = harnessScript(f)
	f.install(t, "harness")
	path := "/v1/category/generateTemplate"
	hash := disclose(t, f.tc, path, f.requestBody(nil))
	rec := f.tc.MakeRequest(http.MethodPost, path, withHash(f.requestBody(map[string]any{"privacyOff": true}), hash))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func classicReply(content string) map[string]any {
	return map[string]any{
		"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}},
		"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 20},
	}
}

func (f *privacyFixture) classicBody(extra map[string]any) map[string]any {
	body := f.requestBody(extra)
	delete(body, "previewMRQL")
	return body
}

// Criterion 2: a classic request, repair call included, sends no protected value.
func TestPrivacyModeClassicRequestAndRepairSendNoEntityValues(t *testing.T) {
	f := newPrivacyFixture(t)
	f.repair = true
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return classicReply(`{"slots":{"CustomHeader":"[conditional path='x'] never closed","CustomHeaderCSS":""},"explanation":"first"}`)
		}
		return classicReply(`{"slots":{"CustomHeader":"<h1>[property path='Name']</h1>","CustomHeaderCSS":".a{color:blue}"},"explanation":"fixed"}`)
	}
	f.install(t, "classic")
	path := "/v1/category/generateTemplate"
	body := f.classicBody(map[string]any{"slot": "CustomHeader", "mode": "html", "target": "slot"})
	hash := disclose(t, f.tc, path, body)
	rec := f.tc.MakeRequest(http.MethodPost, path, withHash(body, hash))
	if rec.Code != http.StatusOK {
		t.Fatalf("classic run: %d %s", rec.Code, rec.Body.String())
	}
	if f.calls != 2 {
		t.Fatalf("expected a first call and a repair call, got %d", f.calls)
	}
	bodies := f.allBodies()
	for _, marker := range f.protected {
		if strings.Contains(bodies, marker) {
			t.Errorf("protected marker %q reached the provider in classic mode", marker)
		}
	}
	for _, marker := range []string{"READABLE_CATEGORY_NAME", "READABLE_META_KEY", "READABLE_USER_REQUEST"} {
		if !strings.Contains(bodies, marker) {
			t.Errorf("readable marker %q missing", marker)
		}
	}
	var out struct {
		Legend []struct{ Value string } `json:"legend"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Legend) == 0 {
		t.Errorf("the classic result carries the legend: %s", rec.Body.String())
	}
}

// classicLeaker builds a classic generator over the real adapter whose input is
// altered below the masker.
func (f *privacyFixture) classicLeaker(fan bool, mutate func(in *application_context.TemplateGenerationInput)) {
	cfg := application_context.TemplateGenerationConfig{APIKey: "k", Model: "ds-model"}
	inner := application_context.NewTemplateGenerator(application_context.NewGenerationTemplateDraftProvider("deepseek", "k", "ds-model", f.transportClient()), cfg)
	f.tc.AppCtx.SetTemplateGenerator(&leakingGenerator{inner: inner, mode: application_context.TemplateGenerationModeClassic, mutate: mutate})
}

func TestPrivacyModeClassicGuardCatchesALeakInTheFirstCall(t *testing.T) {
	f := newPrivacyFixture(t)
	leak := "LEAKFIRST \"q\"\nline — ünï"
	f.script = func(n int, req map[string]any) map[string]any {
		return classicReply(`{"content":".a{}","explanation":"x"}`)
	}
	f.install(t, "classic")
	f.classicLeaker(false, func(in *application_context.TemplateGenerationInput) {
		in.Privacy.Text(leak)
		in.Samples = append(in.Samples, application_context.TemplateSample{"Name": leak})
	})
	f.mu.Lock()
	f.bodies, f.calls = nil, 0
	f.mu.Unlock()
	path := "/v1/category/generateTemplate"
	body := f.classicBody(nil)
	hash := disclose(t, f.tc, path, body)
	rec := f.tc.MakeRequest(http.MethodPost, path, withHash(body, hash))
	if rec.Code != http.StatusInternalServerError || f.calls != 0 || strings.Contains(rec.Body.String(), "ünï") {
		t.Fatalf("first-call leak: %d calls=%d %s", rec.Code, f.calls, rec.Body.String())
	}
}

func TestPrivacyModeClassicGuardCatchesALeakInTheRepairCall(t *testing.T) {
	f := newPrivacyFixture(t)
	leak := "LEAKREPAIR \"q\"\nline — ünï"
	var masker *application_context.TemplatePrivacy
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			// The masker learns the value only now: the first request passed, the repair call must not.
			masker.Text(leak)
			return classicReply(`{"slots":{"CustomHeader":"[conditional path='x'] never closed","CustomHeaderCSS":""},"explanation":"first"}`)
		}
		return classicReply(`{"slots":{"CustomHeader":"<h1>x</h1>","CustomHeaderCSS":""},"explanation":"fixed"}`)
	}
	f.repair = true
	f.install(t, "classic")
	f.classicLeaker(false, func(in *application_context.TemplateGenerationInput) {
		masker = in.Privacy
		in.Samples = append(in.Samples, application_context.TemplateSample{"Name": leak})
	})
	f.mu.Lock()
	f.bodies, f.calls = nil, 0
	f.mu.Unlock()
	path := "/v1/category/generateTemplate"
	body := f.classicBody(map[string]any{"slot": "CustomHeader", "mode": "html"})
	hash := disclose(t, f.tc, path, body)
	f.mu.Lock()
	f.bodies, f.calls = nil, 0
	f.mu.Unlock()
	rec := f.tc.MakeRequest(http.MethodPost, path, withHash(body, hash))
	if rec.Code != http.StatusInternalServerError || f.calls != 1 {
		t.Fatalf("repair-call leak: %d calls=%d %s", rec.Code, f.calls, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "fixed") || strings.Contains(rec.Body.String(), "content") {
		t.Fatalf("no Draft may come back: %s", rec.Body.String())
	}
}

// gatedDraftProvider holds the pair call of one section until another section's
// call has returned, so the masker can learn a value between the two requests.
type gatedDraftProvider struct {
	application_context.TemplateDraftProvider
	gateSection string
	gate        chan struct{}
	onFirstDone func()
}

func (g *gatedDraftProvider) GenerateDraft(ctx context.Context, req application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	if strings.Contains(req.User, "Task: fill the "+g.gateSection+" section") {
		select {
		case <-g.gate:
		case <-time.After(5 * time.Second):
		}
		return g.TemplateDraftProvider.GenerateDraft(ctx, req)
	}
	resp, err := g.TemplateDraftProvider.GenerateDraft(ctx, req)
	if strings.Contains(req.User, "Task: fill the ") {
		g.onFirstDone()
		close(g.gate)
	}
	return resp, err
}

func TestPrivacyModeClassicGuardCatchesALeakInOneFanOutSection(t *testing.T) {
	f := newPrivacyFixture(t)
	leak := "LEAKPAIR \"q\"\nline — ünï"
	var masker *application_context.TemplatePrivacy
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return classicReply(`{"classPrefix":"t","palette":"p","layout":"l","sharedCssAdditions":"","sections":{"CustomHeader":{"fill":true,"intent":"i"},"CustomSidebar":{"fill":true,"intent":"i"}},"explanation":"e"}`)
		}
		return classicReply(`{"slots":{"CustomHeader":"<h1>x</h1>","CustomHeaderCSS":".t-a{}"},"explanation":"ok"}`)
	}
	f.install(t, "classic")
	real := application_context.NewGenerationTemplateDraftProvider("deepseek", "k", "ds-model", f.transportClient())
	gated := &gatedDraftProvider{TemplateDraftProvider: real, gateSection: "CustomSidebar", gate: make(chan struct{}), onFirstDone: func() { masker.Text(leak) }}
	inner := application_context.NewTemplateGenerator(gated, application_context.TemplateGenerationConfig{APIKey: "k", Model: "ds-model"})
	f.tc.AppCtx.SetTemplateGenerator(&leakingGenerator{inner: inner, mode: application_context.TemplateGenerationModeClassic,
		mutate: func(in *application_context.TemplateGenerationInput) {
			masker = in.Privacy
			in.Samples = append(in.Samples, application_context.TemplateSample{"Name": leak})
		}})
	path := "/v1/category/generateTemplate"
	body := f.classicBody(map[string]any{"target": "bundle", "slots": []string{"CustomHeader", "CustomSidebar"}, "mode": "html"})
	delete(body, "slot")
	hash := disclose(t, f.tc, path, body)
	f.mu.Lock()
	f.bodies, f.calls = nil, 0
	f.mu.Unlock()
	rec := f.tc.MakeRequest(http.MethodPost, path, withHash(body, hash))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "CustomHeader") {
		t.Fatalf("fan-out leak: %d %s", rec.Code, rec.Body.String())
	}
	if f.calls != 2 {
		t.Fatalf("the plan and the sibling's pair were sent, the leaking section's was not: calls=%d", f.calls)
	}
}

// A readable Replay leaf and a model identifier that equal a masked value do not trip the guard.
func TestPrivacyModeGuardHasNoFalsePositiveOnReplayOrModel(t *testing.T) {
	f := newPrivacyFixture(t)
	f.script = harnessScript(f)
	f.install(t, "harness")
	// A library value equal to the model identifier and to a word the model said.
	f.tc.DB.Model(&models.Tag{}).Where("name = ?", "PROT_TAG_NAME").Update("name", "ds-model")
	res := f.generate(t, "/v1/category/generateTemplate", nil)
	if res.code != http.StatusOK {
		t.Fatalf("a value equal to the model id must not trip the guard: %d %s", res.code, res.body)
	}
}
