package api_handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"mahresources/jobs"
)

const maxJobCommandRequestBytes = 64 << 10

type JobCommandContext interface {
	ExecuteJobCommand(requestCtx context.Context, request jobs.CommandRequest) (jobs.CommandResult, error)
	ExecuteBulkJobCommand(requestCtx context.Context, request jobs.BulkCommandRequest) []jobs.CommandResult
	GetJob(jobID string) (jobs.Snapshot, error)
}

type jobCommandRequestBody struct {
	ExpectedVersion uint64 `json:"expectedVersion" openapi:"required"`
	IdempotencyKey  string `json:"idempotencyKey,omitempty"`
	Origin          string `json:"origin,omitempty"`
}

type bulkJobCommandRequestBody struct {
	JobIDs         []string `json:"jobIds" openapi:"required"`
	IdempotencyKey string   `json:"idempotencyKey,omitempty"`
	Origin         string   `json:"origin,omitempty"`
}

type JobCommandResultResponse struct {
	JobID       string               `json:"jobId"`
	Key         string               `json:"key"`
	Status      string               `json:"status"`
	Code        string               `json:"code"`
	Message     string               `json:"message,omitempty"`
	Detail      json.RawMessage      `json:"detail,omitempty"`
	SuccessorID string               `json:"successorId,omitempty"`
	Job         *JobSnapshotResponse `json:"job,omitempty"`
}

type JobCommandConflictResponse struct {
	Error  string                   `json:"error"`
	Job    JobSnapshotResponse      `json:"job"`
	Result JobCommandResultResponse `json:"result"`
}

type bulkJobCommandResponse struct {
	Results []JobCommandResultResponse `json:"results"`
}

// GetJobCommandHandler handles POST /v1/jobs/{id}/commands/{command}.
func GetJobCommandHandler(ctx JobCommandContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		jobID, command := vars["id"], vars["command"]
		if jobID == "" || command == "" {
			writeJobError(w, http.StatusBadRequest, "job id and command are required")
			return
		}
		var body jobCommandRequestBody
		if err := decodeJobJSONBody(r, &body); err != nil {
			writeJobError(w, http.StatusBadRequest, "invalid command request body")
			return
		}
		idempotencyKey, err := requestIdempotencyKey(r, body.IdempotencyKey)
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateJobCommandInput(command, idempotencyKey, body.Origin); err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.ExpectedVersion == 0 {
			writeJobError(w, http.StatusBadRequest, "expectedVersion must be a positive integer")
			return
		}

		result, commandErr := ctx.ExecuteJobCommand(r.Context(), jobs.CommandRequest{
			JobID: jobID, Key: command, IdempotencyKey: idempotencyKey,
			ExpectedVersion: body.ExpectedVersion, Origin: body.Origin,
		})
		if commandErr != nil {
			writeJobCommandError(w, ctx, jobID, result, commandErr)
			return
		}
		writeJobJSON(w, http.StatusOK, jobCommandResultResponse(result))
	}
}

// GetBulkJobCommandHandler handles POST /v1/jobs/commands/{command}.
func GetBulkJobCommandHandler(ctx JobCommandContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		command := mux.Vars(r)["command"]
		if command == "" {
			writeJobError(w, http.StatusBadRequest, "command is required")
			return
		}
		var body bulkJobCommandRequestBody
		if err := decodeJobJSONBody(r, &body); err != nil {
			writeJobError(w, http.StatusBadRequest, "invalid bulk command request body")
			return
		}
		if len(body.JobIDs) == 0 {
			writeJobError(w, http.StatusBadRequest, "jobIds must not be empty")
			return
		}
		if len(body.JobIDs) > jobs.MaxBulkCommandJobs {
			writeJobError(w, http.StatusBadRequest, fmt.Sprintf("jobIds may include at most %d jobs", jobs.MaxBulkCommandJobs))
			return
		}
		idempotencyKey, err := requestIdempotencyKey(r, body.IdempotencyKey)
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateJobCommandInput(command, idempotencyKey, body.Origin); err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		results := ctx.ExecuteBulkJobCommand(r.Context(), jobs.BulkCommandRequest{
			JobIDs: body.JobIDs, Key: command, IdempotencyKey: idempotencyKey, Origin: body.Origin,
		})
		response := bulkJobCommandResponse{Results: make([]JobCommandResultResponse, 0, len(results))}
		for _, result := range results {
			response.Results = append(response.Results, jobCommandResultResponse(result))
		}
		writeJobJSON(w, http.StatusOK, response)
	}
}

func decodeJobJSONBody(r *http.Request, target any) error {
	defer r.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxJobCommandRequestBytes+1))
	if err != nil {
		return err
	}
	if len(bodyBytes) > maxJobCommandRequestBytes {
		return fmt.Errorf("request body is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("request body must contain one JSON value")
	}
	return nil
}

func requestIdempotencyKey(r *http.Request, bodyKey string) (string, error) {
	headerKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	bodyKey = strings.TrimSpace(bodyKey)
	if headerKey != "" && bodyKey != "" && headerKey != bodyKey {
		return "", fmt.Errorf("Idempotency-Key header and idempotencyKey body field must match")
	}
	if headerKey != "" {
		return headerKey, nil
	}
	if bodyKey != "" {
		return bodyKey, nil
	}
	return "", fmt.Errorf("Idempotency-Key is required")
}

func validateJobCommandInput(command, idempotencyKey, origin string) error {
	if strings.TrimSpace(command) == "" || len(command) > jobs.MaxCommandKeyBytes {
		return fmt.Errorf("command must be between 1 and %d bytes", jobs.MaxCommandKeyBytes)
	}
	if len(idempotencyKey) > jobs.MaxIdempotencyKeyBytes {
		return fmt.Errorf("Idempotency-Key may not exceed %d bytes", jobs.MaxIdempotencyKeyBytes)
	}
	if len(origin) > jobs.MaxOriginBytes {
		return fmt.Errorf("origin may not exceed %d bytes", jobs.MaxOriginBytes)
	}
	return nil
}

func jobCommandResultResponse(result jobs.CommandResult) JobCommandResultResponse {
	response := JobCommandResultResponse{
		JobID: result.JobID, Key: result.Key, Status: result.Status, Code: result.Code,
		Message: result.Message, Detail: append(json.RawMessage(nil), result.Detail...),
		SuccessorID: result.SuccessorID,
	}
	if result.Job.ID != "" {
		job := jobSnapshotResponse(result.Job)
		response.Job = &job
	}
	return response
}

func writeJobCommandError(w http.ResponseWriter, ctx JobCommandContext, jobID string, result jobs.CommandResult, err error) {
	if errors.Is(err, jobs.ErrVersionConflict) {
		snapshot := result.Job
		if snapshot.ID == "" {
			var readErr error
			snapshot, readErr = ctx.GetJob(jobID)
			if readErr != nil {
				writeJobServiceError(w, readErr)
				return
			}
		}
		result.Job = snapshot
		writeJobJSON(w, http.StatusConflict, JobCommandConflictResponse{
			Error:  "job changed since this command was prepared",
			Job:    jobSnapshotResponse(snapshot),
			Result: jobCommandResultResponse(result),
		})
		return
	}

	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, jobs.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, jobs.ErrInvalidCommand):
		status = http.StatusBadRequest
	case errors.Is(err, jobs.ErrCommandNotAdvertised), errors.Is(err, jobs.ErrCommandKeyReused),
		errors.Is(err, jobs.ErrCommandInFlight), errors.Is(err, jobs.ErrCommandFailed),
		errors.Is(err, jobs.ErrCommandChainConflict):
		status = http.StatusConflict
	}
	message := "job command failed"
	if status == http.StatusBadRequest {
		message = err.Error()
	}
	body := map[string]any{"error": message}
	if result.JobID != "" || result.Code != "" {
		body["result"] = jobCommandResultResponse(result)
	}
	writeJobJSON(w, status, body)
}
