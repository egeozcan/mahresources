//go:build postgres && json1 && fts5

package application_context

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"

	"mahresources/constants"
	"mahresources/models"
)

// The compare has to be one conditional statement, and a statement is where the
// two supported databases differ. SQLite alone cannot show that: an upsert
// whose conflict target does not name the real unique index is accepted by
// SQLite's looser matching and refused by Postgres at run time, and a
// RowsAffected-based answer depends on the dialect actually reporting zero for
// the row it declined to touch.

func newPostgresKVContext(t *testing.T) *MahresourcesContext {
	t.Helper()

	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(&models.PluginKV{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open read-only handle: %v", err)
	}
	t.Cleanup(func() { readOnly.Close() })

	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{
		DbType: constants.DbTypePosgres,
	})
}

func TestPluginKVCompareAndSetPG_Quadrants(t *testing.T) {
	ctx := newPostgresKVContext(t)
	cas := kvCAS(t, ctx)
	const plugin = "pg-cas"

	// Absent onto a missing key writes; onto an existing key it does not.
	if ok, err := cas.PluginKVCompareAndSet(plugin, "k", nil, `"first"`); err != nil || !ok {
		t.Fatalf("compare-and-set from absent = (%v, %v), want (true, nil)", ok, err)
	}
	if ok, err := cas.PluginKVCompareAndSet(plugin, "k", nil, `"second"`); err != nil || ok {
		t.Fatalf("compare-and-set from absent onto an existing key = (%v, %v), want (false, nil)", ok, err)
	}

	// The value compare, both ways.
	if ok, err := cas.PluginKVCompareAndSet(plugin, "k", expect(`"first"`), `"second"`); err != nil || !ok {
		t.Fatalf("compare-and-set from the current value = (%v, %v), want (true, nil)", ok, err)
	}
	if ok, err := cas.PluginKVCompareAndSet(plugin, "k", expect(`"first"`), `"third"`); err != nil || ok {
		t.Fatalf("compare-and-set from a stale value = (%v, %v), want (false, nil)", ok, err)
	}
	if val, _ := kvValue(t, ctx, plugin, "k"); val != `"second"` {
		t.Errorf("stored value = %q, want %q", val, `"second"`)
	}

	// A value expectation must not create a row that was never there.
	if ok, err := cas.PluginKVCompareAndSet(plugin, "gone", expect(`"was here"`), `"back"`); err != nil || ok {
		t.Fatalf("compare-and-set against a missing key = (%v, %v), want (false, nil)", ok, err)
	}
	if _, found := kvValue(t, ctx, plugin, "gone"); found {
		t.Errorf("a refused compare-and-set created the row")
	}

	// A stored null is a value, not a missing row.
	if err := ctx.PluginKVSet(plugin, "nulled", `null`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if ok, err := cas.PluginKVCompareAndSet(plugin, "nulled", nil, `1`); err != nil || ok {
		t.Fatalf("expecting absent against a key holding null = (%v, %v), want (false, nil)", ok, err)
	}
	if ok, err := cas.PluginKVCompareAndSet(plugin, "nulled", expect(`null`), `1`); err != nil || !ok {
		t.Fatalf("expecting null against a key holding null = (%v, %v), want (true, nil)", ok, err)
	}
}

// Several connections inside the same statement, and exactly one of them told
// it wrote.
//
// Whether they really are inside it at the same time is not something this
// test can assert about itself -- one winner is also what a serialized queue of
// writers reports, whatever the implementation is. The harness it runs on is
// held to that separately, in
// TestPluginKVWriterRacePG_CatchesReadThenWrite below.
func TestPluginKVCompareAndSetPG_ExactlyOneWriterWins(t *testing.T) {
	const writers = 8

	run := func(t *testing.T, seed *string) {
		t.Helper()
		ctx := newPostgresKVContext(t)
		cas := kvCAS(t, ctx)
		const plugin, key = "pg-racer", "contested"

		winners := kvWriterRace(t, ctx, cas, plugin, key, seed, writers)
		if len(winners) != 1 {
			t.Fatalf("%d of %d writers were told they wrote, want exactly 1: %v",
				len(winners), writers, winners)
		}
		if val, found := kvValue(t, ctx, plugin, key); !found || val != winners[0] {
			t.Errorf("stored value = %q (found=%v) but %q was told it won", val, found, winners[0])
		}
	}

	t.Run("from absent", func(t *testing.T) { run(t, nil) })
	t.Run("from a value", func(t *testing.T) { run(t, expect(`"seed"`)) })
}

// What the race above is worth, measured.
//
// On Postgres, connecting costs far more than the statement does, so writers
// released into a cold pool queue up in pgx rather than in the row: each one's
// read lands after the previous one's write, and a compare done in Go reports
// exactly one winner too. The test above then passes against the very
// implementation it exists to rule out, and its subject -- the compare being
// atomic at the storage layer -- goes untested on the dialect that has two
// databases' worth of ways to get it wrong.
//
// So the harness is run against that implementation and required to catch it.
// A round is caught when it reports anything other than one winner: with the
// writers genuinely overlapping, several of them read the same state and all
// write. Rounds each get their own context, because a pool that has already
// served this test is not the pool the shipped race starts on -- idle
// connections left over from an earlier round would hand a couple of writers
// the overlap the barrier is supposed to produce for all of them.
//
// Both seed paths, because they fail differently: expecting absent is the
// insert, expecting the seeded value is the update.
func TestPluginKVWriterRacePG_CatchesReadThenWrite(t *testing.T) {
	const (
		writers = 8
		rounds  = 6
	)

	run := func(t *testing.T, seed *string) {
		t.Helper()
		counts := make([]int, 0, rounds)
		for round := 0; round < rounds; round++ {
			ctx := newPostgresKVContext(t)
			winners := kvWriterRace(t, ctx, kvReadThenWriteCAS{ctx},
				"pg-harness", "contested", seed, writers)
			if len(winners) != 1 {
				return
			}
			counts = append(counts, len(winners))
		}
		t.Errorf("%d rounds of %d writers over a read-then-write compare-and-set reported "+
			"one winner every time (%v): the writers are not overlapping, so "+
			"TestPluginKVCompareAndSetPG_ExactlyOneWriterWins would pass against a "+
			"compare done in Go and proves nothing about the statement",
			rounds, writers, counts)
	}

	t.Run("from absent", func(t *testing.T) { run(t, nil) })
	t.Run("from a value", func(t *testing.T) { run(t, expect(`"seed"`)) })
}

// The lost update, on Postgres. The counter ends at the number of
// compare-and-sets that reported success, or a write was lost.
//
// This is the direct form of the claim and it does not depend on a barrier:
// the goroutines loop, so after their first iteration they hold connections and
// genuinely contend. It is the coverage the dialect was missing.
func TestPluginKVCompareAndSetPG_NoLostUpdate(t *testing.T) {
	ctx := newPostgresKVContext(t)
	kvIncrementRace(t, ctx, kvCAS(t, ctx), "pg-counter", "n", 6, 20)
}
