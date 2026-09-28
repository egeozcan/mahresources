package api_tests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"mahresources/jobs"
	"mahresources/models"
)

// TestImportResultOutcomeBelongsToItsActualProducer exercises a failed Apply
// that restores its plan, followed by an admin's successful fresh Apply. The
// parse owner can read the new report but cannot read its producer Job, so its
// result must remain unknown instead of inheriting the older visible failure.
func TestImportResultOutcomeBelongsToItsActualProducer(t *testing.T) {
	tc := setupAuthEnv(t)
	installJobControlPlane(t, tc)
	if err := tc.DB.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	ownerBearer, _ := plainUserBearer(t, tc, "import-report-owner")
	adminBearer := unscopedAdminBearer(t, tc)
	headers := func(bearer string) map[string]string {
		return map[string]string{"Authorization": bearer, "Accept": "application/json", "Content-Type": "application/json"}
	}
	handle, parseID := submitImportParseForTest(t, tc, map[string]string{"Authorization": ownerBearer})
	if parse := waitForCanonicalState(t, tc, parseID, "parse", func(s jobs.Snapshot) bool { return s.State.Terminal() }); parse.State != jobs.StateSucceeded {
		t.Fatalf("parse ended %s: %+v", parse.State, parse.Failure)
	}
	selected := models.Group{Name: "Deleted parent for report provenance regression"}
	if err := tc.DB.Create(&selected).Error; err != nil {
		t.Fatal(err)
	}
	selectedID := selected.ID
	if err := tc.DB.Delete(&selected).Error; err != nil {
		t.Fatal(err)
	}
	decisions, _ := json.Marshal(map[string]any{"parent_group_id": selectedID})
	first := doReq(tc, http.MethodPost, "/v1/imports/"+handle+"/apply", headers(ownerBearer), nil, strings.NewReader(string(decisions)))
	if first.Code != http.StatusAccepted {
		t.Fatalf("first apply %d: %s", first.Code, first.Body.String())
	}
	var firstIDs struct {
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstIDs); err != nil {
		t.Fatal(err)
	}
	failed := waitForCanonicalState(t, tc, firstIDs.CanonicalJobID, "first apply", func(s jobs.Snapshot) bool { return s.State.Terminal() })
	if failed.State != jobs.StateFailed {
		t.Fatalf("first apply %s: %+v", failed.State, failed.Failure)
	}
	firstSnapshot, err := afero.ReadFile(tc.Fs, "_imports/"+handle+"."+firstIDs.CanonicalJobID+".result.json")
	if err != nil {
		t.Fatalf("first Apply snapshot: %v", err)
	}
	// The failed execution really restored the plan through its executor.
	plan := doReq(tc, http.MethodGet, "/v1/imports/"+handle+"/plan", headers(ownerBearer), nil, nil)
	if plan.Code != http.StatusOK {
		t.Fatalf("restored plan %d: %s", plan.Code, plan.Body.String())
	}
	retryBody, _ := json.Marshal(map[string]any{"expectedVersion": failed.Version, "idempotencyKey": "import-result-provenance-retry"})
	retry := doReq(tc, http.MethodPost, "/v1/jobs/"+firstIDs.CanonicalJobID+"/commands/retry", headers(adminBearer), nil, strings.NewReader(string(retryBody)))
	if retry.Code != http.StatusOK {
		t.Fatalf("retry %d: %s", retry.Code, retry.Body.String())
	}
	var retryResult struct {
		SuccessorID string `json:"successorId"`
	}
	if err := json.Unmarshal(retry.Body.Bytes(), &retryResult); err != nil || retryResult.SuccessorID == "" {
		t.Fatalf("retry successor %q, decode error %v: %s", retryResult.SuccessorID, err, retry.Body.String())
	}
	retried := waitForCanonicalState(t, tc, retryResult.SuccessorID, "retried apply", func(s jobs.Snapshot) bool { return s.State.Terminal() })
	if retried.State != jobs.StateFailed {
		t.Fatalf("retried apply %s: %+v", retried.State, retried.Failure)
	}
	retrySnapshot, err := afero.ReadFile(tc.Fs, "_imports/"+handle+"."+retryResult.SuccessorID+".result.json")
	if err != nil {
		t.Fatalf("Retry snapshot: %v", err)
	}
	retriedResult := doReq(tc, http.MethodGet, "/v1/imports/"+handle+"/result", headers(adminBearer), nil, nil)
	var retriedPayload map[string]json.RawMessage
	if retriedResult.Code != http.StatusOK || json.Unmarshal(retriedResult.Body.Bytes(), &retriedPayload) != nil {
		t.Fatalf("retried report %d: %s", retriedResult.Code, retriedResult.Body.String())
	}
	var retriedOutcome string
	if err := json.Unmarshal(retriedPayload["apply_outcome"], &retriedOutcome); err != nil || retriedOutcome != "failed" {
		t.Fatalf("retry report outcome %q, decode error %v: %s", retriedOutcome, err, retriedResult.Body.String())
	}
	var retryProvenance struct {
		ReportSHA256  string          `json:"report_sha256"`
		ProducerJobID string          `json:"producer_job_id"`
		Report        json.RawMessage `json:"report"`
	}
	provenanceBytes, err := afero.ReadFile(tc.Fs, "_imports/"+handle+".result.provenance.json")
	if err != nil || json.Unmarshal(provenanceBytes, &retryProvenance) != nil || retryProvenance.ProducerJobID != retryResult.SuccessorID {
		t.Fatalf("retry provenance %s, error %v", provenanceBytes, err)
	}
	second := doReq(tc, http.MethodPost, "/v1/imports/"+handle+"/apply", headers(adminBearer), nil, strings.NewReader("{}"))
	if second.Code != http.StatusAccepted {
		t.Fatalf("fresh admin apply %d: %s", second.Code, second.Body.String())
	}
	var secondIDs struct {
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondIDs); err != nil {
		t.Fatal(err)
	}
	succeeded := waitForCanonicalState(t, tc, secondIDs.CanonicalJobID, "fresh admin apply", func(s jobs.Snapshot) bool { return s.State.Terminal() })
	if succeeded.State != jobs.StateSucceeded {
		t.Fatalf("fresh admin apply %s: %+v", succeeded.State, succeeded.Failure)
	}
	if after, err := afero.ReadFile(tc.Fs, "_imports/"+handle+"."+firstIDs.CanonicalJobID+".result.json"); err != nil || !bytes.Equal(after, firstSnapshot) {
		t.Fatalf("first Apply snapshot changed after later Apply: %s (%v)", after, err)
	}
	if after, err := afero.ReadFile(tc.Fs, "_imports/"+handle+"."+retryResult.SuccessorID+".result.json"); err != nil || !bytes.Equal(after, retrySnapshot) {
		t.Fatalf("Retry snapshot changed after later Apply: %s (%v)", after, err)
	}
	currentReportBytes, err := afero.ReadFile(tc.Fs, "_imports/"+handle+".result.json")
	if err != nil {
		t.Fatal(err)
	}
	var currentPublication struct {
		ProducerJobID string          `json:"producer_job_id"`
		Report        json.RawMessage `json:"report"`
	}
	currentPublicationBytes, err := afero.ReadFile(tc.Fs, "_imports/"+handle+".result.provenance.json")
	if err != nil || json.Unmarshal(currentPublicationBytes, &currentPublication) != nil {
		t.Fatalf("read current report publication %s: %v", currentPublicationBytes, err)
	}
	if currentPublication.ProducerJobID != secondIDs.CanonicalJobID || !bytes.Equal(currentPublication.Report, currentReportBytes) {
		t.Fatalf("current report publication mismatch: producer=%q report=%s current=%s", currentPublication.ProducerJobID, currentPublication.Report, currentReportBytes)
	}

	ownerResult := doReq(tc, http.MethodGet, "/v1/imports/"+handle+"/result", headers(ownerBearer), nil, nil)
	if ownerResult.Code != http.StatusOK {
		t.Fatalf("owner result %d: %s", ownerResult.Code, ownerResult.Body.String())
	}
	var ownerPayload map[string]json.RawMessage
	if err := json.Unmarshal(ownerResult.Body.Bytes(), &ownerPayload); err != nil {
		t.Fatal(err)
	}
	var ownerOutcome string
	if err := json.Unmarshal(ownerPayload["apply_outcome"], &ownerOutcome); err != nil || ownerOutcome != "unknown" {
		t.Fatalf("owner outcome %q, decode error %v: %s", ownerOutcome, err, ownerResult.Body.String())
	}
	if strings.Contains(ownerResult.Body.String(), secondIDs.CanonicalJobID) || strings.Contains(ownerResult.Body.String(), "_producer_job_id") || strings.Contains(ownerResult.Body.String(), "the import could not be applied") || strings.Contains(ownerResult.Body.String(), `"apply_outcome":"succeeded"`) {
		t.Fatalf("hidden producer details escaped to parse owner: %s", ownerResult.Body.String())
	}
	var report struct {
		CreatedGroups int    `json:"created_groups"`
		GroupIDs      []uint `json:"created_group_ids"`
	}
	if err := json.Unmarshal(ownerResult.Body.Bytes(), &report); err != nil || report.CreatedGroups != 1 || len(report.GroupIDs) != 1 {
		t.Fatalf("current successful report was not preserved: %s (%v)", ownerResult.Body.String(), err)
	}

	adminResult := doReq(tc, http.MethodGet, "/v1/imports/"+handle+"/result", headers(adminBearer), nil, nil)
	if adminResult.Code != http.StatusOK {
		t.Fatalf("admin result %d: %s", adminResult.Code, adminResult.Body.String())
	}
	var adminPayload map[string]json.RawMessage
	if err := json.Unmarshal(adminResult.Body.Bytes(), &adminPayload); err != nil {
		t.Fatal(err)
	}
	var adminOutcome string
	if err := json.Unmarshal(adminPayload["apply_outcome"], &adminOutcome); err != nil || adminOutcome != "succeeded" {
		t.Fatalf("admin outcome %q, decode error %v: %s", adminOutcome, err, adminResult.Body.String())
	}
	if strings.Contains(adminResult.Body.String(), secondIDs.CanonicalJobID) || strings.Contains(adminResult.Body.String(), "_producer_job_id") {
		t.Fatalf("producer identity escaped in result response: %s", adminResult.Body.String())
	}

	// A result asked for on behalf of a different live Apply cannot inherit this
	// report's successful outcome, even though the reader can see both Jobs.
	mismatchHeaders := headers(adminBearer)
	mismatchHeaders["X-Expected-Import-Apply"] = firstIDs.CanonicalJobID
	mismatch := doReq(tc, http.MethodGet, "/v1/imports/"+handle+"/result", mismatchHeaders, nil, nil)
	var mismatchBody map[string]json.RawMessage
	if mismatch.Code != http.StatusOK || json.Unmarshal(mismatch.Body.Bytes(), &mismatchBody) != nil {
		t.Fatalf("mismatched producer result %d: %s", mismatch.Code, mismatch.Body.String())
	}
	var mismatchOutcome string
	if err := json.Unmarshal(mismatchBody["apply_outcome"], &mismatchOutcome); err != nil || mismatchOutcome != "unknown" {
		t.Fatalf("mismatched producer outcome %q, decode error %v: %s", mismatchOutcome, err, mismatch.Body.String())
	}

	// Missing, malformed, expired, wrong-kind and wrong-parse metadata all keep
	// the flat report readable while withholding an outcome.
	resultPath := "_imports/" + handle + ".result.json"
	provenancePath := "_imports/" + handle + ".result.provenance.json"
	reportBytes, err := afero.ReadFile(tc.Fs, resultPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(reportBytes)
	writeProvenance := func(producer string) {
		t.Helper()
		proof, err := json.Marshal(struct {
			ReportSHA256  string          `json:"report_sha256"`
			ProducerJobID string          `json:"producer_job_id"`
			Report        json.RawMessage `json:"report"`
		}{hex.EncodeToString(digest[:]), producer, reportBytes})
		if err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(tc.Fs, provenancePath, proof, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	assertUnknown := func(label string) {
		t.Helper()
		response := doReq(tc, http.MethodGet, "/v1/imports/"+handle+"/result", headers(adminBearer), nil, nil)
		var body map[string]json.RawMessage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
			t.Fatalf("%s result %d: %s", label, response.Code, response.Body.String())
		}
		var state string
		if err := json.Unmarshal(body["apply_outcome"], &state); err != nil || state != "unknown" {
			t.Fatalf("%s outcome %q, decode error %v: %s", label, state, err, response.Body.String())
		}
	}
	if err := tc.Fs.Remove(provenancePath); err != nil {
		t.Fatal(err)
	}
	assertUnknown("legacy report without provenance")
	if err := afero.WriteFile(tc.Fs, provenancePath, []byte(`{"report_sha256":"broken","producer_job_id":17}`), 0o600); err != nil {
		t.Fatal(err)
	}
	assertUnknown("malformed provenance")
	writeProvenance("01a0e63b-0000-7000-8000-000000000000") // expired or pruned producer
	assertUnknown("missing producer Job")
	writeProvenance(parseID) // visible, but it is a parse Job, not an Apply
	assertUnknown("wrong producer kind")

	otherHandle, otherParseID := submitImportParseForTest(t, tc, map[string]string{"Authorization": adminBearer})
	if parse := waitForCanonicalState(t, tc, otherParseID, "second parse", func(s jobs.Snapshot) bool { return s.State.Terminal() }); parse.State != jobs.StateSucceeded {
		t.Fatalf("second parse ended %s: %+v", parse.State, parse.Failure)
	}
	otherApply := doReq(tc, http.MethodPost, "/v1/imports/"+otherHandle+"/apply", headers(adminBearer), nil, strings.NewReader("{}"))
	if otherApply.Code != http.StatusAccepted {
		t.Fatalf("other apply %d: %s", otherApply.Code, otherApply.Body.String())
	}
	var otherIDs struct {
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(otherApply.Body.Bytes(), &otherIDs); err != nil {
		t.Fatal(err)
	}
	otherFinished := waitForCanonicalState(t, tc, otherIDs.CanonicalJobID, "other apply", func(s jobs.Snapshot) bool { return s.State.Terminal() })
	if otherFinished.State != jobs.StateSucceeded {
		t.Fatalf("other apply %s: %+v", otherFinished.State, otherFinished.Failure)
	}
	writeProvenance(otherIDs.CanonicalJobID)
	assertUnknown("producer from another parse/plan")
}
