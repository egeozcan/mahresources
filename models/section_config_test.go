package models

import (
	"mahresources/models/types"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveGroupSectionConfig_NilInput(t *testing.T) {
	cfg := ResolveGroupSectionConfig(nil)

	// All bools should default to true
	assert.True(t, cfg.Tags)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.Merge)
	assert.True(t, cfg.Clone)
	assert.True(t, cfg.TreeLink)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.Breadcrumb)
	assert.True(t, cfg.Description)
	assert.True(t, cfg.MetaSchemaDisplay)

	// Nested bools should default to true
	assert.True(t, cfg.OwnEntities.OwnNotes)
	assert.True(t, cfg.OwnEntities.OwnGroups)
	assert.True(t, cfg.OwnEntities.OwnResources)
	assert.True(t, cfg.RelatedEntities.RelatedGroups)
	assert.True(t, cfg.RelatedEntities.RelatedResources)
	assert.True(t, cfg.RelatedEntities.RelatedNotes)
	assert.True(t, cfg.Relations.ForwardRelations)
	assert.True(t, cfg.Relations.ReverseRelations)

	// All states should default to CollapsibleDefault
	assert.Equal(t, CollapsibleDefault, cfg.OwnEntities.State)
	assert.Equal(t, CollapsibleDefault, cfg.RelatedEntities.State)
	assert.Equal(t, CollapsibleDefault, cfg.Relations.State)
}

func TestResolveGroupSectionConfig_EmptyJSON(t *testing.T) {
	input := types.JSON(`{}`)
	cfg := ResolveGroupSectionConfig(&input)

	assert.True(t, cfg.Tags)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.Merge)
	assert.True(t, cfg.Clone)
	assert.True(t, cfg.TreeLink)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.Breadcrumb)
	assert.True(t, cfg.Description)
	assert.True(t, cfg.MetaSchemaDisplay)
	assert.True(t, cfg.OwnEntities.OwnNotes)
	assert.True(t, cfg.OwnEntities.OwnGroups)
	assert.True(t, cfg.OwnEntities.OwnResources)
	assert.True(t, cfg.RelatedEntities.RelatedGroups)
	assert.True(t, cfg.RelatedEntities.RelatedResources)
	assert.True(t, cfg.RelatedEntities.RelatedNotes)
	assert.True(t, cfg.Relations.ForwardRelations)
	assert.True(t, cfg.Relations.ReverseRelations)
	assert.Equal(t, CollapsibleDefault, cfg.OwnEntities.State)
	assert.Equal(t, CollapsibleDefault, cfg.RelatedEntities.State)
	assert.Equal(t, CollapsibleDefault, cfg.Relations.State)
}

func TestResolveGroupSectionConfig_PartialJSON(t *testing.T) {
	input := types.JSON(`{"tags": false, "ownEntities": {"state": "collapsed"}}`)
	cfg := ResolveGroupSectionConfig(&input)

	// Explicit false should be preserved
	assert.False(t, cfg.Tags)

	// Unset bools should default to true
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.Merge)
	assert.True(t, cfg.Clone)
	assert.True(t, cfg.TreeLink)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.Breadcrumb)
	assert.True(t, cfg.Description)
	assert.True(t, cfg.MetaSchemaDisplay)

	// Explicit state should be preserved
	assert.Equal(t, CollapsibleCollapsed, cfg.OwnEntities.State)

	// Unset nested bools should default to true
	assert.True(t, cfg.OwnEntities.OwnNotes)
	assert.True(t, cfg.OwnEntities.OwnGroups)
	assert.True(t, cfg.OwnEntities.OwnResources)

	// Unset sections should get all defaults
	assert.Equal(t, CollapsibleDefault, cfg.RelatedEntities.State)
	assert.True(t, cfg.RelatedEntities.RelatedGroups)
	assert.Equal(t, CollapsibleDefault, cfg.Relations.State)
	assert.True(t, cfg.Relations.ForwardRelations)
}

func TestResolveGroupSectionConfig_CompleteJSON(t *testing.T) {
	input := types.JSON(`{
		"tags": false,
		"metaJson": false,
		"merge": false,
		"clone": false,
		"treeLink": false,
		"owner": false,
		"breadcrumb": false,
		"description": false,
		"metaSchemaDisplay": false,
		"ownEntities": {
			"state": "open",
			"ownNotes": false,
			"ownGroups": false,
			"ownResources": false
		},
		"relatedEntities": {
			"state": "collapsed",
			"relatedGroups": false,
			"relatedResources": false,
			"relatedNotes": false
		},
		"relations": {
			"state": "off",
			"forwardRelations": false,
			"reverseRelations": false
		}
	}`)
	cfg := ResolveGroupSectionConfig(&input)

	assert.False(t, cfg.Tags)
	assert.False(t, cfg.MetaJson)
	assert.False(t, cfg.Merge)
	assert.False(t, cfg.Clone)
	assert.False(t, cfg.TreeLink)
	assert.False(t, cfg.Owner)
	assert.False(t, cfg.Breadcrumb)
	assert.False(t, cfg.Description)
	assert.False(t, cfg.MetaSchemaDisplay)

	assert.Equal(t, CollapsibleOpen, cfg.OwnEntities.State)
	assert.False(t, cfg.OwnEntities.OwnNotes)
	assert.False(t, cfg.OwnEntities.OwnGroups)
	assert.False(t, cfg.OwnEntities.OwnResources)

	assert.Equal(t, CollapsibleCollapsed, cfg.RelatedEntities.State)
	assert.False(t, cfg.RelatedEntities.RelatedGroups)
	assert.False(t, cfg.RelatedEntities.RelatedResources)
	assert.False(t, cfg.RelatedEntities.RelatedNotes)

	assert.Equal(t, CollapsibleOff, cfg.Relations.State)
	assert.False(t, cfg.Relations.ForwardRelations)
	assert.False(t, cfg.Relations.ReverseRelations)
}

func TestResolveGroupSectionConfig_InvalidJSON(t *testing.T) {
	input := types.JSON(`not valid json at all`)
	cfg := ResolveGroupSectionConfig(&input)

	// Should return all defaults on invalid JSON
	assert.True(t, cfg.Tags)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.Merge)
	assert.True(t, cfg.Clone)
	assert.True(t, cfg.TreeLink)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.Breadcrumb)
	assert.True(t, cfg.Description)
	assert.True(t, cfg.MetaSchemaDisplay)
	assert.Equal(t, CollapsibleDefault, cfg.OwnEntities.State)
	assert.True(t, cfg.OwnEntities.OwnNotes)
	assert.Equal(t, CollapsibleDefault, cfg.RelatedEntities.State)
	assert.Equal(t, CollapsibleDefault, cfg.Relations.State)
}

func TestResolveResourceSectionConfig_NilInput(t *testing.T) {
	cfg := ResolveResourceSectionConfig(nil)

	// All bools should default to true
	assert.True(t, cfg.MetadataGrid)
	assert.True(t, cfg.Notes)
	assert.True(t, cfg.Groups)
	assert.True(t, cfg.Series)
	assert.True(t, cfg.SimilarResources)
	assert.True(t, cfg.Versions)
	assert.True(t, cfg.Tags)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.PreviewImage)
	assert.True(t, cfg.ImageOperations)
	assert.True(t, cfg.CategoryLink)
	assert.True(t, cfg.FileSize)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.Breadcrumb)
	assert.True(t, cfg.Description)
	assert.True(t, cfg.MetaSchemaDisplay)

	// State should default
	assert.Equal(t, CollapsibleDefault, cfg.TechnicalDetails.State)
}

func TestResolveResourceSectionConfig_EmptyJSON(t *testing.T) {
	input := types.JSON(`{}`)
	cfg := ResolveResourceSectionConfig(&input)

	assert.True(t, cfg.MetadataGrid)
	assert.True(t, cfg.Notes)
	assert.True(t, cfg.Groups)
	assert.True(t, cfg.Series)
	assert.True(t, cfg.SimilarResources)
	assert.True(t, cfg.Versions)
	assert.True(t, cfg.Tags)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.PreviewImage)
	assert.True(t, cfg.ImageOperations)
	assert.True(t, cfg.CategoryLink)
	assert.True(t, cfg.FileSize)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.Breadcrumb)
	assert.True(t, cfg.Description)
	assert.True(t, cfg.MetaSchemaDisplay)
	assert.Equal(t, CollapsibleDefault, cfg.TechnicalDetails.State)
}

func TestResolveResourceSectionConfig_PartialJSON(t *testing.T) {
	input := types.JSON(`{"notes": false, "technicalDetails": {"state": "open"}}`)
	cfg := ResolveResourceSectionConfig(&input)

	// Explicit false preserved
	assert.False(t, cfg.Notes)

	// Unset bools default true
	assert.True(t, cfg.MetadataGrid)
	assert.True(t, cfg.Groups)
	assert.True(t, cfg.Series)
	assert.True(t, cfg.SimilarResources)
	assert.True(t, cfg.Versions)
	assert.True(t, cfg.Tags)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.PreviewImage)
	assert.True(t, cfg.ImageOperations)
	assert.True(t, cfg.CategoryLink)
	assert.True(t, cfg.FileSize)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.Breadcrumb)
	assert.True(t, cfg.Description)
	assert.True(t, cfg.MetaSchemaDisplay)

	// Explicit state preserved
	assert.Equal(t, CollapsibleOpen, cfg.TechnicalDetails.State)
}

func TestResolveResourceSectionConfig_CompleteJSON(t *testing.T) {
	input := types.JSON(`{
		"metadataGrid": false,
		"notes": false,
		"groups": false,
		"series": false,
		"similarResources": false,
		"versions": false,
		"tags": false,
		"metaJson": false,
		"previewImage": false,
		"imageOperations": false,
		"categoryLink": false,
		"fileSize": false,
		"owner": false,
		"breadcrumb": false,
		"description": false,
		"metaSchemaDisplay": false,
		"technicalDetails": {
			"state": "collapsed"
		}
	}`)
	cfg := ResolveResourceSectionConfig(&input)

	assert.False(t, cfg.MetadataGrid)
	assert.False(t, cfg.Notes)
	assert.False(t, cfg.Groups)
	assert.False(t, cfg.Series)
	assert.False(t, cfg.SimilarResources)
	assert.False(t, cfg.Versions)
	assert.False(t, cfg.Tags)
	assert.False(t, cfg.MetaJson)
	assert.False(t, cfg.PreviewImage)
	assert.False(t, cfg.ImageOperations)
	assert.False(t, cfg.CategoryLink)
	assert.False(t, cfg.FileSize)
	assert.False(t, cfg.Owner)
	assert.False(t, cfg.Breadcrumb)
	assert.False(t, cfg.Description)
	assert.False(t, cfg.MetaSchemaDisplay)
	assert.Equal(t, CollapsibleCollapsed, cfg.TechnicalDetails.State)
}

func TestResolveResourceSectionConfig_InvalidJSON(t *testing.T) {
	input := types.JSON(`{{{invalid`)
	cfg := ResolveResourceSectionConfig(&input)

	// Should return all defaults
	assert.True(t, cfg.MetadataGrid)
	assert.True(t, cfg.Notes)
	assert.True(t, cfg.Groups)
	assert.True(t, cfg.Series)
	assert.True(t, cfg.SimilarResources)
	assert.True(t, cfg.Versions)
	assert.True(t, cfg.Tags)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.PreviewImage)
	assert.True(t, cfg.ImageOperations)
	assert.True(t, cfg.CategoryLink)
	assert.True(t, cfg.FileSize)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.Breadcrumb)
	assert.True(t, cfg.Description)
	assert.True(t, cfg.MetaSchemaDisplay)
	assert.Equal(t, CollapsibleDefault, cfg.TechnicalDetails.State)
}

func TestResolveGroupSectionConfig_EmptyStateString(t *testing.T) {
	input := types.JSON(`{"ownEntities": {"state": ""}}`)
	cfg := ResolveGroupSectionConfig(&input)
	if cfg.OwnEntities.State != CollapsibleDefault {
		t.Errorf("Empty state string should default, got %q", cfg.OwnEntities.State)
	}
}
