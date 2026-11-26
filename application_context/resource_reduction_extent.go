package application_context

// idChunk bounds an IN-list. SQLite's and Postgres's parameter ceilings are
// reachable here in a way they are not for most queries: an explicit Extent is
// whatever the reviewer selected, and a Group's subtree can be arbitrarily wide.
// Everything below chunks rather than assuming the list is small — the same
// reasoning visibleGroupIDs' own comment gives for not appending an IN-list of
// subtree ids at all.
const idChunk = 500

func mapKeys(m map[uint]bool) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
