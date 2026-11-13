package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
)

func newGenFake() *fakeAPIMRQLGenerator {
	return &fakeAPIMRQLGenerator{result: &application_context.MRQLGenerationResult{
		Query: `type = resource LIMIT 50`, Explanation: "x", Valid: true,
	}}
}

func postGenerate(tc *TestContext, hdr map[string]string) int {
	h := map[string]string{"Content-Type": "application/json"}
	for k, v := range hdr {
		h[k] = v
	}
	return doReq(tc, http.MethodPost, "/v1/mrql/generate", h, nil, strings.NewReader(`{"prompt":"show resources"}`)).Code
}

func TestGenerateMRQL_RateLimitNotBypassedByForwardedFor(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := newGenFake()
	tc.AppCtx.SetMRQLGenerator(fake)
	tc.AppCtx.SetMRQLGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(2, time.Minute))
	var codes []int
	for i := 1; i <= 3; i++ {
		codes = append(codes, postGenerate(tc, map[string]string{"X-Forwarded-For": fmt.Sprintf("10.0.0.%d", i)}))
	}
	if codes[2] != http.StatusTooManyRequests || fake.calls != 2 {
		t.Fatalf("forged XFF must not mint buckets: codes=%v calls=%d", codes, fake.calls)
	}
}

func TestGenerateMRQL_TrustedForwardedForSeparatesClients(t *testing.T) {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) { c.TrustProxyHeaders = true })
	fake := newGenFake()
	tc.AppCtx.SetMRQLGenerator(fake)
	tc.AppCtx.SetMRQLGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(2, time.Minute))
	for i := 1; i <= 3; i++ {
		if code := postGenerate(tc, map[string]string{"X-Forwarded-For": fmt.Sprintf("10.0.0.%d", i)}); code != http.StatusOK {
			t.Fatalf("request %d: %d", i, code)
		}
	}
}

func TestGenerateMRQL_AuthOnKeysOnPrincipal(t *testing.T) {
	tc := setupAuthEnv(t)
	tc.AppCtx.Config.TrustProxyHeaders = true
	fake := newGenFake()
	tc.AppCtx.SetMRQLGenerator(fake)
	tc.AppCtx.SetMRQLGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(2, time.Minute))
	user := roleBearer(t, tc, models.RoleUser)
	editor := roleBearer(t, tc, models.RoleEditor)
	for _, b := range []string{user, editor} {
		for i := 0; i < 2; i++ {
			if code := postGenerate(tc, map[string]string{"Authorization": b, "X-Forwarded-For": fmt.Sprintf("10.1.%d.1", i)}); code != http.StatusOK {
				t.Fatalf("each principal gets its own budget: %d", code)
			}
		}
	}
	if code := postGenerate(tc, map[string]string{"Authorization": user, "X-Forwarded-For": "10.9.9.9"}); code != http.StatusTooManyRequests {
		t.Fatalf("varying XFF must not escape the principal bucket: %d", code)
	}
}

func TestGenerateMRQL_GuestStillForbidden(t *testing.T) {
	tc := setupAuthEnv(t)
	tc.AppCtx.SetMRQLGenerator(newGenFake())
	guest := roleBearer(t, tc, models.RoleGuest)
	if code := postGenerate(tc, map[string]string{"Authorization": guest}); code != http.StatusForbidden {
		t.Fatalf("guest: %d", code)
	}
}

func TestTemplateGenerate_RateLimitNotBypassedByForwardedFor(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.SetTemplateGenerator(&fakeAPITemplateGenerator{result: slotResult()})
	tc.AppCtx.SetTemplateGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(1, time.Minute))
	tc.AppCtx.SetTemplateDisclosureRateLimiter(application_context.NewMRQLGenerationRateLimiter(1, time.Minute))
	post := func(xff string, dry bool) int {
		body := `{"target":"slot","slot":"CustomHeader","prompt":"p","groundingHash":"` + fakeGroundingHash + `"`
		if dry {
			body += `,"dryRun":true`
		}
		body += `}`
		return doReq(tc, http.MethodPost, "/v1/category/generateTemplate",
			map[string]string{"Content-Type": "application/json", "X-Forwarded-For": xff}, nil, strings.NewReader(body)).Code
	}
	if c := post("10.0.0.1", false); c != http.StatusOK {
		t.Fatalf("first: %d", c)
	}
	if c := post("10.0.0.2", false); c != http.StatusTooManyRequests {
		t.Fatalf("second with other XFF: %d", c)
	}
	if c := post("10.0.0.3", true); c != http.StatusOK {
		t.Fatalf("first dry run: %d", c)
	}
	if c := post("10.0.0.4", true); c != http.StatusTooManyRequests {
		t.Fatalf("second dry run with other XFF: %d", c)
	}
}

func newestLog(t *testing.T, tc *TestContext, entityType string) models.LogEntry {
	t.Helper()
	var e models.LogEntry
	if err := tc.DB.Where("entity_type = ?", entityType).Order("id DESC").First(&e).Error; err != nil {
		t.Fatalf("no %s log entry: %v", entityType, err)
	}
	return e
}

func TestAuditClientIP_HonorsTrustFlag(t *testing.T) {
	hdr := map[string]string{"Content-Type": "application/json", "X-Forwarded-For": "192.168.1.50", "X-Real-IP": "192.168.1.51"}
	for _, trust := range []bool{false, true} {
		tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) { c.TrustProxyHeaders = trust })
		rr := doReq(tc, http.MethodPost, "/v1/tag", hdr, nil, strings.NewReader(`{"name":"audit-ip"}`))
		if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
			t.Fatalf("create tag: %d %s", rr.Code, rr.Body.String())
		}
		want := "192.0.2.1"
		if trust {
			want = "192.168.1.50"
		}
		if got := newestLog(t, tc, "tag").IPAddress; got != want {
			t.Fatalf("trust=%v: audit IP %q want %q", trust, got, want)
		}
	}
}

func TestAuditClientIP_AdminSettingsActor(t *testing.T) {
	tc := setupAuthEnv(t)
	tc.AppCtx.Settings().SetAuditor(application_context.NewContextAuditor(tc.AppCtx))
	admin := roleBearer(t, tc, models.RoleAdmin)
	hdr := map[string]string{"Content-Type": "application/json", "Authorization": admin, "X-Forwarded-For": "6.6.6.6"}
	var lastID uint
	// Each settings write path (single set, bulk apply, reset) passes its own
	// actor IP to the auditor, so each must write a fresh audit row that
	// records the socket address rather than the forged header.
	step := func(name, method, path, body string) {
		t.Helper()
		rr := doReq(tc, method, path, hdr, nil, strings.NewReader(body))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", name, rr.Code, rr.Body.String())
		}
		e := newestLog(t, tc, "runtime_setting")
		if e.ID == lastID {
			t.Fatalf("%s: no new runtime_setting audit row", name)
		}
		lastID = e.ID
		if e.IPAddress != "192.0.2.1" {
			t.Fatalf("%s: forged XFF recorded: %q", name, e.IPAddress)
		}
	}
	step("set", http.MethodPut, "/v1/admin/settings/max_upload_size", `{"value":"1048576"}`)
	step("apply", http.MethodPut, "/v1/admin/settings", `{"values":{"max_upload_size":"2097152"}}`)
	step("reset", http.MethodDelete, "/v1/admin/settings/max_upload_size", ``)
}
