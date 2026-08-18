//go:build json1 && fts5

package application_context

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
)

// The two race shapes the compare-and-set is claimed against, shared by the
// SQLite and the Postgres tests rather than copied into each.
//
// Sharing them is the point, not tidiness. A race test is worth exactly what
// its barrier is worth: if the writers do not really overlap, every
// implementation reports one winner and the test passes while proving nothing.
// One copy means a barrier that is wrong is wrong in one place and fixed in
// one place, and TestPluginKVWriterRacePG_CatchesReadThenWrite can measure that
// one place against an implementation known to be broken.

// kvReadThenWriteCAS is the implementation the compare-and-set exists to rule
// out: read with PluginKVGet, compare in Go, then write with the unconditional
// upsert. It is not a straw man -- it is what a plugin author writes today, and
// what the operation would collapse back into if the compare left the
// statement.
//
// It is here so a harness can be held against something known to lose writes.
// Nothing outside these tests refers to it.
type kvReadThenWriteCAS struct{ ctx *MahresourcesContext }

func (r kvReadThenWriteCAS) PluginKVCompareAndSet(pluginName, key string, expected *string, value string) (bool, error) {
	current, found, err := r.ctx.PluginKVGet(pluginName, key)
	if err != nil {
		return false, err
	}
	if expected == nil {
		if found {
			return false, nil
		}
	} else if !found || current != *expected {
		return false, nil
	}
	if err := r.ctx.PluginKVSet(pluginName, key, value); err != nil {
		return false, err
	}
	return true, nil
}

// warmKVPool leaves one live connection per writer sitting in the pool, so the
// writers released below have nothing left to dial.
//
// It holds them all at once on purpose. A warm-up query per goroutine warms
// nothing in particular, because one connection handed back and taken again can
// serve all of them; connections held at the same time have to be distinct. The
// idle limit is raised first because database/sql keeps two and closes the rest
// as they are returned, which would undo the warming as it finished -- and two
// warm connections out of eight is a race between two writers with a queue of
// six behind them.
func warmKVPool(t *testing.T, ctx *MahresourcesContext, writers int) {
	t.Helper()

	sqlDB, err := ctx.db.DB()
	if err != nil {
		t.Fatalf("pool handle: %v", err)
	}
	sqlDB.SetMaxIdleConns(writers)

	// The SQLite race context caps the pool below the writer count, and asking
	// for a connection past that cap waits for one to be handed back, which
	// while they are all held is never.
	if open := sqlDB.Stats().MaxOpenConnections; open > 0 && open < writers {
		writers = open
	}

	conns := make([]*sql.Conn, 0, writers)
	for i := 0; i < writers; i++ {
		conn, err := sqlDB.Conn(context.Background())
		if err != nil {
			t.Fatalf("open warm connection %d: %v", i, err)
		}
		conns = append(conns, conn)
	}
	for _, conn := range conns {
		if err := conn.Close(); err != nil {
			t.Fatalf("return warm connection: %v", err)
		}
	}
}

// kvWriterRace releases writers goroutines at one key with one expectation and
// reports which of them were told they wrote. Exactly one may be, on either
// path into the row: everyone expecting absent is the insert, everyone
// expecting the seeded value is the update.
//
// A writer that fails is a broken harness rather than an outcome, so it is
// reported here and not left for the caller to interpret.
func kvWriterRace(t *testing.T, ctx *MahresourcesContext, cas kvCompareAndSetter, plugin, key string, seed *string, writers int) []string {
	t.Helper()

	if seed != nil {
		if err := ctx.PluginKVSet(plugin, key, *seed); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	warmKVPool(t, ctx, writers)

	var (
		ready   = make(chan struct{}, writers)
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
			ready <- struct{}{}
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
	// The release, and the whole of what these tests rest on. What it has to
	// produce is writers inside the statement together rather than queued in
	// front of it, and the expensive part of getting there is not the
	// statement: opening a connection costs far more, so writers released into
	// a cold pool simply run one after another and every implementation, atomic
	// or not, reports one winner. Hence the warmed pool above, and hence
	// waiting here until every goroutine is parked on start before letting any
	// of them go -- what is left between the release and the statement is then
	// a scheduler wakeup rather than a dial.
	//
	// TestPluginKVWriterRacePG_CatchesReadThenWrite is what says this is
	// enough, by running the same barrier against an implementation known to
	// lose writes and requiring it to be caught.
	for i := 0; i < writers; i++ {
		<-ready
	}
	close(start)
	wg.Wait()

	for _, err := range errs {
		t.Errorf("a writer failed: %v", err)
	}
	return winners
}

// kvIncrementRace is the retry loop an author actually writes: read the
// counter, compare-and-set it to one more, try again when the compare refuses.
// It runs from several goroutines at once and asserts that the counter ends at
// the number of compare-and-sets that reported success.
//
// Those two numbers come apart exactly when a write is lost, which is what a
// compare performed in Go rather than in the statement produces: two goroutines
// read the same number, both are told they wrote, and one increment vanishes.
// No single-goroutine test can see it.
func kvIncrementRace(t *testing.T, ctx *MahresourcesContext, cas kvCompareAndSetter, plugin, key string, writers, perWriter int) {
	t.Helper()

	// A compare that can never match -- an expectation serialized unlike what
	// set stores, a predicate that matches nothing -- refuses every attempt,
	// and an uncapped retry loop would hang until the suite's own timeout
	// instead of saying so.
	const maxAttempts = 10000

	if err := ctx.PluginKVSet(plugin, key, "0"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		errs      []error
	)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			done, attempts := 0, 0
			for done < perWriter {
				if attempts++; attempts > maxAttempts {
					mu.Lock()
					errs = append(errs, fmt.Errorf("%d attempts produced %d writes: the compare "+
						"refuses every time", attempts-1, done))
					mu.Unlock()
					return
				}
				current, found, err := ctx.PluginKVGet(plugin, key)
				if err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("read: %w", err))
					mu.Unlock()
					return
				}
				if !found {
					mu.Lock()
					errs = append(errs, fmt.Errorf("the counter row disappeared"))
					mu.Unlock()
					return
				}
				var n int
				if _, err := fmt.Sscanf(current, "%d", &n); err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("counter held %q: %w", current, err))
					mu.Unlock()
					return
				}
				ok, err := cas.PluginKVCompareAndSet(plugin, key, expect(current), fmt.Sprint(n+1))
				if err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("compare-and-set: %w", err))
					mu.Unlock()
					return
				}
				if ok {
					done++
					mu.Lock()
					successes++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	for _, err := range errs {
		t.Errorf("a writer failed: %v", err)
	}

	final, found := kvValue(t, ctx, plugin, key)
	if !found {
		t.Fatalf("the counter row is gone")
	}
	if want := writers * perWriter; successes != want {
		t.Errorf("%d successful compare-and-sets, want %d", successes, want)
	}
	if want := fmt.Sprint(successes); final != want {
		t.Errorf("counter = %s after %d successful compare-and-sets, want %s: a write was lost, "+
			"so the compare did not happen in the statement that wrote", final, successes, want)
	}
}
