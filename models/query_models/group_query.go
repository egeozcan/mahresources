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
	SearchParentsForName  bool
	SearchChildrenForName bool
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
	UpdatedBefore         string
	UpdatedAfter          string
	RelationTypeId        uint
	RelationSide          uint
	MetaQuery             []ColumnMeta
	SortBy                []string
	URL                   string
	Ids                   []uint
	// MRQL is an optional MRQL filter expression (package 5 list-page bar),
	// parsed with mrql.ParseFilter (type = "group" implied). Empty = no filter.
	MRQL string
}
