package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/util"
	"mahresources/server"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestEnvWithShareConfig builds a test context with a specific
// SharePublicURL value so BH-033 can exercise the conditional URL rendering.
func setupTestEnvWithShareConfig(t *testing.T, sharePublicURL string) *TestContext {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Query{}, &models.Series{}, &models.Resource{}, &models.ResourceVersion{},
		&models.Note{}, &models.NoteBlock{}, &models.Tag{}, &models.Group{},
		&models.Category{}, &models.ResourceCategory{}, &models.NoteType{},
		&models.Preview{}, &models.GroupRelation{}, &models.GroupRelationType{},
		&models.ImageHash{}, &models.ResourceSimilarity{}, &models.LogEntry{},
		&models.PluginState{}, &models.PluginKV{}, &models.SavedMRQLQuery{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	util.AddInitialData(db)

	cfg := &application_context.MahresourcesConfig{
		DbType:          constants.DbTypeSqlite,
		BindAddress:     "127.0.0.1:8181",
		SharePort:       "8182",
		ShareBindAddress: "127.0.0.1",
		SharePublicURL:  sharePublicURL,
	}
	fs := afero.NewMemMapFs()
	sqlDB, _ := db.DB()
	roDB := sqlx.NewDb(sqlDB, "sqlite3")
	appCtx := application_context.NewMahresourcesContext(fs, db, roDB, cfg)
	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)
	appCtx.DefaultResourceCategoryID = defaultRC.ID

	srv := server.CreateServer(appCtx, fs, map[string]string{})
	return &TestContext{AppCtx: appCtx, Router: srv.Handler, DB: db}
}

// TestShareURL_NoFallback_WhenUnconfigured covers BH-033: when SHARE_PUBLIC_URL
// is empty, the note detail page must not synthesize an absolute share URL
// from SharePort + ShareBindAddress. The old fallback produced URLs like
// http://127.0.0.1:8182/... which are useless to any external recipient and
// misleading on any non-loopback deployment.
func TestShareURL_NoFallback_WhenUnconfigured(t *testing.T) {
	tc := setupTestEnvWithShareConfig(t, "")
	note := tc.CreateDummyNote("BH-033 no fallback")
	if _, err := tc.AppCtx.ShareNote(note.ID); err != nil {
		t.Fatalf("share: %v", err)
	}

	resp := tc.MakeRequest(http.MethodGet, "/note?id=1", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /note returned %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()

	// Historical bind-address fallback must not appear.
	if strings.Contains(body, "http://127.0.0.1:8182") ||
		strings.Contains(body, "http://127.0.0.1:8181/s/") ||
		strings.Contains(body, "http://0.0.0.0:") {
		t.Fatalf("share URL constructed from bind address even though SHARE_PUBLIC_URL is empty; body excerpt: %s", firstSnippet(body, 2000))
	}

	// A warning pointer to the config key MUST be visible to the admin so
	// they know why the sidebar is degraded.
	if !strings.Contains(body, "SHARE_PUBLIC_URL") {
		t.Errorf("warning message missing — page should reference SHARE_PUBLIC_URL config")
	}
}

// TestShareURL_UsesPublicURL_WhenSet covers the positive BH-033 path.
func TestShareURL_UsesPublicURL_WhenSet(t *testing.T) {
	tc := setupTestEnvWithShareConfig(t, "https://share.example.com")
	note := tc.CreateDummyNote("BH-033 uses public URL")
	if _, err := tc.AppCtx.ShareNote(note.ID); err != nil {
		t.Fatalf("share: %v", err)
	}

	resp := tc.MakeRequest(http.MethodGet, "/note?id=1", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /note returned %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()

	if !strings.Contains(body, "https://share.example.com") {
		t.Errorf("expected share URL base %q in page body; body excerpt: %s", "https://share.example.com", firstSnippet(body, 2000))
	}

	// Trailing slashes must not produce double-slash URLs, so verify the
	// normalized form.
	if strings.Contains(body, "https://share.example.com//s/") {
		t.Errorf("share URL double-slash: SharePublicURL trailing slash not stripped")
	}
}

// TestShareURL_StripsTrailingSlash verifies a configured URL with a trailing
// slash is normalized so templates concatenating "/s/<token>" don't produce
// https://host//s/token. BH-033.
func TestShareURL_StripsTrailingSlash(t *testing.T) {
	tc := setupTestEnvWithShareConfig(t, "https://share.example.com/")
	note := tc.CreateDummyNote("BH-033 trailing slash")
	if _, err := tc.AppCtx.ShareNote(note.ID); err != nil {
		t.Fatalf("share: %v", err)
	}
	resp := tc.MakeRequest(http.MethodGet, "/note?id=1", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /note returned %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if strings.Contains(body, "https://share.example.com//s/") {
		t.Errorf("trailing slash not stripped from SharePublicURL")
	}
}

// firstSnippet returns the first n bytes of s for test log legibility.
func firstSnippet(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
