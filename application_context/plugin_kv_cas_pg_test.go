//go:build postgres && json1 && fts5

package application_context

import (
	"fmt"
	"sync"
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

// Postgres runs the writers genuinely in parallel, so this is the sharper half
// of the concurrency claim: with several connections all inside the same
// statement, exactly one may be told it wrote.
func TestPluginKVCompareAndSetPG_ExactlyOneWriterWins(t *testing.T) {
	const writers = 8

	run := func(t *testing.T, seed *string) {
		t.Helper()
		ctx := newPostgresKVContext(t)
		cas := kvCAS(t, ctx)
		const plugin, key = "pg-racer", "contested"

		if seed != nil {
			if err := ctx.PluginKVSet(plugin, key, *seed); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}

		var (
			start   = make(chan struct{})
			wg      sync.WaitGroup
			mu      sync.Mutex
			winners []string
			errs    []error
		)
		for i := 0; i < writers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				mine := fmt.Sprintf(`"writer-%d"`, i)
				<-start
				ok, err := cas.PluginKVCompareAndSet(plugin, key, seed, mine)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs = append(errs, err)
					return
				}
				if ok {
					winners = append(winners, mine)
				}
			}(i)
		}
		close(start)
		wg.Wait()

		for _, err := range errs {
			t.Errorf("a writer failed: %v", err)
		}
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
