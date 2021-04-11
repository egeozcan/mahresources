package template_entities

import (
	"mahresources/http_utils"
	"mahresources/models"
	"net/url"
	"strconv"
)

type NamedEntityRelationDisplay struct {
	SelectedRelations *[]*DisplayedRelation
	RelationsMap      *map[uint]*DisplayedRelation
}

type DisplayedRelation struct {
	Name   string
	Link   string
	Active bool
	ID     uint
}

func (td *NamedEntityRelationDisplay) GetRelation(id uint) *DisplayedRelation {
	return (*td.RelationsMap)[id]
}

func GenerateRelationsDisplay(selectedIds []uint, namedEntities *[]models.NamedEntity, reqUrl string, resetPage bool, urlParam string) *NamedEntityRelationDisplay {
	selectedRelations := make([]*DisplayedRelation, 0, 10)
	relationMap := make(map[uint]*DisplayedRelation)
	existingRelations := make(map[uint]struct{})
	member := struct{}{}

	for _, relationId := range selectedIds {
		existingRelations[relationId] = member
	}

	for _, entity := range *namedEntities {
		_, active := existingRelations[entity.GetId()]

		parsedBaseUrl, _ := url.Parse(reqUrl)
		q := parsedBaseUrl.Query()

		if resetPage {
			q.Del("page")
		}

		relatedId := strconv.Itoa(int(entity.GetId()))

		if q.Get(urlParam) == "" {
			q.Set(urlParam, relatedId)
		} else if !active {
			q[urlParam] = append(q[urlParam], relatedId)
		} else {
			q[urlParam] = http_utils.RemoveValue(q[urlParam], relatedId)
		}

		parsedBaseUrl.RawQuery = q.Encode()

		displayedRelation := DisplayedRelation{
			Name:   entity.GetName(),
			Link:   parsedBaseUrl.String(),
			Active: active,
			ID:     entity.GetId(),
		}

		if active {
			selectedRelations = append(selectedRelations, &displayedRelation)
		}

		relationMap[entity.GetId()] = &displayedRelation
	}

	return &NamedEntityRelationDisplay{
		SelectedRelations: &selectedRelations,
		RelationsMap:      &relationMap,
	}
}
