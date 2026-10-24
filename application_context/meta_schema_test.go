package application_context

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Plan 2026-10-24 section 1e. The compiler's default loader opened file://
// references, so a MetaSchema naming a local file made the server read it on
// every save, and a FIFO blocked the request with no deadline.

// requireRefusedByLoader asserts that compiling failed because the refusing
// loader was asked for wantURL. The library wraps a loader's error, and only a
// loader's error, in a LoadURLError carrying the URL it asked for, so a match on
// both the URL and our sentinel records the loader call that refused it.
func requireRefusedByLoader(t *testing.T, err error, wantURL string) {
	t.Helper()
	if err == nil {
		t.Fatalf("schema referencing %s compiled; want it refused", wantURL)
	}
	// LoadURLError has no Unwrap, so errors.Is cannot see the loader's error.
	var loadErr *jsonschema.LoadURLError
	if !errors.As(err, &loadErr) {
		t.Fatalf("error is not a LoadURLError: %v", err)
	}
	if loadErr.Err != errExternalSchemaRef {
		t.Fatalf("error does not come from the refusing loader: %v", err)
	}
	if loadErr.URL != wantURL {
		t.Errorf("loader was asked for %q, want %q", loadErr.URL, wantURL)
	}
	if !strings.Contains(err.Error(), "use a local #/$defs reference") {
		t.Errorf("error does not tell the author what to do instead: %v", err)
	}
}

func TestValidateMetaSchemaRefusesFileRef(t *testing.T) {
	path := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(path, []byte(`{"type":"string"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fileURL := "file://" + filepath.ToSlash(path)

	err := ValidateMetaSchema(`{"type":"object","properties":{"x":{"$ref":"` + fileURL + `"}}}`)
	requireRefusedByLoader(t, err, fileURL)
}

func TestValidateMetaSchemaRefusesRemoteRefsWithoutNetwork(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"type":"string"}`))
	}))
	defer srv.Close()
	httpsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"type":"string"}`))
	}))
	defer httpsSrv.Close()

	for _, ref := range []string{srv.URL + "/s.json", httpsSrv.URL + "/s.json"} {
		t.Run(ref[:strings.Index(ref, ":")], func(t *testing.T) {
			err := ValidateMetaSchema(`{"properties":{"x":{"$ref":"` + ref + `"}}}`)
			requireRefusedByLoader(t, err, ref)
		})
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the servers received %d requests; want none", n)
	}
}

func TestValidateMetaSchemaRefusesUnknownRemoteMetaschema(t *testing.T) {
	const meta = "https://example.invalid/my-metaschema"
	err := ValidateMetaSchema(`{"$schema":"` + meta + `","type":"object"}`)
	requireRefusedByLoader(t, err, meta)
}

func TestValidateMetaSchemaStillCompilesLocalSchemas(t *testing.T) {
	cases := map[string]string{
		"draft 2020-12": `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`,
		"draft-07":      `{"$schema":"http://json-schema.org/draft-07/schema#","type":"object"}`,
		"local $defs ref": `{"type":"object","properties":{"x":{"$ref":"#/$defs/name"}},` +
			`"$defs":{"name":{"type":"string"}}}`,
		"no $schema": `{"type":"object","properties":{"camera":{"type":"string"}}}`,
	}
	for name, schema := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateMetaSchema(schema); err != nil {
				t.Errorf("ValidateMetaSchema(%s) = %v, want nil", schema, err)
			}
		})
	}
}
