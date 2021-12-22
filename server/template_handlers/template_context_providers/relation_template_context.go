package template_context_providers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
	"net/http"
)

func RelationTypeListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.RelationshipTypeQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		fromCategories, err := context.GetCategoriesWithIds(&[]uint{query.FromCategory}, 0)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		toCategories, err := context.GetCategoriesWithIds(&[]uint{query.ToCategory}, 0)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		relationTypes, err := context.GetRelationTypes(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		relationTypesCount, err := context.GetRelationTypesCount(&query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), relationTypesCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":      "Relation Types",
			"relationTypes":  relationTypes,
			"pagination":     pagination,
			"fromCategories": fromCategories,
			"toCategories":   toCategories,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/relationType/new",
			},
		}.Update(baseContext)
	}
}

func RelationTypeContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		relationType, err := context.GetRelationType(query.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":    "Relation " + relationType.Name,
			"relationType": relationType,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  fmt.Sprintf("/relationType/edit?id=%v", relationType.ID),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  fmt.Sprintf("/v1/relationType/delete?Id=%v", relationType.ID),
			},
			"mainEntity": relationType,
		}.Update(baseContext)
	}
}

func RelationTypeCreateContextProvider(_ *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Relation Type",
		}.Update(staticTemplateCtx(request))

		return tplContext
	}
}

func RelationTypeEditContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		relationType, err := context.GetRelationType(query.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		tplContext := pongo2.Context{
			"pageTitle":    "Edit Relation Type",
			"relationType": relationType,
		}.Update(baseContext)

		return tplContext
	}
}

func RelationListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.GroupRelationshipQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		fromTypes, err := context.GetRelationTypesWithIds(&[]uint{query.GroupRelationTypeId})

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		toGroups, err := context.GetGroupsWithIds(&[]uint{query.ToGroupId})

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		fromGroups, err := context.GetGroupsWithIds(&[]uint{query.FromGroupId})

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		relations, err := context.GetRelations(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		relationTypesCount, err := context.GetRelationsCount(&query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), relationTypesCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":  "Relations",
			"pagination": pagination,
			"relations":  relations,
			"fromTypes":  fromTypes,
			"fromGroups": fromGroups,
			"toGroups":   toGroups,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/relation/new",
			},
		}.Update(baseContext)
	}
}

func RelationContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		relation, err := context.GetRelation(query.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		groupFrom, err := context.GetGroup(*relation.FromGroupId)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		groupTo, err := context.GetGroup(*relation.ToGroupId)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		link := fmt.Sprintf("/v1/relation/delete?Id=%v", relation.ID)
		pageTitle := fmt.Sprintf("Relation from %v to %v", relation.FromGroup.Name, relation.ToGroup.Name)

		return pongo2.Context{
			"pageTitle": pageTitle,
			"relation":  relation,
			"groupFrom": groupFrom,
			"groupTo":   groupTo,
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  link,
			},
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  fmt.Sprintf("/relation/edit?id=%v", relation.ID),
			},
			"mainEntity": relation,
		}.Update(baseContext)
	}
}

func RelationCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)
		tplContext := pongo2.Context{
			"pageTitle": "Create Relation",
		}.Update(baseContext)

		var query query_models.GroupRelationshipQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		if query.GroupRelationTypeId != 0 {
			ids := []uint{query.GroupRelationTypeId}
			relationType, err := context.GetRelationTypesWithIds(&ids)

			if err != nil {
				fmt.Println(err)

				return addErrContext(err, baseContext)
			}

			tplContext["relationType"] = relationType
		}

		if query.FromGroupId != 0 {
			ids := []uint{query.FromGroupId}
			fromGroup, err := context.GetGroupsWithIds(&ids)

			if err != nil {
				fmt.Println(err)

				return addErrContext(err, baseContext)
			}

			tplContext["fromGroup"] = fromGroup
		}

		if query.ToGroupId != 0 {
			ids := []uint{query.ToGroupId}
			toGroup, err := context.GetGroupsWithIds(&ids)

			if err != nil {
				fmt.Println(err)

				return addErrContext(err, baseContext)
			}

			tplContext["toGroup"] = toGroup
		}

		return tplContext
	}
}

func RelationEditContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)
		tplContext := pongo2.Context{
			"pageTitle": "Edit Relation",
		}.Update(baseContext)

		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		relation, err := context.GetRelation(query.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		tplContext["relation"] = relation

		return tplContext
	}
}
