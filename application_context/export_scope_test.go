package application_context

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"mahresources/archive"
	"mahresources/auth"
	"mahresources/models"
)

// fullBFSRequest enables both traversal branches. They are independently gated
// on Scope.RelatedM2M and Scope.GroupRelations, whose zero value is false — a
// request that sets only RelatedDepth exercises neither, so any confinement
// assertion built on it passes vacuously.
func fullBFSRequest(rootID uint) *ExportRequest {
	return &ExportRequest{
		RootGroupIDs: []uint{rootID},
		RelatedDepth: 2,
		Scope: archive.ExportScope{
			Subtree:        true,
			OwnedResources: true,
			OwnedNotes:     true,
			RelatedM2M:     true,
			GroupRelations: true,
		},
	}
}

// relationOnlyRequest disables Subtree so Phase A seeds the plan with the root
// alone. Anything else that appears must have been discovered by the
// GroupRelation BFS — which is what makes a positive traversal assertion mean
// something. With Subtree enabled, Phase A pre-adds every in-subtree group and
// the assertion passes even if traversal is switched off entirely.
func relationOnlyRequest(rootID uint) *ExportRequest {
	return &ExportRequest{
		RootGroupIDs: []uint{rootID},
		RelatedDepth: 2,
		Scope:        archive.ExportScope{GroupRelations: true},
	}
}

func scopedGuest(scopeGroupID uint) *auth.Principal {
	return &auth.Principal{UserID: 42, Role: models.RoleGuest, ScopeGroupID: &scopeGroupID}
}

func mustCreateForExportScope(t *testing.T, ctx *MahresourcesContext, v any) {
	t.Helper()
	if err := ctx.db.Create(v).Error; err != nil {
		t.Fatalf("create %T: %v", v, err)
	}
}

func planHasGroup(plan *exportPlan, id uint) bool {
	for _, g := range plan.groupIDs {
		if g == id {
			return true
		}
	}
	return false
}

// A group-limited principal must not pull groups outside its subtree into an
// export via typed GroupRelation edges. bfsCollectGroupRelations reads the
// group_relations table, which scopeColumn() does not map, so the scope read
// callback applies no filter there — the target ID must be checked explicitly.
func TestScopedExport_GroupRelationEdgeStaysInSubtree(t *testing.T) {
	ctx := createTestContext(t)

	root := &models.Group{Name: "rel-root"}
	mustCreateForExportScope(t, ctx, root)
	// A child of root, so it is genuinely inside the subtree.
	child := &models.Group{Name: "rel-child", OwnerId: &root.ID}
	mustCreateForExportScope(t, ctx, child)
	outside := &models.Group{Name: "rel-outside-secret"}
	mustCreateForExportScope(t, ctx, outside)

	rt := &models.GroupRelationType{Name: "rel-type"}
	mustCreateForExportScope(t, ctx, rt)
	// One edge that must be followed (in-subtree) and one that must not.
	mustCreateForExportScope(t, ctx, &models.GroupRelation{FromGroupId: &root.ID, ToGroupId: &child.ID, RelationTypeId: &rt.ID})
	mustCreateForExportScope(t, ctx, &models.GroupRelation{FromGroupId: &root.ID, ToGroupId: &outside.ID, RelationTypeId: &rt.ID})

	// Subtree is off, so only BFS can introduce child or outside.
	req := relationOnlyRequest(root.ID)

	// Control: unscoped export follows BOTH edges. Without this, the scoped
	// assertion below could pass simply because the BFS never ran.
	ctrl, err := ctx.buildExportPlan(req)
	if err != nil {
		t.Fatalf("control buildExportPlan: %v", err)
	}
	if !planHasGroup(ctrl, outside.ID) || !planHasGroup(ctrl, child.ID) {
		t.Fatalf("control invalid: unscoped export did not follow both relations "+
			"(groupIDs=%v, want %d and %d); the scoped assertions would prove nothing",
			ctrl.groupIDs, child.ID, outside.ID)
	}

	plan, err := ctx.WithPrincipal(scopedGuest(root.ID)).buildExportPlan(req)
	if err != nil {
		t.Fatalf("scoped buildExportPlan: %v", err)
	}

	if planHasGroup(plan, outside.ID) {
		t.Errorf("out-of-subtree group %d entered a scoped export plan via a GroupRelation edge (groupIDs=%v)",
			outside.ID, plan.groupIDs)
	}
	// Positive: confinement must not be achieved by rejecting every candidate.
	// Phase A cannot have supplied child here (Subtree is off), so its presence
	// proves the scoped BFS actually traversed the visible edge.
	if !planHasGroup(plan, child.ID) {
		t.Errorf("in-subtree relation target %d missing from scoped export plan (groupIDs=%v); "+
			"the fix must filter by visibility, not skip the traversal", child.ID, plan.groupIDs)
	}
	if !plan.shellGroupIDs[child.ID] {
		t.Errorf("in-subtree relation target %d was not recorded as a BFS-discovered shell group "+
			"(shellGroupIDs=%v)", child.ID, plan.shellGroupIDs)
	}
}

// The estimate endpoint never loads group payloads, so it is the reachable
// surface: it returns 200 and its counts must not disclose out-of-subtree
// groups.
func TestScopedExport_EstimateDoesNotCountOutOfSubtreeGroups(t *testing.T) {
	ctx := createTestContext(t)

	root := &models.Group{Name: "est-root"}
	mustCreateForExportScope(t, ctx, root)
	outside := &models.Group{Name: "est-outside-secret"}
	mustCreateForExportScope(t, ctx, outside)

	rt := &models.GroupRelationType{Name: "est-type"}
	mustCreateForExportScope(t, ctx, rt)
	mustCreateForExportScope(t, ctx, &models.GroupRelation{FromGroupId: &root.ID, ToGroupId: &outside.ID, RelationTypeId: &rt.ID})

	est, err := ctx.WithPrincipal(scopedGuest(root.ID)).EstimateExport(fullBFSRequest(root.ID))
	if err != nil {
		t.Fatalf("EstimateExport: %v", err)
	}
	if est.Counts.Groups != 1 {
		t.Errorf("scoped estimate counted %d groups, want 1 (only the root)", est.Counts.Groups)
	}
	if est.Counts.ShellGroups != 0 {
		t.Errorf("scoped estimate reported %d shell groups, want 0 — the count discloses "+
			"how many out-of-subtree groups the caller's subtree relates to", est.Counts.ShellGroups)
	}
}

// End to end: a scoped export must produce a usable archive whose bytes contain
// no trace of an out-of-subtree group. Before the fix this aborted with
// "load group N: record not found" after writing a manifest naming that group.
func TestScopedExport_ArchiveExcludesOutOfSubtreeGroup(t *testing.T) {
	ctx := createTestContext(t)

	root := &models.Group{Name: "arc-root"}
	mustCreateForExportScope(t, ctx, root)
	outside := &models.Group{Name: "arc-outside-secret"}
	mustCreateForExportScope(t, ctx, outside)

	rt := &models.GroupRelationType{Name: "arc-type"}
	mustCreateForExportScope(t, ctx, rt)
	mustCreateForExportScope(t, ctx, &models.GroupRelation{FromGroupId: &root.ID, ToGroupId: &outside.ID, RelationTypeId: &rt.ID})

	var buf bytes.Buffer
	err := ctx.WithPrincipal(scopedGuest(root.ID)).
		StreamExport(context.Background(), fullBFSRequest(root.ID), &buf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("scoped StreamExport failed: %v", err)
	}
	// The manifest is written from plan state before payloads load, so checking
	// only the payload files would miss a leaked name.
	if bytes.Contains(buf.Bytes(), []byte("arc-outside-secret")) {
		t.Error("out-of-subtree group name present in scoped export archive bytes")
	}
	if !bytes.Contains(buf.Bytes(), []byte("arc-root")) {
		t.Error("in-subtree root group missing from scoped export archive bytes")
	}
}

// A relationship whose target was filtered out must not survive as a targetless
// payload. The group_relations row stays readable (that table is not
// scope-mapped), so loadGroupPayload would otherwise emit a GroupRelationPayload
// with neither ToRef nor DanglingRef: invalid per the archive contract and
// silently dropped by the importer.
//
// Note the dangling path is not an acceptable alternative here — a DanglingRef
// stub carries the target's Name, so emitting one would leak the very thing the
// confinement fix suppresses. Scoped exports must omit the relationship.
func TestScopedExport_NoTargetlessRelationshipPayload(t *testing.T) {
	ctx := createTestContext(t)

	root := &models.Group{Name: "tr-root"}
	mustCreateForExportScope(t, ctx, root)
	child := &models.Group{Name: "tr-child", OwnerId: &root.ID}
	mustCreateForExportScope(t, ctx, child)
	outside := &models.Group{Name: "tr-outside-secret"}
	mustCreateForExportScope(t, ctx, outside)

	rt := &models.GroupRelationType{Name: "tr-type"}
	mustCreateForExportScope(t, ctx, rt)
	mustCreateForExportScope(t, ctx, &models.GroupRelation{FromGroupId: &root.ID, ToGroupId: &child.ID, RelationTypeId: &rt.ID})
	mustCreateForExportScope(t, ctx, &models.GroupRelation{FromGroupId: &root.ID, ToGroupId: &outside.ID, RelationTypeId: &rt.ID})

	var buf bytes.Buffer
	if err := ctx.WithPrincipal(scopedGuest(root.ID)).
		StreamExport(context.Background(), fullBFSRequest(root.ID), &buf, func(ev ProgressEvent) {}); err != nil {
		t.Fatalf("scoped StreamExport: %v", err)
	}

	payload := groupPayloadFromTar(t, &buf, "groups/g0001.json")
	var got struct {
		Relationships []struct {
			ToRef       string `json:"to_ref"`
			DanglingRef string `json:"dangling_ref"`
			TypeName    string `json:"type_name"`
		} `json:"relationships"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal group payload: %v", err)
	}

	if len(got.Relationships) == 0 {
		t.Fatal("no relationships in payload; the in-subtree relation should have survived")
	}
	for i, r := range got.Relationships {
		if r.ToRef == "" && r.DanglingRef == "" {
			t.Errorf("relationship %d has neither to_ref nor dangling_ref: %+v", i, r)
		}
	}
	if len(got.Relationships) != 1 {
		t.Errorf("got %d relationships, want 1 (only the in-subtree edge)", len(got.Relationships))
	}
}

// groupPayloadFromTar extracts one JSON entry from an uncompressed export tar.
func groupPayloadFromTar(t *testing.T, buf *bytes.Buffer, name string) []byte {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		if h.Name == name {
			b, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			return b
		}
	}
	t.Fatalf("%s not found in archive", name)
	return nil
}

// bfsEnsureOwners reads from resources/notes, which scopeColumn() does map, so
// its owner IDs are already confined. Pinned so a future scopeColumn change
// cannot silently open a second path.
func TestScopedExport_OwnerBackfillStaysInSubtree(t *testing.T) {
	ctx := createTestContext(t)

	root := &models.Group{Name: "own-root"}
	mustCreateForExportScope(t, ctx, root)
	outside := &models.Group{Name: "own-outside-secret"}
	mustCreateForExportScope(t, ctx, outside)

	outRes := &models.Resource{Name: "own-outside-res", OwnerId: &outside.ID, Hash: "cafebabe", FileSize: 7}
	mustCreateForExportScope(t, ctx, outRes)
	if err := ctx.db.Model(root).Association("RelatedResources").Append(outRes); err != nil {
		t.Fatalf("relate resource: %v", err)
	}

	plan, err := ctx.WithPrincipal(scopedGuest(root.ID)).buildExportPlan(fullBFSRequest(root.ID))
	if err != nil {
		t.Fatalf("scoped buildExportPlan: %v", err)
	}
	if planHasGroup(plan, outside.ID) {
		t.Errorf("out-of-subtree owner group %d entered a scoped export plan (groupIDs=%v)",
			outside.ID, plan.groupIDs)
	}
}
