//go:build json1 && fts5

package application_context

import (
	"testing"
)

func TestPluginKV_SetGetDelete(t *testing.T) {
	ctx := createTestContext(t)

	if err := ctx.PluginKVSet("test-plugin", "my_key", `"hello"`); err != nil {
		t.Fatalf("KVSet failed: %v", err)
	}

	val, found, err := ctx.PluginKVGet("test-plugin", "my_key")
	if err != nil {
		t.Fatalf("KVGet failed: %v", err)
	}
	if !found {
		t.Fatal("expected key to be found")
	}
	if val != `"hello"` {
		t.Errorf("expected %q, got %q", `"hello"`, val)
	}

	if err := ctx.PluginKVSet("test-plugin", "my_key", `42`); err != nil {
		t.Fatalf("KVSet upsert failed: %v", err)
	}
	val, _, _ = ctx.PluginKVGet("test-plugin", "my_key")
	if val != `42` {
		t.Errorf("expected %q after upsert, got %q", `42`, val)
	}

	if err := ctx.PluginKVDelete("test-plugin", "my_key"); err != nil {
		t.Fatalf("KVDelete failed: %v", err)
	}
	_, found, _ = ctx.PluginKVGet("test-plugin", "my_key")
	if found {
		t.Fatal("expected key to be deleted")
	}

	if err := ctx.PluginKVDelete("test-plugin", "nope"); err != nil {
		t.Fatalf("KVDelete of missing key failed: %v", err)
	}
}

func TestPluginKV_ListWithPrefix(t *testing.T) {
	ctx := createTestContext(t)

	ctx.PluginKVSet("test-plugin", "cat:images", `1`)
	ctx.PluginKVSet("test-plugin", "cat:docs", `2`)
	ctx.PluginKVSet("test-plugin", "other", `3`)

	keys, err := ctx.PluginKVList("test-plugin", "")
	if err != nil {
		t.Fatalf("KVList failed: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	keys, err = ctx.PluginKVList("test-plugin", "cat:")
	if err != nil {
		t.Fatalf("KVList with prefix failed: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys with prefix 'cat:', got %d", len(keys))
	}
	if keys[0] != "cat:docs" || keys[1] != "cat:images" {
		t.Errorf("unexpected key order: %v", keys)
	}
}

func TestPluginKV_Isolation(t *testing.T) {
	ctx := createTestContext(t)

	ctx.PluginKVSet("plugin-a", "key", `"a-value"`)
	ctx.PluginKVSet("plugin-b", "key", `"b-value"`)

	val, found, _ := ctx.PluginKVGet("plugin-a", "key")
	if !found || val != `"a-value"` {
		t.Errorf("plugin-a got wrong value: %q", val)
	}

	val, found, _ = ctx.PluginKVGet("plugin-b", "key")
	if !found || val != `"b-value"` {
		t.Errorf("plugin-b got wrong value: %q", val)
	}

	keys, _ := ctx.PluginKVList("plugin-a", "")
	if len(keys) != 1 {
		t.Errorf("plugin-a should see 1 key, got %d", len(keys))
	}
}

func TestPluginKV_Purge(t *testing.T) {
	ctx := createTestContext(t)

	ctx.PluginKVSet("doomed", "k1", `1`)
	ctx.PluginKVSet("doomed", "k2", `2`)
	ctx.PluginKVSet("survivor", "k1", `3`)

	if err := ctx.PluginKVPurge("doomed"); err != nil {
		t.Fatalf("KVPurge failed: %v", err)
	}

	keys, _ := ctx.PluginKVList("doomed", "")
	if len(keys) != 0 {
		t.Errorf("expected 0 keys after purge, got %d", len(keys))
	}

	keys, _ = ctx.PluginKVList("survivor", "")
	if len(keys) != 1 {
		t.Errorf("survivor should still have 1 key, got %d", len(keys))
	}
}
