package application_context

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	_, err := ctx.AddRemoteResource(&query_models.ResourceFromRemoteCreator{URL: srv.URL})
	if err == nil {
		t.Fatal("fetched from a private address with no -allow-private-fetch entry permitting it")
	}
	assertRefusalIsNotAnOracle(t, err)
}

func TestAddRemoteResource_ReachesAPrivateAddressTheOperatorNamed(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")
	srv := internalService(t)

	resource, err := ctx.AddRemoteResource(&query_models.ResourceFromRemoteCreator{URL: srv.URL})
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
