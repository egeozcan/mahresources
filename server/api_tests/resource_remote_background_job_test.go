package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// TestBackgroundRemoteResourceIsACanonicalJob pins the create-resource form's
// "Download in background" path to the Job control plane. That form posts to
// /v1/resource/remote?background=true, not to /v1/download/submit, and it used to
// hand the URL straight to the queue: the transfer ran and created its Resource,
// but no durable Job was ever accepted, so the Jobs panel and /jobs never showed
// the download at all.
func TestBackgroundRemoteResourceIsACanonicalJob(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = fmt.Fprintf(w, "background-form-bytes-%d", time.Now().UnixNano())
	}))
	t.Cleanup(srv.Close)

	res := tc.MakeFormRequest(http.MethodPost, "/v1/resource/remote?background=true",
		url.Values{"URL": {srv.URL + "/form-background.bin"}})
	if res.Code != http.StatusAccepted {
		t.Fatalf("background submit answered %d: %s", res.Code, res.Body.String())
	}
	var submitted struct {
		Queued bool `json:"queued"`
		Jobs   []struct {
			ID             string `json:"id"`
			CanonicalJobID string `json:"canonicalJobId"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &submitted); err != nil || len(submitted.Jobs) != 1 || !submitted.Queued {
		t.Fatalf("unexpected submit response %s (%v)", res.Body.String(), err)
	}
	canonicalID := submitted.Jobs[0].CanonicalJobID
	if canonicalID == "" {
		t.Fatalf("the background download was not accepted as a durable Job: %s", res.Body.String())
	}

	deadline := time.Now().Add(20 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		detail := tc.MakeRequest(http.MethodGet, "/v1/jobs/"+canonicalID, nil)
		if detail.Code == http.StatusOK {
			var job struct {
				State   string `json:"state"`
				Outputs []struct {
					Key          string `json:"key"`
					Type         string `json:"type"`
					Availability string `json:"availability"`
				} `json:"outputs"`
			}
			if err := json.Unmarshal(detail.Body.Bytes(), &job); err == nil {
				last = detail.Body.String()
				if job.State == "succeeded" {
					for _, output := range job.Outputs {
						if output.Key == "resource" && output.Type == "entity" && output.Availability == "available" {
							return
						}
					}
					t.Fatalf("the succeeded Job names no created Resource: %s", last)
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("the background download's Job never succeeded; last detail %s", last)
}
