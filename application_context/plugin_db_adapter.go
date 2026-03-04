package application_context

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
)

// pluginDBAdapter implements plugin_system.EntityQuerier using MahresourcesContext.
type pluginDBAdapter struct {
	ctx *MahresourcesContext
}

// NewPluginDBAdapter creates an adapter for plugin DB access.
func NewPluginDBAdapter(ctx *MahresourcesContext) plugin_system.EntityQuerier {
	return &pluginDBAdapter{ctx: ctx}
}

func (a *pluginDBAdapter) GetNoteData(id uint) (map[string]any, error) {
	note, err := a.ctx.GetNote(id)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"id":          float64(note.ID),
		"name":        note.Name,
		"description": note.Description,
		"meta":        string(note.Meta),
	}
	if note.NoteType != nil {
		result["note_type"] = note.NoteType.Name
	}
	if note.OwnerId != nil {
		result["owner_id"] = float64(*note.OwnerId)
	}
	if len(note.Tags) > 0 {
		tags := make([]any, len(note.Tags))
		for i, t := range note.Tags {
			tags[i] = map[string]any{"id": float64(t.ID), "name": t.Name}
		}
		result["tags"] = tags
	}
	return result, nil
}

func (a *pluginDBAdapter) GetResourceData(id uint) (map[string]any, error) {
	resource, err := a.ctx.GetResource(id)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"id":                float64(resource.ID),
		"name":              resource.Name,
		"description":       resource.Description,
		"meta":              string(resource.Meta),
		"content_type":      resource.ContentType,
		"original_filename": resource.OriginalName,
		"hash":              resource.Hash,
	}
	if resource.OwnerId != nil {
		result["owner_id"] = float64(*resource.OwnerId)
	}
	if len(resource.Tags) > 0 {
		tags := make([]any, len(resource.Tags))
		for i, t := range resource.Tags {
			tags[i] = map[string]any{"id": float64(t.ID), "name": t.Name}
		}
		result["tags"] = tags
	}
	return result, nil
}

func (a *pluginDBAdapter) GetGroupData(id uint) (map[string]any, error) {
	group, err := a.ctx.GetGroup(id)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"id":          float64(group.ID),
		"name":        group.Name,
		"description": group.Description,
		"meta":        string(group.Meta),
	}
	if group.OwnerId != nil {
		result["owner_id"] = float64(*group.OwnerId)
	}
	if group.Category != nil {
		result["category"] = group.Category.Name
	}
	if len(group.Tags) > 0 {
		tags := make([]any, len(group.Tags))
		for i, t := range group.Tags {
			tags[i] = map[string]any{"id": float64(t.ID), "name": t.Name}
		}
		result["tags"] = tags
	}
	return result, nil
}

func (a *pluginDBAdapter) GetTagData(id uint) (map[string]any, error) {
	tag, err := a.ctx.GetTag(id)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":   float64(tag.ID),
		"name": tag.Name,
	}, nil
}

func (a *pluginDBAdapter) GetCategoryData(id uint) (map[string]any, error) {
	cat, err := a.ctx.GetCategory(id)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":          float64(cat.ID),
		"name":        cat.Name,
		"description": cat.Description,
	}, nil
}

// queryLimit extracts a capped limit from the filter map.
// Default is 20, maximum is 100.
func queryLimit(filter map[string]any) int {
	limit := 20
	if l, ok := filter["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 100 {
			limit = 100
		}
	}
	return limit
}

// queryOffset extracts a capped offset from the filter map.
// Default is 0, maximum is 10000.
func queryOffset(filter map[string]any) int {
	offset := 0
	if o, ok := filter["offset"].(float64); ok && o > 0 {
		offset = int(o)
		if offset > 10000 {
			offset = 10000
		}
	}
	return offset
}

func (a *pluginDBAdapter) QueryNotes(filter map[string]any) ([]map[string]any, error) {
	limit := queryLimit(filter)
	offset := queryOffset(filter)
	query := &query_models.NoteQuery{}
	if name, ok := filter["name"].(string); ok {
		query.Name = name
	}
	notes, err := a.ctx.GetNotes(offset, limit, query)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(notes))
	for i, n := range notes {
		results[i] = map[string]any{
			"id":          float64(n.ID),
			"name":        n.Name,
			"description": n.Description,
		}
	}
	return results, nil
}

func (a *pluginDBAdapter) QueryResources(filter map[string]any) ([]map[string]any, error) {
	limit := queryLimit(filter)
	offset := queryOffset(filter)
	query := &query_models.ResourceSearchQuery{}
	if name, ok := filter["name"].(string); ok {
		query.Name = name
	}
	if ct, ok := filter["content_type"].(string); ok {
		query.ContentType = ct
	}
	resources, err := a.ctx.GetResources(offset, limit, query)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(resources))
	for i, r := range resources {
		results[i] = map[string]any{
			"id":           float64(r.ID),
			"name":         r.Name,
			"content_type": r.ContentType,
		}
	}
	return results, nil
}

func (a *pluginDBAdapter) QueryGroups(filter map[string]any) ([]map[string]any, error) {
	limit := queryLimit(filter)
	offset := queryOffset(filter)
	query := &query_models.GroupQuery{}
	if name, ok := filter["name"].(string); ok {
		query.Name = name
	}
	groups, err := a.ctx.GetGroups(offset, limit, query)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(groups))
	for i, g := range groups {
		results[i] = map[string]any{
			"id":          float64(g.ID),
			"name":        g.Name,
			"description": g.Description,
		}
	}
	return results, nil
}

const maxResourceFileSize = 50 * 1024 * 1024 // 50MB

func (a *pluginDBAdapter) GetResourceFileData(id uint) (string, string, error) {
	resource, err := a.ctx.GetResourceByID(id)
	if err != nil {
		return "", "", err
	}

	fs, err := a.ctx.GetFsForStorageLocation(resource.StorageLocation)
	if err != nil {
		return "", "", fmt.Errorf("storage not available: %w", err)
	}

	file, err := fs.Open(resource.GetCleanLocation())
	if err != nil {
		return "", "", fmt.Errorf("could not open file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxResourceFileSize+1))
	if err != nil {
		return "", "", fmt.Errorf("could not read file: %w", err)
	}
	if len(data) > maxResourceFileSize {
		return "", "", fmt.Errorf("file too large (max %d bytes)", maxResourceFileSize)
	}

	return base64.StdEncoding.EncodeToString(data), resource.ContentType, nil
}

func (a *pluginDBAdapter) CreateResourceFromURL(url string, options map[string]any) (map[string]any, error) {
	creator := &query_models.ResourceFromRemoteCreator{
		URL: url,
	}
	applyResourceOptions(&creator.ResourceQueryBase, options)

	resource, err := a.ctx.AddRemoteResource(creator)
	if err != nil {
		return nil, err
	}
	return resourceToMap(resource), nil
}

func (a *pluginDBAdapter) CreateResourceFromData(base64Data string, options map[string]any) (map[string]any, error) {
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 data: %w", err)
	}

	creator := &query_models.ResourceCreator{}
	applyResourceOptions(&creator.ResourceQueryBase, options)

	fileName := "plugin_upload"
	if name, ok := options["name"].(string); ok && name != "" {
		fileName = name
	}

	resource, err := a.ctx.AddResource(io.NopCloser(bytes.NewReader(data)), fileName, creator)
	if err != nil {
		return nil, err
	}
	return resourceToMap(resource), nil
}

// resourceToMap converts a Resource model to a map suitable for Lua.
func resourceToMap(r *models.Resource) map[string]any {
	result := map[string]any{
		"id":                float64(r.ID),
		"name":              r.Name,
		"description":       r.Description,
		"content_type":      r.ContentType,
		"original_filename": r.OriginalName,
		"hash":              r.Hash,
	}
	if r.OwnerId != nil {
		result["owner_id"] = float64(*r.OwnerId)
	}
	return result
}

// applyResourceOptions sets common fields from plugin options map.
func applyResourceOptions(base *query_models.ResourceQueryBase, options map[string]any) {
	if name, ok := options["name"].(string); ok {
		base.Name = name
	}
	if desc, ok := options["description"].(string); ok {
		base.Description = desc
	}
	if ownerID, ok := options["owner_id"].(float64); ok && ownerID > 0 {
		base.OwnerId = uint(ownerID)
	}
	if tags, ok := options["tags"].([]any); ok {
		for _, t := range tags {
			if id, ok := t.(float64); ok {
				base.Tags = append(base.Tags, uint(id))
			}
		}
	}
	if groups, ok := options["groups"].([]any); ok {
		for _, g := range groups {
			if id, ok := g.(float64); ok {
				base.Groups = append(base.Groups, uint(id))
			}
		}
	}
}
