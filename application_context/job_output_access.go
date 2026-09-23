package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdfs "io/fs"
	"mime"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"mahresources/auth"
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

// JobOutputContent is one typed resource returned by an output reader. Exactly
// one of Body, Data or Location is generally populated.
type JobOutputContent struct {
	Output      jobs.Output
	Body        io.ReadCloser
	Data        json.RawMessage
	Location    string
	ContentType string
	Filename    string
	Inline      bool
}

// JobOutputOpener is an optional Kind adapter capability for outputs that need
// specialized access, such as a command-history reference resolved by runId.
// Implementations receive only the stored reference and visible canonical Job;
// they must not accept caller-provided IDs, must verify source Job identity, and
// must return a sanitized bounded representation.
type JobOutputOpener interface {
	OpenJobOutput(context.Context, JobOutputOpenRequest) (JobOutputContent, error)
}

// OpenJobOutput reauthorizes the canonical Job and its output on every open.
// Job visibility, current write capability, the output's own availability, and
// Kind-specific policy are separate checks; a bookmark to an old output cannot
// act as a bearer capability.
func (ctx *MahresourcesContext) openJobOutput(requestCtx context.Context, jobID, key string) (JobOutputContent, error) {
	if requestCtx == nil {
		requestCtx = context.Background()
	}
	service, err := ctx.requireJobService()
	if err != nil {
		return JobOutputContent{}, err
	}
	snapshot, err := service.Get(ctx.jobDeps(), ctx.jobAccess(), jobID)
	if err != nil {
		return JobOutputContent{}, err
	}
	principal := ctx.Principal()
	if principal == nil || (!principal.IsAdmin() && !principal.CanWrite()) {
		return JobOutputContent{}, ErrJobOutputForbidden
	}
	outputs, err := service.Outputs(ctx.jobDeps(), ctx.jobAccess(), jobID)
	if err != nil {
		return JobOutputContent{}, err
	}
	output, found := findJobOutput(outputs, key)
	if !found {
		return JobOutputContent{}, jobs.ErrNotFound
	}
	if output.Availability != jobs.OutputAvailable || (output.ExpiresAt != nil && !output.ExpiresAt.After(time.Now().UTC())) {
		return JobOutputContent{}, ErrJobOutputUnavailable
	}

	request := JobOutputOpenRequest{Snapshot: snapshot, Output: output, Principal: principal}
	if adapter, ok := service.AdapterFor(snapshot.Kind, snapshot.KindVersion); ok {
		if opener, ok := adapter.(JobOutputOpener); ok {
			content, err := opener.OpenJobOutput(requestCtx, request)
			if err != nil {
				return JobOutputContent{}, err
			}
			content.Output = output
			return content, nil
		}
	}

	return ctx.openStandardJobOutput(output)
}

func (ctx *MahresourcesContext) openStandardJobOutput(output jobs.Output) (JobOutputContent, error) {
	switch output.Type {
	case jobs.OutputTypeSummary:
		if !json.Valid(output.Reference) {
			return JobOutputContent{}, ErrJobOutputInvalid
		}
		return JobOutputContent{Output: output, Data: append(json.RawMessage(nil), output.Reference...), ContentType: "application/json"}, nil
	case jobs.OutputTypeEntity:
		location, err := ctx.resolveJobEntityOutput(output.Reference)
		if err != nil {
			return JobOutputContent{}, err
		}
		return JobOutputContent{Output: output, Location: location}, nil
	case jobs.OutputTypeExternalLink:
		location, err := safeJobExternalLink(output.Reference)
		if err != nil {
			return JobOutputContent{}, err
		}
		return JobOutputContent{Output: output, Location: location}, nil
	case jobs.OutputTypeArtifact, jobs.OutputTypeReport, jobs.OutputTypeLog:
		return ctx.openJobFileOutput(output)
	default:
		return JobOutputContent{}, ErrJobOutputInvalid
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

func (ctx *MahresourcesContext) openJobFileOutput(output jobs.Output) (JobOutputContent, error) {
	var reference struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(output.Reference, &reference); err != nil || reference.Path == "" {
		return JobOutputContent{}, ErrJobOutputInvalid
	}
	path, err := rootedJobOutputPath(reference.Path)
	if err != nil {
		return JobOutputContent{}, ErrJobOutputInvalid
	}
	fileSystem := ctx.GetDefaultFs()
	file, err := fileSystem.Open(path)
	if err != nil {
		if errors.Is(err, stdfs.ErrNotExist) {
			return JobOutputContent{}, ErrJobOutputUnavailable
		}
		return JobOutputContent{}, fmt.Errorf("open job output: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return JobOutputContent{}, fmt.Errorf("inspect job output: %w", err)
	}
	if info.IsDir() {
		_ = file.Close()
		return JobOutputContent{}, ErrJobOutputInvalid
	}
	contentType := "application/octet-stream"
	inline := output.Type == jobs.OutputTypeReport || output.Type == jobs.OutputTypeLog
	if detected := mime.TypeByExtension(filepath.Ext(path)); detected != "" {
		contentType = detected
	}
	if output.Type == jobs.OutputTypeReport && strings.HasSuffix(strings.ToLower(path), ".json") {
		contentType = "application/json"
	}
	return JobOutputContent{
		Output: output, Body: file, ContentType: contentType,
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
