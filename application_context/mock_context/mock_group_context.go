package mock_context

import (
	"errors"
	"mahresources/models"
	"mahresources/models/query_models"
	"time"
)

var errNotImplemented = errors.New("mock: not implemented")

type MockGroupContext struct{}

func NewMockGroupContext() *MockGroupContext {
	return &MockGroupContext{}
}

func (r MockGroupContext) CreateGroup(g *query_models.GroupCreator) (*models.Group, error) {
	return nil, errNotImplemented
}

func (r MockGroupContext) UpdateGroup(g *query_models.GroupEditor) (*models.Group, error) {
	return nil, errNotImplemented
}

func (r MockGroupContext) BulkAddTagsToGroups(query *query_models.BulkEditQuery) error {
	return errNotImplemented
}

func (r MockGroupContext) BulkRemoveTagsFromGroups(query *query_models.BulkEditQuery) error {
	return errNotImplemented
}

func (r MockGroupContext) BulkAddMetaToGroups(query *query_models.BulkEditMetaQuery) error {
	return errNotImplemented
}

func (r MockGroupContext) MergeGroups(winnerId uint, loserIds []uint) error {
	return errNotImplemented
}

func (r MockGroupContext) DuplicateGroup(id uint) (*models.Group, error) {
	return nil, errNotImplemented
}

func (r MockGroupContext) DeleteGroup(groupId uint) error {
	return errNotImplemented
}

func (r MockGroupContext) BulkDeleteGroups(query *query_models.BulkQuery) error {
	return errNotImplemented
}

func (r MockGroupContext) UpdateGroupName(id uint, name string) error {
	return errNotImplemented
}

func (r MockGroupContext) UpdateGroupDescription(id uint, description string) error {
	return errNotImplemented
}

func (MockGroupContext) GetGroups(offset, maxResults int, query *query_models.GroupQuery) ([]models.Group, error) {
	return []models.Group{}, nil
}

func (MockGroupContext) GetGroup(id uint) (*models.Group, error) {
	return &models.Group{
		ID:        0,
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}, nil
}

func (r MockGroupContext) FindParentsOfGroup(id uint) ([]models.Group, error) {
	return []models.Group{}, nil
}
