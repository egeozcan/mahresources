package api_tests

import (
	"fmt"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/types"
)

// [using] loads its target through the shortcode query executor, so the target
// is confined to the viewer's subtree exactly as an [mrql] result is. These run
// the real executor through the description preview, which renders shortcodes
// against the entity the way its detail page does.

func TestUsingShortcode_ScopedPrincipalCannotReachOutside(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)

	// The in-subtree note's Meta points at the out-of-subtree note, so the
	// id-path source is tested with a value a user could have written.
	if err := tc.DB.Model(&models.Note{}).Where("id = ?", f.nInID).
		Update("meta", types.JSON(fmt.Sprintf(`{"ref":%d}`, f.nOutID))).Error; err != nil {
		t.Fatalf("set meta: %v", err)
	}

	preview := func(bearer, text string) string {
		t.Helper()
		return previewHTML(t, previewDescription(tc, bearer, "note", f.nInID, text))
	}
	const body = `[property path="Name"][else]hidden`

	outside := map[string]string{
		"id note":      fmt.Sprintf(`[using type="note" id="%d"]%s[/using]`, f.nOutID, body),
		"id group":     fmt.Sprintf(`[using type="group" id="%d"]%s[/using]`, f.outsideID, body),
		"id resource":  fmt.Sprintf(`[using type="resource" id="%d"]%s[/using]`, f.rOutID, body),
		"id-path note": `[using type="note" id-path="ref"]` + body + `[/using]`,
		"query":        `[using query='type = note AND name = "sf-nOut" ORDER BY name' scope="global"]` + body + `[/using]`,
	}
	for name, text := range outside {
		html := preview(f.bearer, text)
		if strings.Contains(html, "sf-nOut") || strings.Contains(html, "sf-outside") || strings.Contains(html, "sf-rOut") {
			t.Errorf("%s: [using] leaked an out-of-subtree entity: %s", name, html)
		}
		if !strings.Contains(html, "hidden") {
			t.Errorf("%s: expected the [else] branch, got %s", name, html)
		}
	}

	// Controls: the same sources reach in-subtree targets, so the checks above
	// are not passing because [using] renders nothing at all.
	inside := map[string]struct{ text, want string }{
		"id group":  {fmt.Sprintf(`[using type="group" id="%d"]%s[/using]`, f.childID, body), "sf-child"},
		"rel owner": {`[using rel="owner"]` + body + `[/using]`, "sf-child"},
		"rel root":  {`[using rel="root"]` + body + `[/using]`, "sf-root"},
		"id-prop":   {`[using type="group" id-prop="OwnerId"]` + body + `[/using]`, "sf-child"},
		"query":     {`[using query='type = resource AND name = "sf-rIn" ORDER BY name' scope="global"]` + body + `[/using]`, "sf-rIn"},
	}
	for name, c := range inside {
		if html := preview(f.bearer, c.text); !strings.Contains(html, c.want) {
			t.Errorf("%s: expected %q, got %s", name, c.want, html)
		}
	}

	// And an unconfined viewer does reach the outside note, so the [else] above
	// is the scope at work, not a lookup that cannot succeed.
	adminB := roleBearer(t, tc, models.RoleAdmin)
	if html := preview(adminB, outside["id-path note"]); !strings.Contains(html, "sf-nOut") {
		t.Errorf("admin: expected the outside note through id-path, got %s", html)
	}
}

func TestUsingShortcode_BodyRendersAgainstTarget(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	if err := tc.DB.Model(&models.Group{}).Where("id = ?", f.childID).
		Update("meta", types.JSON(`{"status":"active"}`)).Error; err != nil {
		t.Fatalf("set meta: %v", err)
	}

	html := previewHTML(t, previewDescription(tc, f.bearer, "note", f.nInID,
		`[using rel="owner"][meta path="status" inline="true"]|[link]|[using rel="owner"][property path="Name"][/using][/using]`))
	want := fmt.Sprintf("active|/group?id=%d|sf-root", f.childID)
	if !strings.Contains(html, want) {
		t.Errorf("expected %q in %s", want, html)
	}
}
