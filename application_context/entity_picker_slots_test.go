package application_context

import (
	"encoding/json"
	"fmt"
	"mahresources/models/query_models"
	"testing"
)

// A slot that is decoded but omitted from either write mapping must fail its
// reload assertion. JSON input also lets this regression run before fields exist.
func TestEntityPickerSlotsPersistAndClear(t *testing.T) {
	for _, kind := range []string{"category", "noteType", "resourceCategory"} {
		t.Run(kind, func(t *testing.T) {
			ctx := createTestContext(t)
			write := func(id uint, html, css string) (any, error) {
				input := fmt.Sprintf(`{"ID":%d,"Name":"Picker carrier","CustomEntityPickerResult":%q,"CustomEntityPickerResultCSS":%q}`, id, html, css)
				switch kind {
				case "category":
					if id == 0 {
						return ctx.CreateCategory(decodePickerSlotInput[query_models.CategoryCreator](t, input))
					}
					return ctx.UpdateCategory(decodePickerSlotInput[query_models.CategoryEditor](t, input))
				case "noteType":
					return ctx.CreateOrUpdateNoteType(decodePickerSlotInput[query_models.NoteTypeEditor](t, input))
				default:
					if id == 0 {
						return ctx.CreateResourceCategory(decodePickerSlotInput[query_models.ResourceCategoryCreator](t, input))
					}
					return ctx.UpdateResourceCategory(decodePickerSlotInput[query_models.ResourceCategoryEditor](t, input))
				}
			}
			var id uint
			for _, values := range [][2]string{{"<b>[property path=\"Name\"]</b>", ".picker{color:blue}"}, {"<i>Updated</i>", ".picker{color:red}"}, {"", ""}} {
				created, err := write(id, values[0], values[1])
				if err != nil {
					t.Fatal(err)
				}
				id = uint(pickerSlotJSON(t, created)["ID"].(float64))
				var stored any
				switch kind {
				case "category":
					stored, err = ctx.GetCategory(id)
				case "noteType":
					stored, err = ctx.GetNoteType(id)
				default:
					stored, err = ctx.GetResourceCategory(id)
				}
				if err != nil {
					t.Fatal(err)
				}
				got := pickerSlotJSON(t, stored)
				if got["CustomEntityPickerResult"] != values[0] || got["CustomEntityPickerResultCSS"] != values[1] {
					t.Fatalf("reloaded slots = (%v, %v), want (%q, %q)", got["CustomEntityPickerResult"], got["CustomEntityPickerResultCSS"], values[0], values[1])
				}
			}
		})
	}
}

func TestEntityPickerSlotsGenericBuilders(t *testing.T) {
	const input = `{"Name":"Carrier","SectionConfig":"{}","CustomEntityPickerResult":"<b>Picker</b>","CustomEntityPickerResultCSS":".picker{color:red}"}`
	category, err := buildCategory(decodePickerSlotInput[query_models.CategoryCreator](t, input))
	if err != nil {
		t.Fatal(err)
	}
	resourceCategory, err := buildResourceCategory(decodePickerSlotInput[query_models.ResourceCategoryCreator](t, input))
	if err != nil {
		t.Fatal(err)
	}
	noteType, err := buildNoteType(decodePickerSlotInput[query_models.NoteTypeEditor](t, input))
	if err != nil {
		t.Fatal(err)
	}
	for _, built := range []any{category, resourceCategory, noteType} {
		got := pickerSlotJSON(t, built)
		if got["CustomEntityPickerResult"] != "<b>Picker</b>" || got["CustomEntityPickerResultCSS"] != ".picker{color:red}" {
			t.Errorf("generic builder dropped slots: %T", built)
		}
	}
}

func TestEntityPickerSlotsPluginCreateUpdatePatch(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}
	for _, carrier := range []struct {
		name          string
		create        func(map[string]any) (map[string]any, error)
		update, patch func(uint, map[string]any) (map[string]any, error)
		get           func(uint) (map[string]any, error)
	}{
		{"category", adapter.CreateCategory, adapter.UpdateCategory, adapter.PatchCategory, adapter.GetCategoryData},
		{"noteType", adapter.CreateNoteType, adapter.UpdateNoteType, adapter.PatchNoteType, adapter.GetNoteTypeData},
		{"resourceCategory", adapter.CreateResourceCategory, adapter.UpdateResourceCategory, adapter.PatchResourceCategory, adapter.GetResourceCategoryData},
	} {
		t.Run(carrier.name, func(t *testing.T) {
			created, err := carrier.create(map[string]any{"name": carrier.name, "custom_entity_picker_result": "<b>Created</b>", "custom_entity_picker_result_css": ".picker{color:red}"})
			if err != nil {
				t.Fatal(err)
			}
			id := uint(created["id"].(float64))
			check := func(html, css string) {
				t.Helper()
				got, err := carrier.get(id)
				if err != nil {
					t.Fatal(err)
				}
				if got["custom_entity_picker_result"] != html || got["custom_entity_picker_result_css"] != css {
					t.Fatalf("plugin round trip slots = (%v, %v), want (%q, %q)", got["custom_entity_picker_result"], got["custom_entity_picker_result_css"], html, css)
				}
			}
			check("<b>Created</b>", ".picker{color:red}")
			_, err = carrier.update(id, map[string]any{"name": carrier.name, "custom_entity_picker_result": "<i>Updated</i>", "custom_entity_picker_result_css": ".picker{color:blue}"})
			if err != nil {
				t.Fatal(err)
			}
			check("<i>Updated</i>", ".picker{color:blue}")
			_, err = carrier.patch(id, map[string]any{"description": "Keep omitted slots"})
			if err != nil {
				t.Fatal(err)
			}
			check("<i>Updated</i>", ".picker{color:blue}")
			_, err = carrier.patch(id, map[string]any{"custom_entity_picker_result": "", "custom_entity_picker_result_css": ""})
			if err != nil {
				t.Fatal(err)
			}
			check("", "")
		})
	}
}

func decodePickerSlotInput[T any](t *testing.T, input string) *T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(input), &value); err != nil {
		t.Fatal(err)
	}
	return &value
}

func pickerSlotJSON(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return *decodePickerSlotInput[map[string]any](t, string(data))
}
