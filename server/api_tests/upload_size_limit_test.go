package api_tests

import (
	"crypto/rand"
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"
)

// BH-034: resource + version upload paths now bound request body size via
// http.MaxBytesReader. Over-limit uploads reject with HTTP 400 (or 413 if the
// underlying net/http pipeline surfaces it); under-limit uploads succeed.

// TestResourceUpload_RejectsOversize: posting a body past MaxUploadSize must
// not reach disk. The exact status is either 400 (ParseMultipartForm surface)
// or 413 (if the handler is wired to map MaxBytesError explicitly). Both are
// acceptable per the design (plan Task B — "HTTP 413 optional").
func TestResourceUpload_RejectsOversize(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.Config.MaxUploadSize = 1 << 20 // 1 MiB

	buf := make([]byte, 2<<20) // 2 MiB — payload alone exceeds the limit
	_, _ = rand.Read(buf)

	body, ct := makeMultipartUpload(t, "resource", "big.bin", buf,
		map[string]string{"Name": "BH-034 oversize resource"})
	resp := tc.makeMultipartRequest(t, http.MethodPost, "/v1/resource", body, ct)
	if resp.Code != http.StatusBadRequest && resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 400 or 413 for over-limit upload, got %d; body=%s", resp.Code, resp.Body.String())
	}
}

// TestResourceUpload_AcceptsUnderLimit: baseline to prove the guard doesn't
// break normal uploads.
func TestResourceUpload_AcceptsUnderLimit(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.Config.MaxUploadSize = 4 << 20 // 4 MiB

	buf := make([]byte, 128<<10) // 128 KiB well under the limit
	_, _ = rand.Read(buf)

	body, ct := makeMultipartUpload(t, "resource", "small.bin", buf,
		map[string]string{"Name": "BH-034 under-limit"})
	resp := tc.makeMultipartRequest(t, http.MethodPost, "/v1/resource", body, ct)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for under-limit upload, got %d; body=%s", resp.Code, resp.Body.String())
	}
}

// TestResourceVersionUpload_RejectsOversize: the /v1/resource/version/upload
// path must enforce the same guard — a 2 MiB version upload with a 1 MiB
// MaxUploadSize must reject before the file lands on disk.
func TestResourceVersionUpload_RejectsOversize(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.Config.MaxUploadSize = 1 << 20 // 1 MiB

	// Seed a resource so there's something to add a version to.
	res := &models.Resource{Name: "BH-034 version host", Hash: "bh034-host"}
	if err := tc.DB.Create(res).Error; err != nil {
		t.Fatalf("seed resource: %v", err)
	}

	buf := make([]byte, 2<<20) // 2 MiB payload
	_, _ = rand.Read(buf)

	body, ct := makeMultipartUpload(t, "file", "v2.bin", buf,
		map[string]string{"comment": "BH-034 oversize version"})
	url := fmt.Sprintf("/v1/resource/versions?resourceId=%d", res.ID)
	resp := tc.makeMultipartRequest(t, http.MethodPost, url, body, ct)
	if resp.Code != http.StatusBadRequest && resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 400 or 413 for oversize version upload, got %d; body=%s", resp.Code, resp.Body.String())
	}
}
