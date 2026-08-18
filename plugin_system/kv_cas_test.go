package plugin_system

import (
	"testing"
)

// The Lua half of the compare-and-set. The store's own conditional statement is
// tested against a real database in application_context; what is left here is
// the surface an author writes against: what the three arguments mean, and what
// comes back.

// enableKVPlugin loads one plugin against a mock store and enables it, which is
// what runs its init(). Assertions live in the Lua so a failure names the
// property that broke.
func enableKVPlugin(t *testing.T, name, body string) *mockKVStore {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, name, `
		plugin = { name = "`+name+`", version = "1.0.0", description = "test" }
		function init()
`+body+`
		end
	`)
	store := newMockKVStore()

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Close)
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin(name); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
	return store
}

// A compare-and-set whose failure cannot be told from its success is not one.
// The refusal also has to be a return value rather than a raise: an author's
// answer to losing the race is to read again and retry, which they cannot do
// from a handler that has already been unwound.
func TestKVCompareAndSet_ReportsWhetherItWrote(t *testing.T) {
	store := enableKVPlugin(t, "test-cas", `
			mah.kv.set("k", 1)

			local won = mah.kv.compare_and_set("k", 1, 2)
			if won ~= true then
				error("a compare against the current value returned " .. tostring(won) .. ", want true")
			end

			local lost = mah.kv.compare_and_set("k", 1, 3)
			if lost ~= false then
				error("a compare against a stale value returned " .. tostring(lost) .. ", want false")
			end

			-- Reached only if losing the race did not unwind the handler.
			mah.kv.set("kept_going", true)
	`)

	if got := store.data["test-cas"]["k"]; got != "2" {
		t.Errorf("stored value = %q, want %q: the losing compare-and-set wrote anyway", got, "2")
	}
	if _, ok := store.data["test-cas"]["kept_going"]; !ok {
		t.Errorf("the handler did not continue after a refused compare-and-set")
	}
}

// "The key must not exist yet" needs its own expected value. Lua nil already
// means the stored JSON null, and an empty table is an ordinary value, so the
// sentinel has to be recognised by identity before the expectation is
// serialized -- a sentinel that went through the encoder would arrive as an
// empty object and mean something an author can legitimately store.
func TestKVCompareAndSet_AbsentIsItsOwnExpectation(t *testing.T) {
	store := enableKVPlugin(t, "test-cas-absent", `
			local created = mah.kv.compare_and_set("fresh", mah.kv.ABSENT, "made")
			if created ~= true then
				error("compare-and-set from ABSENT on a missing key returned " .. tostring(created))
			end

			local again = mah.kv.compare_and_set("fresh", mah.kv.ABSENT, "twice")
			if again ~= false then
				error("compare-and-set from ABSENT onto an existing key returned " .. tostring(again))
			end

			local empty = mah.kv.compare_and_set("other", {}, "made")
			if empty ~= false then
				error("an empty-table expectation matched a missing key: " .. tostring(empty))
			end
	`)

	if got := store.data["test-cas-absent"]["fresh"]; got != `"made"` {
		t.Errorf("stored value = %q, want %q", got, `"made"`)
	}
	if _, ok := store.data["test-cas-absent"]["other"]; ok {
		t.Errorf("a refused compare-and-set created the key")
	}
}

// The other half of the same asymmetry: nil is the stored null, and it must not
// match a row that is not there.
func TestKVCompareAndSet_NilExpectsTheStoredNull(t *testing.T) {
	store := enableKVPlugin(t, "test-cas-nil", `
			mah.kv.set("nulled", nil)

			local matched = mah.kv.compare_and_set("nulled", nil, "value")
			if matched ~= true then
				error("expecting nil against a key holding null returned " .. tostring(matched))
			end

			local missing = mah.kv.compare_and_set("nothing", nil, "value")
			if missing ~= false then
				error("expecting nil matched a missing key: " .. tostring(missing))
			end
	`)

	if got := store.data["test-cas-nil"]["nulled"]; got != `"value"` {
		t.Errorf("stored value = %q, want %q", got, `"value"`)
	}
	if _, ok := store.data["test-cas-nil"]["nothing"]; ok {
		t.Errorf("expecting nil against a missing key created it")
	}
}

// The expectation is a Lua value, so it has to be serialized exactly the way
// set serializes what it stores. Anything else and a table read back with
// mah.kv.get can never be compared against.
func TestKVCompareAndSet_ExpectedIsSerializedLikeSet(t *testing.T) {
	enableKVPlugin(t, "test-cas-table", `
			mah.kv.set("cfg", { theme = "dark", size = 3 })

			local matched = mah.kv.compare_and_set("cfg", { theme = "dark", size = 3 }, { theme = "light" })
			if matched ~= true then
				error("a table expectation equal to what set stored returned " .. tostring(matched))
			end

			local stale = mah.kv.compare_and_set("cfg", { theme = "dark", size = 3 }, 1)
			if stale ~= false then
				error("a table expectation that no longer matches returned " .. tostring(stale))
			end
	`)
}

// No store is not a lost race. Returning false there would tell an author that
// somebody else got in first and invite them to retry forever; every other
// mah.kv write says so instead, and this one has to as well.
func TestKVCompareAndSet_RaisesWhenTheStoreIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "test-cas-nostore", `
		plugin = { name = "test-cas-nostore", version = "1.0.0", description = "test" }
		function init()
			-- Without this, pcall of a missing function reports the same failure
			-- as a refusal and the test would pass before the function exists.
			if type(mah.kv.compare_and_set) ~= "function" then
				error("mah.kv.compare_and_set is not a function")
			end
			local ok = pcall(mah.kv.compare_and_set, "k", mah.kv.ABSENT, 1)
			if ok then
				error("compare-and-set returned with no store wired, want a raised error")
			end
		end
	`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	// Deliberately no SetKVStore.

	if err := mgr.EnablePlugin("test-cas-nostore"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
}

// The cap on a stored value is enforced and, until this item, said nowhere. A
// plugin has to be able to read it, because meeting it the other way raises and
// takes the handler with it.
func TestKV_MaxValueSizeIsReadable(t *testing.T) {
	enableKVPlugin(t, "test-kv-cap", `
			if type(mah.kv.max_value_size) ~= "number" then
				error("mah.kv.max_value_size is " .. type(mah.kv.max_value_size) .. ", want a number")
			end
			if mah.kv.max_value_size <= 0 then
				error("mah.kv.max_value_size = " .. tostring(mah.kv.max_value_size))
			end
			-- The documented way to stay under it: mah.json.encode runs the same
			-- encoder set does, so the length it reports is the length set checks.
			if #mah.json.encode({ a = 1 }) > mah.kv.max_value_size then
				error("a two-byte table does not fit under the cap")
			end
	`)
}
