package api_handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/plugin_commands"
)

type pluginCommandHistoryStub struct {
	runs       []plugin_commands.RunRecord
	count      int64
	detail     plugin_commands.RunView
	available  bool
	cancelID   string
	cancelErr  error
	listOffset int
	listLimit  int
}

func (s *pluginCommandHistoryStub) GetPluginCommandRuns(offset, limit int) ([]plugin_commands.RunRecord, int64, error) {
	s.listOffset, s.listLimit = offset, limit
	return s.runs, s.count, nil
}
func (s *pluginCommandHistoryStub) GetPluginCommandRun(id string) (plugin_commands.RunView, bool, error) {
	if id != s.detail.ID {
		return plugin_commands.RunView{}, false, plugin_commands.ErrRunNotFound
	}
	return s.detail, s.available, nil
}
func (s *pluginCommandHistoryStub) CancelPluginCommandRun(id string) error {
	s.cancelID = id
	return s.cancelErr
}

func TestPluginCommandHistoryListIsBoundedAndOmitsOutput(t *testing.T) {
	now := time.Now().UTC()
	stub := &pluginCommandHistoryStub{runs: []plugin_commands.RunRecord{{ID: "run-1", PluginName: "media", Status: plugin_commands.RunStatusQueued, CreatedAt: now}}, count: 51}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/plugin/command-runs?page=2", nil)
	GetPluginCommandRunsHandler(stub)(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if stub.listOffset != pluginCommandHistoryPageSize || stub.listLimit != pluginCommandHistoryPageSize {
		t.Fatalf("list bounds = offset %d limit %d", stub.listOffset, stub.listLimit)
	}
	var payload struct {
		Runs  []map[string]any `json:"runs"`
		Count int64            `json:"count"`
		Page  int              `json:"page"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 51 || payload.Page != 2 || len(payload.Runs) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	if _, ok := payload.Runs[0]["output"]; ok {
		t.Fatal("list leaked output")
	}
}

func TestPluginCommandHistoryClampsPageBeforeOffsetMultiplication(t *testing.T) {
	stub := &pluginCommandHistoryStub{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/plugin/command-runs?page=9223372036854775807", nil)
	GetPluginCommandRunsHandler(stub)(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	const maxPage = int64(1_000_000_000)
	wantOffset := int((maxPage - 1) * int64(pluginCommandHistoryPageSize))
	if stub.listOffset != wantOffset {
		t.Fatalf("list offset = %d, want clamped %d", stub.listOffset, wantOffset)
	}
	var payload struct {
		Page int64 `json:"page"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Page != maxPage {
		t.Fatalf("page = %d, want %d", payload.Page, maxPage)
	}
}

func TestPluginCommandHistoryDetailReportsPrunedOutput(t *testing.T) {
	stub := &pluginCommandHistoryStub{detail: plugin_commands.RunView{RunRecord: plugin_commands.RunRecord{ID: "run-1", Status: plugin_commands.RunStatusSucceeded}}, available: false}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/plugin/command-run?id=run-1", nil)
	GetPluginCommandRunHandler(stub)(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		OutputAvailable bool `json:"outputAvailable"`
		Run             struct {
			ID string `json:"ID"`
		} `json:"run"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.OutputAvailable || payload.Run.ID != "run-1" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestPluginCommandCancelMapsTypedErrorsAndRedirectsHTML(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "missing", err: plugin_commands.ErrRunNotFound, want: http.StatusNotFound},
		{name: "terminal", err: plugin_commands.ErrRunNotCancellable, want: http.StatusConflict},
		{name: "live state conflict", err: &download_queue.StateConflictError{JobID: "run-1", Action: "cancelled", Status: download_queue.JobStatusCompleted}, want: http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &pluginCommandHistoryStub{cancelErr: tc.err}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/plugin/command-run/cancel", strings.NewReader(url.Values{"id": {"run-1"}}.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Accept", "application/json")
			GetPluginCommandRunCancelHandler(stub)(recorder, request)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tc.want, recorder.Body.String())
			}
		})
	}

	stub := &pluginCommandHistoryStub{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/plugin/command-run/cancel", strings.NewReader("id=run-1"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "text/html")
	GetPluginCommandRunCancelHandler(stub)(recorder, request)
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/admin/plugin-command-runs?notice=cancelled" {
		t.Fatalf("redirect = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
	if stub.cancelID != "run-1" {
		t.Fatalf("cancel id = %q", stub.cancelID)
	}
}

func TestPluginCommandCancelRejectsMissingID(t *testing.T) {
	stub := &pluginCommandHistoryStub{}
	recorder := httptest.NewRecorder()
	GetPluginCommandRunCancelHandler(stub)(recorder, httptest.NewRequest(http.MethodPost, "/v1/plugin/command-run/cancel", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !errors.Is(stub.cancelErr, nil) || stub.cancelID != "" {
		t.Fatal("cancel called for missing id")
	}
}
