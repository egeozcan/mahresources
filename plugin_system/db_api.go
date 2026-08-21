package plugin_system

import (
	"context"
	"fmt"
	"math"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// EntityQuerier provides database access to entities for plugins.
// All methods return map[string]any to avoid importing models into the plugin_system package.
// Numeric values must be float64 for Lua compatibility.
//
// The single-entity getters distinguish "not found" from "the read failed":
// not found is (nil, nil), a failure is (nil, err). Lua sees the same split —
// see registerDbModule — so a plugin can back off on an outage instead of
// concluding the corpus is empty.
type EntityQuerier interface {
	// Single entity by ID — returns nil map if not found
	GetNoteData(id uint) (map[string]any, error)
	GetResourceData(id uint) (map[string]any, error)
	GetGroupData(id uint) (map[string]any, error)
	GetTagData(id uint) (map[string]any, error)
	GetCategoryData(id uint) (map[string]any, error)
	GetNoteTypeData(id uint) (map[string]any, error)
	GetResourceCategoryData(id uint) (map[string]any, error)
	// Taxonomy listing. Taxonomies are small and have no owner, so these take
	// the same {name=..., limit=..., offset=...} filter shape as the entity
	// queries but no scoping fields.
	ListTags(filter map[string]any) ([]map[string]any, error)
	ListCategories(filter map[string]any) ([]map[string]any, error)
	ListNoteTypes(filter map[string]any) ([]map[string]any, error)
	ListResourceCategories(filter map[string]any) ([]map[string]any, error)
	// List queries with simple filters
	QueryNotes(filter map[string]any) ([]map[string]any, error)
	QueryResources(filter map[string]any) ([]map[string]any, error)
	QueryGroups(filter map[string]any) ([]map[string]any, error)
	// Count queries — same filters as Query* but return count only
	CountNotes(filter map[string]any) (int64, error)
	CountResources(filter map[string]any) (int64, error)
	CountGroups(filter map[string]any) (int64, error)
	// Resource file data — returns base64 content and MIME type
	GetResourceFileData(id uint) (string, string, error)
	// Resource creation
	CreateResourceFromURL(reqCtx context.Context, url string, options map[string]any) (map[string]any, error)
	CreateResourceFromData(base64Data string, options map[string]any) (map[string]any, error)
	// Resource versioning
	AddResourceVersionFromURL(resourceID uint, url string, comment string) (map[string]any, error)
}

// EntityWriter provides write access to entities for plugins.
// Note: Update methods replace ALL fields; omitted fields revert to zero values.
// This includes associations — updating a note without specifying tags will clear its tags.
// Use Patch methods for partial updates that preserve unspecified fields.
type EntityWriter interface {
	CreateGroup(opts map[string]any) (map[string]any, error)
	UpdateGroup(id uint, opts map[string]any) (map[string]any, error)
	PatchGroup(id uint, opts map[string]any) (map[string]any, error)
	DeleteGroup(id uint) error
	CreateNote(opts map[string]any) (map[string]any, error)
	UpdateNote(id uint, opts map[string]any) (map[string]any, error)
	PatchNote(id uint, opts map[string]any) (map[string]any, error)
	DeleteNote(id uint) error
	CreateTag(opts map[string]any) (map[string]any, error)
	UpdateTag(id uint, opts map[string]any) (map[string]any, error)
	PatchTag(id uint, opts map[string]any) (map[string]any, error)
	DeleteTag(id uint) error
	CreateCategory(opts map[string]any) (map[string]any, error)
	UpdateCategory(id uint, opts map[string]any) (map[string]any, error)
	PatchCategory(id uint, opts map[string]any) (map[string]any, error)
	DeleteCategory(id uint) error
	CreateResourceCategory(opts map[string]any) (map[string]any, error)
	UpdateResourceCategory(id uint, opts map[string]any) (map[string]any, error)
	PatchResourceCategory(id uint, opts map[string]any) (map[string]any, error)
	DeleteResourceCategory(id uint) error
	CreateNoteType(opts map[string]any) (map[string]any, error)
	UpdateNoteType(id uint, opts map[string]any) (map[string]any, error)
	PatchNoteType(id uint, opts map[string]any) (map[string]any, error)
	DeleteNoteType(id uint) error
	CreateGroupRelation(opts map[string]any) (map[string]any, error)
	UpdateGroupRelation(opts map[string]any) (map[string]any, error)
	PatchGroupRelation(opts map[string]any) (map[string]any, error)
	DeleteGroupRelation(id uint) error
	CreateRelationType(opts map[string]any) (map[string]any, error)
	UpdateRelationType(opts map[string]any) (map[string]any, error)
	PatchRelationType(opts map[string]any) (map[string]any, error)
	DeleteRelationType(id uint) error
	AddTagsToEntity(entityType string, id uint, tagIds []uint) error
	RemoveTagsFromEntity(entityType string, id uint, tagIds []uint) error
	AddGroupsToEntity(entityType string, id uint, groupIds []uint) error
	RemoveGroupsFromEntity(entityType string, id uint, groupIds []uint) error
	AddResourcesToNote(noteId uint, resourceIds []uint) error
	RemoveResourcesFromNote(noteId uint, resourceIds []uint) error
	UpdateResource(id uint, opts map[string]any) (map[string]any, error)
	PatchResource(id uint, opts map[string]any) (map[string]any, error)
	DeleteResource(id uint) error
}

// PluginLogger persists plugin log messages to the application log store.
type PluginLogger interface {
	PluginLog(pluginName, level, message string, details map[string]any)
}

// MaxKVValueSize is the largest serialized value a KVStore accepts, in bytes.
//
// It sits with the interface rather than with the implementation that enforces
// it because both sides need the same number: the store refuses anything larger,
// and mah.kv publishes it as max_value_size so an author can measure a value
// before writing it rather than only after the refusal unwound their handler.
const MaxKVValueSize = 8 * 1024 * 1024

// KVStore provides per-plugin key-value storage for plugins.
type KVStore interface {
	KVGet(pluginName, key string) (string, bool, error)
	KVSet(pluginName, key, value string) error
	// KVCompareAndSet writes value only when the stored value is still
	// expected, and reports whether it wrote. A nil expected is the other
	// expectation, "no row may exist yet", which is a different state from a
	// row holding the JSON null a Lua nil stores.
	//
	// The comparison must happen in the statement that writes. An
	// implementation that reads and then writes reintroduces the lost update
	// this exists to close, and no single-threaded caller can tell the two
	// apart.
	KVCompareAndSet(pluginName, key string, expected *string, value string) (bool, error)
	KVDelete(pluginName, key string) error
	KVList(pluginName, prefix string) ([]string, error)
	KVPurge(pluginName string) error
}

// SetKVStore sets the key-value store for plugin data persistence.
// This is called after context creation to break the circular dependency
// between plugin_system and application_context.
func (pm *PluginManager) SetKVStore(kv KVStore) {
	pm.kvStore.Store(kv)
}

// getKVStore returns the current KVStore, or nil if not yet set.
func (pm *PluginManager) getKVStore() KVStore {
	v := pm.kvStore.Load()
	if v == nil {
		return nil
	}
	return v.(KVStore)
}

// SetPluginLogger sets the logger for plugin log messages.
// This is called after context creation to break the circular dependency
// between plugin_system and application_context.
func (pm *PluginManager) SetPluginLogger(pl PluginLogger) {
	pm.logger.Store(pl)
}

// getPluginLogger returns the current PluginLogger, or nil if not yet set.
func (pm *PluginManager) getPluginLogger() PluginLogger {
	v := pm.logger.Load()
	if v == nil {
		return nil
	}
	return v.(PluginLogger)
}

// SetEntityQuerier sets the database provider for plugin DB access.
// This is called after context creation to break the circular dependency
// between plugin_system and application_context.
func (pm *PluginManager) SetEntityQuerier(eq EntityQuerier) {
	pm.dbProvider.Store(eq)
}

// getDbProvider returns the current EntityQuerier, or nil if not yet set.
func (pm *PluginManager) getDbProvider() EntityQuerier {
	v := pm.dbProvider.Load()
	if v == nil {
		return nil
	}
	return v.(EntityQuerier)
}

// SetEntityWriter sets the database writer for plugin entity CRUD.
// This is called after context creation to break the circular dependency
// between plugin_system and application_context.
func (pm *PluginManager) SetEntityWriter(ew EntityWriter) {
	pm.dbWriter.Store(ew)
}

// getDbWriter returns the current EntityWriter, or nil if not yet set.
func (pm *PluginManager) getDbWriter() EntityWriter {
	v := pm.dbWriter.Load()
	if v == nil {
		return nil
	}
	return v.(EntityWriter)
}

// PrincipalBinder produces querier and writer views bound to one invocation:
// running as the user who triggered it, and carrying the chain of plugin VMs
// already executing so a nested hook dispatch can refuse to re-enter one.
//
// It is one method rather than a context parameter on the 62 EntityQuerier and
// EntityWriter methods because the implementation is a one-field struct whose
// principal-bound clone is a single line — threading a context through every
// signature would be churn with no payoff.
type PrincipalBinder interface {
	BindInvocation(inv *Invocation) (EntityQuerier, EntityWriter)
}

// SetPrincipalBinder installs the binder used to run plugin DB calls as their
// triggering principal. Called after context creation, like the setters above.
//
// Optional: with no binder installed, plugin calls fall back to the unbound
// provider and behave exactly as they did before per-invocation binding. Tests
// that inject a bare fake querier therefore need no binder.
func (pm *PluginManager) SetPrincipalBinder(b PrincipalBinder) {
	pm.principalBinder.Store(b)
}

// getPrincipalBinder returns the current PrincipalBinder, or nil if not set.
func (pm *PluginManager) getPrincipalBinder() PrincipalBinder {
	v := pm.principalBinder.Load()
	if v == nil {
		return nil
	}
	return v.(PrincipalBinder)
}

// querierFor returns the EntityQuerier a mah.db read made from L must use: bound
// to L's triggering principal when a binder is installed, and the unbound
// provider otherwise.
//
// It returns nil for a revoked VM, and every caller already renders that as
// "database not available". Revocation stops new dispatch and new
// registrations, but a worker that was already inside keeps its
// fully-installed mah table until it finishes — up to the async allowance,
// which is five minutes. Without this, DisablePlugin could return, the UI could
// show the plugin off, and mah.db.delete_resource could still succeed. A
// disable that is reported complete has to be complete for the operations that
// change data.

func (pm *PluginManager) querierFor(L *lua.LState) EntityQuerier {
	if !pm.stateIsLive(L) {
		return nil
	}
	inv := pm.invocationFor(L)
	if tx := inv.transactionBinding(); tx != nil {
		q, _ := bindOntoTransaction(tx, inv)
		return q
	}
	b := pm.getPrincipalBinder()
	if b == nil {
		return pm.getDbProvider()
	}
	q, _ := b.BindInvocation(inv)
	if q == nil {
		return pm.getDbProvider()
	}
	return q
}

// bindOntoTransaction rebinds inv onto the open transaction's own handle,
// rather than handing back the binding the transaction was opened with.
//
// The difference is the call chain, and it is not cosmetic. The stored binding
// was built when the transaction opened, so its context carries the *opening*
// invocation — and every hook a write through it fires is dispatched from that
// context. A nested plugin's write would therefore announce itself with a chain
// that does not contain the nested plugin, so the re-entry guard — whose whole
// job is to stop a plugin being notified of a write made while it is running —
// would let that dispatch through and go for a VM mutex this same goroutine
// already holds. A before-hook blocks for hookLockWait and then fails the
// write; an after-hook blocks and is dropped. Both inside the transaction,
// holding the database write lock for the wait.
//
// Rebinding through the transaction's *own* adapter is what keeps the handle:
// BindInvocation clones that adapter's context, whose db is the transaction,
// and WithPrincipal preserves the connection pool. Going back to the
// process-wide binder here is the version that would lose it.
func bindOntoTransaction(tx TransactionBinding, inv *Invocation) (EntityQuerier, EntityWriter) {
	binder, ok := tx.(PrincipalBinder)
	if !ok {
		return tx, tx
	}
	q, w := binder.BindInvocation(inv)
	if q == nil || w == nil {
		return tx, tx
	}
	return q, w
}

// querierForFetch is querierFor for the two writers that open a socket. The
// plugin's egress policy rides along on the invocation, so the host-side
// downloader can apply layers (b) and (c) to the request it makes.
func (pm *PluginManager) querierForFetch(L *lua.LState, egress NetworkPolicy) EntityQuerier {
	if !pm.stateIsLive(L) {
		return nil
	}
	b := pm.getPrincipalBinder()
	if b == nil {
		// Deliberately NOT the querierFor fallback to the unbound provider.
		// The policy travels on the invocation, so without a binder there is
		// nothing to carry it, and the host-side downloader would fetch with no
		// dial-time deny and no redirect re-check — an unpoliced server-side
		// fetch, which is the hole this whole path exists to close. Every
		// wired-up context installs a binder next to the querier
		// (application_context/context.go), so this is a misconfiguration
		// rather than a mode.
		return nil
	}
	q, _ := b.BindInvocation(pm.invocationFor(L).withEgress(egress))
	return q
}

// writerFor is querierFor for writes. This is the call that makes a plugin's
// creates land with a real CreatedByUserId instead of NULL.
func (pm *PluginManager) writerFor(L *lua.LState) EntityWriter {
	if !pm.stateIsLive(L) {
		return nil
	}
	inv := pm.invocationFor(L)
	if tx := inv.transactionBinding(); tx != nil {
		_, w := bindOntoTransaction(tx, inv)
		return w
	}
	b := pm.getPrincipalBinder()
	if b == nil {
		return pm.getDbWriter()
	}
	_, w := b.BindInvocation(inv)
	if w == nil {
		return pm.getDbWriter()
	}
	return w
}

// loggerFor is getPluginLogger for a call made from L. Inside a
// mah.db.transaction it is the transaction's own handle: mah.log writes a row,
// and a row written on a second connection blocks on the writer lock the
// transaction is holding.
func (pm *PluginManager) loggerFor(L *lua.LState) PluginLogger {
	if tx := pm.invocationFor(L).transactionBinding(); tx != nil {
		return tx
	}
	return pm.getPluginLogger()
}

// loggerForInvocation is loggerFor where there is no LState to read — hook
// dispatch, which has the invocation itself.
func (pm *PluginManager) loggerForInvocation(inv *Invocation) PluginLogger {
	if tx := inv.transactionBinding(); tx != nil {
		return tx
	}
	return pm.getPluginLogger()
}

// MRQLExecutor provides MRQL query execution for plugins.
type MRQLExecutor interface {
	ExecuteMRQL(ctx context.Context, query string, opts MRQLExecOptions) (*MRQLResult, error)
}

// MRQLExecOptions carries execution parameters including scope.
type MRQLExecOptions struct {
	Limit   int               // max items (default 20)
	Buckets int               // max GROUP BY buckets (default 5)
	ScopeID uint              // resolved owner_id for scoping (0 = no scope filter)
	Params  map[string]string // $name placeholder bindings (nil/empty when none)

	// ActorUserID is the user whose call this is, or 0 when there is none
	// (auth off, or a context-less worker path). It is the same uint the rest
	// of the host exchanges — see actor.go — and it is here because mrql_query
	// reaches the database through its own executor rather than through
	// querierFor, so the identity BindInvocation applies never reached it.
	// Without it a group-confined user's hook could ask MRQL for the whole
	// database.
	ActorUserID uint
}

// MRQLResult holds query results in a plugin_system-safe form (no model imports).
type MRQLResult struct {
	EntityType string
	Mode       string // "flat", "aggregated", "bucketed"
	Items      []map[string]any
	Rows       []map[string]any
	Groups     []MRQLResultGroup
}

// MRQLResultGroup is a bucket of items sharing a common key.
type MRQLResultGroup struct {
	Key   map[string]any
	Items []map[string]any
}

// SetMRQLExecutor sets the MRQL query executor for plugins.
func (pm *PluginManager) SetMRQLExecutor(me MRQLExecutor) {
	pm.mrqlExecutor.Store(me)
}

// mrqlExecutorFor is getMRQLExecutor for a call made from L: nil for a revoked
// VM, which the caller already renders as "not available".
//
// mrql_query reaches the database without going through querierFor at all — a
// global or group-scoped query never touches it — so the liveness check on
// those two accessors did not cover this one.
func (pm *PluginManager) mrqlExecutorFor(L *lua.LState) MRQLExecutor {
	if !pm.stateIsLive(L) {
		return nil
	}
	return pm.getMRQLExecutor()
}

// getMRQLExecutor returns the current MRQLExecutor, or nil if not yet set.
func (pm *PluginManager) getMRQLExecutor() MRQLExecutor {
	v := pm.mrqlExecutor.Load()
	if v == nil {
		return nil
	}
	return v.(MRQLExecutor)
}

// checkEntityID reads an entity id argument, rejecting anything that is not a
// positive whole number.
//
// Lua has one number type, so a computed id can arrive fractional — and a plain
// uint() conversion truncates it silently. mah.db.delete_resource(1.9) deleting
// resource 1 is not a rounding error, it is the wrong row.
func checkEntityID(L *lua.LState, argIdx int) uint {
	n := float64(L.CheckNumber(argIdx))
	// 2^53 is where float64 stops representing consecutive integers, so it is
	// the largest id Lua can hold without the value already being approximate.
	// Bounding lower would reject ids the database can legitimately issue.
	if n <= 0 || n != math.Trunc(n) || n > maxLuaExactInteger {
		L.ArgError(argIdx, fmt.Sprintf("entity id must be a positive whole number, got %v", n))
	}
	return uint(n)
}

// maxLuaExactInteger is 2^53-1, the largest integer float64 represents with no
// other integer sharing its bits — JavaScript's MAX_SAFE_INTEGER, and the same
// bound for Lua numbers.
//
// 2^53-1 rather than 2^53 because 2^53+1 is not representable and arrives as
// 2^53: accepting 2^53 would let one specific unrepresentable id through as a
// different row, which is the whole failure this bound exists to stop.
const maxLuaExactInteger = 1<<53 - 1

// idOptionKeys are the option-table keys that name a single entity, and
// idListOptionKeys those that name several. They are validated at the Lua
// boundary for the same reason a positional id is: an id that is not a whole
// number cannot be honoured, and every way of not honouring it is silently
// wrong. Truncating 2.9 picks group 2; treating it as absent clears the owner
// or drops a filter. Raising is the only answer that does not act on a value
// the caller did not mean.
var idOptionKeys = []string{
	"owner_id", "category_id", "note_type_id", "resource_category_id",
	"series_id", "from_group_id", "to_group_id", "relation_type_id",
	"from_category", "to_category", "id",
}

var idListOptionKeys = []string{"tags", "groups", "notes", "resources"}

// validEntityIDValue reports whether a Lua value can be read as an entity id.
// allowZero covers a scalar key like owner_id, where 0 is a deliberate clear;
// list members have no such meaning.
func validEntityIDValue(v lua.LValue, allowZero bool) bool {
	n, ok := v.(lua.LNumber)
	if !ok {
		// A string or a table where an id belongs is not "absent" — the adapter
		// reads it as 0, which clears an owner or empties an association list.
		return false
	}
	f := float64(n)
	if f != math.Trunc(f) || f > maxLuaExactInteger {
		return false
	}
	if allowZero {
		return f >= 0
	}
	return f > 0
}

// checkEntityIDOpts validates every id-shaped entry of an options table.
func checkEntityIDOpts(L *lua.LState, argIdx int, tbl *lua.LTable) {
	if tbl == nil {
		return
	}
	for _, key := range idOptionKeys {
		v := tbl.RawGetString(key)
		if v == lua.LNil {
			continue
		}
		if !validEntityIDValue(v, true) {
			L.ArgError(argIdx, fmt.Sprintf("%s must be a whole number, got %v", key, v))
		}
	}
	for _, key := range idListOptionKeys {
		v := tbl.RawGetString(key)
		if v == lua.LNil {
			continue
		}
		list, ok := v.(*lua.LTable)
		if !ok {
			// Not "no ids": the adapter reads an unusable value as an empty
			// list and clears the association. `tags = "photos"` must fail, not
			// strip every tag.
			L.ArgError(argIdx, fmt.Sprintf("%s must be an array of ids, got %s", key, v.Type()))
		}
		checkEntityIDList(L, argIdx, list)
	}
}

// isArrayLike reports whether tbl is a proper Lua array: consecutive integer
// keys from 1, and nothing else.
//
// It matters because the Go conversion silently drops keys it cannot use. A
// table keyed by anything else — `{[true] = 7}`, `{named = 7}` — converts to an
// empty or differently-shaped value, and a writer reads that as "no ids" and
// clears the association. The shape has to be rejected where it is still
// visible as a mistake.
func isArrayLike(tbl *lua.LTable) bool {
	maxN := tbl.MaxN()
	total := 0
	tbl.ForEach(func(_, _ lua.LValue) { total++ })
	return total == maxN
}

// checkEntityIDList validates an array of entity ids, as the relationship
// writers and the association keys take.
func checkEntityIDList(L *lua.LState, argIdx int, tbl *lua.LTable) {
	if tbl == nil {
		return
	}
	if !isArrayLike(tbl) {
		L.ArgError(argIdx, "entity ids must be given as an array, e.g. {1, 2, 3}")
	}
	tbl.ForEach(func(_, value lua.LValue) {
		if !validEntityIDValue(value, false) {
			L.ArgError(argIdx, fmt.Sprintf("entity ids must be positive whole numbers, got %v", value))
		}
	})
}

// registerDbModule registers the mah.db sub-table in the Lua VM.
// Functions check pm.dbProvider at call time (not at registration) so they
// work even though the provider is set after plugin loading.
//
// grants splits the table: db:read installs everything built on querierFor,
// db:write everything built on writerFor. That is the line the code already
// drew, so every function has an unambiguous side. A plugin granted neither
// gets no mah.db at all.
//
// Every registration goes through setRead or setWrite — never dbMod.RawSetString
// directly, which internal/arch/plugin_capability_gate_test.go enforces, so a
// function added later cannot land on the wrong side of the split by omission.
func (pm *PluginManager) registerDbModule(L *lua.LState, mahMod *lua.LTable, grants CapabilitySet, egress NetworkPolicy) {
	canRead := grants.Has(CapDBRead)
	canWrite := grants.Has(CapDBWrite)
	if !canRead && !canWrite {
		return
	}

	dbMod := L.NewTable()

	setRead := func(name string, fn lua.LGFunction) {
		if canRead {
			dbMod.RawSetString(name, L.NewFunction(fn))
		}
	}
	setWrite := func(name string, fn lua.LGFunction) {
		if canWrite {
			dbMod.RawSetString(name, L.NewFunction(fn))
		}
	}
	// setFetchingWrite is for the two writers that open a socket.
	//
	// They hand a URL to the application's own downloader rather than going
	// through mah.http, so `db:write` alone would leave a plugin a full
	// server-side fetch primitive that its declared `network` list cannot
	// describe — the hole this package exists to close, reached through a
	// different door. A function that opens a socket is a network function
	// whichever module it is filed under, so it needs `http` too, and its URL
	// goes through the same three egress layers.
	setFetchingWrite := func(name string, fn lua.LGFunction) {
		if canWrite && grants.Has(CapHTTP) {
			dbMod.RawSetString(name, L.NewFunction(fn))
		}
	}

	// --- Reads ---
	//
	// Every read returns (value, error). A failure is (nil, error_string); a
	// getter that simply found nothing is (nil, nil). Before this, both arrived
	// as a bare nil — and a failed count arrived as 0 — so a plugin branching on
	// "no rows" took the empty-data branch during an outage, which is
	// destructive for anything that then archives, deletes or re-uploads.
	//
	// The single-value call shape (`local n = mah.db.count_notes{...}`) is
	// unaffected: Lua discards the extra return value.

	// registerGetter: mah.db.X(id) -> table | nil | (nil, error)
	registerGetter := func(name string, fn func(EntityQuerier, uint) (map[string]any, error)) {
		setRead(name, func(L *lua.LState) int {
			db := pm.querierFor(L)
			if db == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("database not available"))
				return 2
			}
			id := checkEntityID(L, 1)
			data, err := fn(db, id)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			if data == nil {
				// Found nothing, and nothing went wrong.
				L.Push(lua.LNil)
				return 1
			}
			L.Push(goToLuaTable(L, data))
			return 1
		})
	}

	// registerLister: mah.db.X(filter) -> array of tables or (nil, error)
	registerLister := func(name string, fn func(EntityQuerier, map[string]any) ([]map[string]any, error)) {
		setRead(name, func(L *lua.LState) int {
			db := pm.querierFor(L)
			if db == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("database not available"))
				return 2
			}
			filterTbl := L.OptTable(1, L.NewTable())
			checkEntityIDOpts(L, 1, filterTbl)
			filter := luaTableToGoMap(filterTbl)
			results, err := fn(db, filter)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			tbl := L.NewTable()
			for i, item := range results {
				tbl.RawSetInt(i+1, goToLuaTable(L, item))
			}
			L.Push(tbl)
			return 1
		})
	}

	// registerCounter: mah.db.X(filter) -> number or (nil, error)
	registerCounter := func(name string, fn func(EntityQuerier, map[string]any) (int64, error)) {
		setRead(name, func(L *lua.LState) int {
			db := pm.querierFor(L)
			if db == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("database not available"))
				return 2
			}
			filterTbl := L.OptTable(1, L.NewTable())
			checkEntityIDOpts(L, 1, filterTbl)
			filter := luaTableToGoMap(filterTbl)
			count, err := fn(db, filter)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LNumber(count))
			return 1
		})
	}

	// mah.db.get_*(id) -> table | nil | (nil, error)
	registerGetter("get_note", func(db EntityQuerier, id uint) (map[string]any, error) { return db.GetNoteData(id) })
	registerGetter("get_resource", func(db EntityQuerier, id uint) (map[string]any, error) { return db.GetResourceData(id) })
	registerGetter("get_group", func(db EntityQuerier, id uint) (map[string]any, error) { return db.GetGroupData(id) })
	registerGetter("get_tag", func(db EntityQuerier, id uint) (map[string]any, error) { return db.GetTagData(id) })
	registerGetter("get_category", func(db EntityQuerier, id uint) (map[string]any, error) { return db.GetCategoryData(id) })
	registerGetter("get_note_type", func(db EntityQuerier, id uint) (map[string]any, error) { return db.GetNoteTypeData(id) })
	registerGetter("get_resource_category", func(db EntityQuerier, id uint) (map[string]any, error) {
		return db.GetResourceCategoryData(id)
	})

	// mah.db.query_notes({name = "meeting%", limit = 10}) -> array of tables
	registerLister("query_notes", func(db EntityQuerier, f map[string]any) ([]map[string]any, error) { return db.QueryNotes(f) })
	// mah.db.query_resources({name = "photo%", content_type = "image/%", limit = 10}) -> array of tables
	registerLister("query_resources", func(db EntityQuerier, f map[string]any) ([]map[string]any, error) {
		return db.QueryResources(f)
	})
	// mah.db.query_groups({name = "team%", limit = 10}) -> array of tables
	registerLister("query_groups", func(db EntityQuerier, f map[string]any) ([]map[string]any, error) { return db.QueryGroups(f) })

	// Taxonomy listing. Without these, the commonest plugin pattern of all —
	// find a tag by name, create it only if it is genuinely absent — was not
	// expressible without hardcoded IDs or a detour through MRQL.
	registerLister("list_tags", func(db EntityQuerier, f map[string]any) ([]map[string]any, error) { return db.ListTags(f) })
	registerLister("list_categories", func(db EntityQuerier, f map[string]any) ([]map[string]any, error) {
		return db.ListCategories(f)
	})
	registerLister("list_note_types", func(db EntityQuerier, f map[string]any) ([]map[string]any, error) {
		return db.ListNoteTypes(f)
	})
	registerLister("list_resource_categories", func(db EntityQuerier, f map[string]any) ([]map[string]any, error) {
		return db.ListResourceCategories(f)
	})

	// mah.db.count_*(filter) -> number or (nil, error)
	registerCounter("count_notes", func(db EntityQuerier, f map[string]any) (int64, error) { return db.CountNotes(f) })
	registerCounter("count_resources", func(db EntityQuerier, f map[string]any) (int64, error) { return db.CountResources(f) })
	registerCounter("count_groups", func(db EntityQuerier, f map[string]any) (int64, error) { return db.CountGroups(f) })

	// mah.db.get_resource_data(id) -> (base64_string, mime_type) or (nil, nil, error)
	//
	// The error is the THIRD value, not the second: success already occupies two
	// slots, and putting it second would hand the caller an error string where
	// it expects a MIME type.
	//
	// Unlike the entity getters, this does not separate "not found" from
	// "failed". A caller is about to use the bytes; if it cannot have them it
	// wants the reason, and "no such resource" is as good a reason as "file too
	// large (max 50MB)" or "storage not available" — all three used to arrive as
	// the same bare nil.
	setRead("get_resource_data", func(L *lua.LState) int {
		// A read, and refused inside a transaction all the same: it pulls the
		// file's bytes off a filesystem that may be an alternative or a remote
		// one, and base64-encodes up to the size cap. The rule is about I/O
		// under the write lock, not about writes — a slow read here stalls every
		// other writer in the process exactly as a slow fetch would.
		if pm.inTransaction(L) {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LString(refusedInTransaction("mah.db.get_resource_data", whyItWaits)))
			return 3
		}
		db := pm.querierFor(L)
		if db == nil {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LString("database not available"))
			return 3
		}
		id := checkEntityID(L, 1)
		base64Data, mimeType, err := db.GetResourceFileData(id)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 3
		}
		L.Push(lua.LString(base64Data))
		L.Push(lua.LString(mimeType))
		return 2
	})

	// mah.db.create_resource_from_url(url, options) -> table or (nil, error)
	setFetchingWrite("create_resource_from_url", func(L *lua.LState) int {
		if pm.inTransaction(L) {
			L.Push(lua.LNil)
			L.Push(lua.LString(refusedInTransaction("mah.db.create_resource_from_url", whyItWaits)))
			return 2
		}
		db := pm.querierForFetch(L, egress)
		if db == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database not available"))
			return 2
		}
		url := L.CheckString(1)
		// Layer (a), applied here rather than in the downloader because
		// AddRemoteResource splits its input on newlines and fetches every
		// line: a check on "the URL" would gate the first and wave through the
		// rest. Checked before anything is created, so a refused batch leaves
		// nothing behind.
		for _, one := range strings.Split(url, "\n") {
			if strings.TrimSpace(one) == "" {
				continue
			}
			if err := checkEgressHost(egress, strings.TrimSpace(one)); err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
		}
		opts := make(map[string]any)
		if optTbl := L.OptTable(2, nil); optTbl != nil {
			checkEntityIDOpts(L, 2, optTbl)
			opts = luaTableToGoMap(optTbl)
		}
		// The invocation's context, so the fetch is bounded by whatever budget
		// the caller already has. This is the last mah.db call that could hold
		// the plugin's VM lock for the host's full remote timeout; mah.http's
		// sync path was capped the same way (effectiveSyncTimeout).
		result, err := db.CreateResourceFromURL(pm.luaContext(L), url, opts)
		InvalidateMRQLCache(pm.luaContext(L))
		if err != nil {
			logEgressRefusal(err, pm.pluginNameFor(L), "GET", url)
			L.Push(lua.LNil)
			// Sanitized like the mah.http paths: the host-side downloader wraps
			// a dial refusal, and both our own message and Go's *net.OpError
			// prefix carry the RESOLVED address. Handing that to Lua turns
			// every refusal into an internal DNS map — and this door needs no
			// `network` list at all to be walked, since an absent list is
			// unrestricted and so passes layer (a) for every name.
			L.Push(lua.LString(egressErrorForPlugin(err)))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	})

	// mah.db.create_resource_from_data(base64, options) -> table or (nil, error)
	setWrite("create_resource_from_data", func(L *lua.LState) int {
		if pm.inTransaction(L) {
			L.Push(lua.LNil)
			L.Push(lua.LString(refusedInTransaction("mah.db.create_resource_from_data", whyItWaits)))
			return 2
		}
		db := pm.querierFor(L)
		if db == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database not available"))
			return 2
		}
		base64Data := L.CheckString(1)
		opts := make(map[string]any)
		if optTbl := L.OptTable(2, nil); optTbl != nil {
			checkEntityIDOpts(L, 2, optTbl)
			opts = luaTableToGoMap(optTbl)
		}
		result, err := db.CreateResourceFromData(base64Data, opts)
		InvalidateMRQLCache(pm.luaContext(L))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	})

	// mah.db.add_resource_version_from_url(resource_id, url, comment) -> table or (nil, error)
	setFetchingWrite("add_resource_version_from_url", func(L *lua.LState) int {
		if pm.inTransaction(L) {
			L.Push(lua.LNil)
			L.Push(lua.LString(refusedInTransaction("mah.db.add_resource_version_from_url", whyItWaits)))
			return 2
		}
		db := pm.querierForFetch(L, egress)
		if db == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database not available"))
			return 2
		}
		resourceID := checkEntityID(L, 1)
		url := L.CheckString(2)
		if err := checkEgressHost(egress, url); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		comment := L.OptString(3, "")
		result, err := db.AddResourceVersionFromURL(resourceID, url, comment)
		InvalidateMRQLCache(pm.luaContext(L))
		if err != nil {
			logEgressRefusal(err, pm.pluginNameFor(L), "GET", url)
			L.Push(lua.LNil)
			// Sanitized: see create_resource_from_url above.
			L.Push(lua.LString(egressErrorForPlugin(err)))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	})

	// --- Entity CRUD + Patch ---
	// Helper-based registration to avoid boilerplate.

	// Entities with (id, opts) signature for update/patch
	type idOptsFunc = func(EntityWriter, uint, map[string]any) (map[string]any, error)
	// Entities with (opts) signature for create/update/patch (id embedded in opts)
	type optsFunc = func(EntityWriter, map[string]any) (map[string]any, error)
	// Delete functions
	type deleteFunc = func(EntityWriter, uint) error

	// registerOptsWriter: mah.db.X(opts) -> table or (nil, error)
	registerOptsWriter := func(name string, fn optsFunc) {
		setWrite(name, func(L *lua.LState) int {
			w := pm.writerFor(L)
			if w == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("database writer not available"))
				return 2
			}
			optsTbl := L.CheckTable(1)
			checkEntityIDOpts(L, 1, optsTbl)
			opts := luaTableToGoMap(optsTbl)
			result, err := fn(w, opts)
			InvalidateMRQLCache(pm.luaContext(L))
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(goToLuaTable(L, result))
			return 1
		})
	}

	// registerIdOptsWriter: mah.db.X(id, opts) -> table or (nil, error)
	registerIdOptsWriter := func(name string, fn idOptsFunc) {
		setWrite(name, func(L *lua.LState) int {
			w := pm.writerFor(L)
			if w == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("database writer not available"))
				return 2
			}
			id := checkEntityID(L, 1)
			optsTbl := L.CheckTable(2)
			checkEntityIDOpts(L, 2, optsTbl)
			opts := luaTableToGoMap(optsTbl)
			result, err := fn(w, id, opts)
			InvalidateMRQLCache(pm.luaContext(L))
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(goToLuaTable(L, result))
			return 1
		})
	}

	// registerDelete: mah.db.X(id) -> true or (nil, error)
	registerDelete := func(name string, fn deleteFunc) {
		setWrite(name, func(L *lua.LState) int {
			w := pm.writerFor(L)
			if w == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("database writer not available"))
				return 2
			}
			id := checkEntityID(L, 1)
			err := fn(w, id)
			InvalidateMRQLCache(pm.luaContext(L))
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LTrue)
			return 1
		})
	}

	// Group
	registerOptsWriter("create_group", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.CreateGroup(o) })
	registerIdOptsWriter("update_group", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) { return w.UpdateGroup(id, o) })
	registerIdOptsWriter("patch_group", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) { return w.PatchGroup(id, o) })
	registerDelete("delete_group", func(w EntityWriter, id uint) error { return w.DeleteGroup(id) })

	// Note
	registerOptsWriter("create_note", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.CreateNote(o) })
	registerIdOptsWriter("update_note", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) { return w.UpdateNote(id, o) })
	registerIdOptsWriter("patch_note", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) { return w.PatchNote(id, o) })
	registerDelete("delete_note", func(w EntityWriter, id uint) error { return w.DeleteNote(id) })

	// Tag
	registerOptsWriter("create_tag", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.CreateTag(o) })
	registerIdOptsWriter("update_tag", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) { return w.UpdateTag(id, o) })
	registerIdOptsWriter("patch_tag", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) { return w.PatchTag(id, o) })
	registerDelete("delete_tag", func(w EntityWriter, id uint) error { return w.DeleteTag(id) })

	// Category
	registerOptsWriter("create_category", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.CreateCategory(o) })
	registerIdOptsWriter("update_category", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) {
		return w.UpdateCategory(id, o)
	})
	registerIdOptsWriter("patch_category", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) { return w.PatchCategory(id, o) })
	registerDelete("delete_category", func(w EntityWriter, id uint) error { return w.DeleteCategory(id) })

	// ResourceCategory
	registerOptsWriter("create_resource_category", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.CreateResourceCategory(o) })
	registerIdOptsWriter("update_resource_category", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) {
		return w.UpdateResourceCategory(id, o)
	})
	registerIdOptsWriter("patch_resource_category", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) {
		return w.PatchResourceCategory(id, o)
	})
	registerDelete("delete_resource_category", func(w EntityWriter, id uint) error { return w.DeleteResourceCategory(id) })

	// NoteType
	registerOptsWriter("create_note_type", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.CreateNoteType(o) })
	registerIdOptsWriter("update_note_type", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) {
		return w.UpdateNoteType(id, o)
	})
	registerIdOptsWriter("patch_note_type", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) { return w.PatchNoteType(id, o) })
	registerDelete("delete_note_type", func(w EntityWriter, id uint) error { return w.DeleteNoteType(id) })

	// GroupRelation (id embedded in opts for update/patch)
	registerOptsWriter("create_group_relation", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.CreateGroupRelation(o) })
	registerOptsWriter("update_group_relation", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.UpdateGroupRelation(o) })
	registerOptsWriter("patch_group_relation", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.PatchGroupRelation(o) })
	registerDelete("delete_group_relation", func(w EntityWriter, id uint) error { return w.DeleteGroupRelation(id) })

	// RelationType (id embedded in opts for update/patch)
	registerOptsWriter("create_relation_type", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.CreateRelationType(o) })
	registerOptsWriter("update_relation_type", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.UpdateRelationType(o) })
	registerOptsWriter("patch_relation_type", func(w EntityWriter, o map[string]any) (map[string]any, error) { return w.PatchRelationType(o) })
	registerDelete("delete_relation_type", func(w EntityWriter, id uint) error { return w.DeleteRelationType(id) })

	// Resource. Creation lives above (from URL or from base64 data), because it
	// has to put bytes on a filesystem; edit and delete are ordinary writes.
	// patch ships with update deliberately: update replaces every field,
	// associations included, so patching is the only safe way to change one
	// thing about a resource.
	//
	// One exception to replace-all, inherited from EditResource and shared with
	// the HTTP edit path: an empty meta, width or height is ignored rather than
	// written, so those three cannot be cleared. Pass "{}" to empty meta.
	registerIdOptsWriter("update_resource", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) {
		return w.UpdateResource(id, o)
	})
	registerIdOptsWriter("patch_resource", func(w EntityWriter, id uint, o map[string]any) (map[string]any, error) {
		return w.PatchResource(id, o)
	})
	// Not registerDelete: delete_resource is the one writer whose effects are
	// not all in the database. DeleteResource removes the file after its own
	// writes commit — which inside a transaction is a savepoint release, not a
	// commit — so a later rollback restores the row and leaves the bytes gone.
	setWrite("delete_resource", func(L *lua.LState) int {
		w := pm.writerFor(L)
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		if pm.inTransaction(L) {
			L.Push(lua.LNil)
			L.Push(lua.LString(refusedInTransaction("mah.db.delete_resource", whyItCannotBeUndone)))
			return 2
		}
		id := checkEntityID(L, 1)
		err := w.DeleteResource(id)
		InvalidateMRQLCache(pm.luaContext(L))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	})

	// --- Relationship management ---

	// mah.db.add_tags(entity_type, id, tag_ids) -> true or (nil, error)
	setWrite("add_tags", func(L *lua.LState) int {
		w := pm.writerFor(L)
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		entityType := L.CheckString(1)
		id := checkEntityID(L, 2)
		idsTbl := L.CheckTable(3)
		checkEntityIDList(L, 3, idsTbl)
		ids := luaTableToUintSlice(idsTbl)
		err := w.AddTagsToEntity(entityType, id, ids)
		InvalidateMRQLCache(pm.luaContext(L))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	})

	// mah.db.remove_tags(entity_type, id, tag_ids) -> true or (nil, error)
	setWrite("remove_tags", func(L *lua.LState) int {
		w := pm.writerFor(L)
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		entityType := L.CheckString(1)
		id := checkEntityID(L, 2)
		idsTbl := L.CheckTable(3)
		checkEntityIDList(L, 3, idsTbl)
		ids := luaTableToUintSlice(idsTbl)
		err := w.RemoveTagsFromEntity(entityType, id, ids)
		InvalidateMRQLCache(pm.luaContext(L))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	})

	// mah.db.add_groups(entity_type, id, group_ids) -> true or (nil, error)
	setWrite("add_groups", func(L *lua.LState) int {
		w := pm.writerFor(L)
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		entityType := L.CheckString(1)
		id := checkEntityID(L, 2)
		idsTbl := L.CheckTable(3)
		checkEntityIDList(L, 3, idsTbl)
		ids := luaTableToUintSlice(idsTbl)
		err := w.AddGroupsToEntity(entityType, id, ids)
		InvalidateMRQLCache(pm.luaContext(L))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	})

	// mah.db.remove_groups(entity_type, id, group_ids) -> true or (nil, error)
	setWrite("remove_groups", func(L *lua.LState) int {
		w := pm.writerFor(L)
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		entityType := L.CheckString(1)
		id := checkEntityID(L, 2)
		idsTbl := L.CheckTable(3)
		checkEntityIDList(L, 3, idsTbl)
		ids := luaTableToUintSlice(idsTbl)
		err := w.RemoveGroupsFromEntity(entityType, id, ids)
		InvalidateMRQLCache(pm.luaContext(L))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	})

	// mah.db.add_resources_to_note(note_id, resource_ids) -> true or (nil, error)
	setWrite("add_resources_to_note", func(L *lua.LState) int {
		w := pm.writerFor(L)
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		noteId := checkEntityID(L, 1)
		idsTbl := L.CheckTable(2)
		checkEntityIDList(L, 2, idsTbl)
		ids := luaTableToUintSlice(idsTbl)
		err := w.AddResourcesToNote(noteId, ids)
		InvalidateMRQLCache(pm.luaContext(L))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	})

	// mah.db.remove_resources_from_note(note_id, resource_ids) -> true or (nil, error)
	setWrite("remove_resources_from_note", func(L *lua.LState) int {
		w := pm.writerFor(L)
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		noteId := checkEntityID(L, 1)
		idsTbl := L.CheckTable(2)
		checkEntityIDList(L, 2, idsTbl)
		ids := luaTableToUintSlice(idsTbl)
		err := w.RemoveResourcesFromNote(noteId, ids)
		InvalidateMRQLCache(pm.luaContext(L))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	})

	// mah.db.transaction(fn) -> true or (nil, error)
	//
	// Every mah.db, mah.kv and mah.log call made while fn runs — including one
	// made by another plugin's hook that fn's writes fire — joins one database
	// transaction. fn raising an error rolls all of it back; mah.abort still
	// aborts. What is refused inside it is listed at refusedInTransaction.
	//
	// A write registration because it is one: CapWrite is the capability that
	// decides whether a plugin may change anything, and a transaction is only
	// ever a way of changing several things at once.
	setWrite("transaction", func(L *lua.LState) int {
		// Availability is asked of the same accessor every other write uses, so
		// a revoked VM refuses here exactly as it refuses the writes inside.
		if w := pm.writerFor(L); w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		fn := L.CheckFunction(1)
		return pm.runDbTransaction(L, fn)
	})

	// mah.db.mrql_query(query, opts) -> result_table or (nil, error_string)
	setRead("mrql_query", func(L *lua.LState) int {
		// Arguments are checked before availability: a malformed call is
		// malformed whether or not the executor happens to be wired up, and
		// answering "not available" would hide the real mistake.
		query := L.CheckString(1)
		optsTbl := L.OptTable(2, L.NewTable())
		// scope_entity_id names an entity like any other id, and truncating it
		// scopes the query to a different group's subtree.
		// Zero is allowed and means "no entity" — a shortcode rendering outside
		// an entity context passes ctx.entity_id straight through, and it is 0
		// there. What is rejected is a value that cannot be an id at all, which
		// would otherwise truncate into a different group's subtree.
		if v := optsTbl.RawGetString("scope_entity_id"); v != lua.LNil && !validEntityIDValue(v, true) {
			L.ArgError(2, fmt.Sprintf("scope_entity_id must be a whole number, got %v", v))
		}

		executor := pm.mrqlExecutorFor(L)
		if executor == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("MRQL executor not available"))
			return 2
		}

		optsMap := luaTableToGoMap(optsTbl)

		// Extract options
		limit := 20
		if v, ok := optsMap["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		buckets := 5
		if v, ok := optsMap["buckets"].(float64); ok && v > 0 {
			buckets = int(v)
		}

		// Scope resolution
		var scopeID uint
		scopeEntityID := uint(0)
		if v, ok := optsMap["scope_entity_id"].(float64); ok {
			scopeEntityID = uint(v)
		}
		scopeStr := "entity"
		if v, ok := optsMap["scope"].(string); ok && v != "" {
			scopeStr = v
		}

		// Sentinel that matches no real owner_id — used when a non-global
		// scope cannot be resolved, to guarantee empty results instead of
		// fanning out to the entire dataset.
		unresolvedSentinel := ^uint(0) >> 1

		switch scopeStr {
		case "global":
			scopeID = 0
		case "parent":
			db := pm.querierFor(L)
			if db == nil || scopeEntityID == 0 {
				scopeID = unresolvedSentinel
			} else {
				entityType := ""
				if v, ok := optsMap["entity_type"].(string); ok {
					entityType = v
				}
				scopeID = resolveParentScope(db, scopeEntityID, entityType)
			}
		case "root":
			db := pm.querierFor(L)
			if db == nil || scopeEntityID == 0 {
				scopeID = unresolvedSentinel
			} else {
				entityType := ""
				if v, ok := optsMap["entity_type"].(string); ok {
					entityType = v
				}
				scopeID = resolveRootScope(db, scopeEntityID, entityType)
			}
		default: // "entity"
			entityType := ""
			if v, ok := optsMap["entity_type"].(string); ok {
				entityType = v
			}
			// owner_id always points to a group. For group entities, the entity
			// ID itself is a valid owner_id. For resources/notes, using the raw
			// entity ID would match an unrelated group with the same numeric ID.
			// Instead, use the entity's OwnerId (its parent group).
			if entityType == "group" {
				scopeID = scopeEntityID
			} else if scopeEntityID > 0 {
				db := pm.querierFor(L)
				if db != nil {
					ownerID := lookupOwnerViaQuerier(db, scopeEntityID, entityType)
					if ownerID > 0 {
						scopeID = ownerID
					} else {
						scopeID = unresolvedSentinel
					}
				} else {
					scopeID = unresolvedSentinel
				}
			}
		}

		// Parameter placeholder bindings (opts.params table). Values are
		// stringified; the executor lenient-coerces them like typed literals.
		var params map[string]string
		if pv, ok := optsMap["params"].(map[string]any); ok && len(pv) > 0 {
			params = make(map[string]string, len(pv))
			for k, v := range pv {
				params[k] = fmt.Sprint(v)
			}
		}

		actor := pm.actorFor(L)
		execOpts := MRQLExecOptions{
			Limit:       limit,
			Buckets:     buckets,
			ScopeID:     scopeID,
			Params:      params,
			ActorUserID: actor,
		}

		// Check cache
		reqCtx := pm.luaContext(L)
		if reqCtx == nil {
			reqCtx = context.Background()
		}
		cacheKey := MRQLCacheKey(query, scopeID, limit, buckets, params, actor)
		if cache := MRQLCacheFromContext(reqCtx); cache != nil {
			if cached, ok := cache.Get(cacheKey); ok {
				L.Push(mrqlResultToLua(L, cached))
				return 1
			}
		}

		result, err := executor.ExecuteMRQL(reqCtx, query, execOpts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Cache the result
		if cache := MRQLCacheFromContext(reqCtx); cache != nil {
			cache.Put(cacheKey, result)
		}

		L.Push(mrqlResultToLua(L, result))
		return 1
	})

	mahMod.RawSetString("db", dbMod)
}

// resolveParentScope looks up the owner_id of an entity.
func resolveParentScope(db EntityQuerier, entityID uint, entityType string) uint {
	var data map[string]any
	var err error
	switch entityType {
	case "group":
		data, err = db.GetGroupData(entityID)
	case "resource":
		data, err = db.GetResourceData(entityID)
	case "note":
		data, err = db.GetNoteData(entityID)
	default:
		data, err = db.GetGroupData(entityID)
	}
	if err != nil || data == nil {
		return ^uint(0) >> 1 // sentinel: ensures empty results
	}
	if ownerID, ok := data["owner_id"].(float64); ok && ownerID > 0 {
		return uint(ownerID)
	}
	return ^uint(0) >> 1
}

// resolveRootScope walks the ownership chain to find the root.
// entityType is needed for the first hop (resource/note/group have different
// lookup methods), after which we walk groups only.
func resolveRootScope(db EntityQuerier, entityID uint, entityType string) uint {
	// First hop: look up the entity's owner using its actual type
	ownerID := lookupOwnerViaQuerier(db, entityID, entityType)
	if ownerID == 0 {
		// Entity has no owner. For groups, the entity itself is a valid
		// owner_id. For resources/notes, returning the raw entity ID would
		// collide with an unrelated group — use sentinel instead.
		if entityType == "group" {
			return entityID
		}
		return ^uint(0) >> 1
	}
	current := ownerID
	for i := 0; i < 50; i++ {
		data, err := db.GetGroupData(current)
		if err != nil || data == nil {
			return current
		}
		parentID, ok := data["owner_id"].(float64)
		if !ok || parentID <= 0 {
			return current
		}
		current = uint(parentID)
	}
	return current
}

// lookupOwnerViaQuerier returns the owner_id of an entity using EntityQuerier.
func lookupOwnerViaQuerier(db EntityQuerier, entityID uint, entityType string) uint {
	var data map[string]any
	var err error
	switch entityType {
	case "resource":
		data, err = db.GetResourceData(entityID)
	case "note":
		data, err = db.GetNoteData(entityID)
	default:
		data, err = db.GetGroupData(entityID)
	}
	if err != nil || data == nil {
		return 0
	}
	if ownerID, ok := data["owner_id"].(float64); ok && ownerID > 0 {
		return uint(ownerID)
	}
	return 0
}

// mrqlResultToLua converts an MRQLResult to a Lua table.
func mrqlResultToLua(L *lua.LState, result *MRQLResult) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("mode", lua.LString(result.Mode))
	tbl.RawSetString("entity_type", lua.LString(result.EntityType))

	switch result.Mode {
	case "flat":
		items := L.NewTable()
		for i, item := range result.Items {
			items.RawSetInt(i+1, goToLuaTable(L, item))
		}
		tbl.RawSetString("items", items)
	case "aggregated":
		rows := L.NewTable()
		for i, row := range result.Rows {
			rows.RawSetInt(i+1, goToLuaTable(L, row))
		}
		tbl.RawSetString("rows", rows)
	case "bucketed":
		groups := L.NewTable()
		for i, g := range result.Groups {
			groupTbl := L.NewTable()
			groupTbl.RawSetString("key", goToLuaTable(L, g.Key))
			items := L.NewTable()
			for j, item := range g.Items {
				items.RawSetInt(j+1, goToLuaTable(L, item))
			}
			groupTbl.RawSetString("items", items)
			groups.RawSetInt(i+1, groupTbl)
		}
		tbl.RawSetString("groups", groups)
	}

	return tbl
}

// luaTableToUintSlice converts a Lua table (array of numbers) to []uint.
func luaTableToUintSlice(tbl *lua.LTable) []uint {
	var result []uint
	tbl.ForEach(func(_, value lua.LValue) {
		if n, ok := value.(lua.LNumber); ok && float64(n) > 0 {
			result = append(result, uint(n))
		}
	})
	return result
}
