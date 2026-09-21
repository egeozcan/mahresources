//go:build postgres && json1 && fts5

package application_context

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

func newSeriesPostgresContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(
		&models.Query{}, &models.Resource{}, &models.Note{}, &models.Tag{},
		&models.Group{}, &models.Category{}, &models.NoteType{}, &models.Preview{},
		&models.GroupRelation{}, &models.GroupRelationType{}, &models.ImageHash{},
		&models.ResourceSimilarity{}, &models.LogEntry{}, &models.ResourceCategory{},
		&models.Series{}, &models.NoteBlock{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open read-only handle: %v", err)
	}
	t.Cleanup(func() { _ = readOnly.Close() })
	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{DbType: constants.DbTypePosgres})
}

func TestSeriesCreatorDetectionUsesPostgresInsertResult(t *testing.T) {
	ctx := newSeriesPostgresContext(t)

	explicit := &models.Series{Name: "Explicit Empty", Slug: "explicit-empty-pg", Meta: []byte("{}")}
	if err := ctx.db.Create(explicit).Error; err != nil {
		t.Fatalf("create explicit series: %v", err)
	}

	if err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		_, creator, err := txCtx.GetOrCreateSeriesForResource(txCtx.db, explicit.Slug)
		if err != nil {
			return err
		}
		if creator {
			t.Fatal("an existing empty Postgres series was treated as newly inserted")
		}

		_, creator, err = txCtx.GetOrCreateSeriesForResource(txCtx.db, "new-series-pg")
		if err != nil {
			return err
		}
		if !creator {
			t.Fatal("a newly inserted Postgres series was not treated as creator")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func assertSeriesJSONEqual(t *testing.T, got types.JSON, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode got JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode wanted JSON: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON mismatch: got %s, want %s", got, want)
	}
}

func TestConcurrentPostgresSeriesPatchAndAssignmentStayConsistent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		assign func(*query_models.ResourceQueryBase, *models.Series)
	}{
		{name: "id", assign: func(q *query_models.ResourceQueryBase, s *models.Series) { q.SeriesId = s.ID }},
		{name: "existing-slug", assign: func(q *query_models.ResourceQueryBase, s *models.Series) { q.SeriesSlug = s.Slug }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newSeriesPostgresContext(t)
			series, err := ctx.CreateSeries(&query_models.SeriesCreator{
				Name: "Assignment Race", Slug: "assignment-race-" + tc.name, Meta: `{"version":0}`,
			})
			if err != nil {
				t.Fatal(err)
			}

			var seriesReads atomic.Int32
			patchRead := make(chan struct{})
			assignmentRead := make(chan struct{})
			releasePatch := make(chan struct{})
			callbackName := "test:pause-series-patch-before-assignment-" + tc.name
			if err := ctx.db.Callback().Query().After("gorm:query").Register(callbackName, func(db *gorm.DB) {
				if db.Statement == nil || db.Statement.Table != "series" ||
					!strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") {
					return
				}
				switch seriesReads.Add(1) {
				case 1:
					close(patchRead)
					<-releasePatch
				case 2:
					close(assignmentRead)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(callbackName) })

			patchDone := make(chan error, 1)
			go func() {
				_, err := ctx.UpdateSeries(&query_models.SeriesEditor{ID: series.ID, Meta: `{"version":1}`})
				patchDone <- err
			}()
			select {
			case <-patchRead:
			case <-time.After(5 * time.Second):
				t.Fatal("metadata patch never reached its series read")
			}

			base := query_models.ResourceQueryBase{Name: "Concurrent " + tc.name, Meta: `{}`}
			tc.assign(&base, series)
			type assignmentResult struct {
				resource *models.Resource
				err      error
			}
			assignmentDone := make(chan assignmentResult, 1)
			go func() {
				resource, err := ctx.AddResource(
					io.NopCloser(strings.NewReader("series assignment race "+tc.name)),
					"assignment-"+tc.name+".txt",
					&query_models.ResourceCreator{ResourceQueryBase: base},
				)
				assignmentDone <- assignmentResult{resource: resource, err: err}
			}()

			assignmentReadBeforeRelease := false
			select {
			case <-assignmentRead:
				assignmentReadBeforeRelease = true
			case <-time.After(500 * time.Millisecond):
				// A shared FOR UPDATE lock keeps assignment from deriving metadata
				// until the in-flight metadata patch commits.
			}
			var result assignmentResult
			if assignmentReadBeforeRelease {
				result = <-assignmentDone
				if result.err != nil {
					t.Fatal(result.err)
				}
			}
			close(releasePatch)
			if err := <-patchDone; err != nil {
				t.Fatal(err)
			}
			if !assignmentReadBeforeRelease {
				result = <-assignmentDone
				if result.err != nil {
					t.Fatal(result.err)
				}
			}

			var stored models.Resource
			if err := ctx.db.First(&stored, result.resource.ID).Error; err != nil {
				t.Fatal(err)
			}
			var meta map[string]any
			if err := json.Unmarshal(stored.Meta, &meta); err != nil {
				t.Fatal(err)
			}
			if meta["version"] != float64(1) {
				t.Fatalf("assignment persisted stale effective metadata: %s", stored.Meta)
			}
		})
	}
}

func TestConcurrentPostgresSeriesPatchAndExistingMemberMoveStayConsistent(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	from, err := ctx.CreateSeries(&query_models.SeriesCreator{
		Name: "From", Slug: "move-from", Meta: `{"from":0}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	to, err := ctx.CreateSeries(&query_models.SeriesCreator{
		Name: "To", Slug: "move-to", Meta: `{"to":true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	fromID := from.ID
	moving := &models.Resource{Name: "Moving", SeriesID: &fromID, OwnMeta: types.JSON("{}"), Meta: from.Meta}
	keeper := &models.Resource{Name: "Keeper", SeriesID: &fromID, OwnMeta: types.JSON("{}"), Meta: from.Meta}
	if err := ctx.db.Create(moving).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Create(keeper).Error; err != nil {
		t.Fatal(err)
	}

	patchRead := make(chan struct{})
	releasePatch := make(chan struct{})
	var paused atomic.Bool
	callbackName := "test:pause-series-patch-before-member-move"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "series" &&
			strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") && paused.CompareAndSwap(false, true) {
			close(patchRead)
			<-releasePatch
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(callbackName) })

	patchDone := make(chan error, 1)
	go func() {
		_, err := ctx.UpdateSeries(&query_models.SeriesEditor{ID: from.ID, Meta: `{"from":1}`})
		patchDone <- err
	}()
	select {
	case <-patchRead:
	case <-time.After(5 * time.Second):
		t.Fatal("metadata patch never reached its locked read")
	}

	type editResult struct {
		resource *models.Resource
		err      error
	}
	moveDone := make(chan editResult, 1)
	go func() {
		resource, err := ctx.EditResource(&query_models.ResourceEditor{
			ID: moving.ID,
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name: moving.Name, Meta: string(moving.Meta), SeriesId: to.ID,
			},
		})
		moveDone <- editResult{resource: resource, err: err}
	}()

	var move editResult
	moveCompletedBeforeRelease := false
	select {
	case move = <-moveDone:
		moveCompletedBeforeRelease = true
		if move.err != nil {
			t.Fatal(move.err)
		}
	case <-time.After(500 * time.Millisecond):
		// A move must wait on the old Series lock as well as its destination.
	}
	close(releasePatch)
	if err := <-patchDone; err != nil {
		t.Fatal(err)
	}
	if !moveCompletedBeforeRelease {
		move = <-moveDone
		if move.err != nil {
			t.Fatal(move.err)
		}
	}

	var stored models.Resource
	if err := ctx.db.First(&stored, moving.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SeriesID == nil || *stored.SeriesID != to.ID {
		t.Fatalf("resource did not move to destination Series: %v", stored.SeriesID)
	}
	effective, err := mergeMeta(to.Meta, stored.OwnMeta)
	if err != nil {
		t.Fatal(err)
	}
	var gotMeta, wantMeta map[string]any
	if err := json.Unmarshal(stored.Meta, &gotMeta); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(effective, &wantMeta); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(gotMeta) != fmt.Sprint(wantMeta) {
		t.Fatalf("moved Resource metadata is inconsistent: got=%s want=%s own=%s", stored.Meta, effective, stored.OwnMeta)
	}
}

func TestPluginPatchResourcePreservesPostReadSeriesChanges(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	series, err := ctx.CreateSeries(&query_models.SeriesCreator{
		Name: "Adapter Patch", Slug: "adapter-patch", Meta: `{"base":"old"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	resource := &models.Resource{
		Name: "member", Meta: types.JSON(`{"base":"old","own":1}`),
		OwnMeta: types.JSON(`{"own":1}`), SeriesID: &series.ID,
	}
	if err := ctx.db.Create(resource).Error; err != nil {
		t.Fatal(err)
	}

	oldAfterRead := pluginPatchResourceAfterRead
	pluginPatchResourceAfterRead = func() {
		pluginPatchResourceAfterRead = nil
		if _, err := ctx.UpdateSeries(&query_models.SeriesEditor{ID: series.ID, Meta: `{"base":"new"}`}); err != nil {
			t.Errorf("patch Series in adapter read gap: %v", err)
		}
	}
	t.Cleanup(func() { pluginPatchResourceAfterRead = oldAfterRead })

	adapter := &pluginDBAdapter{ctx: ctx}
	if _, err := adapter.PatchResource(resource.ID, map[string]any{"name": "renamed"}); err != nil {
		t.Fatal(err)
	}

	var stored models.Resource
	if err := ctx.db.First(&stored, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SeriesID == nil || *stored.SeriesID != series.ID {
		t.Fatalf("membership = %v, want Series %d", stored.SeriesID, series.ID)
	}
	assertSeriesJSONEqual(t, stored.OwnMeta, `{"own":1}`)
	assertSeriesJSONEqual(t, stored.Meta, `{"base":"new","own":1}`)
}

func TestPluginPatchResourcePreservesPostReadMembershipChange(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	a, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "A", Slug: "adapter-member-a", Meta: `{"base":"a"}`})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "B", Slug: "adapter-member-b", Meta: `{"base":"b"}`})
	if err != nil {
		t.Fatal(err)
	}
	resource := &models.Resource{
		Name: "member", Meta: types.JSON(`{"base":"a","own":1}`),
		OwnMeta: types.JSON(`{"own":1}`), SeriesID: &a.ID,
	}
	if err := ctx.db.Create(resource).Error; err != nil {
		t.Fatal(err)
	}

	oldAfterRead := pluginPatchResourceAfterRead
	pluginPatchResourceAfterRead = func() {
		pluginPatchResourceAfterRead = nil
		if _, err := ctx.EditResource(&query_models.ResourceEditor{
			ID: resource.ID,
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name: resource.Name, Meta: `{"base":"b","own":1}`, SeriesId: b.ID,
			},
		}); err != nil {
			t.Errorf("move Resource in adapter read gap: %v", err)
			return
		}
		if err := ctx.DeleteSeries(a.ID); err != nil {
			t.Errorf("delete obsolete source Series: %v", err)
		}
	}
	t.Cleanup(func() { pluginPatchResourceAfterRead = oldAfterRead })

	adapter := &pluginDBAdapter{ctx: ctx}
	if _, err := adapter.PatchResource(resource.ID, map[string]any{"name": "renamed"}); err != nil {
		t.Fatal(err)
	}

	var stored models.Resource
	if err := ctx.db.First(&stored, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SeriesID == nil || *stored.SeriesID != b.ID {
		t.Fatalf("membership = %v, want Series B (%d)", stored.SeriesID, b.ID)
	}
	assertSeriesJSONEqual(t, stored.Meta, `{"base":"b","own":1}`)
}

func TestConcurrentPostgresSeriesPatchAndSameSeriesEditPreserveFreshMeta(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	series, err := ctx.CreateSeries(&query_models.SeriesCreator{
		Name: "Same Series Edit", Slug: "same-series-edit", Meta: `{"base":"old"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	resource := &models.Resource{
		Name: "member", Meta: types.JSON(`{"base":"old","own":1}`),
		OwnMeta: types.JSON(`{"own":1}`), SeriesID: &series.ID,
	}
	if err := ctx.db.Create(resource).Error; err != nil {
		t.Fatal(err)
	}

	seriesLocked := make(chan struct{})
	releasePatch := make(chan struct{})
	var paused atomic.Bool
	queryCallback := "test:pause-patch-before-same-series-edit"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(queryCallback, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "series" &&
			strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") &&
			paused.CompareAndSwap(false, true) {
			close(seriesLocked)
			<-releasePatch
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(queryCallback) })

	patchDone := make(chan error, 1)
	go func() {
		_, err := ctx.UpdateSeries(&query_models.SeriesEditor{ID: series.ID, Meta: `{"base":"new"}`})
		patchDone <- err
	}()
	select {
	case <-seriesLocked:
	case <-time.After(5 * time.Second):
		t.Fatal("patch did not lock the Series")
	}

	initialResourceRead := make(chan struct{})
	var sawInitialRead atomic.Bool
	resourceCallback := "test:observe-edit-prelock-resource-read"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(resourceCallback, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "resources" &&
			!strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") &&
			sawInitialRead.CompareAndSwap(false, true) {
			close(initialResourceRead)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(resourceCallback) })

	editDone := make(chan error, 1)
	go func() {
		_, err := ctx.EditResource(&query_models.ResourceEditor{
			ID: resource.ID,
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name: "renamed", SeriesId: series.ID,
			},
		})
		editDone <- err
	}()
	select {
	case <-initialResourceRead:
	case <-time.After(5 * time.Second):
		close(releasePatch)
		t.Fatal("edit did not read the Resource before waiting for the Series")
	}
	close(releasePatch)

	for name, done := range map[string]<-chan error{"patch": patchDone, "edit": editDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s failed: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not finish", name)
		}
	}

	var stored models.Resource
	if err := ctx.db.First(&stored, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	assertSeriesJSONEqual(t, stored.OwnMeta, `{"own":1}`)
	assertSeriesJSONEqual(t, stored.Meta, `{"base":"new","own":1}`)
}

func TestConcurrentPostgresMovesOfSameResourceRetryMembershipDrift(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	a, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "A", Slug: "same-move-a", Meta: `{"base":"a"}`})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "B", Slug: "same-move-b", Meta: `{"base":"b"}`})
	if err != nil {
		t.Fatal(err)
	}
	c, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "C", Slug: "same-move-c", Meta: `{"base":"c"}`})
	if err != nil {
		t.Fatal(err)
	}
	resource := &models.Resource{
		Name: "member", Meta: types.JSON(`{"base":"a","own":1}`),
		OwnMeta: types.JSON(`{"own":1}`), SeriesID: &a.ID,
	}
	if err := ctx.db.Create(resource).Error; err != nil {
		t.Fatal(err)
	}

	firstMoveLocked := make(chan struct{})
	releaseFirstMove := make(chan struct{})
	var paused atomic.Bool
	seriesCallback := "test:pause-first-same-resource-move"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(seriesCallback, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "series" &&
			strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") &&
			paused.CompareAndSwap(false, true) {
			close(firstMoveLocked)
			<-releaseFirstMove
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(seriesCallback) })

	moveOneDone := make(chan error, 1)
	go func() {
		_, err := ctx.EditResource(&query_models.ResourceEditor{
			ID: resource.ID,
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name: "to-b", Meta: `{"base":"b","own":1}`, SeriesId: b.ID,
			},
		})
		moveOneDone <- err
	}()
	select {
	case <-firstMoveLocked:
	case <-time.After(5 * time.Second):
		t.Fatal("first move did not lock its Series rows")
	}

	secondRead := make(chan struct{})
	var observedSecondRead atomic.Bool
	resourceCallback := "test:observe-second-move-source-read"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(resourceCallback, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "resources" &&
			!strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") &&
			observedSecondRead.CompareAndSwap(false, true) {
			close(secondRead)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(resourceCallback) })

	moveTwoDone := make(chan error, 1)
	go func() {
		_, err := ctx.EditResource(&query_models.ResourceEditor{
			ID: resource.ID,
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name: "to-c", SeriesId: c.ID,
			},
		})
		moveTwoDone <- err
	}()
	select {
	case <-secondRead:
	case <-time.After(5 * time.Second):
		close(releaseFirstMove)
		t.Fatal("second move did not read the original membership")
	}
	close(releaseFirstMove)

	for name, done := range map[string]<-chan error{"first move": moveOneDone, "second move": moveTwoDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s failed: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not finish", name)
		}
	}

	var stored models.Resource
	if err := ctx.db.First(&stored, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SeriesID == nil || *stored.SeriesID != c.ID {
		t.Fatalf("final membership = %v, want Series C (%d)", stored.SeriesID, c.ID)
	}
	assertSeriesJSONEqual(t, stored.OwnMeta, `{"base":"b","own":1}`)
	assertSeriesJSONEqual(t, stored.Meta, `{"base":"b","own":1}`)
}

func TestPostgresEditRetriesWhenDiscoveredSourceSeriesWasRemoved(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	a, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "A", Slug: "missing-source-a", Meta: `{"base":"a"}`})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "B", Slug: "missing-source-b", Meta: `{"base":"b"}`})
	if err != nil {
		t.Fatal(err)
	}
	resource := &models.Resource{
		Name: "member", Meta: types.JSON(`{"base":"a","own":1}`),
		OwnMeta: types.JSON(`{"own":1}`), SeriesID: &a.ID,
	}
	if err := ctx.db.Create(resource).Error; err != nil {
		t.Fatal(err)
	}

	removalLocked := make(chan struct{})
	releaseRemoval := make(chan struct{})
	var paused atomic.Bool
	seriesCallback := "test:pause-removal-before-edit-lock"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(seriesCallback, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "series" &&
			strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") &&
			paused.CompareAndSwap(false, true) {
			close(removalLocked)
			<-releaseRemoval
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(seriesCallback) })

	removeDone := make(chan error, 1)
	go func() { removeDone <- ctx.RemoveResourceFromSeries(resource.ID) }()
	select {
	case <-removalLocked:
	case <-time.After(5 * time.Second):
		t.Fatal("removal did not lock source Series")
	}

	editRead := make(chan struct{})
	var observedRead atomic.Bool
	resourceCallback := "test:observe-edit-before-source-removal"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(resourceCallback, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "resources" &&
			!strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") &&
			observedRead.CompareAndSwap(false, true) {
			close(editRead)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(resourceCallback) })

	editDone := make(chan error, 1)
	go func() {
		_, err := ctx.EditResource(&query_models.ResourceEditor{
			ID: resource.ID,
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name: "moved", Meta: `{"base":"b","own":1}`, SeriesId: b.ID,
			},
		})
		editDone <- err
	}()
	select {
	case <-editRead:
	case <-time.After(5 * time.Second):
		close(releaseRemoval)
		t.Fatal("edit did not discover the old source Series")
	}
	close(releaseRemoval)
	for name, done := range map[string]<-chan error{"remove": removeDone, "edit": editDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s failed: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not finish", name)
		}
	}

	var stored models.Resource
	if err := ctx.db.First(&stored, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SeriesID == nil || *stored.SeriesID != b.ID {
		t.Fatalf("membership = %v, want Series B (%d)", stored.SeriesID, b.ID)
	}
	assertSeriesJSONEqual(t, stored.Meta, `{"base":"b","own":1}`)
}

func TestPostgresRemovalRefreshesAfterConcurrentMove(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	a, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "A", Slug: "remove-move-a", Meta: `{"base":"a"}`})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "B", Slug: "remove-move-b", Meta: `{"base":"b"}`})
	if err != nil {
		t.Fatal(err)
	}
	resource := &models.Resource{
		Name: "member", Meta: types.JSON(`{"base":"a","own":1}`),
		OwnMeta: types.JSON(`{"own":1}`), SeriesID: &a.ID,
	}
	if err := ctx.db.Create(resource).Error; err != nil {
		t.Fatal(err)
	}

	moveLocked := make(chan struct{})
	releaseMove := make(chan struct{})
	var paused atomic.Bool
	seriesCallback := "test:pause-move-before-removal"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(seriesCallback, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "series" &&
			strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") &&
			paused.CompareAndSwap(false, true) {
			close(moveLocked)
			<-releaseMove
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(seriesCallback) })

	moveDone := make(chan error, 1)
	go func() {
		_, err := ctx.EditResource(&query_models.ResourceEditor{
			ID: resource.ID,
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name: "moved", Meta: `{"base":"b","own":1}`, SeriesId: b.ID,
			},
		})
		moveDone <- err
	}()
	select {
	case <-moveLocked:
	case <-time.After(5 * time.Second):
		t.Fatal("move did not lock source and destination Series")
	}

	removalRead := make(chan struct{})
	var observedRead atomic.Bool
	resourceCallback := "test:observe-removal-before-move"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(resourceCallback, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "resources" &&
			!strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") &&
			observedRead.CompareAndSwap(false, true) {
			close(removalRead)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(resourceCallback) })

	removeDone := make(chan error, 1)
	go func() { removeDone <- ctx.RemoveResourceFromSeries(resource.ID) }()
	select {
	case <-removalRead:
	case <-time.After(5 * time.Second):
		close(releaseMove)
		t.Fatal("removal did not read the original membership")
	}
	close(releaseMove)
	for name, done := range map[string]<-chan error{"move": moveDone, "remove": removeDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s failed: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not finish", name)
		}
	}

	var stored models.Resource
	if err := ctx.db.First(&stored, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SeriesID != nil {
		t.Fatalf("membership = %v, want detached", *stored.SeriesID)
	}
	assertSeriesJSONEqual(t, stored.OwnMeta, `{}`)
	assertSeriesJSONEqual(t, stored.Meta, `{"base":"b","own":1}`)
}

func TestPostgresDeleteRebacksUpContentChangedBeforeResourceLock(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	resource := &models.Resource{
		Name: "version race", Hash: "old-hash", Location: "old.bin",
		Meta: types.JSON(`{}`), OwnMeta: types.JSON(`{}`),
	}
	if err := ctx.db.Create(resource).Error; err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(ctx.fs, "old.bin", []byte("old bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(ctx.fs, "new.bin", []byte("new bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	beforeResourceLock := make(chan struct{})
	releaseDelete := make(chan struct{})
	var paused atomic.Bool
	oldAfterBackup := deleteResourceAfterBackup
	deleteResourceAfterBackup = func() {
		if paused.CompareAndSwap(false, true) {
			close(beforeResourceLock)
			<-releaseDelete
		}
	}
	t.Cleanup(func() { deleteResourceAfterBackup = oldAfterBackup })

	deleteDone := make(chan error, 1)
	go func() { deleteDone <- ctx.DeleteResource(resource.ID) }()
	select {
	case <-beforeResourceLock:
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not reach the Resource lock after its first backup")
	}
	if err := ctx.db.Model(&models.Resource{}).Where("id = ?", resource.ID).
		Updates(map[string]any{"hash": "new-hash", "location": "new.bin"}).Error; err != nil {
		close(releaseDelete)
		t.Fatal(err)
	}
	close(releaseDelete)
	select {
	case err := <-deleteDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not finish")
	}

	newBackup := fmt.Sprintf("/deleted/deleted/new-hash__%d__nil___new.bin", resource.ID)
	got, err := afero.ReadFile(ctx.fs, newBackup)
	if err != nil {
		t.Fatalf("read refreshed backup: %v", err)
	}
	if string(got) != "new bytes" {
		t.Fatalf("backup = %q, want refreshed content", got)
	}
	oldBackup := fmt.Sprintf("/deleted/deleted/old-hash__%d__nil___old.bin", resource.ID)
	if exists, err := afero.Exists(ctx.fs, oldBackup); err != nil || exists {
		t.Fatalf("stale backup exists=%v err=%v", exists, err)
	}
	if exists, err := afero.Exists(ctx.fs, "new.bin"); err != nil || exists {
		t.Fatalf("refreshed source exists=%v err=%v", exists, err)
	}
}

func TestPostgresDeleteRetriesWhenOldSeriesWasRemoved(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	series, err := ctx.CreateSeries(&query_models.SeriesCreator{
		Name: "Removed During Delete", Slug: "removed-during-delete", Meta: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	resource := &models.Resource{Name: "member", Meta: types.JSON(`{}`), OwnMeta: types.JSON(`{}`), SeriesID: &series.ID}
	if err := ctx.db.Create(resource).Error; err != nil {
		t.Fatal(err)
	}

	afterBackup := make(chan struct{})
	releaseDelete := make(chan struct{})
	var paused atomic.Bool
	oldAfterBackup := deleteResourceAfterBackup
	deleteResourceAfterBackup = func() {
		if paused.CompareAndSwap(false, true) {
			close(afterBackup)
			<-releaseDelete
		}
	}
	t.Cleanup(func() { deleteResourceAfterBackup = oldAfterBackup })

	deleteDone := make(chan error, 1)
	go func() { deleteDone <- ctx.DeleteResource(resource.ID) }()
	select {
	case <-afterBackup:
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not finish backup preparation")
	}
	if err := ctx.RemoveResourceFromSeries(resource.ID); err != nil {
		close(releaseDelete)
		t.Fatal(err)
	}
	close(releaseDelete)
	select {
	case err := <-deleteDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not retry after old Series removal")
	}
	var count int64
	if err := ctx.db.Model(&models.Resource{}).Where("id = ?", resource.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("resource still exists after delete retry")
	}
}

func TestPostgresResourceEditAndDeleteUseSeriesBeforeResourceLockOrder(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	series, err := ctx.CreateSeries(&query_models.SeriesCreator{
		Name: "Edit Delete Race", Slug: "edit-delete-race", Meta: `{"series":true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	resource := &models.Resource{
		Name: "member", Meta: types.JSON(`{"series":true,"member":1}`),
		OwnMeta: types.JSON(`{"member":1}`), SeriesID: &series.ID,
	}
	if err := ctx.db.Create(resource).Error; err != nil {
		t.Fatal(err)
	}

	seriesLocked := make(chan struct{})
	releaseEdit := make(chan struct{})
	var pausedEdit atomic.Bool
	queryCallback := "test:pause-edit-after-series-lock"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(queryCallback, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "series" &&
			strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") &&
			pausedEdit.CompareAndSwap(false, true) {
			close(seriesLocked)
			<-releaseEdit
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(queryCallback) })

	resourceDeleted := make(chan struct{})
	releaseDelete := make(chan struct{})
	var pausedDelete atomic.Bool
	deleteCallback := "test:pause-delete-after-resource-row"
	if err := ctx.db.Callback().Delete().After("gorm:delete").Register(deleteCallback, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "resources" &&
			pausedDelete.CompareAndSwap(false, true) {
			close(resourceDeleted)
			<-releaseDelete
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Delete().Remove(deleteCallback) })

	editDone := make(chan error, 1)
	go func() {
		_, err := ctx.EditResource(&query_models.ResourceEditor{
			ID: resource.ID,
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name: "edited", Meta: `{"series":true,"member":2}`, SeriesId: series.ID,
			},
		})
		editDone <- err
	}()
	select {
	case <-seriesLocked:
	case <-time.After(5 * time.Second):
		t.Fatal("edit did not lock the Series")
	}

	deleteDone := make(chan error, 1)
	go func() { deleteDone <- ctx.DeleteResource(resource.ID) }()
	deleteReachedResourceEarly := false
	select {
	case <-resourceDeleted:
		deleteReachedResourceEarly = true
	case <-time.After(300 * time.Millisecond):
		// Correct order: delete is waiting for the Series held by edit and has
		// not yet acquired/deleted the Resource row.
	}

	close(releaseEdit)
	if !deleteReachedResourceEarly {
		select {
		case <-resourceDeleted:
		case <-time.After(5 * time.Second):
			t.Fatal("delete did not reach the Resource after edit released the Series")
		}
	}
	close(releaseDelete)
	for name, done := range map[string]<-chan error{"edit": editDone, "delete": deleteDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s failed: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not finish", name)
		}
	}
}

func TestPostgresBulkAndMergeDeletionUseSeriesBeforeResourceLockOrder(t *testing.T) {
	for _, mode := range []string{"bulk", "merge"} {
		t.Run(mode, func(t *testing.T) {
			ctx := newSeriesPostgresContext(t)
			series, err := ctx.CreateSeries(&query_models.SeriesCreator{
				Name: "Batch Delete Race", Slug: "batch-delete-race-" + mode, Meta: `{"series":true}`,
			})
			if err != nil {
				t.Fatal(err)
			}
			resource := &models.Resource{
				Name: "loser", Meta: types.JSON(`{"series":true,"member":1}`),
				OwnMeta: types.JSON(`{"member":1}`), SeriesID: &series.ID,
			}
			if err := ctx.db.Create(resource).Error; err != nil {
				t.Fatal(err)
			}
			winner := &models.Resource{Name: "winner", Meta: types.JSON(`{}`), OwnMeta: types.JSON(`{}`)}
			if mode == "merge" {
				if err := ctx.db.Create(winner).Error; err != nil {
					t.Fatal(err)
				}
			}

			seriesLocked := make(chan struct{})
			releaseEdit := make(chan struct{})
			var pausedEdit atomic.Bool
			queryCallback := "test:pause-batch-delete-edit-" + mode
			if err := ctx.db.Callback().Query().After("gorm:query").Register(queryCallback, func(db *gorm.DB) {
				if db.Statement != nil && db.Statement.Table == "series" &&
					strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") &&
					pausedEdit.CompareAndSwap(false, true) {
					close(seriesLocked)
					<-releaseEdit
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(queryCallback) })

			resourceDeleted := make(chan struct{})
			releaseDelete := make(chan struct{})
			var pausedDelete atomic.Bool
			deleteCallback := "test:pause-batch-resource-delete-" + mode
			if err := ctx.db.Callback().Delete().After("gorm:delete").Register(deleteCallback, func(db *gorm.DB) {
				if db.Statement != nil && db.Statement.Table == "resources" &&
					pausedDelete.CompareAndSwap(false, true) {
					close(resourceDeleted)
					<-releaseDelete
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ctx.db.Callback().Delete().Remove(deleteCallback) })

			editDone := make(chan error, 1)
			go func() {
				_, err := ctx.EditResource(&query_models.ResourceEditor{
					ID: resource.ID,
					ResourceQueryBase: query_models.ResourceQueryBase{
						Name: "edited", Meta: `{"series":true,"member":2}`, SeriesId: series.ID,
					},
				})
				editDone <- err
			}()
			select {
			case <-seriesLocked:
			case <-time.After(5 * time.Second):
				t.Fatal("edit did not lock the Series")
			}

			deleteDone := make(chan error, 1)
			go func() {
				if mode == "bulk" {
					deleteDone <- ctx.BulkDeleteResources(&query_models.BulkQuery{ID: []uint{resource.ID}})
					return
				}
				deleteDone <- ctx.MergeResources(winner.ID, []uint{resource.ID}, false)
			}()
			select {
			case <-resourceDeleted:
				close(releaseEdit)
				close(releaseDelete)
				t.Fatal("batch deletion reached the Resource before the held Series was released")
			case <-time.After(300 * time.Millisecond):
			}
			close(releaseEdit)
			select {
			case <-resourceDeleted:
			case <-time.After(5 * time.Second):
				t.Fatal("batch deletion did not reach the Resource after Series release")
			}
			close(releaseDelete)

			for name, done := range map[string]<-chan error{"edit": editDone, mode: deleteDone} {
				select {
				case err := <-done:
					if err != nil {
						t.Fatalf("%s failed: %v", name, err)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("%s did not finish", name)
				}
			}
		})
	}
}

func TestReciprocalPostgresSeriesMovesUseCanonicalLockOrder(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	a, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "A", Slug: "swap-a", Meta: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "B", Slug: "swap-b", Meta: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	aID, bID := a.ID, b.ID
	x := &models.Resource{Name: "X", SeriesID: &aID, OwnMeta: types.JSON("{}"), Meta: types.JSON("{}")}
	y := &models.Resource{Name: "Y", SeriesID: &bID, OwnMeta: types.JSON("{}"), Meta: types.JSON("{}")}
	if err := ctx.db.Create(x).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Create(y).Error; err != nil {
		t.Fatal(err)
	}

	lockReached := make(chan struct{}, 2)
	releaseLocks := make(chan struct{})
	callbackName := "test:barrier-reciprocal-series-moves"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "series" && strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") {
			lockReached <- struct{}{}
			<-releaseLocks
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(callbackName) })

	move := func(resource *models.Resource, destination uint) <-chan error {
		done := make(chan error, 1)
		go func() {
			_, err := ctx.EditResource(&query_models.ResourceEditor{
				ID: resource.ID,
				ResourceQueryBase: query_models.ResourceQueryBase{
					Name: resource.Name, Meta: string(resource.Meta), SeriesId: destination,
				},
			})
			done <- err
		}()
		return done
	}
	xDone := move(x, b.ID)
	yDone := move(y, a.ID)

	select {
	case <-lockReached:
	case <-time.After(5 * time.Second):
		t.Fatal("neither move reached its Series lock")
	}
	select {
	case <-lockReached:
		// On the broken destination-first implementation both transactions lock
		// one different row and reach the barrier, then deadlock after release.
	case <-time.After(500 * time.Millisecond):
		// Canonical multi-row locking keeps the second transaction outside until
		// the first commits.
	}
	close(releaseLocks)
	if err := <-xDone; err != nil {
		t.Fatalf("move X to B: %v", err)
	}
	if err := <-yDone; err != nil {
		t.Fatalf("move Y to A: %v", err)
	}

	var storedX, storedY models.Resource
	if err := ctx.db.First(&storedX, x.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.First(&storedY, y.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedX.SeriesID == nil || *storedX.SeriesID != b.ID || storedY.SeriesID == nil || *storedY.SeriesID != a.ID {
		t.Fatalf("reciprocal moves did not both commit: X=%v Y=%v", storedX.SeriesID, storedY.SeriesID)
	}
}

func TestConcurrentPostgresSeriesPatchesPreserveOmittedFields(t *testing.T) {
	ctx := newSeriesPostgresContext(t)
	series, err := ctx.CreateSeries(&query_models.SeriesCreator{
		Name: "Original", Slug: "concurrent-patches", Meta: `{"version":0}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	seriesID := series.ID
	resource := &models.Resource{
		Name: "Episode", SeriesID: &seriesID, OwnMeta: types.JSON("{}"), Meta: types.JSON(`{"version":0}`),
	}
	if err := ctx.db.Create(resource).Error; err != nil {
		t.Fatal(err)
	}

	var seriesReads atomic.Int32
	firstRead := make(chan struct{})
	secondRead := make(chan struct{})
	releaseFirst := make(chan struct{})
	callbackName := "test:pause-first-series-patch"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db.Statement == nil || db.Statement.Table != "series" ||
			!strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") {
			return
		}
		switch seriesReads.Add(1) {
		case 1:
			close(firstRead)
			<-releaseFirst
		case 2:
			close(secondRead)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(callbackName) })

	nameDone := make(chan error, 1)
	go func() {
		_, err := ctx.UpdateSeries(&query_models.SeriesEditor{ID: series.ID, Name: "Renamed"})
		nameDone <- err
	}()
	select {
	case <-firstRead:
	case <-time.After(5 * time.Second):
		t.Fatal("name-only patch never reached its series read")
	}

	metaDone := make(chan error, 1)
	go func() {
		_, err := ctx.UpdateSeries(&query_models.SeriesEditor{ID: series.ID, Meta: `{"version":1}`})
		metaDone <- err
	}()

	secondReadBeforeRelease := false
	select {
	case <-secondRead:
		secondReadBeforeRelease = true
	case <-time.After(500 * time.Millisecond):
		// With FOR UPDATE, the metadata patch is blocked before its read until
		// the name-only patch releases the series row.
	}
	if secondReadBeforeRelease {
		if err := <-metaDone; err != nil {
			t.Fatal(err)
		}
	}
	close(releaseFirst)
	if err := <-nameDone; err != nil {
		t.Fatal(err)
	}
	if !secondReadBeforeRelease {
		if err := <-metaDone; err != nil {
			t.Fatal(err)
		}
	}

	var stored models.Series
	if err := ctx.db.First(&stored, series.ID).Error; err != nil {
		t.Fatal(err)
	}
	var seriesMeta map[string]any
	if err := json.Unmarshal(stored.Meta, &seriesMeta); err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Renamed" || seriesMeta["version"] != float64(1) {
		t.Fatalf("concurrent patches lost an omitted field: name=%q meta=%s", stored.Name, stored.Meta)
	}
	var storedResource models.Resource
	if err := ctx.db.First(&storedResource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	var resourceMeta map[string]any
	if err := json.Unmarshal(storedResource.Meta, &resourceMeta); err != nil {
		t.Fatal(err)
	}
	if resourceMeta["version"] != float64(1) {
		t.Fatalf("resource effective metadata diverged from series: %s", storedResource.Meta)
	}
}
