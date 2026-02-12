package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

type RelationshipWriter interface {
	EditRelation(query query_models.GroupRelationshipQuery) (*models.GroupRelation, error)
	AddRelation(fromGroupId, toGroupId, relationTypeId uint) (*models.GroupRelation, error)
	AddRelationType(query *query_models.RelationshipTypeEditorQuery) (*models.GroupRelationType, error)
	EditRelationType(query *query_models.RelationshipTypeEditorQuery) (*models.GroupRelationType, error)
}

type RelationshipReader interface {
	GetRelationTypes(offset int, maxResults int, query *query_models.RelationshipTypeQuery) ([]*models.GroupRelationType, error)
}

type RelationshipDeleter interface {
	DeleteRelationship(relationshipId uint) error
	DeleteRelationshipType(relationshipTypeId uint) error
}
