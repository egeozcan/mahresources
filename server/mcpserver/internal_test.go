package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSafetyTablesDoNotOverlap(t *testing.T) {
	seen := map[string]string{}
	check := func(table string, ids map[string]bool) {
		for id := range ids {
			if prev, ok := seen[id]; ok {
				t.Errorf("%s is in both %s and %s", id, prev, table)
			}
			seen[id] = table
		}
	}
	check("readOnlyPosts", readOnlyPosts)
	check("destructive", destructive)
	check("mutating", mutating)
	ex := map[string]bool{}
	for id := range excluded {
		ex[id] = true
	}
	check("excluded", ex)
}

func testConfirmer(t *testing.T) *confirmer {
	t.Helper()
	c, err := newConfirmerWithBoot([]byte("key"), []byte("boot"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestConfirmationExpires(t *testing.T) {
	c := testConfirmer(t)
	now := time.Unix(1_000_000, 0)
	c.now = func() time.Time { return now }
	b := challenge{UserID: 1, AuthHash: "a", Tool: "delete", OpID: "deleteNote", ArgsHash: "h"}
	state, err := c.issue(b)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(confirmationTTL + time.Second)
	if err := c.redeem(state, b); err != errChallengeExpired {
		t.Fatalf("redeem after expiry = %v, want expired", err)
	}
}

func TestConfirmationFromAnotherBootIsInvalid(t *testing.T) {
	a := testConfirmer(t)
	b, err := newConfirmerWithBoot([]byte("key"), []byte("other boot"))
	if err != nil {
		t.Fatal(err)
	}
	want := challenge{UserID: 1, OpID: "deleteNote"}
	state, _ := a.issue(want)
	if err := b.redeem(state, want); err != errChallengeInvalid {
		t.Fatalf("redeem on another boot = %v, want invalid", err)
	}
	if err := a.redeem(state, want); err != nil {
		t.Fatalf("redeem on the issuing boot = %v", err)
	}
}

func TestUsedNoncesAreSweptAfterExpiry(t *testing.T) {
	c := testConfirmer(t)
	now := time.Unix(2_000_000, 0)
	c.now = func() time.Time { return now }
	want := challenge{OpID: "x"}
	s1, _ := c.issue(want)
	if err := c.redeem(s1, want); err != nil {
		t.Fatal(err)
	}
	now = now.Add(confirmationTTL + time.Minute)
	s2, _ := c.issue(want)
	if err := c.redeem(s2, want); err != nil {
		t.Fatal(err)
	}
	if len(c.used) != 1 {
		t.Fatalf("used set holds %d nonces, want 1 after the sweep", len(c.used))
	}
}

func TestHashArgsIsStableAcrossKeyOrderAndNumberText(t *testing.T) {
	var a, b map[string]any
	decode := func(s string) map[string]any {
		m, err := decodeArgs(json.RawMessage(s))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	a, b = decode(`{"Id": 7, "x": [1,2]}`), decode(`{"x":[1,2],"Id":7}`)
	ha, _ := hashArgs(a)
	hb, _ := hashArgs(b)
	if ha != hb {
		t.Fatal("equal arguments hash differently")
	}
	hc, _ := hashArgs(decode(`{"Id": 8, "x": [1,2]}`))
	if ha == hc {
		t.Fatal("different arguments hash equally")
	}
}

func TestDispatchCutsOffOversizedResponses(t *testing.T) {
	var ctxErr error
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := []byte(strings.Repeat("x", 1024))
		for range 100 {
			if _, err := w.Write(chunk); err != nil {
				break
			}
		}
		ctxErr = r.Context().Err()
	})
	parent := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	op := &Operation{ID: "big", Method: http.MethodGet, Path: "/v1/big"}
	resp, err := dispatch(handler, parent, op, map[string]any{}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Truncated || len(resp.Body) > 4096 {
		t.Fatalf("truncated=%v len=%d", resp.Truncated, len(resp.Body))
	}
	if ctxErr == nil {
		t.Fatal("the handler's context was not cancelled when the limit was hit")
	}
}

func TestDispatchInheritsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var got error
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.Context().Err() })
	parent := httptest.NewRequest(http.MethodPost, "/mcp", nil).WithContext(ctx)
	if _, err := dispatch(handler, parent, &Operation{Method: http.MethodGet, Path: "/v1/x"}, nil, 1024); err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("a cancelled /mcp request did not cancel the dispatched handler")
	}
}

func TestDispatchRefusesTheMCPEndpoint(t *testing.T) {
	parent := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	_, err := dispatch(http.NotFoundHandler(), parent, &Operation{Method: http.MethodPost, Path: mcpPath}, nil, 1024)
	if err == nil {
		t.Fatal("dispatch to /mcp was allowed")
	}
}

func TestBuildRequestEncoding(t *testing.T) {
	parent := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	parent.Header.Set("Authorization", "Bearer tok")
	parent.Header.Set("Cookie", "session=steal")
	parent.RemoteAddr = "10.1.2.3:4444"

	op := &Operation{
		Method: http.MethodPost,
		Path:   "/v1/jobs/{id}/commands/{command}",
		Params: []Param{
			{Name: "id", In: "path", Required: true},
			{Name: "command", In: "path", Required: true},
			{Name: "page", In: "query"},
		},
		Body: &Body{ContentType: "application/json", Schema: map[string]any{"type": "object"}},
	}
	for _, bad := range []string{"a/b", "..", ".", `a\\b`, ""} {
		if _, err := buildRequest(context.Background(), parent, op, map[string]any{"id": bad, "command": "retry"}); err == nil {
			t.Errorf("path segment %q was accepted", bad)
		}
	}
	args, _ := decodeArgs(json.RawMessage(`{"id":"a b?c","command":"retry","page":2,"Note":{"k":1}}`))
	req, err := buildRequest(context.Background(), parent, op, args)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.EscapedPath() != "/v1/jobs/a%20b%3Fc/commands/retry" || req.URL.Query().Get("page") != "2" {
		t.Fatalf("url = %s", req.URL.String())
	}
	if req.Header.Get("Authorization") != "Bearer tok" || req.Header.Get("Cookie") != "" {
		t.Fatalf("headers = %v", req.Header)
	}
	if req.RemoteAddr != "10.1.2.3:4444" {
		t.Fatalf("remote addr = %s", req.RemoteAddr)
	}
	body, _ := io.ReadAll(req.Body)
	if string(body) != `{"Note":{"k":1}}` || req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("body = %s (%s)", body, req.Header.Get("Content-Type"))
	}

	if _, err := buildRequest(context.Background(), parent, op, map[string]any{"command": "x"}); err == nil {
		t.Fatal("a missing path parameter was accepted")
	}

	form := &Operation{Method: http.MethodPost, Path: "/v1/x",
		Body: &Body{ContentType: "application/x-www-form-urlencoded", Schema: map[string]any{"type": "object"}}}
	fargs, _ := decodeArgs(json.RawMessage(`{"ID":[1,2],"Name":"n","On":true}`))
	freq, _ := buildRequest(context.Background(), parent, form, fargs)
	fb, _ := io.ReadAll(freq.Body)
	if string(fb) != "ID=1&ID=2&Name=n&On=true" {
		t.Fatalf("form body = %s", fb)
	}
}

func TestAdmissionLeavesOtherPathsAlone(t *testing.T) {
	called := false
	h := Admission(true, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodGet, "/v1/notes", nil)
	req.Header.Set("Origin", "https://evil.example")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !called {
		t.Fatal("Admission intercepted a non-MCP path")
	}
}

func TestFoldKeyMatchesEqualFold(t *testing.T) {
	names := []string{"Losers", "losers", "LOSERS", "Lo\u017fers", "Id", "ID", "id", "\u212aey", "Key", "key", "Winner", "Name", "name\u00df", "NAMESS"}
	for _, a := range names {
		for _, b := range names {
			if got, want := foldKey(a) == foldKey(b), strings.EqualFold(a, b); got != want {
				t.Errorf("foldKey(%q)==foldKey(%q) is %v, strings.EqualFold says %v", a, b, got, want)
			}
		}
	}
}
