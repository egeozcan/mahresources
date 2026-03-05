package application_context

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
	"net/url"
	"strings"
)

// pluginDBAdapter implements plugin_system.EntityQuerier using MahresourcesContext.
type pluginDBAdapter struct {
	ctx *MahresourcesContext
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
	// Use GetResourceByID (no association preloading) since we only need
	// StorageLocation, Location, and ContentType — not tags or relations.
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
	lower := strings.ToLower(url)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return nil, fmt.Errorf("unsupported URL scheme (only http and https are allowed)")
	}

	creator := &query_models.ResourceFromRemoteCreator{
		URL: url,
	}
	applyResourceOptions(&creator.ResourceQueryBase, options)

	// AddRemoteResource uses FileName (not ResourceQueryBase.Name) for naming.
	// Propagate the Name option so the plugin-specified name is used instead of
	// falling back to path.Base(url).
	if name, ok := options["name"].(string); ok && name != "" {
		creator.FileName = name
	}

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
// Note: this intentionally omits description, meta, and tags (unlike GetResourceData)
// because newly-created resources may not have those fields populated yet.
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

// Compile-time interface compliance checks.
var _ plugin_system.EntityWriter = (*pluginDBAdapter)(nil)
var _ plugin_system.PluginLogger = (*pluginDBAdapter)(nil)
var _ plugin_system.KVStore = (*pluginDBAdapter)(nil)

func (a *pluginDBAdapter) KVGet(pluginName, key string) (string, bool, error) {
	return a.ctx.PluginKVGet(pluginName, key)
}
func (a *pluginDBAdapter) KVSet(pluginName, key, value string) error {
	return a.ctx.PluginKVSet(pluginName, key, value)
}
func (a *pluginDBAdapter) KVDelete(pluginName, key string) error {
	return a.ctx.PluginKVDelete(pluginName, key)
}
func (a *pluginDBAdapter) KVList(pluginName, prefix string) ([]string, error) {
	return a.ctx.PluginKVList(pluginName, prefix)
}
func (a *pluginDBAdapter) KVPurge(pluginName string) error {
	return a.ctx.PluginKVPurge(pluginName)
}

// PluginLog persists a plugin log message to the application log store.
func (a *pluginDBAdapter) PluginLog(pluginName, level, message string, details map[string]any) {
	switch level {
	case "warning":
		a.ctx.Logger().Warning("plugin", "plugin", nil, pluginName, message, details)
	case "error":
		a.ctx.Logger().Error("plugin", "plugin", nil, pluginName, message, details)
	default:
		a.ctx.Logger().Info("plugin", "plugin", nil, pluginName, message, details)
	}
}

// --- Helper functions for extracting typed values from option maps ---

// getStringOpt extracts a string value from an options map.
func getStringOpt(opts map[string]any, key string) string {
	if v, ok := opts[key].(string); ok {
		return v
	}
	return ""
}

// getUintOpt extracts a uint value from an options map (expects float64 from Lua).
func getUintOpt(opts map[string]any, key string) uint {
	if v, ok := opts[key].(float64); ok && v > 0 {
		return uint(v)
	}
	return 0
}

// getUintSliceOpt extracts a []uint from an options map.
// Handles both []any (proper arrays) and map[string]any (Lua tables with
// integer keys that luaTableToGoMap parses as maps).
func getUintSliceOpt(opts map[string]any, key string) []uint {
	switch v := opts[key].(type) {
	case []any:
		result := make([]uint, 0, len(v))
		for _, item := range v {
			if id, ok := item.(float64); ok && id > 0 {
				result = append(result, uint(id))
			}
		}
		return result
	case map[string]any:
		result := make([]uint, 0, len(v))
		for _, item := range v {
			if id, ok := item.(float64); ok && id > 0 {
				result = append(result, uint(id))
			}
		}
		return result
	}
	return nil
}

// --- Patch helpers: use current value when key is absent from opts ---

// patchString returns opts[key] if present, otherwise current.
func patchString(opts map[string]any, key, current string) string {
	if _, exists := opts[key]; exists {
		return getStringOpt(opts, key)
	}
	return current
}

// patchUint returns opts[key] if present, otherwise current.
func patchUint(opts map[string]any, key string, current uint) uint {
	if _, exists := opts[key]; exists {
		return getUintOpt(opts, key)
	}
	return current
}

// patchUintSlice returns opts[key] if present, otherwise current.
func patchUintSlice(opts map[string]any, key string, current []uint) []uint {
	if _, exists := opts[key]; exists {
		return getUintSliceOpt(opts, key)
	}
	return current
}

func uintPtrVal(p *uint) uint {
	if p == nil {
		return 0
	}
	return *p
}

func extractTagIDs(tags []*models.Tag) []uint {
	ids := make([]uint, len(tags))
	for i, t := range tags {
		ids[i] = t.ID
	}
	return ids
}

func extractGroupIDs(groups []*models.Group) []uint {
	ids := make([]uint, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
	}
	return ids
}

func extractResourceIDs(resources []*models.Resource) []uint {
	ids := make([]uint, len(resources))
	for i, r := range resources {
		ids[i] = r.ID
	}
	return ids
}

// --- Converter functions: model -> map[string]any (float64 for Lua) ---

func groupToMap(g *models.Group) map[string]any {
	result := map[string]any{
		"id":          float64(g.ID),
		"name":        g.Name,
		"description": g.Description,
		"meta":        string(g.Meta),
	}
	if g.OwnerId != nil {
		result["owner_id"] = float64(*g.OwnerId)
	}
	if g.CategoryId != nil {
		result["category_id"] = float64(*g.CategoryId)
	}
	return result
}

func noteToMap(n *models.Note) map[string]any {
	result := map[string]any{
		"id":          float64(n.ID),
		"name":        n.Name,
		"description": n.Description,
		"meta":        string(n.Meta),
	}
	if n.OwnerId != nil {
		result["owner_id"] = float64(*n.OwnerId)
	}
	if n.NoteTypeId != nil {
		result["note_type_id"] = float64(*n.NoteTypeId)
	}
	return result
}

func tagToMap(t *models.Tag) map[string]any {
	return map[string]any{
		"id":          float64(t.ID),
		"name":        t.Name,
		"description": t.Description,
	}
}

func categoryToMap(c *models.Category) map[string]any {
	return map[string]any{
		"id":             float64(c.ID),
		"name":           c.Name,
		"description":    c.Description,
		"custom_header":  c.CustomHeader,
		"custom_sidebar": c.CustomSidebar,
		"custom_summary": c.CustomSummary,
		"custom_avatar":  c.CustomAvatar,
		"meta_schema":    c.MetaSchema,
	}
}

func resourceCategoryToMap(rc *models.ResourceCategory) map[string]any {
	return map[string]any{
		"id":             float64(rc.ID),
		"name":           rc.Name,
		"description":    rc.Description,
		"custom_header":  rc.CustomHeader,
		"custom_sidebar": rc.CustomSidebar,
		"custom_summary": rc.CustomSummary,
		"custom_avatar":  rc.CustomAvatar,
		"meta_schema":    rc.MetaSchema,
	}
}

func noteTypeToMap(nt *models.NoteType) map[string]any {
	return map[string]any{
		"id":             float64(nt.ID),
		"name":           nt.Name,
		"description":    nt.Description,
		"custom_header":  nt.CustomHeader,
		"custom_sidebar": nt.CustomSidebar,
		"custom_summary": nt.CustomSummary,
		"custom_avatar":  nt.CustomAvatar,
	}
}

func groupRelationToMap(r *models.GroupRelation) map[string]any {
	result := map[string]any{
		"id":          float64(r.ID),
		"name":        r.Name,
		"description": r.Description,
	}
	if r.FromGroupId != nil {
		result["from_group_id"] = float64(*r.FromGroupId)
	}
	if r.ToGroupId != nil {
		result["to_group_id"] = float64(*r.ToGroupId)
	}
	if r.RelationTypeId != nil {
		result["relation_type_id"] = float64(*r.RelationTypeId)
	}
	return result
}

func relationTypeToMap(rt *models.GroupRelationType) map[string]any {
	result := map[string]any{
		"id":          float64(rt.ID),
		"name":        rt.Name,
		"description": rt.Description,
	}
	if rt.FromCategoryId != nil {
		result["from_category_id"] = float64(*rt.FromCategoryId)
	}
	if rt.ToCategoryId != nil {
		result["to_category_id"] = float64(*rt.ToCategoryId)
	}
	if rt.BackRelationId != nil {
		result["back_relation_id"] = float64(*rt.BackRelationId)
	}
	return result
}

// --- EntityWriter: Group CRUD ---

func (a *pluginDBAdapter) CreateGroup(opts map[string]any) (map[string]any, error) {
	creator := &query_models.GroupCreator{
		Name:        getStringOpt(opts, "name"),
		Description: getStringOpt(opts, "description"),
		Meta:        getStringOpt(opts, "meta"),
		URL:         getStringOpt(opts, "url"),
		CategoryId:  getUintOpt(opts, "category_id"),
		OwnerId:     getUintOpt(opts, "owner_id"),
		Tags:        getUintSliceOpt(opts, "tags"),
		Groups:      getUintSliceOpt(opts, "groups"),
	}
	group, err := a.ctx.CreateGroup(creator)
	if err != nil {
		return nil, err
	}
	return groupToMap(group), nil
}

func (a *pluginDBAdapter) UpdateGroup(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.GroupEditor{
		GroupCreator: query_models.GroupCreator{
			Name:        getStringOpt(opts, "name"),
			Description: getStringOpt(opts, "description"),
			Meta:        getStringOpt(opts, "meta"),
			URL:         getStringOpt(opts, "url"),
			CategoryId:  getUintOpt(opts, "category_id"),
			OwnerId:     getUintOpt(opts, "owner_id"),
			Tags:        getUintSliceOpt(opts, "tags"),
			Groups:      getUintSliceOpt(opts, "groups"),
		},
		ID: id,
	}
	group, err := a.ctx.UpdateGroup(editor)
	if err != nil {
		return nil, err
	}
	return groupToMap(group), nil
}

func (a *pluginDBAdapter) DeleteGroup(id uint) error {
	return a.ctx.DeleteGroup(id)
}

func (a *pluginDBAdapter) PatchGroup(id uint, opts map[string]any) (map[string]any, error) {
	group, err := a.ctx.GetGroup(id)
	if err != nil {
		return nil, err
	}
	var urlStr string
	if group.URL != nil {
		u := url.URL(*group.URL)
		urlStr = u.String()
	}
	editor := &query_models.GroupEditor{
		ID: id,
		GroupCreator: query_models.GroupCreator{
			Name:        patchString(opts, "name", group.Name),
			Description: patchString(opts, "description", group.Description),
			Meta:        patchString(opts, "meta", string(group.Meta)),
			URL:         patchString(opts, "url", urlStr),
			CategoryId:  patchUint(opts, "category_id", uintPtrVal(group.CategoryId)),
			OwnerId:     patchUint(opts, "owner_id", uintPtrVal(group.OwnerId)),
			Tags:        patchUintSlice(opts, "tags", extractTagIDs(group.Tags)),
			Groups:      patchUintSlice(opts, "groups", extractGroupIDs(group.RelatedGroups)),
		},
	}
	result, err := a.ctx.UpdateGroup(editor)
	if err != nil {
		return nil, err
	}
	return groupToMap(result), nil
}

// --- EntityWriter: Note CRUD ---

func (a *pluginDBAdapter) CreateNote(opts map[string]any) (map[string]any, error) {
	editor := &query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name:        getStringOpt(opts, "name"),
			Description: getStringOpt(opts, "description"),
			Meta:        getStringOpt(opts, "meta"),
			StartDate:   getStringOpt(opts, "start_date"),
			EndDate:     getStringOpt(opts, "end_date"),
			OwnerId:     getUintOpt(opts, "owner_id"),
			NoteTypeId:  getUintOpt(opts, "note_type_id"),
			Tags:        getUintSliceOpt(opts, "tags"),
			Groups:      getUintSliceOpt(opts, "groups"),
			Resources:   getUintSliceOpt(opts, "resources"),
		},
		ID: 0, // ID=0 means create
	}

	note, err := a.ctx.CreateOrUpdateNote(editor)
	if err != nil {
		return nil, err
	}
	return noteToMap(note), nil
}

func (a *pluginDBAdapter) UpdateNote(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name:        getStringOpt(opts, "name"),
			Description: getStringOpt(opts, "description"),
			Meta:        getStringOpt(opts, "meta"),
			StartDate:   getStringOpt(opts, "start_date"),
			EndDate:     getStringOpt(opts, "end_date"),
			OwnerId:     getUintOpt(opts, "owner_id"),
			NoteTypeId:  getUintOpt(opts, "note_type_id"),
			Tags:        getUintSliceOpt(opts, "tags"),
			Groups:      getUintSliceOpt(opts, "groups"),
			Resources:   getUintSliceOpt(opts, "resources"),
		},
		ID: id, // ID!=0 means update
	}

	note, err := a.ctx.CreateOrUpdateNote(editor)
	if err != nil {
		return nil, err
	}
	return noteToMap(note), nil
}

func (a *pluginDBAdapter) DeleteNote(id uint) error {
	return a.ctx.DeleteNote(id)
}

func (a *pluginDBAdapter) PatchNote(id uint, opts map[string]any) (map[string]any, error) {
	note, err := a.ctx.GetNote(id)
	if err != nil {
		return nil, err
	}
	var startDate, endDate string
	if note.StartDate != nil {
		startDate = note.StartDate.Format("2006-01-02T03:04")
	}
	if note.EndDate != nil {
		endDate = note.EndDate.Format("2006-01-02T03:04")
	}
	editor := &query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name:        patchString(opts, "name", note.Name),
			Description: patchString(opts, "description", note.Description),
			Meta:        patchString(opts, "meta", string(note.Meta)),
			StartDate:   patchString(opts, "start_date", startDate),
			EndDate:     patchString(opts, "end_date", endDate),
			OwnerId:     patchUint(opts, "owner_id", uintPtrVal(note.OwnerId)),
			NoteTypeId:  patchUint(opts, "note_type_id", uintPtrVal(note.NoteTypeId)),
			Tags:        patchUintSlice(opts, "tags", extractTagIDs(note.Tags)),
			Groups:      patchUintSlice(opts, "groups", extractGroupIDs(note.Groups)),
			Resources:   patchUintSlice(opts, "resources", extractResourceIDs(note.Resources)),
		},
		ID: id,
	}

	result, err := a.ctx.CreateOrUpdateNote(editor)
	if err != nil {
		return nil, err
	}
	return noteToMap(result), nil
}

// --- EntityWriter: Tag CRUD ---

func (a *pluginDBAdapter) CreateTag(opts map[string]any) (map[string]any, error) {
	creator := &query_models.TagCreator{
		Name:        getStringOpt(opts, "name"),
		Description: getStringOpt(opts, "description"),
	}
	tag, err := a.ctx.CreateTag(creator)
	if err != nil {
		return nil, err
	}
	return tagToMap(tag), nil
}

func (a *pluginDBAdapter) UpdateTag(id uint, opts map[string]any) (map[string]any, error) {
	creator := &query_models.TagCreator{
		ID:          id,
		Name:        getStringOpt(opts, "name"),
		Description: getStringOpt(opts, "description"),
	}
	tag, err := a.ctx.UpdateTag(creator)
	if err != nil {
		return nil, err
	}
	return tagToMap(tag), nil
}

func (a *pluginDBAdapter) DeleteTag(id uint) error {
	return a.ctx.DeleteTag(id)
}

func (a *pluginDBAdapter) PatchTag(id uint, opts map[string]any) (map[string]any, error) {
	tag, err := a.ctx.GetTag(id)
	if err != nil {
		return nil, err
	}
	creator := &query_models.TagCreator{
		ID:          id,
		Name:        patchString(opts, "name", tag.Name),
		Description: patchString(opts, "description", tag.Description),
	}
	result, err := a.ctx.UpdateTag(creator)
	if err != nil {
		return nil, err
	}
	return tagToMap(result), nil
}

// --- EntityWriter: Category CRUD ---

func (a *pluginDBAdapter) CreateCategory(opts map[string]any) (map[string]any, error) {
	creator := &query_models.CategoryCreator{
		Name:          getStringOpt(opts, "name"),
		Description:   getStringOpt(opts, "description"),
		CustomHeader:  getStringOpt(opts, "custom_header"),
		CustomSidebar: getStringOpt(opts, "custom_sidebar"),
		CustomSummary: getStringOpt(opts, "custom_summary"),
		CustomAvatar:  getStringOpt(opts, "custom_avatar"),
		MetaSchema:    getStringOpt(opts, "meta_schema"),
	}
	cat, err := a.ctx.CreateCategory(creator)
	if err != nil {
		return nil, err
	}
	return categoryToMap(cat), nil
}

func (a *pluginDBAdapter) UpdateCategory(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.CategoryEditor{
		CategoryCreator: query_models.CategoryCreator{
			Name:          getStringOpt(opts, "name"),
			Description:   getStringOpt(opts, "description"),
			CustomHeader:  getStringOpt(opts, "custom_header"),
			CustomSidebar: getStringOpt(opts, "custom_sidebar"),
			CustomSummary: getStringOpt(opts, "custom_summary"),
			CustomAvatar:  getStringOpt(opts, "custom_avatar"),
			MetaSchema:    getStringOpt(opts, "meta_schema"),
		},
		ID: id,
	}
	cat, err := a.ctx.UpdateCategory(editor)
	if err != nil {
		return nil, err
	}
	return categoryToMap(cat), nil
}

func (a *pluginDBAdapter) DeleteCategory(id uint) error {
	return a.ctx.DeleteCategory(id)
}

func (a *pluginDBAdapter) PatchCategory(id uint, opts map[string]any) (map[string]any, error) {
	cat, err := a.ctx.GetCategory(id)
	if err != nil {
		return nil, err
	}
	editor := &query_models.CategoryEditor{
		CategoryCreator: query_models.CategoryCreator{
			Name:          patchString(opts, "name", cat.Name),
			Description:   patchString(opts, "description", cat.Description),
			CustomHeader:  patchString(opts, "custom_header", cat.CustomHeader),
			CustomSidebar: patchString(opts, "custom_sidebar", cat.CustomSidebar),
			CustomSummary: patchString(opts, "custom_summary", cat.CustomSummary),
			CustomAvatar:  patchString(opts, "custom_avatar", cat.CustomAvatar),
			MetaSchema:    patchString(opts, "meta_schema", cat.MetaSchema),
		},
		ID: id,
	}
	result, err := a.ctx.UpdateCategory(editor)
	if err != nil {
		return nil, err
	}
	return categoryToMap(result), nil
}

// --- EntityWriter: ResourceCategory CRUD ---

func (a *pluginDBAdapter) CreateResourceCategory(opts map[string]any) (map[string]any, error) {
	creator := &query_models.ResourceCategoryCreator{
		Name:          getStringOpt(opts, "name"),
		Description:   getStringOpt(opts, "description"),
		CustomHeader:  getStringOpt(opts, "custom_header"),
		CustomSidebar: getStringOpt(opts, "custom_sidebar"),
		CustomSummary: getStringOpt(opts, "custom_summary"),
		CustomAvatar:  getStringOpt(opts, "custom_avatar"),
		MetaSchema:    getStringOpt(opts, "meta_schema"),
	}
	rc, err := a.ctx.CreateResourceCategory(creator)
	if err != nil {
		return nil, err
	}
	return resourceCategoryToMap(rc), nil
}

func (a *pluginDBAdapter) UpdateResourceCategory(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.ResourceCategoryEditor{
		ResourceCategoryCreator: query_models.ResourceCategoryCreator{
			Name:          getStringOpt(opts, "name"),
			Description:   getStringOpt(opts, "description"),
			CustomHeader:  getStringOpt(opts, "custom_header"),
			CustomSidebar: getStringOpt(opts, "custom_sidebar"),
			CustomSummary: getStringOpt(opts, "custom_summary"),
			CustomAvatar:  getStringOpt(opts, "custom_avatar"),
			MetaSchema:    getStringOpt(opts, "meta_schema"),
		},
		ID: id,
	}
	rc, err := a.ctx.UpdateResourceCategory(editor)
	if err != nil {
		return nil, err
	}
	return resourceCategoryToMap(rc), nil
}

func (a *pluginDBAdapter) DeleteResourceCategory(id uint) error {
	return a.ctx.DeleteResourceCategory(id)
}

func (a *pluginDBAdapter) PatchResourceCategory(id uint, opts map[string]any) (map[string]any, error) {
	rc, err := a.ctx.GetResourceCategory(id)
	if err != nil {
		return nil, err
	}
	editor := &query_models.ResourceCategoryEditor{
		ResourceCategoryCreator: query_models.ResourceCategoryCreator{
			Name:          patchString(opts, "name", rc.Name),
			Description:   patchString(opts, "description", rc.Description),
			CustomHeader:  patchString(opts, "custom_header", rc.CustomHeader),
			CustomSidebar: patchString(opts, "custom_sidebar", rc.CustomSidebar),
			CustomSummary: patchString(opts, "custom_summary", rc.CustomSummary),
			CustomAvatar:  patchString(opts, "custom_avatar", rc.CustomAvatar),
			MetaSchema:    patchString(opts, "meta_schema", rc.MetaSchema),
		},
		ID: id,
	}
	result, err := a.ctx.UpdateResourceCategory(editor)
	if err != nil {
		return nil, err
	}
	return resourceCategoryToMap(result), nil
}

// --- EntityWriter: NoteType CRUD ---

func (a *pluginDBAdapter) CreateNoteType(opts map[string]any) (map[string]any, error) {
	editor := &query_models.NoteTypeEditor{
		ID:            0, // ID=0 means create
		Name:          getStringOpt(opts, "name"),
		Description:   getStringOpt(opts, "description"),
		CustomHeader:  getStringOpt(opts, "custom_header"),
		CustomSidebar: getStringOpt(opts, "custom_sidebar"),
		CustomSummary: getStringOpt(opts, "custom_summary"),
		CustomAvatar:  getStringOpt(opts, "custom_avatar"),
	}
	nt, err := a.ctx.CreateOrUpdateNoteType(editor)
	if err != nil {
		return nil, err
	}
	return noteTypeToMap(nt), nil
}

func (a *pluginDBAdapter) UpdateNoteType(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.NoteTypeEditor{
		ID:            id,
		Name:          getStringOpt(opts, "name"),
		Description:   getStringOpt(opts, "description"),
		CustomHeader:  getStringOpt(opts, "custom_header"),
		CustomSidebar: getStringOpt(opts, "custom_sidebar"),
		CustomSummary: getStringOpt(opts, "custom_summary"),
		CustomAvatar:  getStringOpt(opts, "custom_avatar"),
	}
	nt, err := a.ctx.CreateOrUpdateNoteType(editor)
	if err != nil {
		return nil, err
	}
	return noteTypeToMap(nt), nil
}

func (a *pluginDBAdapter) DeleteNoteType(id uint) error {
	return a.ctx.DeleteNoteType(id)
}

func (a *pluginDBAdapter) PatchNoteType(id uint, opts map[string]any) (map[string]any, error) {
	nt, err := a.ctx.GetNoteType(id)
	if err != nil {
		return nil, err
	}
	editor := &query_models.NoteTypeEditor{
		ID:            id,
		Name:          patchString(opts, "name", nt.Name),
		Description:   patchString(opts, "description", nt.Description),
		CustomHeader:  patchString(opts, "custom_header", nt.CustomHeader),
		CustomSidebar: patchString(opts, "custom_sidebar", nt.CustomSidebar),
		CustomSummary: patchString(opts, "custom_summary", nt.CustomSummary),
		CustomAvatar:  patchString(opts, "custom_avatar", nt.CustomAvatar),
	}
	result, err := a.ctx.CreateOrUpdateNoteType(editor)
	if err != nil {
		return nil, err
	}
	return noteTypeToMap(result), nil
}

// --- EntityWriter: GroupRelation CRUD ---

func (a *pluginDBAdapter) CreateGroupRelation(opts map[string]any) (map[string]any, error) {
	fromGroupId := getUintOpt(opts, "from_group_id")
	toGroupId := getUintOpt(opts, "to_group_id")
	relationTypeId := getUintOpt(opts, "relation_type_id")

	if fromGroupId == 0 || toGroupId == 0 || relationTypeId == 0 {
		return nil, fmt.Errorf("from_group_id, to_group_id, and relation_type_id are required")
	}

	relation, err := a.ctx.AddRelation(fromGroupId, toGroupId, relationTypeId)
	if err != nil {
		return nil, err
	}

	// Optionally set name and description via EditRelation
	name := getStringOpt(opts, "name")
	description := getStringOpt(opts, "description")
	if name != "" || description != "" {
		relation, err = a.ctx.EditRelation(query_models.GroupRelationshipQuery{
			Id:          relation.ID,
			Name:        name,
			Description: description,
		})
		if err != nil {
			return nil, err
		}
	}

	return groupRelationToMap(relation), nil
}

func (a *pluginDBAdapter) UpdateGroupRelation(opts map[string]any) (map[string]any, error) {
	query := query_models.GroupRelationshipQuery{
		Id:          getUintOpt(opts, "id"),
		Name:        getStringOpt(opts, "name"),
		Description: getStringOpt(opts, "description"),
	}
	if query.Id == 0 {
		return nil, fmt.Errorf("id is required for updating a group relation")
	}
	relation, err := a.ctx.EditRelation(query)
	if err != nil {
		return nil, err
	}
	return groupRelationToMap(relation), nil
}

func (a *pluginDBAdapter) DeleteGroupRelation(id uint) error {
	return a.ctx.DeleteRelationship(id)
}

func (a *pluginDBAdapter) PatchGroupRelation(opts map[string]any) (map[string]any, error) {
	id := getUintOpt(opts, "id")
	if id == 0 {
		return nil, fmt.Errorf("id is required for patching a group relation")
	}
	rel, err := a.ctx.GetRelation(id)
	if err != nil {
		return nil, err
	}
	query := query_models.GroupRelationshipQuery{
		Id:          id,
		Name:        patchString(opts, "name", rel.Name),
		Description: patchString(opts, "description", rel.Description),
	}
	result, err := a.ctx.EditRelation(query)
	if err != nil {
		return nil, err
	}
	return groupRelationToMap(result), nil
}

// --- EntityWriter: RelationType CRUD ---

func (a *pluginDBAdapter) CreateRelationType(opts map[string]any) (map[string]any, error) {
	query := &query_models.RelationshipTypeEditorQuery{
		Name:         getStringOpt(opts, "name"),
		Description:  getStringOpt(opts, "description"),
		FromCategory: getUintOpt(opts, "from_category"),
		ToCategory:   getUintOpt(opts, "to_category"),
		ReverseName:  getStringOpt(opts, "reverse_name"),
	}
	rt, err := a.ctx.AddRelationType(query)
	if err != nil {
		return nil, err
	}
	return relationTypeToMap(rt), nil
}

func (a *pluginDBAdapter) UpdateRelationType(opts map[string]any) (map[string]any, error) {
	query := &query_models.RelationshipTypeEditorQuery{
		Id:           getUintOpt(opts, "id"),
		Name:         getStringOpt(opts, "name"),
		Description:  getStringOpt(opts, "description"),
		FromCategory: getUintOpt(opts, "from_category"),
		ToCategory:   getUintOpt(opts, "to_category"),
	}
	if query.Id == 0 {
		return nil, fmt.Errorf("id is required for updating a relation type")
	}
	rt, err := a.ctx.EditRelationType(query)
	if err != nil {
		return nil, err
	}
	return relationTypeToMap(rt), nil
}

func (a *pluginDBAdapter) DeleteRelationType(id uint) error {
	return a.ctx.DeleteRelationshipType(id)
}

func (a *pluginDBAdapter) PatchRelationType(opts map[string]any) (map[string]any, error) {
	id := getUintOpt(opts, "id")
	if id == 0 {
		return nil, fmt.Errorf("id is required for patching a relation type")
	}
	rt, err := a.ctx.GetRelationType(id)
	if err != nil {
		return nil, err
	}
	query := &query_models.RelationshipTypeEditorQuery{
		Id:           id,
		Name:         patchString(opts, "name", rt.Name),
		Description:  patchString(opts, "description", rt.Description),
		FromCategory: patchUint(opts, "from_category", uintPtrVal(rt.FromCategoryId)),
		ToCategory:   patchUint(opts, "to_category", uintPtrVal(rt.ToCategoryId)),
	}
	result, err := a.ctx.EditRelationType(query)
	if err != nil {
		return nil, err
	}
	return relationTypeToMap(result), nil
}

// --- EntityWriter: Resource deletion ---

func (a *pluginDBAdapter) DeleteResource(id uint) error {
	return a.ctx.DeleteResource(id)
}

// --- EntityWriter: Relationship management ---

func (a *pluginDBAdapter) AddTagsToEntity(entityType string, id uint, tagIds []uint) error {
	if len(tagIds) == 0 {
		return nil
	}
	switch entityType {
	case "group":
		return a.ctx.BulkAddTagsToGroups(&query_models.BulkEditQuery{
			BulkQuery: query_models.BulkQuery{ID: []uint{id}},
			EditedId:  tagIds,
		})
	case "resource":
		return a.ctx.BulkAddTagsToResources(&query_models.BulkEditQuery{
			BulkQuery: query_models.BulkQuery{ID: []uint{id}},
			EditedId:  tagIds,
		})
	case "note":
		return a.ctx.AddTagsToNote(id, tagIds)
	default:
		return fmt.Errorf("unsupported entity type for AddTagsToEntity: %s", entityType)
	}
}

func (a *pluginDBAdapter) RemoveTagsFromEntity(entityType string, id uint, tagIds []uint) error {
	if len(tagIds) == 0 {
		return nil
	}
	switch entityType {
	case "group":
		return a.ctx.BulkRemoveTagsFromGroups(&query_models.BulkEditQuery{
			BulkQuery: query_models.BulkQuery{ID: []uint{id}},
			EditedId:  tagIds,
		})
	case "resource":
		return a.ctx.BulkRemoveTagsFromResources(&query_models.BulkEditQuery{
			BulkQuery: query_models.BulkQuery{ID: []uint{id}},
			EditedId:  tagIds,
		})
	case "note":
		return a.ctx.RemoveTagsFromNote(id, tagIds)
	default:
		return fmt.Errorf("unsupported entity type for RemoveTagsFromEntity: %s", entityType)
	}
}

func (a *pluginDBAdapter) AddGroupsToEntity(entityType string, id uint, groupIds []uint) error {
	if len(groupIds) == 0 {
		return nil
	}
	switch entityType {
	case "resource":
		return a.ctx.BulkAddGroupsToResources(&query_models.BulkEditQuery{
			BulkQuery: query_models.BulkQuery{ID: []uint{id}},
			EditedId:  groupIds,
		})
	case "note":
		return a.ctx.AddGroupsToNote(id, groupIds)
	default:
		return fmt.Errorf("unsupported entity type for AddGroupsToEntity: %s", entityType)
	}
}

func (a *pluginDBAdapter) RemoveGroupsFromEntity(entityType string, id uint, groupIds []uint) error {
	if len(groupIds) == 0 {
		return nil
	}
	switch entityType {
	case "resource":
		return a.ctx.RemoveGroupsFromResource(id, groupIds)
	case "note":
		return a.ctx.RemoveGroupsFromNote(id, groupIds)
	default:
		return fmt.Errorf("unsupported entity type for RemoveGroupsFromEntity: %s", entityType)
	}
}

func (a *pluginDBAdapter) AddResourcesToNote(noteId uint, resourceIds []uint) error {
	if len(resourceIds) == 0 {
		return nil
	}
	return a.ctx.AddResourcesToNote(noteId, resourceIds)
}

func (a *pluginDBAdapter) RemoveResourcesFromNote(noteId uint, resourceIds []uint) error {
	if len(resourceIds) == 0 {
		return nil
	}
	return a.ctx.RemoveResourcesFromNote(noteId, resourceIds)
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
