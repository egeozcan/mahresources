package query_models

type GroupTreeNode struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	CategoryName string `json:"categoryName"`
	ChildCount   int    `json:"childCount"`
	OwnerID      *uint  `json:"ownerId"`
}

type GroupTreeRow struct {
	GroupTreeNode
	Level int `json:"level"`
}
