package archive

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Reader streams an archive. Usage:
//
//	r, err := NewReader(src)
//	if err != nil { return err }
//	defer r.Close()
//
//	manifest, err := r.ReadManifest()      // reads first tar entry only
//	if err != nil { return err }
//
//	// Exactly one Walk per Reader. Construct a new Reader from a fresh
//	// source if you need a second pass.
//	if err := r.Walk(myVisitor); err != nil { return err }
//
// Reader is not safe for concurrent use.
type Reader struct {
	tr       *tar.Reader
	gz       *gzip.Reader
	manifest *Manifest
	walked   bool
}

// NewReader detects whether the input is gzipped (magic bytes 0x1f 0x8b) and
// constructs a tar.Reader appropriately. The Reader does not take ownership
// of src — the caller is responsible for closing it if necessary.
func NewReader(src io.Reader) (*Reader, error) {
	pr := &peekedReader{r: src}
	header, _ := pr.Peek(2)
	r := &Reader{}
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		gz, err := gzip.NewReader(pr)
		if err != nil {
			return nil, fmt.Errorf("archive: gzip header invalid: %w", err)
		}
		r.gz = gz
		r.tr = tar.NewReader(gz)
	} else {
		r.tr = tar.NewReader(pr)
	}
	return r, nil
}

// ReadManifest reads the first tar entry and parses it. Must be called
// exactly once per Reader and before Walk. The tar reader's cursor advances
// past the manifest entry only; no other entries are read.
func (r *Reader) ReadManifest() (*Manifest, error) {
	if r.manifest != nil {
		return r.manifest, nil
	}
	hdr, err := r.tr.Next()
	if err != nil {
		return nil, fmt.Errorf("archive: read first entry: %w", err)
	}
	if hdr.Name != "manifest.json" {
		return nil, fmt.Errorf("archive: first entry %q != manifest.json", hdr.Name)
	}
	var m Manifest
	dec := json.NewDecoder(r.tr)
	// Do NOT call DisallowUnknownFields — §6.4 requires forward compatibility
	// with unknown top-level keys.
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("archive: parse manifest: %w", err)
	}
	if !isSupportedVersion(m.SchemaVersion) {
		return nil, &ErrUnsupportedSchemaVersion{Got: m.SchemaVersion, Supported: SupportedVersions}
	}
	r.manifest = &m
	return &m, nil
}

// Manifest returns the already-parsed manifest, or nil if ReadManifest has
// not yet been called.
func (r *Reader) Manifest() *Manifest {
	return r.manifest
}

// Visitor hook interfaces. Implement only the ones you care about; Walk
// uses type assertions to dispatch. Any hook that returns a non-nil error
// aborts the walk and the error is returned from Walk. Blob and Preview
// hooks receive an io.Reader bound to the current tar entry — do NOT hold
// on to it past the hook's return, since the underlying tar reader advances
// immediately after.
type GroupVisitor interface {
	OnGroup(p *GroupPayload) error
}
type NoteVisitor interface {
	OnNote(p *NotePayload) error
}
type ResourceVisitor interface {
	OnResource(p *ResourcePayload) error
}
type SeriesVisitor interface {
	OnSeries(p *SeriesPayload) error
}
type BlobVisitor interface {
	OnBlob(hash string, body io.Reader, size int64) error
}
type PreviewVisitor interface {
	OnPreview(previewExportID string, body io.Reader, size int64) error
}
type CategoryDefsVisitor interface {
	OnCategoryDefs(defs []CategoryDef) error
}
type NoteTypeDefsVisitor interface {
	OnNoteTypeDefs(defs []NoteTypeDef) error
}
type ResourceCategoryDefsVisitor interface {
	OnResourceCategoryDefs(defs []ResourceCategoryDef) error
}
type TagDefsVisitor interface {
	OnTagDefs(defs []TagDef) error
}
type GroupRelationTypeDefsVisitor interface {
	OnGroupRelationTypeDefs(defs []GroupRelationTypeDef) error
}

// Walk consumes all remaining tar entries (everything after the manifest)
// in tar order and dispatches to v via the typed hook interfaces above.
// Walk may only be called once per Reader. It is the single streaming
// iteration path — there is no seeking, no buffering, and no random access.
func (r *Reader) Walk(v any) error {
	if r.walked {
		return fmt.Errorf("archive: Reader already walked; construct a new Reader to walk again")
	}
	if r.manifest == nil {
		return fmt.Errorf("archive: ReadManifest must be called before Walk")
	}
	r.walked = true

	for {
		hdr, err := r.tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("archive: walk entry: %w", err)
		}
		if err := r.dispatch(hdr, v); err != nil {
			return err
		}
	}
}

func (r *Reader) dispatch(hdr *tar.Header, v any) error {
	name := hdr.Name
	switch {
	case strings.HasPrefix(name, "groups/") && strings.HasSuffix(name, ".json"):
		if gv, ok := v.(GroupVisitor); ok {
			var p GroupPayload
			if err := json.NewDecoder(r.tr).Decode(&p); err != nil {
				return fmt.Errorf("archive: parse %s: %w", name, err)
			}
			return gv.OnGroup(&p)
		}
	case strings.HasPrefix(name, "notes/") && strings.HasSuffix(name, ".json"):
		if nv, ok := v.(NoteVisitor); ok {
			var p NotePayload
			if err := json.NewDecoder(r.tr).Decode(&p); err != nil {
				return fmt.Errorf("archive: parse %s: %w", name, err)
			}
			return nv.OnNote(&p)
		}
	case strings.HasPrefix(name, "resources/") && strings.HasSuffix(name, ".json"):
		if rv, ok := v.(ResourceVisitor); ok {
			var p ResourcePayload
			if err := json.NewDecoder(r.tr).Decode(&p); err != nil {
				return fmt.Errorf("archive: parse %s: %w", name, err)
			}
			return rv.OnResource(&p)
		}
	case strings.HasPrefix(name, "series/") && strings.HasSuffix(name, ".json"):
		if sv, ok := v.(SeriesVisitor); ok {
			var p SeriesPayload
			if err := json.NewDecoder(r.tr).Decode(&p); err != nil {
				return fmt.Errorf("archive: parse %s: %w", name, err)
			}
			return sv.OnSeries(&p)
		}
	case strings.HasPrefix(name, "blobs/"):
		if bv, ok := v.(BlobVisitor); ok {
			hash := name[len("blobs/"):]
			return bv.OnBlob(hash, r.tr, hdr.Size)
		}
	case strings.HasPrefix(name, "previews/"):
		if pv, ok := v.(PreviewVisitor); ok {
			id := name[len("previews/"):]
			return pv.OnPreview(id, r.tr, hdr.Size)
		}
	case name == "schemas/categories.json":
		if cv, ok := v.(CategoryDefsVisitor); ok {
			var defs []CategoryDef
			if err := json.NewDecoder(r.tr).Decode(&defs); err != nil {
				return err
			}
			return cv.OnCategoryDefs(defs)
		}
	case name == "schemas/note_types.json":
		if nv, ok := v.(NoteTypeDefsVisitor); ok {
			var defs []NoteTypeDef
			if err := json.NewDecoder(r.tr).Decode(&defs); err != nil {
				return err
			}
			return nv.OnNoteTypeDefs(defs)
		}
	case name == "schemas/resource_categories.json":
		if rcv, ok := v.(ResourceCategoryDefsVisitor); ok {
			var defs []ResourceCategoryDef
			if err := json.NewDecoder(r.tr).Decode(&defs); err != nil {
				return err
			}
			return rcv.OnResourceCategoryDefs(defs)
		}
	case name == "schemas/tags.json":
		if tv, ok := v.(TagDefsVisitor); ok {
			var defs []TagDef
			if err := json.NewDecoder(r.tr).Decode(&defs); err != nil {
				return err
			}
			return tv.OnTagDefs(defs)
		}
	case name == "schemas/group_relation_types.json":
		if gtv, ok := v.(GroupRelationTypeDefsVisitor); ok {
			var defs []GroupRelationTypeDef
			if err := json.NewDecoder(r.tr).Decode(&defs); err != nil {
				return err
			}
			return gtv.OnGroupRelationTypeDefs(defs)
		}
	}
	return nil
}

// Close releases the gzip reader if any. Idempotent.
func (r *Reader) Close() error {
	if r.gz != nil {
		err := r.gz.Close()
		r.gz = nil
		return err
	}
	return nil
}

func isSupportedVersion(v int) bool {
	for _, s := range SupportedVersions {
		if s == v {
			return true
		}
	}
	return false
}

// peekedReader wraps an io.Reader with a 2-byte peek so we can detect gzip
// magic without consuming the bytes from the source.
type peekedReader struct {
	r        io.Reader
	peek     []byte
	consumed bool
}

func (p *peekedReader) Peek(n int) ([]byte, error) {
	if len(p.peek) >= n {
		return p.peek[:n], nil
	}
	need := n - len(p.peek)
	buf := make([]byte, need)
	read, err := io.ReadFull(p.r, buf)
	p.peek = append(p.peek, buf[:read]...)
	if err != nil {
		return p.peek, err
	}
	return p.peek, nil
}

func (p *peekedReader) Read(b []byte) (int, error) {
	if !p.consumed && len(p.peek) > 0 {
		n := copy(b, p.peek)
		p.peek = p.peek[n:]
		if len(p.peek) == 0 {
			p.consumed = true
		}
		return n, nil
	}
	return p.r.Read(b)
}
