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

	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/jobs"
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
	principal := ctx.Principal()
	openable := make([]jobs.Output, 0, len(outputs))
	for _, output := range outputs {
		request := JobOutputOpenRequest{Snapshot: snapshot, Output: output, Principal: principal}
		if err := ctx.authorizeJobOutput(context.Background(), service, request); err != nil {
			if errors.Is(err, ErrJobOutputForbidden) {
				continue
			}
			return nil, err
		}
		openable = append(openable, output)
	}
	return openable, nil
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

func (ctx *MahresourcesContext) resolveJobEntityOutput(reference json.RawMessage) (string, error) {
	var ref struct {
		ResourceID  uint `json:"resourceId"`
		GroupID     uint `json:"groupId"`
		NoteID      uint `json:"noteId"`
		ReductionID uint `json:"reductionId"`
	}
	if err := json.Unmarshal(reference, &ref); err != nil {
		return "", ErrJobOutputInvalid
	}
	switch {
	case ref.ResourceID > 0 && ref.GroupID == 0 && ref.NoteID == 0 && ref.ReductionID == 0:
		if _, err := ctx.GetResourceByID(ref.ResourceID); err != nil {
			return "", jobs.ErrNotFound
		}
		return fmt.Sprintf("/resource?id=%d", ref.ResourceID), nil
	case ref.GroupID > 0 && ref.ResourceID == 0 && ref.NoteID == 0 && ref.ReductionID == 0:
		if _, err := ctx.GetGroup(ref.GroupID); err != nil {
			return "", jobs.ErrNotFound
		}
		return fmt.Sprintf("/group?id=%d", ref.GroupID), nil
	case ref.NoteID > 0 && ref.ResourceID == 0 && ref.GroupID == 0 && ref.ReductionID == 0:
		if _, err := ctx.GetNote(ref.NoteID); err != nil {
			return "", jobs.ErrNotFound
		}
		return fmt.Sprintf("/note?id=%d", ref.NoteID), nil
	case ref.ReductionID > 0 && ref.ResourceID == 0 && ref.GroupID == 0 && ref.NoteID == 0:
		ownerID, restricted := reductionOwnerFilter(ctx.Principal())
		if _, err := ctx.GetResourceReduction(ref.ReductionID, ownerID, restricted); err != nil {
			return "", jobs.ErrNotFound
		}
		return fmt.Sprintf("/reduction?id=%d", ref.ReductionID), nil
	default:
		return "", ErrJobOutputInvalid
	}
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
		Filename: safeJobOutputFilename(output.Label, path), Inline: inline,
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

func safeJobOutputFilename(label, rawPath string) string {
	name := strings.TrimSpace(label)
	if name == "" {
		name = filepath.Base(rawPath)
	}
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "job-output"
	}
	return name
}
