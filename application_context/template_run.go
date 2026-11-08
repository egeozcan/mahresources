package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mahresources/shortcodes"
)

// The run loop of harness mode (docs/plans/2026-11-07-template-generation-harness.md
// section 3). The generator stays DB-free: Tools run through the executor the
// handler put in TemplateRunEnv, and the provider is a TemplateConversationProvider.

// templateRunFinalCheckTimeout bounds the access check made before a Draft or
// transcript is released. It runs under the request context, not the run
// context, which may already have reached its deadline.
const templateRunFinalCheckTimeout = 5 * time.Second

// templateRunStubRetention is how many Turns of Tool results stay complete when
// the provider allows history to be rewritten.
const templateRunStubRetention = 3

// discloseHarness validates and normalizes a harness request and assembles the
// Seed, the Tools and the Disclosure. GenerateTemplate runs what this returns.
func (g *defaultTemplateGenerator) discloseHarness(ctx context.Context, in TemplateGenerationInput, prompt string) (templateRequest, error) {
	if strings.TrimSpace(g.config.APIKey) == "" || g.conversation == nil {
		return templateRequest{}, ErrTemplateGenerationNotConfigured
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || len(prompt) > MaxTemplateGenerationPromptLength {
		return templateRequest{}, fmt.Errorf("%w: prompt must be between 1 and %d characters", ErrTemplateGenerationBadRequest, MaxTemplateGenerationPromptLength)
	}
	// A caller that built no run environment must not start a run by accident.
	if in.Run == nil || in.Run.Executor == nil {
		return templateRequest{}, fmt.Errorf("%w: a harness run needs a run environment", ErrTemplateGenerationBadRequest)
	}
	if in.Target == "" {
		in.Target = TemplateTargetSlot
	}
	in.Postgres = g.config.Postgres
	in = normalizeSlotMode(in)
	if _, err := templateFanOutFor(in); err != nil {
		return templateRequest{}, err
	}

	settings := TemplateHarnessSettings{}
	if g.config.Harness != nil {
		settings = *g.config.Harness
	}
	settings = settings.withDefaults()
	fields, required, cssAdditions := templateRunFieldsOf(in)
	profile := g.config.profileFor(templateCallKind(in))
	sections := buildTemplateRunSections(in, prompt, fields, required, cssAdditions, settings)
	if err := privacySeedError(in); err != nil {
		return templateRequest{}, err
	}
	registerRunSkeleton(in.Privacy, g.conversation, profile.Model, TemplateRunToolSpecs())
	plan := &harnessPlan{
		fields: fields, required: required, cssAdditions: cssAdditions,
		sections: sections,
		system:   promptMessage(sections, 0, promptRoleSystem),
		seed:     promptMessage(sections, 0, promptRoleUser),
		tools:    TemplateRunToolSpecs(),
		profile:  profile, settings: settings,
	}

	// A model that cannot call Tools is refused where that is detectable without
	// a paid call. An error means "unknown" and does not refuse.
	if supported, err := g.conversation.SupportsTools(ctx, profile.Model); err == nil && !supported {
		return templateRequest{}, fmt.Errorf("%w: the model %s does not support tool calling, which generation runs need", ErrTemplateGenerationBadRequest, profile.Model)
	}
	seedRequest := ConversationRequest{System: plan.system, Tools: plan.tools, Messages: []ConversationMessage{{Role: "user", Text: plan.seed}}}
	if estimateConversationTokens(seedRequest)+templateRunFinalReserve(profile) > settings.TokenBudget {
		return templateRequest{}, fmt.Errorf("%w: the prompt is too large for the run's token budget of %d; raise ai_template_run_token_budget or shorten the request", ErrTemplateGenerationBadRequest, settings.TokenBudget)
	}
	disclosure := g.newHarnessDisclosure(in, plan)
	return templateRequest{in: in, prompt: prompt, disclosure: disclosure, run: plan}, nil
}

func templateRunFinalReserve(profile TemplateCallProfile) int {
	return min(profile.MaxTokens, templateRunFinalReserveTokens)
}

func (g *defaultTemplateGenerator) generateHarness(ctx context.Context, in TemplateGenerationInput, prompt, groundingHash string) (*TemplateGenerationResult, error) {
	request, err := g.discloseHarness(ctx, in, prompt)
	if err != nil {
		return nil, err
	}
	if groundingHash == "" {
		return nil, ErrTemplateGenerationGroundingRequired
	}
	if groundingHash != request.disclosure.GroundingHash {
		return nil, ErrTemplateGenerationGroundingChanged
	}
	return g.runHarness(ctx, request)
}

// runMessage is a message of the conversation with what stubbing needs to know.
type runMessage struct {
	msg     ConversationMessage
	turn    int
	results []runResultMeta
}

// runResultMeta describes one Tool result of a tool message.
type runResultMeta struct {
	label         string // "get_entity type=group id=3", for the stub
	turn          int
	transcriptIdx int
	stub          string // fixed once written
}

type harnessRun struct {
	g      *defaultTemplateGenerator
	reqCtx context.Context
	runCtx context.Context
	in     TemplateGenerationInput
	plan   *harnessPlan
	env    *TemplateRunEnv
	sink   TemplateRunSink
	// deadlineAt is when the run context expires.
	deadlineAt time.Time

	msgs        []runMessage
	transcript  []TemplateTranscriptRecord
	subs        *templateRunSubmissions
	explanation string
	// pendingRecords are instruction records not yet sent.
	pendingRecords []TemplateTranscriptRecord
	// unsentResults are transcript indexes of Tool results not yet sent.
	unsentResults []int
	privacy       *TemplatePrivacy
	undo          *commitUndo
	turn          int
	used          int
	plainTexts    int
	lengthTurns   int
	fieldNames    map[string]bool
}

func (g *defaultTemplateGenerator) runHarness(ctx context.Context, request templateRequest) (*TemplateGenerationResult, error) {
	plan := request.run
	// The transport guard reads the masker from the context the provider call runs on.
	ctx = WithTemplatePrivacy(ctx, request.in.Privacy)
	runCtx, cancel := context.WithTimeout(ctx, plan.settings.Deadline)
	defer cancel()
	r := &harnessRun{
		g: g, reqCtx: ctx, runCtx: runCtx, in: request.in, plan: plan, env: request.in.Run, sink: request.in.Run.Sink,
		subs:       newTemplateRunSubmissions(request.in, plan.cssAdditions),
		fieldNames: map[string]bool{},
		privacy:    request.in.Privacy,
	}
	r.deadlineAt, _ = runCtx.Deadline()
	for _, f := range plan.fields {
		r.fieldNames[f] = true
	}
	return r.loop()
}

func (r *harnessRun) loop() (*TemplateGenerationResult, error) {
	// The Seed is recorded with the first request actually sent (commitPending).
	r.pendingRecords = append(r.pendingRecords,
		TemplateTranscriptRecord{Turn: 1, Kind: TemplateTranscriptSeed, Text: r.plan.system, Bytes: len(r.plan.system)},
		TemplateTranscriptRecord{Turn: 1, Kind: TemplateTranscriptSeed, Text: r.plan.seed, Bytes: len(r.plan.seed)})
	r.msgs = append(r.msgs, runMessage{msg: ConversationMessage{Role: "user", Text: r.plan.seed}, turn: 1})

	for turn := 1; ; turn++ {
		r.turn = turn
		if err := r.reqCtx.Err(); err != nil {
			return nil, err
		}
		if r.runCtx.Err() != nil {
			return r.conclude(TemplateStopDeadline, ErrTemplateGenerationTimeout)
		}
		if err := r.checkAccess(r.runCtx); err != nil {
			return r.exitOnCheck(err)
		}

		reason := r.finalReason(turn)
		if reason != "" {
			r.appendInstruction(turn, templateRunFinalInstruction)
		}
		req, estimate, stubs, ok := r.admit(turn, &reason)
		if !ok {
			return r.conclude(TemplateStopBudget, nil)
		}
		if r.sink != nil {
			r.sink.TurnStarted(TemplateTurnEvent{Turn: turn, MaxTurns: r.plan.settings.MaxTurns, FinalReason: reason})
		}
		final := reason != ""

		turnStart := time.Now()
		resp, err := r.callProvider(req, func() { r.commitPending(turn, stubs) })
		if err != nil {
			return r.exitOnCallError(err)
		}
		r.used += ChargeableTokens(resp.Usage, estimate, req.MaxTokens)
		if r.sink != nil {
			r.sink.Usage(TemplateUsageEvent{Turn: turn, BudgetUsed: r.used, BudgetMax: r.plan.settings.TokenBudget})
		}
		// A run whose access changed during its provider call runs no Tools.
		if err := r.checkAccess(r.runCtx); err != nil {
			return r.exitOnCheck(err)
		}

		r.recordAssistant(turn, resp, time.Since(turnStart))
		override, done := r.handleReply(turn, final, resp)
		stop := ""
		switch {
		case done:
			stop = TemplateStopDone
		case override != "":
			stop = override
		case final:
			stop = reason
		}
		if stop != "" {
			return r.conclude(stop, nil)
		}
	}
}

// finalReason decides, before the outgoing messages are finished, whether this
// is the final Turn, and why. It records the first reason that applies, in the
// plan's order.
func (r *harnessRun) finalReason(turn int) string {
	switch {
	case turn >= r.plan.settings.MaxTurns:
		return TemplateStopTurnCap
	case r.env.Finish != nil && r.env.Finish():
		return TemplateStopFinishNow
	case time.Until(r.deadlineAt) < r.plan.profile.Timeout+r.g.toolTimeoutOrDefault():
		return TemplateStopDeadline
	case r.plan.settings.TokenBudget-r.used < templateRunFinalReserve(r.plan.profile):
		return TemplateStopBudget
	}
	return ""
}

// admit estimates the outgoing request and clamps max_tokens to what remains of
// the budget (plan section 3, step 3). A Turn that cannot keep a final reserve
// becomes final with what is left; under the minimum, nothing is called.
func (r *harnessRun) admit(turn int, reason *string) (ConversationRequest, int, []stubCommit, bool) {
	remaining := r.plan.settings.TokenBudget - r.used
	var stubs []stubCommit
	build := func() ConversationRequest {
		var msgs []ConversationMessage
		msgs, stubs = r.outgoing(turn)
		return ConversationRequest{
			System: r.plan.system, Messages: msgs, Tools: r.plan.tools,
			Model: r.plan.profile.Model, Thinking: r.plan.profile.Thinking,
		}
	}
	req := build()
	estimate := estimateConversationTokens(req)
	maxTokens := min(r.plan.profile.MaxTokens, remaining-estimate)
	if *reason == "" && maxTokens < templateRunFinalReserve(r.plan.profile) {
		*reason = TemplateStopBudget
		r.appendInstruction(turn, templateRunFinalInstruction)
		req = build()
		estimate = estimateConversationTokens(req)
		maxTokens = min(r.plan.profile.MaxTokens, remaining-estimate)
	}
	if maxTokens < templateRunMinTurnTokens {
		return req, estimate, nil, false
	}
	req.MaxTokens = maxTokens
	return req, estimate, stubs, true
}

// appendInstruction adds an append-only user message. It never edits the Seed
// or a Replay.
//
// Its transcript record waits in pendingRecords until a provider call is
// attempted (commitPending), so a Turn that budget admission refuses leaves no
// instruction in a transcript that claims to list what was sent.
func (r *harnessRun) appendInstruction(turn int, text string) {
	r.privacy.Keep(text)
	r.msgs = append(r.msgs, runMessage{msg: ConversationMessage{Role: "user", Text: text}, turn: turn})
	r.pendingRecords = append(r.pendingRecords, TemplateTranscriptRecord{Turn: turn, Kind: TemplateTranscriptInstruction, Text: text, Bytes: len(text)})
}

func (r *harnessRun) record(rec TemplateTranscriptRecord) int {
	r.transcript = append(r.transcript, rec)
	return len(r.transcript) - 1
}

// stubCommit is a stub computed for a tentative request, written only once the
// request is sent.
type stubCommit struct {
	msg, result int
	text        string
}

// commitPending writes what the request being sent introduced: the Seed and
// instruction records, and the stubs with the Turn they started in. Building a
// tentative request touches none of it.
func (r *harnessRun) commitPending(turn int, stubs []stubCommit) {
	undo := &commitUndo{recStart: len(r.transcript), results: append([]int(nil), r.unsentResults...)}
	r.undo = undo
	for _, rec := range r.pendingRecords {
		rec.Sent = true
		r.record(rec)
	}
	r.pendingRecords = nil
	// This request carries every Tool result so far.
	for _, idx := range r.unsentResults {
		r.transcript[idx].Sent = true
	}
	r.unsentResults = nil
	for _, s := range stubs {
		meta := &r.msgs[s.msg].results[s.result]
		if meta.stub == "" {
			meta.stub = s.text
			r.transcript[meta.transcriptIdx].StubbedFrom = turn
			undo.stubs = append(undo.stubs, s)
		}
	}
}

// commitUndo remembers what commitPending marked as sent, so a request the
// privacy guard refused can be taken back: the Transcript must never claim a
// blocked request was sent.
type commitUndo struct {
	recStart int
	results  []int
	stubs    []stubCommit
}

// rollbackCommit marks the refused Turn's records sent:false and reverts a stub
// first written in that Turn.
func (r *harnessRun) rollbackCommit() {
	u := r.undo
	if u == nil {
		return
	}
	for i := u.recStart; i < len(r.transcript); i++ {
		r.transcript[i].Sent = false
	}
	for _, idx := range u.results {
		r.transcript[idx].Sent = false
	}
	for _, s := range u.stubs {
		meta := &r.msgs[s.msg].results[s.result]
		r.transcript[meta.transcriptIdx].StubbedFrom = 0
		meta.stub = ""
	}
}

// outgoing returns the messages of this Turn's request. When the provider lets
// history be rewritten, Tool results older than the last templateRunStubRetention
// Turns are replaced by a stub that is fixed once written, so the history
// changes once per result. Every other provider is never rewritten: Anthropic
// binds each thinking signature to every preceding message.
func (r *harnessRun) outgoing(turn int) ([]ConversationMessage, []stubCommit) {
	rewritable := r.g.conversation.HistoryRewritable(r.plan.profile.Model)
	out := make([]ConversationMessage, len(r.msgs))
	var stubs []stubCommit
	for i := range r.msgs {
		m := r.msgs[i]
		if rewritable && len(m.results) > 0 {
			results := append([]ToolResult(nil), m.msg.Results...)
			for j := range results {
				meta := r.msgs[i].results[j]
				text := meta.stub
				if text == "" && meta.turn < turn-templateRunStubRetention {
					text = r.privacy.Keep(fmt.Sprintf("%s result, %d bytes, superseded", meta.label, len(results[j].Text)))
					stubs = append(stubs, stubCommit{msg: i, result: j, text: text})
				}
				if text != "" {
					results[j].Text = text
				}
			}
			m.msg.Results = results
		}
		out[i] = m.msg
	}
	return out, stubs
}

// checkAccess runs the executor's access check and sorts the outcome: nil,
// ErrTemplateRunAccessChanged, or the context's own error. Any other failure
// to verify access fails closed as a change.
func (r *harnessRun) checkAccess(ctx context.Context) error {
	err := r.env.Executor.CheckAccess(ctx)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrTemplateRunAccessChanged):
		return ErrTemplateRunAccessChanged
	case ctx.Err() != nil:
		return ctx.Err()
	}
	return ErrTemplateRunAccessChanged
}

// exitOnCheck ends the run after a failed access check: an access change
// releases nothing; client cancellation is a plain context error; an expired
// run deadline concludes the run as a deadline.
func (r *harnessRun) exitOnCheck(err error) (*TemplateGenerationResult, error) {
	if errors.Is(err, ErrTemplateRunAccessChanged) {
		return nil, &TemplateRunError{Err: ErrTemplateRunAccessChanged}
	}
	if r.reqCtx.Err() != nil {
		return nil, r.reqCtx.Err()
	}
	return r.conclude(TemplateStopDeadline, ErrTemplateGenerationTimeout)
}

// callProvider makes one provider call under the call timeout and the
// process-wide slot semaphore. The wait for a slot is inside the timeout. The
// access check runs after the slot is acquired and immediately before Converse,
// so a Turn that queued behind other runs and was revoked meanwhile sends
// nothing.
func (r *harnessRun) callProvider(req ConversationRequest, commit func()) (ConversationResponse, error) {
	timeout := min(r.plan.profile.Timeout, time.Until(r.deadlineAt))
	callCtx, cancel := context.WithTimeout(r.runCtx, timeout)
	defer cancel()
	select {
	case r.g.slots <- struct{}{}:
		defer func() { <-r.g.slots }()
	case <-callCtx.Done():
		return ConversationResponse{}, callError{callCtx, callCtx.Err()}
	}
	if err := callCtx.Err(); err != nil {
		return ConversationResponse{}, callError{callCtx, err}
	}
	if err := r.checkAccess(callCtx); err != nil {
		if errors.Is(err, ErrTemplateRunAccessChanged) {
			return ConversationResponse{}, err
		}
		return ConversationResponse{}, callError{callCtx, err}
	}
	// Past the slot and the last access check: this request is really sent.
	commit()
	start := time.Now()
	resp, err := r.g.conversation.Converse(callCtx, req)
	r.g.config.Logf("template generation: phase=run turn=%d model=%s finish=%s dur=%dms err=%t",
		r.turn, r.plan.profile.Model, resp.Finish, time.Since(start).Milliseconds(), err != nil)
	if err != nil {
		return ConversationResponse{}, callError{callCtx, err}
	}
	return resp, nil
}

// failPrivacy ends a run whose request the guard refused. The request was not
// sent, so its records go back to sent:false. Nothing is released as a Draft,
// even when the run already has submissions: a Draft built before the leak
// would be safe, but it would hide a bug, so the run fails loudly. The
// transcript and legend are released only if access still stands.
func (r *harnessRun) failPrivacy(cause error) (*TemplateGenerationResult, error) {
	r.rollbackCommit()
	if err := r.reqCtx.Err(); err != nil {
		return nil, err
	}
	finalCtx, cancel := context.WithTimeout(r.reqCtx, templateRunFinalCheckTimeout)
	defer cancel()
	if err := r.checkAccess(finalCtx); err != nil {
		if r.reqCtx.Err() != nil {
			return nil, r.reqCtx.Err()
		}
		return nil, &TemplateRunError{Err: ErrTemplateRunAccessChanged}
	}
	return nil, &TemplateRunError{Err: cause, StopReason: TemplateStopPrivacyGuard, Transcript: r.transcript, Legend: r.privacy.Legend()}
}

// callError remembers which call context a provider failure happened under.
type callError struct {
	ctx context.Context
	err error
}

func (e callError) Error() string { return e.err.Error() }
func (e callError) Unwrap() error { return e.err }

func (r *harnessRun) exitOnCallError(err error) (*TemplateGenerationResult, error) {
	if errors.Is(err, ErrTemplateRunAccessChanged) {
		return nil, &TemplateRunError{Err: ErrTemplateRunAccessChanged}
	}
	if errors.Is(err, ErrTemplatePrivacyLeak) {
		return r.failPrivacy(err)
	}
	// The guard ran out of time or was cancelled before the transport sent
	// anything: take back the records marked as sent, then handle it as the
	// timeout or cancellation it is. An ordinary provider timeout is not rolled
	// back, because that request may really have been sent.
	var interrupted *TemplatePrivacyGuardInterrupted
	if errors.As(err, &interrupted) {
		r.rollbackCommit()
	}
	if r.reqCtx.Err() != nil {
		return nil, r.reqCtx.Err()
	}
	var ce callError
	callCtx := r.runCtx
	if errors.As(err, &ce) {
		callCtx = ce.ctx
	}
	switch {
	case r.runCtx.Err() != nil:
		return r.conclude(TemplateStopDeadline, ErrTemplateGenerationTimeout)
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded):
		return r.conclude(TemplateStopProviderError, ErrTemplateGenerationTimeout)
	}
	return r.conclude(TemplateStopProviderError, fmt.Errorf("%w: %v", ErrTemplateGenerationProvider, err))
}

func (r *harnessRun) recordAssistant(turn int, resp ConversationResponse, took time.Duration) {
	calls := make([]ToolCall, len(resp.Calls))
	copy(calls, resp.Calls)
	rec := TemplateTranscriptRecord{
		Turn: turn, Kind: TemplateTranscriptAssistant, Sent: true, Text: resp.Text, Bytes: len(resp.Text),
		DurationMs: took.Milliseconds(), Replay: append(json.RawMessage(nil), resp.Replay...),
		ReplayText: string(resp.Replay),
		Usage: &TemplateTranscriptUsage{
			Prompt: resp.Usage.Prompt, Completion: resp.Usage.Completion, Reasoning: resp.Usage.Reasoning,
			CacheHit: resp.Usage.CacheHit, CacheMiss: resp.Usage.CacheMiss,
		},
	}
	for _, c := range calls {
		rec.Calls = append(rec.Calls, TemplateTranscriptCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments})
	}
	r.record(rec)
	r.privacy.KeepModelOutput(resp.Text, calls, resp.Replay)
	r.msgs = append(r.msgs, runMessage{msg: ConversationMessage{Role: "assistant", Text: resp.Text, Calls: calls, Replay: resp.Replay}, turn: turn})
}

// callOutcome is one Tool call's result before it is committed.
type callOutcome struct {
	text    string
	isError bool
	took    time.Duration
	submit  *pendingSubmit
	// readable marks a text the loop itself wrote (a fixed failure text, a
	// submit result): it carries no entity data and joins the guard's corpus.
	readable bool
}

// pendingSubmit is a validated submit call, committed in call order after
// every call of the Turn has ended.
type pendingSubmit struct {
	fields      map[string]string
	order       []string
	issues      map[string][]shortcodes.LintIssue
	explanation string
	done        bool
}

// handleReply acts on a reply. It returns a stop reason that overrides the
// final Turn's own (no_tool_call, length_repeated), and whether a done
// submission committed.
func (r *harnessRun) handleReply(turn int, final bool, resp ConversationResponse) (override string, done bool) {
	if len(resp.Calls) == 0 {
		return r.handleNoCalls(turn, final, resp), false
	}
	snapshot := r.subs.snapshot()
	outcomes := make([]callOutcome, len(resp.Calls))
	var wg sync.WaitGroup
	for i, call := range resp.Calls {
		switch {
		case i >= TemplateRunMaxCallsPerTurn:
			outcomes[i] = callOutcome{text: templateRunTooManyText, isError: true, readable: true}
		case call.Incomplete:
			outcomes[i] = callOutcome{text: templateRunCutOffText, isError: true, readable: true}
		case call.Malformed:
			outcomes[i] = callOutcome{text: templateRunMalformedText, isError: true, readable: true}
		case !isCatalogueTool(call.Name):
			outcomes[i] = callOutcome{text: "unknown Tool; the Tools are listed in the request", isError: true, readable: true}
		case final && call.Name != ToolSubmit:
			outcomes[i] = callOutcome{text: templateRunNotRunFinalText, isError: true, readable: true}
		case call.Name == ToolSubmit:
			wg.Add(1)
			go func(i int, call ToolCall) {
				defer wg.Done()
				outcomes[i] = r.validateSubmit(call)
			}(i, call)
		default:
			wg.Add(1)
			go func(i int, call ToolCall) {
				defer wg.Done()
				outcomes[i] = r.runTool(turn, call, snapshot)
			}(i, call)
		}
	}
	wg.Wait()

	// Commit in call order, so two submits of one field in a Turn resolve to the
	// later call whichever finished validating first.
	for i := range outcomes {
		p := outcomes[i].submit
		if p == nil {
			continue
		}
		for _, field := range p.order {
			r.subs.commit(field, p.fields[field])
			errs, warns := countIssues(p.issues[field])
			if r.sink != nil {
				r.sink.Submitted(TemplateSubmissionEvent{Turn: turn, Field: field, Errors: errs, Warnings: warns})
			}
		}
		if p.explanation != "" {
			r.explanation = p.explanation
		}
		done = done || p.done
	}

	results := make([]ToolResult, len(outcomes))
	metas := make([]runResultMeta, len(outcomes))
	completeCall := false
	for i, o := range outcomes {
		call := resp.Calls[i]
		if o.readable {
			r.privacy.Keep(o.text)
		}
		results[i] = ToolResult{CallID: call.ID, Text: o.text, IsError: o.isError}
		idx := r.record(TemplateTranscriptRecord{
			Turn: turn, Kind: TemplateTranscriptToolResult, Text: o.text, CallID: call.ID, IsError: o.isError,
			Bytes: len(o.text), DurationMs: o.took.Milliseconds(),
		})
		r.unsentResults = append(r.unsentResults, idx)
		metas[i] = runResultMeta{label: stubLabel(call), turn: turn, transcriptIdx: idx}
		if r.sink != nil {
			r.sink.ToolFinished(TemplateToolEvent{
				Turn: turn, Name: progressName(call.Name), Args: r.progressArgs(call.Arguments),
				Bytes: len(o.text), IsError: o.isError, DurationMs: o.took.Milliseconds(),
			})
		}
		if !call.Incomplete && !call.Malformed {
			completeCall = true
		}
	}
	r.msgs = append(r.msgs, runMessage{msg: ConversationMessage{Role: "tool", Results: results}, turn: turn, results: metas})

	if resp.Finish == "length" && !completeCall {
		r.lengthTurns++
		if r.lengthTurns >= 2 {
			return TemplateStopLengthRepeated, done
		}
	} else {
		r.lengthTurns = 0
	}
	return "", done
}

// handleNoCalls answers a reply with no Tool call: a length cut gets the same
// message a cut call gets, plain text gets one nudge.
func (r *harnessRun) handleNoCalls(turn int, final bool, resp ConversationResponse) string {
	if resp.Finish == "length" {
		r.lengthTurns++
		if r.lengthTurns >= 2 {
			return TemplateStopLengthRepeated
		}
		if !final {
			r.appendInstruction(turn, "Your reply was "+templateRunCutOffText+".")
		}
		return ""
	}
	r.lengthTurns = 0
	r.plainTexts++
	if final || r.plainTexts >= 2 {
		return TemplateStopNoToolCall
	}
	r.appendInstruction(turn, templateRunNudgeText)
	return ""
}

// runTool executes one non-submit Tool under its timeout, containing a panic.
func (r *harnessRun) runTool(turn int, call ToolCall, snapshot map[string]string) (out callOutcome) {
	start := time.Now()
	defer func() {
		out.took = time.Since(start)
		if p := recover(); p != nil {
			r.g.config.Logf("template generation: Tool %s panicked: %v", call.Name, p)
			out.text, out.isError, out.readable = "the Tool failed", true, true
		}
	}()
	timeout := r.g.toolTimeoutOrDefault()
	if call.Name == ToolPreview {
		timeout = r.g.previewTimeoutOrDefault()
	}
	ctx, cancel := context.WithTimeout(r.runCtx, timeout)
	defer cancel()
	result, err := r.env.Executor.Execute(ctx, TemplateToolInvocation{
		CallID: call.ID, Name: call.Name, Arguments: call.Arguments, Turn: turn, Snapshot: snapshot,
	})
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)):
		return callOutcome{text: "the Tool timed out", isError: true, readable: true}
	case err != nil:
		return callOutcome{text: "the Tool failed", isError: true, readable: true}
	}
	return callOutcome{text: result.Text, isError: result.IsError}
}

// validateSubmit parses and validates a submit call against the Turn-start
// state. It commits nothing.
func (r *harnessRun) validateSubmit(call ToolCall) (out callOutcome) {
	start := time.Now()
	defer func() {
		out.took = time.Since(start)
		if p := recover(); p != nil {
			r.g.config.Logf("template generation: submit panicked: %v", p)
			out = callOutcome{text: "the Tool failed", isError: true, took: time.Since(start), readable: true}
		}
	}()
	args, err := parseSubmitArgs(call.Arguments, r.fieldNames, r.plan.fields)
	if err != nil {
		return callOutcome{text: submitErrorText(err.Error()), isError: true, readable: true}
	}
	ctx, cancel := context.WithTimeout(r.runCtx, r.g.toolTimeoutOrDefault())
	defer cancel()
	pending := &pendingSubmit{fields: args.fields, issues: map[string][]shortcodes.LintIssue{}, explanation: args.explanation, done: args.done}
	var refusals []string
	for _, field := range r.plan.fields {
		value, ok := args.fields[field]
		if !ok {
			continue
		}
		// A Placeholder in a Draft is refused, not linted (P11): an invalid
		// submission is still committed, and a Placeholder must never reach one.
		if placeholder, bad := r.privacy.ContainsIssued(value); bad {
			refusals = append(refusals, fmt.Sprintf("%s refused: contains the Placeholder %s; reference the field with [meta]/[property] instead", field, placeholder))
			continue
		}
		pending.order = append(pending.order, field)
		pending.issues[field] = r.subs.validateSubmission(ctx, field, value)
	}
	text := submitResultText(pending.order, pending.issues, args.explanation)
	if len(refusals) > 0 {
		// The call's done is ignored; its other fields commit normally.
		pending.done = false
		text += "\n" + strings.Join(refusals, "\n")
		return callOutcome{text: text, isError: true, submit: pending, readable: true}
	}
	return callOutcome{text: text, submit: pending, readable: true}
}

// conclude ends the run. Before anything is released (a Draft, a recovered
// Draft or a transcript) access is checked once more, under the request
// context with its own 5 s timeout and not the run context, which may already
// have reached its deadline: a Draft recovered after the run deadline is
// validated and delivered normally, and client cancellation still cancels.
func (r *harnessRun) conclude(stop string, cause error) (*TemplateGenerationResult, error) {
	if err := r.reqCtx.Err(); err != nil {
		return nil, err
	}
	finalCtx, cancel := context.WithTimeout(r.reqCtx, templateRunFinalCheckTimeout)
	defer cancel()
	if err := r.checkAccess(finalCtx); err != nil {
		if r.reqCtx.Err() != nil {
			return nil, r.reqCtx.Err()
		}
		return nil, &TemplateRunError{Err: ErrTemplateRunAccessChanged}
	}
	if !r.subs.any() {
		if cause == nil {
			cause = fmt.Errorf("%w: the model returned no draft", ErrTemplateGenerationProvider)
		}
		return nil, &TemplateRunError{Err: cause, StopReason: stop, Transcript: r.transcript, Legend: r.privacy.Legend()}
	}
	result := r.buildResult(finalCtx, stop, cause)
	// A cancelled lookup counts as existing for the linter, so the Draft built
	// under a cancelled request must not be released.
	if err := r.reqCtx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// buildResult builds the Draft from the last committed submission of each
// field, re-linted with the one validator.
func (r *harnessRun) buildResult(ctx context.Context, stop string, cause error) *TemplateGenerationResult {
	in := r.in
	result := &TemplateGenerationResult{Target: in.Target, StopReason: stop, Transcript: r.transcript, Legend: r.privacy.Legend()}
	var issues []shortcodes.LintIssue
	issuesByField := map[string][]shortcodes.LintIssue{}
	submitted := map[string]bool{}
	for _, field := range r.plan.fields {
		if _, ok := r.subs.raw[field]; !ok {
			continue
		}
		submitted[field] = true
		result.Submitted = append(result.Submitted, field)
		fieldIssues := r.subs.validateSubmission(ctx, field, r.subs.raw[field])
		issuesByField[field] = fieldIssues
	}

	// A pair is applied whole or not at all.
	for _, field := range r.plan.fields {
		if strings.HasSuffix(field, "CSS") || !r.fieldNames[field+"CSS"] || submitted[field] == submitted[field+"CSS"] {
			continue
		}
		have, missing := field, field+"CSS"
		if submitted[field+"CSS"] {
			have, missing = field+"CSS", field
		}
		issuesByField[have] = append(issuesByField[have], shortcodes.LintIssue{
			Severity: shortcodes.SeverityError,
			Message:  fmt.Sprintf("%s was submitted without %s, so this pair cannot be applied.", have, missing),
		})
	}

	switch in.Target {
	case TemplateTargetBundle:
		result.Slots = map[string]string{}
		result.SlotIssues = map[string][]shortcodes.LintIssue{}
		for _, field := range result.Submitted {
			result.Slots[field] = r.subs.resolved(field)
			if len(issuesByField[field]) > 0 {
				result.SlotIssues[field] = issuesByField[field]
			}
			issues = append(issues, issuesByField[field]...)
		}
	default:
		field := r.plan.fields[0]
		if submitted[field] {
			result.ContentSubmitted = true
			result.Content = r.subs.resolved(field)
			issues = append(issues, issuesByField[field]...)
		}
	}

	// Backstop for P11: submit refuses a Placeholder, so this should never fire.
	var bad []string
	for _, field := range result.Submitted {
		if _, found := r.privacy.ContainsIssued(r.subs.resolved(field)); found {
			bad = append(bad, field)
		}
	}
	if len(bad) > 0 {
		withholdFields(result, bad, "")
		issues = append(issues, result.Issues...)
	}

	if r.plan.required {
		for _, field := range r.plan.fields {
			if !submitted[field] {
				issues = append(issues, shortcodes.LintIssue{Severity: shortcodes.SeverityWarning, Message: field + " was requested but the model did not submit it."})
			}
		}
	}
	if stop == TemplateStopDeadline || stop == TemplateStopProviderError {
		why := "the provider failed"
		if stop == TemplateStopDeadline || errors.Is(cause, ErrTemplateGenerationTimeout) {
			why = "it ran out of time"
		}
		issues = append(issues, shortcodes.LintIssue{Severity: shortcodes.SeverityWarning, Message: "The run ended early (" + why + "); this is the last draft the model submitted."})
	}
	explanation, explanationIssues := templateExplanation(r.explanation, in.Target == TemplateTargetBundle)
	issues = append(issues, explanationIssues...)
	if issues == nil {
		issues = []shortcodes.LintIssue{}
	}
	result.Explanation = explanation
	result.Issues = issues
	result.Valid = !hasErrorIssue(issues)
	return result
}

// Tool names a call may carry.
var templateRunToolNames = func() map[string]bool {
	m := map[string]bool{}
	for _, spec := range templateRunToolSpecs {
		m[spec.Name] = true
	}
	return m
}()

func isCatalogueTool(name string) bool { return templateRunToolNames[name] }

func progressName(name string) string {
	if isCatalogueTool(name) {
		return name
	}
	return "unknown"
}

// templateRunSlotNames are the template fields a progress event may name.
var templateRunSlotNames = func() map[string]bool {
	m := map[string]bool{"MetaSchema": true, "SectionConfig": true, "CustomCSS": true}
	for _, slot := range []string{
		"CustomHeader", "CustomSidebar", "CustomSummary", "CustomAvatar", "CustomDetailFooter", "CustomHoverCard",
		"CustomPreview", "CustomLightbox", "CustomCell", "CustomOwnEntities", "CustomListHeader", "CustomListFooter",
		"CustomMRQLResult", "CustomEntityPickerResult",
	} {
		m[slot], m[slot+"CSS"] = true, true
	}
	return m
}()

// progressArgs keeps only enumerated arguments of a call for the progress
// stream: a field name from the known set, the entity, direction and carrier
// enums, and integers as decimal text. Everything else is dropped, so a frame
// never carries content, queries, names or error text.
func (r *harnessRun) progressArgs(raw string) map[string]string {
	var args map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil
	}
	enums := map[string]map[string]bool{
		"type":        {"group": true, "resource": true, "note": true},
		"kind":        {"group": true, "resource": true, "note": true},
		"entityType":  {"group": true, "resource": true, "note": true},
		"direction":   {"up": true, "down": true, "across": true},
		"carrierType": {"category": true, "resourceCategory": true, "noteType": true},
	}
	out := map[string]string{}
	for key, value := range args {
		switch key {
		case "field":
			var s string
			if json.Unmarshal(value, &s) == nil && (r.fieldNames[s] || templateRunSlotNames[s]) {
				out[key] = s
			}
		case "type", "kind", "entityType", "direction", "carrierType":
			var s string
			if json.Unmarshal(value, &s) == nil && enums[key][s] {
				out[key] = s
			}
		case "id", "entityId", "number", "from", "to", "offset", "limit", "buckets":
			var n int64
			dec := json.NewDecoder(bytes.NewReader(value))
			if dec.Decode(&n) == nil && n >= 0 && n < 1<<31 {
				out[key] = strconv.FormatInt(n, 10)
			}
		}
	}
	return out
}

// stubLabel names a Tool result for its stub: the Tool and its enumerated
// arguments, never free text.
func stubLabel(call ToolCall) string {
	var args map[string]json.RawMessage
	label := progressName(call.Name)
	if json.Unmarshal([]byte(call.Arguments), &args) != nil {
		return label
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var s string
		var n int64
		switch {
		case (k == "type" || k == "carrierType" || k == "direction" || k == "kind") && json.Unmarshal(args[k], &s) == nil && len(s) <= 20 && isAlphaWord(s):
			label += " " + k + "=" + s
		case (k == "id" || k == "number") && json.Unmarshal(args[k], &n) == nil:
			label += " " + k + "=" + strconv.FormatInt(n, 10)
		}
	}
	return label
}

func isAlphaWord(s string) bool {
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return s != ""
}
