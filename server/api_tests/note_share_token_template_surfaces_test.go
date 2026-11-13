package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// A note type's card, hover-card and MRQL-result templates render on pages
// other than the note's own: the owner group, a linked resource or tag, the
// note type, the hover card and a rendered MRQL query, each against the note
// as loaded. The note page's strip covers none of them, so the shortcode
// resolver itself keeps ShareToken from a reader that may not share (#107),
// while a reader that may share keeps seeing what its template prints.
func TestNoteShareToken_NoteTypeTemplateCannotPrintItOffTheNotePage(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildShareTokenFixture(t, tc)
	updates := map[string]any{}
	for _, col := range []string{"custom_avatar", "custom_summary", "custom_panel_summary", "custom_hover_card", "custom_mrql_result"} {
		updates[col] = `X107:[property path="ShareToken"]:END107`
	}
	if err := tc.DB.Model(&models.NoteType{}).Where("id = ?", f.noteType.ID).Updates(updates).Error; err != nil {
		t.Fatalf("set note type templates: %v", err)
	}
	pages := []string{
		fmt.Sprintf("/group?id=%d", f.group.ID),
		fmt.Sprintf("/resource?id=%d", f.resource.ID),
		fmt.Sprintf("/tag?id=%d", f.tag.ID),
		fmt.Sprintf("/noteType?id=%d", f.noteType.ID),
		fmt.Sprintf("/hovercard?type=note&id=%d", f.note.ID),
	}
	for who, r := range shareTokenReaders(t, tc, f, "st107_cards") {
		t.Run(who, func(t *testing.T) {
			for _, path := range pages {
				res := doReq(tc, http.MethodGet, path,
					map[string]string{"Accept": "text/html", "Authorization": r.bearer}, nil, nil)
				if res.Code != http.StatusOK {
					t.Fatalf("GET %s = %d", path, res.Code)
				}
				assertTemplateTokenDisclosure(t, "GET "+path, res.Body.String(), "X107:", f.token, r.wantToken)
			}

			res := doReq(tc, http.MethodPost, "/v1/mrql?render=1",
				map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": r.bearer},
				nil, strings.NewReader(`{"query":"type = note"}`))
			if res.Code != http.StatusOK {
				t.Fatalf("POST /v1/mrql?render=1 = %d: %s", res.Code, res.Body.String())
			}
			var out struct {
				Notes []struct {
					RenderedHTML string `json:"renderedHTML"`
				} `json:"notes"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode mrql: %v", err)
			}
			if len(out.Notes) != 1 {
				t.Fatalf("POST /v1/mrql?render=1 returned %d notes: %s", len(out.Notes), res.Body.String())
			}
			assertTemplateTokenDisclosure(t, "POST /v1/mrql?render=1", out.Notes[0].RenderedHTML, "X107:", f.token, r.wantToken)
		})
	}
}
