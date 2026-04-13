package application_context

import "fmt"

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
	Shell                   bool             `json:"shell,omitempty"`
	OwnerRef                string           `json:"owner_ref,omitempty"`
	ResourceCount           int              `json:"resource_count,omitempty"`
	NoteCount               int              `json:"note_count,omitempty"`
	DescendantResourceCount int              `json:"descendant_resource_count,omitempty"`
	DescendantNoteCount     int              `json:"descendant_note_count,omitempty"`
	Children                []ImportPlanItem `json:"children,omitempty"`
	GUIDMatch               bool             `json:"guid_match,omitempty"`
	GUIDMatchID             uint             `json:"guid_match_id,omitempty"`
	GUIDMatchName           string           `json:"guid_match_name,omitempty"`
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
	DecisionKey      string               `json:"decision_key"`
	SourceKey        string               `json:"source_key"`
	SourceExportID   string               `json:"source_export_id,omitempty"`
	HasPayload       bool                 `json:"has_payload"`
	Suggestion       string               `json:"suggestion"`
	DestinationID    *uint                `json:"destination_id,omitempty"`
	DestinationName  string               `json:"destination_name,omitempty"`
	Ambiguous        bool                 `json:"ambiguous,omitempty"`
	Alternatives     []MappingAlternative `json:"alternatives,omitempty"`
	FromCategoryName string               `json:"from_category_name,omitempty"`
	ToCategoryName   string               `json:"to_category_name,omitempty"`
	GUIDConflict     bool                 `json:"guid_conflict,omitempty"`
	GUIDMatchID      uint                 `json:"guid_match_id,omitempty"`
	GUIDMatchName    string               `json:"guid_match_name,omitempty"`
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
	GUIDMatches         int `json:"guid_matches"`
}

// ImportDecisions holds all user decisions from the review screen.
type ImportDecisions struct {
	ParentGroupID            *uint                       `json:"parent_group_id,omitempty"`
	ResourceCollisionPolicy  string                      `json:"resource_collision_policy"`
	GUIDCollisionPolicy      string                      `json:"guid_collision_policy,omitempty"`
	AcknowledgeMissingHashes bool                        `json:"acknowledge_missing_hashes,omitempty"`
	MappingActions           map[string]MappingAction    `json:"mapping_actions"`
	DanglingActions          map[string]DanglingAction   `json:"dangling_actions"`
	ShellGroupActions        map[string]ShellGroupAction `json:"shell_group_actions,omitempty"`
	ExcludedItems            []string                    `json:"excluded_items"`
}

type MappingAction struct {
	Include       bool   `json:"include"`
	Action        string `json:"action"`
	DestinationID *uint  `json:"destination_id,omitempty"`
	RenameTo      string `json:"rename_to,omitempty"`
}

type DanglingAction struct {
	Action        string `json:"action"`
	DestinationID *uint  `json:"destination_id,omitempty"`
}

type ShellGroupAction struct {
	Action        string `json:"action"`                   // "create" or "map_to_existing"
	DestinationID *uint  `json:"destination_id,omitempty"` // required when Action = "map_to_existing"
}

// ImportApplyResult summarizes what the apply job did. Persisted as JSON to
// _imports/<jobId>.result.json so the UI/CLI can fetch it, and so partial-failure
// results list created IDs for manual cleanup (spec §9.5).
type ImportApplyResult struct {
	CreatedCategories         int      `json:"created_categories"`
	CreatedNoteTypes          int      `json:"created_note_types"`
	CreatedResourceCategories int      `json:"created_resource_categories"`
	CreatedTags               int      `json:"created_tags"`
	CreatedGRTs               int      `json:"created_grts"`
	CreatedSeries             int      `json:"created_series"`
	ReusedSeries              int      `json:"reused_series"`
	CreatedGroups             int      `json:"created_groups"`
	CreatedResources          int      `json:"created_resources"`
	SkippedByHash             int      `json:"skipped_by_hash"`
	SkippedMissingBytes       int      `json:"skipped_missing_bytes"`
	CreatedNotes              int      `json:"created_notes"`
	CreatedPreviews           int      `json:"created_previews"`
	CreatedVersions           int      `json:"created_versions"`
	CreatedShellGroups        int      `json:"created_shell_groups"`
	MappedShellGroups         int      `json:"mapped_shell_groups"`
	Warnings                  []string `json:"warnings"`

	// Created IDs per phase — enables manual cleanup after partial failure.
	CreatedGroupIDs    []uint `json:"created_group_ids,omitempty"`
	CreatedResourceIDs []uint `json:"created_resource_ids,omitempty"`
	CreatedNoteIDs     []uint `json:"created_note_ids,omitempty"`
}

func (p *ImportPlan) HasUnresolvedAmbiguities(decisions *ImportDecisions) bool {
	allMappings := make([]MappingEntry, 0)
	allMappings = append(allMappings, p.Mappings.Categories...)
	allMappings = append(allMappings, p.Mappings.NoteTypes...)
	allMappings = append(allMappings, p.Mappings.ResourceCategories...)
	allMappings = append(allMappings, p.Mappings.Tags...)
	allMappings = append(allMappings, p.Mappings.GroupRelationTypes...)

	for _, entry := range allMappings {
		if !entry.Ambiguous {
			continue
		}
		action, ok := decisions.MappingActions[entry.DecisionKey]
		if ok && !action.Include {
			continue
		}
		if !ok || action.Action == "" {
			return true
		}
		if action.Action == "map" && action.DestinationID == nil {
			return true
		}
	}
	return false
}

func (p *ImportPlan) ValidateForApply(decisions *ImportDecisions) error {
	if p.HasUnresolvedAmbiguities(decisions) {
		return fmt.Errorf("unresolved ambiguous mappings in review plan")
	}
	if p.ManifestOnlyMissingHashes > 0 && !decisions.AcknowledgeMissingHashes {
		return fmt.Errorf("missing-hash acknowledgement required: %d resources have no bytes", p.ManifestOnlyMissingHashes)
	}
	// Validate shell group decisions (skip excluded items — they won't be applied)
	excludedSet := make(map[string]bool, len(decisions.ExcludedItems))
	for _, id := range decisions.ExcludedItems {
		excludedSet[id] = true
	}
	for exportID, action := range decisions.ShellGroupActions {
		if excludedSet[exportID] {
			continue
		}
		if action.Action == "map_to_existing" && action.DestinationID == nil {
			return fmt.Errorf("shell group %s: map_to_existing requires a destination_id", exportID)
		}
	}
	return nil
}
