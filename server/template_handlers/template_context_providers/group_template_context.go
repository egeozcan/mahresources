package template_context_providers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"encoding/json"
	"mahresources/server/interfaces"
	"mahresources/server/template_handlers/template_entities"
	"net/http"
	"strconv"
)

// FieldDefinition is used to parse the CustomFieldsDefinition from a Category
type FieldDefinition struct {
	Name    string                 `json:"name"`
	Label   string                 `json:"label"`
	Type    string                 `json:"type"`
	Options map[string]interface{} `json:"options"`
}

func GroupsListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.GroupQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		groups, err := context.GetGroups(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		groupsCount, err := context.GetGroupsCount(&query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), groupsCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		tags, err := context.GetTagsWithIds(&query.Tags, 0)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}
		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		notes, err := context.GetNotesWithIds(&query.Notes)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		resources, err := context.GetResourcesWithIds(&query.Resources)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		categories, err := context.GetCategoriesWithIds(&query.Categories, 0)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		groupsSelection, err := context.GetGroupsWithIds(&query.Groups)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		owners, err := context.GetGroupsWithIds(&[]uint{query.OwnerId})

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		tplCtx := pongo2.Context{
			"pageTitle":       "Groups",
			"groups":          groups,
			"owners":          owners,
			"groupsSelection": groupsSelection,
			"categories":      categories,
			"pagination":      pagination,
			"tags":            tags,
			"notes":           notes,
			"resources":       resources,
			"parsedQuery":     query,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/group/new",
			},
			"sortValues": createSortCols([]SortColumn{
				{Name: "Created", Value: "created_at"},
				{Name: "Name", Value: "name"},
				{Name: "Updated", Value: "updated_at"},
			}, query.SortBy),
			"displayOptions": getPathExtensionOptions(request.URL, &[]*SelectOption{
				{Title: "List", Link: "/groups"},
				{Title: "Text", Link: "/groups/text"},
			}),
		}

		// If filtered by a single category, parse its custom field definitions
		if len(query.Categories) == 1 && len(*categories) == 1 {
			selectedCategory := (*categories)[0]
			if len(selectedCategory.CustomFieldsDefinition) > 0 {
				var customFieldDefs []FieldDefinition
				if err := json.Unmarshal(selectedCategory.CustomFieldsDefinition, &customFieldDefs); err == nil {
					tplCtx["singleCategoryCustomFieldDefinitions"] = customFieldDefs
				} else {
					fmt.Println("Error parsing CustomFieldsDefinition for single category filter:", err)
				}
			}
		}

		return tplCtx.Update(baseContext)
	}
}

func GroupCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Add New Group",
		}.Update(staticTemplateCtx(request))

		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil || query.ID == 0 {
			var groupQuery query_models.GroupQuery
			err := decoder.Decode(&groupQuery, request.URL.Query())

			if err == nil {
				tplContext["group"] = groupQuery

				tags, _ := context.GetTagsWithIds(&groupQuery.Tags, 0)
				groups, _ := context.GetGroupsWithIds(&groupQuery.Groups)

				if groupQuery.CategoryId != 0 {
					category, _ := context.GetCategoriesWithIds(&[]uint{groupQuery.CategoryId}, 0)
					tplContext["category"] = category
				}

				if groupQuery.OwnerId != 0 {
					owner, _ := context.GetGroup(groupQuery.OwnerId)
					tplContext["owner"] = []*models.Group{owner}
				}

				tplContext["tags"] = tags
				tplContext["groups"] = groups

				// Handle Custom Fields for new group with category pre-selected
				if groupQuery.CategoryId != 0 {
					category, _ := context.GetCategory(groupQuery.CategoryId) // Assuming GetCategory fetches a single category
					if category != nil && len(category.CustomFieldsDefinition) > 0 {
						var customFieldDefs []FieldDefinition
						if err := json.Unmarshal(category.CustomFieldsDefinition, &customFieldDefs); err == nil {
							tplContext["customFieldDefinitions"] = customFieldDefs
						} else {
							fmt.Println("Error parsing CustomFieldsDefinition for new group:", err)
						}
						// For a new group, Meta would typically be empty or come from query defaults
						if groupQuery.Meta != nil {
							var metaMap map[string]interface{}
							if err := json.Unmarshal(groupQuery.Meta, &metaMap); err == nil {
								tplContext["meta"] = metaMap
							}
						}
					}
				}
			}

			return tplContext
		}

		group, err := context.GetGroup(query.ID)

		if err != nil {
			return addErrContext(err, tplContext)
		}

		tplContext["group"] = group
		tplContext["pageTitle"] = "Edit Group"
		tplContext["tags"] = &group.Tags
		tplContext["groups"] = &group.RelatedGroups

		if group.Owner != nil {
			tplContext["owner"] = []*models.Group{group.Owner}
		}

		// Handle Custom Fields for existing group
		if group.Category != nil && len(group.Category.CustomFieldsDefinition) > 0 {
			var customFieldDefs []FieldDefinition
			if err := json.Unmarshal(group.Category.CustomFieldsDefinition, &customFieldDefs); err == nil {
				tplContext["customFieldDefinitions"] = customFieldDefs
			} else {
				fmt.Println("Error parsing CustomFieldsDefinition for group:", group.ID, err)
			}
		}
		if group.Meta != nil {
			var metaMap map[string]interface{}
			// Assuming group.Meta is types.JSON which is effectively []byte
			if err := json.Unmarshal(group.Meta, &metaMap); err == nil {
				tplContext["meta"] = metaMap
			} else {
                 fmt.Println("Error parsing Meta for group:", group.ID, err)
            }
		}


		return tplContext
	}
}

func GroupContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return groupContextProviderImpl(context)
}

func groupContextProviderImpl(context interfaces.GroupReader) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		group, err := context.GetGroup(query.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		parents, err := context.FindParentsOfGroup(group.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		breadcrumbEls := make([]template_entities.Entry, len(*parents))

		for i, m := range *parents {
			breadcrumbEls[i] = template_entities.Entry{
				Name: m.Name,
				ID:   m.ID,
				Url:  fmt.Sprintf("/group?id=%v", m.ID),
			}
		}

		var prefix string

		if group.Category != nil {
			prefix = group.Category.Name
		} else {
			prefix = "Uncategorized"
		}

		ctxData := pongo2.Context{
			"pageTitle": group.GetName(),
			"prefix":    prefix,
			"group":     group,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/group/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  fmt.Sprintf("/v1/group/delete?Id=%v", group.ID),
			},
			"mainEntity":     group,
			"mainEntityType": "group",
			"breadcrumb": pongo2.Context{
				"HomeName": "Groups",
				"HomeUrl":  "groups",
				"Entries":  breadcrumbEls,
			},
		}

		// Add custom fields definitions and meta for display
		if group.Category != nil && len(group.Category.CustomFieldsDefinition) > 0 {
			var customFieldDefs []FieldDefinition
			if err := json.Unmarshal(group.Category.CustomFieldsDefinition, &customFieldDefs); err == nil {
				ctxData["customFieldDefinitions"] = customFieldDefs

				if group.Meta != nil {
					var metaMap map[string]interface{}
					if err := json.Unmarshal(group.Meta, &metaMap); err == nil {
						ctxData["meta"] = metaMap
					} else {
                        fmt.Println("Error parsing Meta for group display:", group.ID, err)
                    }
				}
			} else {
                fmt.Println("Error parsing CustomFieldsDefinition for group display:", group.ID, err)
            }
		}

		return ctxData.Update(baseContext)
	}
}
