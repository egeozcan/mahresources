package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdfs "io/fs"
	"mime"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/jobs"
	"mahresources/models"
)

var (
	ErrJobOutputUnavailable = errors.New("job output is no longer available")
	ErrJobOutputForbidden   = errors.New("job output is not available to this principal")
	ErrJobOutputInvalid     = errors.New("job output reference is invalid")
)

// JobOutputOpenRequest is the reauthorized, stored output passed to a Kind's
// optional reader. The caller never supplies a reference or run id: both come
// from the canonical Job row after current Job visibility and availability are
// checked. A Kind reader must recheck any stricter role/scope policy and verify
// source identities against Snapshot.ID before returning content. A run-backed
// command-history reader, for example, must verify the stored run's JobID is
// exactly Snapshot.ID.
type JobOutputOpenRequest struct {
	Snapshot  jobs.Snapshot
	Output    jobs.Output
	Principal *auth.Principal
}

// JobOutputOpener is an optional Kind adapter capability for outputs that need
// specialized access, such as a command-history reference resolved by runId.
// Implementations receive only the stored reference and visible canonical Job;
// they must not accept caller-provided IDs, must verify source Job identity, and
// must return a sanitized bounded representation.
type JobOutputOpener interface {
	OpenJobOutput(context.Context, JobOutputOpenRequest) (contracts.JobOutputContent, error)
}

// JobOutputAuthorizer is the Kind-specific half of the current-principal output
// policy. It is evaluated both when detail advertises an output and immediately
// before the stored reference is opened. A Kind that supplies an opener must
// also supply this policy; otherwise its outputs are hidden and refused.
type JobOutputAuthorizer interface {
	AuthorizeJobOutput(context.Context, JobOutputOpenRequest) error
}

// GetOpenableJobOutputs returns only outputs the current principal may open.
// Availability remains in the response so the UI can show expired outputs, but
// hidden Kind-specific outputs are indistinguishable from absent outputs.
//
// An output that names an entity follows openableJobOutput: it is offered only
// while this principal can open that entity.
func (ctx *MahresourcesContext) GetOpenableJobOutputs(jobID string) ([]jobs.Output, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, err
	}
	snapshot, err := service.Get(ctx.jobDeps(), ctx.jobAccess(), jobID)
	if err != nil {
		return nil, err
	}
	outputs, err := service.Outputs(ctx.jobDeps(), ctx.jobAccess(), jobID)
	if err != nil {
		return nil, err
	}
	items := make([]jobOutputOf, 0, len(outputs))
	for _, output := range outputs {
		items = append(items, jobOutputOf{Snapshot: snapshot, Output: output})
	}
	offered, shown, err := ctx.offeredJobOutputs(service, items)
	if err != nil {
		return nil, err
	}
	openable := make([]jobs.Output, 0, len(offered))
	for i := range offered {
		if shown[i] {
			openable = append(openable, offered[i])
		}
	}
	return openable, nil
}

// offeredJobOutputs is what this principal is offered of some Jobs' outputs:
// each output the output policy lets it open (authorizeJobOutput), as
// openableJobOutputs projects it, in order, with whether it is shown. The Job
// API's listing and the legacy plugin-action reads both answer through it, so
// they cannot offer different things.
func (ctx *MahresourcesContext) offeredJobOutputs(service *jobs.Service, items []jobOutputOf) ([]jobs.Output, []bool, error) {
	principal := ctx.Principal()
	authorized := make([]jobOutputOf, 0, len(items))
	positions := make([]int, 0, len(items))
	for i, item := range items {
		request := JobOutputOpenRequest{Snapshot: item.Snapshot, Output: item.Output, Principal: principal}
		if err := ctx.authorizeJobOutput(context.Background(), service, request); err != nil {
			if errors.Is(err, ErrJobOutputForbidden) {
				continue
			}
			return nil, nil, err
		}
		authorized = append(authorized, item)
		positions = append(positions, i)
	}
	projected, projectedShown, err := ctx.openableJobOutputs(authorized)
	if err != nil {
		return nil, nil, err
	}
	offered := make([]jobs.Output, len(items))
	shown := make([]bool, len(items))
	for i, item := range items {
		offered[i] = item.Output
	}
	for j, i := range positions {
		offered[i], shown[i] = projected[j], projectedShown[j]
	}
	return offered, shown, nil
}

// OpenJobOutput reauthorizes the canonical Job and its output on every open.
// Job visibility, current write capability, the output's own availability, and
// Kind-specific policy are separate checks; a bookmark to an old output cannot
// act as a bearer capability.
func (ctx *MahresourcesContext) openJobOutput(requestCtx context.Context, jobID, key string) (contracts.JobOutputContent, error) {
	if requestCtx == nil {
		requestCtx = context.Background()
	}
	service, err := ctx.requireJobService()
	if err != nil {
		return contracts.JobOutputContent{}, err
	}
	snapshot, err := service.Get(ctx.jobDeps(), ctx.jobAccess(), jobID)
	if err != nil {
		return contracts.JobOutputContent{}, err
	}
	principal := ctx.Principal()
	outputs, err := service.Outputs(ctx.jobDeps(), ctx.jobAccess(), jobID)
	if err != nil {
		return contracts.JobOutputContent{}, err
	}
	output, found := findJobOutput(outputs, key)
	if !found {
		return contracts.JobOutputContent{}, jobs.ErrNotFound
	}
	request := JobOutputOpenRequest{Snapshot: snapshot, Output: output, Principal: principal}
	if err := ctx.authorizeJobOutput(requestCtx, service, request); err != nil {
		return contracts.JobOutputContent{}, err
	}
	if output.Availability != jobs.OutputAvailable || (output.ExpiresAt != nil && !output.ExpiresAt.After(time.Now().UTC())) {
		return contracts.JobOutputContent{}, ErrJobOutputUnavailable
	}
	// The same projection the listing applies, so opening an output by its URL
	// shows nothing the listing withheld: an entity output whose entity this
	// principal cannot open is not found, and a result's redirect to such an
	// entity is not in what is returned.
	offered, reachable, err := ctx.openableJobOutput(snapshot.Kind, output)
	if err != nil {
		return contracts.JobOutputContent{}, err
	}
	if !reachable {
		return contracts.JobOutputContent{}, jobs.ErrNotFound
	}
	output = offered
	request.Output = offered

	if adapter, ok := service.AdapterFor(snapshot.Kind, snapshot.KindVersion); ok {
		if opener, ok := adapter.(JobOutputOpener); ok {
			content, err := opener.OpenJobOutput(requestCtx, request)
			if err != nil {
				return contracts.JobOutputContent{}, err
			}
			return content, nil
		}
	}

	return ctx.openStandardJobOutput(output)
}

func (ctx *MahresourcesContext) authorizeJobOutput(requestCtx context.Context, service *jobs.Service, request JobOutputOpenRequest) error {
	principal := request.Principal
	if principal == nil || (!principal.IsAdmin() && !principal.CanWrite()) {
		return ErrJobOutputForbidden
	}
	adapter, registered := service.AdapterFor(request.Snapshot.Kind, request.Snapshot.KindVersion)
	if !registered {
		// An output may need kind-specific authorization before the standard
		// opener reads its persisted reference. If the adapter is missing (for
		// example, after upgrading beyond a stored kind version), there is no
		// policy available to establish that the output remains safe to expose.
		return ErrJobOutputForbidden
	}
	authorizer, hasPolicy := adapter.(JobOutputAuthorizer)
	_, hasOpener := adapter.(JobOutputOpener)
	if hasOpener && !hasPolicy {
		return ErrJobOutputForbidden
	}
	if hasPolicy {
		if requestCtx == nil {
			requestCtx = context.Background()
		}
		return authorizer.AuthorizeJobOutput(requestCtx, request)
	}
	return nil
}

func (ctx *MahresourcesContext) openStandardJobOutput(output jobs.Output) (contracts.JobOutputContent, error) {
	switch output.Type {
	case jobs.OutputTypeSummary:
		if !json.Valid(output.Reference) {
			return contracts.JobOutputContent{}, ErrJobOutputInvalid
		}
		return contracts.JobOutputContent{Data: append(json.RawMessage(nil), output.Reference...), ContentType: "application/json"}, nil
	case jobs.OutputTypeEntity:
		location, err := ctx.resolveJobEntityOutput(output.Reference)
		if err != nil {
			return contracts.JobOutputContent{}, err
		}
		return contracts.JobOutputContent{Location: location}, nil
	case jobs.OutputTypeExternalLink:
		location, err := safeJobExternalLink(output.Reference)
		if err != nil {
			return contracts.JobOutputContent{}, err
		}
		return contracts.JobOutputContent{Location: location}, nil
	case jobs.OutputTypeArtifact, jobs.OutputTypeReport, jobs.OutputTypeLog:
		return ctx.openJobFileOutput(output)
	default:
		return contracts.JobOutputContent{}, ErrJobOutputInvalid
	}
}

// resolveJobEntityOutput resolves an entity output's reference to the page it
// opens, through this context's principal-bound handle. An entity that was
// deleted or that the principal cannot see answers jobs.ErrNotFound; a read that
// failed answers its own error, because it proves neither.
func (ctx *MahresourcesContext) resolveJobEntityOutput(reference json.RawMessage) (string, error) {
	page, id, err := jobEntityTarget(reference)
	if err != nil {
		return "", err
	}
	reachable, err := ctx.jobEntitiesReachable(page, []uint{id})
	if err != nil {
		return "", err
	}
	if !reachable[id] {
		return "", jobs.ErrNotFound
	}
	return fmt.Sprintf("%s?id=%d", page, id), nil
}

// jobEntityTarget reads the one entity an entity output's reference names, as
// the page that shows it and its id. Anything else is ErrJobOutputInvalid.
func jobEntityTarget(reference json.RawMessage) (string, uint, error) {
	var ref struct {
		ResourceID  uint `json:"resourceId"`
		GroupID     uint `json:"groupId"`
		NoteID      uint `json:"noteId"`
		ReductionID uint `json:"reductionId"`
	}
	if err := json.Unmarshal(reference, &ref); err != nil {
		return "", 0, ErrJobOutputInvalid
	}
	switch {
	case ref.ResourceID > 0 && ref.GroupID == 0 && ref.NoteID == 0 && ref.ReductionID == 0:
		return "/resource", ref.ResourceID, nil
	case ref.GroupID > 0 && ref.ResourceID == 0 && ref.NoteID == 0 && ref.ReductionID == 0:
		return "/group", ref.GroupID, nil
	case ref.NoteID > 0 && ref.ResourceID == 0 && ref.GroupID == 0 && ref.ReductionID == 0:
		return "/note", ref.NoteID, nil
	case ref.ReductionID > 0 && ref.ResourceID == 0 && ref.GroupID == 0 && ref.NoteID == 0:
		return "/reduction", ref.ReductionID, nil
	default:
		return "", 0, ErrJobOutputInvalid
	}
}

// jobEntityReachChunk bounds how many ids one reachability read binds, beside
// whatever the scope predicate binds.
const jobEntityReachChunk = 200

// jobEntitiesReachable answers which of ids, all entities of the one page named,
// this context's principal can open. The read runs the query callbacks, so the
// subtree predicate of a group-limited principal applies exactly as it does to
// the entity's own page. A read that failed answers its error, because it proves
// neither answer.
func (ctx *MahresourcesContext) jobEntitiesReachable(page string, ids []uint) (map[uint]bool, error) {
	reachable := make(map[uint]bool, len(ids))
	var model any
	switch page {
	case "/resource":
		model = &models.Resource{}
	case "/group":
		model = &models.Group{}
	case "/note":
		model = &models.Note{}
	case "/reduction":
		ownerID, restricted := reductionOwnerFilter(ctx.Principal())
		for _, id := range ids {
			if _, err := ctx.GetResourceReduction(id, ownerID, restricted); err != nil {
				if errors.Is(err, ErrReductionNotFound) {
					continue
				}
				return nil, fmt.Errorf("read job output reduction: %w", err)
			}
			reachable[id] = true
		}
		return reachable, nil
	default:
		return reachable, nil
	}
	for start := 0; start < len(ids); start += jobEntityReachChunk {
		end := min(start+jobEntityReachChunk, len(ids))
		var found []uint
		if err := ctx.db.Model(model).Where("id IN ?", ids[start:end]).Pluck("id", &found).Error; err != nil {
			return nil, fmt.Errorf("read job output entities: %w", err)
		}
		for _, id := range found {
			reachable[id] = true
		}
	}
	return reachable, nil
}

func safeJobExternalLink(reference json.RawMessage) (string, error) {
	var ref struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(reference, &ref); err != nil {
		return "", ErrJobOutputInvalid
	}
	parsed, err := url.Parse(ref.URL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		return "", ErrJobOutputInvalid
	}
	return parsed.String(), nil
}

func (ctx *MahresourcesContext) openJobFileOutput(output jobs.Output) (contracts.JobOutputContent, error) {
	var reference struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(output.Reference, &reference); err != nil || reference.Path == "" {
		return contracts.JobOutputContent{}, ErrJobOutputInvalid
	}
	path, err := rootedJobOutputPath(reference.Path)
	if err != nil {
		return contracts.JobOutputContent{}, ErrJobOutputInvalid
	}
	fileSystem := ctx.GetDefaultFs()
	file, err := fileSystem.Open(path)
	if err != nil {
		if errors.Is(err, stdfs.ErrNotExist) {
			return contracts.JobOutputContent{}, ErrJobOutputUnavailable
		}
		return contracts.JobOutputContent{}, fmt.Errorf("open job output: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return contracts.JobOutputContent{}, fmt.Errorf("inspect job output: %w", err)
	}
	if info.IsDir() {
		_ = file.Close()
		return contracts.JobOutputContent{}, ErrJobOutputInvalid
	}
	contentType := "application/octet-stream"
	inline := output.Type == jobs.OutputTypeReport || output.Type == jobs.OutputTypeLog
	if detected := mime.TypeByExtension(filepath.Ext(path)); detected != "" {
		contentType = detected
	}
	if output.Type == jobs.OutputTypeReport && strings.HasSuffix(strings.ToLower(path), ".json") {
		contentType = "application/json"
	}
	return contracts.JobOutputContent{
		Body: file, ContentType: contentType,
		Filename: jobOutputFilename(output, path), Inline: inline,
	}, nil
}

func rootedJobOutputPath(raw string) (string, error) {
	if strings.ContainsRune(raw, '\x00') || strings.Contains(raw, "\\") {
		return "", ErrJobOutputInvalid
	}
	clean := filepath.Clean(filepath.FromSlash(raw))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrJobOutputInvalid
	}
	return clean, nil
}

// jobOutputFilename is the name a file output is saved under: its label, when
// it was published, and the stored file's own extension, as
// "exported-archive-20260926-124207.tar.gz". The label says what it is but not
// which one, so two exports would otherwise save under one name, and without the
// extension the saved file opens with nothing. The label is reduced to letters
// and digits joined by hyphens, which also keeps any path or control character
// out of the name. The time is UTC, as the legacy export download names it.
func jobOutputFilename(output jobs.Output, rawPath string) string {
	base := filepath.Base(filepath.ToSlash(rawPath))
	if base == "." || base == "/" {
		base = ""
	}
	extension := jobOutputExtension(base)
	label := strings.TrimSpace(output.Label)
	if extension != "" && strings.HasSuffix(strings.ToLower(label), extension) {
		label = label[:len(label)-len(extension)]
	}
	stem := jobOutputFilenameStem(label)
	if stem == "" {
		stem = jobOutputFilenameStem(strings.TrimSuffix(base, extension))
	}
	if stem == "" {
		stem = "job-output"
	}
	if !output.CreatedAt.IsZero() {
		stem += "-" + output.CreatedAt.UTC().Format("20060102-150405")
	}
	return stem + extension
}

// jobOutputExtension is a stored file name's extension, lowercased, keeping a
// compressed tar's two parts together (".tar.gz").
func jobOutputExtension(name string) string {
	extension := strings.ToLower(filepath.Ext(name))
	if extension == "" || extension == name {
		return ""
	}
	if inner := strings.ToLower(filepath.Ext(strings.TrimSuffix(name, filepath.Ext(name)))); inner == ".tar" {
		return inner + extension
	}
	return extension
}

// jobOutputFilenameStem lowercases a name and joins its runs of letters and
// digits with single hyphens.
func jobOutputFilenameStem(name string) string {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(words, "-")
}
