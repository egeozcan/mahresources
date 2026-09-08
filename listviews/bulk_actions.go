package listviews

import (
	"slices"
	"strings"
)

type BulkActionFilter struct {
	ContentTypes []string `json:"content_types,omitempty"`
	CategoryIDs  []uint   `json:"category_ids,omitempty"`
	NoteTypeIDs  []uint   `json:"note_type_ids,omitempty"`
}

// BulkAction is the shared presentation contract for list actions. Component
// names refer to trusted application templates; handlers remain responsible for
// authorization and validating submitted input.
type BulkAction struct {
	Entities         []string
	Filters          BulkActionFilter
	ID               string
	Label            string
	SubmitLabel      string
	ConfirmComponent string
	NoAjax           bool
	Component        string
	Endpoint         string
	Input            string
	InputEntity      string
	InputLabel       string
	Multiple         bool
	Min              int
	Max              int
	Confirm          string
	Danger           bool
	Plugin           any
}

// Register built-in actions here. Each declaration names every entity it
// supports; neither ordinary lists nor MRQL maintain their own action lists.
var bulkActionCatalog = []BulkAction{
	{ID: "mass-edit", Label: "Mass Edit Selected", Entities: []string{"resource", "note", "group"}, Component: "massEdit", Min: 1},
	{ID: "add-tags", Label: "Add Tag", SubmitLabel: "Add", Entities: []string{"resource", "note", "group"}, Endpoint: "/v1/{entity}s/addTags", Input: "entity", InputEntity: "tag", InputLabel: "Add Tag", Min: 1},
	{ID: "remove-tags", Label: "Remove Tag", SubmitLabel: "Remove", Entities: []string{"resource", "note", "group"}, Endpoint: "/v1/{entity}s/removeTags", Input: "entity", InputEntity: "tag", InputLabel: "Remove Tag", Multiple: true, Min: 1},
	{ID: "add-meta", Label: "Add Meta", SubmitLabel: "Add", Entities: []string{"resource", "note", "group"}, Endpoint: "/v1/{entity}s/addMeta", Input: "meta", Min: 1},
	{ID: "add-groups", Label: "Add Groups", SubmitLabel: "Add", Entities: []string{"resource", "note"}, Endpoint: "/v1/{entity}s/addGroups", Input: "entity", InputEntity: "group", InputLabel: "Add Groups", Multiple: true, Min: 1},
	{ID: "dimensions", Label: "Update Dimensions", Entities: []string{"resource"}, Endpoint: "/v1/resource/recalculateDimensions", Min: 1},
	{ID: "merge", Label: "Merge", Entities: []string{"tag"}, Component: "mergeTags", Min: 1},
	{ID: "delete", Label: "Delete Selected", SubmitLabel: "Delete", NoAjax: true, Entities: []string{"resource", "note", "tag"}, Endpoint: "/v1/{entity}s/delete", Confirm: "Delete {count} {entity}{s}? This cannot be undone.", Danger: true, Min: 1},
	{ID: "delete", Label: "Delete Selected", SubmitLabel: "Delete", NoAjax: true, Entities: []string{"group"}, Endpoint: "/v1/groups/delete", ConfirmComponent: "confirmGroupDelete", Danger: true, Min: 1},
	{ID: "compare", Label: "Compare", Entities: []string{"resource", "group"}, Component: "compare", Min: 2, Max: 2},
	{ID: "reduction", Label: "Resource Reduction", Entities: []string{"resource", "group"}, Component: "reduction", Min: 1},
	{ID: "export", Label: "Export selected", Entities: []string{"group"}, Component: "exportGroups", Min: 1},
	{ID: "retry", Label: "Retry selected", Entities: []string{"download"}, Component: "retryDownloads", Min: 1},
	{ID: "delete", Label: "Delete selected", Entities: []string{"download"}, Component: "deleteDownloads", Min: 1, Danger: true},
}

func BulkActions(entity string) []BulkAction {
	var actions []BulkAction
	for _, declaration := range bulkActionCatalog {
		if !slices.Contains(declaration.Entities, entity) {
			continue
		}
		declaration.Endpoint = strings.ReplaceAll(declaration.Endpoint, "{entity}", entity)
		declaration.Confirm = strings.ReplaceAll(declaration.Confirm, "{entity}", entity)
		actions = append(actions, declaration)
	}
	return actions
}
