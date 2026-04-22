//go:build json1 && fts5

package openapi_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gorilla/mux"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/server"
	"mahresources/server/openapi"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/util"
)

// routesExcludedFromOpenAPI enumerates endpoints intentionally omitted from
// the OpenAPI spec. Keys are method + space + path. Each entry MUST have a
// comment explaining why it's excluded.
var routesExcludedFromOpenAPI = map[string]string{
	// PathPrefix handler in routes.go: router.PathPrefix("/v1/plugins/").
	// Each installed plugin registers its own routes at request time, so
	// we cannot enumerate them statically. Documented in the spec
	// description instead.
	"ANY /v1/plugins/": "dynamic plugin-specific API; routes vary per install",
}

// BH-022: Every /v1/ route registered with the mux MUST appear in the
// OpenAPI spec (or be in routesExcludedFromOpenAPI with a reason).
// This guards against drift — adding a new /v1/ endpoint without also
// adding it to server/routes_openapi.go fails this test.
func TestOpenAPI_RouteRegistrationCoverage(t *testing.T) {
	// Build a minimal app context + router exactly like main.go would.
	// CreateServer wraps the mux.Router in a http.HandlerFunc (BH-032 security
	// headers middleware), which can't be walked. BuildPrimaryRouter exposes
	// the raw router before that wrapping specifically for this drift check.
	ctx := newDriftTestContext(t)
	router := server.BuildPrimaryRouter(ctx, afero.NewMemMapFs(), map[string]string{})

	type liveRoute struct{ Method, Path string }
	var live []liveRoute
	walkErr := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, pathErr := route.GetPathTemplate()
		if pathErr != nil {
			// PathPrefix routes expose their prefix via GetPathTemplate after
			// normalising. If that fails too, try GetPathRegexp.
			return nil
		}
		if !strings.HasPrefix(path, "/v1/") {
			return nil
		}
		methods, _ := route.GetMethods()
		if len(methods) == 0 {
			live = append(live, liveRoute{Method: "ANY", Path: path})
			return nil
		}
		for _, m := range methods {
			live = append(live, liveRoute{Method: m, Path: path})
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("router walk: %v", walkErr)
	}

	// Build the spec and extract its operations
	registry := openapi.NewRegistry()
	server.RegisterAPIRoutesWithOpenAPI(registry)
	spec := registry.GenerateSpec()

	inSpec := map[string]bool{}
	if spec.Paths != nil {
		for path, pathItem := range spec.Paths.Map() {
			for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
				if pathItemOp(pathItem, method) != nil {
					inSpec[method+" "+path] = true
				}
			}
		}
	}

	var missing []string
	seen := map[string]bool{}
	for _, r := range live {
		key := r.Method + " " + r.Path
		if seen[key] {
			continue
		}
		seen[key] = true

		if inSpec[key] {
			continue
		}
		if _, excluded := routesExcludedFromOpenAPI[key]; excluded {
			continue
		}
		// Also accept the ANY-prefixed key if a prefix handler matches
		if r.Method != "ANY" {
			if _, excluded := routesExcludedFromOpenAPI["ANY "+r.Path]; excluded {
				continue
			}
		}
		missing = append(missing, key)
	}

	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("OpenAPI spec missing %d routes (add to server/routes_openapi.go or routesExcludedFromOpenAPI with a reason):\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// pathItemOp extracts an Operation by method name. kin-openapi's PathItem
// has one typed field per method.
func pathItemOp(p *openapi3.PathItem, method string) *openapi3.Operation {
	switch method {
	case http.MethodGet:
		return p.Get
	case http.MethodPost:
		return p.Post
	case http.MethodPut:
		return p.Put
	case http.MethodDelete:
		return p.Delete
	case http.MethodPatch:
		return p.Patch
	}
	return nil
}

// newDriftTestContext builds an in-memory MahresourcesContext that's just
// wired enough to let CreateServer register all routes. It's similar to
// SetupTestEnv in server/api_tests but lives in this package to avoid an
// import cycle.
func newDriftTestContext(t *testing.T) *application_context.MahresourcesContext {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:drift_test?mode=memory&cache=shared"), &gorm.Config{})
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

	sqlDB, _ := db.DB()
	roDB := sqlx.NewDb(sqlDB, "sqlite3")

	config := &application_context.MahresourcesConfig{
		DbType:      constants.DbTypeSqlite,
		BindAddress: ":0",
	}
	fs := afero.NewMemMapFs()
	return application_context.NewMahresourcesContext(fs, db, roDB, config)
}
