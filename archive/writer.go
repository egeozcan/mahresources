package archive

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// Writer streams archive entries into an underlying io.Writer (typically a
// file or an HTTP response). Caller must call Close exactly once.
//
// Not safe for concurrent use.
type Writer struct {
	tw          *tar.Writer
	gz          *gzip.Writer
	counter     *countingWriter
	manifestSet bool

	mu              sync.Mutex
	blobsWritten    map[string]bool
	previewsWritten map[string]bool
}

// countingWriter wraps an io.Writer and tracks how many bytes have been
// passed through. Used by Writer.BytesWritten() to drive the admin export
// page's bytes-written display. When gzip is on, the count is compressed
// bytes on the wire; otherwise it's raw tar bytes. Both are acceptable
// signals for a progress bar — the tar writer's own internal buffering
// flushes through us on each entry, so updates are smooth enough.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// NewWriter wraps the provided io.Writer. If gzipOut is true, output is gzipped.
func NewWriter(w io.Writer, gzipOut bool) (*Writer, error) {
	cw := &countingWriter{w: w}
	out := &Writer{
		counter:         cw,
		blobsWritten:    make(map[string]bool),
		previewsWritten: make(map[string]bool),
	}
	if gzipOut {
		out.gz = gzip.NewWriter(cw)
		out.tw = tar.NewWriter(out.gz)
	} else {
		out.tw = tar.NewWriter(cw)
	}
	return out, nil
}

// BytesWritten returns the number of bytes that have been passed through
// the underlying io.Writer so far. Safe to call at any point during writing.
func (w *Writer) BytesWritten() int64 {
	return w.counter.n
}

// Close flushes the tar writer (and the gzip writer, if any). Must be called
// exactly once. Calling other methods after Close panics.
func (w *Writer) Close() error {
	if err := w.tw.Close(); err != nil {
		return err
	}
	if w.gz != nil {
		return w.gz.Close()
	}
	return nil
}

// WriteManifest must be called exactly once and before any other Write* call.
func (w *Writer) WriteManifest(m *Manifest) error {
	if w.manifestSet {
		return fmt.Errorf("archive: manifest already written")
	}
	w.manifestSet = true
	return w.writeJSONEntry("manifest.json", m, time.Now().UTC())
}

func (w *Writer) WriteCategoryDefs(defs []CategoryDef) error {
	return w.writeJSONEntry("schemas/categories.json", defs, time.Now().UTC())
}

func (w *Writer) WriteNoteTypeDefs(defs []NoteTypeDef) error {
	return w.writeJSONEntry("schemas/note_types.json", defs, time.Now().UTC())
}

func (w *Writer) WriteResourceCategoryDefs(defs []ResourceCategoryDef) error {
	return w.writeJSONEntry("schemas/resource_categories.json", defs, time.Now().UTC())
}

func (w *Writer) WriteTagDefs(defs []TagDef) error {
	return w.writeJSONEntry("schemas/tags.json", defs, time.Now().UTC())
}

func (w *Writer) WriteGroupRelationTypeDefs(defs []GroupRelationTypeDef) error {
	return w.writeJSONEntry("schemas/group_relation_types.json", defs, time.Now().UTC())
}

func (w *Writer) WriteGroup(p *GroupPayload) error {
	return w.writeJSONEntry("groups/"+p.ExportID+".json", p, p.UpdatedAt)
}

func (w *Writer) WriteNote(p *NotePayload) error {
	return w.writeJSONEntry("notes/"+p.ExportID+".json", p, p.UpdatedAt)
}

func (w *Writer) WriteResource(p *ResourcePayload) error {
	return w.writeJSONEntry("resources/"+p.ExportID+".json", p, p.UpdatedAt)
}

func (w *Writer) WriteSeries(p *SeriesPayload) error {
	return w.writeJSONEntry("series/"+p.ExportID+".json", p, time.Now().UTC())
}

// WriteBlob writes raw file bytes content-addressed by hash. Calling
// WriteBlob with the same hash twice is a no-op (the second call is silently
// dropped) — this is how blob de-duplication is enforced at the writer layer.
func (w *Writer) WriteBlob(hash string, r io.Reader, size int64) error {
	w.mu.Lock()
	if w.blobsWritten[hash] {
		w.mu.Unlock()
		// Drain the reader so the caller's source is fully consumed.
		_, _ = io.Copy(io.Discard, r)
		return nil
	}
	w.blobsWritten[hash] = true
	w.mu.Unlock()

	hdr := &tar.Header{
		Name:    "blobs/" + hash,
		Mode:    0644,
		Size:    size,
		ModTime: time.Now().UTC(),
	}
	if err := w.tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := io.Copy(w.tw, r)
	return err
}

// HasBlob reports whether a blob with this hash has already been written.
// Useful for callers that want to skip opening a file when it would dedup.
func (w *Writer) HasBlob(hash string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.blobsWritten[hash]
}

// WritePreview writes preview bytes addressed by preview export ID (not by
// hash — Preview rows aren't content-addressed in the DB).
func (w *Writer) WritePreview(previewExportID string, data []byte) error {
	w.mu.Lock()
	if w.previewsWritten[previewExportID] {
		w.mu.Unlock()
		return nil
	}
	w.previewsWritten[previewExportID] = true
	w.mu.Unlock()

	hdr := &tar.Header{
		Name:    "previews/" + previewExportID,
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: time.Now().UTC(),
	}
	if err := w.tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := w.tw.Write(data)
	return err
}

func (w *Writer) writeJSONEntry(name string, v any, modTime time.Time) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("archive: marshal %s: %w", name, err)
	}
	hdr := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(body)),
		ModTime: modTime,
	}
	if err := w.tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = w.tw.Write(body)
	return err
}
