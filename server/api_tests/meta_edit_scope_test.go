package api_tests

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
)

// metaScopeFixture is a root group with a note and a resource inside it, and an
// unrelated top-level group with a note and a resource of its own. Every row
// carries Meta, so a leak through the editMeta response is visible.
type metaScopeFixture struct {
	rootID, rootNoteID, rootResourceID       uint
	outsideID, outNoteID, outResourceID      uint
	outGroupMeta, outNoteMeta, outResMeta    string
	rootGroupMeta, rootNoteMeta, rootResMeta string
}

func buildMetaScopeFixture(t *testing.T, tc *TestContext) metaScopeFixture {
	t.Helper()
	f := metaScopeFixture{
		rootGroupMeta: `{"inside":"g"}`, rootNoteMeta: `{"inside":"n"}`, rootResMeta: `{"inside":"r"}`,
		outGroupMeta: `{"secret":"s"}`, outNoteMeta: `{"secret":"note-s"}`, outResMeta: `{"secret":"res-s"}`,
	}
	create := func(v any) {
		t.Helper()
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatalf("create %T: %v", v, err)
		}
	}
	root := &models.Group{Name: "ms-root", Meta: types.JSON(f.rootGroupMeta)}
	create(root)
	outside := &models.Group{Name: "ms-outside", Meta: types.JSON(f.outGroupMeta)}
	create(outside)
	rootNote := &models.Note{Name: "ms-root-note", OwnerId: &root.ID, Meta: types.JSON(f.rootNoteMeta)}
	create(rootNote)
	outNote := &models.Note{Name: "ms-out-note", OwnerId: &outside.ID, Meta: types.JSON(f.outNoteMeta)}
	create(outNote)
	rootRes := &models.Resource{Name: "ms-root-res", OwnerId: &root.ID, Location: "ms-root.txt", Meta: types.JSON(f.rootResMeta)}
	create(rootRes)
	outRes := &models.Resource{Name: "ms-out-res", OwnerId: &outside.ID, Location: "ms-out.txt", Meta: types.JSON(f.outResMeta)}
	create(outRes)

	f.rootID, f.rootNoteID, f.rootResourceID = root.ID, rootNote.ID, rootRes.ID
	f.outsideID, f.outNoteID, f.outResourceID = outside.ID, outNote.ID, outRes.ID
	return f
}

// metaScopeCase is one entity of the fixture: its route name, table, the id
// outside the subtree and the id inside it.
type metaScopeCase struct {
	entity, table   string
	outID, inID     uint
	outMeta, inMeta string
}

func (f metaScopeFixture) cases() []metaScopeCase {
	return []metaScopeCase{
		{"group", "groups", f.outsideID, f.rootID, f.outGroupMeta, f.rootGroupMeta},
		{"note", "notes", f.outNoteID, f.rootNoteID, f.outNoteMeta, f.rootNoteMeta},
		{"resource", "resources", f.outResourceID, f.rootResourceID, f.outResMeta, f.rootResMeta},
	}
}

// storedMeta reads a row's Meta through the unscoped test handle.
func storedMeta(t *testing.T, tc *TestContext, table string, id uint) map[string]any {
	t.Helper()
	var rows []struct{ Meta types.JSON }
	if err := tc.DB.Table(table).Select("meta").Where("id = ?", id).Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatalf("read %s %d meta: rows=%d err=%v", table, id, len(rows), err)
	}
	var m map[string]any
	if err := json.Unmarshal(rows[0].Meta, &m); err != nil {
		t.Fatalf("decode %s %d meta %q: %v", table, id, rows[0].Meta, err)
	}
	return m
}

func assertMetaUnchanged(t *testing.T, tc *TestContext, c metaScopeCase) {
	t.Helper()
	var want map[string]any
	_ = json.Unmarshal([]byte(c.outMeta), &want)
	got := storedMeta(t, tc, c.table, c.outID)
	if len(got) != len(want) || got["secret"] != want["secret"] {
		t.Fatalf("%s %d meta changed: got %v, want %v", c.entity, c.outID, got, want)
	}
}

// TestEditMeta_ScopedUserConfinedToSubtree pins issue #93: a group-limited user
// could write a Meta key on any group, note or resource by id, and the response
// handed back the entity's whole Meta.
func TestEditMeta_ScopedUserConfinedToSubtree(t *testing.T) {
	tc := setupAuthEnv(t)
	requireJsonPatch(t, tc.DB)
	f := buildMetaScopeFixture(t, tc)
	h := map[string]string{
		"Accept":        "application/json",
		"Authorization": scopedUserBearer(t, tc, f.rootID),
		"Content-Type":  "application/x-www-form-urlencoded",
	}
	form := url.Values{"path": {"pwned"}, "value": {"1"}}.Encode()

	for _, c := range f.cases() {
		t.Run(c.entity, func(t *testing.T) {
			rr := doReq(tc, http.MethodPost, "/v1/"+c.entity+"/editMeta?id="+itoa(int(c.outID)), h, nil, strings.NewReader(form))
			if rr.Code != http.StatusNotFound {
				t.Fatalf("out-of-subtree editMeta: status %d, want 404; body %s", rr.Code, rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "secret") {
				t.Fatalf("out-of-subtree editMeta leaked Meta: %s", rr.Body.String())
			}
			assertMetaUnchanged(t, tc, c)

			// Positive control: the same request inside the subtree succeeds.
			rr = doReq(tc, http.MethodPost, "/v1/"+c.entity+"/editMeta?id="+itoa(int(c.inID)), h, nil, strings.NewReader(form))
			if rr.Code != http.StatusOK {
				t.Fatalf("in-subtree editMeta: status %d, want 200; body %s", rr.Code, rr.Body.String())
			}
			got := storedMeta(t, tc, c.table, c.inID)
			if got["pwned"] != float64(1) || got["inside"] == nil {
				t.Fatalf("in-subtree editMeta not persisted with siblings: %v", got)
			}
		})
	}
}

// assertScopedUpdateMetaAtPathConfined drives UpdateMetaAtPath directly with a
// principal scoped to the fixture's root, so the same assertions cover the
// SQLite and the Postgres (FOR UPDATE) branches.
func assertScopedUpdateMetaAtPathConfined(t *testing.T, tc *TestContext) {
	t.Helper()
	f := buildMetaScopeFixture(t, tc)
	scoped := tc.AppCtx.WithPrincipal(&auth.Principal{UserID: 9, Role: models.RoleUser, ScopeGroupID: &f.rootID})
	update := func(table string, id uint) (json.RawMessage, error) {
		switch table {
		case "groups":
			return application_context.NewEntityWriter[models.Group](scoped).UpdateMetaAtPath(id, "pwned", json.RawMessage("1"))
		case "notes":
			return application_context.NewEntityWriter[models.Note](scoped).UpdateMetaAtPath(id, "pwned", json.RawMessage("1"))
		default:
			return application_context.NewEntityWriter[models.Resource](scoped).UpdateMetaAtPath(id, "pwned", json.RawMessage("1"))
		}
	}
	for _, c := range f.cases() {
		out, err := update(c.table, c.outID)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("%s outside subtree: err=%v meta=%s, want gorm.ErrRecordNotFound", c.entity, err, out)
		}
		if strings.Contains(string(out), "secret") {
			t.Fatalf("%s outside subtree leaked Meta: %s", c.entity, out)
		}
		assertMetaUnchanged(t, tc, c)

		in, err := update(c.table, c.inID)
		if err != nil {
			t.Fatalf("%s inside subtree: %v", c.entity, err)
		}
		if !strings.Contains(string(in), `"pwned":1`) {
			t.Fatalf("%s inside subtree: response %s lacks the new key", c.entity, in)
		}
		if got := storedMeta(t, tc, c.table, c.inID); got["pwned"] != float64(1) || got["inside"] == nil {
			t.Fatalf("%s inside subtree not persisted with siblings: %v", c.entity, got)
		}
	}
}

func TestUpdateMetaAtPath_ScopedPrincipalConfined(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)
	assertScopedUpdateMetaAtPathConfined(t, tc)
}
