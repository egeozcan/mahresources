package context

import (
	"mahresources/models"
)

func (ctx *MahresourcesContext) GetTags(name string, limit int) (*[]models.Tag, error) {
	var tags []models.Tag

	ctx.db.Where("name like ?", "%"+name+"%").Order("name").Limit(limit).Find(&tags)

	return &tags, nil
}
