//go:build json1 && fts5

package application_context

import (
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
	// The release, and the whole of what these tests rest on. What it has to
	// produce is writers inside the statement together rather than queued in
	// front of it: opening a connection costs far more than the statement does,
	// so writers released into a cold pool simply run one after another and
	// every implementation, atomic or not, reports one winner.
	// TestPluginKVWriterRacePG_CatchesReadThenWrite is what says whether it
	// does.
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
