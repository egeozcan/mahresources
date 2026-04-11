package archive

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRoundTrip_FullManifest(t *testing.T) {
	original := Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC),
		CreatedBy:     "mahresources",
		ExportOptions: ExportOptions{
			Scope:      ExportScope{Subtree: true, OwnedResources: true, OwnedNotes: true, RelatedM2M: true, GroupRelations: true},
			Fidelity:   ExportFidelity{ResourceBlobs: true, ResourceSeries: true},
			SchemaDefs: ExportSchemaDefs{CategoriesAndTypes: true, Tags: true, GroupRelationTypes: true},
		},
		Roots:  []string{"g0001"},
		Counts: Counts{Groups: 2, Notes: 1, Resources: 1, Blobs: 1},
		Entries: Entries{
			Groups: []GroupEntry{
				{ExportID: "g0001", Name: "Root", SourceID: 1, Path: "groups/g0001.json"},
				{ExportID: "g0002", Name: "Child", SourceID: 2, Path: "groups/g0002.json"},
			},
			Notes: []NoteEntry{
				{ExportID: "n0001", Name: "Notes", SourceID: 10, Owner: "g0001", Path: "notes/n0001.json"},
			},
			Resources: []ResourceEntry{
				{ExportID: "r0001", Name: "f.png", SourceID: 100, Owner: "g0001", Hash: "h1", Path: "resources/r0001.json"},
			},
		},
		Dangling: []DanglingRef{
			{ID: "dr0001", Kind: DanglingKindRelatedGroup, From: "g0001", ToStub: DanglingStub{SourceID: 99, Name: "Outsider", Reason: "out_of_scope"}},
		},
	}

	var buf bytes.Buffer
	w, _ := NewWriter(&buf, false)
	_ = w.WriteManifest(&original)
	_ = w.WriteGroup(&GroupPayload{ExportID: "g0001", SourceID: 1, Name: "Root"})
	_ = w.WriteGroup(&GroupPayload{ExportID: "g0002", SourceID: 2, Name: "Child", OwnerRef: "g0001"})
	_ = w.WriteNote(&NotePayload{ExportID: "n0001", SourceID: 10, Name: "Notes", OwnerRef: "g0001"})
	_ = w.WriteResource(&ResourcePayload{ExportID: "r0001", SourceID: 100, Name: "f.png", Hash: "h1", BlobRef: "h1", OwnerRef: "g0001"})
	_ = w.WriteBlob("h1", strings.NewReader("PNG"), 3)
	_ = w.Close()

	r, _ := NewReader(&buf)
	defer r.Close()
	got, err := r.ReadManifest()
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}

	if got.SchemaVersion != SchemaVersion {
		t.Fatalf("schema = %d", got.SchemaVersion)
	}
	if !got.ExportOptions.Scope.Subtree || !got.ExportOptions.Fidelity.ResourceBlobs {
		t.Fatalf("export options round-trip: %+v", got.ExportOptions)
	}
	if len(got.Entries.Groups) != 2 || got.Entries.Groups[1].Name != "Child" {
		t.Fatalf("groups: %+v", got.Entries.Groups)
	}
	if len(got.Dangling) != 1 || got.Dangling[0].Kind != DanglingKindRelatedGroup {
		t.Fatalf("dangling: %+v", got.Dangling)
	}

	// Walk the remainder once with a collecting visitor and spot-check.
	c := newTestCollector()
	if err := r.Walk(c); err != nil {
		t.Fatalf("Walk: %v", err)
	}

	g2, ok := c.groups["g0002"]
	if !ok {
		t.Fatalf("g0002 missing from walk: %+v", c.groups)
	}
	if g2.OwnerRef != "g0001" {
		t.Fatalf("g0002 owner = %q", g2.OwnerRef)
	}

	blob, ok := c.blobs["h1"]
	if !ok {
		t.Fatalf("blob h1 missing")
	}
	if string(blob) != "PNG" {
		t.Fatalf("blob = %q", blob)
	}
}

func TestRoundTrip_GzipPath(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, true)
	_ = w.WriteManifest(&Manifest{SchemaVersion: SchemaVersion, Roots: []string{"g0001"}})
	_ = w.WriteGroup(&GroupPayload{ExportID: "g0001", Name: "X"})
	_ = w.Close()

	r, _ := NewReader(&buf)
	defer r.Close()
	if _, err := r.ReadManifest(); err != nil {
		t.Fatalf("ReadManifest gzip: %v", err)
	}

	c := newTestCollector()
	if err := r.Walk(c); err != nil {
		t.Fatalf("Walk gzip: %v", err)
	}
	g, ok := c.groups["g0001"]
	if !ok || g.Name != "X" {
		t.Fatalf("group collected = %+v", c.groups)
	}
}
