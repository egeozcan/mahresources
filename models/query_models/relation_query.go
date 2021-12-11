package query_models

type GroupRelationshipQuery struct {
	Id                  uint
	FromGroupId         uint
	ToGroupId           uint
	GroupRelationTypeId uint
	Name                string
	Description         string
}

type RelationshipTypeQuery struct {
	Name        string
	Description string
	// Filters the from-category from group category - makes things easier in FE
	ForFromGroup uint
	// Filters the to-category from group category
	ForToGroup   uint
	FromCategory uint
	ToCategory   uint
}

type RelationshipTypeEditorQuery struct {
	Id           uint
	Name         string
	Description  string
	FromCategory uint
	ToCategory   uint
	ReverseName  string
}
