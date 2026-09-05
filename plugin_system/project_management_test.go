package plugin_system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestBundledProjectManagementCompiles(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	if _, err := L.LoadFile(filepath.Join("..", "plugins", "project-management", "plugin.lua")); err != nil {
		t.Fatal(err)
	}
}

func TestBundledProjectManagementGrantsCoverRegistrations(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "plugins", "project-management", "plugin.lua"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := manifestFromLua(t, string(source))
	if err != nil {
		t.Fatal(err)
	}
	for surface, capability := range map[string]string{"mah.action(": CapActions, "mah.on(": CapHooks, "mah.schedule(": CapSchedule, "mah.shortcode(": CapRender, "mah.block_type(": CapRender, "mah.page(": CapPages, "mah.api(": CapAPI, "mah.kv.": CapKV} {
		if strings.Contains(string(source), surface) && !manifest.Capabilities().Has(capability) {
			t.Errorf("%s is registered without %s", surface, capability)
		}
	}
	err = filepath.WalkDir(filepath.Join("..", "plugins", "project-management"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, _ := filepath.Rel(filepath.Join("..", "plugins", "project-management"), path)
		bundled, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fixture, err := os.ReadFile(filepath.Join("..", "e2e", "test-plugins", "project-management", relative))
		if err != nil {
			return err
		}
		if string(bundled) != string(fixture) {
			t.Errorf("PM fixture differs: %s", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
