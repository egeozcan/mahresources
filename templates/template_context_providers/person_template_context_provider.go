package template_context_providers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/constants"
	"mahresources/context"
	"mahresources/http_query"
	"mahresources/http_utils"
	"mahresources/models"
	"mahresources/templates/template_entities"
	"net/http"
	"strconv"
)

func PeopleListContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResults
		var query http_query.PersonQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		people, err := context.GetPeople(int(offset), constants.MaxResults, &query)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		peopleCount, err := context.GetPeopleCount(&query)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), peopleCount, constants.MaxResults, int(page))

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		tags, err := context.GetTagsByName("", 0)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		tagList := models.TagList(*tags)
		tagsDisplay := template_entities.GenerateRelationsDisplay(query.Tags, tagList.ToNamedEntities(), request.URL.String(), true, "tags")

		return pongo2.Context{
			"pageTitle":  "People",
			"people":     people,
			"pagination": pagination,
			"tags":       tagsDisplay,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/person/new",
			},
		}.Update(baseContext)
	}
}

func PersonCreateContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Add New Person",
		}.Update(StaticTemplateCtx(request))

		var query http_query.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			return tplContext
		}

		person, err := context.GetPerson(query.ID)

		if err != nil {
			return tplContext
		}

		tagIDs := make([]uint, len(person.Tags))

		for i, tag := range person.Tags {
			tagIDs[i] = tag.ID
		}

		tagList := models.TagList(person.Tags)
		tagsDisplay := template_entities.GenerateRelationsDisplay(tagIDs, tagList.ToNamedEntities(), request.URL.String(), true, "tags")

		tplContext["person"] = person
		tplContext["pageTitle"] = "Edit Person"
		tplContext["tags"] = tagsDisplay.SelectedRelations

		return tplContext
	}
}

func PersonContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query http_query.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		person, err := context.GetPerson(query.ID)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		return pongo2.Context{
			"pageTitle": "Person: " + person.GetName(),
			"person":    person,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/person/edit?id=" + strconv.Itoa(int(query.ID)),
			},
		}.Update(baseContext)
	}
}
