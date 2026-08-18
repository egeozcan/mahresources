//go:build postgres

package application_context

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"

	"mahresources/constants"
	"mahresources/internal/testpgutil"
	"mahresources/models"
)

// The one thing about mah.db.transaction that behaves differently on the two
// supported databases, and the one whose failure mode would be silent.
//
// Postgres marks a transaction aborted after any failed statement. mah.db hands
// write failures to Lua as return values rather than raising, so a plugin can
// ignore one and keep writing into a transaction that can no longer commit —
// and Postgres answers COMMIT on an aborted transaction with a ROLLBACK. A
// driver that passed that through would report a successful commit that wrote
// nothing.
//
// pgx does not pass it through ("commit unexpectedly resulted in rollback"), so
// this currently holds for free. That is exactly why it is worth a test: no
// code in this repo enforces it, and no SQLite test can, because SQLite does
// not poison a transaction at all. The assertion is deliberately on the
// property rather than on a message — what must never happen is a report and a
// database that disagree.

var pgContainer *testpgutil.Container

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	pgContainer, err = testpgutil.StartContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	pgContainer.Stop(ctx)
	os.Exit(code)
}

func newPostgresPluginContext(t *testing.T, sources map[string]string) *MahresourcesContext {
	t.Helper()

	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(
		&models.Resource{}, &models.Note{}, &models.Tag{}, &models.Group{},
		&models.Category{}, &models.NoteType{}, &models.ResourceCategory{},
		&models.Series{}, &models.Preview{}, &models.GroupRelation{},
		&models.GroupRelationType{}, &models.ImageHash{}, &models.ResourceSimilarity{},
		&models.ResourceVersion{}, &models.NoteBlock{}, &models.LogEntry{},
		&models.PluginKV{}, &models.PluginState{}, &models.User{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pluginDir := t.TempDir()
	for name, src := range sources {
		if err := os.MkdirAll(pluginDir+"/"+name, 0o755); err != nil {
			t.Fatalf("mkdir plugin %s: %v", name, err)
		}
		if err := os.WriteFile(pluginDir+"/"+name+"/plugin.lua", []byte(src), 0o644); err != nil {
			t.Fatalf("write plugin %s: %v", name, err)
		}
	}

	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open read-only handle: %v", err)
	}
	t.Cleanup(func() { readOnly.Close() })

	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{
		DbType:     constants.DbTypePosgres,
		PluginPath: pluginDir,
	})
	pm := ctx.PluginManager()
	if pm == nil {
		t.Fatal("plugin manager was not wired")
	}
	for name := range sources {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin %s: %v", name, err)
		}
	}
	t.Cleanup(pm.Close)
	return ctx
}

// The nested half, and the one nothing else covers. A savepoint is released by
// GORM doing nothing when the callback returns nil, so there is no commit for
// pgx to check — an inner block that ignored a failed write would be told `true`
// while the outer transaction is already doomed, and a plugin can act on that
// answer (mah.start_job does not join the transaction) before it fails.
func TestPluginTransactionPG_NestedTransactionDoesNotReportAFalseSuccess(t *testing.T) {
	const src = `plugin = { name = "nestpoisoner", version = "1.0", description = "ignores a failed write inside a nested transaction" }
function init()
    mah.inject("page_bottom", function(ctx)
        local innerOk, innerErr
        local outerOk, outerErr = mah.db.transaction(function()
            innerOk, innerErr = mah.db.transaction(function()
                mah.db.create_tag({ name = "duplicate" })
                mah.db.create_tag({ name = "later-tag" })
            end)
        end)
        return "inner=" .. tostring(innerOk) .. " outer=" .. tostring(outerOk)
    end)
end
`
	ctx := newPostgresPluginContext(t, map[string]string{"nestpoisoner": src})

	if err := ctx.db.Create(&models.Tag{Name: "duplicate"}).Error; err != nil {
		t.Fatalf("seed duplicate tag: %v", err)
	}

	got := ctx.PluginManager().RenderSlot(context.Background(), "page_bottom", map[string]any{}, nil)
	t.Logf("slot said: %q", got)

	if strings.Contains(got, "inner=true") {
		t.Errorf("the nested transaction reported success (%q) after a write that failed and "+
			"left the transaction unable to commit: a savepoint release has no commit for the "+
			"driver to check, so nothing but the probe catches it", got)
	}

	// The outer one committing is correct and worth stating: rolling back to the
	// savepoint clears Postgres's aborted state, so the outer transaction is
	// usable again and commits honestly — with nothing in it, because the only
	// writes attempted were inside the block that rolled back. That is exactly
	// what a savepoint is for, and it is why the inner failure must be reported
	// rather than swallowed: the plugin is the one that decides whether to carry
	// on.
	if !strings.Contains(got, "outer=true") {
		t.Errorf("the outer transaction reported %q: an inner rollback should leave it usable", got)
	}

	var later int64
	if err := ctx.db.Model(&models.Tag{}).Where("name = ?", "later-tag").Count(&later).Error; err != nil {
		t.Fatalf("count later-tag: %v", err)
	}
	if later != 0 {
		t.Errorf("later-tag rows = %d, want 0", later)
	}
}

func TestPluginTransactionPG_PoisonedTransactionIsNotReportedAsCommitted(t *testing.T) {
	// The first write fails on a duplicate name; the plugin ignores the error
	// exactly as an inattentive author would, and keeps writing.
	const src = `plugin = { name = "poisoner", version = "1.0", description = "ignores a failed write" }
function init()
    mah.inject("page_bottom", function(ctx)
        local ok, err = mah.db.transaction(function()
            mah.db.create_tag({ name = "duplicate" })
            mah.db.create_tag({ name = "later-tag" })
        end)
        return tostring(ok) .. ":" .. tostring(err)
    end)
end
`
	ctx := newPostgresPluginContext(t, map[string]string{"poisoner": src})

	// Seed the collision the first write inside the transaction will hit.
	if err := ctx.db.Create(&models.Tag{Name: "duplicate"}).Error; err != nil {
		t.Fatalf("seed duplicate tag: %v", err)
	}

	got := ctx.PluginManager().RenderSlot(context.Background(), "page_bottom", map[string]any{}, nil)

	var later int64
	if err := ctx.db.Model(&models.Tag{}).Where("name = ?", "later-tag").Count(&later).Error; err != nil {
		t.Fatalf("count later-tag: %v", err)
	}

	// Whatever it reports, report and reality must agree.
	t.Logf("slot said: %q; later-tag rows: %d", got, later)
	committed := len(got) >= 4 && got[:4] == "true"
	if committed && later == 0 {
		t.Errorf("the transaction reported success (%q) but wrote nothing: Postgres answered "+
			"COMMIT on an aborted transaction with a silent ROLLBACK", got)
	}
	if !committed && later != 0 {
		t.Errorf("the transaction reported failure (%q) but left %d row(s) behind", got, later)
	}
}
