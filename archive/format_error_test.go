package archive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
)

// failingReader hands over the first n bytes of an archive and then fails the
// way a storage read fails, not the way a malformed archive reads.
type failingReader struct {
	data []byte
	n    int
	err  error
}

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, f.err
	}
	if len(p) > f.n {
		p = p[:f.n]
	}
	k := copy(p, f.data)
	f.data, f.n = f.data[k:], f.n-k
	return k, nil
}

// failingOnceReader fails its first read past n bytes and then reports the end
// of the stream, as a storage read that errors and closes does.
type failingOnceReader struct {
	failingReader
	failed bool
}

func (f *failingOnceReader) Read(p []byte) (int, error) {
	if f.n <= 0 {
		if f.failed {
			return 0, io.EOF
		}
		f.failed = true
		return 0, f.err
	}
	return f.failingReader.Read(p)
}

func readWhole(src io.Reader) error {
	r, err := NewReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	if _, err := r.ReadManifest(); err != nil {
		return err
	}
	return r.Walk(struct{}{})
}

// An archive refused on its content is a *FormatError; a read that failed under
// the reader is not one, and keeps its cause.
func TestAFormatErrorIsTheArchivesOwnAndAReadFailureIsNot(t *testing.T) {
	whole := writeFixtureArchive(t).Bytes()

	for name, src := range map[string]io.Reader{
		"random bytes":  bytes.NewReader([]byte("this is not a tar archive at all, just nonsense text")),
		"truncated":     bytes.NewReader(whole[:len(whole)/2]),
		"empty":         bytes.NewReader(nil),
		"cut mid-entry": bytes.NewReader(whole[:700]),
	} {
		var format *FormatError
		if err := readWhole(src); !errors.As(err, &format) {
			t.Errorf("%s: %v is not a FormatError", name, err)
		}
	}

	for name, at := range map[string]int{"before the first entry": 0, "inside the first bytes": 1, "in the manifest": 600, "mid-walk": len(whole) - 600} {
		for kind, src := range map[string]io.Reader{
			"and keeps failing":  &failingReader{data: whole, n: at, err: io.ErrClosedPipe},
			"once and then ends": &failingOnceReader{failingReader: failingReader{data: whole, n: at, err: io.ErrClosedPipe}},
		} {
			err := readWhole(src)
			var format *FormatError
			if errors.As(err, &format) || !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("a read failing %s %s answered %v, want the read's own cause and no FormatError", name, kind, err)
			}
		}
	}
}

type failingGroupVisitor struct{ err error }

func (v failingGroupVisitor) OnGroup(*GroupPayload) error { return v.err }

type blobReadingVisitor struct{}

func (blobReadingVisitor) OnBlob(hash string, body io.Reader, _ int64) error {
	if _, err := io.ReadAll(body); err != nil {
		return fmt.Errorf("store blob %s: %w", hash, err)
	}
	return nil
}

// Only the archive's bytes decide a FormatError. A visitor's own failure is
// passed on as it is, whatever it looks like, while a visitor that fails reading
// an entry the archive cut short still reports the archive's verdict.
func TestAVisitorsOwnFailureIsNotAnArchiveVerdict(t *testing.T) {
	whole := writeFixtureArchive(t).Bytes()
	walk := func(src []byte, v any) error {
		r, err := NewReader(bytes.NewReader(src))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		if _, err := r.ReadManifest(); err != nil {
			t.Fatal(err)
		}
		return r.Walk(v)
	}

	for _, own := range []error{io.EOF, io.ErrUnexpectedEOF, &json.SyntaxError{}} {
		err := walk(whole, failingGroupVisitor{err: own})
		var format *FormatError
		if errors.As(err, &format) || !errors.Is(err, own) {
			t.Errorf("a visitor failing with %T answered %v, want its own error and no FormatError", own, err)
		}
	}

	cut := bytes.Index(whole, []byte("PNGDATA"))
	if cut < 0 {
		t.Fatal("the fixture has no blob to cut")
	}
	var format *FormatError
	if err := walk(whole[:cut+3], blobReadingVisitor{}); !errors.As(err, &format) {
		t.Errorf("a blob the archive cut short answered %v, want a FormatError", err)
	}
}
