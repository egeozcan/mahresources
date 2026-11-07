package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/server/api_handlers"
)

// flipAccess is a TemplateRunAccessFactory whose credential can be revoked.
type flipAccess struct {
	tc      *TestContext
	revoked atomic.Bool
}

func (f *flipAccess) factory() api_handlers.TemplateRunAccessFactory {
	return func(*http.Request) func(context.Context) (*application_context.MahresourcesContext, error) {
		return func(ctx context.Context) (*application_context.MahresourcesContext, error) {
			if f.revoked.Load() {
				return nil, application_context.ErrTemplateRunAccessChanged
			}
			return f.tc.AppCtx.WithParentContext(ctx), nil
		}
	}
}

// callGenerate runs the generate handler directly for a principal, on the
// context scopedAPI would hand it.
func callGenerate(tc *TestContext, access api_handlers.TemplateRunAccessFactory, principal *auth.Principal, body map[string]any, header http.Header) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	callGenerateTo(tc, rr, context.Background(), access, principal, body, header)
	return rr
}

func callGenerateTo(tc *TestContext, w http.ResponseWriter, parent context.Context, access api_handlers.TemplateRunAccessFactory, principal *auth.Principal, body map[string]any, header http.Header) {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, categoryGenerate, bytes.NewReader(raw)).WithContext(auth.WithPrincipal(parent, principal))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header[k] = v
	}
	api_handlers.GetGenerateTemplateHandler(tc.AppCtx.WithPrincipal(principal), "group", access)(w, req)
}

var streamHeader = http.Header{"Accept": {"text/event-stream"}}

func framesOf(t *testing.T, body string) []sseFrame {
	t.Helper()
	r := newSSEReader(strings.NewReader(body))
	var frames []sseFrame
	for {
		f, err := r.next()
		if err != nil {
			if err != io.EOF {
				t.Fatalf("parsing the stream: %v", err)
			}
			return frames
		}
		frames = append(frames, f)
	}
}

func lastFrame(t *testing.T, frames []sseFrame) sseFrame {
	t.Helper()
	if len(frames) == 0 {
		t.Fatal("no frames")
	}
	return frames[len(frames)-1]
}

func hashOf(t *testing.T, tc *TestContext, p *auth.Principal, body map[string]any) string {
	t.Helper()
	dry := map[string]any{"dryRun": true}
	for k, v := range body {
		dry[k] = v
	}
	rr := callGenerate(tc, nil, p, dry, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("dry run: %d %s", rr.Code, rr.Body.String())
	}
	var d struct {
		GroundingHash string `json:"groundingHash"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &d)
	return d.GroundingHash
}

// A group-limited account cannot start a run, and is refused before any
// provider call: a synthetic confined editor, and a role that requires a scope
// but has none. Neither reaches the provider on a dry run either.
func TestHarnessRefusesGroupLimitedAccounts(t *testing.T) {
	tc := SetupTestEnv(t)
	conv := &fakeConversation{step: submitOnce(".a{}")}
	installHarness(tc, conv, 5)
	inside := &models.Group{Name: "Inside"}
	if err := tc.DB.Create(inside).Error; err != nil {
		t.Fatal(err)
	}
	principals := map[string]*auth.Principal{
		"confined editor":        {UserID: 7, Username: "confined", Role: models.RoleEditor, ScopeGroupID: &inside.ID},
		"scope required, absent": {UserID: 8, Username: "nobody", Role: models.RoleGuest},
	}
	for name, p := range principals {
		for _, dry := range []bool{true, false} {
			body := withHash(harnessSlotBody(), "any-hash")
			body["dryRun"] = dry
			rr := callGenerate(tc, nil, p, body, nil)
			if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "not available to group-limited accounts") {
				t.Fatalf("%s dryRun=%v: want 403 naming group-limited accounts, got %d %s", name, dry, rr.Code, rr.Body.String())
			}
		}
	}
	if conv.calls.Load() != 0 {
		t.Fatalf("a refused request reached the provider")
	}
	// Mutation check: the same request by an unscoped account is served.
	if rr := callGenerate(tc, nil, auth.SystemPrincipal(), map[string]any{"dryRun": true, "target": "slot", "slot": "CustomCSS", "mode": "css", "prompt": "x"}, nil); rr.Code != http.StatusOK {
		t.Fatalf("an unscoped account should be served: %d %s", rr.Code, rr.Body.String())
	}
}

// Classic mode keeps serving a group-limited account.
func TestClassicModeStillServesGroupLimitedAccounts(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.SetTemplateGenerator(&fakeAPITemplateGenerator{result: slotResult()})
	inside := &models.Group{Name: "Inside"}
	if err := tc.DB.Create(inside).Error; err != nil {
		t.Fatal(err)
	}
	p := &auth.Principal{UserID: 7, Role: models.RoleEditor, ScopeGroupID: &inside.ID}
	rr := callGenerate(tc, nil, p, map[string]any{"dryRun": true, "target": "slot", "slot": "CustomHeader", "prompt": "x"}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("classic dry run by a confined account: %d %s", rr.Code, rr.Body.String())
	}
}

// Access revoked after the generator's final check and before the terminal
// write: neither the Draft nor the transcript is delivered, in either mode, and
// no progress frame carries content a Tool call held.
func TestHarnessTerminalReleaseRechecksAccess(t *testing.T) {
	const secret = "SECRET-SENTINEL-4471"
	script := func(_ context.Context, n int, _ application_context.ConversationRequest) (application_context.ConversationResponse, error) {
		if n == 1 {
			return toolReply(
				apiToolCall("l1", application_context.ToolLint, map[string]any{"field": "CustomCSS", "content": ".x{content:'" + secret + "'}"}),
				apiToolCall("m1", application_context.ToolCheckMRQL, map[string]any{"query": "type = resource AND name ~ \"" + secret + "\""}),
			), nil
		}
		return toolReply(apiSubmit("s1", map[string]string{"CustomCSS": ".draft{color:red}"}, true)), nil
	}
	for _, mode := range []string{"sse", "json"} {
		t.Run(mode, func(t *testing.T) {
			tc := SetupTestEnv(t)
			access := &flipAccess{tc: tc}
			obs := installObserved(tc, &fakeConversation{step: script}, 5)
			obs.afterRun = func() { access.revoked.Store(true) }
			principal := auth.SystemPrincipal()
			hash := hashOf(t, tc, principal, harnessSlotBody())
			var header http.Header
			if mode == "sse" {
				header = streamHeader
			}
			rr := callGenerate(tc, access.factory(), principal, withHash(harnessSlotBody(), hash), header)
			body := rr.Body.String()
			for _, leak := range []string{".draft{color:red}", secret, `"transcript"`, `"content"`} {
				if strings.Contains(body, leak) {
					t.Fatalf("%s: the response leaked %q after access was revoked:\n%s", mode, leak, body)
				}
			}
			if mode == "json" {
				if rr.Code != http.StatusForbidden {
					t.Fatalf("want 403, got %d %s", rr.Code, body)
				}
				return
			}
			frames := framesOf(t, body)
			if last := lastFrame(t, frames); last.Event != "error" || !strings.Contains(last.Data, `"status":403`) {
				t.Fatalf("want a 403 error event last, got %q %s", last.Event, last.Data)
			}
			for _, f := range frames {
				if f.Event == "result" {
					t.Fatalf("a result event was delivered after revocation")
				}
			}
			// The mutation check: unrevoked, the same script does deliver the Draft.
			access.revoked.Store(false)
			obs.afterRun = nil
			ok := callGenerate(tc, access.factory(), principal, withHash(harnessSlotBody(), hash), header)
			if !strings.Contains(ok.Body.String(), ".draft{color:red}") {
				t.Fatalf("mutation check: without revocation the Draft should be delivered: %s", ok.Body.String())
			}
		})
	}
}

// Progress frames carry no free text: a sentinel in a Tool call's content or
// query reaches only the run's final transcript, never an event before it.
func TestHarnessProgressFramesCarryNoToolContent(t *testing.T) {
	const secret = "PROGRESS-SENTINEL-9902"
	tc := SetupTestEnv(t)
	installHarness(tc, &fakeConversation{step: func(_ context.Context, n int, _ application_context.ConversationRequest) (application_context.ConversationResponse, error) {
		if n == 1 {
			return toolReply(
				apiToolCall("l1", application_context.ToolLint, map[string]any{"field": "CustomCSS", "content": ".x{content:'" + secret + "'}"}),
				apiToolCall("m1", application_context.ToolCheckMRQL, map[string]any{"query": secret}),
				apiToolCall("p1", application_context.ToolGetPartial, map[string]any{"name": secret}),
				apiToolCall("z1", "no_such_tool_"+secret, map[string]any{"field": secret}),
			), nil
		}
		return toolReply(apiSubmit("s1", map[string]string{"CustomCSS": ".draft{}"}, true)), nil
	}}, 5)
	hash := hashOf(t, tc, auth.SystemPrincipal(), harnessSlotBody())
	rr := callGenerate(tc, nil, auth.SystemPrincipal(), withHash(harnessSlotBody(), hash), streamHeader)
	frames := framesOf(t, rr.Body.String())
	names := eventNames(frames)
	for _, want := range []string{"run", "turn", "tool", "submission", "usage", "result"} {
		found := false
		for _, n := range names {
			found = found || n == want
		}
		if !found {
			t.Fatalf("event %q missing from %v", want, names)
		}
	}
	for _, f := range frames {
		if f.Event != "result" && strings.Contains(f.Raw, secret) {
			t.Fatalf("progress frame %q carries the sentinel: %s", f.Event, f.Raw)
		}
	}
	if !strings.Contains(lastFrame(t, frames).Data, secret) {
		t.Fatalf("mutation check: the transcript in the result should hold the call as sent")
	}
}

// A provider failure after Tool Turns and before any submission delivers the
// transcript so far in the error event, with a fixed message.
func TestHarnessFailureDeliversTheTranscript(t *testing.T) {
	for _, mode := range []string{"sse", "json"} {
		t.Run(mode, func(t *testing.T) {
			tc := SetupTestEnv(t)
			installHarness(tc, &fakeConversation{step: func(_ context.Context, n int, _ application_context.ConversationRequest) (application_context.ConversationResponse, error) {
				if n == 1 {
					return toolReply(apiToolCall("g1", application_context.ToolGetEntity, map[string]any{"type": "group", "id": 424242})), nil
				}
				return application_context.ConversationResponse{}, errors.New("upstream-secret-detail")
			}}, 5)
			hash := hashOf(t, tc, auth.SystemPrincipal(), harnessSlotBody())
			var header http.Header
			if mode == "sse" {
				header = streamHeader
			}
			rr := callGenerate(tc, nil, auth.SystemPrincipal(), withHash(harnessSlotBody(), hash), header)
			var payload struct {
				Error      string `json:"error"`
				Status     int    `json:"status"`
				StopReason string `json:"stopReason"`
				Transcript []application_context.TemplateTranscriptRecord
			}
			if mode == "sse" {
				last := lastFrame(t, framesOf(t, rr.Body.String()))
				if last.Event != "error" {
					t.Fatalf("want an error event, got %q %s", last.Event, last.Data)
				}
				_ = json.Unmarshal([]byte(last.Data), &payload)
			} else {
				if rr.Code != http.StatusBadGateway {
					t.Fatalf("want 502, got %d", rr.Code)
				}
				_ = json.Unmarshal(rr.Body.Bytes(), &payload)
				payload.Status = rr.Code
			}
			if payload.Status != http.StatusBadGateway || payload.Error != "template generation provider error" || payload.StopReason != "provider_error" {
				t.Fatalf("unexpected error payload: %+v", payload)
			}
			if len(payload.Transcript) < 3 {
				t.Fatalf("the transcript so far should list the Seed, the Tool call and its result: %+v", payload.Transcript)
			}
			if strings.Contains(rr.Body.String(), "upstream-secret-detail") {
				t.Fatalf("the provider's own error text reached the client")
			}
		})
	}
}

// Errors found before the first Turn keep their status codes even when the
// client asked for a stream.
func TestHarnessErrorsBeforeTheStreamKeepTheirStatusCodes(t *testing.T) {
	tc := SetupTestEnv(t)
	installHarness(tc, &fakeConversation{step: submitOnce(".a{}")}, 5)
	p := auth.SystemPrincipal()
	if rr := callGenerate(tc, nil, p, withHash(harnessSlotBody(), "stale-hash"), streamHeader); rr.Code != http.StatusConflict {
		t.Fatalf("a stale hash: want 409, got %d %s", rr.Code, rr.Body.String())
	}
	if rr := callGenerate(tc, nil, p, harnessSlotBody(), streamHeader); rr.Code != http.StatusBadRequest {
		t.Fatalf("a missing hash: want 400, got %d %s", rr.Code, rr.Body.String())
	}
	bad := harnessSlotBody()
	bad["slot"] = "NotASlot"
	if rr := callGenerate(tc, nil, p, bad, streamHeader); rr.Code != http.StatusBadRequest {
		t.Fatalf("an unknown slot: want 400, got %d", rr.Code)
	}
}

// The two opt-ins are request fields, off by default, and part of the hash.
func TestHarnessOptInsAreInTheHash(t *testing.T) {
	tc := SetupTestEnv(t)
	installHarness(tc, &fakeConversation{step: submitOnce(".a{}")}, 5)
	p := auth.SystemPrincipal()
	base := hashOf(t, tc, p, harnessSlotBody())
	if again := hashOf(t, tc, p, harnessSlotBody()); again != base {
		t.Fatalf("two dry runs with unchanged data differ")
	}
	for _, opt := range []string{"previewMRQL", "previewPlugins"} {
		body := harnessSlotBody()
		body[opt] = true
		if h := hashOf(t, tc, p, body); h == base {
			t.Fatalf("%s did not change the hash", opt)
		}
		// A run without the opt-in the hash was approved for is refused.
		if rr := callGenerate(tc, nil, p, withHash(harnessSlotBody(), hashOf(t, tc, p, body)), nil); rr.Code != http.StatusConflict {
			t.Fatalf("%s: a hash approved with the opt-in must not run without it: %d", opt, rr.Code)
		}
	}
	// The Disclosure itself reports the opt-ins.
	body := harnessSlotBody()
	body["previewMRQL"] = true
	body["dryRun"] = true
	rr := callGenerate(tc, nil, p, body, nil)
	if !strings.Contains(rr.Body.String(), `"previewMRQL":true`) {
		t.Fatalf("the Disclosure does not show the MRQL opt-in: %s", rr.Body.String())
	}
}

// blockQueries installs a callback that, while armed, parks every query on a
// table until its context ends, and records that it saw the context end.
type queryBlocker struct {
	armed     atomic.Bool
	table     string
	entered   chan struct{}
	cancelled atomic.Bool
}

func blockQueriesOn(t *testing.T, tc *TestContext, table string) *queryBlocker {
	t.Helper()
	b := &queryBlocker{table: table, entered: make(chan struct{}, 8)}
	name := "block-" + table + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	err := tc.DB.Callback().Query().Before("gorm:query").Register(name, func(db *gorm.DB) {
		if !b.armed.Load() || db.Statement.Table != b.table {
			return
		}
		select {
		case b.entered <- struct{}{}:
		default:
		}
		<-db.Statement.Context.Done()
		b.cancelled.Store(true)
		_ = db.AddError(db.Statement.Context.Err())
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tc.DB.Callback().Query().Remove(name) })
	return b
}

func seedCategoryWithGroups(t *testing.T, tc *TestContext, n int) (*models.Category, []*models.Group) {
	t.Helper()
	cat := &models.Category{Name: "Projects"}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	var groups []*models.Group
	for i := 0; i < n; i++ {
		g := &models.Group{Name: "Project " + strconv.Itoa(i+1), CategoryId: &cat.ID, Meta: types.JSON(`{"marker":"sample-marker-` + strconv.Itoa(i+1) + `"}`)}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatal(err)
		}
		groups = append(groups, g)
	}
	return cat, groups
}

// A client that leaves while a sample or the partial catalogue is being read
// ends the handler without a provider call.
func TestHarnessGroundingCancelCallsNoProvider(t *testing.T) {
	for _, table := range []string{"groups", "template_partials"} {
		t.Run(table, func(t *testing.T) {
			tc := SetupTestEnv(t)
			cat, _ := seedCategoryWithGroups(t, tc, 2)
			conv := &fakeConversation{step: submitOnce(".a{}")}
			installHarness(tc, conv, 5)
			body := harnessSlotBody()
			body["categoryId"] = cat.ID
			hash := hashOf(t, tc, auth.SystemPrincipal(), body)

			blocker := blockQueriesOn(t, tc, table)
			blocker.armed.Store(true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rr := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				callGenerateTo(tc, rr, ctx, nil, auth.SystemPrincipal(), withHash(body, hash), streamHeader)
			}()
			select {
			case <-blocker.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("the grounding read never started")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the handler did not return after the client left")
			}
			if conv.calls.Load() != 0 {
				t.Fatalf("a provider was called after the client left during grounding")
			}
			if !blocker.cancelled.Load() {
				t.Fatalf("the blocked read was not cancelled by the request context")
			}
		})
	}
}

// The mode is read once. Flipping it while a grounding read is paused changes
// neither the refusal, the Disclosure nor the run.
func TestHarnessModeIsOneSnapshotPerRequest(t *testing.T) {
	tc := SetupTestEnv(t)
	cat, _ := seedCategoryWithGroups(t, tc, 1)
	obs := installObserved(tc, &fakeConversation{step: submitOnce(".a{}")}, 5)
	var flipped atomic.Bool
	obs.modeToggle = func() string {
		if flipped.Load() {
			return application_context.TemplateGenerationModeClassic
		}
		return application_context.TemplateGenerationModeHarness
	}
	// The setting changes while the partial catalogue is being read.
	name := "flip-mode"
	_ = tc.DB.Callback().Query().Before("gorm:query").Register(name, func(db *gorm.DB) {
		if db.Statement.Table == "template_partials" {
			flipped.Store(true)
		}
	})
	t.Cleanup(func() { _ = tc.DB.Callback().Query().Remove(name) })

	body := harnessSlotBody()
	body["categoryId"] = cat.ID
	body["dryRun"] = true
	rr := callGenerate(tc, nil, auth.SystemPrincipal(), body, nil)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"mode":"harness"`) {
		t.Fatalf("the Disclosure should be the harness one from the first snapshot: %d %s", rr.Code, rr.Body.String())
	}
	if !flipped.Load() {
		t.Fatal("the mode flip never happened during grounding")
	}
	if n := obs.prepareCount(); n != 1 {
		t.Fatalf("the handler prepared %d times, want one snapshot", n)
	}
}

var sampleRefPattern = regexp.MustCompile(`Sample: \{"type":"group","id":(\d+),`)

// Automatically chosen samples are reachable by get_entity through the IDs in
// the Seed, though the request names none.
func TestHarnessSeedStartingPointsOpenWithGetEntity(t *testing.T) {
	tc := SetupTestEnv(t)
	cat, groups := seedCategoryWithGroups(t, tc, 2)
	var mu sync.Mutex
	var openedIDs []string
	var resultText string
	conv := &fakeConversation{step: func(_ context.Context, n int, req application_context.ConversationRequest) (application_context.ConversationResponse, error) {
		if n == 1 {
			var calls []application_context.ToolCall
			for i, m := range sampleRefPattern.FindAllStringSubmatch(req.Messages[0].Text, -1) {
				id, _ := strconv.Atoi(m[1])
				mu.Lock()
				openedIDs = append(openedIDs, m[1])
				mu.Unlock()
				calls = append(calls, apiToolCall("g"+strconv.Itoa(i), application_context.ToolGetEntity, map[string]any{"type": "group", "id": id}))
			}
			return toolReply(calls...), nil
		}
		var b strings.Builder
		for _, m := range req.Messages {
			for _, r := range m.Results {
				b.WriteString(r.Text)
			}
		}
		mu.Lock()
		resultText = b.String()
		mu.Unlock()
		return toolReply(apiSubmit("s1", map[string]string{"CustomCSS": ".a{}"}, true)), nil
	}}
	installHarness(tc, conv, 5)
	body := harnessSlotBody()
	body["categoryId"] = cat.ID
	hash := hashOf(t, tc, auth.SystemPrincipal(), body)
	rr := callGenerate(tc, nil, auth.SystemPrincipal(), withHash(body, hash), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(openedIDs) != len(groups) {
		t.Fatalf("the Seed named %v, want both samples", openedIDs)
	}
	for _, marker := range []string{"sample-marker-1", "sample-marker-2"} {
		if !strings.Contains(resultText, marker) {
			t.Fatalf("get_entity by the Seed's IDs did not return %s: %s", marker, resultText)
		}
	}
}

// A client that leaves while a Tool's read is blocked cancels that read.
func TestHarnessDisconnectCancelsABlockedToolRead(t *testing.T) {
	tc := SetupTestEnv(t)
	group := tc.CreateDummyGroup("Target")
	blocker := blockQueriesOn(t, tc, "groups")
	conv := &fakeConversation{step: func(_ context.Context, n int, _ application_context.ConversationRequest) (application_context.ConversationResponse, error) {
		blocker.armed.Store(true)
		return toolReply(apiToolCall("g1", application_context.ToolGetEntity, map[string]any{"type": "group", "id": group.ID})), nil
	}}
	installHarness(tc, conv, 5)
	srv := newHarnessServer(t, tc)
	hash := disclose(t, tc, categoryGenerate, harnessSlotBody())

	resp, cancel := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), sameClient)
	r := newSSEReader(resp.Body)
	runID := runIDOf(t, r.nextWithin(t, 5*time.Second))
	select {
	case <-blocker.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the Tool never started its read")
	}
	cancel()
	waitFor(t, 5*time.Second, "the blocked Tool read to be cancelled", blocker.cancelled.Load)
	waitFor(t, 5*time.Second, "the registry entry to be freed", func() bool {
		return postFinish(tc, "/v1/category/generateTemplate/finish", runID, sameClient).Code == http.StatusNotFound
	})
	if conv.calls.Load() != 1 {
		t.Fatalf("the provider was called again after the client left: %d", conv.calls.Load())
	}
}
