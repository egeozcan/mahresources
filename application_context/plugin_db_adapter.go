package application_context

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
	"math"
	"mime/multipart"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
)

// pluginDBAdapter implements plugin_system.EntityQuerier using MahresourcesContext.
type pluginDBAdapter struct {
	ctx *MahresourcesContext
}

// BindInvocation returns querier and writer views that run as the user who
// triggered this plugin call, and that carry the call chain so a nested hook
// dispatch can refuse to re-enter a VM already on it.
//
// This is what makes a plugin's writes land with a real CreatedByUserId. Before
// it, every mah.db call ran on one adapter captured at wiring time whose context
// had no principal, so under -auth everything a plugin created was attributed to
// nobody.
//
// A zero actor deliberately skips the bind rather than binding a role-less
// principal with UserID 0. Auth-off and context-less worker paths land here, and
// the unbound singleton is exactly where the existing default-actor stamping
// already produces the right answer (root). applyPrincipalScope happens to
// early-return on actor 0 today, but relying on that would leave a non-nil
// principal with no role and no id on the context — a state nothing else in the
// tree produces.
func (a *pluginDBAdapter) BindInvocation(inv *plugin_system.Invocation) (plugin_system.EntityQuerier, plugin_system.EntityWriter) {
	var bound *MahresourcesContext
	if inv != nil && inv.ActorUserID != 0 {
		bound = a.ctx.WithPrincipal(a.ctx.principalForPluginActor(inv.ActorUserID))
	} else {
		clone := *a.ctx
		bound = &clone
	}
	bound.pluginInvocation = inv
	if inv != nil {
		// pluginFetch is set for EVERY plugin invocation, not only those
		// carrying a policy. Setting it alongside the policy made it
		// equivalent to "pluginEgress != nil", which left the fail-closed
		// branch in AddRemoteResource dead — and that branch exists precisely
		// for the case where a plugin is fetching and the policy did NOT
		// survive the trip.
		bound.pluginEgress = inv.Egress
		bound.pluginFetch = true
	}

	adapter := &pluginDBAdapter{ctx: bound}
	return adapter, adapter
}

// principalForPluginActor resolves the identity a plugin call must run as, from
// the only thing the plugin host is allowed to know about the caller: a user id.
//
// It reads the account rather than fabricating a principal from the id, and that
// is the whole point. A &auth.Principal{UserID: id} carries no role and no scope
// group, so it is neither IsScoped nor RequiresScope, applyPrincipalScope adds
// no subtree filter, and every mah.db call in the invocation runs unscoped. The
// URL-path deny (auth.PluginCodeAllowed) does not cover this, because hooks are
// not a URL path: they fire from ordinary scoped CRUD that a group-confined user
// is entitled to perform, so such a user creating a note in its own subtree woke
// a plugin that could then read the whole database.
//
// Fail-closed when the account cannot be read. A plugin call can outlive the
// request that started it — an async job or a drained HTTP callback carries its
// submitter's id and runs later — so the account may have been deleted or
// disabled in between, and "I could not find out what you may see" must not mean
// "everything".
//
// Cost: one indexed read per bind, plus the subtree CTE that WithPrincipal
// already runs for a scoped principal. Both are per mah.db call, because that is
// where the bind happens; the CTE is the dominant term and is unavoidable
// wherever the principal comes from. If it ever matters, the fix is to bind once
// per VM entry point rather than once per call — not to guess the principal.
func (ctx *MahresourcesContext) principalForPluginActor(actorID uint) *auth.Principal {
	if actorID == 0 {
		return nil
	}
	user, err := ctx.GetUser(actorID)
	if err != nil {
		// Only an *outage* is logged. A deleted account is an expected refusal
		// and arrives here as ErrUserNotFound, because GetUser maps
		// gorm.ErrRecordNotFound to it rather than returning (nil, nil) — so
		// testing err != nil alone logs the ordinary case and defeats the very
		// distinction the line exists to draw.
		//
		// It matters twice over. An admin deleting a user while that user's
		// async job is still looping over mah.db would otherwise write one row
		// per call; and the row is written by Logger.log through the same db
		// handle whose read just failed, with its own error swallowed to
		// stdout, so in a real outage it is the least likely write to land.
		// Logging only the outage keeps the noise off the common path.
		if !errors.Is(err, ErrUserNotFound) {
			ctx.Logger().Warning(models.LogActionPlugin, "plugin", nil, "Plugin actor unresolved",
				fmt.Sprintf("could not read user %d; denying this plugin call: %v", actorID, err), nil)
		}
		return deniedPluginPrincipal(actorID)
	}
	// GetUser never returns (nil, nil) today, so the nil arm is unreachable. It
	// stays because this is a security-critical resolver and the alternative to
	// an unreachable branch here is a nil dereference if that ever changes.
	if user == nil || user.Disabled {
		return deniedPluginPrincipal(actorID)
	}
	return auth.FromUser(user)
}

// deniedPluginPrincipal is this tree's canonical deny-all identity for
// subtree-scoped data: a role that must be scoped (Role.RequiresScopeGroup) with
// no scope group to resolve, so applyPrincipalScope materialises an empty
// allow-list — matching no rows and rejecting every write to a table
// scopeColumn maps (groups, resources, notes).
//
// It is a statement about what the caller may reach, not a claim that the actor
// is a guest. And it is deliberately not called "deny everything": global
// taxonomy (tags, categories, note types, relation types) carries no owner and
// is not subtree-scoped, so it stays reachable — that is a property of the scope
// mechanism, not of this principal, and role-based capability is not enforced
// below server/ at all.
//
// Relation *edges* were mostly taken out of that set by relationInScope
// (relation_context.go). They are the case that shows why the list is worth
// keeping accurate: an edge is not taxonomy, it is a statement about two
// groups, and nothing about "carries no owner" made it safe to expose.
//
// Closing it took two guards and a backstop. relationInScope covers the direct
// writes; refuseGlobalCascadeWhenScoped (scoping.go) rejects the three taxonomy
// operations that cascade to edges database-wide; and globalCascadeDeleteCallback
// backstops the two of those three whose cascades are transactional, for a
// delete issued through a handle carrying the scope filter.
//
// Still reachable, recorded rather than claimed closed: a group's own category
// change and its deletion both remove incident edges whose far endpoint is
// outside the subtree, and merge's degenerate self-edge sweep is database-wide.
// The first three are closable with subtree predicates; only the general case —
// a confined caller performing any admin-only taxonomy write — needs role
// capability below server/, which does not exist. See CLAUDE.md.
func deniedPluginPrincipal(actorID uint) *auth.Principal {
	return &auth.Principal{UserID: actorID, Role: models.RoleGuest}
}

// skipNotFound collapses a missing row to a nil error, so the getters can
// honour the EntityQuerier contract: (nil, nil) means "no such entity",
// (nil, err) means the read itself failed. A plugin that cannot tell those
// apart takes its empty-data branch during an outage.
func skipNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (a *pluginDBAdapter) GetNoteData(id uint) (map[string]any, error) {
	note, err := a.ctx.GetNote(id)
	if err != nil {
		return nil, skipNotFound(err)
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
		return nil, skipNotFound(err)
	}
	result := map[string]any{
		"id":                float64(resource.ID),
		"name":              resource.Name,
		"description":       resource.Description,
		"meta":              string(resource.Meta),
		"content_type":      resource.ContentType,
		"original_filename": resource.OriginalName,
		"hash":              resource.Hash,
		"width":             float64(resource.Width),
		"height":            float64(resource.Height),
		"file_size":         float64(resource.FileSize),
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
	if len(resource.Groups) > 0 {
		groups := make([]any, len(resource.Groups))
		for i, g := range resource.Groups {
			groups[i] = map[string]any{"id": float64(g.ID), "name": g.Name}
		}
		result["groups"] = groups
	}
	if len(resource.Notes) > 0 {
		notes := make([]any, len(resource.Notes))
		for i, n := range resource.Notes {
			notes[i] = map[string]any{"id": float64(n.ID), "name": n.Name}
		}
		result["notes"] = notes
	}
	return result, nil
}

func (a *pluginDBAdapter) GetGroupData(id uint) (map[string]any, error) {
	group, err := a.ctx.GetGroup(id)
	if err != nil {
		return nil, skipNotFound(err)
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
		return nil, skipNotFound(err)
	}
	return map[string]any{
		"id":   float64(tag.ID),
		"name": tag.Name,
	}, nil
}

func (a *pluginDBAdapter) GetCategoryData(id uint) (map[string]any, error) {
	cat, err := a.ctx.GetCategory(id)
	if err != nil {
		return nil, skipNotFound(err)
	}
	// The full shape, matching list_categories and the other taxonomy getters.
	// It used to return only id/name/description, so a plugin could enumerate
	// categories and read their templates but not fetch one by id and do the
	// same.
	return categoryToMap(cat), nil
}

func (a *pluginDBAdapter) GetNoteTypeData(id uint) (map[string]any, error) {
	nt, err := a.ctx.GetNoteType(id)
	if err != nil {
		return nil, skipNotFound(err)
	}
	return noteTypeToMap(nt), nil
}

func (a *pluginDBAdapter) GetResourceCategoryData(id uint) (map[string]any, error) {
	rc, err := a.ctx.GetResourceCategory(id)
	if err != nil {
		return nil, skipNotFound(err)
	}
	return resourceCategoryToMap(rc), nil
}

// --- Taxonomy listing ---
//
// Taxonomies have no owner and no scoping fields, so these take only
// {name=..., description=..., limit=..., offset=...}. They exist so a plugin
// can resolve a tag by name before creating it — the alternative was a
// hardcoded ID or a detour through MRQL.

func (a *pluginDBAdapter) ListTags(filter map[string]any) ([]map[string]any, error) {
	query := &query_models.TagQuery{
		Name:        getStringOpt(filter, "name"),
		Description: getStringOpt(filter, "description"),
	}
	if sortBy := getStringSliceOpt(filter, "sort_by"); len(sortBy) > 0 {
		query.SortBy = sortBy
	}
	tags, err := a.ctx.GetTags(queryOffset(filter), queryLimit(filter), query)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(tags))
	for i := range tags {
		results[i] = tagToMap(&tags[i])
	}
	return results, nil
}

func (a *pluginDBAdapter) ListCategories(filter map[string]any) ([]map[string]any, error) {
	query := &query_models.CategoryQuery{
		Name:        getStringOpt(filter, "name"),
		Description: getStringOpt(filter, "description"),
	}
	if sortBy := getStringSliceOpt(filter, "sort_by"); len(sortBy) > 0 {
		query.SortBy = sortBy
	}
	categories, err := a.ctx.GetCategories(queryOffset(filter), queryLimit(filter), query)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(categories))
	for i := range categories {
		results[i] = categoryToMap(&categories[i])
	}
	return results, nil
}

func (a *pluginDBAdapter) ListNoteTypes(filter map[string]any) ([]map[string]any, error) {
	query := &query_models.NoteTypeQuery{
		Name:        getStringOpt(filter, "name"),
		Description: getStringOpt(filter, "description"),
	}
	// GetNoteTypes takes the query first, unlike its siblings.
	noteTypes, err := a.ctx.GetNoteTypes(query, queryOffset(filter), queryLimit(filter))
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(noteTypes))
	for i := range noteTypes {
		results[i] = noteTypeToMap(&noteTypes[i])
	}
	return results, nil
}

func (a *pluginDBAdapter) ListResourceCategories(filter map[string]any) ([]map[string]any, error) {
	query := &query_models.ResourceCategoryQuery{
		Name:        getStringOpt(filter, "name"),
		Description: getStringOpt(filter, "description"),
	}
	categories, err := a.ctx.GetResourceCategories(queryOffset(filter), queryLimit(filter), query)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(categories))
	for i := range categories {
		results[i] = resourceCategoryToMap(&categories[i])
	}
	return results, nil
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

func buildNoteQuery(filter map[string]any) *query_models.NoteQuery {
	query := &query_models.NoteQuery{}
	if name, ok := filter["name"].(string); ok {
		query.Name = name
	}
	if oid := getUintOpt(filter, "owner_id"); oid > 0 {
		query.OwnerId = oid
	}
	if ntid := getUintOpt(filter, "note_type_id"); ntid > 0 {
		query.NoteTypeId = ntid
	}
	if tags := getUintSliceOpt(filter, "tags"); len(tags) > 0 {
		query.Tags = tags
	}
	if groups := getUintSliceOpt(filter, "groups"); len(groups) > 0 {
		query.Groups = groups
	}
	if sortBy := getStringSliceOpt(filter, "sort_by"); len(sortBy) > 0 {
		query.SortBy = sortBy
	}
	return query
}

func buildResourceQuery(filter map[string]any) *query_models.ResourceSearchQuery {
	query := &query_models.ResourceSearchQuery{}
	if name, ok := filter["name"].(string); ok {
		query.Name = name
	}
	if ct, ok := filter["content_type"].(string); ok {
		query.ContentType = ct
	}
	if oid := getUintOpt(filter, "owner_id"); oid > 0 {
		query.OwnerId = oid
	}
	if rcid := getUintOpt(filter, "resource_category_id"); rcid > 0 {
		query.ResourceCategoryId = rcid
	}
	if tags := getUintSliceOpt(filter, "tags"); len(tags) > 0 {
		query.Tags = tags
	}
	if groups := getUintSliceOpt(filter, "groups"); len(groups) > 0 {
		query.Groups = groups
	}
	if sortBy := getStringSliceOpt(filter, "sort_by"); len(sortBy) > 0 {
		query.SortBy = sortBy
	}
	return query
}

func buildGroupQuery(filter map[string]any) *query_models.GroupQuery {
	query := &query_models.GroupQuery{}
	if name, ok := filter["name"].(string); ok {
		query.Name = name
	}
	if oid := getUintOpt(filter, "owner_id"); oid > 0 {
		query.OwnerId = oid
	}
	if cid := getUintOpt(filter, "category_id"); cid > 0 {
		query.CategoryId = cid
	}
	if tags := getUintSliceOpt(filter, "tags"); len(tags) > 0 {
		query.Tags = tags
	}
	if sortBy := getStringSliceOpt(filter, "sort_by"); len(sortBy) > 0 {
		query.SortBy = sortBy
	}
	return query
}

func (a *pluginDBAdapter) QueryNotes(filter map[string]any) ([]map[string]any, error) {
	limit := queryLimit(filter)
	offset := queryOffset(filter)
	query := buildNoteQuery(filter)
	notes, err := a.ctx.GetNotes(offset, limit, query)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(notes))
	for i, n := range notes {
		m := map[string]any{
			"id":          float64(n.ID),
			"name":        n.Name,
			"description": n.Description,
			"meta":        string(n.Meta),
			"created_at":  n.CreatedAt.Format(time.RFC3339),
			"updated_at":  n.UpdatedAt.Format(time.RFC3339),
		}
		if n.OwnerId != nil {
			m["owner_id"] = float64(*n.OwnerId)
		}
		results[i] = m
	}
	return results, nil
}

func (a *pluginDBAdapter) QueryResources(filter map[string]any) ([]map[string]any, error) {
	limit := queryLimit(filter)
	offset := queryOffset(filter)
	query := buildResourceQuery(filter)
	resources, err := a.ctx.GetResources(offset, limit, query)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(resources))
	for i, r := range resources {
		m := map[string]any{
			"id":                float64(r.ID),
			"name":              r.Name,
			"description":       r.Description,
			"content_type":      r.ContentType,
			"meta":              string(r.Meta),
			"original_filename": r.OriginalName,
			"hash":              r.Hash,
			"created_at":        r.CreatedAt.Format(time.RFC3339),
			"updated_at":        r.UpdatedAt.Format(time.RFC3339),
		}
		if r.OwnerId != nil {
			m["owner_id"] = float64(*r.OwnerId)
		}
		results[i] = m
	}
	return results, nil
}

func (a *pluginDBAdapter) QueryGroups(filter map[string]any) ([]map[string]any, error) {
	limit := queryLimit(filter)
	offset := queryOffset(filter)
	query := buildGroupQuery(filter)
	groups, err := a.ctx.GetGroups(offset, limit, query)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(groups))
	for i, g := range groups {
		m := map[string]any{
			"id":          float64(g.ID),
			"name":        g.Name,
			"description": g.Description,
			"meta":        string(g.Meta),
			"created_at":  g.CreatedAt.Format(time.RFC3339),
			"updated_at":  g.UpdatedAt.Format(time.RFC3339),
		}
		if g.OwnerId != nil {
			m["owner_id"] = float64(*g.OwnerId)
		}
		results[i] = m
	}
	return results, nil
}

func (a *pluginDBAdapter) CountNotes(filter map[string]any) (int64, error) {
	query := buildNoteQuery(filter)
	return a.ctx.GetNoteCount(query)
}

func (a *pluginDBAdapter) CountResources(filter map[string]any) (int64, error) {
	query := buildResourceQuery(filter)
	return a.ctx.GetResourceCount(query)
}

func (a *pluginDBAdapter) CountGroups(filter map[string]any) (int64, error) {
	query := buildGroupQuery(filter)
	return a.ctx.GetGroupsCount(query)
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

func (a *pluginDBAdapter) AddResourceVersionFromURL(resourceID uint, rawURL string, comment string) (map[string]any, error) {
	lower := strings.ToLower(rawURL)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return nil, fmt.Errorf("unsupported URL scheme (only http and https are allowed)")
	}

	connectTimeout := a.ctx.Config.RemoteResourceConnectTimeout
	overallTimeout := a.ctx.Config.RemoteResourceOverallTimeout
	client := createRemoteResourceHTTPClient(connectTimeout, overallTimeout)
	// This fetch is only ever reached from a plugin, so a missing policy means
	// the invocation lost it on the way here rather than that none applies.
	// Refuse: an unpoliced server-side fetch is what the capability gate on
	// this function exists to prevent.
	if a.ctx.pluginEgress == nil {
		return nil, fmt.Errorf("refusing to fetch: this plugin's network policy is not available")
	}
	if err := plugin_system.CheckEgressURL(*a.ctx.pluginEgress, rawURL); err != nil {
		return nil, err
	}
	client = plugin_system.ApplyEgressPolicy(client, *a.ctx.pluginEgress, connectTimeout)

	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remote URL returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Derive filename from URL path
	parsed, _ := url.Parse(rawURL)
	filename := "version_upload"
	if parsed != nil && parsed.Path != "" {
		parts := strings.Split(parsed.Path, "/")
		if last := parts[len(parts)-1]; last != "" {
			filename = last
		}
	}

	file := &bytesMultipartFile{Reader: bytes.NewReader(data)}
	header := &multipart.FileHeader{
		Filename: filename,
		Size:     int64(len(data)),
	}

	version, err := a.ctx.UploadNewVersion(resourceID, file, header, comment)
	if err != nil {
		return nil, err
	}

	return versionToMap(version), nil
}

// bytesMultipartFile wraps bytes.Reader to satisfy multipart.File.
type bytesMultipartFile struct {
	*bytes.Reader
}

func (b *bytesMultipartFile) Close() error { return nil }

func versionToMap(v *models.ResourceVersion) map[string]any {
	return map[string]any{
		"id":             float64(v.ID),
		"resource_id":    float64(v.ResourceID),
		"version_number": float64(v.VersionNumber),
		"content_type":   v.ContentType,
		"file_size":      float64(v.FileSize),
		"hash":           v.Hash,
	}
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
//
// A fractional value is not read as its floor: Lua has one number type, so an
// id can arrive from a division, and truncating 2.9 to group 2 would move an
// entity under a group the caller never named. Unreadable means absent, which
// on a create or update is the same as omitting the key.
func getUintOpt(opts map[string]any, key string) uint {
	v, ok := opts[key].(float64)
	if !ok || v <= 0 || v != math.Trunc(v) || v > maxLuaExactInteger {
		return 0
	}
	return uint(v)
}

// maxLuaExactInteger is 2^53-1, the largest integer float64 represents
// unambiguously. Mirrors plugin_system's bound of the same name — 2^53+1
// arrives as 2^53, so 2^53 itself cannot be trusted to be the id that was sent.
const maxLuaExactInteger = 1<<53 - 1

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

// getStringSliceOpt extracts a []string from an options map.
// Handles both []any (proper arrays) and map[string]any (Lua tables with
// integer keys that luaTableToGoMap parses as maps).
func getStringSliceOpt(opts map[string]any, key string) []string {
	switch v := opts[key].(type) {
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case map[string]any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// --- Patch helpers: use current value when key is absent from opts ---

// patchString returns opts[key] if present, otherwise current.
// patchString returns the supplied string, or the entity's current one when the
// key is absent — or when the supplied value is not a string at all.
//
// getStringOpt answers "" for a value it cannot read, and on a patch that is
// not "leave it alone", it is "blank it": mah.db.patch_resource(id, {name =
// false}) silently replaced the resource's name with an empty string. An
// explicit "" still clears, because that is unambiguous.
func patchString(opts map[string]any, key, current string) string {
	raw, exists := opts[key]
	if !exists {
		return current
	}
	if _, ok := raw.(string); !ok {
		return current
	}
	return getStringOpt(opts, key)
}

// patchUint returns opts[key] if present, otherwise current.
// patchUint returns the supplied id, or the entity's current one when the key
// is absent — or when the supplied value cannot be read as an id. On a patch,
// falling back to 0 for an unreadable value would not mean "leave it alone", it
// would mean "clear the owner", so an unreadable value keeps what is stored.
func patchUint(opts map[string]any, key string, current uint) uint {
	raw, exists := opts[key]
	if !exists {
		return current
	}
	if _, ok := raw.(float64); !ok {
		return current
	}
	if parsed := getUintOpt(opts, key); parsed > 0 {
		return parsed
	}
	// An explicit 0 is a deliberate clear; a fractional or out-of-range number
	// is not, and must not be read as one.
	if v, _ := raw.(float64); v == 0 {
		return 0
	}
	return current
}

// patchUintSlice returns opts[key] if present, otherwise current.
// patchUintSlice returns the supplied ID list, or the entity's current one when
// the key is absent.
//
// A value of the wrong shape — a string where a list of ids belongs — keeps the
// current list rather than clearing it. getUintSliceOpt cannot tell "you asked
// for none" from "I could not read what you sent", and on a patch the two have
// opposite consequences: `patch_resource(id, {tags = "photos"})` used to
// succeed and strip every tag off the resource. An explicit empty list still
// clears, because that is unambiguous.
func patchUintSlice(opts map[string]any, key string, current []uint) []uint {
	raw, exists := opts[key]
	if !exists {
		return current
	}
	switch raw.(type) {
	case []any, map[string]any:
		return getUintSliceOpt(opts, key)
	default:
		return current
	}
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

func extractNoteIDs(notes []*models.Note) []uint {
	ids := make([]uint, len(notes))
	for i, n := range notes {
		ids[i] = n.ID
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
		"id":                 float64(c.ID),
		"name":               c.Name,
		"description":        c.Description,
		"custom_header":      c.CustomHeader,
		"custom_sidebar":     c.CustomSidebar,
		"custom_summary":     c.CustomSummary,
		"custom_avatar":      c.CustomAvatar,
		"custom_list_header": c.CustomListHeader,
		"custom_mrql_result": c.CustomMRQLResult,
		"custom_css":         c.CustomCSS,
		"meta_schema":        c.MetaSchema,
	}
}

func resourceCategoryToMap(rc *models.ResourceCategory) map[string]any {
	return map[string]any{
		"id":                 float64(rc.ID),
		"name":               rc.Name,
		"description":        rc.Description,
		"custom_header":      rc.CustomHeader,
		"custom_sidebar":     rc.CustomSidebar,
		"custom_summary":     rc.CustomSummary,
		"custom_avatar":      rc.CustomAvatar,
		"custom_list_header": rc.CustomListHeader,
		"custom_mrql_result": rc.CustomMRQLResult,
		"custom_css":         rc.CustomCSS,
		"meta_schema":        rc.MetaSchema,
		"auto_detect_rules":  rc.AutoDetectRules,
	}
}

func noteTypeToMap(nt *models.NoteType) map[string]any {
	return map[string]any{
		"id":                 float64(nt.ID),
		"name":               nt.Name,
		"description":        nt.Description,
		"custom_header":      nt.CustomHeader,
		"custom_sidebar":     nt.CustomSidebar,
		"custom_summary":     nt.CustomSummary,
		"custom_avatar":      nt.CustomAvatar,
		"custom_list_header": nt.CustomListHeader,
		"custom_mrql_result": nt.CustomMRQLResult,
		"custom_css":         nt.CustomCSS,
		"meta_schema":        nt.MetaSchema,
		"section_config":     string(nt.SectionConfig),
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
		startDate = note.StartDate.Format(constants.TimeFormat)
	}
	if note.EndDate != nil {
		endDate = note.EndDate.Format(constants.TimeFormat)
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
		Name:             getStringOpt(opts, "name"),
		Description:      getStringOpt(opts, "description"),
		CustomHeader:     getStringOpt(opts, "custom_header"),
		CustomSidebar:    getStringOpt(opts, "custom_sidebar"),
		CustomSummary:    getStringOpt(opts, "custom_summary"),
		CustomAvatar:     getStringOpt(opts, "custom_avatar"),
		CustomListHeader: getStringOpt(opts, "custom_list_header"),
		CustomMRQLResult: getStringOpt(opts, "custom_mrql_result"),
		CustomCSS:        getStringOpt(opts, "custom_css"),
		MetaSchema:       getStringOpt(opts, "meta_schema"),
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
			Name:             getStringOpt(opts, "name"),
			Description:      getStringOpt(opts, "description"),
			CustomHeader:     getStringOpt(opts, "custom_header"),
			CustomSidebar:    getStringOpt(opts, "custom_sidebar"),
			CustomSummary:    getStringOpt(opts, "custom_summary"),
			CustomAvatar:     getStringOpt(opts, "custom_avatar"),
			CustomListHeader: getStringOpt(opts, "custom_list_header"),
			CustomMRQLResult: getStringOpt(opts, "custom_mrql_result"),
			CustomCSS:        getStringOpt(opts, "custom_css"),
			MetaSchema:       getStringOpt(opts, "meta_schema"),
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
			Name:             patchString(opts, "name", cat.Name),
			Description:      patchString(opts, "description", cat.Description),
			CustomHeader:     patchString(opts, "custom_header", cat.CustomHeader),
			CustomSidebar:    patchString(opts, "custom_sidebar", cat.CustomSidebar),
			CustomSummary:    patchString(opts, "custom_summary", cat.CustomSummary),
			CustomAvatar:     patchString(opts, "custom_avatar", cat.CustomAvatar),
			CustomListHeader: patchString(opts, "custom_list_header", cat.CustomListHeader),
			CustomMRQLResult: patchString(opts, "custom_mrql_result", cat.CustomMRQLResult),
			CustomCSS:        patchString(opts, "custom_css", cat.CustomCSS),
			MetaSchema:       patchString(opts, "meta_schema", cat.MetaSchema),
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
		Name:             getStringOpt(opts, "name"),
		Description:      getStringOpt(opts, "description"),
		CustomHeader:     getStringOpt(opts, "custom_header"),
		CustomSidebar:    getStringOpt(opts, "custom_sidebar"),
		CustomSummary:    getStringOpt(opts, "custom_summary"),
		CustomAvatar:     getStringOpt(opts, "custom_avatar"),
		CustomListHeader: getStringOpt(opts, "custom_list_header"),
		CustomMRQLResult: getStringOpt(opts, "custom_mrql_result"),
		CustomCSS:        getStringOpt(opts, "custom_css"),
		MetaSchema:       getStringOpt(opts, "meta_schema"),
		AutoDetectRules:  getStringOpt(opts, "auto_detect_rules"),
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
			Name:             getStringOpt(opts, "name"),
			Description:      getStringOpt(opts, "description"),
			CustomHeader:     getStringOpt(opts, "custom_header"),
			CustomSidebar:    getStringOpt(opts, "custom_sidebar"),
			CustomSummary:    getStringOpt(opts, "custom_summary"),
			CustomAvatar:     getStringOpt(opts, "custom_avatar"),
			CustomListHeader: getStringOpt(opts, "custom_list_header"),
			CustomMRQLResult: getStringOpt(opts, "custom_mrql_result"),
			CustomCSS:        getStringOpt(opts, "custom_css"),
			MetaSchema:       getStringOpt(opts, "meta_schema"),
			AutoDetectRules:  getStringOpt(opts, "auto_detect_rules"),
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
			Name:             patchString(opts, "name", rc.Name),
			Description:      patchString(opts, "description", rc.Description),
			CustomHeader:     patchString(opts, "custom_header", rc.CustomHeader),
			CustomSidebar:    patchString(opts, "custom_sidebar", rc.CustomSidebar),
			CustomSummary:    patchString(opts, "custom_summary", rc.CustomSummary),
			CustomAvatar:     patchString(opts, "custom_avatar", rc.CustomAvatar),
			CustomListHeader: patchString(opts, "custom_list_header", rc.CustomListHeader),
			CustomMRQLResult: patchString(opts, "custom_mrql_result", rc.CustomMRQLResult),
			CustomCSS:        patchString(opts, "custom_css", rc.CustomCSS),
			MetaSchema:       patchString(opts, "meta_schema", rc.MetaSchema),
			AutoDetectRules:  patchString(opts, "auto_detect_rules", rc.AutoDetectRules),
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
		ID:               0, // ID=0 means create
		Name:             getStringOpt(opts, "name"),
		Description:      getStringOpt(opts, "description"),
		CustomHeader:     getStringOpt(opts, "custom_header"),
		CustomSidebar:    getStringOpt(opts, "custom_sidebar"),
		CustomSummary:    getStringOpt(opts, "custom_summary"),
		CustomAvatar:     getStringOpt(opts, "custom_avatar"),
		CustomListHeader: getStringOpt(opts, "custom_list_header"),
		CustomMRQLResult: getStringOpt(opts, "custom_mrql_result"),
		CustomCSS:        getStringOpt(opts, "custom_css"),
		MetaSchema:       getStringOpt(opts, "meta_schema"),
		SectionConfig:    getStringOpt(opts, "section_config"),
	}
	nt, err := a.ctx.CreateOrUpdateNoteType(editor)
	if err != nil {
		return nil, err
	}
	return noteTypeToMap(nt), nil
}

func (a *pluginDBAdapter) UpdateNoteType(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.NoteTypeEditor{
		ID:               id,
		Name:             getStringOpt(opts, "name"),
		Description:      getStringOpt(opts, "description"),
		CustomHeader:     getStringOpt(opts, "custom_header"),
		CustomSidebar:    getStringOpt(opts, "custom_sidebar"),
		CustomSummary:    getStringOpt(opts, "custom_summary"),
		CustomAvatar:     getStringOpt(opts, "custom_avatar"),
		CustomListHeader: getStringOpt(opts, "custom_list_header"),
		CustomMRQLResult: getStringOpt(opts, "custom_mrql_result"),
		CustomCSS:        getStringOpt(opts, "custom_css"),
		MetaSchema:       getStringOpt(opts, "meta_schema"),
		SectionConfig:    getStringOpt(opts, "section_config"),
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
		ID:               id,
		Name:             patchString(opts, "name", nt.Name),
		Description:      patchString(opts, "description", nt.Description),
		CustomHeader:     patchString(opts, "custom_header", nt.CustomHeader),
		CustomSidebar:    patchString(opts, "custom_sidebar", nt.CustomSidebar),
		CustomSummary:    patchString(opts, "custom_summary", nt.CustomSummary),
		CustomAvatar:     patchString(opts, "custom_avatar", nt.CustomAvatar),
		CustomListHeader: patchString(opts, "custom_list_header", nt.CustomListHeader),
		CustomMRQLResult: patchString(opts, "custom_mrql_result", nt.CustomMRQLResult),
		CustomCSS:        patchString(opts, "custom_css", nt.CustomCSS),
		MetaSchema:       patchString(opts, "meta_schema", nt.MetaSchema),
		SectionConfig:    patchString(opts, "section_config", string(nt.SectionConfig)),
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

	name := getStringOpt(opts, "name")
	description := getStringOpt(opts, "description")
	relation, err := a.ctx.AddRelation(fromGroupId, toGroupId, relationTypeId, name, description)
	if err != nil {
		return nil, err
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

// UpdateResource replaces every field, associations included — an omitted
// `tags` clears the resource's tags. That is the same contract the other
// Update* writers carry, and the reason PatchResource exists beside it.
//
// Exception: EditResource ignores an empty Meta, Width and Height rather than
// writing them (resource_crud_context.go), so those three survive an update
// that omits them, and cannot be cleared through this path either. That is the
// HTTP edit path's behaviour too; it is documented rather than special-cased
// here, because diverging would make the plugin writer and the form writer
// disagree about the same resource.
func (a *pluginDBAdapter) UpdateResource(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.ResourceEditor{
		ID: id,
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:               getStringOpt(opts, "name"),
			Description:        getStringOpt(opts, "description"),
			Meta:               getStringOpt(opts, "meta"),
			OwnerId:            getUintOpt(opts, "owner_id"),
			Groups:             getUintSliceOpt(opts, "groups"),
			Tags:               getUintSliceOpt(opts, "tags"),
			Notes:              getUintSliceOpt(opts, "notes"),
			Category:           getStringOpt(opts, "category"),
			ContentCategory:    getStringOpt(opts, "content_category"),
			ResourceCategoryId: getUintOpt(opts, "resource_category_id"),
			OriginalName:       getStringOpt(opts, "original_filename"),
			OriginalLocation:   getStringOpt(opts, "original_location"),
			Width:              getUintOpt(opts, "width"),
			Height:             getUintOpt(opts, "height"),
			SeriesId:           getUintOpt(opts, "series_id"),
			SeriesSlug:         getStringOpt(opts, "series_slug"),
		},
	}
	resource, err := a.ctx.EditResource(editor)
	if err != nil {
		return nil, err
	}
	return resourceToMap(resource), nil
}

// PatchResource changes only the keys present in opts. Everything else is read
// back from the stored resource and re-sent, because EditResource is a
// replace-all write.
func (a *pluginDBAdapter) PatchResource(id uint, opts map[string]any) (map[string]any, error) {
	current, err := a.ctx.GetResource(id)
	if err != nil {
		return nil, err
	}
	var seriesID uint
	if current.SeriesID != nil {
		seriesID = *current.SeriesID
	}
	editor := &query_models.ResourceEditor{
		ID: id,
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:               patchString(opts, "name", current.Name),
			Description:        patchString(opts, "description", current.Description),
			Meta:               patchString(opts, "meta", string(current.Meta)),
			OwnerId:            patchUint(opts, "owner_id", uintPtrVal(current.OwnerId)),
			Groups:             patchUintSlice(opts, "groups", extractGroupIDs(current.Groups)),
			Tags:               patchUintSlice(opts, "tags", extractTagIDs(current.Tags)),
			Notes:              patchUintSlice(opts, "notes", extractNoteIDs(current.Notes)),
			Category:           patchString(opts, "category", current.Category),
			ContentCategory:    patchString(opts, "content_category", current.ContentCategory),
			ResourceCategoryId: patchUint(opts, "resource_category_id", current.ResourceCategoryId),
			OriginalName:       patchString(opts, "original_filename", current.OriginalName),
			OriginalLocation:   patchString(opts, "original_location", current.OriginalLocation),
			Width:              patchUint(opts, "width", current.Width),
			Height:             patchUint(opts, "height", current.Height),
			SeriesId:           patchUint(opts, "series_id", seriesID),
		},
	}
	resource, err := a.ctx.EditResource(editor)
	if err != nil {
		return nil, err
	}
	return resourceToMap(resource), nil
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
	if meta, ok := options["meta"].(string); ok {
		base.Meta = meta
	}
}
