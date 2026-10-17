package api_tests

import (
	"net/http"
	"testing"

	"github.com/spf13/afero"
)

// A hash-named stored file never changes, but with sign-in on, whether a
// principal may read it can: a browser that kept it without asking would hand
// it to the next account signed in on that browser, past the file guard. So it
// is immutable only without sign-in, and revalidated on every use with it.
func TestStoredFileCacheControlDependsOnSignIn(t *testing.T) {
	const location = "/resources/b7/2e/03/b72e0339b3cec29046dfde0dce529d9a7b590c78.jpg"
	writeStored := func(t *testing.T, tc *TestContext) {
		t.Helper()
		if err := afero.WriteFile(tc.Fs, location, []byte("jpeg bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("sign-in off", func(t *testing.T) {
		tc := SetupTestEnv(t)
		writeStored(t, tc)
		resp := tc.MakeRequest(http.MethodGet, "/files"+location, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d", resp.Code)
		}
		if cc := resp.Header().Get("Cache-Control"); cc != "private, max-age=31536000, immutable" {
			t.Errorf("Cache-Control = %q", cc)
		}
	})

	t.Run("sign-in on", func(t *testing.T) {
		tc := setupAuthEnv(t)
		writeStored(t, tc)
		cookie, _ := loginCookieAndCSRF(t, tc)
		resp := doReq(tc, http.MethodGet, "/files"+location, nil, []*http.Cookie{cookie}, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d", resp.Code)
		}
		if cc := resp.Header().Get("Cache-Control"); cc != "private, no-cache" {
			t.Errorf("Cache-Control = %q, want revalidation on every use", cc)
		}
	})
}
