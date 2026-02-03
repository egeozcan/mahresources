package template_entities

type Entry struct {
	Name     string
	Url      string
	ID       uint
	IsAdmin  bool // Whether this is an admin-only menu entry
	Children []Entry // For dropdown menus
}
