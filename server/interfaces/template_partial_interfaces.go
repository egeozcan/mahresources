package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

// TemplatePartialReader provides read access to template partials.
type TemplatePartialReader interface {
	GetTemplatePartials(query *query_models.TemplatePartialQuery, offset, maxResults int) ([]models.TemplatePartial, error)
}

// TemplatePartialWriter creates or updates template partials.
type TemplatePartialWriter interface {
	CreateOrUpdateTemplatePartial(query *query_models.TemplatePartialEditor) (*models.TemplatePartial, error)
	GetTemplatePartial(id uint) (*models.TemplatePartial, error)
}

// TemplatePartialDeleter deletes template partials.
type TemplatePartialDeleter interface {
	DeleteTemplatePartial(id uint) error
}
