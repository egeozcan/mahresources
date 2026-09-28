package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/cmd/mr/client"
)

func TestCLIImportResultRequiresItsAcceptedApplyAndKeepsFlatJSON(t *testing.T) {
	t.Setenv("MR_TOKEN", "")
	var responseBody string
	var expectedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedHeader = r.Header.Get("X-Expected-Import-Apply")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseBody))
	}))
	defer server.Close()
	api := client.New(server.URL)

	responseBody = `{"created_groups":1,"apply_outcome":"unknown"}`
	if result, err := fetchImportApplyResult(api, "/v1/imports/imp-1/result", "apply-accepted"); err == nil || result != nil {
		t.Fatalf("mismatched newer report returned as CLI result: result=%+v err=%v", result, err)
	}
	if expectedHeader != "apply-accepted" {
		t.Fatalf("result request expected producer %q, want accepted ID", expectedHeader)
	}

	responseBody = `{"created_groups":1,"apply_outcome":"succeeded","apply_failure":"private failure"}`
	result, err := fetchImportApplyResult(api, "/v1/imports/imp-1/result", "apply-accepted")
	if err != nil || result.CreatedGroups != 1 || expectedHeader != "apply-accepted" {
		t.Fatalf("matching result=%+v expected producer=%q error=%v", result, expectedHeader, err)
	}
	flat, err := json.Marshal(result.ImportApplyResult)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(flat), "apply_outcome") || strings.Contains(string(flat), "apply_failure") || strings.Contains(string(flat), "apply-accepted") {
		t.Fatalf("CLI JSON output included outcome or producer metadata: %s", flat)
	}
}
