package api_handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/plugin_system"
)

type commandEnableContextStub struct {
	name    string
	enabled bool
	opts    application_context.PluginEnableOptions
	err     error
}

func (s *commandEnableContextStub) SetPluginEnabledWithOptions(name string, enabled bool, opts application_context.PluginEnableOptions) error {
	s.name, s.enabled, s.opts = name, enabled, opts
	return s.err
}

func commandEnableRequest(t *testing.T, accept string, values url.Values) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/plugin/enable", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", accept)
	return req
}

func TestPluginEnableCommandConfirmationReturnsStructuredJSONWithoutConfirming(t *testing.T) {
	stub := &commandEnableContextStub{err: &plugin_system.CommandConfirmationError{Commands: []plugin_system.CommandDisplay{
		{Name: "download", DisplayArgv: "yt-dlp -- '{{url}}'", TimeoutSeconds: 7200},
	}}}
	recorder := httptest.NewRecorder()
	GetPluginEnableHandler(stub)(recorder, commandEnableRequest(t, "application/json", url.Values{"name": {"media"}}))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	if stub.opts.ConfirmCommands {
		t.Fatal("ordinary enable request was treated as command confirmation")
	}
	var body struct {
		Error                       string                         `json:"error"`
		RequiresCommandConfirmation bool                           `json:"requiresCommandConfirmation"`
		Commands                    []plugin_system.CommandDisplay `json:"commands"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "command confirmation required" || !body.RequiresCommandConfirmation {
		t.Fatalf("unexpected refusal body: %+v", body)
	}
	if len(body.Commands) != 1 || body.Commands[0].DisplayArgv != "yt-dlp -- '{{url}}'" || body.Commands[0].TimeoutSeconds != 7200 {
		t.Fatalf("command review data changed: %+v", body.Commands)
	}
}

func TestPluginEnableCommandConfirmationRedirectsHTMLToValidatedSecondStep(t *testing.T) {
	stub := &commandEnableContextStub{err: &plugin_system.CommandConfirmationError{}}
	recorder := httptest.NewRecorder()
	GetPluginEnableHandler(stub)(recorder, commandEnableRequest(t, "text/html", url.Values{"name": {"media plugin"}}))

	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Location"); got != "/plugins/manage?confirm_commands=media+plugin" {
		t.Fatalf("Location = %q", got)
	}
}

func TestPluginEnableConfirmedRequestCarriesTheExplicitOption(t *testing.T) {
	stub := &commandEnableContextStub{}
	recorder := httptest.NewRecorder()
	GetPluginEnableHandler(stub)(recorder, commandEnableRequest(t, "application/json", url.Values{
		"name":             {"media"},
		"confirm_commands": {"1"},
	}))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if stub.name != "media" || !stub.enabled || !stub.opts.ConfirmCommands {
		t.Fatalf("enable call = name %q enabled=%v opts=%+v", stub.name, stub.enabled, stub.opts)
	}
}
