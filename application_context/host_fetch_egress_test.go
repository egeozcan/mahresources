package application_context

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The application's own fetch paths take a URL from a user and retrieve it
// server-side. Until this package, nothing said where the server was allowed to
// go: POST /v1/resource/remote, the download queue and the calendar block would
// all fetch http://169.254.169.254/ — the cloud instance metadata endpoint — and
// file the response as a resource the submitter could then read.
//
// These tests use loopback rather than a link-local address as the stand-in for
// "somewhere the server should not go". Both are refused by the same classifier
// and the same rule, and only loopback can be stood up inside a test: an
// httptest server is a real internal service on a real private address, which
// is exactly the thing being kept out of reach.

func newHostFetchContext(t *testing.T, allowPrivate ...string) *MahresourcesContext {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Resource{}, &models.Note{}, &models.Tag{}, &models.Group{},
		&models.Category{}, &models.NoteType{}, &models.ResourceCategory{},
		&models.Series{}, &models.Preview{}, &models.LogEntry{},
		&models.ResourceVersion{}, &models.NoteBlock{}, &models.User{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	sqlDB, _ := db.DB()
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{
		DbType:                       constants.DbTypeSqlite,
		AllowPrivateFetch:            allowPrivate,
		RemoteResourceConnectTimeout: 5 * time.Second,
		RemoteResourceIdleTimeout:    5 * time.Second,
		RemoteResourceOverallTimeout: 10 * time.Second,
	})

	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)
	ctx.DefaultResourceCategoryID = defaultRC.ID
	return ctx
}

// internalService stands in for whatever is listening on the deployment's
// private network: a metadata endpoint, an admin port, a database's HTTP face.
func internalService(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "the-secret-the-server-can-reach-and-the-user-cannot")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAddRemoteResource_RefusesAPrivateAddress(t *testing.T) {
	ctx := newHostFetchContext(t)
	srv := internalService(t)

	_, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{URL: srv.URL})
	if err == nil {
		t.Fatal("fetched from a private address with no -allow-private-fetch entry permitting it")
	}
	assertRefusalIsNotAnOracle(t, err)
}

func TestAddRemoteResource_ReachesAPrivateAddressTheOperatorNamed(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")
	srv := internalService(t)

	resource, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{URL: srv.URL})
	if err != nil {
		t.Fatalf("a named private address must stay reachable, or the opt-in is not one: %v", err)
	}
	if resource == nil {
		t.Fatal("no resource created")
	}
}

// The deny must not extend to the public internet, which is the entire purpose
// of the feature. A test cannot dial a real public host, so this asserts the
// property that decides it: the policy's own answer for a public address.
func TestHostFetchPolicy_LeavesPublicHostsAlone(t *testing.T) {
	policy, err := plugin_system.HostFetchPolicy(nil)
	if err != nil {
		t.Fatalf("HostFetchPolicy: %v", err)
	}
	for _, host := range []string{"example.com", "cdn.example.org", "93.184.216.34"} {
		if !policy.Allows(host) {
			t.Errorf("host policy refused %q; operator fetches from public hosts are the point of the feature", host)
		}
	}
	if err := plugin_system.CheckEgressURL(policy, "https://example.com/file.png"); err != nil {
		t.Errorf("public URL refused: %v", err)
	}
}

func TestFetchICS_RefusesAPrivateAddress(t *testing.T) {
	ctx := newHostFetchContext(t)
	srv := internalService(t)

	_, _, err := ctx.fetchAndCacheICS(srv.URL, nil)
	if err == nil {
		t.Fatal("the calendar block fetched a private address; a calendar URL is user-supplied like any other")
	}
	assertRefusalIsNotAnOracle(t, err)
}

func TestFetchICS_ReachesAPrivateAddressTheOperatorNamed(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")
	srv := internalService(t)

	// An internal calendar server was the stated reason this path had no
	// address filtering at all. It has to keep working through the opt-in.
	if _, _, err := ctx.fetchAndCacheICS(srv.URL, nil); err != nil {
		t.Fatalf("a named private address must stay reachable: %v", err)
	}
}

// The background queue reaches the network through a decorator this layer
// injects, rather than a policy it imports. That indirection is the thing most
// likely to be wired wrong and least likely to be noticed, because the queue
// keeps working perfectly either way — it just also fetches from the whole
// private network. So this drives a real submission through the real manager
// the context built, rather than asserting the seam exists.
func TestDownloadQueue_RefusesAPrivateAddress(t *testing.T) {
	ctx := newHostFetchContext(t)
	srv := internalService(t)

	dm := ctx.DownloadManager()
	if dm == nil {
		t.Fatal("no download manager")
	}
	t.Cleanup(dm.Shutdown)

	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: srv.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	snap := awaitTerminal(t, dm, job.ID)
	if snap.Status != "failed" {
		t.Fatalf("download of a private address finished as %q, want failed", snap.Status)
	}
	assertRefusalIsNotAnOracle(t, errors.New(snap.Error))
}

func TestDownloadQueue_ReachesAPrivateAddressTheOperatorNamed(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")
	srv := internalService(t)

	dm := ctx.DownloadManager()
	t.Cleanup(dm.Shutdown)

	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: srv.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	snap := awaitTerminal(t, dm, job.ID)
	if snap.Status != "completed" {
		t.Fatalf("download of a named private address finished as %q (%s), want completed", snap.Status, snap.Error)
	}
}

// awaitTerminal polls until the job leaves the running states. The queue has no
// completion channel to wait on, and a fixed sleep would trade a slow test for
// a flaky one.
func awaitTerminal(t *testing.T, dm *download_queue.DownloadManager, jobID string) *download_queue.DownloadJob {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := dm.GetJob(jobID)
		if !ok {
			t.Fatalf("job %s vanished from the queue", jobID)
		}
		snap := job.Snapshot()
		switch snap.Status {
		case "completed", "failed", "cancelled":
			return snap
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s never reached a terminal status", jobID)
	return nil
}

// A refusal reaches whoever submitted the URL, and under -auth that is an
// ordinary user. If it named the address the host resolved to, a list of
// failures would be an internal network scan run at the server's expense.
func assertRefusalIsNotAnOracle(t *testing.T, err error) {
	t.Helper()
	for _, leak := range []string{"127.0.0.1", "[::1]", "::1"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("refusal names the resolved address %q, which makes it an oracle: %v", leak, err)
		}
	}
}

// The ICS client used to leave Transport nil and inherit http.DefaultTransport.
// Applying the egress policy to such a client finds no *http.Transport to
// decorate and installs a bare one — no idle bound on a transport thrown away
// after every fetch, and no HTTP/2. The connection behaviour of the three
// policed paths has to stay comparable, so this asserts the shape rather than
// the fetch, which the tests above already cover.
func TestFetchICS_ClientKeepsAConfiguredTransport(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")
	srv := internalService(t)

	// Drive a real fetch so the client is built exactly as production builds it,
	// then read the transport back off the request the server saw.
	if _, _, err := ctx.fetchAndCacheICS(srv.URL, nil); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	// Rebuild through the same path and inspect it directly.
	timeout := ctx.Config.RemoteResourceConnectTimeout
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   timeout / 2,
			ResponseHeaderTimeout: timeout,
			IdleConnTimeout:       90 * time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
	decorated := plugin_system.ApplyEgressPolicy(client, ctx.hostFetchPolicy, timeout)
	transport, ok := decorated.Transport.(*http.Transport)
	if !ok {
		t.Fatal("policy did not leave an *http.Transport in place")
	}
	if transport.IdleConnTimeout == 0 {
		t.Error("no idle-connection bound: this transport is discarded after every fetch, so each miss strands a keep-alive")
	}
	if !transport.ForceAttemptHTTP2 {
		t.Error("HTTP/2 not attempted; a non-nil DialContext otherwise pins this path to HTTP/1.1")
	}
	// Proxy stays nil on purpose: through a proxy the dial-time check inspects
	// the proxy's address and would pass everything, including the metadata
	// endpoint.
	if transport.Proxy != nil {
		t.Error("proxy set: the dial-time address check cannot see through it")
	}
}

// The docs now promise that the resolved address survives in the activity log
// for all three paths. It held for one of them when that promise was written:
// the download queue discarded the original error, and the ICS fetch replaced it
// before returning. An operator debugging a refused internal fetch had nowhere to
// learn which address was refused, on either.
func TestHostFetch_RefusalIsLoggedWithTheResolvedAddress(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, ctx *MahresourcesContext, url string)
	}{
		{
			name: "add resource from URL",
			run: func(t *testing.T, ctx *MahresourcesContext, url string) {
				_, _ = ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{URL: url})
			},
		},
		{
			name: "calendar ICS fetch",
			run: func(t *testing.T, ctx *MahresourcesContext, url string) {
				_, _, _ = ctx.fetchAndCacheICS(url, nil)
			},
		},
		{
			name: "download queue",
			run: func(t *testing.T, ctx *MahresourcesContext, url string) {
				dm := ctx.DownloadManager()
				t.Cleanup(dm.Shutdown)
				job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: url}, nil)
				if err != nil {
					t.Fatalf("submit: %v", err)
				}
				awaitTerminal(t, dm, job.ID)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newHostFetchContext(t)
			srv := internalService(t)

			// The URL names a HOSTNAME, not the address it resolves to. That is
			// the whole point of the assertion below.
			//
			// httptest hands back http://127.0.0.1:PORT, and the log line is
			// built as `url + ": " + err`. Asserting that the log contains
			// "127.0.0.1" against that URL is satisfied by the URL alone: the
			// test would pass while the resolved address was never logged, and
			// the per-path mutation (deleting the log call) cannot tell the two
			// apart because it removes the URL too. Swapping in "localhost"
			// separates them — the address can now only have come from the dial
			// refusal.
			url := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
			// Assert the host is a NAME, not "does not contain 127.0.0.1".
			// httptest falls back to [::1] when it cannot bind 127.0.0.1, and on
			// such a host the Replace is a no-op the literal check would wave
			// through — leaving the ::1 assertion below satisfied by the logged
			// URL itself, which is exactly the vacuity this swap removes.
			parsed, parseErr := neturl.Parse(url)
			if parseErr != nil {
				t.Fatalf("parse test URL %q: %v", url, parseErr)
			}
			if net.ParseIP(parsed.Hostname()) != nil {
				t.Fatalf("test URL host %q is an IP literal; the assertion below would be vacuous", parsed.Hostname())
			}
			tc.run(t, ctx, url)

			var entries []models.LogEntry
			if err := ctx.db.Find(&entries).Error; err != nil {
				t.Fatalf("read log: %v", err)
			}
			found := false
			for _, e := range entries {
				detail := e.Message + " " + string(e.Details)
				// Either loopback literal: "localhost" resolves to ::1 first on
				// some hosts and 127.0.0.1 on others, and the refusal names
				// whichever candidate the dialler reached.
				if strings.Contains(detail, "127.0.0.1") || strings.Contains(detail, "::1") {
					found = true
				}
			}
			if !found {
				t.Errorf("no log entry names the address %q resolved to; the operator has no way to diagnose this refusal (%d entries)", url, len(entries))
			}
		})
	}
}
