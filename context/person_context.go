package context

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/api_model"
	"mahresources/database_scopes"
	"mahresources/http_query"
	"mahresources/models"
)

func (ctx *MahresourcesContext) CreatePerson(personQuery *http_query.PersonCreator) (*models.Person, error) {
	if personQuery.Name == "" {
		return nil, errors.New("person name needed")
	}

	person := models.Person{
		Name:        personQuery.Name,
		Surname:     personQuery.Surname,
		Description: personQuery.Description,
	}
	ctx.db.Create(&person)

	if len(personQuery.Tags) > 0 {
		tags := make([]models.Tag, len(personQuery.Tags))
		for i, v := range personQuery.Tags {
			tags[i] = models.Tag{
				Model: gorm.Model{ID: v},
			}
		}
		createTagsErr := ctx.db.Model(&person).Association("Tags").Append(&tags)

		if createTagsErr != nil {
			return nil, createTagsErr
		}
	}

	return &person, nil
}
func (ctx *MahresourcesContext) UpdatePerson(personQuery *http_query.PersonEditor) (*models.Person, error) {
	if personQuery.Name == "" {
		return nil, errors.New("person name needed")
	}

	tags := make([]*models.Tag, len(personQuery.Tags))

	for i, tag := range personQuery.Tags {
		tags[i] = &models.Tag{
			Model: gorm.Model{
				ID: tag,
			},
		}
	}

	person := models.Person{
		Model: gorm.Model{
			ID: personQuery.ID,
		},
		Name:        personQuery.Name,
		Surname:     personQuery.Surname,
		Description: personQuery.Description,
		Tags:        tags,
	}
	ctx.db.Save(&person)

	return &person, nil
}

func (ctx *MahresourcesContext) GetPerson(id uint) (*models.Person, error) {
	var person models.Person
	ctx.db.Preload(clause.Associations).First(&person, id)

	if person.ID == 0 {
		return nil, errors.New("could not load person")
	}

	return &person, nil
}

func (ctx *MahresourcesContext) GetPeople(offset, maxResults int, query *http_query.PersonQuery) (*[]models.Person, error) {
	var people []models.Person

	ctx.db.Scopes(database_scopes.PersonQuery(query)).Limit(maxResults).Offset(int(offset)).Preload("Tags").Find(&people)

	return &people, nil
}

func (ctx *MahresourcesContext) GetPeopleAutoComplete(name string, maxResults int) (*[]api_model.AutoCompleteResult, error) {
	var people []models.Person

	ctx.db.Where("name LIKE ?", "%"+name+"%").Or("surname LIKE ?", "%"+name+"%").Limit(maxResults).Find(&people)

	results := make([]api_model.AutoCompleteResult, len(people))

	for i, v := range people {
		results[i] = api_model.AutoCompleteResult{
			Name: v.Name + " " + v.Surname,
			ID:   v.ID,
		}
	}

	return &results, nil
}

func (ctx *MahresourcesContext) GetPeopleWithIds(ids []uint) (*[]*models.Person, error) {
	var people []*models.Person

	ctx.db.Find(&people, ids)

	return &people, nil
}

func (ctx *MahresourcesContext) GetPeopleCount(query *http_query.PersonQuery) (int64, error) {
	var person models.Person
	var count int64
	ctx.db.Scopes(database_scopes.PersonQuery(query)).Model(&person).Count(&count)

	return count, nil
}

func (ctx *MahresourcesContext) GetTagsForPeople() (*[]models.Tag, error) {
	var tags []models.Tag
	ctx.db.Raw(`SELECT
					  Count(*)
					  , id
					  , name
					from tags t
					join person_tags pt on t.id = pt.tag_id
					group by t.name, t.id
					order by count(*) desc
	`).Scan(&tags)

	return &tags, nil
}
