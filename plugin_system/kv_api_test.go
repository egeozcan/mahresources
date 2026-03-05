package plugin_system

import (
	"sort"
	"testing"
)

type mockKVStore struct {
	data map[string]map[string]string
}

func newMockKVStore() *mockKVStore {
	return &mockKVStore{data: make(map[string]map[string]string)}
}

func (m *mockKVStore) KVGet(pluginName, key string) (string, bool, error) {
	if keys, ok := m.data[pluginName]; ok {
		if val, ok := keys[key]; ok {
			return val, true, nil
		}
	}
	return "", false, nil
}

func (m *mockKVStore) KVSet(pluginName, key, value string) error {
	if m.data[pluginName] == nil {
		m.data[pluginName] = make(map[string]string)
	}
	m.data[pluginName][key] = value
	return nil
}

func (m *mockKVStore) KVDelete(pluginName, key string) error {
	if keys, ok := m.data[pluginName]; ok {
		delete(keys, key)
	}
	return nil
}

func (m *mockKVStore) KVList(pluginName, prefix string) ([]string, error) {
	var result []string
	if keys, ok := m.data[pluginName]; ok {
		for k := range keys {
			if prefix == "" || (len(k) >= len(prefix) && k[:len(prefix)] == prefix) {
				result = append(result, k)
			}
		}
	}
	sort.Strings(result)
	return result, nil
}

func (m *mockKVStore) KVPurge(pluginName string) error {
	delete(m.data, pluginName)
	return nil
}

func TestKV_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "test-kv", `
		plugin = { name = "test-kv", version = "1.0.0", description = "test" }
		function init()
			mah.kv.set("count", 42)
			mah.kv.set("name", "hello")
			mah.kv.set("config", {theme = "dark", size = 3})
		end
	`)
	store := newMockKVStore()

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin("test-kv"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}

	val, ok := store.data["test-kv"]["count"]
	if !ok || val != "42" {
		t.Errorf("expected count=42, got %q (found=%v)", val, ok)
	}
	val, ok = store.data["test-kv"]["name"]
	if !ok || val != `"hello"` {
		t.Errorf("expected name=\"hello\", got %q", val)
	}
}

func TestKV_GetReturnsNilForMissing(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "test-kv-nil", `
		plugin = { name = "test-kv-nil", version = "1.0.0", description = "test" }
		function init()
			local v = mah.kv.get("nonexistent")
			if v ~= nil then
				error("expected nil, got: " .. tostring(v))
			end
		end
	`)
	store := newMockKVStore()

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin("test-kv-nil"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
}

func TestKV_Delete(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "test-kv-del", `
		plugin = { name = "test-kv-del", version = "1.0.0", description = "test" }
		function init()
			mah.kv.set("temp", "value")
			mah.kv.delete("temp")
			local v = mah.kv.get("temp")
			if v ~= nil then
				error("expected nil after delete")
			end
		end
	`)
	store := newMockKVStore()

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin("test-kv-del"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
}

func TestKV_List(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "test-kv-list", `
		plugin = { name = "test-kv-list", version = "1.0.0", description = "test" }
		function init()
			mah.kv.set("cat:images", 1)
			mah.kv.set("cat:docs", 2)
			mah.kv.set("other", 3)

			local all = mah.kv.list()
			if #all ~= 3 then
				error("expected 3 keys, got " .. #all)
			end

			local cats = mah.kv.list("cat:")
			if #cats ~= 2 then
				error("expected 2 keys with prefix, got " .. #cats)
			end
		end
	`)
	store := newMockKVStore()

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin("test-kv-list"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
}

func TestKV_RoundTrip_ComplexValues(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "test-kv-rt", `
		plugin = { name = "test-kv-rt", version = "1.0.0", description = "test" }
		function init()
			mah.kv.set("str", "hello world")
			mah.kv.set("num", 3.14)
			mah.kv.set("bool", true)
			mah.kv.set("obj", {name = "test", count = 5})
			mah.kv.set("arr", {10, 20, 30})

			local s = mah.kv.get("str")
			if type(s) ~= "string" or s ~= "hello world" then
				error("str roundtrip failed: " .. tostring(s))
			end

			local n = mah.kv.get("num")
			if type(n) ~= "number" or n ~= 3.14 then
				error("num roundtrip failed: " .. tostring(n))
			end

			local b = mah.kv.get("bool")
			if type(b) ~= "boolean" or b ~= true then
				error("bool roundtrip failed: " .. tostring(b))
			end

			local o = mah.kv.get("obj")
			if type(o) ~= "table" or o.name ~= "test" or o.count ~= 5 then
				error("obj roundtrip failed")
			end

			local a = mah.kv.get("arr")
			if type(a) ~= "table" or #a ~= 3 or a[1] ~= 10 then
				error("arr roundtrip failed")
			end
		end
	`)
	store := newMockKVStore()

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin("test-kv-rt"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
}
