package mock_context

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"time"
)

type MockGroupContext struct{}

func (r MockGroupContext) CreateGroup(g *query_models.GroupCreator) (*models.Group, error) {
	//TODO implement me
	panic("implement me")
}

func (r MockGroupContext) UpdateGroup(g *query_models.GroupEditor) (*models.Group, error) {
	//TODO implement me
	panic("implement me")
}

func (r MockGroupContext) BulkAddTagsToGroups(query *query_models.BulkEditQuery) error {
	//TODO implement me
	panic("implement me")
}

func (r MockGroupContext) BulkRemoveTagsFromGroups(query *query_models.BulkEditQuery) error {
	//TODO implement me
	panic("implement me")
}

func (r MockGroupContext) BulkAddMetaToGroups(query *query_models.BulkEditMetaQuery) error {
	//TODO implement me
	panic("implement me")
}

func (r MockGroupContext) MergeGroups(winnerId uint, loserIds []uint) error {
	//TODO implement me
	panic("implement me")
}

func (r MockGroupContext) DuplicateGroup(id uint) (*models.Group, error) {
	//TODO implement me
	panic("implement me")
}

func (r MockGroupContext) DeleteGroup(groupId uint) error {
	//TODO implement me
	panic("implement me")
}

func (r MockGroupContext) BulkDeleteGroups(query *query_models.BulkQuery) error {
	//TODO implement me
	panic("implement me")
}

func NewMockGroupContext() *MockGroupContext {
	return &MockGroupContext{}
}

func (MockGroupContext) GetGroups(offset, maxResults int, query *query_models.GroupQuery) (*[]models.Group, error) {
	//TODO implement me
	panic("implement me")
}

func (MockGroupContext) GetGroup(id uint) (*models.Group, error) {
	return &models.Group{
		ID:               0,
		CreatedAt:        time.Time{},
		UpdatedAt:        time.Time{},
		Name:             "",
		Description:      "",
		URL:              nil,
		Meta:             nil,
		Owner:            nil,
		OwnerId:          nil,
		RelatedResources: nil,
		RelatedNotes:     nil,
		RelatedGroups:    nil,
		OwnResources:     nil,
		OwnNotes:         nil,
		OwnGroups:        nil,
		Relationships:    nil,
		BackRelations:    nil,
		Tags:             nil,
		CategoryId:       nil,
		Category:         nil,
	}, nil
}

func (r MockGroupContext) FindParentsOfGroup(id uint) (*[]models.Group, error) {
	return &[]models.Group{}, nil
}
