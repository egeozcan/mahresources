package application_context

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	toolsCatalogueURL = "https://openrouter.ai/api/v1/models?supported_parameters=tools"
	fullCatalogueURL  = "https://openrouter.ai/api/v1/models"
)

// fullCatalogueReply lists every id with its supported_parameters; an id ending
// in "no-tools" lacks "tools".
func fullCatalogueReply(ids ...string) string {
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for i, id := range ids {
		if i > 0 {
			b.WriteString(",")
		}
		params := `["temperature","tools"]`
		if strings.HasSuffix(id, "no-tools") {
			params = `["temperature"]`
		}
		b.WriteString(`{"id":"` + id + `","supported_parameters":` + params + `}`)
	}
	b.WriteString(`]}`)
	return b.String()
}

func catalogueReply(ids ...string) string {
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for i, id := range ids {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"id":"` + id + `"}`)
	}
	b.WriteString(`]}`)
	return b.String()
}

// catalogueFixture serves a tools-filtered list and a full list and resets the
// process cache so tests do not see each other's entries.
func catalogueFixture(t *testing.T, withTools, all []string) *conversationRecorder {
	t.Helper()
	resetOpenRouterCatalogue()
	t.Cleanup(resetOpenRouterCatalogue)
	return newConversationRecorder(func(_ int, url string, _ []byte) (int, string) {
		switch url {
		case toolsCatalogueURL:
			return 200, catalogueReply(withTools...)
		case fullCatalogueURL:
			return 200, fullCatalogueReply(all...)
		}
		return 404, `{}`
	})
}

func supports(p TemplateConversationProvider, model string) (bool, error) {
	return p.SupportsTools(context.Background(), model)
}

func TestSupportsToolsOpenRouterCatalogue(t *testing.T) {
	rec := catalogueFixture(t, []string{"anthropic/claude-x", "deepseek/deepseek-pro"}, []string{"anthropic/claude-x", "deepseek/deepseek-pro", "acme/no-tools"})
	p := NewGenerationConversationProvider("openrouter", "k", "m", rec.client())

	t.Run("listed supports tools", func(t *testing.T) {
		ok, err := supports(p, "anthropic/claude-x")
		if !ok || err != nil {
			t.Fatalf("got %v %v", ok, err)
		}
		if rec.count() != 1 || rec.urls[0] != toolsCatalogueURL {
			t.Fatalf("requests = %v", rec.urls)
		}
		if got := rec.headers[0].Get("Authorization"); got != "" {
			t.Fatalf("the public catalogue must not receive the API key, got %q", got)
		}
	})
	t.Run("listed without tools is refused", func(t *testing.T) {
		ok, err := supports(p, "acme/no-tools")
		if ok || err != nil {
			t.Fatalf("got %v %v", ok, err)
		}
	})
	t.Run("alias is unknown, not refused", func(t *testing.T) {
		ok, err := supports(p, "~anthropic/claude-sonnet-latest")
		if !ok || !errors.Is(err, ErrToolSupportUnknown) {
			t.Fatalf("got %v %v", ok, err)
		}
	})
	t.Run("second lookups hit the cache", func(t *testing.T) {
		before := rec.count()
		for _, m := range []string{"anthropic/claude-x", "acme/no-tools", "~anthropic/claude-sonnet-latest"} {
			if _, err := supports(p, m); err != nil && !errors.Is(err, ErrToolSupportUnknown) {
				t.Fatal(err)
			}
		}
		if rec.count() != before {
			t.Fatalf("cache missed: %d new requests", rec.count()-before)
		}
	})
}

func TestSupportsToolsCacheExpiresAfterTenMinutes(t *testing.T) {
	rec := catalogueFixture(t, []string{"a/b"}, []string{"a/b"})
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	old := openRouterCatalogueNow
	openRouterCatalogueNow = func() time.Time { return now }
	t.Cleanup(func() { openRouterCatalogueNow = old })
	p := NewGenerationConversationProvider("openrouter", "k", "m", rec.client())
	if ok, _ := supports(p, "a/b"); !ok {
		t.Fatal("not supported")
	}
	now = now.Add(9*time.Minute + 59*time.Second)
	supports(p, "a/b")
	if rec.count() != 1 {
		t.Fatalf("refetched inside the TTL: %d", rec.count())
	}
	now = now.Add(2 * time.Second)
	supports(p, "a/b")
	if rec.count() != 2 {
		t.Fatalf("did not refetch after 10 minutes: %d", rec.count())
	}
}

func TestSupportsToolsFailureIsUnknownAndNotCached(t *testing.T) {
	resetOpenRouterCatalogue()
	t.Cleanup(resetOpenRouterCatalogue)
	fail := true
	rec := newConversationRecorder(func(_ int, url string, _ []byte) (int, string) {
		if fail {
			return 500, `{"error":"secret"}`
		}
		return 200, catalogueReply("a/b")
	})
	p := NewGenerationConversationProvider("openrouter", "k", "m", rec.client())
	ok, err := supports(p, "a/b")
	if !ok || err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("got %v %v", ok, err)
	}
	fail = false
	ok, err = supports(p, "a/b")
	if !ok || err != nil || rec.count() != 2 {
		t.Fatalf("failure was cached: ok=%v err=%v requests=%d", ok, err, rec.count())
	}
}

func TestSupportsToolsCancelledContextAndTimeout(t *testing.T) {
	resetOpenRouterCatalogue()
	t.Cleanup(resetOpenRouterCatalogue)
	var deadline time.Duration
	client := &http.Client{Transport: generationRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if d, ok := r.Context().Deadline(); ok {
			deadline = time.Until(d)
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	p := NewGenerationConversationProvider("openrouter", "k", "m", client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ok, err := p.SupportsTools(ctx, "a/b")
	if !ok || !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v %v", ok, err)
	}

	old := openRouterCatalogueTimeout
	openRouterCatalogueTimeout = 20 * time.Millisecond
	t.Cleanup(func() { openRouterCatalogueTimeout = old })
	start := time.Now()
	ok, err = supports(p, "a/b")
	if !ok || err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("got %v %v after %v", ok, err, time.Since(start))
	}
	_ = deadline
}

func TestSupportsToolsDefaultTimeoutIsFiveSeconds(t *testing.T) {
	if openRouterCatalogueTimeout != 5*time.Second || openRouterCatalogueTTL != 10*time.Minute {
		t.Fatalf("timeout=%v ttl=%v", openRouterCatalogueTimeout, openRouterCatalogueTTL)
	}
}

func TestSupportsToolsOtherProvidersAnswerYesWithoutACall(t *testing.T) {
	for _, provider := range []string{"deepseek", "openai", "anthropic"} {
		rec := newConversationRecorder(fixedReply(`{}`))
		ok, err := NewGenerationConversationProvider(provider, "k", "m", rec.client()).SupportsTools(context.Background(), "whatever")
		if !ok || err != nil || rec.count() != 0 {
			t.Fatalf("%s: %v %v %d", provider, ok, err, rec.count())
		}
	}
}

func TestHistoryRewritable(t *testing.T) {
	for _, tc := range []struct {
		provider, model string
		want            bool
	}{
		{"deepseek", "deepseek-pro", true},
		{"openrouter", "deepseek/deepseek-pro", true},
		{"openrouter", "anthropic/claude-x", false},
		{"openrouter", "~deepseek/deepseek-pro-latest", false},
		{"openrouter", "openai/gpt-x", false},
		{"openai", "gpt-x", false},
		{"anthropic", "claude-x", false},
	} {
		if got := NewGenerationConversationProvider(tc.provider, "k", "m", nil).HistoryRewritable(tc.model); got != tc.want {
			t.Errorf("%s %s: %v, want %v", tc.provider, tc.model, got, tc.want)
		}
	}
}

func TestConversationTransportMetadata(t *testing.T) {
	for _, tc := range []struct {
		provider, endpoint string
		options            string
	}{
		{"deepseek", "https://api.deepseek.com/chat/completions", ""},
		{"openrouter", "https://openrouter.ai/api/v1/chat/completions", "provider.require_parameters=true; cache_control=ephemeral (anthropic/ and ~anthropic/ models)"},
		{"openai", "https://api.openai.com/v1/responses", "store=false; include=reasoning.encrypted_content; strict=false"},
		{"anthropic", "https://api.anthropic.com/v1/messages", "cache_control=ephemeral"},
	} {
		p := NewGenerationConversationProvider(tc.provider, "k", "m", nil)
		tr := p.Transport()
		if p.Endpoint() != tc.endpoint || tr.ResponseFormat != "tools" || tr.Stream || tr.Options != tc.options {
			t.Errorf("%s: endpoint=%s transport=%+v", tc.provider, p.Endpoint(), tr)
		}
		if (tc.provider == "anthropic") != (tr.APIVersion == AnthropicGenerationAPIVersion) {
			t.Errorf("%s: api version %q", tc.provider, tr.APIVersion)
		}
		if p.ProviderName() != tc.provider || strings.Contains(tr.Authorization, "k)") {
			t.Errorf("%s: name/authorization", tc.provider)
		}
	}
}

func TestSupportsToolsStaleFilteredCacheDoesNotRefuseANewToolModel(t *testing.T) {
	resetOpenRouterCatalogue()
	t.Cleanup(resetOpenRouterCatalogue)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	old := openRouterCatalogueNow
	openRouterCatalogueNow = func() time.Time { return now }
	t.Cleanup(func() { openRouterCatalogueNow = old })
	full := []string{"a/b", "acme/no-tools"}
	rec := newConversationRecorder(func(_ int, url string, _ []byte) (int, string) {
		if url == toolsCatalogueURL {
			return 200, catalogueReply("a/b")
		}
		return 200, fullCatalogueReply(full...)
	})
	p := NewGenerationConversationProvider("openrouter", "k", "m", rec.client())
	if ok, err := supports(p, "a/b"); !ok || err != nil {
		t.Fatalf("warm: %v %v", ok, err)
	}
	// The full list is fetched later than the filtered one, and by then lists a
	// model the cached filtered list has never heard of.
	now = now.Add(time.Minute)
	full = append(full, "new/tool-model")
	if ok, err := supports(p, "new/tool-model"); !ok || err != nil {
		t.Fatalf("new tool-capable model was refused or unknown: %v %v", ok, err)
	}
	if ok, err := supports(p, "acme/no-tools"); ok || err != nil {
		t.Fatalf("a listed model without tools must still be refused: %v %v", ok, err)
	}
}

func TestSupportsToolsFullRecordWithoutParametersIsUnknown(t *testing.T) {
	resetOpenRouterCatalogue()
	t.Cleanup(resetOpenRouterCatalogue)
	rec := newConversationRecorder(func(_ int, url string, _ []byte) (int, string) {
		if url == toolsCatalogueURL {
			return 200, catalogueReply("a/b")
		}
		return 200, catalogueReply("x/y")
	})
	ok, err := supports(NewGenerationConversationProvider("openrouter", "k", "m", rec.client()), "x/y")
	if !ok || err == nil {
		t.Fatalf("a record that does not say what it supports must not refuse: %v %v", ok, err)
	}
}

func TestSupportsToolsOneTimeoutCoversBothFetches(t *testing.T) {
	resetOpenRouterCatalogue()
	t.Cleanup(resetOpenRouterCatalogue)
	// Both fetches must run under the same context deadline; a fresh window per
	// fetch would give them different ones. No wall-clock bound is involved.
	deadlines := map[string]time.Time{}
	client := &http.Client{Transport: generationRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		d, ok := r.Context().Deadline()
		if !ok {
			t.Errorf("%s fetched without a deadline", r.URL)
		}
		deadlines[r.URL.String()] = d
		body := catalogueReply("a/b")
		if r.URL.String() == fullCatalogueURL {
			body = fullCatalogueReply("a/b")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	ok, err := supports(NewGenerationConversationProvider("openrouter", "k", "m", client), "other/model")
	if !ok || !errors.Is(err, ErrToolSupportUnknown) {
		t.Fatalf("got %v %v", ok, err)
	}
	f, a := deadlines[toolsCatalogueURL], deadlines[fullCatalogueURL]
	if f.IsZero() || a.IsZero() || !f.Equal(a) {
		t.Fatalf("filtered deadline %v, full deadline %v: the fetches must share one", f, a)
	}
	if left := time.Until(f); left > openRouterCatalogueTimeout {
		t.Fatalf("deadline %v ahead exceeds the %v window", left, openRouterCatalogueTimeout)
	}
}

func TestSupportsToolsInvalidEnvelopesAreNotCachedAndNeverRefuse(t *testing.T) {
	for _, bad := range []string{`{}`, `null`, `{"data":null}`, `{"data":{}}`, `{"error":{"message":"secret"}}`, `{"data":[],"error":{"message":"secret"}}`} {
		for _, which := range []string{"filtered", "full"} {
			t.Run(which+" "+bad, func(t *testing.T) {
				resetOpenRouterCatalogue()
				t.Cleanup(resetOpenRouterCatalogue)
				broken := true
				rec := newConversationRecorder(func(_ int, url string, _ []byte) (int, string) {
					switch {
					case url == toolsCatalogueURL && which == "filtered" && broken:
						return 200, bad
					case url == toolsCatalogueURL:
						return 200, catalogueReply("a/b")
					case which == "full" && broken:
						return 200, bad
					}
					return 200, fullCatalogueReply("a/b", "acme/no-tools")
				})
				p := NewGenerationConversationProvider("openrouter", "k", "m", rec.client())
				// For "full", the filtered list is valid and lacks the model, so the
				// full list decides; a broken one must not read as "not listed".
				ok, err := supports(p, "acme/no-tools")
				if !ok || err == nil || strings.Contains(err.Error(), "secret") {
					t.Fatalf("broken %s catalogue: %v %v", which, ok, err)
				}
				wantProviderError(t, err, GenerationErrorClassDecode, 0)
				broken = false
				ok, err = supports(p, "acme/no-tools")
				if ok || err != nil {
					t.Fatalf("the broken reply was cached: %v %v", ok, err)
				}
			})
		}
	}
}
