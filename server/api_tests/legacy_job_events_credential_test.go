package api_tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
)

// The legacy job event streams outlive the request that authenticated them, so
// they have to ask again, as the canonical stream does: a session that was
// logged out, an account that was disabled, and an administrator who was
// demoted must stop receiving other people's work from a stream opened before
// the change. These pin that on both legacy paths and for both feeds the
// stream forwards (queue entries and plugin actions).

var legacyJobEventPaths = []string{"/v1/download/events", "/v1/jobs/events"}

// legacyStreamRevalidationWait bounds how long a revoked stream may stay open.
// The handler rechecks the credential every second while idle.
const legacyStreamRevalidationWait = 5 * time.Second

type legacyJobStream struct {
	writer   *canonicalSSEWriter
	cancel   context.CancelFunc
	finished chan struct{}
}

func openLegacyJobStream(t *testing.T, tc *TestContext, path string, authenticate func(*http.Request)) *legacyJobStream {
	t.Helper()
	streamCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(streamCtx)
	authenticate(request)
	stream := &legacyJobStream{writer: newCanonicalSSEWriter(), cancel: cancel, finished: make(chan struct{})}
	go func() {
		tc.Router.ServeHTTP(stream.writer, request)
		close(stream.finished)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stream.finished:
		case <-time.After(5 * time.Second):
			t.Errorf("%s did not stop after the client disconnected", path)
		}
	})
	if !stream.writer.waitForText("event: init\n", 3*time.Second) {
		t.Fatalf("%s did not send its initial state: %q", path, stream.writer.body())
	}
	return stream
}

func (s *legacyJobStream) closedWithin(timeout time.Duration) bool {
	select {
	case <-s.finished:
		return true
	case <-time.After(timeout):
		return false
	}
}

func withCookie(cookie *http.Cookie) func(*http.Request) {
	return func(request *http.Request) { request.AddCookie(cookie) }
}

func withBearer(bearer string) func(*http.Request) {
	return func(request *http.Request) { request.Header.Set("Authorization", bearer) }
}

func createStreamAdmin(t *testing.T, tc *TestContext, username string) *models.User {
	t.Helper()
	admin, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: username, Password: "password1", Role: models.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create streaming administrator: %v", err)
	}
	return admin
}

func TestLegacyJobEventStreamsEndWhenTheCredentialStopsAuthenticating(t *testing.T) {
	revocations := map[string]func(t *testing.T, tc *TestContext, admin *models.User, cookie *http.Cookie){
		"logout": func(t *testing.T, tc *TestContext, _ *models.User, cookie *http.Cookie) {
			response := doReq(tc, http.MethodPost, "/v1/auth/logout",
				map[string]string{"Accept": "application/json"}, []*http.Cookie{cookie}, nil)
			if response.Code >= 300 {
				t.Fatalf("logout answered %d: %s", response.Code, response.Body.String())
			}
		},
		"disable": func(t *testing.T, tc *TestContext, admin *models.User, _ *http.Cookie) {
			if _, err := tc.AppCtx.UpdateUser(admin.ID, &application_context.UserUpdate{
				Disabled: application_context.UserField[bool]{Set: true, Value: true},
			}); err != nil {
				t.Fatalf("disable the streaming administrator: %v", err)
			}
		},
	}
	for _, path := range legacyJobEventPaths {
		for name, revoke := range revocations {
			t.Run(strings.TrimPrefix(path, "/v1/")+"/"+name, func(t *testing.T) {
				tc := setupAuthEnv(t)
				t.Cleanup(tc.AppCtx.DownloadManager().Shutdown)
				admin := createStreamAdmin(t, tc, "stream-admin")
				cookie, _ := loginSummaryExportSession(t, tc, admin.Username, "password1")
				_, otherID := plainUserBearer(t, tc, "stream-other")

				stream := openLegacyJobStream(t, tc, path, withCookie(cookie))
				before := submitSSEQueueJob(t, tc, otherID)
				if !stream.writer.waitForText(before, 3*time.Second) {
					t.Fatalf("an administrator's stream did not receive another user's job %q before revocation: %s",
						before, stream.writer.body())
				}

				revoke(t, tc, admin, cookie)
				after := submitSSEQueueJob(t, tc, otherID)

				if !stream.closedWithin(legacyStreamRevalidationWait) {
					t.Fatalf("%s stayed open after the credential stopped authenticating", path)
				}
				if strings.Contains(stream.writer.body(), after) {
					t.Fatalf("%s delivered job %q after the credential stopped authenticating: %s",
						path, after, stream.writer.body())
				}
			})
		}
	}
}

// An idle stream has no frame to check its credential for, so it rechecks on a
// timer rather than waiting for the next event to find out.
func TestAnIdleLegacyJobEventStreamClosesAfterLogout(t *testing.T) {
	for _, path := range legacyJobEventPaths {
		t.Run(strings.TrimPrefix(path, "/v1/"), func(t *testing.T) {
			tc := setupAuthEnv(t)
			t.Cleanup(tc.AppCtx.DownloadManager().Shutdown)
			admin := createStreamAdmin(t, tc, "stream-idle")
			cookie, _ := loginSummaryExportSession(t, tc, admin.Username, "password1")
			stream := openLegacyJobStream(t, tc, path, withCookie(cookie))

			response := doReq(tc, http.MethodPost, "/v1/auth/logout",
				map[string]string{"Accept": "application/json"}, []*http.Cookie{cookie}, nil)
			if response.Code >= 300 {
				t.Fatalf("logout answered %d: %s", response.Code, response.Body.String())
			}
			if !stream.closedWithin(legacyStreamRevalidationWait) {
				t.Fatalf("an idle %s stayed open after logout", path)
			}
		})
	}
}

func TestLegacyJobEventStreamsFilterByTheAccountAsItIsNow(t *testing.T) {
	for _, path := range legacyJobEventPaths {
		t.Run(strings.TrimPrefix(path, "/v1/"), func(t *testing.T) {
			tc := setupAuthEnv(t)
			t.Cleanup(tc.AppCtx.DownloadManager().Shutdown)
			scope := &models.Group{Name: "stream-demotion-scope"}
			if err := tc.DB.Create(scope).Error; err != nil {
				t.Fatalf("create scope group: %v", err)
			}
			admin := createStreamAdmin(t, tc, "stream-demoted")
			cookie, _ := loginSummaryExportSession(t, tc, admin.Username, "password1")
			_, otherID := plainUserBearer(t, tc, "stream-demotion-other")

			stream := openLegacyJobStream(t, tc, path, withCookie(cookie))
			before := submitSSEQueueJob(t, tc, otherID)
			if !stream.writer.waitForText(before, 3*time.Second) {
				t.Fatalf("an administrator's stream did not receive another user's job %q: %s", before, stream.writer.body())
			}

			if _, err := tc.AppCtx.UpdateUser(admin.ID, &application_context.UserUpdate{
				Role:         application_context.UserField[models.Role]{Set: true, Value: models.RoleGuest},
				ScopeGroupID: application_context.UserField[*uint]{Set: true, Value: &scope.ID},
			}); err != nil {
				t.Fatalf("demote the streaming administrator: %v", err)
			}

			hidden := submitSSEQueueJob(t, tc, otherID)
			own := submitSSEQueueJob(t, tc, admin.ID)
			// The account still authenticates, so the stream stays open and keeps
			// delivering what the account may see now. Its own job is the proof the
			// stream is alive; queue events arrive in order, so the other user's job,
			// submitted first, would already be in the body if it had leaked.
			if !stream.writer.waitForText(own, 3*time.Second) {
				t.Fatalf("the demoted account's stream stopped delivering its own job %q: %s", own, stream.writer.body())
			}
			if strings.Contains(stream.writer.body(), hidden) {
				t.Fatalf("%s delivered another user's job %q to a demoted account: %s", path, hidden, stream.writer.body())
			}
		})
	}
}

func TestLegacyActionEventsStopForARevokedAdministrator(t *testing.T) {
	tc, owner, _, _ := setupRetryableActionProjectionEnv(t)
	// A second administrator, so the fixture's own stays the last enabled one.
	admin := createStreamAdmin(t, tc, "action-stream-admin")
	adminToken, _, err := tc.AppCtx.CreateApiToken(admin.ID, "stream admin", nil)
	if err != nil {
		t.Fatalf("create administrator token: %v", err)
	}
	stream := openLegacyJobStream(t, tc, "/v1/jobs/events", withBearer("Bearer "+adminToken))

	visible, _ := runResultActionToCompletion(t, tc, owner, "seen-before-revocation")
	if !stream.writer.waitForText(`"id":"`+visible+`"`, 4*time.Second) {
		t.Fatalf("an administrator's stream did not receive another user's action %q: %s", visible, stream.writer.body())
	}

	if _, err := tc.AppCtx.UpdateUser(admin.ID, &application_context.UserUpdate{
		Disabled: application_context.UserField[bool]{Set: true, Value: true},
	}); err != nil {
		t.Fatalf("disable the streaming administrator: %v", err)
	}
	hidden, _ := runResultActionToCompletion(t, tc, owner, "after-revocation")

	if !stream.closedWithin(legacyStreamRevalidationWait) {
		t.Fatal("the legacy action stream stayed open after its account was disabled")
	}
	if strings.Contains(stream.writer.body(), hidden) || strings.Contains(stream.writer.body(), "after-revocation") {
		t.Fatalf("the legacy action stream delivered action %q to a disabled account: %s", hidden, stream.writer.body())
	}
}

func TestLegacyActionEventsFilterForADemotedAdministrator(t *testing.T) {
	tc, owner, _, _ := setupRetryableActionProjectionEnv(t)
	admin := createStreamAdmin(t, tc, "action-stream-demoted")
	adminToken, _, err := tc.AppCtx.CreateApiToken(admin.ID, "stream admin", nil)
	if err != nil {
		t.Fatalf("create administrator token: %v", err)
	}
	stream := openLegacyJobStream(t, tc, "/v1/jobs/events", withBearer("Bearer "+adminToken))

	visible, _ := runResultActionToCompletion(t, tc, owner, "seen-while-admin")
	if !stream.writer.waitForText(`"id":"`+visible+`"`, 4*time.Second) {
		t.Fatalf("an administrator's stream did not receive another user's action %q: %s", visible, stream.writer.body())
	}

	demoted, err := tc.AppCtx.UpdateUser(admin.ID, &application_context.UserUpdate{
		Role: application_context.UserField[models.Role]{Set: true, Value: models.RoleEditor},
	})
	if err != nil {
		t.Fatalf("demote the streaming administrator: %v", err)
	}
	hidden, _ := runResultActionToCompletion(t, tc, owner, "hidden-after-demotion")
	// Longer than the durable projection poll, so both the live feed and the poll
	// have had their chance to forward the other user's action.
	time.Sleep(3 * time.Second)
	demotedCtx := tc.AppCtx.WithPrincipal(auth.FromUser(demoted))
	own, _, err := demotedCtx.RunPluginActionAsync(&demoted.ID, "retry-projection", "result", 1,
		map[string]any{"secret": "own-after-demotion"}, "")
	if err != nil {
		t.Fatalf("run the demoted account's own action: %v", err)
	}
	if !stream.writer.waitForText(`"id":"`+own+`"`, 4*time.Second) {
		t.Fatalf("the demoted account's stream stopped delivering its own action %q: %s", own, stream.writer.body())
	}
	if strings.Contains(stream.writer.body(), hidden) || strings.Contains(stream.writer.body(), "hidden-after-demotion") {
		t.Fatalf("the legacy action stream delivered another user's action %q to a demoted account: %s", hidden, stream.writer.body())
	}
}
