package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_commands"
	"mahresources/plugin_system"
)

var _ plugin_commands.Importer = (*MahresourcesContext)(nil)

// ValidateImport re-establishes every permission at worker start. A durable
// claim records what was requested; it is not a standing grant after a plugin,
// role, user, generation, or subtree changes.
func (ctx *MahresourcesContext) ValidateImport(validation plugin_commands.ImportValidation) error {
	var actorID uint
	if validation.ActorUserID == nil || *validation.ActorUserID == 0 {
		if ctx.AuthEnabled() {
			return fmt.Errorf("plugin command import actor is no longer available")
		}
		// Auth-off invocations deliberately carry no human actor. Resolve the
		// process's root principal just as the create-stamp callback does; the
		// durable claim is stamped with this same id, so deletion between this
		// check and import still nulls the claim and fails closed below.
		root, err := ctx.RootAdminPrincipal()
		if err != nil || root == nil || root.UserID == 0 {
			return fmt.Errorf("plugin command import actor is no longer available")
		}
		actorID = root.UserID
	} else {
		actorID = *validation.ActorUserID
	}
	pm := ctx.PluginManager()
	if pm == nil || !pm.GenerationActive(validation.PluginName, validation.PluginGeneration) {
		return fmt.Errorf("plugin command import generation is no longer active")
	}
	var capabilities plugin_system.CapabilitySet
	for _, info := range pm.Plugins() {
		if info.Name == validation.PluginName && info.Generation == validation.PluginGeneration {
			capabilities = info.Manifest.Capabilities()
			break
		}
	}
	if !capabilities.Has(plugin_system.CapCommands) || !capabilities.Has(plugin_system.CapDBWrite) {
		return fmt.Errorf("plugin command import requires commands and db:write capabilities")
	}

	principal := ctx.principalForPluginActor(actorID)
	bound := ctx.WithPrincipal(principal)
	if err := bound.requireWriteRole("import plugin command output"); err != nil {
		return err
	}
	if err := ValidateAssociationIDs[models.Group](bound.db, validation.Fields.GroupIDs, "groups"); err != nil {
		return fmt.Errorf("validate plugin command import groups: %w", err)
	}
	return nil
}

// ImportResource consumes only a host-created snapshot. The acting principal is
// rebound immediately before AddResource so both scope callbacks and the global
// create stamp use the claim's current, enabled account.
func (ctx *MahresourcesContext) ImportResource(callCtx context.Context, source plugin_commands.ImportSource, fields plugin_commands.ResourceFields, scratchDir string) (uint, error) {
	if err := callCtx.Err(); err != nil {
		return 0, err
	}
	// The validation immediately preceding this call copied the actor into the
	// claim. Read it back so a user-deletion sweep between validation and import
	// cannot turn the worker into an unrestricted system caller.
	var claim models.PluginCommandImport
	if err := ctx.db.Where("id = ?", source.ImportID).First(&claim).Error; err != nil {
		return 0, fmt.Errorf("read plugin command import claim: %w", err)
	}
	if claim.CreatedByUserId == nil || *claim.CreatedByUserId == 0 {
		return 0, fmt.Errorf("plugin command import actor is no longer available")
	}
	actorID := *claim.CreatedByUserId
	principal := ctx.principalForPluginActor(actorID)
	bound := ctx.WithPrincipal(principal)
	if err := bound.requireWriteRole("import plugin command output"); err != nil {
		return 0, err
	}
	if err := ValidateAssociationIDs[models.Group](bound.db, fields.GroupIDs, "groups"); err != nil {
		return 0, fmt.Errorf("validate plugin command import groups: %w", err)
	}

	file := source.File
	if file == nil || source.CreateScratch == nil {
		return 0, fmt.Errorf("plugin command import snapshot capability is unavailable")
	}
	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat plugin command import snapshot: %w", err)
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("plugin command import snapshot is not a regular file")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("rewind plugin command import snapshot: %w", err)
	}

	meta := ""
	if fields.Meta != nil {
		encoded, err := json.Marshal(fields.Meta)
		if err != nil {
			return 0, fmt.Errorf("encode plugin command import meta: %w", err)
		}
		meta = string(encoded)
	}
	name := fields.Name
	if name == "" {
		name = source.FileName
	}
	// The command-import schema intentionally exposes no arbitrary owner_id.
	// A scoped actor's resource therefore belongs to that actor's scope root;
	// otherwise an ownerless insert is rejected by the scope create callback
	// and, worse, would be invisible to the actor immediately after success.
	var ownerID uint
	if principal != nil && principal.ScopeGroupID != nil {
		ownerID = *principal.ScopeGroupID
	}
	query := &query_models.ResourceCreator{ResourceQueryBase: query_models.ResourceQueryBase{
		Name: name, Description: fields.Description, OwnerId: ownerID,
		Groups: append([]uint(nil), fields.GroupIDs...), Tags: append([]uint(nil), fields.TagIDs...),
		Meta: meta, OriginalName: source.FileName,
	}}
	resource, err := bound.addResourceWithOptions(
		&contextImportFile{File: file, ctx: callCtx}, source.FileName, query,
		addResourceOptions{ScratchDir: scratchDir, CreateScratch: source.CreateScratch},
	)
	if err != nil {
		var duplicate *ResourceExistsError
		if errors.As(err, &duplicate) && duplicate.ResourceID != 0 {
			return duplicate.ResourceID, nil
		}
		return 0, err
	}
	return resource.ID, nil
}

type contextImportFile struct {
	File *os.File
	ctx  context.Context
}

func (f *contextImportFile) Read(p []byte) (int, error) {
	select {
	case <-f.ctx.Done():
		return 0, f.ctx.Err()
	default:
		return f.File.Read(p)
	}
}

func (f *contextImportFile) Close() error { return f.File.Close() }

var _ io.ReadCloser = (*contextImportFile)(nil)
