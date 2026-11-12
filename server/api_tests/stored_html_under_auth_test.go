package api_tests

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	xhtml "golang.org/x/net/html"

	"mahresources/application_context"
	"mahresources/models"
)

// Issue #92: with -auth on, a Description or a note text block is written by
// accounts less trusted than the admin who later opens it, so its rendered
// markdown is sanitized for every role before shortcodes expand. With auth off
// raw HTML stays (KAN-4).

// The three payloads from the issue: an event handler, a script element, and
// Alpine directives, which both render paths initialize.
const storedXSSPayload = `<img src=x onerror="window.__xss=1">

<script>window.__xss=1</script>

<div x-data x-init="window.__xss=1"></div>`

const payloadCode = "window.__xss=1"

// liveStoredXSS parses body the way a browser would (template contents
// included) and lists every place the payload became live markup: a <script>
// whose code is the payload, or an event-handler or Alpine attribute whose
// value is it. The page also carries the stored text as data (a block's
// data-source, the blocks JSON inside x-data), where it is a string, not the
// attribute's code, and is not reported.
func liveStoredXSS(t *testing.T, body string) []string {
	t.Helper()
	doc, err := xhtml.Parse(strings.NewReader(body))
	require.NoError(t, err)
	var found []string
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			if n.Data == "script" && n.FirstChild != nil && n.FirstChild.Data == payloadCode {
				found = append(found, "<script>"+n.FirstChild.Data)
			}
			for _, a := range n.Attr {
				name := strings.ToLower(a.Key)
				runs := strings.HasPrefix(name, "on") || strings.HasPrefix(name, "x-") ||
					strings.HasPrefix(name, "@") || strings.HasPrefix(name, ":")
				if runs && a.Val == payloadCode {
					found = append(found, fmt.Sprintf("<%s %s=%q>", n.Data, a.Key, a.Val))
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func assertNoLiveStoredXSS(t *testing.T, where, body string) {
	t.Helper()
	if found := liveStoredXSS(t, body); len(found) > 0 {
		t.Errorf("%s: stored payload rendered as live markup: %v", where, found)
	}
}

type storedXSSFixture struct {
	descNote    *models.Note // carries the payload in its Description
	blockNote   *models.Note // carries the payload in a text block
	adminHeader map[string]string
}

// newStoredXSSFixture stores text as a subtree-confined user, through the API
// routes the UI uses, and returns an admin bearer to view it with. The
// Description and the block live on separate notes because a note with a text
// block shows the block in place of its Description.
func newStoredXSSFixture(t *testing.T, tc *TestContext, text string) storedXSSFixture {
	t.Helper()
	scope := &models.Group{Name: "xss-scope"}
	require.NoError(t, tc.DB.Create(scope).Error)
	descNote := &models.Note{Name: "xss-desc-note", OwnerId: &scope.ID}
	require.NoError(t, tc.DB.Create(descNote).Error)
	blockNote := &models.Note{Name: "xss-block-note", OwnerId: &scope.ID}
	require.NoError(t, tc.DB.Create(blockNote).Error)

	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "mallory", Password: "password1", Role: models.RoleUser, ScopeGroupId: &scope.ID,
	})
	require.NoError(t, err)
	userToken, _, err := tc.AppCtx.CreateApiToken(user.ID, "mallory-token", nil)
	require.NoError(t, err)

	var admin models.User
	require.NoError(t, tc.DB.Where("username = ?", "admin").First(&admin).Error)
	adminToken, _, err := tc.AppCtx.CreateApiToken(admin.ID, "admin-token", nil)
	require.NoError(t, err)

	rr := doReq(tc, http.MethodPost, fmt.Sprintf("/v1/note/editDescription?id=%d", descNote.ID),
		map[string]string{"Accept": "application/json", "Authorization": "Bearer " + userToken, "Content-Type": "application/x-www-form-urlencoded"},
		nil, strings.NewReader(url.Values{"description": {text}}.Encode()))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	blockBody, err := json.Marshal(map[string]any{
		"noteId": blockNote.ID, "type": "text", "position": "a",
		"content": map[string]string{"text": text},
	})
	require.NoError(t, err)
	rr = doReq(tc, http.MethodPost, "/v1/note/block",
		map[string]string{"Accept": "application/json", "Authorization": "Bearer " + userToken, "Content-Type": "application/json"},
		nil, strings.NewReader(string(blockBody)))
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var stored models.Note
	require.NoError(t, tc.DB.First(&stored, descNote.ID).Error)
	require.Equal(t, text, stored.Description, "the text is stored verbatim; only rendering changes")

	return storedXSSFixture{
		descNote:    descNote,
		blockNote:   blockNote,
		adminHeader: map[string]string{"Accept": "text/html", "Authorization": "Bearer " + adminToken},
	}
}

func adminGet(t *testing.T, tc *TestContext, f storedXSSFixture, path string) string {
	t.Helper()
	rr := doReq(tc, http.MethodGet, path, f.adminHeader, nil, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	return rr.Body.String()
}

func TestStoredHTML_AuthOn_ScopedUserPayloadIsInertForAdmin(t *testing.T) {
	tc := setupAuthEnv(t)
	f := newStoredXSSFixture(t, tc, storedXSSPayload)

	descPage := adminGet(t, tc, f, fmt.Sprintf("/note?id=%d", f.descNote.ID))
	require.Contains(t, descPage, "data-description-display", "positive control: the Description rendered")
	assertNoLiveStoredXSS(t, "/note (Description)", descPage)

	blockPage := adminGet(t, tc, f, fmt.Sprintf("/note?id=%d", f.blockNote.ID))
	require.Contains(t, blockPage, "data-rendered-text-block=", "positive control: the block rendered")
	assertNoLiveStoredXSS(t, "/note (text block)", blockPage)

	blocks := adminGet(t, tc, f, fmt.Sprintf("/v1/note/renderedTextBlocks?noteId=%d", f.blockNote.ID))
	require.Contains(t, blocks, "data-rendered-text-block=")
	assertNoLiveStoredXSS(t, "/v1/note/renderedTextBlocks", blocks)

	assertNoLiveStoredXSS(t, "/note/text", adminGet(t, tc, f, fmt.Sprintf("/note/text?id=%d", f.descNote.ID)))

	// The editor's live preview renders what the page would.
	preview := doReq(tc, http.MethodPost, fmt.Sprintf("/v1/note/previewDescription?id=%d", f.descNote.ID),
		map[string]string{"Accept": "application/json", "Content-Type": "application/x-www-form-urlencoded", "Authorization": f.adminHeader["Authorization"]},
		nil, strings.NewReader(url.Values{"text": {storedXSSPayload}}.Encode()))
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	var previewResp struct{ HTML string }
	require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &previewResp))
	require.Contains(t, previewResp.HTML, "<img", "positive control: the preview rendered")
	assertNoLiveStoredXSS(t, "previewDescription", previewResp.HTML)

	// The public share page renders the same Description and block.
	assertNoLiveStoredXSS(t, "share page (Description)", renderShare(t, tc, f.descNote.ID))
	assertNoLiveStoredXSS(t, "share page (text block)", renderShare(t, tc, f.blockNote.ID))
}

// Ordinary markdown, mentions and shortcodes keep working under auth. The
// sanitizer runs before shortcodes expand, so their trusted output is not
// stripped.
func TestStoredHTML_AuthOn_MarkdownMentionsAndShortcodesSurvive(t *testing.T) {
	tc := setupAuthEnv(t)
	mentioned := &models.Group{Name: "Mentioned Group"}
	require.NoError(t, tc.DB.Create(mentioned).Error)
	text := fmt.Sprintf("**bold** [link](https://example.com/a) ![pic](/v1/resource/preview?id=1)\n\n"+
		"| L | R |\n|:--|--:|\n| a | b |\n\n```go\nx := 1\n```\n\n- [x] done\n\n"+
		"Name=[property path=\"Name\"] @[group:%d:Mentioned Group]\n\n"+
		"[meta path=\"rating\" editable=\"true\"]\n\n[evil](javascript:alert(1))", mentioned.ID)
	f := newStoredXSSFixture(t, tc, text)

	rendered := renderedBlocks(t, doReq(tc, http.MethodGet, fmt.Sprintf("/v1/note/renderedTextBlocks?noteId=%d", f.blockNote.ID), f.adminHeader, nil, nil))
	require.Len(t, rendered, 1)
	var b string
	for _, v := range rendered {
		b = v
	}
	for _, want := range []string{
		"<strong>bold</strong>",
		`<a href="https://example.com/a">link</a>`,
		`<img src="/v1/resource/preview?id=1" alt="pic">`,
		`<th style="text-align:left">L</th>`,
		`<code class="language-go">`,
		`<input checked="" disabled="" type="checkbox">`,
		"Name=xss-block-note", // [property] expanded against the note
		fmt.Sprintf(`<a class="mention-badge mention-group" href="/group?id=%d">Mentioned Group</a>`, mentioned.ID),
		"<meta-shortcode", // expanded after sanitizing, which would have stripped this element
	} {
		if !strings.Contains(b, want) {
			t.Errorf("rendered block lacks %q:\n%s", want, b)
		}
	}
	if strings.Contains(b, "javascript:") {
		t.Errorf("javascript: URL survived: %s", b)
	}

	descPage := adminGet(t, tc, f, fmt.Sprintf("/note?id=%d", f.descNote.ID))
	require.Contains(t, descPage, "Name=xss-desc-note", "a shortcode in the Description still expands under auth")
}

// KAN-4 is unchanged with auth off: raw HTML in a Description and a text block
// renders as written.
func TestStoredHTML_AuthOff_RawHTMLStillRenders(t *testing.T) {
	tc := SetupTestEnv(t)
	descNote := &models.Note{Name: "raw-desc", Description: storedXSSPayload}
	require.NoError(t, tc.DB.Create(descNote).Error)
	blockNote := &models.Note{Name: "raw-block"}
	require.NoError(t, tc.DB.Create(blockNote).Error)
	tc.CreateDummyBlock(blockNote.ID, "text", textBlockJSON(t, storedXSSPayload), "a")

	for _, path := range []string{
		fmt.Sprintf("/note?id=%d", descNote.ID),
		fmt.Sprintf("/note?id=%d", blockNote.ID),
		fmt.Sprintf("/note/text?id=%d", descNote.ID),
		fmt.Sprintf("/v1/note/renderedTextBlocks?noteId=%d", blockNote.ID),
	} {
		rr := doReq(tc, http.MethodGet, path, map[string]string{"Accept": "text/html"}, nil, nil)
		require.Equal(t, http.StatusOK, rr.Code, path)
		require.Len(t, liveStoredXSS(t, rr.Body.String()), 3, "%s: all three raw elements render with auth off", path)
	}
	require.Len(t, liveStoredXSS(t, renderShare(t, tc, descNote.ID)), 3, "share page, auth off")
}

// hostileValue is entity data a scoped user controls (a Name, a Meta value, a
// Description) carrying the payload.
const hostileValue = `<img src=x onerror="window.__xss=1">`

// deferredTokenRe finds every sealed token a rendered page carries: the
// [lazy]/[details]/[autorefresh] bodies and the reloadable region of a slot
// holding a [reload], all rendered later by /v1/shortcodes/deferred.
var deferredTokenRe = regexp.MustCompile(`data-(?:token|shortcode-region)="([^"]+)"`)

// renderDeferred renders every sealed token in body through the deferred
// endpoint as the given principal and returns the rendered bodies.
func renderDeferred(t *testing.T, tc *TestContext, headers map[string]string, body string) []string {
	t.Helper()
	var out []string
	for _, m := range deferredTokenRe.FindAllStringSubmatch(body, -1) {
		reqBody, err := json.Marshal(map[string]string{"token": html.UnescapeString(m[1])})
		require.NoError(t, err)
		rr := doReq(tc, http.MethodPost, "/v1/shortcodes/deferred",
			map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": headers["Authorization"]},
			nil, strings.NewReader(string(reqBody)))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var resp struct{ HTML string }
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		out = append(out, resp.HTML)
	}
	return out
}

// A shortcode with raw output, written in a Description or text block, must not
// put the entity data it reads back on the page unescaped: the sanitizer ran
// before shortcodes expanded, so whatever a raw shortcode emits is never
// checked. Under auth, raw is ignored everywhere a render that started in user
// content reaches, including bodies sealed for a later request.
func TestStoredHTML_AuthOn_RawShortcodesInUserContentStayEscaped(t *testing.T) {
	meta := fmt.Sprintf(`{"x":%q,"list":[%q]}`, hostileValue, hostileValue)
	cases := []struct {
		name string
		text string
	}{
		{"property Description raw", hostileValue + ` [property path="Description" raw="true"]`},
		{"property Name raw", `[property path="Name" raw="true"]`},
		{"meta inline raw", `[meta path="x" inline="true" raw="true"]`},
		{"item raw", `[each path="list"][item raw="true"][/each]`},
		{"mrql item template raw", `[mrql query="type = note" format="custom"][property path="Name" raw="true"][/mrql]`},
		{"lazy body raw", `[lazy][property path="Name" raw="true"][/lazy]`},
		{"details body raw", `[details summary="s"][meta path="x" inline="true" raw="true"][/details]`},
		{"autorefresh body raw", `[autorefresh][property path="Name" raw="true"][/autorefresh]`},
		{"reload region raw", `[reload] [property path="Name" raw="true"]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc := setupAuthEnv(t)
			f := newStoredXSSFixture(t, tc, c.text)
			// The scoped user can rename and set Meta on notes in its subtree;
			// stored directly here to keep the fixture to the rendering.
			for _, n := range []*models.Note{f.descNote, f.blockNote} {
				require.NoError(t, tc.DB.Model(&models.Note{}).Where("id = ?", n.ID).
					Updates(map[string]any{"name": hostileValue, "meta": meta}).Error)
			}
			require.NoError(t, tc.DB.Model(&models.Note{}).Where("id = ?", f.blockNote.ID).
				Update("description", hostileValue).Error)

			pages := map[string]string{
				"/note (Description)": adminGet(t, tc, f, fmt.Sprintf("/note?id=%d", f.descNote.ID)),
				"/note (text block)":  adminGet(t, tc, f, fmt.Sprintf("/note?id=%d", f.blockNote.ID)),
				"/v1/note/renderedTextBlocks": adminGet(t, tc, f,
					fmt.Sprintf("/v1/note/renderedTextBlocks?noteId=%d", f.blockNote.ID)),
			}
			preview := doReq(tc, http.MethodPost, fmt.Sprintf("/v1/note/previewDescription?id=%d", f.descNote.ID),
				map[string]string{"Accept": "application/json", "Content-Type": "application/x-www-form-urlencoded", "Authorization": f.adminHeader["Authorization"]},
				nil, strings.NewReader(url.Values{"text": {c.text}}.Encode()))
			require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
			var previewResp struct{ HTML string }
			require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &previewResp))
			pages["previewDescription"] = previewResp.HTML

			for where, body := range pages {
				assertNoLiveStoredXSS(t, where, body)
				rendered := body
				for i, deferred := range renderDeferred(t, tc, f.adminHeader, body) {
					assertNoLiveStoredXSS(t, fmt.Sprintf("%s, deferred body %d", where, i), deferred)
					rendered += deferred
				}
				// Positive control: the shortcode did expand, escaped. Pongo2's
				// own escaping writes &quot;, so this form is the shortcode's.
				require.Contains(t, rendered, html.EscapeString(hostileValue), "%s: the shortcode rendered its value escaped", where)
			}
		})
	}
}

// raw keeps working where no user content is involved: with auth off, and in
// a Note Type template (admin-authored) under auth, eagerly and in a deferred
// body.
func TestStoredHTML_RawShortcodeStillWorksOutsideUserContent(t *testing.T) {
	const rawName = `<em class="raw-ok">raw-ok</em>`

	t.Run("auth off, Description", func(t *testing.T) {
		tc := SetupTestEnv(t)
		n := &models.Note{Name: rawName, Description: `[property path="Name" raw="true"]`}
		require.NoError(t, tc.DB.Create(n).Error)
		rr := doReq(tc, http.MethodGet, fmt.Sprintf("/note?id=%d", n.ID), map[string]string{"Accept": "text/html"}, nil, nil)
		require.Equal(t, http.StatusOK, rr.Code)
		require.Contains(t, rr.Body.String(), rawName)
	})

	t.Run("auth on, Note Type template", func(t *testing.T) {
		tc := setupAuthEnv(t)
		f := newStoredXSSFixture(t, tc, `[property path="Name" raw="true"]`)
		nt := &models.NoteType{
			Name:          "raw-template-type",
			CustomHeader:  `<div id="hdr">[property path="Name" raw="true"]</div>`,
			CustomSidebar: `[lazy][property path="Name" raw="true"][/lazy]`,
		}
		require.NoError(t, tc.DB.Create(nt).Error)
		require.NoError(t, tc.DB.Model(&models.Note{}).Where("id = ?", f.descNote.ID).
			Updates(map[string]any{"name": rawName, "note_type_id": nt.ID}).Error)

		page := adminGet(t, tc, f, fmt.Sprintf("/note?id=%d", f.descNote.ID))
		require.Contains(t, page, `<div id="hdr">`+rawName+`</div>`, "raw in an admin template still renders raw under auth")
		require.Contains(t, page, html.EscapeString(rawName), "the same shortcode in the Description is escaped")
		deferred := renderDeferred(t, tc, f.adminHeader, page)
		require.NotEmpty(t, deferred)
		found := false
		for _, d := range deferred {
			found = found || strings.Contains(d, rawName)
		}
		require.True(t, found, "raw in an admin template's [lazy] body still renders raw: %v", deferred)
	})
}
