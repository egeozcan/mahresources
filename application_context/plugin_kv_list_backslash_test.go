//go:build json1 && fts5

package application_context

import (
	"testing"
)

// TestPluginKV_ListWithBackslashPrefix demonstrates a bug where a backslash in
// the prefix argument to PluginKVList causes incorrect results.
//
// The LIKE pattern escaping in PluginKVList escapes '%' and '_' but does NOT
// escape the backslash character '\'. Since the LIKE clause uses ESCAPE '\',
// a literal backslash in the prefix is interpreted as the escape character
// rather than a literal character. For example, prefix "a\b" becomes pattern
// "a\b%" where "\b" is interpreted as "escaped b" (i.e., just "b"), matching
// keys starting with "ab" instead of keys starting with "a\b".
func TestPluginKV_ListWithBackslashPrefix(t *testing.T) {
	ctx := createTestContext(t)

	pluginName := "backslash-test"

	// Clean up from any previous test run (shared DB)
	defer ctx.PluginKVPurge(pluginName)

	// Set up keys: one with a backslash in the key, one without
	ctx.PluginKVSet(pluginName, `path\to\file1`, `1`)
	ctx.PluginKVSet(pluginName, `path\to\file2`, `2`)
	ctx.PluginKVSet(pluginName, `pathtofile3`, `3`)   // no backslashes — should NOT match prefix "path\to\"

	// List with a prefix that contains backslashes
	keys, err := ctx.PluginKVList(pluginName, `path\to\`)
	if err != nil {
		t.Fatalf("PluginKVList failed: %v", err)
	}

	// We expect exactly 2 keys: "path\to\file1" and "path\to\file2"
	// BUG: because the backslash is not escaped in the LIKE pattern,
	// the prefix "path\to\" is misinterpreted. The LIKE ESCAPE '\'
	// treats each '\' as an escape prefix for the next character,
	// so "path\to\" becomes "pathto" + dangling escape. The actual
	// behavior depends on SQLite's handling of the trailing escape
	// character, but it won't correctly match only keys starting
	// with "path\to\".
	if len(keys) != 2 {
		t.Errorf("expected 2 keys matching prefix 'path\\to\\', got %d: %v", len(keys), keys)
	}

	// Verify the correct keys were returned
	expectedKeys := map[string]bool{
		`path\to\file1`: true,
		`path\to\file2`: true,
	}
	for _, k := range keys {
		if !expectedKeys[k] {
			t.Errorf("unexpected key in results: %q", k)
		}
	}
}
