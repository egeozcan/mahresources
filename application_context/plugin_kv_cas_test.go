//go:build json1 && fts5

package application_context

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
)

// mah.kv.set is an unconditional upsert, so read-modify-write over a stored
// value holds only while nothing writes the key in between. One plugin's Lua is
// single-threaded within a process, which covers a read and a write inside one
// call and nothing wider: a mah.start_job function or a drained mah.http
// callback writes in a later call, a second process is not ordered against the
// first at all, and a VM pool would remove even the single-call hold. These
// tests pin a compare-and-set whose compare happens in the database rather than
// in Go: a read followed by a write reintroduces exactly the lost update the
// operation exists to close, and only the concurrent cases below can tell the
// two implementations apart.

// kvCompareAndSetter is the contract this work adds to the context.
//
// Asserted rather than called directly so the package still compiles before the
// method exists. A direct call would fail the whole package at build time, and
// a build failure reports nothing per test: the docs and message-format pins
// below would never run, and the red would say one thing instead of naming each
// property that is missing.
type kvCompareAndSetter interface {
	PluginKVCompareAndSet(pluginName, key string, expected *string, value string) (bool, error)
}

func kvCAS(t *testing.T, ctx *MahresourcesContext) kvCompareAndSetter {
	t.Helper()
	cas, ok := any(ctx).(kvCompareAndSetter)
	if !ok {
		t.Fatalf("(*MahresourcesContext).PluginKVCompareAndSet(pluginName, key string, " +
			"expected *string, value string) (bool, error) is not implemented")
	}
	return cas
}

// expect is "the stored value must be exactly this". Its absence -- a nil
// *string -- is the other expectation, "no row may exist yet", which is a
// different state from a row holding JSON null.
func expect(s string) *string { return &s }

// kvValueCapBytes is the cap PluginKVSet enforces, written out rather than read
// from maxKVValueSize so the test still names the number if the constant moves
// to somewhere both layers can see it. A documented limit that changes silently
// is the thing being closed here, so a change to it should fail this.
const kvValueCapBytes = 8 * 1024 * 1024

// kvTestPlugin gives each test its own namespace in the package-wide in-memory
// database, and takes it away again afterwards.
func kvTestPlugin(t *testing.T, ctx *MahresourcesContext) string {
	t.Helper()
	name := "cas:" + t.Name()
	t.Cleanup(func() { _ = ctx.PluginKVPurge(name) })
	return name
}

func kvValue(t *testing.T, ctx *MahresourcesContext, plugin, key string) (string, bool) {
	t.Helper()
	val, found, err := ctx.PluginKVGet(plugin, key)
	if err != nil {
		t.Fatalf("PluginKVGet(%q, %q): %v", plugin, key, err)
	}
	return val, found
}

// The four quadrants of the compare. Each one also asserts what the store looks
// like afterwards, because "returned false" and "wrote nothing" are separate
// claims and a CAS is only useful when both hold.
func TestPluginKVCompareAndSet_Quadrants(t *testing.T) {
	t.Run("absent expected, key missing: writes", func(t *testing.T) {
		ctx := createTestContext(t)
		plugin := kvTestPlugin(t, ctx)
		cas := kvCAS(t, ctx)

		ok, err := cas.PluginKVCompareAndSet(plugin, "k", nil, `"first"`)
		if err != nil {
			t.Fatalf("compare-and-set: %v", err)
		}
		if !ok {
			t.Errorf("compare-and-set from absent on a missing key = false, want true")
		}
		if val, found := kvValue(t, ctx, plugin, "k"); !found || val != `"first"` {
			t.Errorf("stored value = %q (found=%v), want %q", val, found, `"first"`)
		}
	})

	t.Run("absent expected, key present: refuses", func(t *testing.T) {
		ctx := createTestContext(t)
		plugin := kvTestPlugin(t, ctx)
		cas := kvCAS(t, ctx)

		if err := ctx.PluginKVSet(plugin, "k", `"mine"`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		ok, err := cas.PluginKVCompareAndSet(plugin, "k", nil, `"theirs"`)
		if err != nil {
			t.Fatalf("compare-and-set: %v", err)
		}
		if ok {
			t.Errorf("compare-and-set from absent onto an existing key = true, want false")
		}
		if val, _ := kvValue(t, ctx, plugin, "k"); val != `"mine"` {
			t.Errorf("stored value = %q, want %q: a refused compare-and-set must not write", val, `"mine"`)
		}
	})

	t.Run("current value expected: writes", func(t *testing.T) {
		ctx := createTestContext(t)
		plugin := kvTestPlugin(t, ctx)
		cas := kvCAS(t, ctx)

		if err := ctx.PluginKVSet(plugin, "k", `1`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		ok, err := cas.PluginKVCompareAndSet(plugin, "k", expect(`1`), `2`)
		if err != nil {
			t.Fatalf("compare-and-set: %v", err)
		}
		if !ok {
			t.Errorf("compare-and-set from the current value = false, want true")
		}
		if val, _ := kvValue(t, ctx, plugin, "k"); val != `2` {
			t.Errorf("stored value = %q, want %q", val, `2`)
		}
	})

	t.Run("stale value expected: refuses", func(t *testing.T) {
		ctx := createTestContext(t)
		plugin := kvTestPlugin(t, ctx)
		cas := kvCAS(t, ctx)

		if err := ctx.PluginKVSet(plugin, "k", `1`); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := ctx.PluginKVSet(plugin, "k", `2`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		ok, err := cas.PluginKVCompareAndSet(plugin, "k", expect(`1`), `3`)
		if err != nil {
			t.Fatalf("compare-and-set: %v", err)
		}
		if ok {
			t.Errorf("compare-and-set from a stale value = true, want false")
		}
		if val, _ := kvValue(t, ctx, plugin, "k"); val != `2` {
			t.Errorf("stored value = %q, want %q", val, `2`)
		}
	})

	// The case an upsert gets wrong. A conditional written as "insert on
	// conflict update where value = expected" inserts happily when there is no
	// conflict, so the caller is told it replaced a value that was never there.
	t.Run("value expected, key missing: refuses and creates nothing", func(t *testing.T) {
		ctx := createTestContext(t)
		plugin := kvTestPlugin(t, ctx)
		cas := kvCAS(t, ctx)

		ok, err := cas.PluginKVCompareAndSet(plugin, "gone", expect(`"was here"`), `"back"`)
		if err != nil {
			t.Fatalf("compare-and-set: %v", err)
		}
		if ok {
			t.Errorf("compare-and-set against a missing key = true, want false")
		}
		if val, found := kvValue(t, ctx, plugin, "gone"); found {
			t.Errorf("a refused compare-and-set created the row anyway, holding %q", val)
		}
	})
}

// "The key does not exist" and "the key holds null" are different states and an
// author needs to say which one it is expecting. Both directions matter: the
// asymmetry is only closed when neither expectation is satisfied by the other's
// state.
func TestPluginKVCompareAndSet_AbsentIsNotStoredNull(t *testing.T) {
	ctx := createTestContext(t)
	plugin := kvTestPlugin(t, ctx)
	cas := kvCAS(t, ctx)

	// mah.kv.set(key, nil) serializes to JSON null, so this row exists and
	// holds null.
	if err := ctx.PluginKVSet(plugin, "nulled", `null`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ok, err := cas.PluginKVCompareAndSet(plugin, "nulled", nil, `1`)
	if err != nil {
		t.Fatalf("compare-and-set: %v", err)
	}
	if ok {
		t.Errorf("expecting absent matched a key holding null: a stored null is a value, not a missing row")
	}
	if val, _ := kvValue(t, ctx, plugin, "nulled"); val != `null` {
		t.Errorf("stored value = %q, want %q", val, `null`)
	}

	ok, err = cas.PluginKVCompareAndSet(plugin, "nulled", expect(`null`), `1`)
	if err != nil {
		t.Fatalf("compare-and-set: %v", err)
	}
	if !ok {
		t.Errorf("expecting null against a key holding null = false, want true")
	}

	// And the mirror: expecting null must not match a row that is not there.
	ok, err = cas.PluginKVCompareAndSet(plugin, "missing", expect(`null`), `1`)
	if err != nil {
		t.Fatalf("compare-and-set: %v", err)
	}
	if ok {
		t.Errorf("expecting null matched a missing key: a missing row holds nothing, not null")
	}
	if _, found := kvValue(t, ctx, plugin, "missing"); found {
		t.Errorf("expecting null against a missing key created it")
	}
}

// The compare is scoped by plugin for the same reason every other kv operation
// is. A conditional statement that forgot the plugin_name predicate would read
// as correct in every single-plugin test.
//
// So every compare below has to contest a row another plugin really holds. A
// compare aimed at a key nobody holds matches nothing with the predicate and
// nothing without it, which is a passing assertion that rules out no
// implementation at all -- and the two failures being ruled out are cross-plugin
// data isolation, on the one write path that can reach two plugins' rows in a
// single statement.
func TestPluginKVCompareAndSet_IsPerPlugin(t *testing.T) {
	ctx := createTestContext(t)
	mine := kvTestPlugin(t, ctx) + ":a"
	theirs := kvTestPlugin(t, ctx) + ":b"
	t.Cleanup(func() {
		_ = ctx.PluginKVPurge(mine)
		_ = ctx.PluginKVPurge(theirs)
	})
	cas := kvCAS(t, ctx)

	if err := ctx.PluginKVSet(theirs, "shared", `"theirs"`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Another plugin holding this key does not make it present for me.
	ok, err := cas.PluginKVCompareAndSet(mine, "shared", nil, `"mine"`)
	if err != nil {
		t.Fatalf("compare-and-set: %v", err)
	}
	if !ok {
		t.Errorf("compare-and-set from absent = false, want true: another plugin's key of the "+
			"same name (%q) is not mine", "shared")
	}
	if val, _ := kvValue(t, ctx, theirs, "shared"); val != `"theirs"` {
		t.Errorf("the other plugin's value became %q, want %q", val, `"theirs"`)
	}

	// Nor can I compare against what they hold. The expectation names the value
	// sitting in their row and not in mine, so a compare that dropped the plugin
	// matches theirs, reports the write to me, and leaves their key holding what
	// I put there.
	ok, err = cas.PluginKVCompareAndSet(mine, "shared", expect(`"theirs"`), `"stolen"`)
	if err != nil {
		t.Fatalf("compare-and-set: %v", err)
	}
	if ok {
		t.Errorf("compare-and-set matched a value that only another plugin holds")
	}
	if val, _ := kvValue(t, ctx, theirs, "shared"); val != `"theirs"` {
		t.Errorf("the other plugin's value became %q, want %q: the compare wrote across plugins",
			val, `"theirs"`)
	}
	if val, _ := kvValue(t, ctx, mine, "shared"); val != `"mine"` {
		t.Errorf("stored value = %q, want %q: a refused compare-and-set must not write", val, `"mine"`)
	}

	// The other half, and the one the return value alone cannot describe. When
	// both plugins hold the same key at the same value, a scoped compare matches
	// the one row that is mine; an unscoped one matches two, and two is not one,
	// so RowsAffected reports a refusal to a caller whose write has already
	// landed in someone else's row.
	if err := ctx.PluginKVSet(theirs, "same", `1`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := ctx.PluginKVSet(mine, "same", `1`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ok, err = cas.PluginKVCompareAndSet(mine, "same", expect(`1`), `2`)
	if err != nil {
		t.Fatalf("compare-and-set: %v", err)
	}
	if !ok {
		t.Errorf("compare-and-set of my own row = false, want true: another plugin holding the " +
			"same key at the same value is not a second row of mine")
	}
	if val, _ := kvValue(t, ctx, mine, "same"); val != `2` {
		t.Errorf("stored value = %q, want %q", val, `2`)
	}
	if val, _ := kvValue(t, ctx, theirs, "same"); val != `1` {
		t.Errorf("the other plugin's value became %q, want %q: the compare wrote across plugins",
			val, `1`)
	}
}

// The 8MB cap applies to whatever writes a value, which now includes the
// compare-and-set. Refusing after the compare has already replaced the row
// would be worse than not having the cap.
func TestPluginKVCompareAndSet_RefusesAnOversizedValue(t *testing.T) {
	ctx := createTestContext(t)
	plugin := kvTestPlugin(t, ctx)
	cas := kvCAS(t, ctx)

	if err := ctx.PluginKVSet(plugin, "k", `"small"`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	oversized := strings.Repeat("x", kvValueCapBytes+1)
	ok, err := cas.PluginKVCompareAndSet(plugin, "k", expect(`"small"`), oversized)
	if err == nil {
		t.Fatalf("compare-and-set with a %d byte value returned no error", len(oversized))
	}
	if ok {
		t.Errorf("compare-and-set reported a write it refused")
	}
	// The author has to be able to see what happened, which means both numbers.
	msg := err.Error()
	if !strings.Contains(msg, fmt.Sprint(len(oversized))) || !strings.Contains(msg, fmt.Sprint(kvValueCapBytes)) {
		t.Errorf("error %q names neither the actual size (%d) nor the limit (%d)",
			msg, len(oversized), kvValueCapBytes)
	}
	if val, _ := kvValue(t, ctx, plugin, "k"); val != `"small"` {
		t.Errorf("stored value = %q, want %q: the refused write landed anyway", val, `"small"`)
	}
}

// The boundary itself. len(value) > max is refused, so exactly the limit is a
// legal value and an off-by-one that rejected it would be a silent narrowing of
// a documented number.
func TestPluginKVCompareAndSet_AcceptsExactlyTheLimit(t *testing.T) {
	ctx := createTestContext(t)
	plugin := kvTestPlugin(t, ctx)
	cas := kvCAS(t, ctx)

	atLimit := strings.Repeat("y", kvValueCapBytes)
	ok, err := cas.PluginKVCompareAndSet(plugin, "big", nil, atLimit)
	if err != nil {
		t.Fatalf("compare-and-set with a value of exactly %d bytes: %v", kvValueCapBytes, err)
	}
	if !ok {
		t.Errorf("compare-and-set of a value at exactly the limit = false, want true")
	}
	if val, _ := kvValue(t, ctx, plugin, "big"); len(val) != kvValueCapBytes {
		t.Errorf("stored %d bytes, want %d", len(val), kvValueCapBytes)
	}
}

// mah.kv.set raises on failure, so the message is all the author gets. It has
// named both numbers since the cap was added; this pins that, because the item
// asks for the message to stay that way while the cap becomes documented.
func TestPluginKVSet_OversizedValueNamesBothSizes(t *testing.T) {
	ctx := createTestContext(t)
	plugin := kvTestPlugin(t, ctx)

	oversized := strings.Repeat("x", kvValueCapBytes+1)
	err := ctx.PluginKVSet(plugin, "k", oversized)
	if err == nil {
		t.Fatalf("PluginKVSet with a %d byte value returned no error", len(oversized))
	}
	msg := err.Error()
	if !strings.Contains(msg, fmt.Sprint(len(oversized))) {
		t.Errorf("error %q does not name the actual size (%d)", msg, len(oversized))
	}
	if !strings.Contains(msg, fmt.Sprint(kvValueCapBytes)) {
		t.Errorf("error %q does not name the limit (%d)", msg, kvValueCapBytes)
	}
}

// newKVRaceContext opens a temp-file SQLite database (WAL, busy_timeout) whose
// pool hands out several connections, because that is the only shape in which
// two writers of one row really do run at once. The package's shared in-memory
// handle cannot show this.
func newKVRaceContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kv.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.PluginKV{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(4)
	return NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
}

// Exactly one writer wins, on both paths into the row.
//
// The insert path (everyone expects absent) and the update path (everyone
// expects the same seeded value) fail differently: an implementation that reads
// in Go and then writes lets several writers agree the row is absent and all
// insert -- or, with the upsert already there, all report success while only
// the last value survives.
func TestPluginKVCompareAndSet_ExactlyOneWriterWins(t *testing.T) {
	const writers = 8

	run := func(t *testing.T, seed *string) {
		t.Helper()
		ctx := newKVRaceContext(t)
		cas := kvCAS(t, ctx)
		const plugin, key = "racer", "contested"

		winners := kvWriterRace(t, ctx, cas, plugin, key, seed, writers)
		if len(winners) != 1 {
			t.Fatalf("%d of %d writers were told they wrote, want exactly 1: %v",
				len(winners), writers, winners)
		}
		val, found := kvValue(t, ctx, plugin, key)
		if !found {
			t.Fatalf("no row after %d compare-and-sets, one of which reported success", writers)
		}
		if val != winners[0] {
			t.Errorf("stored value = %q but %q was told it won", val, winners[0])
		}
	}

	t.Run("from absent", func(t *testing.T) { run(t, nil) })
	t.Run("from a value", func(t *testing.T) { run(t, expect(`"seed"`)) })
}

// The lost update itself, on SQLite. Postgres runs the same harness in
// TestPluginKVCompareAndSetPG_NoLostUpdate, because the statement is where the
// two databases differ and this is the property that depends on it.
func TestPluginKVCompareAndSet_NoLostUpdate(t *testing.T) {
	ctx := newKVRaceContext(t)
	kvIncrementRace(t, ctx, kvCAS(t, ctx), "counter", "n", 6, 20)
}
