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

// TestScopedUser_PluginBlockRenderConfined proves the block-render endpoint
// enforces subtree scope: an out-of-subtree block is not found, an in-subtree
// block reaches the handler past the visibility gate, and an admin is never
// scope-limited.
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

	// The block type intentionally does NOT match the URL plugin prefix, so a
	// VISIBLE block yields a deterministic 400 ("block type does not belong to
	// this plugin") once it passes the visibility gate, while an INVISIBLE block
	// must short-circuit to 404 ("block not found").
	inBlock := tc.CreateDummyBlock(inNote.ID, "other:foo", "{}", "a")
	outBlock := tc.CreateDummyBlock(outNote.ID, "other:foo", "{}", "a")

	hdr := map[string]string{"Authorization": scopedUserBearer(t, tc, root.ID)}
	renderPath := func(blockID uint) string {
		return fmt.Sprintf("/v1/plugins/myplugin/block/render?blockId=%d&mode=view", blockID)
	}

	// Out-of-subtree block → not found (leak closed).
	if rr := doReq(tc, http.MethodGet, renderPath(outBlock.ID), hdr, nil, nil); rr.Code != http.StatusNotFound {
		t.Fatalf("out-of-subtree block render must be 404, got %d (%s)", rr.Code, rr.Body.String())
	}

	// In-subtree block → passes the scope gate (400 for the mismatched type),
	// proving the fix does not over-block legitimate blocks.
	if rr := doReq(tc, http.MethodGet, renderPath(inBlock.ID), hdr, nil, nil); rr.Code != http.StatusBadRequest {
		t.Fatalf("in-subtree block render should pass the scope gate (400 mismatched type), got %d (%s)", rr.Code, rr.Body.String())
	}

	// Admin is never scope-limited: even the out-of-subtree block is reachable.
	adminHdr := map[string]string{"Authorization": roleBearer(t, tc, models.RoleAdmin)}
	if rr := doReq(tc, http.MethodGet, renderPath(outBlock.ID), adminHdr, nil, nil); rr.Code == http.StatusNotFound {
		t.Fatalf("admin must reach any block; got 404 (%s)", rr.Body.String())
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
