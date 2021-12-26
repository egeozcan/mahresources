package query_models

type GroupCreator struct {
	Name        string
	Description string
	Tags        []uint
	Groups      []uint
	CategoryId  uint
	OwnerId     uint
	Meta        string
	URL         string
}

type GroupEditor struct {
	GroupCreator
	ID uint
}

type GroupQuery struct {
	Name                  string
	Description           string
	Tags                  []uint
	SearchParentsForTags  bool
	SearchChildrenForTags bool
	Notes                 []uint
	Groups                []uint
	OwnerId               uint
	Resources             []uint
	Categories            []uint
	CategoryId            uint
	CreatedBefore         string
	CreatedAfter          string
	RelationTypeId        uint
	RelationSide          uint
	MetaQuery             []ColumnMeta
	SortBy                string
	URL                   string
}
