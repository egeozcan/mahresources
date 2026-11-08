package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/plugin_system"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// The Tool executor of a harness run (plan section 2): the read Tools,
// run against the live database as the account the run started as. Argument
// validation, caps and fencing are application_context's
// (template_run_tools_exec.go); this file adds the database.

const (
	// templateRunPreviewConcurrency and templateRunDiffConcurrency bound how
	// many of a run's parallel calls render or diff at once.
	templateRunPreviewConcurrency = 2
	templateRunDiffConcurrency    = 1

	templateRunRenderOutputBytes = 256 << 10
	templateRunRenderParsedBytes = 512 << 10
)

// TemplateRunToolsConfig is what the generate handler knows when it builds the
// executor for one run.
type TemplateRunToolsConfig struct {
	// Start is the application context bound to the request's principal.
	Start *application_context.MahresourcesContext
	// Access is the TemplateRunAccessFactory function for this request; nil
	// means access cannot change (CheckAccess then only honours ctx).
	Access func(ctx context.Context) (*application_context.MahresourcesContext, error)
	// Principal is the request's principal. Preview rendering needs it on the
	// render context, which is derived from the run context.
	Principal *auth.Principal
	// EntityType is the member type the carrier being edited governs (group,
	// resource or note); CarrierID is that carrier, 0 on the create form.
	EntityType string
	CarrierID  uint
	// PreviewEntityID is the entity a preview renders against when the call
	// names none (the Seed's first sample); 0 means the call must name one.
	PreviewEntityID uint
	OptIns          application_context.TemplateRunOptIns
	// FieldValue returns a field's value in the Turn-start snapshot: its latest
	// committed submission, else the editor value. Used when lint or preview
	// omit content.
	FieldValue func(field string) (string, bool)
	// Input is the run's generation input; Validate is the single harness
	// validator (application_context.ValidateTemplateRunField) so lint, submit
	// and the final Draft agree. Nil Validate falls back to
	// ValidateTemplateRunContent.
	Input    application_context.TemplateGenerationInput
	Validate func(ctx context.Context, in application_context.TemplateGenerationInput, field, content string) []shortcodes.LintIssue
	// Known, DisplayTypes and DisplayTypesTruncated are the request's plugin
	// snapshot; zero values are read from Start.
	Known                 shortcodes.KnownShortcodes
	DisplayTypes          []string
	DisplayTypesTruncated bool
	// Privacy is the run's masker (nil when Privacy mode is off). Every Entity
	// value a Tool returns passes through it; readable results join its corpus.
	Privacy *application_context.TemplatePrivacy
}

// NewTemplateToolExecutor builds the executor for one run.
func NewTemplateToolExecutor(cfg TemplateRunToolsConfig) application_context.TemplateToolExecutor {
	e := &templateToolExecutor{
		cfg:      cfg,
		bound:    cfg.Start,
		previews: make(chan struct{}, templateRunPreviewConcurrency),
		diffs:    make(chan struct{}, templateRunDiffConcurrency),
	}
	if e.cfg.Known == nil {
		refs := serializeTemplateGenerationPluginReferencesForPrompt(cfg.Start)
		e.cfg.Known, e.cfg.DisplayTypes, e.cfg.DisplayTypesTruncated = refs.Known, refs.DisplayTypes, refs.DisplayTypesTruncated
	}
	return e
}

type templateToolExecutor struct {
	cfg      TemplateRunToolsConfig
	mu       sync.RWMutex
	bound    *application_context.MahresourcesContext
	previews chan struct{}
	diffs    chan struct{}
}

// CheckAccess re-resolves the original credential under ctx. When access is
// unchanged it swaps the bound context for the rebuilt one, so the subtree
// allow-list in force is the one just resolved. An error leaves the bound
// context as it was.
func (e *templateToolExecutor) CheckAccess(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.cfg.Access == nil {
		return nil
	}
	next, err := e.cfg.Access(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.bound = next
	e.mu.Unlock()
	return nil
}

func (e *templateToolExecutor) current() *application_context.MahresourcesContext {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.bound
}

// toolRefusal is a failure the model should read: it becomes an error result and
// does not end the run.
type toolRefusal string

func (r toolRefusal) Error() string { return string(r) }

func refusef(format string, args ...any) error { return toolRefusal(fmt.Sprintf(format, args...)) }

// Execute runs one Tool call. It returns an error only when ctx ended, which
// the loop turns into the run's stop; every other failure is an error result.
func (e *templateToolExecutor) Execute(ctx context.Context, call application_context.TemplateToolInvocation) (out application_context.TemplateToolOutput, err error) {
	// Every error result is a refusal or a failure text the server (or the model's
	// own argument) wrote, never a stored value, so it joins the readable corpus
	// at the one place all of them pass.
	defer func() {
		if out.IsError {
			e.cfg.Privacy.Keep(out.Text)
		}
	}()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[template-run] Tool %q panicked: %v\n%s", call.Name, r, debug.Stack())
			out, err = application_context.ToolErrorResult("the Tool failed"), nil
		}
	}()
	if cerr := ctx.Err(); cerr != nil {
		return application_context.TemplateToolOutput{}, cerr
	}
	args, derr := application_context.DecodeToolArguments(call.Name, call.Arguments)
	if derr != nil {
		return application_context.ToolErrorResult(derr.Error()), nil
	}
	// Each call runs on a copy rebased onto its own context, so cancel, the
	// run deadline and the per-Tool timeout reach every query.
	mc := e.current().WithParentContext(ctx)

	var body string
	switch a := args.(type) {
	case application_context.GetEntityArgs:
		body, err = e.getEntity(mc, a)
	case application_context.ListRelatedArgs:
		body, err = e.listRelated(mc, a)
	case application_context.GetTemplateArgs:
		body, err = e.getTemplate(mc, a)
	case application_context.ListCarriersArgs:
		body, err = e.listCarriers(mc, a)
	case application_context.ListMembersArgs:
		body, err = e.listMembers(mc, a)
	case application_context.RevisionArgs:
		switch call.Name {
		case application_context.ToolListRevisions:
			body, err = e.listRevisions(mc, a)
		case application_context.ToolGetRevision:
			body, err = e.getRevision(mc, a)
		default:
			body, err = e.diffRevisions(ctx, mc, a)
		}
	case application_context.LintArgs:
		body, err = e.lint(ctx, mc, a, call.Snapshot)
	case application_context.CheckMRQLArgs:
		body, err = e.checkMRQL(mc, a)
	case application_context.PreviewArgs:
		body, err = e.preview(ctx, mc, a, call.Snapshot)
	case application_context.NameArgs:
		if call.Name == application_context.ToolGetPartial {
			body, err = e.getPartial(mc, a)
		} else {
			body, err = e.getShortcodeDocs(mc, a)
		}
	default:
		return application_context.ToolErrorResult(fmt.Sprintf("unknown Tool %q", call.Name)), nil
	}
	if err != nil {
		return e.failure(ctx, call.Name, err)
	}
	// A result computed while the context ended may rest on a lookup that was
	// cut short; it is not released.
	if cerr := ctx.Err(); cerr != nil {
		return application_context.TemplateToolOutput{}, cerr
	}
	// Under Privacy mode a masker that hit a limit has masked only part of what
	// this result would show, so the whole result is thrown away.
	if perr := e.cfg.Privacy.Err(); perr != nil {
		return application_context.ToolErrorResult("too much entity data for Privacy mode"), nil
	}
	// Readable leaves are registered where each Tool builds its result (partial
	// source, lint messages, check_mrql text, documentation, template slots), not
	// from the finished body: a body that mixes readable and stored text must not
	// be exempted wholesale.
	return application_context.ShapeToolResultFor(e.cfg.Privacy, "tool "+call.Name, body, application_context.ToolResultCap(call.Name)), nil
}

// failure turns a Tool error into the result the model sees. A missing entity
// and an out-of-scope one share one text.
func (e *templateToolExecutor) failure(ctx context.Context, tool string, err error) (application_context.TemplateToolOutput, error) {
	if cerr := ctx.Err(); cerr != nil {
		return application_context.TemplateToolOutput{}, cerr
	}
	var refusal toolRefusal
	switch {
	case errors.As(err, &refusal):
		return application_context.ToolErrorResult(string(refusal)), nil
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, application_context.ErrTemplateRunEntityUnavailable),
		errors.Is(err, application_context.ErrTemplateRevisionNotFound):
		return application_context.ToolErrorResult(application_context.ToolEntityUnavailable), nil
	case errors.Is(err, application_context.ErrRoleCapability):
		return application_context.ToolErrorResult("not allowed: reading revisions needs edit rights to that carrier"), nil
	case errors.Is(err, shortcodes.ErrRenderBudgetExceeded):
		return application_context.ToolErrorResult("the render exceeded its output or source budget"), nil
	}
	log.Printf("[template-run] Tool %q failed: %v", tool, err)
	return application_context.ToolErrorResult("the Tool failed to read that"), nil
}

// carrierTypeFor maps a member entity type to its carrier type.
func carrierTypeFor(entityType string) string {
	switch entityType {
	case "group":
		return models.TemplateCarrierCategory
	case "resource":
		return models.TemplateCarrierResourceCategory
	}
	return models.TemplateCarrierNoteType
}

// entityTypeFor maps a carrier type to the member entity type it governs.
func entityTypeFor(carrierType string) string {
	switch carrierType {
	case models.TemplateCarrierCategory:
		return "group"
	case models.TemplateCarrierResourceCategory:
		return "resource"
	}
	return "note"
}

func carrierNameOf(carrier any) string {
	switch c := carrier.(type) {
	case *models.Category:
		return c.Name
	case *models.ResourceCategory:
		return c.Name
	case *models.NoteType:
		return c.Name
	}
	return ""
}

// revisionTarget resolves which carrier a revision Tool addresses: the named one,
// else the carrier being edited.
func (e *templateToolExecutor) revisionTarget(a application_context.RevisionArgs) (string, uint, error) {
	if a.CarrierType != "" {
		return a.CarrierType, a.ID, nil
	}
	if e.cfg.CarrierID == 0 {
		return "", 0, refusef("the carrier being edited is not saved yet; pass carrierType and id")
	}
	return carrierTypeFor(e.cfg.EntityType), e.cfg.CarrierID, nil
}

// fullSampleMeta decodes an entity's whole Meta, keeping numbers exact. The
// sample projection clips Meta for the prompt; a Tool shows all of it, and the
// result cap bounds the size.
func fullSampleMeta(entity any) any {
	var raw []byte
	switch x := entity.(type) {
	case *models.Group:
		raw = x.Meta
	case *models.Resource:
		raw = x.Meta
	case *models.Note:
		raw = x.Meta
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil
	}
	return v
}

type toolCarrierRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func (e *templateToolExecutor) getEntity(mc *application_context.MahresourcesContext, a application_context.GetEntityArgs) (string, error) {
	entity, carrierID, err := loadPreviewEntity(mc, a.Type, a.ID)
	if err != nil {
		return "", err
	}
	privacy := e.cfg.Privacy
	schema := application_context.TemplateEntityMetaSchema(entity)
	if carrierID != 0 && schema == "" {
		if carrier, cerr := loadPreviewCarrier(mc, a.Type, carrierID); cerr == nil {
			schema = carrierMetaSchema(carrier)
		}
	}
	sample := application_context.ProjectTemplateSampleFor(entity, true, privacy, schema)
	if meta := fullSampleMeta(entity); meta != nil {
		// The whole Meta, not the clipped sample: masked with the entity's own
		// category schema, so an enum value stays readable.
		sample["Meta"] = privacy.Value(meta, parseToolSchema(schema))
	}
	privacy.KeepScalar(int64(a.ID))
	result := map[string]any{"type": a.Type, "id": a.ID, "entity": sample}
	if carrierID != 0 {
		if carrier, cerr := loadPreviewCarrier(mc, a.Type, carrierID); cerr == nil {
			privacy.KeepScalar(int64(carrierID))
			result["carrier"] = toolCarrierRef{ID: carrierID, Name: privacy.Keep(carrierNameOf(carrier))}
		}
	}
	return application_context.MarshalToolJSON(result), nil
}

type toolEntry struct {
	Type    string          `json:"type"`
	ID      uint            `json:"id"`
	Name    string          `json:"name"`
	Carrier *toolCarrierRef `json:"carrier,omitempty"`
}

func toolEntries(privacy *application_context.TemplatePrivacy, in []application_context.TemplateRelatedEntry) []toolEntry {
	out := make([]toolEntry, len(in))
	for i, e := range in {
		privacy.KeepScalar(int64(e.ID))
		out[i] = toolEntry{Type: e.Type, ID: e.ID, Name: privacy.Text(e.Name)}
		if e.CarrierID != 0 {
			privacy.KeepScalar(int64(e.CarrierID))
			out[i].Carrier = &toolCarrierRef{ID: e.CarrierID, Name: privacy.Keep(e.CarrierName)}
		}
	}
	return out
}

func pagedJSON(privacy *application_context.TemplatePrivacy, key string, entries any, more bool, offset, n int) string {
	result := map[string]any{key: entries, "more": more}
	if more {
		result["nextOffset"] = offset + n
		// The generated page cursor is a preserved number: readable.
		privacy.KeepScalar(int64(offset + n))
	}
	return application_context.MarshalToolJSON(result)
}

func (e *templateToolExecutor) listRelated(mc *application_context.MahresourcesContext, a application_context.ListRelatedArgs) (string, error) {
	entries, more, err := mc.ListTemplateRunRelated(a.Type, a.ID, a.Direction, a.Kind, a.Offset, application_context.ToolListEntryCap)
	if err != nil {
		return "", err
	}
	e.cfg.Privacy.KeepScalar(int64(a.Offset + len(entries)))
	return pagedJSON(e.cfg.Privacy, "entries", toolEntries(e.cfg.Privacy, entries), more, a.Offset, len(entries)), nil
}

func (e *templateToolExecutor) listMembers(mc *application_context.MahresourcesContext, a application_context.ListMembersArgs) (string, error) {
	entries, more, err := mc.ListTemplateRunMembers(a.CarrierType, a.ID, a.Offset, application_context.ToolListEntryCap)
	if err != nil {
		return "", err
	}
	out := toolEntries(e.cfg.Privacy, entries)
	e.cfg.Privacy.KeepScalar(int64(a.Offset + len(entries)))
	for i := range out {
		out[i].Carrier = nil // the carrier is the one asked about
	}
	return pagedJSON(e.cfg.Privacy, "entries", out, more, a.Offset, len(entries)), nil
}

func (e *templateToolExecutor) getTemplate(mc *application_context.MahresourcesContext, a application_context.GetTemplateArgs) (string, error) {
	carrier, err := loadPreviewCarrier(mc, entityTypeFor(a.CarrierType), a.ID)
	if err != nil {
		return "", err
	}
	snapshot := models.NewTemplateSnapshot(a.CarrierType, carrier)
	fields := map[string]any{}
	if len(a.Fields) == 0 {
		for k, v := range snapshot {
			fields[k] = e.templateFieldValue(k, v)
		}
	} else {
		for _, name := range a.Fields {
			v, ok := snapshot[name]
			if !ok {
				return "", refusef("%s is not a field of a %s; fields: %s", name, a.CarrierType, strings.Join(snapshotNames(snapshot), ", "))
			}
			fields[name] = e.templateFieldValue(name, v)
		}
	}
	return application_context.MarshalToolJSON(map[string]any{
		"carrierType": a.CarrierType, "id": a.ID, "name": e.cfg.Privacy.Keep(carrierNameOf(carrier)), "fields": fields,
	}), nil
}

func snapshotNames(s models.TemplateSnapshot) []string {
	names := make([]string, 0, len(s))
	for k := range s {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

type toolCarrierEntry struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (e *templateToolExecutor) listCarriers(mc *application_context.MahresourcesContext, a application_context.ListCarriersArgs) (string, error) {
	rows, more, err := mc.ListTemplateRunCarriers(a.CarrierType, a.NameQuery, a.Offset, application_context.ToolListEntryCap)
	if err != nil {
		return "", err
	}
	entries := make([]toolCarrierEntry, len(rows))
	for i, r := range rows {
		e.cfg.Privacy.KeepScalar(int64(r.ID))
		entries[i] = toolCarrierEntry{r.ID, e.cfg.Privacy.Keep(r.Name), e.cfg.Privacy.Text(r.Description)}
	}
	return pagedJSON(e.cfg.Privacy, "entries", entries, more, a.Offset, len(entries)), nil
}

func (e *templateToolExecutor) listRevisions(mc *application_context.MahresourcesContext, a application_context.RevisionArgs) (string, error) {
	carrierType, id, err := e.revisionTarget(a)
	if err != nil {
		return "", err
	}
	revs, more, err := mc.ListTemplateRevisionsPage(carrierType, id, application_context.ToolListEntryCap, a.Offset)
	if err != nil {
		return "", err
	}
	if e.cfg.Privacy == nil {
		return pagedJSON(e.cfg.Privacy, "revisions", revs, more, a.Offset, len(revs)), nil
	}
	masked := make([]map[string]any, len(revs))
	for i, r := range revs {
		masked[i] = e.maskRevisionSummary(r)
	}
	e.cfg.Privacy.KeepScalar(int64(a.Offset + len(revs)))
	return pagedJSON(e.cfg.Privacy, "revisions", masked, more, a.Offset, len(revs)), nil
}

func (e *templateToolExecutor) getRevision(mc *application_context.MahresourcesContext, a application_context.RevisionArgs) (string, error) {
	carrierType, id, err := e.revisionTarget(a)
	if err != nil {
		return "", err
	}
	detail, err := mc.GetTemplateRevision(carrierType, id, a.Number)
	if err != nil {
		return "", err
	}
	if a.Field != "" {
		v, ok := detail.Editor[a.Field]
		if !ok {
			return "", refusef("%s is not a field of that revision", a.Field)
		}
		e.cfg.Privacy.KeepScalar(int64(detail.Number))
		e.cfg.Privacy.Keep(a.Field)
		return application_context.MarshalToolJSON(map[string]any{"number": detail.Number, "field": a.Field, "value": e.templateFieldValue(a.Field, v)}), nil
	}
	var label any = detail.Label
	var createdAt any = detail.CreatedAt
	if privacy := e.cfg.Privacy; privacy != nil {
		label, createdAt = privacy.Text(detail.Label), privacy.Date(detail.CreatedAt)
		privacy.KeepScalar(int64(detail.Number))
	}
	fieldValues := make(map[string]any, len(detail.Editor))
	for name, v := range detail.Editor {
		e.cfg.Privacy.Keep(name)
		fieldValues[name] = e.templateFieldValue(name, v)
	}
	e.cfg.Privacy.Keep(detail.Source)
	for _, slot := range detail.ChangedSlots {
		e.cfg.Privacy.Keep(slot)
	}
	return application_context.MarshalToolJSON(map[string]any{
		"number": detail.Number, "label": label, "source": detail.Source, "createdAt": createdAt,
		"changedSlots": detail.ChangedSlots, "fields": fieldValues,
	}), nil
}

// acquire takes a slot of a run-local semaphore, or gives up with ctx.
func acquire(ctx context.Context, sem chan struct{}) error {
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *templateToolExecutor) diffRevisions(ctx context.Context, mc *application_context.MahresourcesContext, a application_context.RevisionArgs) (string, error) {
	carrierType, id, err := e.revisionTarget(a)
	if err != nil {
		return "", err
	}
	if err := acquire(ctx, e.diffs); err != nil {
		return "", err
	}
	defer func() { <-e.diffs }()
	diff, err := diffTemplateRevisionsBounded(ctx, mc, carrierType, id, a.From, a.To)
	if err != nil {
		return "", err
	}
	e.vetDiff(diff)
	return application_context.MarshalToolJSON(diff), nil
}

// fieldNames is the set of template fields a lint or preview may name.
func (e *templateToolExecutor) knownField(field string) error {
	for _, f := range models.TemplateFields(carrierTypeFor(e.cfg.EntityType)) {
		if f.Name == field {
			return nil
		}
	}
	names := []string{}
	for _, f := range models.TemplateFields(carrierTypeFor(e.cfg.EntityType)) {
		names = append(names, f.Name)
	}
	return refusef("%s is not a template field; fields: %s", field, strings.Join(names, ", "))
}

// contentFor is the content a call means: its own, else the field's value in
// the Turn-start snapshot.
func (e *templateToolExecutor) contentFor(field string, given *string, snapshot map[string]string) (string, error) {
	if given != nil {
		return *given, nil
	}
	if v, ok := snapshot[field]; ok {
		return v, nil
	}
	if e.cfg.FieldValue != nil {
		if v, ok := e.cfg.FieldValue(field); ok {
			return v, nil
		}
	}
	return "", refusef("no content: pass content, or submit %s first", field)
}

type toolLintIssue struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Start    int    `json:"start,omitempty"`
	End      int    `json:"end,omitempty"`
}

func (e *templateToolExecutor) validation(mc *application_context.MahresourcesContext) application_context.TemplateRunValidation {
	return application_context.TemplateRunValidation{
		Known:                 e.cfg.Known,
		ValidateMRQL:          templateMRQLValidator(mc.MRQLStaticEnv()),
		DisplayTypes:          e.cfg.DisplayTypes,
		DisplayTypesTruncated: e.cfg.DisplayTypesTruncated,
		PartialExists: func(ctx context.Context, name string) (bool, error) {
			// Rebased per call, so a blocked lookup is cancelled with the Tool.
			p, err := mc.WithParentContext(ctx).GetTemplatePartialByName(name)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return err == nil && p != nil, err
		},
	}
}

func (e *templateToolExecutor) lint(ctx context.Context, mc *application_context.MahresourcesContext, a application_context.LintArgs, snapshot map[string]string) (string, error) {
	if err := e.knownField(a.Field); err != nil {
		return "", err
	}
	content, err := e.contentFor(a.Field, a.Content, snapshot)
	if err != nil {
		return "", err
	}
	// Over the limit is an error result, not a lint verdict.
	if err := application_context.CheckTemplateToolSource(content); err != nil {
		return "", refusef("%v", err)
	}
	var issues []shortcodes.LintIssue
	if e.cfg.Validate != nil {
		issues = e.cfg.Validate(ctx, e.cfg.Input, a.Field, content)
	} else {
		issues = application_context.ValidateTemplateRunContent(ctx, e.validation(mc), a.Field, content)
	}
	out := make([]toolLintIssue, len(issues))
	errs, warns := 0, 0
	privacy := e.cfg.Privacy
	privacy.Keep(a.Field)
	for i, is := range issues {
		// Lint messages quote the author's source and the shortcode catalogue.
		privacy.Keep(is.Message)
		privacy.Keep(is.Severity)
		privacy.KeepScalar(int64(is.Start))
		privacy.KeepScalar(int64(is.End))
		out[i] = toolLintIssue{Severity: is.Severity, Message: is.Message, Start: is.Start, End: is.End}
		switch is.Severity {
		case shortcodes.SeverityError:
			errs++
		case shortcodes.SeverityWarning:
			warns++
		}
	}
	privacy.KeepScalar(int64(errs))
	privacy.KeepScalar(int64(warns))
	return application_context.MarshalToolJSON(map[string]any{"field": a.Field, "errors": errs, "warnings": warns, "issues": out}), nil
}

func (e *templateToolExecutor) checkMRQL(mc *application_context.MahresourcesContext, a application_context.CheckMRQLArgs) (string, error) {
	// The same checker lint reaches through LintOptions.ValidateMRQL.
	err := templateMRQLValidator(mc.MRQLStaticEnv())(a.Query, shortcodes.MRQLCheckOptions{Params: a.Params, Limit: a.Limit, Buckets: a.Buckets})
	if err != nil {
		// The checker's text describes the model's own query.
		e.cfg.Privacy.Keep(err.Error())
		return application_context.MarshalToolJSON(map[string]any{"ok": false, "error": err.Error()}), nil
	}
	return application_context.MarshalToolJSON(map[string]any{"ok": true}), nil
}

func (e *templateToolExecutor) getPartial(mc *application_context.MahresourcesContext, a application_context.NameArgs) (string, error) {
	p, err := mc.GetTemplatePartialByName(a.Name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", refusef("no partial named %q", a.Name)
		}
		return "", err
	}
	// The source is the author's template and is readable; the description is an
	// Entity value and is masked.
	return application_context.MarshalToolJSON(map[string]any{"name": e.cfg.Privacy.Keep(p.Name), "description": e.cfg.Privacy.Text(p.Description), "source": e.cfg.Privacy.Keep(p.Content)}), nil
}

func (e *templateToolExecutor) getShortcodeDocs(mc *application_context.MahresourcesContext, a application_context.NameArgs) (string, error) {
	name := strings.Trim(strings.TrimSpace(a.Name), "[]")
	for _, d := range shortcodes.BuiltinDocs() {
		if d.Name == name {
			var b strings.Builder
			writeBuiltinShortcodeDoc(&b, d)
			return e.cfg.Privacy.Keep(b.String()), nil
		}
	}
	if pm := mc.PluginManager(); pm != nil {
		for _, sc := range pm.AuthoringSnapshot().Shortcodes {
			if sc.FullName == name {
				var b strings.Builder
				writePluginShortcodeDoc(&b, canonicalPluginShortcodeDoc(sc))
				return e.cfg.Privacy.Keep(b.String()), nil
			}
		}
	}
	return "", refusef("no built-in or plugin shortcode named %q", name)
}

func (e *templateToolExecutor) preview(ctx context.Context, mc *application_context.MahresourcesContext, a application_context.PreviewArgs, snapshot map[string]string) (string, error) {
	// Held across the entity load and the render: six parallel calls are two
	// previews of work at a time, not six.
	if err := acquire(ctx, e.previews); err != nil {
		return "", err
	}
	defer func() { <-e.previews }()
	if err := e.knownField(a.Field); err != nil {
		return "", err
	}
	content, err := e.contentFor(a.Field, a.Content, snapshot)
	if err != nil {
		return "", err
	}
	css := ""
	if a.CSS != nil {
		css = *a.CSS
	} else if !strings.HasSuffix(a.Field, "CSS") {
		if v, ok := snapshot[a.Field+"CSS"]; ok {
			css = v
		} else if e.cfg.FieldValue != nil {
			css, _ = e.cfg.FieldValue(a.Field + "CSS")
		}
	}
	for _, src := range []string{content, css} {
		if err := application_context.CheckTemplateToolSource(src); err != nil {
			return "", refusef("%v", err)
		}
	}
	// The sources about to be rendered are the author's template, whichever way
	// they were resolved (the call, a submission, the editor value). They are
	// registered as readable; the rendered output never is.
	e.cfg.Privacy.Keep(content)
	e.cfg.Privacy.Keep(css)

	carrierMode := strings.HasPrefix(a.Field, "CustomListHeader") || strings.HasPrefix(a.Field, "CustomListFooter")
	var entity any
	switch {
	case carrierMode:
		if e.cfg.CarrierID == 0 {
			return "", refusef("the carrier being edited is not saved yet, so a list header or footer has nothing to render against")
		}
		entity, err = loadPreviewCarrier(mc, e.cfg.EntityType, e.cfg.CarrierID)
	default:
		entityType, entityID := a.EntityType, a.EntityID
		if entityID == 0 {
			entityType, entityID = e.cfg.EntityType, e.cfg.PreviewEntityID
		}
		if entityID == 0 {
			return "", refusef("pass entityType and entityId to preview against")
		}
		entity, _, err = loadPreviewEntity(mc, entityType, entityID)
	}
	if err != nil {
		return "", err
	}
	// Under Privacy mode the preview renders against a masked copy of the entity
	// and everything loaded with it. The meta context is built from the copy, so
	// the presentation names it reads (parent and root group names) are masked
	// too; only IDs are resolved from the database (P14).
	privacy := e.cfg.Privacy
	if privacy != nil {
		masked, merr := privacy.Entity(ctx, entity)
		if merr != nil {
			if errors.Is(merr, application_context.ErrTemplatePrivacyLimit) {
				return "", refusef("too much entity data for Privacy mode")
			}
			return "", merr
		}
		entity = masked
	}
	metaCtx := template_filters.BuildMetaContextForEntity(entity, mc)
	if metaCtx == nil {
		return "", errors.New("could not build the preview context")
	}
	// The scope, parent and root IDs are resolved from the database, beyond the
	// owners the entity loaded, and [link to="root"] prints them: preserved scalars.
	privacy.KeepScalar(int64(metaCtx.EntityID))
	privacy.KeepScalar(int64(metaCtx.ScopeGroupID))
	privacy.KeepScalar(int64(metaCtx.ParentGroupID))
	privacy.KeepScalar(int64(metaCtx.RootGroupID))

	renderCtx, cancel := buildMRQLAPIRenderContext(ctx, mc, false)
	defer cancel()
	if e.cfg.Principal != nil {
		renderCtx = auth.WithPrincipal(renderCtx, e.cfg.Principal)
	}
	renderCtx = shortcodes.WithRenderBudget(renderCtx, shortcodes.RenderBudget{
		OutputBytes: templateRunRenderOutputBytes, ParsedSourceBytes: templateRunRenderParsedBytes,
	})
	if privacy != nil {
		// Partial source is the author's template, readable; it joins the corpus
		// before it is expanded. The rendered output is never registered.
		inner := template_filters.BuildPartialResolver(mc)
		renderCtx = shortcodes.WithPartialResolver(renderCtx, func(name string) (string, bool) {
			content, found := inner(name)
			if found {
				privacy.Keep(content)
			}
			return content, found
		})
		// Text made from a preserved number or the stand-in time is readable.
		renderCtx = shortcodes.WithScalarFormatObserver(renderCtx, func(s string) { privacy.Keep(s) })
	}

	var executor shortcodes.QueryExecutor = template_filters.UnavailableQueryExecutor()
	if e.cfg.OptIns.PreviewMRQL {
		executor = cappedQueryExecutor(template_filters.BuildModelPreviewQueryExecutor(mc), previewMRQLLimitCap)
		if privacy != nil {
			executor = privacyQueryExecutor(privacy, executor)
		}
	}
	var renderer shortcodes.PluginRenderer
	if e.cfg.OptIns.PreviewPlugins && privacy == nil {
		renderCtx = plugin_system.WithModelPreview(renderCtx)
		renderer = previewPluginRenderer(renderCtx, mc)
	}

	result := map[string]any{"field": a.Field}
	if strings.HasSuffix(a.Field, "CSS") {
		out, err := processTemplateToolSource(renderCtx, content, *metaCtx, renderer, executor)
		if err != nil {
			return "", err
		}
		result["css"] = out
	} else {
		html, err := processTemplateToolSource(renderCtx, content, *metaCtx, renderer, executor)
		if err != nil {
			return "", err
		}
		result["html"] = html
		if css != "" {
			cssOut, err := processTemplateToolSource(renderCtx, css, *metaCtx, renderer, executor)
			if err != nil {
				return "", err
			}
			result["css"] = cssOut
		}
	}
	if perr := privacy.Err(); perr != nil {
		return "", refusef("too much entity data for Privacy mode")
	}
	return application_context.MarshalToolJSON(result), nil
}

// The fixed texts a Privacy mode preview shows in place of a saved query and of
// a failed inline query: a saved query's text, scope and errors can carry stored
// values.
const (
	privacySavedQueryText  = "saved queries are not available in Privacy mode"
	privacyQueryFailedText = "the query failed; details are hidden in Privacy mode; use check_mrql to check its syntax"
)

// privacyQueryExecutor wraps a preview's [mrql] executor for Privacy mode
// (P15): saved queries are refused, an inline query's error becomes fixed text
// (ErrQueryUnavailable keeps its inert rendering), and every result is masked
// field by field into new containers.
func privacyQueryExecutor(privacy *application_context.TemplatePrivacy, inner shortcodes.QueryExecutor) shortcodes.QueryExecutor {
	// The fixed texts are rendered into the preview (HTML-escaped), so they are
	// registered when the executor is built.
	for _, text := range []string{privacySavedQueryText, privacyQueryFailedText} {
		privacy.Keep(text)
		privacy.Keep(html.EscapeString(text))
	}
	return func(reqCtx context.Context, query string, opts shortcodes.QueryOptions) (*shortcodes.QueryResult, error) {
		if opts.SavedName != "" {
			return nil, errors.New(privacySavedQueryText)
		}
		result, err := inner(reqCtx, query, opts)
		if err != nil {
			if errors.Is(err, shortcodes.ErrQueryUnavailable) {
				return nil, err
			}
			return nil, errors.New(privacyQueryFailedText)
		}
		masked, err := privacy.QueryResult(reqCtx, result)
		if err != nil {
			return nil, errors.New(privacyQueryFailedText)
		}
		return masked, nil
	}
}

// maskRevisionSummary is a revision's list entry with its Entity values masked:
// the label and the applied entries through Text, the instant through Date. The
// rest is IDs, numbers, slot names and the source enum.
func (e *templateToolExecutor) maskRevisionSummary(r contracts.TemplateRevisionSummary) map[string]any {
	privacy := e.cfg.Privacy
	raw, _ := json.Marshal(r)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	out["label"] = privacy.Text(r.Label)
	out["createdAt"] = privacy.Date(r.CreatedAt)
	applied := make([]string, len(r.Applied))
	for i, entry := range r.Applied {
		applied[i] = privacy.Text(entry)
	}
	out["applied"] = applied
	privacy.KeepScalar(int64(r.ID))
	privacy.KeepScalar(int64(r.Number))
	privacy.KeepScalar(int64(r.CarrierID))
	if r.CreatedByUserId != nil {
		privacy.KeepScalar(int64(*r.CreatedByUserId))
	}
	if r.RestoredFrom != nil {
		privacy.KeepScalar(int64(*r.RestoredFrom))
	}
	return out
}

// parseToolSchema decodes a MetaSchema for masking; nil when there is none.
func parseToolSchema(schema string) any {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	var root map[string]any
	if json.Unmarshal([]byte(schema), &root) != nil {
		return nil
	}
	return root
}

// templateFieldValue is one tracked template field as the model sees it. The
// slots, MetaSchema and SectionConfig are the author's vocabulary (P5) and are
// registered as readable, leaf by leaf. AutoDetectRules is not: its rules carry
// stored content types and file names, so its string leaves are masked like
// Meta. Nothing here registers a whole response.
func (e *templateToolExecutor) templateFieldValue(name string, v any) any {
	privacy := e.cfg.Privacy
	if privacy == nil {
		return v
	}
	if name == "AutoDetectRules" {
		if s, ok := v.(string); ok {
			return string(privacy.Meta([]byte(s), ""))
		}
		return privacy.Value(v, nil)
	}
	switch x := v.(type) {
	case string:
		privacy.Keep(x)
	case bool:
		privacy.KeepScalar(x)
	default:
		privacy.Keep(application_context.MarshalToolJSON(x))
	}
	return v
}

// vetDiff makes a revision diff safe to send: the diff of AutoDetectRules has
// its lines masked, every other slot's lines are template source and join the
// readable corpus, with the slot, field and operation names.
func (e *templateToolExecutor) vetDiff(diff *contracts.TemplateRevisionDiff) {
	privacy := e.cfg.Privacy
	if privacy == nil || diff == nil {
		return
	}
	privacy.KeepScalar(int64(diff.CarrierID))
	privacy.KeepScalar(int64(diff.FromNumber))
	if diff.ToNumber != nil {
		privacy.KeepScalar(int64(*diff.ToNumber))
	}
	for si := range diff.Slots {
		privacy.Keep(diff.Slots[si].Slot)
		for fi := range diff.Slots[si].Fields {
			field := &diff.Slots[si].Fields[fi]
			privacy.Keep(field.Field)
			for hi := range field.Hunks {
				hunk := &field.Hunks[hi]
				privacy.Keep(hunk.Op)
				for li, line := range hunk.Lines {
					if field.Field == "AutoDetectRules" {
						hunk.Lines[li] = privacy.Text(line)
					} else {
						privacy.Keep(line)
					}
				}
			}
		}
	}
}
