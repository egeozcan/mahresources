package query_models

// MassEditQuery is the wire shape of one mass-edit request. It is flat, with no
// struct tags, matching entity_query.go's convention: the field names are the
// wire names, and both encoding/json and gorilla/schema match case-insensitively,
// so tagsOp from a JSON body and TagsOp from a form both land.
//
// Every field is a scalar or a slice of scalars, so gorilla/schema decodes the
// whole thing from urlencoded and multipart bodies with no converter.
//
// An unrecognised verb (TagsOp "replce", say) is refused, never defaulted:
// silently reading a typo'd "replace" as "add" is the class of mistake that only
// surfaces once the data is wrong.
type MassEditQuery struct {
	// --- targeting ---
	// ID is the explicit selection; a form sends repeated id=1&id=2 entries.
	ID []uint
	// Target selects how the target set is resolved: "" or "ids" uses ID;
	// "filter" re-runs Filter (the list page's raw query string) server-side.
	Target string
	// Filter is the list page's raw query string, e.g. "tags=3&ownerId=9&mrql=...".
	// Used only when Target is "filter".
	Filter string
	// ExpectedCount is required when Target is "filter": the server re-counts the
	// filtered set and refuses with a conflict unless the count matches exactly.
	// A POINTER because zero is a legitimate confirmed count (an empty set over
	// an empty filter) that must be distinguishable from "omitted".
	ExpectedCount *uint

	// --- ops: an empty verb means the op is absent ---
	// TagsOp applies to all three entities.
	TagsOp   string
	TagIds   []uint
	// GroupsOp applies to resources and notes (related groups).
	GroupsOp string
	GroupIds []uint
	// NotesOp applies to resources and groups (related notes).
	NotesOp  string
	NoteIds  []uint
	// ResourcesOp applies to notes and groups (related resources).
	ResourcesOp string
	ResourceIds []uint
	// RelatedGroupsOp applies to groups only (group-to-group relations).
	RelatedGroupsOp string
	RelatedGroupIds []uint

	// OwnerOp is "set" or "clear". A Group's owner is its parent, so the same
	// op re-parents groups.
	OwnerOp string
	OwnerId uint

	// MetaOp is "merge", "removeKeys" or "replace".
	MetaOp   string
	Meta     string   // JSON object, for merge and replace
	MetaKeys []string // for removeKeys

	// DryRun resolves the target set and parses the ops, committing nothing.
	DryRun bool
}
