package context

import (
	"mahresources/models"
)

func (ctx *MahresourcesContext) GetTags(name string, limit int) (*[]models.Tag, error) {
	var tags []models.Tag

	query := ctx.db.Where("name like ?", "%"+name+"%").Order("name")

	if limit > 0 {
		query = query.Limit(limit)
	}

	query.Find(&tags)

	return &tags, nil
}
