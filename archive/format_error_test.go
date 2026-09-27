package archive

import (
	"bytes"
	"errors"
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

	for name, at := range map[string]int{"before the first entry": 0, "in the manifest": 600, "mid-walk": len(whole) - 600} {
		err := readWhole(&failingReader{data: whole, n: at, err: io.ErrClosedPipe})
		var format *FormatError
		if errors.As(err, &format) || !errors.Is(err, io.ErrClosedPipe) {
			t.Errorf("a read failing %s answered %v, want the read's own cause and no FormatError", name, err)
		}
	}
}
