package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

type GroupReader interface {
	GetGroups(offset, maxResults int, query *query_models.GroupQuery) ([]models.Group, error)
	GetGroup(id uint) (*models.Group, error)
	FindParentsOfGroup(id uint) ([]models.Group, error)
}

// --- Granular Group Writer Interfaces ---

// GroupCreator handles group creation operations
type GroupCreator interface {
	CreateGroup(g *query_models.GroupCreator) (*models.Group, error)
}

// GroupUpdater handles group update operations
type GroupUpdater interface {
	UpdateGroup(g *query_models.GroupEditor) (*models.Group, error)
}

// BulkGroupTagEditor handles bulk tag operations on groups
type BulkGroupTagEditor interface {
	BulkAddTagsToGroups(query *query_models.BulkEditQuery) error
	BulkRemoveTagsFromGroups(query *query_models.BulkEditQuery) error
}

// BulkGroupMetaEditor handles bulk meta operations on groups
type BulkGroupMetaEditor interface {
	BulkAddMetaToGroups(query *query_models.BulkEditMetaQuery) error
}

// GroupMerger handles group merging operations
type GroupMerger interface {
	MergeGroups(winnerId uint, loserIds []uint) error
}

// GroupDuplicator handles group duplication operations
type GroupDuplicator interface {
	DuplicateGroup(id uint) (*models.Group, error)
}

// GroupCRUD combines create and update operations for handlers that do both
type GroupCRUD interface {
	GroupCreator
	GroupUpdater
}

// --- Composite Interface (backward compatibility) ---

// GroupWriter combines all group write operations
type GroupWriter interface {
	GroupCreator
	GroupUpdater
	BulkGroupTagEditor
	BulkGroupMetaEditor
	GroupMerger
	GroupDuplicator
}

type GroupDeleter interface {
	DeleteGroup(groupId uint) error
	BulkDeleteGroups(query *query_models.BulkQuery) error
}

// GroupMetaReader provides access to group metadata keys
type GroupMetaReader interface {
	GroupMetaKeys() ([]MetaKey, error)
}
