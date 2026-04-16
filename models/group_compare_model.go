package models

// CompareListItem is one normalized comparable item shown in group compare buckets.
type CompareListItem struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	URL           string `json:"url,omitempty"`
	SecondaryText string `json:"secondaryText,omitempty"`
}

// CompareListDiff groups normalized items into shared and side-specific buckets.
type CompareListDiff struct {
	Shared         []CompareListItem `json:"shared"`
	OnlyLeft       []CompareListItem `json:"onlyLeft"`
	OnlyRight      []CompareListItem `json:"onlyRight"`
	SharedCount    int               `json:"sharedCount"`
	OnlyLeftCount  int               `json:"onlyLeftCount"`
	OnlyRightCount int               `json:"onlyRightCount"`
	TotalCount     int               `json:"totalCount"`
	Same           bool              `json:"same"`
}

// GroupComparison holds structured side-by-side group comparison data.
type GroupComparison struct {
	Group1 *Group `json:"group1"`
	Group2 *Group `json:"group2"`

	CrossGroup bool `json:"crossGroup"`
	SameGroup  bool `json:"sameGroup"`

	Category1 string `json:"category1"`
	Category2 string `json:"category2"`
	Owner1    string `json:"owner1"`
	Owner2    string `json:"owner2"`
	URL1      string `json:"url1"`
	URL2      string `json:"url2"`

	SameName        bool `json:"sameName"`
	SameCategory    bool `json:"sameCategory"`
	SameOwner       bool `json:"sameOwner"`
	SameURL         bool `json:"sameURL"`
	SameCreatedAt   bool `json:"sameCreatedAt"`
	SameUpdatedAt   bool `json:"sameUpdatedAt"`
	SameDescription bool `json:"sameDescription"`
	SameMeta        bool `json:"sameMeta"`

	Tags             CompareListDiff `json:"tags"`
	OwnGroups        CompareListDiff `json:"ownGroups"`
	OwnNotes         CompareListDiff `json:"ownNotes"`
	OwnResources     CompareListDiff `json:"ownResources"`
	RelatedGroups    CompareListDiff `json:"relatedGroups"`
	RelatedNotes     CompareListDiff `json:"relatedNotes"`
	RelatedResources CompareListDiff `json:"relatedResources"`
	ForwardRelations CompareListDiff `json:"forwardRelations"`
	ReverseRelations CompareListDiff `json:"reverseRelations"`

	OwnEntitiesCount     int `json:"ownEntitiesCount"`
	RelatedEntitiesCount int `json:"relatedEntitiesCount"`
	RelationsCount       int `json:"relationsCount"`

	CoreDifferentCount    int  `json:"coreDifferentCount"`
	CoreSame              bool `json:"coreSame"`
	TagsSame              bool `json:"tagsSame"`
	OwnEntitiesSame       bool `json:"ownEntitiesSame"`
	RelatedEntitiesSame   bool `json:"relatedEntitiesSame"`
	RelationsSame         bool `json:"relationsSame"`
	HasDifferences        bool `json:"hasDifferences"`
	DifferentSectionCount int  `json:"differentSectionCount"`

	DescriptionLeftText  string `json:"descriptionLeftText"`
	DescriptionRightText string `json:"descriptionRightText"`
	MetaLeftText         string `json:"metaLeftText"`
	MetaRightText        string `json:"metaRightText"`
}

func (c *GroupComparison) Finalize() {
	if c == nil {
		return
	}

	c.TagsSame = c.Tags.Same
	c.OwnEntitiesCount = c.OwnGroups.TotalCount + c.OwnNotes.TotalCount + c.OwnResources.TotalCount
	c.RelatedEntitiesCount = c.RelatedGroups.TotalCount + c.RelatedNotes.TotalCount + c.RelatedResources.TotalCount
	c.RelationsCount = c.ForwardRelations.TotalCount + c.ReverseRelations.TotalCount

	c.OwnEntitiesSame = c.OwnGroups.Same && c.OwnNotes.Same && c.OwnResources.Same
	c.RelatedEntitiesSame = c.RelatedGroups.Same && c.RelatedNotes.Same && c.RelatedResources.Same
	c.RelationsSame = c.ForwardRelations.Same && c.ReverseRelations.Same

	coreSameFlags := []bool{
		c.SameName,
		c.SameCategory,
		c.SameOwner,
		c.SameURL,
		c.SameCreatedAt,
		c.SameUpdatedAt,
	}
	c.CoreDifferentCount = 0
	for _, same := range coreSameFlags {
		if !same {
			c.CoreDifferentCount++
		}
	}
	c.CoreSame = c.CoreDifferentCount == 0

	c.DifferentSectionCount = 0
	for _, same := range []bool{
		c.CoreSame,
		c.TagsSame,
		c.OwnEntitiesSame,
		c.RelatedEntitiesSame,
		c.RelationsSame,
		c.SameDescription,
		c.SameMeta,
	} {
		if !same {
			c.DifferentSectionCount++
		}
	}
	c.HasDifferences = c.DifferentSectionCount > 0
}
