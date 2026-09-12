package template_handlers

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/server/template_handlers/loaders"
	"sync"
)

var pickerTemplates struct {
	sync.Once
	set *pongo2.TemplateSet
}

// RenderEntityPickerDefault renders identification only. The dialog owns all
// selection controls, so navigation cannot accidentally toggle a pending choice.
func RenderEntityPickerDefault(entityType string, row contracts.EntityPickerEntity) (string, error) {
	pickerTemplates.Do(func() {
		pickerTemplates.set = pongo2.NewSet("entity-picker", loaders.MustNewLocalFileSystemLoader("./templates", nil))
	})
	tpl, err := pickerTemplates.set.FromCache("/partials/entityPickerResult.tpl")
	if err != nil {
		return "", err
	}
	typeName := ""
	switch entity := row.Raw.(type) {
	case models.Resource:
		if entity.ResourceCategory != nil {
			typeName = entity.ResourceCategory.Name
		}
	case models.Group:
		if entity.Category != nil {
			typeName = entity.Category.Name
		}
	case models.Note:
		if entity.NoteType != nil {
			typeName = entity.NoteType.Name
		}
	}
	return tpl.Execute(pongo2.Context{"entityType": entityType, "id": row.ID, "name": row.Name, "entity": row.Raw, "typeName": typeName})
}
