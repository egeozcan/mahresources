//go:build postgres && json1 && fts5

package application_context

import (
	"encoding/json"
	"strings"
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

// Engine parity for the mass edit: the same op table run against Postgres,
// focused on the two places the dialects can drift — the tag replace with an
// empty list, and the meta column's JSON result shape. Each test asserts on
// decoded JSON (map equality), not on the stored bytes, because jsonb
// normalises whitespace and key order.
//
// The SQLite arms of these properties live in mass_edit_context_test.go.

func newMassEditPGContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx, _ := newMassEditPGContextWithDSN(t)
	return ctx
}

func newMassEditPGContextWithDSN(t *testing.T) (*MahresourcesContext, string) {
	t.Helper()
	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(
		&models.Resource{}, &models.Group{}, &models.Tag{}, &models.Note{},
		&models.Category{}, &models.ResourceCategory{}, &models.Series{},
		&models.ResourceVersion{}, &models.Preview{}, &models.NoteType{},
		&models.GroupRelation{}, &models.GroupRelationType{}, &models.ImageHash{},
		&models.ResourceSimilarity{}, &models.LogEntry{}, &models.NoteBlock{},
		&models.PluginKV{}, &models.Query{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open read-only handle: %v", err)
	}
	t.Cleanup(func() { readOnly.Close() })

	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{
		DbType: constants.DbTypePosgres,
	})
	defaultRC := &models.ResourceCategory{Name: "Default"}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)
	return ctx, dsn
}

type massEditPGFixture struct {
	ctx  *MahresourcesContext
	tag  *models.Tag
	tag2 *models.Tag
	res  []*models.Resource
}

func newMassEditPGFixture(t *testing.T, name string, n int) *massEditPGFixture {
	t.Helper()
	ctx := newMassEditPGContext(t)

	tag := &models.Tag{Name: name + "-tag"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	tag2 := &models.Tag{Name: name + "-tag2"}
	if err := ctx.db.Create(tag2).Error; err != nil {
		t.Fatalf("create second tag: %v", err)
	}
	resources := make([]*models.Resource, 0, n)
	for i := 0; i < n; i++ {
		r := &models.Resource{Name: name + "-res"}
		if err := ctx.db.Create(r).Error; err != nil {
			t.Fatalf("create resource: %v", err)
		}
		resources = append(resources, r)
	}
	return &massEditPGFixture{ctx: ctx, tag: tag, tag2: tag2, res: resources}
}

func (f *massEditPGFixture) ids() []uint {
	ids := make([]uint, 0, len(f.res))
	for _, r := range f.res {
		ids = append(ids, r.ID)
	}
	return ids
}

func pgResourceTagIDs(t *testing.T, ctx *MahresourcesContext, id uint) []uint {
	t.Helper()
	var ids []uint
	if err := ctx.db.Raw("SELECT tag_id FROM resource_tags WHERE resource_id = ? ORDER BY tag_id", id).Scan(&ids).Error; err != nil {
		t.Fatalf("read tags: %v", err)
	}
	return ids
}

func pgResourceMeta(t *testing.T, ctx *MahresourcesContext, id uint, column string) map[string]any {
	t.Helper()
	var raw []byte
	if err := ctx.db.Raw("SELECT "+column+" FROM resources WHERE id = ?", id).Row().Scan(&raw); err != nil {
		t.Fatalf("read %s: %v", column, err)
	}
	out := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal %s %s: %v", column, raw, err)
		}
	}
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func TestMassEditPGTagsReplaceWithEmptyListClearsAll(t *testing.T) {
	f := newMassEditPGFixture(t, "pgclear", 2)
	for _, r := range f.res {
		if err := f.ctx.db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", r.ID, f.tag.ID).Error; err != nil {
			t.Fatalf("tag resource: %v", err)
		}
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     f.ids(),
		TagsOp: "replace", // no TagIds: clear all
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	for i, r := range f.res {
		if got := pgResourceTagIDs(t, f.ctx, r.ID); len(got) != 0 {
			t.Errorf("resource %d still has tags %v — the NOT IN () trap reproduced on Postgres", i, got)
		}
	}
}

func TestMassEditPGTagsReplaceGivesExactSetEquality(t *testing.T) {
	f := newMassEditPGFixture(t, "pgreplacetags", 2)
	for _, r := range f.res {
		if err := f.ctx.db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", r.ID, f.tag.ID).Error; err != nil {
			t.Fatalf("tag resource: %v", err)
		}
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     f.ids(),
		TagsOp: "replace",
		TagIds: []uint{f.tag2.ID},
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	for i, r := range f.res {
		if got := pgResourceTagIDs(t, f.ctx, r.ID); len(got) != 1 || got[0] != f.tag2.ID {
			t.Errorf("resource %d has tags %v, want exactly [%d]", i, got, f.tag2.ID)
		}
	}
}

func TestMassEditPGOwnerSetCommits(t *testing.T) {
	f := newMassEditPGFixture(t, "pgowner", 2)
	owner := &models.Group{Name: "pg-owner"}
	if err := f.ctx.db.Create(owner).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	result, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:      f.ids(),
		OwnerOp: "set",
		OwnerId: owner.ID,
	})
	if err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	if result.Ops[0].RowsAffected != 2 {
		t.Errorf("rowsAffected=%d, want 2", result.Ops[0].RowsAffected)
	}
	for i, r := range f.res {
		var fresh models.Resource
		if err := f.ctx.db.First(&fresh, r.ID).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		if fresh.OwnerId == nil || *fresh.OwnerId != owner.ID {
			t.Errorf("resource %d owner = %v, want %d", i, fresh.OwnerId, owner.ID)
		}
	}
}

func TestMassEditPGRelatedGroupsDropsSelfEdge(t *testing.T) {
	f := newMassEditPGFixture(t, "pgself", 0)
	g := &models.Group{Name: "pg-self-group"}
	if err := f.ctx.db.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	other := &models.Group{Name: "pg-other-group"}
	if err := f.ctx.db.Create(other).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	if _, err := f.ctx.MassEditGroups(&query_models.MassEditQuery{
		ID:              []uint{g.ID},
		RelatedGroupsOp: "add",
		RelatedGroupIds: []uint{g.ID, other.ID},
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	var selfEdges int64
	if err := f.ctx.db.Raw("SELECT COUNT(*) FROM group_related_groups WHERE group_id = ? AND related_group_id = ?", g.ID, g.ID).Scan(&selfEdges).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if selfEdges != 0 {
		t.Errorf("a self-edge was produced on Postgres (%d rows)", selfEdges)
	}
	var otherEdges int64
	if err := f.ctx.db.Raw("SELECT COUNT(*) FROM group_related_groups WHERE group_id = ? AND related_group_id = ?", g.ID, other.ID).Scan(&otherEdges).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if otherEdges != 1 {
		t.Errorf("the relation to the other group is missing (%d rows)", otherEdges)
	}
}

func TestMassEditPGMetaMergeIsJSONEquivalentToSQLite(t *testing.T) {
	f := newMassEditPGFixture(t, "pgmerge", 1)
	r := f.res[0]
	if err := f.ctx.db.Model(r).Update("meta", `{"keep":1,"gone":2}`).Error; err != nil {
		t.Fatalf("seed meta: %v", err)
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{r.ID},
		MetaOp: "merge",
		Meta:   `{"added":true,"gone":null}`,
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	meta := pgResourceMeta(t, f.ctx, r.ID, "meta")
	if meta["added"] != true || meta["keep"] != float64(1) {
		t.Errorf("meta = %v", meta)
	}
	if _, present := meta["gone"]; present {
		t.Error("the merged null did not delete the key on Postgres")
	}
	if _, present := pgResourceMeta(t, f.ctx, r.ID, "own_meta")["added"]; !present {
		t.Error("own_meta did not receive the patch on Postgres")
	}
}

func TestMassEditPGMetaMergeNullOnSeriesSuppresses(t *testing.T) {
	f := newMassEditPGFixture(t, "pgsupp", 1)
	r := f.res[0]

	series := &models.Series{
		Name: "pg-series",
		Slug: "pg-series",
		Meta: types.JSON(`{"k":"from-series","other":"series-value"}`),
	}
	if err := f.ctx.db.Create(series).Error; err != nil {
		t.Fatalf("create series: %v", err)
	}
	if err := f.ctx.db.Model(r).Updates(map[string]any{
		"series_id": series.ID,
		"meta":      `{"k":"override"}`,
		"own_meta":  `{"k":"override"}`,
	}).Error; err != nil {
		t.Fatalf("assign series: %v", err)
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{r.ID},
		MetaOp: "merge",
		Meta:   `{"k":null}`,
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	if _, present := pgResourceMeta(t, f.ctx, r.ID, "meta")["k"]; present {
		t.Error("k still present in meta on Postgres")
	}
	own := pgResourceMeta(t, f.ctx, r.ID, "own_meta")
	if v, present := own["k"]; !present || v != nil {
		t.Errorf("own_meta[k] = %v (present=%v), want an explicit null", v, present)
	}
}

func TestMassEditPGMetaRemoveKeysOnSeriesSuppressesReinheritance(t *testing.T) {
	f := newMassEditPGFixture(t, "pgrm", 1)
	r := f.res[0]

	series := &models.Series{Name: "pg-rm-series", Slug: "pg-rm-series", Meta: types.JSON(`{"k":"from-series"}`)}
	if err := f.ctx.db.Create(series).Error; err != nil {
		t.Fatalf("create series: %v", err)
	}
	if err := f.ctx.db.Model(r).Updates(map[string]any{
		"series_id": series.ID,
		"meta":      `{"k":"v","z":"zv"}`,
		"own_meta":  `{"k":"v","z":"zv"}`,
	}).Error; err != nil {
		t.Fatalf("assign series: %v", err)
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:       []uint{r.ID},
		MetaOp:   "removeKeys",
		MetaKeys: []string{"k"},
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	if _, present := pgResourceMeta(t, f.ctx, r.ID, "meta")["k"]; present {
		t.Error("k still in effective meta on Postgres")
	}
	own := pgResourceMeta(t, f.ctx, r.ID, "own_meta")
	if v, present := own["k"]; !present || v != nil {
		t.Errorf("own_meta[k] = %v (present=%v), want an explicit null so the series does not re-inherit", v, present)
	}
}

func TestMassEditPGMetaReplaceOnSeriesComputesOwnMeta(t *testing.T) {
	f := newMassEditPGFixture(t, "pgrep", 1)
	r := f.res[0]

	series := &models.Series{
		Name: "pg-rep-series",
		Slug: "pg-rep-series",
		Meta: types.JSON(`{"k":"from-series","other":"series-value"}`),
	}
	if err := f.ctx.db.Create(series).Error; err != nil {
		t.Fatalf("create series: %v", err)
	}
	if err := f.ctx.db.Model(r).Update("series_id", series.ID).Error; err != nil {
		t.Fatalf("assign series: %v", err)
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{r.ID},
		MetaOp: "replace",
		Meta:   `{"k":"new-value"}`,
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	want, err := computeOwnMeta(types.JSON(`{"k":"new-value"}`), series.Meta, true)
	if err != nil {
		t.Fatalf("computeOwnMeta: %v", err)
	}
	got := pgResourceMeta(t, f.ctx, r.ID, "own_meta")
	expected := map[string]any{}
	_ = json.Unmarshal(want, &expected)
	if len(got) != len(expected) {
		t.Fatalf("own_meta = %v, want %v", got, expected)
	}
	if k, present := got["k"]; !present || k != "new-value" {
		t.Errorf("own_meta[k] = %v, want the submitted override", k)
	}
	if o, present := got["other"]; !present || o != nil {
		t.Errorf("own_meta[other] = %v, want the recorded removal", o)
	}
	if meta := pgResourceMeta(t, f.ctx, r.ID, "meta"); meta["k"] != "new-value" {
		t.Errorf("meta = %v, want the submitted effective value", meta)
	}
}

func TestMassEditPGMetaMergeOntoSQLNULL(t *testing.T) {
	f := newMassEditPGFixture(t, "pgnull", 1)
	if err := f.ctx.db.Model(f.res[0]).Update("meta", gorm.Expr("NULL")).Error; err != nil {
		t.Fatalf("null the meta: %v", err)
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{f.res[0].ID},
		MetaOp: "merge",
		Meta:   `{"a":1}`,
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	if meta := pgResourceMeta(t, f.ctx, f.res[0].ID, "meta"); meta["a"] != float64(1) {
		t.Errorf("merging onto a NULL meta lost the patch on Postgres: %v", meta)
	}
}

// A tag deleted between the far-endpoint validation and the join-table INSERT
// must not leave a dangling row behind. Postgres migrations create no foreign
// keys on the join tables, so only the row lock taken by the validation closes
// the window: the validation's SELECT ... FOR UPDATE blocks on the deleter's
// uncommitted DELETE, and after it commits the re-read sees the tag is gone and
// refuses the whole edit. Without the lock the validation's Count would see the
// committed (still-present) tag, the insert would proceed, and the commit would
// orphan the join row.
func TestMassEditPGLockRefusesTagDeletedMidFlight(t *testing.T) {
	ctx, dsn := newMassEditPGContextWithDSN(t)

	tag := &models.Tag{Name: "pglockrace-tag"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	res := &models.Resource{Name: "pglockrace-res"}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}

	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open racing connection: %v", err)
	}
	t.Cleanup(func() { readOnly.Close() })

	txB, err := readOnly.Beginx()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := txB.Exec("DELETE FROM tags WHERE id = $1", tag.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := ctx.MassEditResources(&query_models.MassEditQuery{
			ID:     []uint{res.ID},
			TagsOp: "add",
			TagIds: []uint{tag.ID},
		})
		errCh <- err
	}()

	// Wait until the mass edit's transaction is BLOCKED on the tag row (a lock
	// in pg_locks with granted = false), so the commit below lands while the
	// validation is genuinely in flight. Without the FOR UPDATE the mass edit
	// would sail through validation, and either the wait below times out or
	// the final assertions fail on the dangling row.
	blocked := false
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		select {
		case err := <-errCh:
			t.Fatalf("mass edit returned before ever blocking: %v", err)
		default:
		}
		// A FOR UPDATE row lock is a tuple/transaction lock, not a relation
		// lock, so pg_locks alone cannot see it. Look for the mass edit's own
		// validation statement in pg_stat_activity waiting on a lock — that is
		// exactly the blocked SELECT ... FOR UPDATE on tags.
		var waiting int64
		if err := readOnly.Get(&waiting, `
			SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock'
			  AND query ILIKE '%FROM "tags"%'
			  AND query ILIKE '%FOR UPDATE%'`); err != nil {
			t.Fatalf("poll pg_stat_activity: %v", err)
		}
		if waiting > 0 {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("the mass edit never blocked on the deleted tag row — the validation lock is missing")
	}
	if err := txB.Commit(); err != nil {
		t.Fatalf("commit deleter: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "one or more tags not found") {
			t.Fatalf("expected a not-found refusal after the concurrent delete, got %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the mass edit never returned")
	}

	var dangling int64
	if err := ctx.db.Raw("SELECT COUNT(*) FROM resource_tags WHERE tag_id = ?", tag.ID).Scan(&dangling).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if dangling != 0 {
		t.Fatalf("a join row outlived the tag it names: %d dangling rows", dangling)
	}
}

// Exact replacement must clear join rows whose far row is GONE. Postgres
// migrations create no join-table foreign keys, so a dangling edge survives the
// plain SELECT of live far endpoints; the replace's delete phase has to sweep
// it anyway, or "replace with nothing" quietly preserves corruption that even
// an unscoped delete would have removed.
func TestMassEditPGReplaceClearsDanglingJoinRows(t *testing.T) {
	f := newMassEditPGFixture(t, "pgdangling", 2)

	other := &models.Group{Name: "pg-dangling-other"}
	if err := f.ctx.db.Create(other).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	// One live edge, one dangling edge (the far group row is deleted outright).
	if err := f.ctx.db.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?)",
		f.res[0].ID, other.ID).Error; err != nil {
		t.Fatalf("seed live edge: %v", err)
	}
	danglingGroupID := other.ID + 424242
	if err := f.ctx.db.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?)",
		f.res[0].ID, danglingGroupID).Error; err != nil {
		t.Fatalf("seed dangling edge: %v", err)
	}

	// Replace with nothing: every edge on the targets must go, dangling included.
	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:       f.ids(),
		GroupsOp: "replace",
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	for i, r := range f.res {
		var edges int64
		if err := f.ctx.db.Raw("SELECT COUNT(*) FROM groups_related_resources WHERE resource_id = ?", r.ID).Scan(&edges).Error; err != nil {
			t.Fatalf("count edges: %v", err)
		}
		if edges != 0 {
			t.Fatalf("resource %d still has %d edges after replace-with-nothing", i, edges)
		}
	}
}

// The group-tree advisory lock serialises re-parents: while another
// transaction holds it, a mass edit's group owner op must not even begin its
// cycle walk. Without the protocol, a concurrent merge or group update could
// extend the owner's ancestry chain after the walk and before the write,
// completing a cycle the check approved.
func TestMassEditPGGroupReparentWaitsForTheTreeLock(t *testing.T) {
	ctx, dsn := newMassEditPGContextWithDSN(t)

	parent := &models.Group{Name: "pgtree-parent"}
	if err := ctx.db.Create(parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	target := &models.Group{Name: "pgtree-target"}
	if err := ctx.db.Create(target).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	child := &models.Group{Name: "pgtree-child", OwnerId: &parent.ID}
	if err := ctx.db.Create(child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}

	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open racing connection: %v", err)
	}
	t.Cleanup(func() { readOnly.Close() })

	txB, err := readOnly.Beginx()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := txB.Exec("SELECT pg_advisory_xact_lock($1)", groupTreeAdvisoryLockKey); err != nil {
		t.Fatalf("take tree lock: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := ctx.MassEditGroups(&query_models.MassEditQuery{
			ID:      []uint{target.ID},
			OwnerOp: "set",
			OwnerId: child.ID,
		})
		errCh <- err
	}()

	// Wait until the mass edit is blocked on the advisory lock, then release.
	blocked := false
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		select {
		case err := <-errCh:
			t.Fatalf("mass edit ran without waiting for the tree lock: %v", err)
		default:
		}
		var waiting int64
		if err := readOnly.Get(&waiting, `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted`); err != nil {
			t.Fatalf("poll pg_locks: %v", err)
		}
		if waiting > 0 {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("the group re-parent never blocked on the tree lock — the protocol is missing")
	}
	if err := txB.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("mass edit failed after the tree lock released: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the mass edit never returned")
	}

	var fresh models.Group
	if err := ctx.db.First(&fresh, target.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if fresh.OwnerId == nil || *fresh.OwnerId != child.ID {
		t.Errorf("target owner = %v, want %d", fresh.OwnerId, child.ID)
	}
}
