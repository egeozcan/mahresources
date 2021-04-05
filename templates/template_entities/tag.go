package template_entities

import (
	"mahresources/http_utils"
	"mahresources/models"
	"net/url"
	"strconv"
)

type TagsDisplay struct {
	Tags   *[]*DisplayedTag
	TagMap *map[uint]*DisplayedTag
}

type DisplayedTag struct {
	Name   string
	Link   string
	Active bool
}

func (td *TagsDisplay) GetTag(id uint) *DisplayedTag {
	return (*td.TagMap)[id]
}

func GenerateTagsSelection(selectedIds []uint, tags *[]models.Tag, reqUrl string, resetPage bool, tagsParam string) *TagsDisplay {
	displayedTags := make([]*DisplayedTag, 0, 10)
	tagMap := make(map[uint]*DisplayedTag)
	existingTags := make(map[uint]struct{})
	member := struct{}{}

	for _, tagId := range selectedIds {
		existingTags[tagId] = member
	}

	for _, tag := range *tags {
		_, active := existingTags[tag.ID]

		parsedBaseUrl, _ := url.Parse(reqUrl)
		q := parsedBaseUrl.Query()

		if resetPage {
			q.Del("page")
		}

		tagId := strconv.Itoa(int(tag.ID))

		if q.Get(tagsParam) == "" {
			q.Set(tagsParam, tagId)
		} else if !active {
			q[tagsParam] = append(q[tagsParam], tagId)
		} else {
			q[tagsParam] = http_utils.RemoveValue(q[tagsParam], tagId)
		}

		parsedBaseUrl.RawQuery = q.Encode()

		displayedTag := DisplayedTag{
			Name:   tag.Name,
			Link:   parsedBaseUrl.String(),
			Active: active,
		}

		displayedTags = append(displayedTags, &displayedTag)
		tagMap[tag.ID] = &displayedTag
	}

	return &TagsDisplay{
		Tags:   &displayedTags,
		TagMap: &tagMap,
	}
}
