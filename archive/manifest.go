package archive

import "time"

// Manifest is the always-first tar entry. Stream-parsing it tells the reader
// everything it needs to navigate the rest of the archive without reading
// every entity file.
type Manifest struct {
	SchemaVersion    int            `json:"schema_version"`
	CreatedAt        time.Time      `json:"created_at"`
	CreatedBy        string         `json:"created_by"`
	SourceInstanceID string         `json:"source_instance_id,omitempty"`
	ExportOptions    ExportOptions  `json:"export_options"`
	Roots            []string       `json:"roots"`
	Counts           Counts         `json:"counts"`
	Entries          Entries        `json:"entries"`
	SchemaDefs       SchemaDefIndex `json:"schema_defs"`
	Dangling         []DanglingRef  `json:"dangling_references"`
	Warnings         []string       `json:"warnings"`
}

type ExportOptions struct {
	Scope      ExportScope      `json:"scope"`
	Fidelity   ExportFidelity   `json:"fidelity"`
	SchemaDefs ExportSchemaDefs `json:"schema_defs"`
	Gzip       bool             `json:"gzip"`
}

type ExportScope struct {
	Subtree        bool `json:"subtree"`
	OwnedResources bool `json:"owned_resources"`
	OwnedNotes     bool `json:"owned_notes"`
	RelatedM2M     bool `json:"related_m2m"`
	GroupRelations bool `json:"group_relations"`
}

type ExportFidelity struct {
	ResourceBlobs    bool `json:"resource_blobs"`
	ResourceVersions bool `json:"resource_versions"`
	ResourcePreviews bool `json:"resource_previews"`
	ResourceSeries   bool `json:"resource_series"`
}

type ExportSchemaDefs struct {
	CategoriesAndTypes bool `json:"categories_and_types"`
	Tags               bool `json:"tags"`
	GroupRelationTypes bool `json:"group_relation_types"`
}

type Counts struct {
	Groups    int `json:"groups"`
	Notes     int `json:"notes"`
	Resources int `json:"resources"`
	Series    int `json:"series"`
	Blobs     int `json:"blobs"`
	Previews  int `json:"previews"`
	Versions  int `json:"versions"`
}

type Entries struct {
	Groups    []GroupEntry    `json:"groups"`
	Notes     []NoteEntry     `json:"notes"`
	Resources []ResourceEntry `json:"resources"`
	Series    []SeriesEntry   `json:"series"`
}

type GroupEntry struct {
	ExportID string `json:"export_id"`
	Name     string `json:"name"`
	SourceID uint   `json:"source_id"`
	Path     string `json:"path"`
}

type NoteEntry struct {
	ExportID string `json:"export_id"`
	Name     string `json:"name"`
	SourceID uint   `json:"source_id"`
	Owner    string `json:"owner"`
	Path     string `json:"path"`
}

type ResourceEntry struct {
	ExportID string `json:"export_id"`
	Name     string `json:"name"`
	SourceID uint   `json:"source_id"`
	Owner    string `json:"owner,omitempty"`
	Hash     string `json:"hash"`
	Path     string `json:"path"`
}

type SeriesEntry struct {
	ExportID string `json:"export_id"`
	Name     string `json:"name"`
	SourceID uint   `json:"source_id"`
	Path     string `json:"path"`
}

type SchemaDefIndex struct {
	Categories         []SchemaDefEntry `json:"categories"`
	NoteTypes          []SchemaDefEntry `json:"note_types"`
	ResourceCategories []SchemaDefEntry `json:"resource_categories"`
	Tags               []SchemaDefEntry `json:"tags"`
	GroupRelationTypes []SchemaDefEntry `json:"group_relation_types"`
}

type SchemaDefEntry struct {
	ExportID string `json:"export_id"`
	Name     string `json:"name"`
	SourceID uint   `json:"source_id"`
	Path     string `json:"path"`
}

type DanglingRef struct {
	ID               string       `json:"id"`
	Kind             string       `json:"kind"`
	From             string       `json:"from"`
	RelationTypeName string       `json:"relation_type_name,omitempty"`
	ToStub           DanglingStub `json:"to_stub"`
}

type DanglingStub struct {
	SourceID uint   `json:"source_id"`
	Name     string `json:"name"`
	Reason   string `json:"reason"`
}

// Dangling reference kinds.
const (
	DanglingKindRelatedGroup      = "related_group"
	DanglingKindRelatedResource   = "related_resource"
	DanglingKindRelatedNote       = "related_note"
	DanglingKindGroupRelation     = "group_relation"
	DanglingKindResourceSeriesSib = "resource_series_sibling"
)

// GroupPayload is the on-disk JSON shape for groups/<export_id>.json.
// Foreign keys are export IDs (g0001 etc.), not destination DB IDs.
type GroupPayload struct {
	ExportID         string                 `json:"export_id"`
	SourceID         uint                   `json:"source_id"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	URL              string                 `json:"url"`
	OwnerRef         string                 `json:"owner_ref,omitempty"`
	CategoryRef      string                 `json:"category_ref,omitempty"`
	CategoryName     string                 `json:"category_name,omitempty"`
	Tags             []TagRef               `json:"tags"`
	RelatedGroups    []string               `json:"related_groups"`
	RelatedResources []string               `json:"related_resources"`
	RelatedNotes     []string               `json:"related_notes"`
	Relationships    []GroupRelationPayload `json:"relationships"`
	Meta             map[string]any         `json:"meta"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// GroupRelationPayload is one row in Group.Relationships. Either ToRef (in
// scope) or DanglingRef (out of scope) is set, never both.
type GroupRelationPayload struct {
	TypeRef          string `json:"type_ref,omitempty"`
	TypeName         string `json:"type_name,omitempty"`
	FromCategoryName string `json:"from_category_name,omitempty"`
	ToCategoryName   string `json:"to_category_name,omitempty"`
	ToRef            string `json:"to_ref,omitempty"`
	DanglingRef      string `json:"dangling_ref,omitempty"`
	Name             string `json:"name"`
	Description      string `json:"description"`
}

// TagRef carries both the export-internal ref (when D2 is on) and the tag
// name (always present). One of them is enough to import; both makes the
// importer's life simpler.
type TagRef struct {
	Ref  string `json:"ref,omitempty"`
	Name string `json:"name"`
}

// NotePayload is the on-disk JSON shape for notes/<export_id>.json.
type NotePayload struct {
	ExportID     string             `json:"export_id"`
	SourceID     uint               `json:"source_id"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	OwnerRef     string             `json:"owner_ref,omitempty"`
	NoteTypeRef  string             `json:"note_type_ref,omitempty"`
	NoteTypeName string             `json:"note_type_name,omitempty"`
	Tags         []TagRef           `json:"tags"`
	Resources    []string           `json:"resources"`
	Groups       []string           `json:"groups"`
	StartDate    *time.Time         `json:"start_date,omitempty"`
	EndDate      *time.Time         `json:"end_date,omitempty"`
	Meta         map[string]any     `json:"meta"`
	Blocks       []NoteBlockPayload `json:"blocks"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

// NoteBlockPayload preserves position as a string (fractional indexing); the
// importer recreates blocks ordered by Position ASC.
type NoteBlockPayload struct {
	Type     string         `json:"type"`
	Position string         `json:"position"`
	Content  map[string]any `json:"content"`
	State    map[string]any `json:"state"`
}

// ResourcePayload is the on-disk JSON shape for resources/<export_id>.json.
type ResourcePayload struct {
	ExportID             string                   `json:"export_id"`
	SourceID             uint                     `json:"source_id"`
	Name                 string                   `json:"name"`
	OriginalName         string                   `json:"original_name"`
	OriginalLocation     string                   `json:"original_location"`
	Hash                 string                   `json:"hash"`
	HashType             string                   `json:"hash_type"`
	FileSize             int64                    `json:"file_size"`
	ContentType          string                   `json:"content_type"`
	ContentCategory      string                   `json:"content_category"`
	Width                uint                     `json:"width"`
	Height               uint                     `json:"height"`
	Description          string                   `json:"description"`
	Category             string                   `json:"category"`
	Meta                 map[string]any           `json:"meta"`
	OwnMeta              map[string]any           `json:"own_meta"`
	OwnerRef             string                   `json:"owner_ref,omitempty"`
	ResourceCategoryRef  string                   `json:"resource_category_ref,omitempty"`
	ResourceCategoryName string                   `json:"resource_category_name,omitempty"`
	Tags                 []TagRef                 `json:"tags"`
	Groups               []string                 `json:"groups"`
	Notes                []string                 `json:"notes"`
	BlobRef              string                   `json:"blob_ref,omitempty"`
	BlobMissing          bool                     `json:"blob_missing,omitempty"`
	SeriesRef            string                   `json:"series_ref,omitempty"`
	CurrentVersionRef    string                   `json:"current_version_ref,omitempty"`
	Versions             []ResourceVersionPayload `json:"versions,omitempty"`
	Previews             []PreviewPayload         `json:"previews,omitempty"`
	CreatedAt            time.Time                `json:"created_at"`
	UpdatedAt            time.Time                `json:"updated_at"`
}

type ResourceVersionPayload struct {
	VersionExportID string    `json:"version_export_id"`
	VersionNumber   int       `json:"version_number"`
	Hash            string    `json:"hash"`
	HashType        string    `json:"hash_type"`
	FileSize        int64     `json:"file_size"`
	ContentType     string    `json:"content_type"`
	Width           uint      `json:"width"`
	Height          uint      `json:"height"`
	Comment         string    `json:"comment"`
	BlobRef         string    `json:"blob_ref,omitempty"`
	BlobMissing     bool      `json:"blob_missing,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type PreviewPayload struct {
	PreviewExportID string `json:"preview_export_id"`
	Width           uint   `json:"width"`
	Height          uint   `json:"height"`
	ContentType     string `json:"content_type"`
}

// SeriesPayload — Series has Name + Slug + Meta. No Description field.
type SeriesPayload struct {
	ExportID string         `json:"export_id"`
	SourceID uint           `json:"source_id"`
	Name     string         `json:"name"`
	Slug     string         `json:"slug"`
	Meta     map[string]any `json:"meta"`
}

// CategoryDef / NoteTypeDef / ResourceCategoryDef share the same shape — all
// the Custom HTML fields plus MetaSchema and SectionConfig.
type CategoryDef struct {
	ExportID         string         `json:"export_id"`
	SourceID         uint           `json:"source_id"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	CustomHeader     string         `json:"custom_header"`
	CustomSidebar    string         `json:"custom_sidebar"`
	CustomSummary    string         `json:"custom_summary"`
	CustomAvatar     string         `json:"custom_avatar"`
	CustomMRQLResult string         `json:"custom_mrql_result"`
	MetaSchema       string         `json:"meta_schema"`
	SectionConfig    map[string]any `json:"section_config"`
}

// NoteTypeDef is structurally identical to CategoryDef but is exported as its
// own type so the importer's resolver branches by type.
type NoteTypeDef = CategoryDef

// ResourceCategoryDef adds AutoDetectRules.
type ResourceCategoryDef struct {
	CategoryDef
	AutoDetectRules string `json:"auto_detect_rules"`
}

type TagDef struct {
	ExportID    string         `json:"export_id"`
	SourceID    uint           `json:"source_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Meta        map[string]any `json:"meta"`
}

type GroupRelationTypeDef struct {
	ExportID         string `json:"export_id"`
	SourceID         uint   `json:"source_id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	FromCategoryRef  string `json:"from_category_ref,omitempty"`
	ToCategoryRef    string `json:"to_category_ref,omitempty"`
	FromCategoryName string `json:"from_category_name"`
	ToCategoryName   string `json:"to_category_name"`
	BackRelationRef  string `json:"back_relation_ref,omitempty"`
}
