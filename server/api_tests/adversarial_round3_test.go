package api_tests

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"mahresources/auth"
	"mahresources/models"
)

// Round-3 adversarial findings:
//
//   - Plugin block rendering ignored group scope: the /v1/plugins/{name}/block/render
//     route passed the unscoped singleton context, so GetBlock/GetNote ran without
//     the caller's subtree filter and a group-limited principal could render an
//     out-of-subtree plugin block (and leak its note name) by guessing the block ID.
//   - Thumbnail mutations lost the GORM scope filter: SetCustomThumbnail/ClearThumbnails
//     re-bound the DB to the bare HTTP request context, dropping the scope value, so a
//     group-limited principal could set or clear previews for an arbitrary resource ID.
//
// Both must be confined to the caller's subtree; admins/unscoped principals are
// unaffected.

// TestScopedUser_PluginBlockRenderConfined proves the block-render endpoint is
// closed to group-confined principals. Plugin code runs against an unscoped DB
// handle, so confined principals are denied ALL plugin-code endpoints
// (fail-closed) regardless of whether the target block is inside their subtree;
// admins/unscoped principals are unaffected. The handler also still re-binds to
// the scoped context (defense-in-depth) for any future role that may reach it.
func TestScopedUser_PluginBlockRenderConfined(t *testing.T) {
	tc := setupAuthEnv(t)

	root := &models.Group{Name: "pbr-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "pbr-outside"}
	tc.DB.Create(outside)

	inNote := &models.Note{Name: "pbr-in", OwnerId: &root.ID}
	tc.DB.Create(inNote)
	outNote := &models.Note{Name: "pbr-out", OwnerId: &outside.ID}
	tc.DB.Create(outNote)

	inBlock := tc.CreateDummyBlock(inNote.ID, "other:foo", "{}", "a")
	outBlock := tc.CreateDummyBlock(outNote.ID, "other:foo", "{}", "a")

	hdr := map[string]string{"Authorization": scopedUserBearer(t, tc, root.ID)}
	renderPath := func(blockID uint) string {
		return fmt.Sprintf("/v1/plugins/myplugin/block/render?blockId=%d&mode=view", blockID)
	}

	// A confined principal is denied the plugin-code endpoint outright, for both
	// in- and out-of-subtree blocks (no information leak about either).
	for _, b := range []uint{inBlock.ID, outBlock.ID} {
		if rr := doReq(tc, http.MethodGet, renderPath(b), hdr, nil, nil); rr.Code != http.StatusForbidden {
			t.Fatalf("scoped user block render must be 403 (confined), got %d (%s)", rr.Code, rr.Body.String())
		}
	}

	// Admin is never confined: the out-of-subtree block is reachable (passes the
	// confinement guard; the mismatched plugin type yields 400, never 403/404).
	adminHdr := map[string]string{"Authorization": roleBearer(t, tc, models.RoleAdmin)}
	if rr := doReq(tc, http.MethodGet, renderPath(outBlock.ID), adminHdr, nil, nil); rr.Code == http.StatusNotFound || rr.Code == http.StatusForbidden {
		t.Fatalf("admin must reach any block; got %d (%s)", rr.Code, rr.Body.String())
	}
}

// TestScopedUser_ThumbnailMutationsConfined proves SetCustomThumbnail and
// ClearThumbnails confine a group-limited principal to its subtree, while an
// admin/unscoped context may still mutate any resource.
func TestScopedUser_ThumbnailMutationsConfined(t *testing.T) {
	tc := setupAuthEnv(t)

	root := &models.Group{Name: "tn-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "tn-outside"}
	tc.DB.Create(outside)

	inRes := &models.Resource{Name: "tn-in", OwnerId: &root.ID}
	tc.DB.Create(inRes)
	outRes := &models.Resource{Name: "tn-out", OwnerId: &outside.ID}
	tc.DB.Create(outRes)

	// Seed an existing preview on the out-of-subtree resource so we can prove a
	// scoped ClearThumbnails leaves it untouched.
	outPrev := &models.Preview{ResourceId: &outRes.ID, Data: []byte{1, 2, 3}, ContentType: "image/jpeg"}
	tc.DB.Create(outPrev)

	scoped := tc.AppCtx.WithPrincipal(&auth.Principal{UserID: 7, Role: models.RoleUser, ScopeGroupID: &root.ID})
	bg := context.Background()
	img := func() *bytes.Reader { return bytes.NewReader(createTestPNG(t, 8, 8)) }

	previewCount := func(resID uint) int64 {
		var n int64
		tc.DB.Model(&models.Preview{}).Where("resource_id = ?", resID).Count(&n)
		return n
	}

	// ClearThumbnails on an out-of-subtree resource must fail and delete nothing.
	if err := scoped.ClearThumbnails(bg, outRes.ID); err == nil {
		t.Fatalf("ClearThumbnails on out-of-subtree resource must fail")
	}
	if c := previewCount(outRes.ID); c != 1 {
		t.Fatalf("out-of-subtree preview must be untouched, got %d rows", c)
	}

	// SetCustomThumbnail on an out-of-subtree resource must fail and create nothing.
	if err := scoped.SetCustomThumbnail(bg, outRes.ID, img()); err == nil {
		t.Fatalf("SetCustomThumbnail on out-of-subtree resource must fail")
	}
	if c := previewCount(outRes.ID); c != 1 {
		t.Fatalf("SetCustomThumbnail must not touch an out-of-subtree resource, got %d rows", c)
	}

	// In-subtree resource: SetCustomThumbnail succeeds (replaces previews).
	if err := scoped.SetCustomThumbnail(bg, inRes.ID, img()); err != nil {
		t.Fatalf("SetCustomThumbnail on in-subtree resource should succeed, got %v", err)
	}
	if c := previewCount(inRes.ID); c == 0 {
		t.Fatalf("in-subtree SetCustomThumbnail should create a preview")
	}
	if err := scoped.ClearThumbnails(bg, inRes.ID); err != nil {
		t.Fatalf("ClearThumbnails on in-subtree resource should succeed, got %v", err)
	}
	if c := previewCount(inRes.ID); c != 0 {
		t.Fatalf("in-subtree ClearThumbnails should remove previews, got %d", c)
	}

	// Backstop: an admin/unscoped context may still mutate any resource.
	if err := tc.AppCtx.SetCustomThumbnail(bg, outRes.ID, img()); err != nil {
		t.Fatalf("admin SetCustomThumbnail on any resource should succeed, got %v", err)
	}
	if err := tc.AppCtx.ClearThumbnails(bg, outRes.ID); err != nil {
		t.Fatalf("admin ClearThumbnails on any resource should succeed, got %v", err)
	}
	if c := previewCount(outRes.ID); c != 0 {
		t.Fatalf("admin ClearThumbnails should remove previews, got %d", c)
	}
}
