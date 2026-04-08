package models

import (
	"encoding/json"
	"mahresources/models/types"
)

// CollapsibleState controls the collapsible behavior of a section.
type CollapsibleState string

const (
	CollapsibleDefault   CollapsibleState = "default"
	CollapsibleOpen      CollapsibleState = "open"
	CollapsibleCollapsed CollapsibleState = "collapsed"
	CollapsibleOff       CollapsibleState = "off"
)

// GroupSectionConfig controls which sections are visible on a group detail page.
type GroupSectionConfig struct {
	OwnEntities       GroupOwnEntitiesConfig     `json:"ownEntities"`
	RelatedEntities   GroupRelatedEntitiesConfig `json:"relatedEntities"`
	Relations         GroupRelationsConfig       `json:"relations"`
	Tags              bool                       `json:"tags"`
	MetaJson          bool                       `json:"metaJson"`
	Merge             bool                       `json:"merge"`
	Clone             bool                       `json:"clone"`
	TreeLink          bool                       `json:"treeLink"`
	Owner             bool                       `json:"owner"`
	Breadcrumb        bool                       `json:"breadcrumb"`
	Description       bool                       `json:"description"`
	MetaSchemaDisplay bool                       `json:"metaSchemaDisplay"`
}

type GroupOwnEntitiesConfig struct {
	State        string `json:"state"` // plain string so pongo2 comparisons work
	OwnNotes     bool   `json:"ownNotes"`
	OwnGroups    bool   `json:"ownGroups"`
	OwnResources bool   `json:"ownResources"`
}

type GroupRelatedEntitiesConfig struct {
	State            string `json:"state"` // plain string so pongo2 comparisons work
	RelatedGroups    bool   `json:"relatedGroups"`
	RelatedResources bool   `json:"relatedResources"`
	RelatedNotes     bool   `json:"relatedNotes"`
}

type GroupRelationsConfig struct {
	State            string `json:"state"` // plain string so pongo2 comparisons work
	ForwardRelations bool   `json:"forwardRelations"`
	ReverseRelations bool   `json:"reverseRelations"`
}

// ResourceSectionConfig controls which sections are visible on a resource detail page.
type ResourceSectionConfig struct {
	TechnicalDetails  ResourceTechnicalDetailsConfig `json:"technicalDetails"`
	MetadataGrid      bool                           `json:"metadataGrid"`
	Notes             bool                           `json:"notes"`
	Groups            bool                           `json:"groups"`
	Series            bool                           `json:"series"`
	SimilarResources  bool                           `json:"similarResources"`
	Versions          bool                           `json:"versions"`
	Tags              bool                           `json:"tags"`
	MetaJson          bool                           `json:"metaJson"`
	PreviewImage      bool                           `json:"previewImage"`
	ImageOperations   bool                           `json:"imageOperations"`
	CategoryLink      bool                           `json:"categoryLink"`
	FileSize          bool                           `json:"fileSize"`
	Owner             bool                           `json:"owner"`
	Breadcrumb        bool                           `json:"breadcrumb"`
	Description       bool                           `json:"description"`
	MetaSchemaDisplay bool                           `json:"metaSchemaDisplay"`
}

type ResourceTechnicalDetailsConfig struct {
	State string `json:"state"` // plain string so pongo2 comparisons work
}

// --- raw (pointer-based) intermediates for unmarshaling ---

type rawGroupSectionConfig struct {
	OwnEntities       *rawGroupOwnEntities     `json:"ownEntities"`
	RelatedEntities   *rawGroupRelatedEntities `json:"relatedEntities"`
	Relations         *rawGroupRelations       `json:"relations"`
	Tags              *bool                    `json:"tags"`
	MetaJson          *bool                    `json:"metaJson"`
	Merge             *bool                    `json:"merge"`
	Clone             *bool                    `json:"clone"`
	TreeLink          *bool                    `json:"treeLink"`
	Owner             *bool                    `json:"owner"`
	Breadcrumb        *bool                    `json:"breadcrumb"`
	Description       *bool                    `json:"description"`
	MetaSchemaDisplay *bool                    `json:"metaSchemaDisplay"`
}

type rawGroupOwnEntities struct {
	State        *CollapsibleState `json:"state"`
	OwnNotes     *bool             `json:"ownNotes"`
	OwnGroups    *bool             `json:"ownGroups"`
	OwnResources *bool             `json:"ownResources"`
}

type rawGroupRelatedEntities struct {
	State            *CollapsibleState `json:"state"`
	RelatedGroups    *bool             `json:"relatedGroups"`
	RelatedResources *bool             `json:"relatedResources"`
	RelatedNotes     *bool             `json:"relatedNotes"`
}

type rawGroupRelations struct {
	State            *CollapsibleState `json:"state"`
	ForwardRelations *bool             `json:"forwardRelations"`
	ReverseRelations *bool             `json:"reverseRelations"`
}

type rawResourceSectionConfig struct {
	TechnicalDetails  *rawResourceTechnicalDetails `json:"technicalDetails"`
	MetadataGrid      *bool                        `json:"metadataGrid"`
	Notes             *bool                        `json:"notes"`
	Groups            *bool                        `json:"groups"`
	Series            *bool                        `json:"series"`
	SimilarResources  *bool                        `json:"similarResources"`
	Versions          *bool                        `json:"versions"`
	Tags              *bool                        `json:"tags"`
	MetaJson          *bool                        `json:"metaJson"`
	PreviewImage      *bool                        `json:"previewImage"`
	ImageOperations   *bool                        `json:"imageOperations"`
	CategoryLink      *bool                        `json:"categoryLink"`
	FileSize          *bool                        `json:"fileSize"`
	Owner             *bool                        `json:"owner"`
	Breadcrumb        *bool                        `json:"breadcrumb"`
	Description       *bool                        `json:"description"`
	MetaSchemaDisplay *bool                        `json:"metaSchemaDisplay"`
}

type rawResourceTechnicalDetails struct {
	State *CollapsibleState `json:"state"`
}

// --- helpers ---

func boolDefault(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func stateDefault(p *CollapsibleState, def CollapsibleState) string {
	if p == nil || *p == "" {
		return string(def)
	}
	return string(*p)
}

// --- resolvers ---

// ResolveGroupSectionConfig parses JSON into a GroupSectionConfig, filling
// missing keys with defaults (bools default to true, states to "default").
func ResolveGroupSectionConfig(data *types.JSON) GroupSectionConfig {
	defaults := GroupSectionConfig{
		OwnEntities: GroupOwnEntitiesConfig{
			State: string(CollapsibleDefault), OwnNotes: true, OwnGroups: true, OwnResources: true,
		},
		RelatedEntities: GroupRelatedEntitiesConfig{
			State: string(CollapsibleDefault), RelatedGroups: true, RelatedResources: true, RelatedNotes: true,
		},
		Relations: GroupRelationsConfig{
			State: string(CollapsibleDefault), ForwardRelations: true, ReverseRelations: true,
		},
		Tags: true, MetaJson: true, Merge: true, Clone: true, TreeLink: true,
		Owner: true, Breadcrumb: true, Description: true, MetaSchemaDisplay: true,
	}

	if data == nil || len(*data) == 0 {
		return defaults
	}

	var raw rawGroupSectionConfig
	if err := json.Unmarshal([]byte(*data), &raw); err != nil {
		return defaults
	}

	cfg := GroupSectionConfig{
		Tags:              boolDefault(raw.Tags, true),
		MetaJson:          boolDefault(raw.MetaJson, true),
		Merge:             boolDefault(raw.Merge, true),
		Clone:             boolDefault(raw.Clone, true),
		TreeLink:          boolDefault(raw.TreeLink, true),
		Owner:             boolDefault(raw.Owner, true),
		Breadcrumb:        boolDefault(raw.Breadcrumb, true),
		Description:       boolDefault(raw.Description, true),
		MetaSchemaDisplay: boolDefault(raw.MetaSchemaDisplay, true),
	}

	if raw.OwnEntities != nil {
		cfg.OwnEntities = GroupOwnEntitiesConfig{
			State:        stateDefault(raw.OwnEntities.State, CollapsibleDefault),
			OwnNotes:     boolDefault(raw.OwnEntities.OwnNotes, true),
			OwnGroups:    boolDefault(raw.OwnEntities.OwnGroups, true),
			OwnResources: boolDefault(raw.OwnEntities.OwnResources, true),
		}
	} else {
		cfg.OwnEntities = defaults.OwnEntities
	}

	if raw.RelatedEntities != nil {
		cfg.RelatedEntities = GroupRelatedEntitiesConfig{
			State:            stateDefault(raw.RelatedEntities.State, CollapsibleDefault),
			RelatedGroups:    boolDefault(raw.RelatedEntities.RelatedGroups, true),
			RelatedResources: boolDefault(raw.RelatedEntities.RelatedResources, true),
			RelatedNotes:     boolDefault(raw.RelatedEntities.RelatedNotes, true),
		}
	} else {
		cfg.RelatedEntities = defaults.RelatedEntities
	}

	if raw.Relations != nil {
		cfg.Relations = GroupRelationsConfig{
			State:            stateDefault(raw.Relations.State, CollapsibleDefault),
			ForwardRelations: boolDefault(raw.Relations.ForwardRelations, true),
			ReverseRelations: boolDefault(raw.Relations.ReverseRelations, true),
		}
	} else {
		cfg.Relations = defaults.Relations
	}

	return cfg
}

// ResolveResourceSectionConfig parses JSON into a ResourceSectionConfig, filling
// missing keys with defaults (bools default to true, states to "default").
func ResolveResourceSectionConfig(data *types.JSON) ResourceSectionConfig {
	defaults := ResourceSectionConfig{
		TechnicalDetails:  ResourceTechnicalDetailsConfig{State: string(CollapsibleDefault)},
		MetadataGrid:      true,
		Notes:             true,
		Groups:            true,
		Series:            true,
		SimilarResources:  true,
		Versions:          true,
		Tags:              true,
		MetaJson:          true,
		PreviewImage:      true,
		ImageOperations:   true,
		CategoryLink:      true,
		FileSize:          true,
		Owner:             true,
		Breadcrumb:        true,
		Description:       true,
		MetaSchemaDisplay: true,
	}

	if data == nil || len(*data) == 0 {
		return defaults
	}

	var raw rawResourceSectionConfig
	if err := json.Unmarshal([]byte(*data), &raw); err != nil {
		return defaults
	}

	cfg := ResourceSectionConfig{
		MetadataGrid:      boolDefault(raw.MetadataGrid, true),
		Notes:             boolDefault(raw.Notes, true),
		Groups:            boolDefault(raw.Groups, true),
		Series:            boolDefault(raw.Series, true),
		SimilarResources:  boolDefault(raw.SimilarResources, true),
		Versions:          boolDefault(raw.Versions, true),
		Tags:              boolDefault(raw.Tags, true),
		MetaJson:          boolDefault(raw.MetaJson, true),
		PreviewImage:      boolDefault(raw.PreviewImage, true),
		ImageOperations:   boolDefault(raw.ImageOperations, true),
		CategoryLink:      boolDefault(raw.CategoryLink, true),
		FileSize:          boolDefault(raw.FileSize, true),
		Owner:             boolDefault(raw.Owner, true),
		Breadcrumb:        boolDefault(raw.Breadcrumb, true),
		Description:       boolDefault(raw.Description, true),
		MetaSchemaDisplay: boolDefault(raw.MetaSchemaDisplay, true),
	}

	if raw.TechnicalDetails != nil {
		cfg.TechnicalDetails = ResourceTechnicalDetailsConfig{
			State: stateDefault(raw.TechnicalDetails.State, CollapsibleDefault),
		}
	} else {
		cfg.TechnicalDetails = defaults.TechnicalDetails
	}

	return cfg
}
