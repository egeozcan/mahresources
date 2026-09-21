//go:build json1 && fts5

package application_context

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestResourceSeriesLockQueriesAreChunked(t *testing.T) {
	ctx := createTestContext(t)
	oldChunkSize := resourceSeriesLockChunkSize
	resourceSeriesLockChunkSize = 2
	t.Cleanup(func() { resourceSeriesLockChunkSize = oldChunkSize })

	ids := make([]uint, 0, 5)
	for i := 0; i < 5; i++ {
		series := &models.Series{Name: "chunk", Slug: fmt.Sprintf("chunk-%d", i), Meta: []byte(`{}`)}
		if err := ctx.db.Create(series).Error; err != nil {
			t.Fatal(err)
		}
		resource := &models.Resource{Name: fmt.Sprintf("resource-%d", i), SeriesID: &series.ID}
		if err := ctx.db.Create(resource).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, resource.ID)
	}

	queryCount := 0
	callbackName := "test:resource-series-lock-chunks"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db.Statement == nil || (db.Statement.Table != "resources" && db.Statement.Table != "series") ||
			!strings.Contains(strings.ToUpper(db.Statement.SQL.String()), " IN ") {
			return
		}
		queryCount++
		if len(db.Statement.Vars) > resourceSeriesLockChunkSize {
			t.Errorf("%s lock query used %d binds, chunk size %d", db.Statement.Table, len(db.Statement.Vars), resourceSeriesLockChunkSize)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(callbackName) })

	if err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		_, err := lockResourcesAfterSeries(txCtx.db, ids)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if queryCount < 9 { // three discovery, three Series-lock and three Resource-lock chunks
		t.Fatalf("observed %d chunked queries, want at least 9", queryCount)
	}
}

func TestWhitespaceSeriesMetaDefaultsToEmptyObject(t *testing.T) {
	ctx := createTestContext(t)
	series, err := ctx.CreateSeries(&query_models.SeriesCreator{
		Name: "Whitespace Meta", Slug: "whitespace-meta", Meta: " \n\t ",
	})
	if err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}
	if string(series.Meta) != "{}" {
		t.Fatalf("whitespace metadata persisted as %q instead of an empty object", series.Meta)
	}
	resource, err := ctx.AddResource(
		io.NopCloser(strings.NewReader("whitespace series metadata payload")),
		"whitespace.txt",
		&query_models.ResourceCreator{ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "Whitespace", Meta: `{}`, SeriesId: series.ID,
		}},
	)
	if err != nil {
		t.Fatalf("assign resource to whitespace-meta series: %v", err)
	}
	if resource.SeriesID == nil || *resource.SeriesID != series.ID {
		t.Fatalf("resource series = %v, want %d", resource.SeriesID, series.ID)
	}
}

func TestExplicitEmptySeriesDoesNotAdoptFirstResourceMeta(t *testing.T) {
	ctx := createTestContext(t)
	series, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "Explicit Empty", Slug: "explicit-empty"})
	if err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}

	resource, err := ctx.AddResource(
		io.NopCloser(strings.NewReader("explicit empty series payload")),
		"explicit.txt",
		&query_models.ResourceCreator{ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "Explicit", Meta: `{"item":"kept-own"}`, SeriesSlug: series.Slug,
		}},
	)
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}

	var storedSeries models.Series
	if err := ctx.db.First(&storedSeries, series.ID).Error; err != nil {
		t.Fatalf("read series: %v", err)
	}
	if string(storedSeries.Meta) != "{}" {
		t.Fatalf("explicit empty series adopted resource metadata: %s", storedSeries.Meta)
	}

	var storedResource models.Resource
	if err := ctx.db.First(&storedResource, resource.ID).Error; err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if string(storedResource.OwnMeta) != `{"item":"kept-own"}` {
		t.Fatalf("resource metadata was not retained as own metadata: %s", storedResource.OwnMeta)
	}
}

func TestAddRemoteResourceHonoursPositiveSeriesIDOverSlug(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")
	byID, err := ctx.CreateSeries(&query_models.SeriesCreator{
		Name: "By ID", Slug: "remote-by-id", Meta: `{"series":"id"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	bySlug, err := ctx.CreateSeries(&query_models.SeriesCreator{
		Name: "By Slug", Slug: "remote-by-slug", Meta: `{"series":"slug"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("remote series assignment"))
	}))
	t.Cleanup(srv.Close)

	resource, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{
		URL: srv.URL,
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "remote.txt", Meta: `{}`, SeriesId: byID.ID, SeriesSlug: bySlug.Slug,
		},
	})
	if err != nil {
		t.Fatalf("AddRemoteResource: %v", err)
	}
	var stored models.Resource
	if err := ctx.db.First(&stored, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SeriesID == nil || *stored.SeriesID != byID.ID {
		t.Fatalf("remote resource series = %v, want ID %d to win over slug", stored.SeriesID, byID.ID)
	}
	if string(stored.Meta) != `{"series":"id"}` {
		t.Fatalf("remote resource effective meta = %s", stored.Meta)
	}
}

func TestSeriesAssignmentPersistsEffectiveMeta(t *testing.T) {
	for _, tc := range []struct {
		name   string
		assign func(*query_models.ResourceQueryBase, *models.Series)
	}{
		{name: "id", assign: func(q *query_models.ResourceQueryBase, s *models.Series) { q.SeriesId = s.ID }},
		{name: "slug", assign: func(q *query_models.ResourceQueryBase, s *models.Series) { q.SeriesSlug = s.Slug }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := createTestContext(t)
			series, err := ctx.CreateSeries(&query_models.SeriesCreator{
				Name: "Inherited " + tc.name,
				Slug: "inherited-" + tc.name,
				Meta: `{"publisher":"Example"}`,
			})
			if err != nil {
				t.Fatalf("CreateSeries: %v", err)
			}
			base := query_models.ResourceQueryBase{Name: "Episode " + tc.name, Meta: `{}`}
			tc.assign(&base, series)
			resource, err := ctx.AddResource(
				io.NopCloser(strings.NewReader("inherit series metadata "+tc.name)),
				"episode-"+tc.name+".txt",
				&query_models.ResourceCreator{ResourceQueryBase: base},
			)
			if err != nil {
				t.Fatalf("AddResource: %v", err)
			}

			var stored models.Resource
			if err := ctx.db.First(&stored, resource.ID).Error; err != nil {
				t.Fatalf("read resource: %v", err)
			}
			if string(stored.Meta) != `{"publisher":"Example"}` {
				t.Fatalf("stored effective metadata = %s", stored.Meta)
			}
			if string(stored.OwnMeta) != "{}" {
				t.Fatalf("stored own metadata = %s", stored.OwnMeta)
			}
		})
	}
}
