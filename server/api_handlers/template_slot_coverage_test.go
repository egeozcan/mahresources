package api_handlers

import (
	"reflect"
	"strings"
	"testing"

	"mahresources/models"
)

// carrierModels maps the entityType string the generation handlers take to the
// model whose Custom* fields it must offer.
var carrierModels = map[string]any{
	"group":    models.Category{},
	"resource": models.ResourceCategory{},
	"note":     models.NoteType{},
}

// customSlotFieldsOf returns the Custom* string fields declared on a carrier,
// minus CustomCSS, which the generation handlers treat as a slot but which is
// listed in templateGenerateSharedSlots like the rest.
func customSlotFieldsOf(model any) []string {
	t := reflect.TypeOf(model)
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if strings.HasPrefix(f.Name, "Custom") && f.Type.Kind() == reflect.String {
			out = append(out, f.Name)
		}
	}
	return out
}

// TestEveryCustomSlotIsGeneratable is the drift guard for the slot set. A new
// Custom* field on one of the three carriers is a field an author has to be able
// to fill, and the natural-language generator is the surface most easily
// forgotten: nothing fails to compile when a field is added to a model and not
// to templateGenerateSharedSlots, the single-slot endpoint simply answers
// "unknown template slot" for it forever.
func TestEveryCustomSlotIsGeneratable(t *testing.T) {
	for carrier, model := range carrierModels {
		offered := map[string]bool{}
		for _, s := range templateGenerateBundleSlots(carrier) {
			offered[s] = true
		}
		for _, field := range customSlotFieldsOf(model) {
			if !offered[field] {
				t.Errorf("%s: model field %s is not in templateGenerateBundleSlots(%q); add it to templateGenerateSharedSlots or templateGenerateCarrierSlots", carrier, field, carrier)
			}
			if !templateGenerateSlotAllowed(carrier, field) {
				t.Errorf("%s: templateGenerateSlotAllowed(%q, %q) is false", carrier, carrier, field)
			}
		}
	}
}

// TestGenerationOffersNoSlotTheCarrierLacks is the other direction. Offering a
// slot a carrier has no field for would let the model draft markup that the save
// then silently drops, because the request DTO has nowhere to put it.
func TestGenerationOffersNoSlotTheCarrierLacks(t *testing.T) {
	for carrier, model := range carrierModels {
		has := map[string]bool{}
		for _, f := range customSlotFieldsOf(model) {
			has[f] = true
		}
		for _, s := range templateGenerateBundleSlots(carrier) {
			if !has[s] {
				t.Errorf("%s: generation offers %s, which %T does not declare", carrier, s, model)
			}
		}
	}
}

// TestCarrierOnlySlotsAreNotOfferedElsewhere pins the specific asymmetry the
// slot set encodes, so a well-meaning "make the carriers symmetric" edit to
// templateGenerateSharedSlots fails here rather than in a user's browser.
func TestCarrierOnlySlotsAreNotOfferedElsewhere(t *testing.T) {
	only := map[string]string{
		"CustomOwnEntities": "group",
		"CustomPreview":     "resource",
		"CustomLightbox":    "resource",
		"CustomCell":        "resource",
	}
	for slot, owner := range only {
		for carrier := range carrierModels {
			allowed := templateGenerateSlotAllowed(carrier, slot)
			if carrier == owner && !allowed {
				t.Errorf("%s should be generatable on %s", slot, carrier)
			}
			if carrier != owner && allowed {
				t.Errorf("%s is offered on %s, but only %s has a surface for it", slot, carrier, owner)
			}
		}
	}
}
