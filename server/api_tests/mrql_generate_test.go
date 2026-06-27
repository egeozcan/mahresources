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
		Query:       `type = resource LIMIT 50`,
		Explanation: "Finds resources.",
		Valid:       true,
		Errors:      []map[string]any{},
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
		Query:       `type = resource LIMIT 50`,
		Explanation: "Finds resources.",
		Valid:       true,
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
		Query:       `type = resource LIMIT 50`,
		Explanation: "Finds resources.",
		Valid:       true,
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
