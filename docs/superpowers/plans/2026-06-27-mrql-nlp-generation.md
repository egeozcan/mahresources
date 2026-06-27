# MRQL Natural-Language Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a CSRF-protected web-only "Describe results" flow that generates MRQL through DeepSeek, validates/lints it locally, explains it, and lets the user run it explicitly from the existing `/mrql` editor.

**Architecture:** Build a backend generation service with an injectable provider client, a generated-query lint layer in the `mrql` package, and a new `POST /v1/mrql/generate` handler that is deliberately not read-via-POST. Extend the existing Alpine/CodeMirror MRQL editor with generation state, stale-response protection, explanation display, and explicit apply behavior for invalid or stale drafts. Keep execution on the existing `/v1/mrql` path.

**Tech Stack:** Go, GORM, Gorilla Mux, MRQL parser/validator, Pongo2 templates, Alpine.js, CodeMirror, Vite, Playwright, Docusaurus docs.

**Spec:** `docs/superpowers/specs/2026-06-27-mrql-nlp-generation-design.md`

---

## File Structure

**New files:**

- `mrql/generation_lint.go` — generator-specific lint rules over the parsed MRQL AST.
- `mrql/generation_lint_test.go` — table tests for generated-query lint.
- `application_context/mrql_generation.go` — generator-facing request/result types, prompt building, provider interface, service orchestration, size limits, and error values.
- `application_context/mrql_generation_test.go` — unit tests using a fake provider.
- `application_context/mrql_generation_rate_limiter.go` — small in-memory per-key limiter for provider calls.
- `application_context/mrql_generation_rate_limiter_test.go` — limiter unit tests.
- `application_context/deepseek_client.go` — DeepSeek HTTP client and response parsing.
- `application_context/deepseek_client_test.go` — `httptest.Server` coverage for request/response/error behavior.
- `server/api_tests/mrql_generate_test.go` — API tests for route, auth, CSRF, fake provider, and error mapping.
- `e2e/tests/mrql-generate.spec.ts` — browser tests for generate/apply/error states.

**Modified files:**

- `application_context/context.go` — add DeepSeek config fields and `MRQLGenerator` getter/setter.
- `main.go` — parse env-only DeepSeek config and wire it into `MahresourcesConfig`.
- `server/api_handlers/mrql_api_handlers.go` — add request/response structs and handler for `/v1/mrql/generate`.
- `server/routes.go` — register `POST /v1/mrql/generate` without adding it to read-via-POST.
- `server/routes_openapi.go` — document the new generation endpoint.
- `src/components/mrqlEditor.js` — generation state/methods, stale-response handling, apply behavior, saved-query/result clearing.
- `templates/mrql.tpl` — generation panel UI and a11y/status markup.
- `e2e/pages/MRQLPage.ts` — helpers and locators for generation tests.
- `docs-site/docs/features/mrql.md` — user/admin privacy note.
- `docs-site/docs/configuration/advanced.md` — DeepSeek env config notes.
- `CLAUDE.md` — configuration table update for DeepSeek env vars.

---

## Task 1: Generated-Query Lint

**Files:**
- Create: `mrql/generation_lint.go`
- Create: `mrql/generation_lint_test.go`

- [ ] **Step 1: Write failing lint tests**

Create `mrql/generation_lint_test.go`:

```go
package mrql

import (
	"strings"
	"testing"
)

func TestLintGeneratedQuery(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantValid bool
		wantMsg   string
	}{
		{
			name:      "valid modest resource query",
			query:     `type = resource AND contentType ~ "image/*" LIMIT 50`,
			wantValid: true,
		},
		{
			name:      "limit too large",
			query:     `type = resource LIMIT 1000000`,
			wantValid: false,
			wantMsg:   "LIMIT must be between 1 and 500",
		},
		{
			name:      "offset too large",
			query:     `type = resource LIMIT 50 OFFSET 10001`,
			wantValid: false,
			wantMsg:   "OFFSET must be between 0 and 10000",
		},
		{
			name:      "text wildcard only",
			query:     `TEXT ~ "*" LIMIT 50`,
			wantValid: false,
			wantMsg:   "TEXT search must contain at least one alphanumeric term",
		},
		{
			name:      "text punctuation only",
			query:     `TEXT ~ "!!!" LIMIT 50`,
			wantValid: false,
			wantMsg:   "TEXT search must contain at least one alphanumeric term",
		},
		{
			name:      "string category rejected",
			query:     `type = group AND category = "Invoices" LIMIT 50`,
			wantValid: false,
			wantMsg:   "category requires a numeric ID in generated MRQL",
		},
		{
			name:      "numeric category allowed",
			query:     `type = group AND category = 7 LIMIT 50`,
			wantValid: true,
		},
		{
			name:      "string note type rejected",
			query:     `type = note AND noteType = "Meeting" LIMIT 50`,
			wantValid: false,
			wantMsg:   "noteType requires a numeric ID in generated MRQL",
		},
		{
			name:      "string resource category rejected",
			query:     `type = resource AND resourceCategory = "Scans" LIMIT 50`,
			wantValid: false,
			wantMsg:   "resourceCategory requires a numeric ID in generated MRQL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.query, err)
			}
			if err := Validate(parsed); err != nil {
				t.Fatalf("Validate(%q): %v", tc.query, err)
			}

			errs := LintGeneratedQuery(parsed)
			if tc.wantValid && len(errs) != 0 {
				t.Fatalf("expected no lint errors, got %#v", errs)
			}
			if !tc.wantValid {
				if len(errs) == 0 {
					t.Fatalf("expected lint error containing %q", tc.wantMsg)
				}
				got := errs[0]["message"]
				if got == nil || !strings.Contains(got.(string), tc.wantMsg) {
					t.Fatalf("first lint error = %#v, want message containing %q", errs[0], tc.wantMsg)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run the lint test and verify RED**

Run:

```bash
go test --tags 'json1 fts5' ./mrql -run TestLintGeneratedQuery -count=1
```

Expected: compile failure like `undefined: LintGeneratedQuery`.

- [ ] **Step 3: Implement generated-query lint**

Create `mrql/generation_lint.go`:

```go
package mrql

import "unicode"

const (
	MaxGeneratedLimit  = 500
	MaxGeneratedOffset = 10000
)

func LintGeneratedQuery(q *Query) []map[string]any {
	var errs []map[string]any
	add := func(pos int, msg string) {
		err := map[string]any{"message": msg}
		if pos >= 0 {
			err["pos"] = pos
			err["length"] = 1
		}
		errs = append(errs, err)
	}

	if q.Limit > MaxGeneratedLimit || q.Limit == 0 {
		add(-1, "LIMIT must be between 1 and 500 for generated MRQL")
	}
	if q.Offset > MaxGeneratedOffset {
		add(-1, "OFFSET must be between 0 and 10000 for generated MRQL")
	}

	walkGeneratedNode(q.Where, func(n Node) {
		switch expr := n.(type) {
		case *TextSearchExpr:
			if !containsAlphaNum(expr.Value.Value) {
				add(expr.Pos(), "TEXT search must contain at least one alphanumeric term")
			}
		case *ComparisonExpr:
			name := expr.Field.Name()
			if requiresGeneratedNumericID(name) {
				if _, ok := expr.Value.(*NumberLiteral); !ok {
					add(expr.Pos(), name+" requires a numeric ID in generated MRQL")
				}
			}
		case *InExpr:
			name := expr.Field.Name()
			if requiresGeneratedNumericID(name) {
				for _, v := range expr.Values {
					if _, ok := v.(*NumberLiteral); !ok {
						add(expr.Pos(), name+" requires a numeric ID in generated MRQL")
						break
					}
				}
			}
		}
	})

	return errs
}

func walkGeneratedNode(n Node, visit func(Node)) {
	if n == nil {
		return
	}
	visit(n)
	switch x := n.(type) {
	case *BinaryExpr:
		walkGeneratedNode(x.Left, visit)
		walkGeneratedNode(x.Right, visit)
	case *NotExpr:
		walkGeneratedNode(x.Expr, visit)
	}
}

func containsAlphaNum(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func requiresGeneratedNumericID(field string) bool {
	switch field {
	case "category", "resourceCategory", "noteType":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run lint tests and verify GREEN**

Run:

```bash
go test --tags 'json1 fts5' ./mrql -run TestLintGeneratedQuery -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 1**

Run:

```bash
git add mrql/generation_lint.go mrql/generation_lint_test.go
git commit -m "feat(mrql): lint generated queries"
```

---

## Task 2: Generator Service, Prompt Contract, DeepSeek Client

**Files:**
- Create: `application_context/mrql_generation.go`
- Create: `application_context/mrql_generation_test.go`
- Create: `application_context/deepseek_client.go`
- Create: `application_context/deepseek_client_test.go`
- Modify: `application_context/context.go`

- [ ] **Step 1: Write failing generator service tests**

Create `application_context/mrql_generation_test.go`:

```go
package application_context

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeMRQLDraftProvider struct {
	query       string
	explanation string
	err         error
	seenPrompt  string
}

func (f *fakeMRQLDraftProvider) GenerateDraft(ctx context.Context, prompt string) (providerMRQLDraft, error) {
	f.seenPrompt = prompt
	if f.err != nil {
		return providerMRQLDraft{}, f.err
	}
	return providerMRQLDraft{Query: f.query, Explanation: f.explanation}, nil
}

func TestMRQLGeneratorSuccessValidatesAndExplains(t *testing.T) {
	provider := &fakeMRQLDraftProvider{
		query:       `type = resource AND contentType ~ "image/*" LIMIT 50`,
		explanation: "Finds up to 50 image resources.",
	}
	gen := NewMRQLGenerator(provider, MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second})

	got, err := gen.GenerateMRQL(context.Background(), "show image resources")
	if err != nil {
		t.Fatalf("GenerateMRQL: %v", err)
	}
	if !got.Valid {
		t.Fatalf("expected valid result, got %#v", got.Errors)
	}
	if got.Query != provider.query || got.Explanation != provider.explanation {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestMRQLGeneratorMissingKey(t *testing.T) {
	gen := NewMRQLGenerator(&fakeMRQLDraftProvider{}, MRQLGenerationConfig{Model: "deepseek-v4-pro", Timeout: time.Second})
	_, err := gen.GenerateMRQL(context.Background(), "anything")
	if !errors.Is(err, ErrMRQLGenerationNotConfigured) {
		t.Fatalf("expected ErrMRQLGenerationNotConfigured, got %v", err)
	}
}

func TestMRQLGeneratorPromptLength(t *testing.T) {
	gen := NewMRQLGenerator(&fakeMRQLDraftProvider{}, MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second})
	_, err := gen.GenerateMRQL(context.Background(), strings.Repeat("x", MaxMRQLGenerationPromptLength+1))
	if !errors.Is(err, ErrMRQLGenerationBadRequest) {
		t.Fatalf("expected bad request for long prompt, got %v", err)
	}
}

func TestMRQLGeneratorInvalidGeneratedQuery(t *testing.T) {
	provider := &fakeMRQLDraftProvider{query: `type = resource LIMIT 1000000`, explanation: "Too many."}
	gen := NewMRQLGenerator(provider, MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second})

	got, err := gen.GenerateMRQL(context.Background(), "all resources")
	if err != nil {
		t.Fatalf("GenerateMRQL should return invalid result, not transport error: %v", err)
	}
	if got.Valid {
		t.Fatalf("expected invalid result")
	}
	if len(got.Errors) == 0 || !strings.Contains(got.Errors[0]["message"].(string), "LIMIT") {
		t.Fatalf("expected LIMIT lint error, got %#v", got.Errors)
	}
}

func TestMRQLGeneratorDoesNotLeakLocalVocabularyIntoPrompt(t *testing.T) {
	provider := &fakeMRQLDraftProvider{query: `TEXT ~ "invoice" LIMIT 50`, explanation: "Finds invoice text."}
	gen := NewMRQLGenerator(provider, MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second})

	_, err := gen.GenerateMRQL(context.Background(), "find invoices")
	if err != nil {
		t.Fatalf("GenerateMRQL: %v", err)
	}
	for _, forbidden := range []string{"SecretLocalTag", "PrivateCategory", "ConfidentialNoteType", "HiddenResourceCategory"} {
		if strings.Contains(provider.seenPrompt, forbidden) {
			t.Fatalf("prompt leaked local vocabulary %q in %q", forbidden, provider.seenPrompt)
		}
	}
}
```

- [ ] **Step 2: Run generator tests and verify RED**

Run:

```bash
go test --tags 'json1 fts5' ./application_context -run 'TestMRQLGenerator' -count=1
```

Expected: compile failure for missing generator types.

- [ ] **Step 3: Implement generator service and context seam**

Create `application_context/mrql_generation.go` with these exported types and functions:

```go
package application_context

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"mahresources/mrql"
)

const (
	MaxMRQLGenerationPromptLength      = 2000
	MaxMRQLGeneratedQueryLength        = 2000
	MaxMRQLGeneratedExplanationLength  = 1000
	DefaultDeepSeekMRQLGenerationModel = "deepseek-v4-pro"
	DefaultDeepSeekMRQLGenerationTimeout = 20 * time.Second
)

var (
	ErrMRQLGenerationNotConfigured = errors.New("mrql generation is not configured")
	ErrMRQLGenerationBadRequest    = errors.New("bad mrql generation request")
	ErrMRQLGenerationProvider      = errors.New("mrql generation provider error")
	ErrMRQLGenerationTimeout       = errors.New("mrql generation provider timeout")
)

type MRQLGenerationConfig struct {
	APIKey  string
	Model   string
	Timeout time.Duration
}

type MRQLGenerationResult struct {
	Query       string           `json:"query"`
	Explanation string           `json:"explanation"`
	Valid       bool             `json:"valid"`
	Errors      []map[string]any `json:"errors"`
}

type MRQLGenerator interface {
	GenerateMRQL(ctx context.Context, prompt string) (*MRQLGenerationResult, error)
}

type providerMRQLDraft struct {
	Query       string
	Explanation string
}

type MRQLDraftProvider interface {
	GenerateDraft(ctx context.Context, prompt string) (providerMRQLDraft, error)
}

type defaultMRQLGenerator struct {
	provider MRQLDraftProvider
	config   MRQLGenerationConfig
}

func NewMRQLGenerator(provider MRQLDraftProvider, cfg MRQLGenerationConfig) MRQLGenerator {
	if cfg.Model == "" {
		cfg.Model = DefaultDeepSeekMRQLGenerationModel
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultDeepSeekMRQLGenerationTimeout
	}
	return &defaultMRQLGenerator{provider: provider, config: cfg}
}

func (g *defaultMRQLGenerator) GenerateMRQL(ctx context.Context, prompt string) (*MRQLGenerationResult, error) {
	if strings.TrimSpace(g.config.APIKey) == "" {
		return nil, ErrMRQLGenerationNotConfigured
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || len(prompt) > MaxMRQLGenerationPromptLength {
		return nil, fmt.Errorf("%w: prompt must be between 1 and %d characters", ErrMRQLGenerationBadRequest, MaxMRQLGenerationPromptLength)
	}

	callCtx, cancel := context.WithTimeout(ctx, g.config.Timeout)
	defer cancel()

	draft, err := g.provider.GenerateDraft(callCtx, buildMRQLGenerationPrompt(prompt))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return nil, ErrMRQLGenerationTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrMRQLGenerationProvider, err)
	}

	query := strings.TrimSpace(draft.Query)
	explanation := strings.TrimSpace(draft.Explanation)
	if query == "" || len(query) > MaxMRQLGeneratedQueryLength ||
		explanation == "" || len(explanation) > MaxMRQLGeneratedExplanationLength {
		return nil, fmt.Errorf("%w: provider returned an invalid draft shape", ErrMRQLGenerationProvider)
	}

	result := &MRQLGenerationResult{Query: query, Explanation: explanation, Valid: true, Errors: []map[string]any{}}
	parsed, parseErr := mrql.Parse(query)
	if parseErr != nil {
		result.Valid = false
		result.Errors = []map[string]any{{"message": parseErr.Error()}}
		return result, nil
	}
	if validateErr := mrql.Validate(parsed); validateErr != nil {
		result.Valid = false
		result.Errors = []map[string]any{{"message": validateErr.Error()}}
		return result, nil
	}
	if lintErrs := mrql.LintGeneratedQuery(parsed); len(lintErrs) > 0 {
		result.Valid = false
		result.Errors = lintErrs
	}
	return result, nil
}

func buildMRQLGenerationPrompt(userPrompt string) string {
	return strings.Join([]string{
		"Generate one Mahresources MRQL query from the user request.",
		"Return strict JSON only with keys query and explanation.",
		"Use only MRQL syntax rules and values explicitly present in the user request.",
		"Do not use local tags, categories, note types, resource categories, group names, filenames, or metadata keys unless the user typed them.",
		"Clause order: expression, SCOPE, GROUP BY, ORDER BY, LIMIT, OFFSET.",
		"Strings must be double-quoted.",
		"Use TEXT only as TEXT ~ \"plain words\" with at least one alphanumeric word.",
		"Use contentType ~ \"image/*\" for MIME patterns.",
		"Use relative dates like -30d or supported date functions, never natural-language dates.",
		"category, resourceCategory, and noteType require numeric IDs; if no ID is present, prefer tags, name, or TEXT.",
		"GROUP BY requires explicit type. Use COUNT() for aggregate counts.",
		"Add LIMIT 50 for broad queries unless the user asks for another small limit.",
		"User request: " + userPrompt,
	}, "\n")
}
```

Modify `application_context/context.go`:

```go
type MahresourcesConfig struct {
	// existing fields...
	DeepSeekAPIKey  string
	DeepSeekModel   string
	DeepSeekTimeout time.Duration
}

type MahresourcesInputConfig struct {
	// existing fields...
	DeepSeekAPIKey  string
	DeepSeekModel   string
	DeepSeekTimeout time.Duration
}

type MahresourcesContext struct {
	// existing fields...
	mrqlGenerator MRQLGenerator
}

func (ctx *MahresourcesContext) MRQLGenerator() MRQLGenerator {
	return ctx.mrqlGenerator
}

func (ctx *MahresourcesContext) SetMRQLGenerator(generator MRQLGenerator) {
	ctx.mrqlGenerator = generator
}
```

Also propagate `DeepSeekAPIKey`, `DeepSeekModel`, and `DeepSeekTimeout` from `MahresourcesInputConfig` to `MahresourcesConfig` in `CreateContextWithConfig`.

- [ ] **Step 4: Run generator tests and verify GREEN**

Run:

```bash
go test --tags 'json1 fts5' ./application_context -run 'TestMRQLGenerator' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write failing DeepSeek client tests**

Create `application_context/deepseek_client_test.go`:

```go
package application_context

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeepSeekClientSendsJSONChatRequest(t *testing.T) {
	var auth string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"query\":\"type = resource LIMIT 50\",\"explanation\":\"Finds resources.\"}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	client := NewDeepSeekMRQLDraftProvider(server.URL, "secret-key", "deepseek-v4-pro", server.Client())
	got, err := client.GenerateDraft(context.Background(), "prompt body")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if auth != "Bearer secret-key" {
		t.Fatalf("Authorization header = %q", auth)
	}
	for _, want := range []string{`"model":"deepseek-v4-pro"`, `"stream":false`, `"response_format"`, `"json_object"`, `"max_tokens":800`} {
		if !strings.Contains(body, want) {
			t.Fatalf("request body missing %s: %s", want, body)
		}
	}
	if got.Query != `type = resource LIMIT 50` || got.Explanation != "Finds resources." {
		t.Fatalf("unexpected draft: %#v", got)
	}
}

func TestDeepSeekClientRejectsMalformedProviderContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not-json"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	client := NewDeepSeekMRQLDraftProvider(server.URL, "secret-key", "deepseek-v4-pro", server.Client())
	if _, err := client.GenerateDraft(context.Background(), "prompt body"); err == nil {
		t.Fatal("expected malformed content error")
	}
}

func TestDeepSeekClientRejectsLengthFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"query\":\"type = resource LIMIT 50\",\"explanation\":\"x\"}"},"finish_reason":"length"}]}`))
	}))
	defer server.Close()

	client := NewDeepSeekMRQLDraftProvider(server.URL, "secret-key", "deepseek-v4-pro", server.Client())
	if _, err := client.GenerateDraft(context.Background(), "prompt body"); err == nil {
		t.Fatal("expected finish_reason error")
	}
}

func TestDeepSeekClientTimeoutUsesContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewDeepSeekMRQLDraftProvider(server.URL, "secret-key", "deepseek-v4-pro", server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	if _, err := client.GenerateDraft(ctx, "prompt body"); err == nil {
		t.Fatal("expected timeout/context error")
	}
}
```

- [ ] **Step 6: Run DeepSeek tests and verify RED**

Run:

```bash
go test --tags 'json1 fts5' ./application_context -run 'TestDeepSeekClient' -count=1
```

Expected: compile failure for missing `NewDeepSeekMRQLDraftProvider`.

- [ ] **Step 7: Implement DeepSeek client**

Create `application_context/deepseek_client.go`:

```go
package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type deepSeekMRQLDraftProvider struct {
	url    string
	apiKey string
	model  string
	client *http.Client
}

func NewDeepSeekMRQLDraftProvider(url, apiKey, model string, client *http.Client) MRQLDraftProvider {
	if client == nil {
		client = http.DefaultClient
	}
	if model == "" {
		model = DefaultDeepSeekMRQLGenerationModel
	}
	return &deepSeekMRQLDraftProvider{url: url, apiKey: apiKey, model: model, client: client}
}

func (p *deepSeekMRQLDraftProvider) GenerateDraft(ctx context.Context, prompt string) (providerMRQLDraft, error) {
	body := map[string]any{
		"model": p.model,
		"stream": false,
		"max_tokens": 800,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": "You generate MRQL. Return JSON only."},
			{"role": "user", "content": prompt},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return providerMRQLDraft{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(payload))
	if err != nil {
		return providerMRQLDraft{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return providerMRQLDraft{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerMRQLDraft{}, fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return providerMRQLDraft{}, err
	}
	if len(decoded.Choices) == 0 {
		return providerMRQLDraft{}, fmt.Errorf("provider returned no choices")
	}
	choice := decoded.Choices[0]
	switch choice.FinishReason {
	case "", "stop":
	default:
		return providerMRQLDraft{}, fmt.Errorf("provider did not finish cleanly")
	}
	content := strings.TrimSpace(choice.Message.Content)
	if content == "" {
		return providerMRQLDraft{}, fmt.Errorf("provider returned empty content")
	}

	var draft providerMRQLDraft
	if err := json.Unmarshal([]byte(content), &draft); err != nil {
		return providerMRQLDraft{}, err
	}
	return draft, nil
}
```

- [ ] **Step 8: Run application_context generator and client tests**

Run:

```bash
go test --tags 'json1 fts5' ./application_context -run 'TestMRQLGenerator|TestDeepSeekClient' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit Task 2**

Run:

```bash
git add application_context/context.go application_context/mrql_generation.go application_context/mrql_generation_test.go application_context/deepseek_client.go application_context/deepseek_client_test.go
git commit -m "feat(mrql): add DeepSeek generation service"
```

---

## Task 3: API Route, Config Wiring, Rate Limit, Auth And CSRF

**Files:**
- Create: `application_context/mrql_generation_rate_limiter.go`
- Create: `application_context/mrql_generation_rate_limiter_test.go`
- Create: `server/api_tests/mrql_generate_test.go`
- Modify: `main.go`
- Modify: `server/api_handlers/mrql_api_handlers.go`
- Modify: `server/routes.go`
- Modify: `server/routes_openapi.go`

- [ ] **Step 1: Write failing rate-limiter tests**

Create `application_context/mrql_generation_rate_limiter_test.go`:

```go
package application_context

import (
	"testing"
	"time"
)

func TestMRQLGenerationRateLimiter(t *testing.T) {
	limiter := NewMRQLGenerationRateLimiter(2, time.Minute)
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)

	if !limiter.Allow("user-1", now) {
		t.Fatal("first request should pass")
	}
	if !limiter.Allow("user-1", now.Add(time.Second)) {
		t.Fatal("second request should pass")
	}
	if limiter.Allow("user-1", now.Add(2*time.Second)) {
		t.Fatal("third request in same window should be limited")
	}
	if !limiter.Allow("user-1", now.Add(time.Minute+time.Second)) {
		t.Fatal("request after window should pass")
	}
	if !limiter.Allow("user-2", now.Add(2*time.Second)) {
		t.Fatal("different key should have independent quota")
	}
}
```

- [ ] **Step 2: Run limiter test and verify RED**

Run:

```bash
go test --tags 'json1 fts5' ./application_context -run TestMRQLGenerationRateLimiter -count=1
```

Expected: compile failure for missing limiter.

- [ ] **Step 3: Implement rate limiter**

Create `application_context/mrql_generation_rate_limiter.go`:

```go
package application_context

import (
	"sync"
	"time"
)

type MRQLGenerationRateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	keys   map[string]generationRateBucket
}

type generationRateBucket struct {
	start time.Time
	count int
}

func NewMRQLGenerationRateLimiter(max int, window time.Duration) *MRQLGenerationRateLimiter {
	return &MRQLGenerationRateLimiter{max: max, window: window, keys: map[string]generationRateBucket{}}
}

func (l *MRQLGenerationRateLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.keys[key]
	if b.start.IsZero() || now.Sub(b.start) >= l.window {
		l.keys[key] = generationRateBucket{start: now, count: 1}
		return true
	}
	if b.count >= l.max {
		return false
	}
	b.count++
	l.keys[key] = b
	return true
}
```

- [ ] **Step 4: Run limiter test and verify GREEN**

Run:

```bash
go test --tags 'json1 fts5' ./application_context -run TestMRQLGenerationRateLimiter -count=1
```

Expected: PASS.

- [ ] **Step 5: Write failing API tests**

Create `server/api_tests/mrql_generate_test.go`:

```go
package api_tests

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
)

type fakeAPIMRQLGenerator struct {
	result *application_context.MRQLGenerationResult
	err    error
	calls  int
}

func (f *fakeAPIMRQLGenerator) GenerateMRQL(ctx context.Context, prompt string) (*application_context.MRQLGenerationResult, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestMRQLGenerateMissingConfig(t *testing.T) {
	tc := SetupTestEnv(t)
	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/generate", map[string]any{"prompt": "show images"})
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d (%s)", resp.Code, resp.Body.String())
	}
}

func TestMRQLGenerateSuccess(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.SetMRQLGenerator(&fakeAPIMRQLGenerator{result: &application_context.MRQLGenerationResult{
		Query: `type = resource LIMIT 50`, Explanation: "Finds resources.", Valid: true, Errors: []map[string]any{},
	}})

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/generate", map[string]any{"prompt": "show resources"})
	if resp.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", resp.Code, resp.Body.String())
	}
	var body application_context.MRQLGenerationResult
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Valid || body.Query != `type = resource LIMIT 50` {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestMRQLGenerateRequiresWriteCapability(t *testing.T) {
	tc := setupAuthEnv(t)
	bearer := roleBearer(t, tc, models.RoleGuest)
	resp := doReq(tc, http.MethodPost, "/v1/mrql/generate",
		map[string]string{"Content-Type": "application/json", "Authorization": bearer},
		nil, strings.NewReader(`{"prompt":"show resources"}`))
	if resp.Code != http.StatusForbidden {
		t.Fatalf("guest bearer should be forbidden, got %d (%s)", resp.Code, resp.Body.String())
	}
}

func TestMRQLGenerateRequiresCSRFForCookieSession(t *testing.T) {
	tc := setupAuthEnv(t)
	tc.AppCtx.SetMRQLGenerator(&fakeAPIMRQLGenerator{result: &application_context.MRQLGenerationResult{
		Query: `type = resource LIMIT 50`, Explanation: "Finds resources.", Valid: true,
	}})
	cookie, token := loginCookieAndCSRF(t, tc)

	noToken := doReq(tc, http.MethodPost, "/v1/mrql/generate",
		map[string]string{"Content-Type": "application/json"}, []*http.Cookie{cookie},
		strings.NewReader(`{"prompt":"show resources"}`))
	if noToken.Code != http.StatusForbidden {
		t.Fatalf("cookie request without CSRF should be 403, got %d (%s)", noToken.Code, noToken.Body.String())
	}

	withToken := doReq(tc, http.MethodPost, "/v1/mrql/generate",
		map[string]string{"Content-Type": "application/json", "X-CSRF-Token": token}, []*http.Cookie{cookie},
		strings.NewReader(`{"prompt":"show resources"}`))
	if withToken.Code != http.StatusOK {
		t.Fatalf("cookie request with CSRF should be 200, got %d (%s)", withToken.Code, withToken.Body.String())
	}
}

func TestMRQLGenerateRateLimit(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPIMRQLGenerator{result: &application_context.MRQLGenerationResult{
		Query: `type = resource LIMIT 50`, Explanation: "Finds resources.", Valid: true,
	}}
	tc.AppCtx.SetMRQLGenerator(fake)
	tc.AppCtx.SetMRQLGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(1, time.Minute))

	first := tc.MakeRequest(http.MethodPost, "/v1/mrql/generate", map[string]any{"prompt": "show resources"})
	if first.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d", first.Code)
	}
	second := tc.MakeRequest(http.MethodPost, "/v1/mrql/generate", map[string]any{"prompt": "show resources"})
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request should be 429, got %d (%s)", second.Code, second.Body.String())
	}
	if fake.calls != 1 {
		t.Fatalf("provider should only be called once, got %d", fake.calls)
	}
}
```

- [ ] **Step 6: Run API tests and verify RED**

Run:

```bash
go test --tags 'json1 fts5' ./server/api_tests -run 'TestMRQLGenerate' -count=1
```

Expected: route returns 404 or compile failure for missing setter.

- [ ] **Step 7: Add context limiter seam and config wiring**

Modify `application_context/context.go`:

```go
type MahresourcesContext struct {
	// existing fields...
	mrqlGenerationLimiter *MRQLGenerationRateLimiter
}

func (ctx *MahresourcesContext) MRQLGenerationRateLimiter() *MRQLGenerationRateLimiter {
	if ctx.mrqlGenerationLimiter == nil {
		ctx.mrqlGenerationLimiter = NewMRQLGenerationRateLimiter(10, time.Minute)
	}
	return ctx.mrqlGenerationLimiter
}

func (ctx *MahresourcesContext) SetMRQLGenerationRateLimiter(l *MRQLGenerationRateLimiter) {
	ctx.mrqlGenerationLimiter = l
}
```

Modify `main.go`:

```go
deepSeekAPIKey := os.Getenv("DEEPSEEK_API_KEY")
deepSeekModel := getEnvOrDefault("DEEPSEEK_MODEL", application_context.DefaultDeepSeekMRQLGenerationModel)
deepSeekTimeoutRaw := getEnvOrDefault("DEEPSEEK_TIMEOUT", application_context.DefaultDeepSeekMRQLGenerationTimeout.String())
deepSeekTimeout, err := time.ParseDuration(deepSeekTimeoutRaw)
if err != nil {
	log.Fatalf("invalid DEEPSEEK_TIMEOUT=%q: %v", deepSeekTimeoutRaw, err)
}
```

Add the parsed values to `MahresourcesConfig` construction:

```go
DeepSeekAPIKey:  deepSeekAPIKey,
DeepSeekModel:   deepSeekModel,
DeepSeekTimeout: deepSeekTimeout,
```

After `appContext` is created, wire the default generator only if `DeepSeekAPIKey` is not empty:

```go
if appContext.Config.DeepSeekAPIKey != "" {
	provider := application_context.NewDeepSeekMRQLDraftProvider(
		"https://api.deepseek.com/chat/completions",
		appContext.Config.DeepSeekAPIKey,
		appContext.Config.DeepSeekModel,
		nil,
	)
	appContext.SetMRQLGenerator(application_context.NewMRQLGenerator(provider, application_context.MRQLGenerationConfig{
		APIKey:  appContext.Config.DeepSeekAPIKey,
		Model:   appContext.Config.DeepSeekModel,
		Timeout: appContext.Config.DeepSeekTimeout,
	}))
}
```

- [ ] **Step 8: Add handler and route**

Modify `server/api_handlers/mrql_api_handlers.go`:

```go
type mrqlGenerateRequest struct {
	Prompt string `json:"prompt" schema:"prompt"`
}

func GetGenerateMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlGenerateRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Prompt) == "" {
			http_utils.HandleError(errors.New("prompt is required"), writer, request, http.StatusBadRequest)
			return
		}

		generator := ctx.MRQLGenerator()
		if generator == nil {
			http_utils.HandleError(errors.New("MRQL generation is not configured"), writer, request, http.StatusServiceUnavailable)
			return
		}
		key := application_context.ClientIP(request)
		if !ctx.MRQLGenerationRateLimiter().Allow(key, time.Now()) {
			http_utils.HandleError(errors.New("MRQL generation rate limit exceeded"), writer, request, http.StatusTooManyRequests)
			return
		}

		result, err := generator.GenerateMRQL(request.Context(), req.Prompt)
		if err != nil {
			switch {
			case errors.Is(err, application_context.ErrMRQLGenerationNotConfigured):
				http_utils.HandleError(errors.New("MRQL generation is not configured"), writer, request, http.StatusServiceUnavailable)
			case errors.Is(err, application_context.ErrMRQLGenerationBadRequest):
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			case errors.Is(err, application_context.ErrMRQLGenerationTimeout):
				http_utils.HandleError(errors.New("MRQL generation timed out"), writer, request, http.StatusGatewayTimeout)
			default:
				http_utils.HandleError(errors.New("MRQL generation provider error"), writer, request, http.StatusBadGateway)
			}
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}
```

Modify `server/routes.go`:

```go
router.Methods(http.MethodPost).Path("/v1/mrql/generate").HandlerFunc(api_handlers.GetGenerateMRQLHandler(appContext))
```

Do not modify `isReadViaPost` in `server/authz_policy.go`.

- [ ] **Step 9: Add OpenAPI entry**

Modify `server/routes_openapi.go` inside `registerMRQLRoutes`:

```go
r.Register(openapi.RouteInfo{
	Method:      http.MethodPost,
	Path:        "/v1/mrql/generate",
	OperationID: "generateMRQL",
	Summary:     "Generate an MRQL draft from natural language",
	Description: `Generates, parses, validates, and lints an MRQL draft from a natural-language prompt.

Request body fields:
  - prompt (string, required) — user request to convert into MRQL

The endpoint does not execute MRQL. The server sends the prompt and syntax-only MRQL instructions to the configured DeepSeek provider.`,
	Tags:                 mrqlTag,
	RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
	ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
})
```

- [ ] **Step 10: Run API and limiter tests**

Run:

```bash
go test --tags 'json1 fts5' ./application_context -run TestMRQLGenerationRateLimiter -count=1
go test --tags 'json1 fts5' ./server/api_tests -run 'TestMRQLGenerate' -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit Task 3**

Run:

```bash
git add application_context/context.go application_context/mrql_generation_rate_limiter.go application_context/mrql_generation_rate_limiter_test.go main.go server/api_handlers/mrql_api_handlers.go server/routes.go server/routes_openapi.go server/api_tests/mrql_generate_test.go
git commit -m "feat(mrql): expose generation API"
```

---

## Task 4: MRQL Editor UI And Browser Tests

**Files:**
- Modify: `src/components/mrqlEditor.js`
- Modify: `templates/mrql.tpl`
- Modify: `e2e/pages/MRQLPage.ts`
- Create: `e2e/tests/mrql-generate.spec.ts`

- [ ] **Step 1: Write failing E2E tests**

Create `e2e/tests/mrql-generate.spec.ts`:

```ts
import { test, expect } from '../fixtures/base.fixture';
import { MRQLPage } from '../pages/MRQLPage';

test.describe('MRQL generation', () => {
  test('generates a valid query into the editor with explanation', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await page.route('/v1/mrql/generate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          query: 'type = resource AND contentType ~ "image/*" LIMIT 50',
          explanation: 'Finds up to 50 image resources.',
          valid: true,
          errors: [],
        }),
      });
    });

    await mrql.navigate();
    await mrql.enterGenerationPrompt('show image resources');
    await mrql.generateMRQL();

    await expect(mrql.generationExplanation).toContainText('Finds up to 50 image resources.');
    await expect.poll(() => mrql.getEditorContent()).toBe('type = resource AND contentType ~ "image/*" LIMIT 50');
  });

  test('invalid generation stays out of the editor until explicitly applied', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await page.route('/v1/mrql/generate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          query: 'type = resource LIMIT 1000000',
          explanation: 'Too broad.',
          valid: false,
          errors: [{ message: 'LIMIT must be between 1 and 500' }],
        }),
      });
    });

    await mrql.navigate();
    await mrql.enterQuery('name ~ "keep-me"');
    await mrql.enterGenerationPrompt('all resources');
    await mrql.generateMRQL();

    await expect(mrql.generationError).toContainText('LIMIT must be between 1 and 500');
    await expect.poll(() => mrql.getEditorContent()).toBe('name ~ "keep-me"');

    await mrql.useGeneratedQuery();
    await expect.poll(() => mrql.getEditorContent()).toBe('type = resource LIMIT 1000000');
  });

  test('provider error leaves editor content unchanged', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await page.route('/v1/mrql/generate', async (route) => {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'MRQL generation is not configured' }),
      });
    });

    await mrql.navigate();
    await mrql.enterQuery('name ~ "keep-me"');
    await mrql.enterGenerationPrompt('show resources');
    await mrql.generateMRQL();

    await expect(mrql.generationError).toContainText('MRQL generation is not configured');
    await expect.poll(() => mrql.getEditorContent()).toBe('name ~ "keep-me"');
  });

  test('generated query clears saved-query update affordance and stale results', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await page.route('/v1/mrql/generate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          query: 'type = note LIMIT 50',
          explanation: 'Finds notes.',
          valid: true,
          errors: [],
        }),
      });
    });

    await mrql.navigate();
    const queryName = `Generation Reset ${Date.now()}`;
    await mrql.enterQuery('name ~ "test"');
    await mrql.saveQuery(queryName);
    await mrql.loadSavedQuery(queryName);
    await mrql.executeQuery();
    await expect(mrql.resultsSection.locator('h2')).toBeVisible();

    await mrql.enterGenerationPrompt('show notes');
    await mrql.generateMRQL();

    await expect(page.locator('[data-testid="mrql-update-button"]')).toBeHidden();
    await expect(mrql.resultsSection.locator('h2')).toHaveCount(0);
  });
});
```

- [ ] **Step 2: Run new E2E tests and verify RED**

Run:

```bash
cd e2e && npx playwright test tests/mrql-generate.spec.ts --project=chromium
```

Expected: fails because generation locators/helpers/UI do not exist.

- [ ] **Step 3: Add MRQLPage helper locators**

Modify `e2e/pages/MRQLPage.ts`:

```ts
readonly generationPrompt: Locator;
readonly generationButton: Locator;
readonly generationStatus: Locator;
readonly generationError: Locator;
readonly generationExplanation: Locator;
readonly useGeneratedButton: Locator;
```

Initialize them in the constructor:

```ts
this.generationPrompt = page.getByTestId('mrql-generation-prompt');
this.generationButton = page.getByTestId('mrql-generate-button');
this.generationStatus = page.getByTestId('mrql-generation-status');
this.generationError = page.getByTestId('mrql-generation-error');
this.generationExplanation = page.getByTestId('mrql-generation-explanation');
this.useGeneratedButton = page.getByTestId('mrql-use-generated-button');
```

Add helper methods:

```ts
async enterGenerationPrompt(prompt: string) {
  await this.generationPrompt.fill(prompt);
}

async generateMRQL() {
  await this.generationButton.click();
  await expect(this.generationButton).toBeEnabled({ timeout: 15000 });
}

async useGeneratedQuery() {
  await this.useGeneratedButton.click();
}
```

- [ ] **Step 4: Add Alpine generation state and methods**

Modify `src/components/mrqlEditor.js`, adding state fields:

```js
generationPrompt: '',
generating: false,
generationError: '',
generationStatus: '',
generatedQuery: '',
generatedExplanation: '',
generatedValid: null,
generatedErrors: [],
_generationRequestId: 0,
_generationEditorSnapshot: '',
```

Add methods inside the returned object:

```js
async generateFromPrompt() {
  const prompt = this.generationPrompt.trim();
  this.generationError = '';
  this.generationStatus = '';
  this.generatedQuery = '';
  this.generatedExplanation = '';
  this.generatedValid = null;
  this.generatedErrors = [];

  if (!prompt) {
    this.generationError = 'Describe what results you want first.';
    this.$nextTick(() => this.$refs.generationPrompt?.focus());
    return;
  }

  const requestId = ++this._generationRequestId;
  const editorSnapshot = this.getQuery();
  this._generationEditorSnapshot = editorSnapshot;
  this.generating = true;
  this.generationStatus = 'Generating...';

  try {
    const resp = await fetch('/v1/mrql/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt }),
    });
    const data = await resp.json().catch(() => null);
    if (!resp.ok) {
      this.generationError = data?.error || data?.Error || `Generation failed (${resp.status})`;
      this.generationStatus = '';
      return;
    }
    if (requestId !== this._generationRequestId) return;

    this.generatedQuery = data.query || '';
    this.generatedExplanation = data.explanation || '';
    this.generatedValid = !!data.valid;
    this.generatedErrors = Array.isArray(data.errors) ? data.errors : [];

    if (!this.generatedValid) {
      this.generationStatus = 'Generated query needs review.';
      this.generationError = this.generatedErrors.map((e) => e.message || JSON.stringify(e)).join('; ') || 'Generated query is invalid.';
      return;
    }

    if (this.getQuery() !== editorSnapshot) {
      this.generationStatus = 'Generated query is ready.';
      return;
    }

    this.applyGeneratedQuery();
    this.generationStatus = 'Generated query is ready.';
  } catch (err) {
    this.generationError = err.message || 'Network error';
    this.generationStatus = '';
  } finally {
    if (requestId === this._generationRequestId) this.generating = false;
  }
},

applyGeneratedQuery() {
  if (!this.generatedQuery) return;
  this.setQuery(this.generatedQuery);
  this.clearLoadedSaved();
  this.result = null;
  this.error = '';
  this.defaultLimitApplied = false;
  this.appliedLimit = 0;
  this.scheduleValidation();
},
```

- [ ] **Step 5: Add template generation panel**

Modify `templates/mrql.tpl` above the editor container:

```html
<div class="mb-4 border border-stone-200 rounded-md p-3 bg-stone-50" aria-label="Generate MRQL from natural language">
    <label for="mrql-generation-prompt" class="block text-sm font-medium text-stone-700 mb-1">Describe results</label>
    <textarea id="mrql-generation-prompt"
              x-ref="generationPrompt"
              x-model="generationPrompt"
              data-testid="mrql-generation-prompt"
              rows="2"
              :aria-invalid="generationError ? 'true' : 'false'"
              aria-describedby="mrql-generation-help mrql-generation-message"
              class="w-full border border-stone-300 rounded-md px-3 py-2 text-sm focus:ring-amber-600 focus:border-amber-600"
              placeholder="Images from the last 30 days tagged invoice"></textarea>
    <p id="mrql-generation-help" class="mt-1 text-xs text-stone-500">Sends only this text and MRQL syntax instructions to the configured provider.</p>
    <div class="mt-2 flex items-center gap-2">
        <button type="button"
                @click="generateFromPrompt()"
                data-testid="mrql-generate-button"
                :disabled="generating"
                :aria-busy="generating.toString()"
                class="inline-flex items-center px-3 py-2 border border-stone-300 rounded-md shadow-sm text-sm font-mono font-medium text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer">
            <span x-text="generating ? 'Generating...' : 'Generate'"></span>
        </button>
        <template x-if="generatedQuery && (!generatedValid || getQuery() !== generatedQuery)">
            <button type="button"
                    @click="applyGeneratedQuery()"
                    data-testid="mrql-use-generated-button"
                    class="px-3 py-2 text-sm font-mono text-stone-700 bg-white border border-stone-300 rounded-md hover:bg-stone-50 cursor-pointer">
                Use generated query
            </button>
        </template>
    </div>
    <div id="mrql-generation-message" class="mt-2 space-y-2">
        <template x-if="generationStatus">
            <p data-testid="mrql-generation-status" role="status" aria-live="polite" class="text-sm text-stone-700 font-mono" x-text="generationStatus"></p>
        </template>
        <template x-if="generationError">
            <p data-testid="mrql-generation-error" role="alert" class="text-sm text-red-700 font-mono" x-text="generationError"></p>
        </template>
        <template x-if="generatedExplanation">
            <p data-testid="mrql-generation-explanation" class="text-sm text-stone-600" x-text="generatedExplanation"></p>
        </template>
        <template x-if="generatedQuery && !generatedValid">
            <pre data-testid="mrql-generated-query-preview" class="text-xs bg-white border border-stone-200 rounded p-2 overflow-x-auto"><code x-text="generatedQuery"></code></pre>
        </template>
    </div>
</div>
```

- [ ] **Step 6: Build JS**

Run:

```bash
npm run build-js
```

Expected: Vite build succeeds.

- [ ] **Step 7: Run new E2E tests and verify GREEN**

Run:

```bash
cd e2e && npx playwright test tests/mrql-generate.spec.ts --project=chromium
```

Expected: PASS.

- [ ] **Step 8: Commit Task 4**

Run:

```bash
git add src/components/mrqlEditor.js templates/mrql.tpl e2e/pages/MRQLPage.ts e2e/tests/mrql-generate.spec.ts public/dist
git commit -m "feat(mrql): add web generation UI"
```

If `public/dist` is ignored or unchanged, omit it from `git add`.

---

## Task 5: Documentation, OpenAPI Regeneration, Full Verification

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs-site/docs/features/mrql.md`
- Modify: `docs-site/docs/configuration/advanced.md`
- Modify: `openapi.yaml`

- [ ] **Step 1: Update configuration docs**

Modify the configuration table in `CLAUDE.md`:

```markdown
| `DEEPSEEK_API_KEY` | DeepSeek API key for `/mrql` natural-language generation. Env-only; no CLI flag in v1. |
| `DEEPSEEK_MODEL` | DeepSeek model for MRQL generation (default: `deepseek-v4-pro`). |
| `DEEPSEEK_TIMEOUT` | Timeout for one DeepSeek MRQL generation call (default: `20s`). Invalid values fail startup. |
```

Add an admin/privacy note to `docs-site/docs/features/mrql.md` near the "Accessing MRQL" or execution endpoint section:

```markdown
## Natural-Language Generation

When `DEEPSEEK_API_KEY` is configured, the `/mrql` editor can draft MRQL from a "Describe results" prompt. The server sends only the text you type and syntax-only MRQL instructions to DeepSeek. It does not send local tag lists, category names, note types, resource categories, saved queries, or database contents.

Generated MRQL is parsed, validated, and linted locally, then shown with an explanation. It is not executed until you press Run. Generation is CSRF-protected and requires write access when authentication is enabled.
```

- [ ] **Step 2: Regenerate OpenAPI**

Run:

```bash
go run ./cmd/openapi-gen
```

Expected: `openapi.yaml` includes `POST /v1/mrql/generate`.

- [ ] **Step 3: Run targeted Go tests**

Run:

```bash
go test --tags 'json1 fts5' ./mrql ./application_context ./server/api_tests -run 'TestLintGeneratedQuery|TestMRQLGenerator|TestDeepSeekClient|TestMRQLGenerationRateLimiter|TestMRQLGenerate' -count=1
```

Expected: PASS.

- [ ] **Step 4: Run targeted frontend build and E2E**

Run:

```bash
npm run build-js
cd e2e && npx playwright test tests/mrql-generate.spec.ts --project=chromium
```

Expected: both commands PASS.

- [ ] **Step 5: Run broader verification**

Run:

```bash
go test --tags 'json1 fts5' ./mrql/... ./application_context/... ./server/api_tests/... -count=1
npm run build
```

Expected: both commands PASS.

- [ ] **Step 6: Commit Task 5**

Run:

```bash
git add CLAUDE.md docs-site/docs/features/mrql.md docs-site/docs/configuration openapi.yaml
git commit -m "docs(mrql): document natural-language generation"
```

- [ ] **Step 7: Final status check**

Run:

```bash
git status --short
git log --oneline -5
```

Expected: only unrelated pre-existing untracked files remain, and the last commits correspond to Tasks 1-5.

---

## Execution Notes

- Use TDD. Each task starts with a failing test and only then production code.
- Do not add `/v1/mrql/generate` to `isReadViaPost`; that would also exempt CSRF.
- Do not log prompt text, provider request bodies, provider response bodies, generated query, or generated explanation.
- Do not store `DEEPSEEK_API_KEY` in runtime settings or expose it through admin context.
- Keep invalid generated MRQL out of CodeMirror by default. It can appear in the generation preview panel and be applied by explicit user action.
- Do not auto-run generated MRQL.
