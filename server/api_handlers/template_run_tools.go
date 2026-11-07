package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// The Tool executor of a harness run (plan section 2): the twelve read Tools,
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
	// Known, DisplayTypes and DisplayTypesTruncated are the request's plugin
	// snapshot; zero values are read from Start.
	Known                 shortcodes.KnownShortcodes
	DisplayTypes          []string
	DisplayTypesTruncated bool
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
		body, err = e.lint(ctx, mc, a)
	case application_context.CheckMRQLArgs:
		body, err = e.checkMRQL(mc, a)
	case application_context.PreviewArgs:
		body, err = e.preview(ctx, mc, a)
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
	return application_context.ShapeToolResult("tool "+call.Name, body, application_context.ToolResultCap(call.Name)), nil
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
	sample := application_context.ProjectTemplateSample(entity, true)
	if meta := fullSampleMeta(entity); meta != nil {
		sample["Meta"] = meta
	}
	result := map[string]any{"type": a.Type, "id": a.ID, "entity": sample}
	if carrierID != 0 {
		if carrier, cerr := loadPreviewCarrier(mc, a.Type, carrierID); cerr == nil {
			result["carrier"] = toolCarrierRef{ID: carrierID, Name: carrierNameOf(carrier)}
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

func toolEntries(in []application_context.TemplateRelatedEntry) []toolEntry {
	out := make([]toolEntry, len(in))
	for i, e := range in {
		out[i] = toolEntry{Type: e.Type, ID: e.ID, Name: e.Name}
		if e.CarrierID != 0 {
			out[i].Carrier = &toolCarrierRef{ID: e.CarrierID, Name: e.CarrierName}
		}
	}
	return out
}

func pagedJSON(key string, entries any, more bool, offset, n int) string {
	result := map[string]any{key: entries, "more": more}
	if more {
		result["nextOffset"] = offset + n
	}
	return application_context.MarshalToolJSON(result)
}

func (e *templateToolExecutor) listRelated(mc *application_context.MahresourcesContext, a application_context.ListRelatedArgs) (string, error) {
	entries, more, err := mc.ListTemplateRunRelated(a.Type, a.ID, a.Direction, a.Kind, a.Offset, application_context.ToolListEntryCap)
	if err != nil {
		return "", err
	}
	return pagedJSON("entries", toolEntries(entries), more, a.Offset, len(entries)), nil
}

func (e *templateToolExecutor) listMembers(mc *application_context.MahresourcesContext, a application_context.ListMembersArgs) (string, error) {
	entries, more, err := mc.ListTemplateRunMembers(a.CarrierType, a.ID, a.Offset, application_context.ToolListEntryCap)
	if err != nil {
		return "", err
	}
	out := toolEntries(entries)
	for i := range out {
		out[i].Carrier = nil // the carrier is the one asked about
	}
	return pagedJSON("entries", out, more, a.Offset, len(entries)), nil
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
			fields[k] = v
		}
	} else {
		for _, name := range a.Fields {
			v, ok := snapshot[name]
			if !ok {
				return "", refusef("%s is not a field of a %s; fields: %s", name, a.CarrierType, strings.Join(snapshotNames(snapshot), ", "))
			}
			fields[name] = v
		}
	}
	return application_context.MarshalToolJSON(map[string]any{
		"carrierType": a.CarrierType, "id": a.ID, "name": carrierNameOf(carrier), "fields": fields,
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
	limit := application_context.ToolListEntryCap + 1
	var entries []toolCarrierEntry
	switch a.CarrierType {
	case models.TemplateCarrierCategory:
		rows, err := mc.GetCategories(a.Offset, limit, &query_models.CategoryQuery{Name: a.NameQuery, SortBy: []string{"id asc"}})
		if err != nil {
			return "", err
		}
		for _, r := range rows {
			entries = append(entries, toolCarrierEntry{r.ID, r.Name, r.Description})
		}
	case models.TemplateCarrierResourceCategory:
		rows, err := mc.GetResourceCategories(a.Offset, limit, &query_models.ResourceCategoryQuery{Name: a.NameQuery})
		if err != nil {
			return "", err
		}
		for _, r := range rows {
			entries = append(entries, toolCarrierEntry{r.ID, r.Name, r.Description})
		}
	default:
		rows, err := mc.GetNoteTypes(&query_models.NoteTypeQuery{Name: a.NameQuery}, a.Offset, limit)
		if err != nil {
			return "", err
		}
		for _, r := range rows {
			entries = append(entries, toolCarrierEntry{r.ID, r.Name, r.Description})
		}
	}
	more := len(entries) > application_context.ToolListEntryCap
	if more {
		entries = entries[:application_context.ToolListEntryCap]
	}
	if entries == nil {
		entries = []toolCarrierEntry{}
	}
	return pagedJSON("entries", entries, more, a.Offset, len(entries)), nil
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
	return pagedJSON("revisions", revs, more, a.Offset, len(revs)), nil
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
		return application_context.MarshalToolJSON(map[string]any{"number": detail.Number, "field": a.Field, "value": v}), nil
	}
	return application_context.MarshalToolJSON(map[string]any{
		"number": detail.Number, "label": detail.Label, "source": detail.Source, "createdAt": detail.CreatedAt,
		"changedSlots": detail.ChangedSlots, "fields": detail.Editor,
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
func (e *templateToolExecutor) contentFor(field string, given *string) (string, error) {
	if given != nil {
		return *given, nil
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

func (e *templateToolExecutor) lint(ctx context.Context, mc *application_context.MahresourcesContext, a application_context.LintArgs) (string, error) {
	if err := e.knownField(a.Field); err != nil {
		return "", err
	}
	content, err := e.contentFor(a.Field, a.Content)
	if err != nil {
		return "", err
	}
	issues := application_context.ValidateTemplateRunContent(ctx, e.validation(mc), a.Field, content)
	out := make([]toolLintIssue, len(issues))
	errs, warns := 0, 0
	for i, is := range issues {
		out[i] = toolLintIssue{Severity: is.Severity, Message: is.Message, Start: is.Start, End: is.End}
		switch is.Severity {
		case shortcodes.SeverityError:
			errs++
		case shortcodes.SeverityWarning:
			warns++
		}
	}
	return application_context.MarshalToolJSON(map[string]any{"field": a.Field, "errors": errs, "warnings": warns, "issues": out}), nil
}

func (e *templateToolExecutor) checkMRQL(mc *application_context.MahresourcesContext, a application_context.CheckMRQLArgs) (string, error) {
	// The same checker lint reaches through LintOptions.ValidateMRQL.
	err := templateMRQLValidator(mc.MRQLStaticEnv())(a.Query, shortcodes.MRQLCheckOptions{Params: a.Params, Limit: a.Limit, Buckets: a.Buckets})
	if err != nil {
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
	return application_context.MarshalToolJSON(map[string]any{"name": p.Name, "description": p.Description, "source": p.Content}), nil
}

func (e *templateToolExecutor) getShortcodeDocs(mc *application_context.MahresourcesContext, a application_context.NameArgs) (string, error) {
	name := strings.Trim(strings.TrimSpace(a.Name), "[]")
	for _, d := range shortcodes.BuiltinDocs() {
		if d.Name == name {
			var b strings.Builder
			writeBuiltinShortcodeDoc(&b, d)
			return b.String(), nil
		}
	}
	if pm := mc.PluginManager(); pm != nil {
		for _, sc := range pm.AuthoringSnapshot().Shortcodes {
			if sc.FullName == name {
				var b strings.Builder
				writePluginShortcodeDoc(&b, canonicalPluginShortcodeDoc(sc))
				return b.String(), nil
			}
		}
	}
	return "", refusef("no built-in or plugin shortcode named %q", name)
}

func (e *templateToolExecutor) preview(ctx context.Context, mc *application_context.MahresourcesContext, a application_context.PreviewArgs) (string, error) {
	// Held across the entity load and the render: six parallel calls are two
	// previews of work at a time, not six.
	if err := acquire(ctx, e.previews); err != nil {
		return "", err
	}
	defer func() { <-e.previews }()
	if err := e.knownField(a.Field); err != nil {
		return "", err
	}
	content, err := e.contentFor(a.Field, a.Content)
	if err != nil {
		return "", err
	}
	css := ""
	if a.CSS != nil {
		css = *a.CSS
	} else if !strings.HasSuffix(a.Field, "CSS") && e.cfg.FieldValue != nil {
		css, _ = e.cfg.FieldValue(a.Field + "CSS")
	}
	for _, src := range []string{content, css} {
		if err := application_context.CheckTemplateToolSource(src); err != nil {
			return "", refusef("%v", err)
		}
	}

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
	metaCtx := template_filters.BuildMetaContextForEntity(entity, mc)
	if metaCtx == nil {
		return "", errors.New("could not build the preview context")
	}

	renderCtx, cancel := buildMRQLAPIRenderContext(ctx, mc, false)
	defer cancel()
	if e.cfg.Principal != nil {
		renderCtx = auth.WithPrincipal(renderCtx, e.cfg.Principal)
	}
	renderCtx = shortcodes.WithRenderBudget(renderCtx, shortcodes.RenderBudget{
		OutputBytes: templateRunRenderOutputBytes, ParsedSourceBytes: templateRunRenderParsedBytes,
	})

	var executor shortcodes.QueryExecutor = template_filters.UnavailableQueryExecutor()
	if e.cfg.OptIns.PreviewMRQL {
		executor = cappedQueryExecutor(template_filters.BuildModelPreviewQueryExecutor(mc), previewMRQLLimitCap)
	}
	var renderer shortcodes.PluginRenderer
	if e.cfg.OptIns.PreviewPlugins {
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
	return application_context.MarshalToolJSON(result), nil
}
