package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/server/http_utils"
)

// The harness side of the generate handler (plan sections 2 to 5): the run
// environment, the run registry, the finish route and the delivery of a run's
// one result or error, after a last access check.

// templateRunFinalCheckTimeout bounds the handler's access check made
// immediately before a Draft or transcript is written. It runs under the
// request's context and not the run's, which may already have reached its
// deadline: a Draft recovered after the deadline is still delivered.
const templateRunFinalCheckTimeout = 5 * time.Second

// harnessPreparation is what the handler knows after grounding: everything the
// run environment is built from.
type harnessPreparation struct {
	// ctx is the request-scoped context rebased onto the request, which the
	// grounding reads ran on and the executor's Tools start from.
	ctx         *application_context.MahresourcesContext
	r           *http.Request
	entityType  string
	carrier     any
	carrierID   uint
	carrierName string
	sampleRefs  []application_context.TemplateRef
	request     templateGenerateRequest
	principal   *auth.Principal
	access      TemplateRunAccessFactory

	siblings      map[string]string
	metaSchema    string
	sectionConfig string
}

// env builds the run environment and finishes the input the harness needs: the
// partial lookup and the starting points. Finish and Sink are the caller's.
func (p harnessPreparation) env(in *application_context.TemplateGenerationInput) *application_context.TemplateRunEnv {
	in.PartialExists = p.partialExists
	in.StartingPoints = p.startingPoints()

	principal := p.principal
	if principal == nil {
		principal = &auth.Principal{}
	}
	optIns := application_context.TemplateRunOptIns{PreviewMRQL: p.request.PreviewMRQL, PreviewPlugins: p.request.PreviewPlugins}
	values := p.editorValues(*in)

	var access func(ctx context.Context) (*application_context.MahresourcesContext, error)
	if p.access != nil {
		access = p.access(p.r)
	}
	cfg := TemplateRunToolsConfig{
		Start:      p.ctx,
		Access:     access,
		Principal:  principal,
		EntityType: p.entityType,
		OptIns:     optIns,
		FieldValue: func(field string) (string, bool) { return values[field], true },
		Input:      *in,
		Validate:   application_context.ValidateTemplateRunField,
		Known:      in.Known, DisplayTypes: in.DisplayTypes, DisplayTypesTruncated: in.DisplayTypesTruncated,
	}
	if p.carrier != nil {
		cfg.CarrierID = p.carrierID
	}
	if len(p.sampleRefs) > 0 {
		cfg.PreviewEntityID = p.sampleRefs[0].ID
	}
	return &application_context.TemplateRunEnv{
		Executor: NewTemplateToolExecutor(cfg),
		Account: application_context.TemplateRunAccount{
			// Only an unscoped account reaches a run, so the label is always the
			// whole library; the hashed fields carry the actual identity.
			Label:  "everything",
			UserID: principal.UserID, Role: string(principal.Role),
			ScopeGroupID: principal.ScopeGroupID, SuperUser: principal.SuperUser,
		},
		OptIns: optIns,
	}
}

// partialExists is the harness validator's partial lookup, rebased onto the
// context it is given so a blocked lookup ends with the Tool or the run.
func (p harnessPreparation) partialExists(ctx context.Context, name string) (bool, error) {
	partial, err := p.ctx.WithParentContext(ctx).GetTemplatePartialByName(name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil && partial != nil, err
}

// startingPoints names the edited carrier and the loaded samples by the IDs the
// model opens them by.
func (p harnessPreparation) startingPoints() application_context.TemplateStartingPoints {
	sp := application_context.TemplateStartingPoints{Samples: p.sampleRefs}
	if p.carrier != nil {
		sp.Carrier = application_context.TemplateRef{Type: carrierTypeFor(p.entityType), ID: p.carrierID, Name: p.carrierName}
	}
	return sp
}

// editorValues is every field's value in the editor, for an omitted-content
// lint or preview that has no submission yet: the saved slots with the client's
// unsaved values over them.
func (p harnessPreparation) editorValues(in application_context.TemplateGenerationInput) map[string]string {
	values := map[string]string{}
	for field, content := range p.siblings {
		values[field] = content
	}
	if in.Target == application_context.TemplateTargetSlot && in.Slot != "" {
		values[in.Slot] = in.CurrentContent
	} else {
		var sent map[string]string
		if json.Unmarshal([]byte(in.CurrentContent), &sent) == nil {
			for field, content := range sent {
				values[field] = content
			}
		}
	}
	values["MetaSchema"] = p.metaSchema
	values["SectionConfig"] = p.sectionConfig
	return values
}

// stillHasAccess re-checks the request's credential under a fresh request-parent
// context. It is the last check before anything the run read is released.
func (p harnessPreparation) stillHasAccess() bool {
	if p.access == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(p.r.Context(), templateRunFinalCheckTimeout)
	defer cancel()
	_, err := p.access(p.r)(ctx)
	return err == nil
}

type harnessServe struct {
	generator application_context.TemplateGenerator
	input     application_context.TemplateGenerationInput
	prepared  harnessPreparation
	prompt    string
	hash      string
}

func acceptsEventStream(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

// templateRunAccountKey is the key a run is limited and finished by.
func templateRunAccountKey(r *http.Request) string {
	var userID uint
	if p := auth.PrincipalFromContext(r.Context()); p != nil {
		userID = p.UserID
	}
	return application_context.TemplateRunAccountKey(userID, application_context.ClientIP(r))
}

// serveHarnessRun registers the run, runs it, and delivers its one result or
// error: as an event when the client asked for a stream and the run got as far
// as its first Turn, as an ordinary response otherwise.
func serveHarnessRun(w http.ResponseWriter, r *http.Request, s harnessServe) {
	p := s.prepared
	p.r = r
	runCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	handle, err := application_context.StartTemplateRun(templateRunAccountKey(r), cancel)
	if err != nil {
		writeTemplateGenerationError(w, r, err)
		return
	}
	defer handle.Release()

	in := s.input
	env := p.env(&in)
	env.Finish = handle.FinishRequested
	var stream *templateRunStream
	if acceptsEventStream(r) && canStreamTo(w) {
		stream = newTemplateRunStream(runCtx, cancel, w, runEventFor(handle.ID, s.generator), templateRunStreamConfig{})
		defer stream.Close()
		env.Sink = stream
	}
	in.Run = env

	result, genErr := s.generator.GenerateTemplate(runCtx, in, s.prompt, s.hash)

	// The client left, or a write failed and cancelled the run: nobody is
	// listening, so nothing is released.
	if r.Context().Err() != nil || (stream != nil && stream.failed.Load()) {
		return
	}
	streamed := stream != nil && stream.Started()
	if streamed && !stream.Drain() {
		return
	}
	// Immediately before the terminal write, after the drain.
	accessOK := p.stillHasAccess()
	if r.Context().Err() != nil {
		return
	}
	if !accessOK {
		result, genErr = nil, application_context.ErrTemplateRunAccessChanged
	}

	if streamed {
		_ = stream.Deliver(terminalFrame(result, genErr))
		return
	}
	if genErr != nil {
		writeHarnessJSONError(w, r, genErr)
		return
	}
	w.Header().Set("Content-Type", constants.JSON)
	_ = json.NewEncoder(w).Encode(result)
}

func runEventFor(id string, generator application_context.TemplateGenerator) templateRunRunEvent {
	ev := templateRunRunEvent{RunID: id}
	if c, ok := generator.(interface {
		RunCaps() application_context.TemplateRunCaps
	}); ok {
		caps := c.RunCaps()
		ev.MaxTurns, ev.DeadlineSeconds, ev.TokenBudget = caps.MaxTurns, int(caps.Deadline/time.Second), caps.TokenBudget
	}
	return ev
}

// terminalFrame is the run's result event, or its error event. An access change
// carries no transcript; any other failure carries the one it kept.
func terminalFrame(result *application_context.TemplateGenerationResult, genErr error) []byte {
	if genErr == nil {
		return templateRunFrame("result", result)
	}
	status, shown := classifyTemplateGenerationError(genErr)
	payload := map[string]any{"message": shown.Error(), "status": status}
	if transcript, stop := failureTranscript(genErr); status != http.StatusForbidden {
		if stop != "" {
			payload["stopReason"] = stop
		}
		if len(transcript) > 0 {
			payload["transcript"] = transcript
		}
	}
	return templateRunFrame("error", payload)
}

// failureTranscript is the transcript and stop reason a failed run kept, from
// its TemplateRunError and never from progress events.
func failureTranscript(err error) ([]application_context.TemplateTranscriptRecord, string) {
	var runErr *application_context.TemplateRunError
	if !errors.As(err, &runErr) {
		return nil, ""
	}
	return runErr.Transcript, enumOrEmpty(runErr.StopReason, stopReasonSet)
}

// writeHarnessJSONError answers a failed run that has no stream. Status codes
// are the classic handler's; a failure that kept a transcript adds it to the
// body so the caller sees what was sent.
func writeHarnessJSONError(w http.ResponseWriter, r *http.Request, err error) {
	status, shown := classifyTemplateGenerationError(err)
	transcript, stop := failureTranscript(err)
	if status == http.StatusForbidden || len(transcript) == 0 || http_utils.RequestAcceptsHTML(r) {
		writeTemplateGenerationError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", constants.JSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": shown.Error(), "stopReason": stop, "transcript": transcript})
}

// GetFinishTemplateRunHandler handles
// POST /v1/{category|resourceCategory|noteType}/generateTemplate/finish: it asks
// a live run to make its next Turn the final one. Only the account that started
// the run can; an unknown run and another account's are both 404, so run IDs
// cannot be probed. It reads no entities, so it needs no request-scoped
// context; mounted under the carrier's prefix it inherits the generate route's
// role gate.
func GetFinishTemplateRunHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RunID string `json:"runId" schema:"runId"`
		}
		if err := tryFillStructValuesFromRequest(&req, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		runID := strings.TrimSpace(req.RunID)
		if runID == "" {
			http_utils.HandleError(errors.New("runId is required"), w, r, http.StatusBadRequest)
			return
		}
		if !application_context.FinishTemplateRun(runID, templateRunAccountKey(r)) {
			http_utils.HandleError(errors.New("no such generation run"), w, r, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}
