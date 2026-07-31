package template_entities

type Entry struct {
	Name    string
	Url     string
	ID      uint
	IsAdmin bool // Whether this is an admin-only menu entry
	// Confirm overrides the destructive confirmation text for a deleteAction.
	// Empty means "use confirmAction's default". Finding 98: deleting a category
	// still in use asked "Are you sure you want to delete?" and then silently
	// moved its groups to Uncategorized, so the blast radius has to travel with
	// the action rather than be hardcoded in the partial.
	Confirm  string
	Children []Entry // For dropdown menus
}
