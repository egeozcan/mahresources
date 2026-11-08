package api_handlers

import (
	"context"
	"encoding/json"
	"testing"

	"mahresources/application_context"
)

// A protected Name that is the string "1050" must not stop a run because a page
// cursor says 1050 (review finding 5).
func TestPagedJSONRegistersItsCursorAsReadable(t *testing.T) {
	p := application_context.NewTemplatePrivacy("pg")
	p.Text("1050")
	body := pagedJSON(p, "entries", []string{}, true, 1000, 50)
	payload, _ := json.Marshal(map[string]string{"m": body})
	if err := p.CheckOutbound(context.Background(), payload); err != nil {
		t.Fatalf("the generated nextOffset is readable: %v", err)
	}
}
