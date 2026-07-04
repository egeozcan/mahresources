package application_context

import (
	"errors"
	"regexp"
	"strings"

	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
)

// templatePartialNamePattern enforces kebab-case names so [partial name="…"]
// references stay parseable and lintable.
var templatePartialNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func (ctx *MahresourcesContext) GetTemplatePartial(id uint) (*models.TemplatePartial, error) {
	var partial models.TemplatePartial
	return &partial, ctx.db.First(&partial, id).Error
}

// GetTemplatePartialByName resolves a partial by its unique name. It powers the
// [partial name="…"] resolver.
func (ctx *MahresourcesContext) GetTemplatePartialByName(name string) (*models.TemplatePartial, error) {
	var partial models.TemplatePartial
	if err := ctx.db.Where("name = ?", name).First(&partial).Error; err != nil {
		return nil, err
	}
	return &partial, nil
}

func (ctx *MahresourcesContext) GetTemplatePartials(query *query_models.TemplatePartialQuery, offset, maxResults int) ([]models.TemplatePartial, error) {
	var partials []models.TemplatePartial
	err := ctx.db.Scopes(database_scopes.TemplatePartialQuery(query)).Order("name asc").Limit(maxResults).Offset(offset).Find(&partials).Error
	return partials, err
}

func (ctx *MahresourcesContext) GetTemplatePartialsCount(query *query_models.TemplatePartialQuery) (int64, error) {
	var partial models.TemplatePartial
	var count int64
	return count, ctx.db.Scopes(database_scopes.TemplatePartialQuery(query)).Model(&partial).Count(&count).Error
}

func (ctx *MahresourcesContext) CreateOrUpdateTemplatePartial(query *query_models.TemplatePartialEditor) (*models.TemplatePartial, error) {
	isNew := query.ID == 0
	var partial models.TemplatePartial
	if query.ID != 0 {
		if err := ctx.db.First(&partial, query.ID).Error; err != nil {
			return nil, err
		}
	}

	name := strings.TrimSpace(query.Name)
	if name != "" {
		if !templatePartialNamePattern.MatchString(name) {
			return nil, errors.New("template partial name must be kebab-case: lowercase letters, digits, and hyphens, starting with a letter")
		}
		partial.Name = name
	} else if isNew {
		return nil, errors.New("template partial name must be non-empty")
	}

	partial.Description = query.Description
	partial.Content = query.Content

	if err := ctx.db.Save(&partial).Error; err != nil {
		return nil, err
	}

	if isNew {
		ctx.Logger().Info(models.LogActionCreate, "templatePartial", &partial.ID, partial.Name, "Created template partial", nil)
	} else {
		ctx.Logger().Info(models.LogActionUpdate, "templatePartial", &partial.ID, partial.Name, "Updated template partial", nil)
	}

	return &partial, nil
}

func (ctx *MahresourcesContext) DeleteTemplatePartial(id uint) error {
	var partial models.TemplatePartial
	if err := ctx.db.First(&partial, id).Error; err != nil {
		return err
	}
	name := partial.Name

	err := ctx.db.Delete(&partial).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "templatePartial", &id, name, "Deleted template partial", nil)
	}
	return err
}
