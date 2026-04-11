package archive

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func writeFixtureArchive(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, false)
	_ = w.WriteManifest(&Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC),
		CreatedBy:     "mahresources",
		Roots:         []string{"g0001"},
		Counts:        Counts{Groups: 1, Resources: 1, Blobs: 1},
		Entries: Entries{
			Groups:    []GroupEntry{{ExportID: "g0001", Name: "Books", SourceID: 17, Path: "groups/g0001.json"}},
			Resources: []ResourceEntry{{ExportID: "r0001", Name: "cover.jpg", SourceID: 9001, Owner: "g0001", Hash: "abc", Path: "resources/r0001.json"}},
		},
	})
	_ = w.WriteGroup(&GroupPayload{ExportID: "g0001", SourceID: 17, Name: "Books"})
	_ = w.WriteResource(&ResourcePayload{ExportID: "r0001", SourceID: 9001, Name: "cover.jpg", Hash: "abc", BlobRef: "abc"})
	_ = w.WriteBlob("abc", strings.NewReader("PNGDATA"), 7)
	_ = w.Close()
	return &buf
}

func TestReader_ReadManifest(t *testing.T) {
	buf := writeFixtureArchive(t)
	r, err := NewReader(buf)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer r.Close()

	m, err := r.ReadManifest()
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if m.SchemaVersion != SchemaVersion {
		t.Fatalf("schema = %d", m.SchemaVersion)
	}
	if len(m.Entries.Groups) != 1 || m.Entries.Groups[0].ExportID != "g0001" {
		t.Fatalf("entries = %+v", m.Entries.Groups)
	}
}

// testCollector implements every Visitor hook and keeps the decoded entries
// in maps for spot-checks in round-trip tests. Blob and preview bodies are
// drained into byte slices so the tar reader can advance.
type testCollector struct {
	groups    map[string]*GroupPayload
	notes     map[string]*NotePayload
	resources map[string]*ResourcePayload
	series    map[string]*SeriesPayload
	blobs     map[string][]byte
	previews  map[string][]byte

	categoryDefs         []CategoryDef
	noteTypeDefs         []NoteTypeDef
	resourceCategoryDefs []ResourceCategoryDef
	tagDefs              []TagDef
	grtDefs              []GroupRelationTypeDef
}

func newTestCollector() *testCollector {
	return &testCollector{
		groups:    map[string]*GroupPayload{},
		notes:     map[string]*NotePayload{},
		resources: map[string]*ResourcePayload{},
		series:    map[string]*SeriesPayload{},
		blobs:     map[string][]byte{},
		previews:  map[string][]byte{},
	}
}

func (c *testCollector) OnGroup(p *GroupPayload) error       { c.groups[p.ExportID] = p; return nil }
func (c *testCollector) OnNote(p *NotePayload) error         { c.notes[p.ExportID] = p; return nil }
func (c *testCollector) OnResource(p *ResourcePayload) error { c.resources[p.ExportID] = p; return nil }
func (c *testCollector) OnSeries(p *SeriesPayload) error     { c.series[p.ExportID] = p; return nil }

func (c *testCollector) OnBlob(hash string, body io.Reader, size int64) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	c.blobs[hash] = data
	return nil
}

func (c *testCollector) OnPreview(id string, body io.Reader, size int64) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	c.previews[id] = data
	return nil
}

func (c *testCollector) OnCategoryDefs(defs []CategoryDef) error { c.categoryDefs = defs; return nil }
func (c *testCollector) OnNoteTypeDefs(defs []NoteTypeDef) error { c.noteTypeDefs = defs; return nil }
func (c *testCollector) OnResourceCategoryDefs(defs []ResourceCategoryDef) error {
	c.resourceCategoryDefs = defs
	return nil
}
func (c *testCollector) OnTagDefs(defs []TagDef) error { c.tagDefs = defs; return nil }
func (c *testCollector) OnGroupRelationTypeDefs(defs []GroupRelationTypeDef) error {
	c.grtDefs = defs
	return nil
}

func TestReader_RejectsUnknownSchemaVersion(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, false)
	_ = w.WriteManifest(&Manifest{SchemaVersion: 999})
	_ = w.Close()

	r, _ := NewReader(&buf)
	_, err := r.ReadManifest()
	if err == nil {
		t.Fatalf("ReadManifest accepted future version")
	}
	var verErr *ErrUnsupportedSchemaVersion
	if !errors.As(err, &verErr) {
		t.Fatalf("error type = %T, want ErrUnsupportedSchemaVersion", err)
	}
	if verErr.Got != 999 {
		t.Fatalf("err.Got = %d", verErr.Got)
	}
}

func TestReader_TolerantToUnknownTopLevelKeys(t *testing.T) {
	// Build a manifest by hand with extra fields.
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	body := []byte(`{"schema_version":1,"created_by":"future","mystery_field":"hello"}`)
	_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Size: int64(len(body)), Mode: 0644})
	_, _ = tw.Write(body)
	_ = tw.Close()

	r, _ := NewReader(&raw)
	m, err := r.ReadManifest()
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if m.SchemaVersion != 1 {
		t.Fatalf("schema = %d", m.SchemaVersion)
	}
}

func TestReader_WalkDispatchesToVisitor(t *testing.T) {
	buf := writeFixtureArchive(t)
	r, _ := NewReader(buf)
	defer r.Close()

	if _, err := r.ReadManifest(); err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}

	c := newTestCollector()
	if err := r.Walk(c); err != nil {
		t.Fatalf("Walk: %v", err)
	}

	g, ok := c.groups["g0001"]
	if !ok || g.Name != "Books" {
		t.Fatalf("groups = %+v", c.groups)
	}

	res, ok := c.resources["r0001"]
	if !ok || res.Hash != "abc" {
		t.Fatalf("resources = %+v", c.resources)
	}

	blob, ok := c.blobs["abc"]
	if !ok {
		t.Fatalf("blob missing")
	}
	if string(blob) != "PNGDATA" {
		t.Fatalf("blob = %q", blob)
	}
}

func TestReader_WalkBeforeManifestIsError(t *testing.T) {
	buf := writeFixtureArchive(t)
	r, _ := NewReader(buf)
	defer r.Close()
	if err := r.Walk(newTestCollector()); err == nil {
		t.Fatalf("Walk without ReadManifest should fail")
	}
}

func TestReader_WalkOnceOnly(t *testing.T) {
	buf := writeFixtureArchive(t)
	r, _ := NewReader(buf)
	defer r.Close()
	if _, err := r.ReadManifest(); err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if err := r.Walk(newTestCollector()); err != nil {
		t.Fatalf("first Walk: %v", err)
	}
	if err := r.Walk(newTestCollector()); err == nil {
		t.Fatalf("second Walk should fail")
	}
}
