package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mahresources/application_context"
)

// goldenRunFixture is where the recorded run lives. The frontend's unit tests
// and the harness E2E replay it, so a change to the wire on either side fails
// a test instead of only the browser at runtime.
const goldenRunFixture = "../../e2e/fixtures/template-run-golden.json"

// goldenBody is the request the category editor sends for the CustomHeader
// cluster, so the E2E can drive the recorded run through the real page.
func goldenBody() map[string]any {
	return map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "a header", "privacyOff": true}
}

const (
	goldenHeader    = `<h1 class="gold">[property path="Name"]</h1>`
	goldenHeaderCSS = `.gold{color:goldenrod}`
)

type goldenRun struct {
	Disclosure json.RawMessage `json:"disclosure"`
	Stream     string          `json:"stream"`
}

// TestHarnessRunGoldenStream records a scripted run through the real handler:
// the dry-run Disclosure and the event stream, with run IDs and timings
// normalised. Set UPDATE_GOLDEN=1 to rewrite the fixture after an intended
// wire change.
func TestHarnessRunGoldenStream(t *testing.T) {
	tc := SetupTestEnv(t)
	n := func(v int) *int { return &v }
	conv := &fakeConversation{step: func(_ context.Context, call int, _ application_context.ConversationRequest) (application_context.ConversationResponse, error) {
		usage := application_context.ConversationUsage{Prompt: n(1000 * call), Completion: n(50), CacheHit: n(0), CacheMiss: n(1000 * call)}
		if call == 1 {
			r := toolReply(
				apiToolCall("c1", application_context.ToolGetShortcodeDocs, map[string]any{"name": "meta"}),
				apiToolCall("c2", application_context.ToolLint, map[string]any{"field": "CustomHeader", "content": goldenHeader}),
			)
			r.Usage = usage
			return r, nil
		}
		r := toolReply(apiSubmit("s1", map[string]string{"CustomHeader": goldenHeader, "CustomHeaderCSS": goldenHeaderCSS}, true))
		r.Usage = usage
		return r, nil
	}}
	installHarness(tc, conv, 5)
	srv := newHarnessServer(t, tc)

	dry := map[string]any{"dryRun": true}
	for k, v := range goldenBody() {
		dry[k] = v
	}
	dryResp := tc.MakeRequest(http.MethodPost, categoryGenerate, dry)
	if dryResp.Code != http.StatusOK {
		t.Fatalf("dry run: %d %s", dryResp.Code, dryResp.Body.String())
	}
	hash := disclose2(t, dryResp.Body.String())

	resp, _ := postStream(t, srv.URL, categoryGenerate, withHash(goldenBody(), hash), sameClient)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: %d", resp.StatusCode)
	}
	frames := newSSEReader(resp.Body).readUntilEnd(t, 10*time.Second)
	if frames[len(frames)-1].Event != "result" {
		t.Fatalf("run ended with %v", eventNames(frames))
	}

	var stream bytes.Buffer
	for _, f := range frames {
		stream.WriteString("event: " + f.Event + "\ndata: " + normaliseGolden(t, f.Data) + "\n\n")
	}
	got := goldenRun{Disclosure: json.RawMessage(normaliseGolden(t, dryResp.Body.String())), Stream: stream.String()}
	out, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, '\n')

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenRunFixture), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenRunFixture, out, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenRunFixture)
	if err != nil {
		t.Fatalf("%v (run with UPDATE_GOLDEN=1 to create it)", err)
	}
	if !bytes.Equal(want, out) {
		t.Fatalf("the recorded run changed; if the wire change is intended, rerun with UPDATE_GOLDEN=1 and update the frontend that reads it.\ngot:\n%s", out)
	}
}

// normaliseGolden replaces or drops the values that differ between runs (run
// IDs, durations, the grounding hash) and cuts long text, so the fixture is stable,
// and re-encodes the JSON with sorted keys.
func normaliseGolden(t *testing.T, data string) string {
	t.Helper()
	var v any
	dec := json.NewDecoder(bytes.NewReader([]byte(data)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("frame data is not JSON: %v: %s", err, data)
	}
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			// A record whose text is cut takes the cut text's size, so a docs
			// edit that changes the prompt's length does not churn the fixture.
			if str, ok := x["text"].(string); ok && len(str) > 120 {
				if _, has := x["bytes"]; has {
					x["bytes"] = len(str[:120] + " [cut for the fixture]")
				}
			}
			for k, val := range x {
				switch k {
				case "runId":
					x[k] = "run-golden"
				case "durationMs":
					// omitempty drops it from a record that took under 1 ms,
					// so whether it is present at all depends on timing.
					delete(x, k)
				case "groundingHash":
					x[k] = "hash-golden"
				case "text":
					// Prompt and docs text changes with every docs edit; the
					// frontend reads its shape, not its words.
					if str, ok := val.(string); ok && len(str) > 120 {
						x[k] = str[:120] + " [cut for the fixture]"
					}
				default:
					x[k] = walk(val)
				}
			}
		case []any:
			for i := range x {
				x[i] = walk(x[i])
			}
		}
		return v
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(walk(v)); err != nil {
		t.Fatal(err)
	}
	return string(bytes.TrimRight(buf.Bytes(), "\n"))
}
