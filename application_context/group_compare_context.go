package application_context

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"mahresources/models"
	"mahresources/models/types"
)

func (ctx *MahresourcesContext) CompareGroupsCross(g1ID uint, g2ID uint) (*models.GroupComparison, error) {
	if g1ID == 0 {
		return nil, errors.New("group 1 ID (g1) is required")
	}
	if g2ID == 0 {
		g2ID = g1ID
	}

	group1, err := ctx.getGroupForCompare(g1ID)
	if err != nil {
		return nil, err
	}

	group2, err := ctx.getGroupForCompare(g2ID)
	if err != nil {
		return nil, err
	}

	descriptionLeft := normalizeCompareText(group1.Description)
	descriptionRight := normalizeCompareText(group2.Description)
	metaLeft := normalizeJSONText([]byte(group1.Meta))
	metaRight := normalizeJSONText([]byte(group2.Meta))

	comparison := &models.GroupComparison{
		Group1:               group1,
		Group2:               group2,
		CrossGroup:           g1ID != g2ID,
		SameGroup:            g1ID == g2ID,
		Category1:            compareCategoryName(group1.Category),
		Category2:            compareCategoryName(group2.Category),
		Owner1:               compareGroupName(group1.Owner),
		Owner2:               compareGroupName(group2.Owner),
		URL1:                 compareURL(group1.URL),
		URL2:                 compareURL(group2.URL),
		SameName:             group1.Name == group2.Name,
		SameCreatedAt:        group1.CreatedAt.Equal(group2.CreatedAt),
		SameUpdatedAt:        group1.UpdatedAt.Equal(group2.UpdatedAt),
		SameDescription:      descriptionLeft == descriptionRight,
		SameMeta:             metaLeft == metaRight,
		DescriptionLeftText:  descriptionLeft,
		DescriptionRightText: descriptionRight,
		MetaLeftText:         metaLeft,
		MetaRightText:        metaRight,
		Tags:                 compareDiff(normalizeTags(group1.Tags), normalizeTags(group2.Tags)),
		OwnGroups:            compareDiff(normalizeGroups(group1.OwnGroups), normalizeGroups(group2.OwnGroups)),
		OwnNotes:             compareDiff(normalizeNotes(group1.OwnNotes), normalizeNotes(group2.OwnNotes)),
		OwnResources:         compareDiff(normalizeResources(group1.OwnResources), normalizeResources(group2.OwnResources)),
		RelatedGroups:        compareDiff(normalizeGroups(group1.RelatedGroups), normalizeGroups(group2.RelatedGroups)),
		RelatedNotes:         compareDiff(normalizeNotes(group1.RelatedNotes), normalizeNotes(group2.RelatedNotes)),
		RelatedResources:     compareDiff(normalizeResources(group1.RelatedResources), normalizeResources(group2.RelatedResources)),
		ForwardRelations:     compareDiff(normalizeForwardRelations(group1.Relationships), normalizeForwardRelations(group2.Relationships)),
		ReverseRelations:     compareDiff(normalizeReverseRelations(group1.BackRelations), normalizeReverseRelations(group2.BackRelations)),
	}

	comparison.SameCategory = comparison.Category1 == comparison.Category2
	comparison.SameOwner = comparison.Owner1 == comparison.Owner2
	comparison.SameURL = comparison.URL1 == comparison.URL2
	comparison.Finalize()

	return comparison, nil
}

func (ctx *MahresourcesContext) getGroupForCompare(id uint) (*models.Group, error) {
	var group models.Group

	err := ctx.db.
		Preload("Tags").
		Preload("Owner").
		Preload("Category").
		Preload("OwnGroups").
		Preload("OwnGroups.Category").
		Preload("OwnNotes").
		Preload("OwnResources").
		Preload("RelatedGroups").
		Preload("RelatedGroups.Category").
		Preload("RelatedNotes").
		Preload("RelatedResources").
		Preload("Relationships").
		Preload("Relationships.ToGroup").
		Preload("Relationships.ToGroup.Category").
		Preload("Relationships.RelationType").
		Preload("BackRelations").
		Preload("BackRelations.FromGroup").
		Preload("BackRelations.FromGroup.Category").
		Preload("BackRelations.RelationType").
		First(&group, id).Error

	return &group, err
}

func compareDiff(left, right []models.CompareListItem) models.CompareListDiff {
	leftMap := make(map[string]models.CompareListItem, len(left))
	rightMap := make(map[string]models.CompareListItem, len(right))

	for _, item := range left {
		leftMap[item.Key] = item
	}
	for _, item := range right {
		rightMap[item.Key] = item
	}

	result := models.CompareListDiff{
		Shared:    make([]models.CompareListItem, 0),
		OnlyLeft:  make([]models.CompareListItem, 0),
		OnlyRight: make([]models.CompareListItem, 0),
	}

	for key, item := range leftMap {
		if _, exists := rightMap[key]; exists {
			result.Shared = append(result.Shared, item)
			continue
		}
		result.OnlyLeft = append(result.OnlyLeft, item)
	}

	for key, item := range rightMap {
		if _, exists := leftMap[key]; exists {
			continue
		}
		result.OnlyRight = append(result.OnlyRight, item)
	}

	sortCompareListItems(result.Shared)
	sortCompareListItems(result.OnlyLeft)
	sortCompareListItems(result.OnlyRight)

	result.SharedCount = len(result.Shared)
	result.OnlyLeftCount = len(result.OnlyLeft)
	result.OnlyRightCount = len(result.OnlyRight)
	result.TotalCount = result.SharedCount + result.OnlyLeftCount + result.OnlyRightCount
	result.Same = result.OnlyLeftCount == 0 && result.OnlyRightCount == 0

	return result
}

func sortCompareListItems(items []models.CompareListItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Label == items[j].Label {
			return items[i].Key < items[j].Key
		}
		return strings.ToLower(items[i].Label) < strings.ToLower(items[j].Label)
	})
}

func normalizeTags(tags []*models.Tag) []models.CompareListItem {
	items := make([]models.CompareListItem, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		items = append(items, models.CompareListItem{
			Key:   fmt.Sprintf("tag:%d", tag.ID),
			Label: tag.Name,
			URL:   fmt.Sprintf("/tag?id=%d", tag.ID),
		})
	}
	return items
}

func normalizeGroups(groups []*models.Group) []models.CompareListItem {
	items := make([]models.CompareListItem, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		items = append(items, models.CompareListItem{
			Key:           fmt.Sprintf("group:%d", group.ID),
			Label:         group.GetName(),
			URL:           fmt.Sprintf("/group?id=%d", group.ID),
			SecondaryText: compareCategoryName(group.Category),
		})
	}
	return items
}

func normalizeNotes(notes []*models.Note) []models.CompareListItem {
	items := make([]models.CompareListItem, 0, len(notes))
	for _, note := range notes {
		if note == nil {
			continue
		}
		items = append(items, models.CompareListItem{
			Key:   fmt.Sprintf("note:%d", note.ID),
			Label: note.GetName(),
			URL:   fmt.Sprintf("/note?id=%d", note.ID),
		})
	}
	return items
}

func normalizeResources(resources []*models.Resource) []models.CompareListItem {
	items := make([]models.CompareListItem, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		items = append(items, models.CompareListItem{
			Key:   fmt.Sprintf("resource:%d", resource.ID),
			Label: resource.GetName(),
			URL:   fmt.Sprintf("/resource?id=%d", resource.ID),
		})
	}
	return items
}

func normalizeForwardRelations(relations []*models.GroupRelation) []models.CompareListItem {
	items := make([]models.CompareListItem, 0, len(relations))
	for _, relation := range relations {
		if relation == nil || relation.ToGroupId == nil {
			continue
		}
		typeID := uint(0)
		if relation.RelationTypeId != nil {
			typeID = *relation.RelationTypeId
		}
		counterpart := compareGroupName(relation.ToGroup)
		typeName := compareRelationTypeName(relation)
		items = append(items, models.CompareListItem{
			Key:           fmt.Sprintf("forward:%d:%d", typeID, *relation.ToGroupId),
			Label:         formatRelationLabel(typeName, counterpart),
			URL:           fmt.Sprintf("/relation?id=%d", relation.ID),
			SecondaryText: compactRelationText(relation),
		})
	}
	return items
}

func normalizeReverseRelations(relations []*models.GroupRelation) []models.CompareListItem {
	items := make([]models.CompareListItem, 0, len(relations))
	for _, relation := range relations {
		if relation == nil || relation.FromGroupId == nil {
			continue
		}
		typeID := uint(0)
		if relation.RelationTypeId != nil {
			typeID = *relation.RelationTypeId
		}
		counterpart := compareGroupName(relation.FromGroup)
		typeName := compareRelationTypeName(relation)
		items = append(items, models.CompareListItem{
			Key:           fmt.Sprintf("reverse:%d:%d", typeID, *relation.FromGroupId),
			Label:         formatRelationLabel(typeName, counterpart),
			URL:           fmt.Sprintf("/relation?id=%d", relation.ID),
			SecondaryText: compactRelationText(relation),
		})
	}
	return items
}

func compareGroupName(group *models.Group) string {
	if group == nil {
		return "None"
	}
	name := strings.TrimSpace(group.GetName())
	if name == "" {
		return fmt.Sprintf("Group %d", group.ID)
	}
	return name
}

func compareCategoryName(category *models.Category) string {
	if category == nil {
		return "None"
	}
	name := strings.TrimSpace(category.Name)
	if name == "" {
		return fmt.Sprintf("Category %d", category.ID)
	}
	return name
}

func compareURL(u *types.URL) string {
	if u == nil {
		return ""
	}
	urlValue := url.URL(*u)
	return urlValue.String()
}

func compareRelationTypeName(relation *models.GroupRelation) string {
	if relation == nil || relation.RelationType == nil {
		return "Relation"
	}
	name := strings.TrimSpace(relation.RelationType.Name)
	if name == "" {
		return "Relation"
	}
	return name
}

func compactRelationText(relation *models.GroupRelation) string {
	if relation == nil {
		return ""
	}

	texts := make([]string, 0, 2)
	if trimmed := strings.TrimSpace(relation.Name); trimmed != "" {
		texts = append(texts, trimmed)
	}
	if trimmed := strings.TrimSpace(relation.Description); trimmed != "" {
		texts = append(texts, trimmed)
	}
	return strings.Join(texts, " - ")
}

func formatRelationLabel(typeName string, counterpart string) string {
	if counterpart == "" || counterpart == "None" {
		return typeName
	}
	return fmt.Sprintf("%s: %s", typeName, counterpart)
}

func normalizeCompareText(text string) string {
	return strings.TrimSpace(text)
}

func normalizeJSONText(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "{}"
	}

	var normalized any
	if err := json.Unmarshal(trimmed, &normalized); err != nil {
		return string(trimmed)
	}

	pretty, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return string(trimmed)
	}
	return string(pretty)
}
