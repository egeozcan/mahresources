package application_context

// ImportPlan is the parsed result of an import tar. Persisted as JSON to
// _imports/<jobId>.plan.json and served via GET /v1/imports/{jobId}/plan.
type ImportPlan struct {
	JobID            string           `json:"job_id"`
	SchemaVersion    int              `json:"schema_version"`
	SourceInstanceID string           `json:"source_instance_id,omitempty"`
	Counts           ImportPlanCounts `json:"counts"`
	Items            []ImportPlanItem `json:"items"`
	Mappings         ImportMappings   `json:"mappings"`
	SeriesInfo       []SeriesMapping  `json:"series_info"`
	DanglingRefs     []DanglingRefPlan `json:"dangling_refs"`
	Conflicts        ConflictSummary  `json:"conflicts"`
	ManifestOnlyMissingHashes int     `json:"manifest_only_missing_hashes"`
	Warnings         []string         `json:"warnings"`
}

type ImportPlanCounts struct {
	Groups    int `json:"groups"`
	Notes     int `json:"notes"`
	Resources int `json:"resources"`
	Series    int `json:"series"`
	Blobs     int `json:"blobs"`
	Previews  int `json:"previews"`
	Versions  int `json:"versions"`
}

// ImportPlanItem is one node in the hierarchical item tree. Groups form
// the tree structure via OwnerRef; resources and notes are leaf counts.
// DescendantResourceCount / DescendantNoteCount include this node's own
// counts plus all descendant subtree counts, enabling roll-up display.
type ImportPlanItem struct {
	ExportID                string           `json:"export_id"`
	Kind                    string           `json:"kind"`
	Name                    string           `json:"name"`
	OwnerRef                string           `json:"owner_ref,omitempty"`
	ResourceCount           int              `json:"resource_count,omitempty"`
	NoteCount               int              `json:"note_count,omitempty"`
	DescendantResourceCount int              `json:"descendant_resource_count,omitempty"`
	DescendantNoteCount     int              `json:"descendant_note_count,omitempty"`
	Children                []ImportPlanItem `json:"children,omitempty"`
}

type ImportMappings struct {
	Categories         []MappingEntry `json:"categories"`
	NoteTypes          []MappingEntry `json:"note_types"`
	ResourceCategories []MappingEntry `json:"resource_categories"`
	Tags               []MappingEntry `json:"tags"`
	GroupRelationTypes []MappingEntry `json:"group_relation_types"`
}

// MappingEntry represents one schema-def reference that needs resolution.
//
// DecisionKey is the globally unique key the UI uses for decisions, prefixed
// with a type discriminator so same-named entries across different mapping
// types never collide. Format: "<type>:<identity>".
type MappingEntry struct {
	DecisionKey     string `json:"decision_key"`
	SourceKey       string `json:"source_key"`
	SourceExportID  string `json:"source_export_id,omitempty"`
	HasPayload      bool   `json:"has_payload"`
	Suggestion      string `json:"suggestion"`
	DestinationID   *uint  `json:"destination_id,omitempty"`
	DestinationName string `json:"destination_name,omitempty"`
	Ambiguous       bool   `json:"ambiguous,omitempty"`
	Alternatives    []MappingAlternative `json:"alternatives,omitempty"`
	FromCategoryName string `json:"from_category_name,omitempty"`
	ToCategoryName   string `json:"to_category_name,omitempty"`
}

type MappingAlternative struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// DecisionKeyFor returns the globally-unique key the UI uses to store this
// entry's decisions. Prefixed with type discriminator.
func DecisionKeyFor(typeName string, entry MappingEntry) string {
	if entry.FromCategoryName != "" || entry.ToCategoryName != "" {
		return typeName + ":" + entry.SourceKey + "|" + entry.FromCategoryName + "|" + entry.ToCategoryName
	}
	if entry.SourceExportID != "" {
		return typeName + ":" + entry.SourceExportID
	}
	return typeName + ":" + entry.SourceKey
}

type SeriesMapping struct {
	ExportID string `json:"export_id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Action   string `json:"action"`
	DestID   *uint  `json:"dest_id,omitempty"`
	DestName string `json:"dest_name,omitempty"`
}

type DanglingRefPlan struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	FromExportID     string `json:"from_export_id"`
	FromName         string `json:"from_name"`
	StubSourceID     uint   `json:"stub_source_id"`
	StubName         string `json:"stub_name"`
	RelationTypeName string `json:"relation_type_name,omitempty"`
}

type ConflictSummary struct {
	ResourceHashMatches int `json:"resource_hash_matches"`
}

// ImportDecisions holds all user decisions from the review screen.
type ImportDecisions struct {
	ParentGroupID            *uint                    `json:"parent_group_id,omitempty"`
	ResourceCollisionPolicy  string                   `json:"resource_collision_policy"`
	AcknowledgeMissingHashes bool                     `json:"acknowledge_missing_hashes,omitempty"`
	MappingActions           map[string]MappingAction  `json:"mapping_actions"`
	DanglingActions          map[string]DanglingAction `json:"dangling_actions"`
	ExcludedItems            []string                 `json:"excluded_items"`
}

type MappingAction struct {
	Include       bool   `json:"include"`
	Action        string `json:"action"`
	DestinationID *uint  `json:"destination_id,omitempty"`
}

type DanglingAction struct {
	Action        string `json:"action"`
	DestinationID *uint  `json:"destination_id,omitempty"`
}
