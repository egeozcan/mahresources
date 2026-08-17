package application_context

// defaultMaxActionEntities bounds one plugin-action run when no operator value
// is configured.
//
// The number is a safety ceiling, not a policy: an action that wants a tighter
// rule declares bulk_max, which is checked first and independently. What this
// stops is the shape of the request rather than its intent — the async branch
// creates one goroutine, one job-map entry and one SSE notification per
// submitted id, eagerly, before any of them runs, and a 1MB request body admits
// on the order of 10^5 ids. Execution is bounded (three at a time) but the
// queue was bounded by nothing, and the cleanup ticker only reaps jobs that
// already finished.
const defaultMaxActionEntities = 1000

// MaxActionEntities is how many entities one plugin-action run may name.
//
// Zero means "not configured" and selects the default, never "refuse
// everything". Every api_test and every programmatic embed builds a
// MahresourcesConfig{} whose zero value was never a decision, and reading it as
// a limit of zero would break each of them with a message about a policy nobody
// set.
func (ctx *MahresourcesContext) MaxActionEntities() int {
	if n := ctx.Config.MaxActionEntities; n > 0 {
		return n
	}
	return defaultMaxActionEntities
}
