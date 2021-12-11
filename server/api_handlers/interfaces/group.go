package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

type GroupReader interface {
	GetGroups(offset, maxResults int, query *query_models.GroupQuery) (*[]models.Group, error)
	GetGroup(id uint) (*models.Group, error)
}

type GroupWriter interface {
	CreateGroup(g *query_models.GroupCreator) (*models.Group, error)
	UpdateGroup(g *query_models.GroupEditor) (*models.Group, error)
}

type GroupDeleter interface {
	DeleteGroup(groupId uint) error
}
