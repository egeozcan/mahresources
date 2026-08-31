package plugin_system

import "log"

// The names a plugin may register against.
//
// mah.on and mah.inject took any string and stored it, so a misspelled event
// registered happily and never dispatched, and a misspelled slot registered
// happily and never rendered: a plugin that loaded cleanly, reported nothing
// and did nothing. Nothing in the host could tell a typo from a name it would
// one day dispatch, because there was no list to compare against.
//
// Both sides of each surface read the lists below. Registration refuses a name
// outside them, and dispatch reports one, so the two cannot come to disagree
// about which names are live. A second list that agrees today is a drift bug
// scheduled for later.
//
// Neither list is decidable from inside this package: the events are dispatched
// from application_context and the slots are declared in the templates, and
// this package may not import either. internal/arch/plugin_catalogue_drift_test.go
// reads both sources and fails the build when a catalogue stops describing them.

// AllHookEvents is every lifecycle event the host dispatches, in the order the
// documented reference table lists them. Five entities times three operations,
// before and after.
var AllHookEvents = []string{
	"before_resource_create", "after_resource_create",
	"before_resource_update", "after_resource_update",
	"before_resource_delete", "after_resource_delete",

	"before_note_create", "after_note_create",
	"before_note_update", "after_note_update",
	"before_note_delete", "after_note_delete",

	"before_group_create", "after_group_create",
	"before_group_update", "after_group_update",
	"before_group_delete", "after_group_delete",

	"before_tag_create", "after_tag_create",
	"before_tag_update", "after_tag_update",
	"before_tag_delete", "after_tag_delete",

	"before_category_create", "after_category_create",
	"before_category_update", "after_category_update",
	"before_category_delete", "after_category_delete",
	// Job lifecycle. Not entity lifecycle, which is what the rest of this list
	// is -- these fire from the download queue's terminal edge, for every job
	// kind it runs. They are after-only by construction: a job that has finished
	// cannot be vetoed. Behind CapJobEvents rather than CapHooks, because
	// observing every job in the deployment is a different power from reacting
	// to a write the caller just made.
	"after_job_completed", "after_job_failed", "after_job_cancelled",

	// Mass edit. One request-scoped pair for the highest-blast-radius write in
	// the application, veto-only, with no field rewriting (there are no fields
	// to rewrite). Per-row before_resource_update hooks here would mean up to
	// 10,000 Lua invocations inside one write transaction on single-writer
	// SQLite, which is exactly the mistake BulkDeleteResources' hook pair
	// exists to avoid repeating at scale.
	"before_mass_edit", "after_mass_edit",
}

// AllInjectionSlots is every slot a template declares with {% plugin_slot %},
// layout slots first (those six run on every page), then the per-entity ones.
var AllInjectionSlots = []string{
	"head", "page_top", "sidebar_top", "sidebar_bottom", "page_bottom", "scripts",

	"resource_detail_before", "resource_detail_after", "resource_detail_sidebar",
	"note_detail_before", "note_detail_after", "note_detail_sidebar",
	"group_detail_before", "group_detail_after", "group_detail_sidebar",

	"resource_list_before", "resource_list_after",
	"note_list_before", "note_list_after",
	"group_list_before", "group_list_after",
}

// The lists indexed for lookup. Built from the slices rather than written out
// again, so a name can only be added in one place.
var (
	hookEventSet     = indexNames(AllHookEvents)
	injectionSlotSet = indexNames(AllInjectionSlots)
)

func indexNames(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}

// IsHookEvent reports whether event is one the host dispatches.
//
// Exact match, deliberately. A name differing by case, by a stray space or by
// two words swapped is a name nothing dispatches, so folding any of that in
// would hand back the silent no-op this catalogue exists to stop.
func IsHookEvent(event string) bool {
	_, ok := hookEventSet[event]
	return ok
}

// IsInjectionSlot reports whether slot is one a template renders.
func IsInjectionSlot(slot string) bool {
	_, ok := injectionSlotSet[slot]
	return ok
}

// reportUnknownDispatch says that the host asked for a name no plugin could
// have registered against.
//
// Registration only admits catalogue names, so this is the host's own typo, and
// it is exactly as silent as the plugin's was: nothing is registered, nothing
// runs, and the caller's write or page proceeds none the wiser. Said once per
// name, because an event is dispatched on every write of its entity and a line
// per write would bury the one line that matters.
//
// Nothing is said for a catalogue name nobody registered against. That is the
// ordinary case rather than a mistake, and warning on it would drown the real
// one.
func (pm *PluginManager) reportUnknownDispatch(kind, name string) {
	if _, seen := pm.unknownDispatchWarned.LoadOrStore(kind+":"+name, struct{}{}); seen {
		return
	}
	log.Printf("[plugin] warning: %s %q is not one this host knows, so no plugin can be registered "+
		"for it and it will never do anything", kind, name)
}

// jobHookEvents is the subset of AllHookEvents that CapJobEvents gates.
var jobHookEvents = map[string]struct{}{
	"after_job_completed": {},
	"after_job_failed":    {},
	"after_job_cancelled": {},
}

// IsJobHookEvent reports whether an event is one of the job-lifecycle events.
//
// They live in AllHookEvents like every other event, so mah.on accepts the name
// and the drift scan covers it — but registering for one needs CapJobEvents
// rather than CapHooks, because it observes every job in the deployment rather
// than a write the caller just made.
func IsJobHookEvent(event string) bool {
	_, ok := jobHookEvents[event]
	return ok
}
