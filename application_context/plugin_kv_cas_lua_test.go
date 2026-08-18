//go:build json1 && fts5

package application_context

import (
	"fmt"
	"strings"
	"testing"
)

// The compare-and-set as a plugin actually reaches it: through the Lua binding,
// the invocation, the host adapter and a real database. The unit tests above
// pin the statement; these pin that a plugin gets the same operation whether or
// not it is inside mah.db.transaction, which is where a second connection would
// quietly take over.

func casPlugin(body string) string {
	return `plugin = { name = "caswriter", version = "1.0", description = "compare-and-sets a cursor" }
function init()
    mah.inject("page_bottom", function(ctx)
` + body + `
    end)
end
`
}

// A compare-and-set inside a transaction must see the transaction's own
// uncommitted writes, and its result must commit and roll back with everything
// else. A store reached on a second connection sees neither: it would compare
// against the pre-transaction value and refuse.
func TestPluginKVCompareAndSet_JoinsTheTransaction(t *testing.T) {
	t.Run("compares against the transaction's own writes", func(t *testing.T) {
		ctx := newTwoPluginContext(t, map[string]string{"caswriter": casPlugin(`
        mah.kv.set("cursor", "before")
        local inner
        mah.db.transaction(function()
            mah.kv.set("cursor", "mid")
            inner = mah.kv.compare_and_set("cursor", "mid", "after")
        end)
        return "inner=" .. tostring(inner) .. " final=" .. tostring(mah.kv.get("cursor"))
`)})

		got := runSlot(ctx, "page_bottom")
		if !strings.Contains(got, "inner=true") {
			t.Errorf("slot reported %q: the compare-and-set did not see the value written "+
				"earlier in the same transaction, so it did not run on the transaction's handle", got)
		}
		if !strings.Contains(got, "final=after") {
			t.Errorf("slot reported %q, want final=after: the committed compare-and-set did not "+
				"survive the transaction", got)
		}
	})

	// The compare has to have succeeded before the rollback, or "before" is
	// still there for the uninteresting reason that nothing ever wrote.
	t.Run("rolls back with it", func(t *testing.T) {
		ctx := newTwoPluginContext(t, map[string]string{"caswriter": casPlugin(`
        mah.kv.set("cursor", "before")
        local inner
        mah.db.transaction(function()
            inner = mah.kv.compare_and_set("cursor", "before", "after")
            error("no")
        end)
        return "inner=" .. tostring(inner) .. " final=" .. tostring(mah.kv.get("cursor"))
`)})

		got := runSlot(ctx, "page_bottom")
		if !strings.Contains(got, "inner=true") {
			t.Errorf("slot reported %q, want inner=true: the compare-and-set that the rollback "+
				"has to undo never happened", got)
		}
		if !strings.Contains(got, "final=before") {
			t.Errorf("slot reported %q, want final=before: the compare-and-set survived the rollback", got)
		}
	})
}

// Outside a transaction the same call has to reach the same store. Stated
// separately because the two paths are different objects.
func TestPluginKVCompareAndSet_WorksOutsideATransaction(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{"caswriter": casPlugin(`
        mah.kv.set("cursor", "before")
        local won = mah.kv.compare_and_set("cursor", "before", "after")
        local lost = mah.kv.compare_and_set("cursor", "before", "again")
        return "won=" .. tostring(won) .. " lost=" .. tostring(lost) ..
            " final=" .. tostring(mah.kv.get("cursor"))
`)})

	got := runSlot(ctx, "page_bottom")
	for _, want := range []string{"won=true", "lost=false", "final=after"} {
		if !strings.Contains(got, want) {
			t.Errorf("slot reported %q, want it to contain %q", got, want)
		}
	}
}

// The cap has to be readable before the write rather than only discoverable by
// failing one, because failing one raises and takes the author's handler with
// it. The number a plugin reads must be the number the store enforces.
func TestPluginKV_MaxValueSizeIsVisibleToLua(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{"caswriter": casPlugin(`
        return tostring(mah.kv.max_value_size)
`)})

	if got, want := runSlot(ctx, "page_bottom"), fmt.Sprint(kvValueCapBytes); got != want {
		t.Errorf("mah.kv.max_value_size = %q, want %q (the cap PluginKVSet enforces)", got, want)
	}
}

const pluginLuaAPIPath = "../docs-site/docs/features/plugin-lua-api.md"

// The cap is enforced today and documented nowhere, so an author meets it as a
// raised error in production. The page that describes mah.kv has to name it,
// along with the two things that let an author avoid it and retry safely.
func TestPluginLuaAPIDocs_KVSectionIsHonest(t *testing.T) {
	section := docsSection(t, pluginLuaAPIPath, "## mah.kv")

	for _, want := range []struct{ text, why string }{
		{fmt.Sprint(kvValueCapBytes), "the value size cap, in the bytes the raised error names"},
		{"max_value_size", "the way to check a value against the cap before writing it"},
		{"compare_and_set", "the compare-and-set"},
	} {
		if !strings.Contains(section, want.text) {
			t.Errorf("%s: the mah.kv section does not mention %q (%s)", pluginLuaAPIPath, want.text, want.why)
		}
	}
}
