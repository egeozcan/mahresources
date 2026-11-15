package api_tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/server"
)

const (
	mcpNew    = "2026-07-28"
	mcpLegacy = "2025-11-25"
)

// mcpEnv starts a real HTTP server with the MCP endpoint enabled.
type mcpEnv struct {
	tc     *TestContext
	srv    *httptest.Server
	bearer string // the admin's token ("" when auth is off)
}

func newMCPEnv(t *testing.T, authOn bool, origins ...string) *mcpEnv {
	t.Helper()
	return newMCPEnvOn(t, setupTestEnvWithConfig, authOn, origins...)
}

// newMCPEnvOn builds the environment with setup, so the Postgres suite can
// run the same flows (mcp_pg_test.go).
func newMCPEnvOn(t *testing.T, setup func(*testing.T, func(*application_context.MahresourcesConfig)) *TestContext,
	authOn bool, origins ...string) *mcpEnv {
	t.Helper()
	tc := setup(t, func(c *application_context.MahresourcesConfig) {
		c.MCPEnabled = true
		c.MCPAllowedOrigins = origins
		if authOn {
			c.AuthEnabled = true
			c.SessionTTL = time.Hour
		}
	})
	if tc.DB.Dialector.Name() == "sqlite" {
		if sqlDB, err := tc.DB.DB(); err == nil {
			sqlDB.SetMaxOpenConns(1)
		}
	}
	env := &mcpEnv{tc: tc}
	if authOn {
		admin, err := tc.AppCtx.EnsureAdminUser("admin", "adminpw1")
		if err != nil {
			t.Fatalf("bootstrap admin: %v", err)
		}
		raw, _, err := tc.AppCtx.CreateApiToken(admin.ID, "mcp", nil)
		if err != nil {
			t.Fatalf("admin token: %v", err)
		}
		env.bearer = "Bearer " + raw
	}
	env.srv = httptest.NewServer(tc.Router)
	t.Cleanup(env.srv.Close)
	return env
}

type headerTransport struct {
	headers map[string]string
	base    http.RoundTripper
}

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h.headers {
		r.Header.Set(k, v)
	}
	return h.base.RoundTrip(r)
}

type clientOpts struct {
	protocol string
	bearer   string
	// answer, when set, makes the client advertise form elicitation and
	// answer every elicitation with it.
	answer *mcp.ElicitResult
	// manual disables the client's automatic MRTR retry.
	manual bool
	// seen collects elicitation messages.
	seen *[]string
}

func (e *mcpEnv) connect(t *testing.T, o clientOpts) *mcp.ClientSession {
	t.Helper()
	copts := &mcp.ClientOptions{}
	if o.answer != nil {
		answer := o.answer
		seen := o.seen
		copts.ElicitationHandler = func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			if seen != nil {
				*seen = append(*seen, req.Params.Message)
			}
			return answer, nil
		}
	}
	if o.manual {
		copts.MultiRoundTrip = &mcp.MultiRoundTripOptions{Disabled: true}
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, copts)
	headers := map[string]string{}
	if o.bearer != "" {
		headers["Authorization"] = o.bearer
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:   e.srv.URL + "/mcp",
		HTTPClient: &http.Client{Transport: headerTransport{headers: headers, base: http.DefaultTransport}},
	}
	protocol := o.protocol
	if protocol == "" {
		protocol = mcpNew
	}
	cs, err := client.Connect(context.Background(), transport, &mcp.ClientSessionOptions{ProtocolVersion: protocol})
	if err != nil {
		t.Fatalf("connect (%s): %v", protocol, err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return res
}

func resultText(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

var accept = &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}

func (e *mcpEnv) noteExists(t *testing.T, id uint) bool {
	t.Helper()
	var n int64
	e.tc.DB.Model(&models.Note{}).Where("id = ?", id).Count(&n)
	return n > 0
}

func rawPost(t *testing.T, url string, headers map[string]string) *http.Response {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()
	return resp
}

func TestMCP_DisabledByDefault(t *testing.T) {
	tc := SetupTestEnv(t)
	rr := tc.MakeRequest(http.MethodPost, "/mcp", map[string]any{})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("POST /mcp with MCP off = %d, want 404", rr.Code)
	}
}

func TestMCP_ListsToolsOnBothProtocols(t *testing.T) {
	env := newMCPEnv(t, false)
	for _, protocol := range []string{mcpNew, mcpLegacy} {
		t.Run(protocol, func(t *testing.T) {
			cs := env.connect(t, clientOpts{protocol: protocol})
			res, err := cs.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatalf("list tools: %v", err)
			}
			byName := map[string]*mcp.Tool{}
			for _, tool := range res.Tools {
				byName[tool.Name] = tool
			}
			for _, want := range []string{"search", "mrql_query", "get_note", "delete", "merge", "search_operations", "describe_operation", "call_operation"} {
				if byName[want] == nil {
					t.Errorf("tool %s missing", want)
				}
			}
			if d := byName["delete"]; d != nil && (d.Annotations == nil || d.Annotations.DestructiveHint == nil || !*d.Annotations.DestructiveHint) {
				t.Errorf("delete is not marked destructive: %+v", d.Annotations)
			}
			if g := byName["get_note"]; g != nil && (g.Annotations == nil || !g.Annotations.ReadOnlyHint) {
				t.Errorf("get_note is not marked read-only")
			}
		})
	}
}

func TestMCP_Admission(t *testing.T) {
	env := newMCPEnv(t, true, "https://agent.example")
	url := env.srv.URL + "/mcp"

	// A browser session cookie alone is refused as unauthenticated, before CSRF.
	login := doReq(env.tc, http.MethodPost, "/v1/auth/login", map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		nil, strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	cookie := sessionCookie(t, login)

	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"no token", nil, http.StatusUnauthorized},
		{"cookie only", map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}, http.StatusUnauthorized},
		{"invalid token", map[string]string{"Authorization": "Bearer nope"}, http.StatusUnauthorized},
		{"hostile origin", map[string]string{"Authorization": env.bearer, "Origin": "https://evil.example"}, http.StatusForbidden},
		{"null origin", map[string]string{"Authorization": env.bearer, "Origin": "null"}, http.StatusForbidden},
		{"fetch metadata only", map[string]string{"Authorization": env.bearer, "Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"rebinding: origin equals host", map[string]string{"Authorization": env.bearer, "Origin": env.srv.URL}, http.StatusForbidden},
		{"allowlisted origin", map[string]string{"Authorization": env.bearer, "Origin": "https://agent.example", "MCP-Protocol-Version": mcpLegacy}, http.StatusOK},
		{"token", map[string]string{"Authorization": env.bearer, "MCP-Protocol-Version": mcpLegacy}, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rawPost(t, url, c.headers).StatusCode; got != c.want {
				t.Fatalf("status = %d, want %d", got, c.want)
			}
		})
	}

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req, _ := http.NewRequest(method, url, nil)
		req.Header.Set("Authorization", env.bearer)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s /mcp = %d, want 405", method, resp.StatusCode)
		}
	}
}

func TestMCP_ReadMatchesREST(t *testing.T) {
	env := newMCPEnv(t, true)
	note := env.tc.CreateDummyNote("mcp read note")
	cs := env.connect(t, clientOpts{bearer: env.bearer})

	res := callTool(t, cs, "get_note", map[string]any{"id": note.ID})
	if res.IsError {
		t.Fatalf("get_note failed: %s", resultText(res))
	}
	sc, _ := res.StructuredContent.(map[string]any)
	if sc["Name"] != "mcp read note" {
		t.Fatalf("structured content = %v", res.StructuredContent)
	}

	gen := callTool(t, cs, "call_operation", map[string]any{"operationId": "getNote", "arguments": map[string]any{"id": note.ID}})
	if gen.IsError || !strings.Contains(resultText(gen), "mcp read note") {
		t.Fatalf("call_operation getNote = %s", resultText(gen))
	}

	missing := callTool(t, cs, "get_note", map[string]any{"id": 999999})
	if !missing.IsError || !strings.Contains(resultText(missing), "HTTP 404") {
		t.Fatalf("missing note = %v %s", missing.IsError, resultText(missing))
	}
}

func TestMCP_EditNameReachesTheHandler(t *testing.T) {
	env := newMCPEnv(t, false)
	note := env.tc.CreateDummyNote("before")
	cs := env.connect(t, clientOpts{})
	res := callTool(t, cs, "call_operation", map[string]any{"operationId": "editNoteName", "arguments": map[string]any{"id": note.ID, "Name": "after"}})
	if res.IsError {
		t.Fatalf("editNoteName: %s", resultText(res))
	}
	var got models.Note
	env.tc.DB.First(&got, note.ID)
	if got.Name != "after" {
		t.Fatalf("name = %q, want after", got.Name)
	}
}

func TestMCP_GuestReadsButCannotWrite(t *testing.T) {
	env := newMCPEnv(t, true)
	scope := env.tc.CreateDummyGroup("guest scope")
	u, err := env.tc.AppCtx.CreateUser(&application_context.UserInput{Username: "guest1", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &scope.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := env.tc.AppCtx.CreateApiToken(u.ID, "g", nil)
	if err != nil {
		t.Fatal(err)
	}
	cs := env.connect(t, clientOpts{bearer: "Bearer " + raw})

	if res := callTool(t, cs, "list_notes", map[string]any{}); res.IsError {
		t.Fatalf("guest list_notes refused: %s", resultText(res))
	}
	res := callTool(t, cs, "create_note", map[string]any{"Name": "guest note"})
	if !res.IsError || !strings.Contains(resultText(res), "HTTP 403") {
		t.Fatalf("guest create_note = %v %s", res.IsError, resultText(res))
	}
}

func TestMCP_CredentialOperationsAreUnreachable(t *testing.T) {
	env := newMCPEnv(t, true)
	cs := env.connect(t, clientOpts{bearer: env.bearer})
	list := resultText(callTool(t, cs, "search_operations", map[string]any{}))
	for _, id := range []string{"createOwnToken", "changeOwnPassword", "createUser", "updateUser", "login"} {
		if strings.Contains(list, id+" |") {
			t.Errorf("%s is listed by search_operations", id)
		}
		res := callTool(t, cs, "call_operation", map[string]any{"operationId": id, "arguments": map[string]any{"name": "x"}})
		if !res.IsError {
			t.Errorf("call_operation %s ran: %s", id, resultText(res))
		}
	}
	var tokens int64
	env.tc.DB.Model(&models.ApiToken{}).Count(&tokens)
	if tokens != 1 {
		t.Fatalf("tokens = %d, want only the test's own", tokens)
	}
}

func TestMCP_BinaryOperationsReturnLinks(t *testing.T) {
	env := newMCPEnv(t, false)
	cs := env.connect(t, clientOpts{})
	res := callTool(t, cs, "call_operation", map[string]any{"operationId": "viewResource", "arguments": map[string]any{"id": 5}})
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("viewResource = %s", resultText(res))
	}
	link, ok := res.Content[0].(*mcp.ResourceLink)
	if !ok || !strings.HasSuffix(link.URI, "/v1/resource/view?id=5") {
		t.Fatalf("first content = %#v", res.Content[0])
	}
}

// --- confirmations ---

func TestMCP_DestructiveRefusedWithoutAConfirmingClient(t *testing.T) {
	env := newMCPEnv(t, true)
	for _, c := range []struct {
		name string
		opts clientOpts
	}{
		{"legacy protocol with elicitation", clientOpts{protocol: mcpLegacy, answer: accept}},
		{"legacy protocol", clientOpts{protocol: mcpLegacy}},
		{"new protocol without elicitation", clientOpts{protocol: mcpNew}},
	} {
		t.Run(c.name, func(t *testing.T) {
			note := env.tc.CreateDummyNote("keep me " + c.name)
			c.opts.bearer = env.bearer
			cs := env.connect(t, c.opts)
			res := callTool(t, cs, "delete", map[string]any{"entity": "note", "id": note.ID})
			if !res.IsError || !strings.Contains(resultText(res), "needs a confirmation this client cannot show") {
				t.Fatalf("delete = %v %s", res.IsError, resultText(res))
			}
			if !strings.Contains(resultText(res), "keep me") {
				t.Errorf("refusal does not carry the preview: %s", resultText(res))
			}
			if !env.noteExists(t, note.ID) {
				t.Fatal("the note was deleted without a confirmation")
			}
		})
	}
}

func TestMCP_ConfirmAcceptDeletes(t *testing.T) {
	env := newMCPEnv(t, true)
	note := env.tc.CreateDummyNote("doomed note")
	var seen []string
	cs := env.connect(t, clientOpts{bearer: env.bearer, answer: accept, seen: &seen})
	res := callTool(t, cs, "delete", map[string]any{"entity": "note", "id": note.ID})
	if res.IsError {
		t.Fatalf("confirmed delete failed: %s", resultText(res))
	}
	if env.noteExists(t, note.ID) {
		t.Fatal("note still exists after a confirmed delete")
	}
	if len(seen) != 1 || !strings.Contains(seen[0], `"doomed note"`) || !strings.Contains(seen[0], "deleteNote") {
		t.Fatalf("elicitation messages = %q", seen)
	}
}

func TestMCP_ConfirmDeclineAndCancelKeep(t *testing.T) {
	env := newMCPEnv(t, true)
	for _, answer := range []*mcp.ElicitResult{
		{Action: "decline"},
		{Action: "cancel"},
		{Action: "accept", Content: map[string]any{"confirm": false}},
	} {
		note := env.tc.CreateDummyNote("survivor")
		cs := env.connect(t, clientOpts{bearer: env.bearer, answer: answer})
		res := callTool(t, cs, "call_operation", map[string]any{"operationId": "deleteNote", "arguments": map[string]any{"Id": note.ID}})
		if res.IsError || !strings.Contains(resultText(res), "Cancelled") {
			t.Fatalf("%+v: %v %s", answer, res.IsError, resultText(res))
		}
		if !env.noteExists(t, note.ID) {
			t.Fatalf("%+v deleted the note", answer)
		}
	}
}

// challengeFor runs a destructive call with MRTR disabled and returns the
// requestState of the confirmation it asks for.
func challengeFor(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res := callTool(t, cs, name, args)
	if !res.NeedsInput() || res.RequestState == "" {
		t.Fatalf("%s did not ask for a confirmation: %v %s", name, res.IsError, resultText(res))
	}
	if _, ok := res.InputRequests["confirm"].(*mcp.ElicitParams); !ok {
		t.Fatalf("input requests = %#v", res.InputRequests)
	}
	return res.RequestState
}

func retry(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, state string) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: name, Arguments: args, RequestState: state,
		InputResponses: mcp.InputResponseMap{"confirm": accept},
	})
	if err != nil {
		t.Fatalf("retry %s: %v", name, err)
	}
	return res
}

func TestMCP_ConfirmationIsBoundToItsCall(t *testing.T) {
	env := newMCPEnv(t, true)
	manual := clientOpts{bearer: env.bearer, answer: accept, manual: true}
	cs := env.connect(t, manual)

	a := env.tc.CreateDummyNote("a")
	b := env.tc.CreateDummyNote("b")
	group := env.tc.CreateDummyGroup("g")
	argsA := map[string]any{"operationId": "deleteNote", "arguments": map[string]any{"Id": a.ID}}
	state := challengeFor(t, cs, "call_operation", argsA)

	mustRefuse := func(label, name string, args map[string]any, st string, wantMsg string) {
		t.Helper()
		res := retry(t, cs, name, args, st)
		if !res.IsError || !strings.Contains(resultText(res), wantMsg) {
			t.Fatalf("%s: %v %q", label, res.IsError, resultText(res))
		}
	}

	mustRefuse("other arguments", "call_operation",
		map[string]any{"operationId": "deleteNote", "arguments": map[string]any{"Id": b.ID}}, state, "different caller, operation or arguments")
	mustRefuse("other operation, same arguments", "call_operation",
		map[string]any{"operationId": "deleteGroup", "arguments": map[string]any{"Id": a.ID}}, state, "different caller, operation or arguments")
	mustRefuse("other tool, same operation and arguments", "delete",
		map[string]any{"entity": "note", "id": a.ID}, state, "different caller, operation or arguments")
	mustRefuse("tampered state", "call_operation", argsA, state[:len(state)-4]+"AAAA", "invalid")

	// Another token of the same user, and another user, are both refused.
	admin, _ := env.tc.AppCtx.GetUserByUsername("admin")
	raw2, _, err := env.tc.AppCtx.CreateApiToken(admin.ID, "second", nil)
	if err != nil {
		t.Fatal(err)
	}
	other := env.connect(t, clientOpts{bearer: "Bearer " + raw2, answer: accept, manual: true})
	mustRefuseOn := func(label string, cs *mcp.ClientSession) {
		t.Helper()
		res := retry(t, cs, "call_operation", argsA, state)
		if !res.IsError {
			t.Fatalf("%s: retry ran: %s", label, resultText(res))
		}
	}
	mustRefuseOn("second token", other)
	editor, err := env.tc.AppCtx.CreateUser(&application_context.UserInput{Username: "ed", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatal(err)
	}
	raw3, _, _ := env.tc.AppCtx.CreateApiToken(editor.ID, "e", nil)
	mustRefuseOn("other user", env.connect(t, clientOpts{bearer: "Bearer " + raw3, answer: accept, manual: true}))

	if !env.noteExists(t, a.ID) || !env.noteExists(t, b.ID) {
		t.Fatal("a refused retry deleted a note")
	}
	var groups int64
	env.tc.DB.Model(&models.Group{}).Where("id = ?", group.ID).Count(&groups)
	if groups != 1 {
		t.Fatal("the group was touched")
	}

	// The genuine retry runs once; replaying it is refused.
	if res := retry(t, cs, "call_operation", argsA, state); res.IsError {
		t.Fatalf("genuine retry failed: %s", resultText(res))
	}
	if env.noteExists(t, a.ID) {
		t.Fatal("genuine retry did not delete")
	}
	mustRefuse("replay", "call_operation", argsA, state, "already used")
}

func TestMCP_ConcurrentRetriesRunOnce(t *testing.T) {
	env := newMCPEnv(t, false)
	cs := env.connect(t, clientOpts{answer: accept, manual: true})
	note := env.tc.CreateDummyNote("raced")
	args := map[string]any{"entity": "note", "id": note.ID}
	state := challengeFor(t, cs, "delete", args)

	const n = 6
	var wg sync.WaitGroup
	results := make([]*mcp.CallToolResult, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "delete", Arguments: args, RequestState: state,
				InputResponses: mcp.InputResponseMap{"confirm": accept},
			})
			if err == nil {
				results[i] = r
			}
		}()
	}
	wg.Wait()
	ran, used := 0, 0
	for _, r := range results {
		switch {
		case r == nil:
		case !r.IsError:
			ran++
		case strings.Contains(resultText(r), "already used"):
			used++
		}
	}
	if ran != 1 || used != n-1 {
		t.Fatalf("ran=%d used=%d, want 1 and %d", ran, used, n-1)
	}
}

func TestMCP_ConfirmationDoesNotSurviveRestart(t *testing.T) {
	env := newMCPEnv(t, false)
	cs := env.connect(t, clientOpts{answer: accept, manual: true})
	note := env.tc.CreateDummyNote("restart")
	args := map[string]any{"entity": "note", "id": note.ID}
	state := challengeFor(t, cs, "delete", args)

	// A second server over the same database and signing key stands in for a
	// restarted process: it draws a new boot nonce.
	restarted := httptest.NewServer(server.CreateServer(env.tc.AppCtx, env.tc.Fs, map[string]string{}).Handler)
	t.Cleanup(restarted.Close)
	env2 := &mcpEnv{tc: env.tc, srv: restarted}
	cs2 := env2.connect(t, clientOpts{answer: accept, manual: true})
	res := retry(t, cs2, "delete", args, state)
	if !res.IsError || !strings.Contains(resultText(res), "restarted") {
		t.Fatalf("pre-restart state = %v %s", res.IsError, resultText(res))
	}
	if !env.noteExists(t, note.ID) {
		t.Fatal("note deleted with a pre-restart confirmation")
	}
}

func TestMCP_ScopedPreviewNamesOnlyVisibleTargets(t *testing.T) {
	env := newMCPEnv(t, true)
	root := &models.Group{Name: "mcp-root"}
	env.tc.DB.Create(root)
	outside := &models.Group{Name: "mcp-outside"}
	env.tc.DB.Create(outside)
	inNote := &models.Note{Name: "visible-note", OwnerId: &root.ID}
	env.tc.DB.Create(inNote)
	outNote := &models.Note{Name: "secret-note", OwnerId: &outside.ID}
	env.tc.DB.Create(outNote)

	bearer := scopedUserBearer(t, env.tc, root.ID)
	var seen []string
	cs := env.connect(t, clientOpts{bearer: bearer, answer: accept, seen: &seen})

	list := resultText(callTool(t, cs, "list_notes", map[string]any{}))
	if strings.Contains(list, "secret-note") || !strings.Contains(list, "visible-note") {
		t.Fatalf("scoped list_notes = %s", list)
	}

	res := callTool(t, cs, "call_operation", map[string]any{"operationId": "bulkDeleteNotes",
		"arguments": map[string]any{"ID": []any{inNote.ID, outNote.ID}}})
	if len(seen) != 1 {
		t.Fatalf("elicitations = %q", seen)
	}
	if strings.Contains(seen[0], "secret-note") || !strings.Contains(seen[0], "visible-note") ||
		!strings.Contains(seen[0], "not found or not visible") {
		t.Fatalf("preview = %q", seen[0])
	}
	_ = res
	if !env.noteExists(t, outNote.ID) {
		t.Fatal("a scoped user's confirmed bulk delete removed a note outside the scope")
	}
}

func TestMCP_MassEditNeedsExplicitIDs(t *testing.T) {
	env := newMCPEnv(t, false)
	cs := env.connect(t, clientOpts{answer: accept})
	res := callTool(t, cs, "call_operation", map[string]any{"operationId": "massEditNotes",
		"arguments": map[string]any{"Filter": "type = note", "Meta": `{"x":1}`}})
	if !res.IsError || !strings.Contains(resultText(res), "must name its targets with ID") {
		t.Fatalf("filter mass edit = %v %s", res.IsError, resultText(res))
	}
}

func TestMCP_SearchOperationsFindsEverything(t *testing.T) {
	env := newMCPEnv(t, false)
	cs := env.connect(t, clientOpts{})
	out := resultText(callTool(t, cs, "search_operations", map[string]any{"query": "reduction apply"}))
	if !strings.Contains(out, "applyResourceReduction") || !strings.Contains(out, "destructive") {
		t.Fatalf("search = %s", out)
	}
	desc := callTool(t, cs, "describe_operation", map[string]any{"operationId": "cropResource"})
	var d map[string]any
	if err := json.Unmarshal([]byte(resultText(desc)), &d); err != nil || d["inputSchema"] == nil {
		t.Fatalf("describe cropResource = %s", resultText(desc))
	}
}

// encoding/json matches field names case-insensitively, so a guard that
// looks for "Filter" or sets "DryRun" must see the argument under its
// declared name whatever its casing (review round 1, findings 1 and 2).
func TestMCP_CaseVariantArgumentsCannotDodgeGuards(t *testing.T) {
	env := newMCPEnv(t, false)
	target := env.tc.CreateDummyNote("pinned")
	other := env.tc.CreateDummyNote("unexpected-target")

	// A client that cannot confirm: nothing destructive may run, preview or not.
	cs := env.connect(t, clientOpts{})
	res := callTool(t, cs, "call_operation", map[string]any{"operationId": "massEditNotes", "arguments": map[string]any{
		"ID": []any{target.ID}, "MetaOp": "replace", "Meta": `{"bypassed":true}`, "dryRun": false,
	}})
	if !res.IsError {
		t.Fatalf("unconfirmable mass edit = %s", resultText(res))
	}
	var got models.Note
	env.tc.DB.First(&got, target.ID)
	if strings.Contains(string(got.Meta), "bypassed") {
		t.Fatalf("the preview's dry run wrote meta: %s", got.Meta)
	}

	confirming := env.connect(t, clientOpts{answer: accept})
	for _, args := range []map[string]any{
		{"ID": []any{target.ID}, "Target": "filter", "filter": "name = \"unexpected-target\"", "ExpectedCount": 1, "MetaOp": "replace", "Meta": `{"changed":true}`},
		{"ID": []any{target.ID}, "target": "mrql", "mrqlquery": "name = \"unexpected-target\"", "ExpectedCount": 1, "MetaOp": "replace", "Meta": `{"changed":true}`},
		{"ID": []any{target.ID}, "Target": "filter", "MetaOp": "replace", "Meta": `{"changed":true}`},
	} {
		res := callTool(t, confirming, "call_operation", map[string]any{"operationId": "massEditNotes", "arguments": args})
		if !res.IsError {
			t.Fatalf("selection by %v ran: %s", args, resultText(res))
		}
	}
	var otherAfter models.Note
	env.tc.DB.First(&otherAfter, other.ID)
	if strings.Contains(string(otherAfter.Meta), "changed") {
		t.Fatalf("a filter-selected note was edited: %s", otherAfter.Meta)
	}

	dup := callTool(t, confirming, "call_operation", map[string]any{"operationId": "deleteNote", "arguments": map[string]any{
		"Id": target.ID, "id": other.ID,
	}})
	if !dup.IsError || !strings.Contains(resultText(dup), "given twice") {
		t.Fatalf("ambiguous arguments = %v %s", dup.IsError, resultText(dup))
	}
	if !env.noteExists(t, target.ID) || !env.noteExists(t, other.ID) {
		t.Fatal("an ambiguous delete removed a note")
	}
}

func TestMCP_BulkRevokeSharesReachesTheHandler(t *testing.T) {
	env := newMCPEnv(t, false)
	note := env.tc.CreateDummyNote("shared")
	token := "tok-" + strings.Repeat("a", 28)
	env.tc.DB.Model(&models.Note{}).Where("id = ?", note.ID).Update("share_token", token)

	cs := env.connect(t, clientOpts{answer: accept})
	res := callTool(t, cs, "call_operation", map[string]any{"operationId": "bulkRevokeShares", "arguments": map[string]any{"ids": []any{note.ID}}})
	if res.IsError {
		t.Fatalf("bulkRevokeShares: %s", resultText(res))
	}
	var got models.Note
	env.tc.DB.First(&got, note.ID)
	if got.ShareToken != nil && *got.ShareToken != "" {
		t.Fatalf("share token still set after a confirmed revoke: %s", resultText(res))
	}
}

// Applying a reduction destroys every checked cluster's losers, so the
// confirmation must say how many (review round 1, finding 8).
func TestMCP_ReductionConfirmationStatesWhatItRemoves(t *testing.T) {
	env := newMCPEnv(t, false)
	a := env.tc.CreateResourceWithType(t, "dup-a.jpg", "image/jpeg")
	b := env.tc.CreateResourceWithType(t, "dup-b.jpg", "image/jpeg")
	red := createReduction(t, env.tc, "Holiday duplicates", []uint{a.ID, b.ID})

	var seen []string
	cs := env.connect(t, clientOpts{answer: &mcp.ElicitResult{Action: "decline"}, seen: &seen})
	callTool(t, cs, "call_operation", map[string]any{"operationId": "applyResourceReduction",
		"arguments": map[string]any{"id": red.ID, "version": red.Version}})
	if len(seen) != 1 || !strings.Contains(seen[0], `Reduction "Holiday duplicates": 0 checked clusters; 0 resources would be removed`) {
		t.Fatalf("reduction preview = %q", seen)
	}
}

// encoding/json folds names with Unicode simple folding, so "Loſers"
// (U+017F) reaches the Losers field. Folding by ASCII case alone would let a
// second spelling replace the confirmed targets (review round 2, finding 1).
func TestMCP_UnicodeFoldedArgumentIsADuplicate(t *testing.T) {
	env := newMCPEnv(t, false)
	winner := env.tc.CreateDummyGroup("winner")
	shown := env.tc.CreateDummyGroup("shown")
	hidden := env.tc.CreateDummyGroup("hidden")
	cs := env.connect(t, clientOpts{answer: accept})
	res := callTool(t, cs, "call_operation", map[string]any{"operationId": "mergeGroups", "arguments": map[string]any{
		"Winner": winner.ID, "Losers": []any{shown.ID}, "Loſers": []any{hidden.ID},
	}})
	if !res.IsError || !strings.Contains(resultText(res), "given twice") {
		t.Fatalf("folded duplicate = %v %s", res.IsError, resultText(res))
	}
	var n int64
	env.tc.DB.Model(&models.Group{}).Where("id IN ?", []uint{shown.ID, hidden.ID}).Count(&n)
	if n != 2 {
		t.Fatalf("a refused merge removed %d groups", 2-n)
	}
}

// Operations that destroy data as a side effect need a confirmation too
// (review round 2, findings 2-5).
func TestMCP_SideEffectDestructionNeedsConfirmation(t *testing.T) {
	env := newMCPEnv(t, false)
	res := env.tc.CreateResourceWithType(t, "in-series.jpg", "image/jpeg")
	series := &models.Series{Name: "only member", Slug: "only-member"}
	env.tc.DB.Create(series)
	env.tc.DB.Model(&models.Resource{}).Where("id = ?", res.ID).Update("series_id", series.ID)

	cs := env.connect(t, clientOpts{}) // cannot confirm
	for _, c := range []struct {
		op   string
		args map[string]any
	}{
		{"removeResourceFromSeries", map[string]any{"id": res.ID}},
		{"rotateResource", map[string]any{"ID": res.ID, "Degrees": 90}},
		{"runPluginAction", map[string]any{"plugin": "x", "action": "y"}},
		{"editNoteType", map[string]any{"ID": 1, "ApplyTemplatesToShares": true}},
	} {
		out := callTool(t, cs, "call_operation", map[string]any{"operationId": c.op, "arguments": c.args})
		if !out.IsError || !strings.Contains(resultText(out), "needs a confirmation") {
			t.Errorf("%s ran without a confirmation: %s", c.op, resultText(out))
		}
	}
	var n int64
	env.tc.DB.Model(&models.Series{}).Where("id = ?", series.ID).Count(&n)
	if n != 1 {
		t.Fatal("the series was deleted without a confirmation")
	}

	// An edit that leaves series membership alone stays a plain edit.
	plain := callTool(t, cs, "edit_resource", map[string]any{"ID": res.ID, "Name": "renamed.jpg"})
	if plain.IsError {
		t.Fatalf("a plain edit was refused: %s", resultText(plain))
	}
}

// Review round 3: updates whose handlers delete relations, and template
// partials that share pages render, need a confirmation.
func TestMCP_RelationAndPublicationSideEffectsNeedConfirmation(t *testing.T) {
	env := newMCPEnv(t, false)
	group := env.tc.CreateDummyGroup("categorised")
	cs := env.connect(t, clientOpts{}) // cannot confirm
	for _, c := range []struct {
		op   string
		args map[string]any
	}{
		{"createOrUpdateGroup", map[string]any{"ID": group.ID, "Name": "categorised", "CategoryId": 2}},
		{"editRelationType", map[string]any{"Id": 1, "FromCategory": 2}},
		{"createOrUpdateTemplatePartial", map[string]any{"Name": "share-banner", "Content": "x"}},
		{"updateTemplatePartial", map[string]any{"ID": 7, "Content": "x"}},
	} {
		out := callTool(t, cs, "call_operation", map[string]any{"operationId": c.op, "arguments": c.args})
		if !out.IsError || !strings.Contains(resultText(out), "needs a confirmation") {
			t.Errorf("%s ran without a confirmation: %s", c.op, resultText(out))
		}
	}
	// Creating a group touches no existing relation and stays a plain write.
	if out := callTool(t, cs, "create_group", map[string]any{"Name": "fresh"}); out.IsError {
		t.Fatalf("create_group was refused: %s", resultText(out))
	}
}

// MetaQuery entries are parsed as key:OP:value strings, so the schema must
// say so instead of advertising objects that would be misread (finding 4).
func TestMCP_MetaQueryFiltersAsStrings(t *testing.T) {
	env := newMCPEnv(t, false)
	open := env.tc.CreateDummyNote("open one")
	env.tc.DB.Model(&models.Note{}).Where("id = ?", open.ID).Update("meta", `{"status":"open"}`)
	closed := env.tc.CreateDummyNote("closed one")
	env.tc.DB.Model(&models.Note{}).Where("id = ?", closed.ID).Update("meta", `{"status":"closed"}`)

	cs := env.connect(t, clientOpts{})
	desc := resultText(callTool(t, cs, "describe_operation", map[string]any{"operationId": "listNotes"}))
	if !strings.Contains(desc, "key:OP:value") {
		t.Fatalf("listNotes MetaQuery schema does not describe its grammar: %s", desc)
	}
	res := resultText(callTool(t, cs, "list_notes", map[string]any{"MetaQuery": []any{"status:EQ:open"}}))
	if !strings.Contains(res, "open one") || strings.Contains(res, "closed one") {
		t.Fatalf("MetaQuery filter = %s", res)
	}
	bad := callTool(t, cs, "list_notes", map[string]any{"MetaQuery": []any{map[string]any{"name": "status", "operation": "EQ", "value": "open"}}})
	if !bad.IsError {
		t.Fatalf("an object MetaQuery entry was accepted: %s", resultText(bad))
	}
}

// Review round 5: editResource cannot remove series membership over JSON,
// so MCP points at removeResourceFromSeries instead of confirming a no-op;
// saved-query bind values that share a routing parameter's name go under
// "body".
func TestMCP_SeriesRemovalAndBodyBindValues(t *testing.T) {
	env := newMCPEnv(t, false)
	res := env.tc.CreateResourceWithType(t, "s.jpg", "image/jpeg")
	cs := env.connect(t, clientOpts{answer: accept})
	out := callTool(t, cs, "edit_resource", map[string]any{"ID": res.ID, "SeriesId": 0})
	if !out.IsError || !strings.Contains(resultText(out), "removeResourceFromSeries") {
		t.Fatalf("edit_resource SeriesId 0 = %v %s", out.IsError, resultText(out))
	}

	q := &models.Query{Name: "bind-name", Text: "SELECT :name AS value"}
	env.tc.DB.Create(q)
	run := callTool(t, cs, "call_operation", map[string]any{"operationId": "runQuery",
		"arguments": map[string]any{"id": q.ID, "body": map[string]any{"name": "Alice"}}})
	if run.IsError || !strings.Contains(resultText(run), "Alice") {
		t.Fatalf("runQuery with a body bind value = %v %s", run.IsError, resultText(run))
	}
}

// Review round 8: MRQL reads param.<name> bind values and export preflight
// from the query string only.
func TestMCP_MRQLParamsAndExportPreflight(t *testing.T) {
	env := newMCPEnv(t, false)
	env.tc.CreateDummyNote("meeting notes")
	cs := env.connect(t, clientOpts{})

	res := callTool(t, cs, "mrql_query", map[string]any{"query": `type = "note" AND name ~ $needle`, "param.needle": "meeting"})
	if res.IsError || !strings.Contains(resultText(res), "meeting notes") {
		t.Fatalf("param.needle = %v %s", res.IsError, resultText(res))
	}
	pre := callTool(t, cs, "call_operation", map[string]any{"operationId": "exportMRQL",
		"arguments": map[string]any{"query": `type = "note" LIMIT 10`, "format": "json", "preflight": "1"}})
	if pre.IsError || !strings.Contains(resultText(pre), "HTTP 204") {
		t.Fatalf("export preflight = %v %s", pre.IsError, resultText(pre))
	}
}

// Review round 10: a field hidden in "body" must not retarget a confirmed
// call, and a confirmation shows every argument it covers.
func TestMCP_BodyCannotRetargetAConfirmation(t *testing.T) {
	env := newMCPEnv(t, false)
	shown := env.tc.CreateDummyNote("shown")
	hidden := env.tc.CreateDummyNote("hidden")
	cs := env.connect(t, clientOpts{answer: accept})
	for _, args := range []map[string]any{
		{"Id": shown.ID, "body": map[string]any{"ID": hidden.ID}},
		{"Id": shown.ID, "body": map[string]any{"id": hidden.ID}},
		{"Id": shown.ID, "Extra": 1, "body": map[string]any{"extra": 2}},
	} {
		res := callTool(t, cs, "call_operation", map[string]any{"operationId": "deleteNote", "arguments": args})
		if !res.IsError {
			t.Fatalf("%v ran: %s", args, resultText(res))
		}
	}
	if !env.noteExists(t, shown.ID) || !env.noteExists(t, hidden.ID) {
		t.Fatal("a refused call deleted a note")
	}

	huge := callTool(t, cs, "call_operation", map[string]any{"operationId": "deleteNote",
		"arguments": map[string]any{"Id": shown.ID, "AAAA": strings.Repeat("x", 20000)}})
	if !huge.IsError || !strings.Contains(resultText(huge), "too large") {
		t.Fatalf("oversized confirmation = %v %s", huge.IsError, clipForLog(resultText(huge)))
	}

	var seen []string
	full := env.connect(t, clientOpts{answer: &mcp.ElicitResult{Action: "decline"}, seen: &seen})
	callTool(t, full, "call_operation", map[string]any{"operationId": "deleteNote",
		"arguments": map[string]any{"Id": shown.ID, "AAAA": strings.Repeat("y", 1100) + "TAIL"}})
	if len(seen) != 1 || !strings.Contains(seen[0], "TAIL") {
		t.Fatalf("the confirmation clipped its arguments")
	}
}

func TestMCP_SearchSeveralTypes(t *testing.T) {
	env := newMCPEnv(t, false)
	env.tc.CreateDummyNote("needle note")
	env.tc.CreateDummyGroup("needle group")
	cs := env.connect(t, clientOpts{})
	desc := resultText(callTool(t, cs, "describe_operation", map[string]any{"operationId": "globalSearch"}))
	if !strings.Contains(desc, `"types":{"description"`) || strings.Contains(desc, `"types":{"items"`) {
		t.Fatalf("globalSearch types is not documented as one comma-separated string: %s", desc)
	}
	res := resultText(callTool(t, cs, "search", map[string]any{"q": "needle", "types": "note,group"}))
	if !strings.Contains(res, "needle note") || !strings.Contains(res, "needle group") {
		t.Fatalf("multi-type search = %s", res)
	}
}

func clipForLog(s string) string {
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}

// Review round 11: a body can be an opaque, case-sensitive map.
func TestMCP_OpaqueBodyKeepsCaseDistinctKeys(t *testing.T) {
	env := newMCPEnv(t, false)
	cs := env.connect(t, clientOpts{answer: accept})
	res := callTool(t, cs, "call_operation", map[string]any{"operationId": "updatePluginSettings",
		"arguments": map[string]any{"name": "no-such-plugin", "body": map[string]any{"key": "lower", "Key": "upper", "name": "Alice"}}})
	if strings.Contains(resultText(res), "given twice") || strings.Contains(resultText(res), "duplicates") {
		t.Fatalf("case-distinct settings were refused: %s", resultText(res))
	}
}
