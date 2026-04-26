package plugin_system

// EntityRefReader resolves entity_ref param IDs against the database, applying
// the supplied filter. Implementations live outside plugin_system (e.g.,
// application_context) to keep this package free of DB coupling.
//
// Each method returns the subset of `ids` that EXIST and match `filter`. The
// returned slice may be in any order. Implementations are responsible for
// chunking large id sets to stay under SQLite's variable-binding limit.
type EntityRefReader interface {
	ResourcesMatching(ids []uint, filter ActionFilter) ([]uint, error)
	NotesMatching(ids []uint, filter ActionFilter) ([]uint, error)
	GroupsMatching(ids []uint, filter ActionFilter) ([]uint, error)
}
