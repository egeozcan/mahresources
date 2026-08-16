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

// ActionEntityDataReader reads the filter-relevant fields of the entities an
// action was asked to run on, so the execute path can re-apply the very same
// predicate the offer path used (actionMatchesFilters). Implementations live
// outside plugin_system for the same reason EntityRefReader's do.
//
// The returned map is keyed by entity ID and holds only the keys
// actionMatchesFilters reads ("content_type", "category_id", "note_type_id"),
// with the Go types it asserts (string, uint, uint). An ID that does not exist,
// or is not visible to the caller's principal, MUST be absent from the map
// rather than mapped to an empty entry, and a column that is NULL MUST be an
// absent key rather than a zero value: a missing key is read as "does not
// match", which is the fail-closed answer, while 0 is a real id.
type ActionEntityDataReader interface {
	EntityFilterData(entity string, ids []uint) (map[uint]map[string]any, error)
}
