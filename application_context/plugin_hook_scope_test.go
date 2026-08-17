package application_context

import (
	"context"
	"sort"
	"strings"
	"testing"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The plugin-path deny (auth.PluginCodeAllowed) matches URL paths: a
// group-confined principal cannot reach /plugins/… or /v1/plugins/…, and cannot
// render a shortcode. Hooks are not a URL path. They fire from ordinary scoped
// CRUD, which a confined user is allowed to perform, and the hook's mah.db calls
// run on whatever handle BindInvocation produced.
//
// So the question these tests answer is not "can a confined user call a plugin"
// but "what does a plugin see when a confined user's own write wakes it up".
//
// scopeProbePlugin registers an after_note_create hook that asks mah.db what
// groups, notes and resources exist, and stashes the names on the Lua side. The
// counters never touch the database, so they are readable through the probe slot
// whatever the hook's own writes did.
func scopeProbePlugin() string {
	return `plugin = { name = "hooktest", version = "1.0", description = "reports what a hook can see" }
local seen = { groups = {}, notes = {}, resources = {}, mrql = {} }

local function names(rows)
    local out = {}
    for _, row in ipairs(rows or {}) do
        out[#out + 1] = row.name
    end
    table.sort(out)
    return out
end

function init()
    mah.on("after_note_create", function(data)
        seen.groups = names(mah.db.query_groups({ limit = 100 }))
        seen.notes = names(mah.db.query_notes({ limit = 100 }))
        seen.resources = names(mah.db.query_resources({ limit = 100 }))
        -- mah.db.mrql_query reaches the database through a different adapter
        -- than every other mah.db function, so it needs its own probe.
        local result = mah.db.mrql_query("type=group", { limit = 100, scope = "global" })
        seen.mrql = names(result and result.items)
        return data
    end)

    mah.inject("probe", function(ctx)
        local parts = {}
        for _, kind in ipairs({ "groups", "notes", "resources", "mrql" }) do
            parts[#parts + 1] = kind .. ":" .. table.concat(seen[kind], "|")
        end
        return table.concat(parts, " ")
    end)
end
`
}

// hookSaw reads back what the hook's mah.db call returned for one entity kind.
func hookSaw(t *testing.T, ctx *MahresourcesContext, kind string) []string {
	t.Helper()
	out := ctx.PluginManager().RenderSlot(context.Background(), "probe", map[string]any{})
	for _, part := range strings.Fields(out) {
		key, value, ok := strings.Cut(part, ":")
		if !ok || key != kind {
			continue
		}
		if value == "" {
			return nil
		}
		names := strings.Split(value, "|")
		sort.Strings(names)
		return names
	}
	t.Fatalf("probe slot returned %q, which has no %s entry", out, kind)
	return nil
}

// scopeProbeFixture builds two disjoint group subtrees, puts one note and one
// resource in each, and returns a principal confined to "inside".
//
// The user's role is RoleUser rather than RoleGuest deliberately: a guest cannot
// write at all, so a guest can never trigger a create hook. A scope-limited
// *user* is the principal that both is confined and can perform the write that
// wakes the plugin up, which makes it the only role this defect is reachable
// through.
func scopeProbeFixture(t *testing.T, ctx *MahresourcesContext) (*auth.Principal, *models.Group) {
	t.Helper()

	inside := &models.Group{Name: "inside"}
	outside := &models.Group{Name: "outside"}
	for _, g := range []*models.Group{inside, outside} {
		if err := ctx.db.Create(g).Error; err != nil {
			t.Fatalf("create group %s: %v", g.Name, err)
		}
	}

	for _, spec := range []struct {
		name  string
		owner *models.Group
	}{{"inside-note", inside}, {"outside-note", outside}} {
		note := &models.Note{Name: spec.name, OwnerId: &spec.owner.ID}
		if err := ctx.db.Create(note).Error; err != nil {
			t.Fatalf("create note %s: %v", spec.name, err)
		}
	}

	for _, spec := range []struct {
		name  string
		owner *models.Group
	}{{"inside-res", inside}, {"outside-res", outside}} {
		res := &models.Resource{
			Name:     spec.name,
			Hash:     "hash-" + spec.name,
			Location: "loc-" + spec.name,
			OwnerId:  &spec.owner.ID,
		}
		if err := ctx.db.Create(res).Error; err != nil {
			t.Fatalf("create resource %s: %v", spec.name, err)
		}
	}

	user, err := ctx.CreateUser(&UserInput{
		Username:     "confined",
		Password:     "password1",
		Role:         models.RoleUser,
		ScopeGroupId: &inside.ID,
	})
	if err != nil {
		t.Fatalf("create confined user: %v", err)
	}
	return auth.FromUser(user), inside
}

// A confined principal's own write must not hand the plugin a wider view of the
// database than the principal has. BindInvocation rebuilds the principal from
// the actor id alone, so the role and the scope group are dropped on the way
// into the VM: the reconstructed principal is neither IsScoped nor
// RequiresScope, applyPrincipalScope adds no subtree filter, and every mah.db
// read in the hook runs unscoped.
func TestHookInvocation_ConfinedPrincipalDoesNotWidenPluginReads(t *testing.T) {
	ctx := newPluginHookTestContext(t, scopeProbePlugin())
	principal, inside := scopeProbeFixture(t, ctx)

	scoped := ctx.WithPrincipal(principal)
	if _, err := scoped.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "trigger", OwnerId: inside.ID},
	}); err != nil {
		t.Fatalf("confined user could not create a note in its own subtree: %v", err)
	}

	for _, tc := range []struct {
		kind   string
		denied string
	}{
		{"groups", "outside"},
		{"notes", "outside-note"},
		{"resources", "outside-res"},
		{"mrql", "outside"},
	} {
		saw := hookSaw(t, ctx, tc.kind)
		if len(saw) == 0 {
			t.Errorf("%s: the hook saw nothing at all, so this proves nothing about scoping", tc.kind)
			continue
		}
		for _, name := range saw {
			if name == tc.denied {
				t.Errorf("%s: hook triggered by a principal confined to %q saw %q, which is outside that subtree (saw %v)",
					tc.kind, inside.Name, tc.denied, saw)
			}
		}
	}
}

// The other half of the same rule: confining the principal must not blind the
// plugin to what that principal legitimately reaches. A fix that simply refused
// every hook-originated read would pass the test above and be useless.
func TestHookInvocation_ConfinedPrincipalStillSeesItsOwnSubtree(t *testing.T) {
	ctx := newPluginHookTestContext(t, scopeProbePlugin())
	principal, inside := scopeProbeFixture(t, ctx)

	scoped := ctx.WithPrincipal(principal)
	if _, err := scoped.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "trigger", OwnerId: inside.ID},
	}); err != nil {
		t.Fatalf("confined user could not create a note in its own subtree: %v", err)
	}

	for _, tc := range []struct {
		kind   string
		wanted string
	}{
		{"groups", "inside"},
		{"notes", "inside-note"},
		{"resources", "inside-res"},
		{"mrql", "inside"},
	} {
		saw := hookSaw(t, ctx, tc.kind)
		found := false
		for _, name := range saw {
			if name == tc.wanted {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: hook did not see %q, which is inside the triggering principal's own scope (saw %v)",
				tc.kind, tc.wanted, saw)
		}
	}
}

// A plugin call can outlive the request that started it — an async job or a
// drained HTTP callback carries only its submitter's id and runs later — so the
// account can be gone or disabled by the time the VM reaches the database. Not
// knowing what a caller may see must not resolve to "everything".
func TestPrincipalForPluginActor_UnresolvableActorIsDeniedAllScopedData(t *testing.T) {
	ctx := newPluginHookTestContext(t, scopeProbePlugin())
	_, inside := scopeProbeFixture(t, ctx)

	disabled, err := ctx.CreateUser(&UserInput{
		Username: "gone", Password: "password1", Role: models.RoleEditor, Disabled: true,
	})
	if err != nil {
		t.Fatalf("create disabled user: %v", err)
	}

	for _, tc := range []struct {
		name  string
		actor uint
	}{
		{"deleted account", 4242},
		{"disabled account", disabled.ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bound := ctx.WithPrincipal(ctx.principalForPluginActor(tc.actor))

			groups, err := bound.GetGroups(0, 100, &query_models.GroupQuery{})
			if err != nil {
				t.Fatalf("GetGroups: %v", err)
			}
			if len(groups) != 0 {
				t.Errorf("an unresolvable actor read %d groups; it must match none", len(groups))
			}

			// Not merely "the subtree it was scoped to": nothing at all,
			// including the group a real confined principal would be allowed.
			if bound.GroupVisible(inside.ID) {
				t.Errorf("an unresolvable actor can see group %d", inside.ID)
			}
		})
	}
}

// The deny is logged only when it is an outage, never when it is the ordinary
// case.
//
// GetUser maps gorm.ErrRecordNotFound to ErrUserNotFound rather than returning
// (nil, nil), so a deleted account arrives on the same err != nil branch a
// failed read does. Logging that branch unconditionally would write one row per
// mah.db call for the case the design documents as expected — an admin deleting
// a user while that user's async job is still looping — and it is exactly the
// case where the write succeeds, since the database is healthy.
func TestPrincipalForPluginActor_ExpectedRefusalsAreNotLogged(t *testing.T) {
	ctx := newPluginHookTestContext(t, scopeProbePlugin())

	disabled, err := ctx.CreateUser(&UserInput{
		Username: "quiet", Password: "password1", Role: models.RoleEditor, Disabled: true,
	})
	if err != nil {
		t.Fatalf("create disabled user: %v", err)
	}

	// entity_name, not message: Logger.Warning's fourth argument is the entity
	// name and the fifth is the message, so the label lands in entity_name and
	// the detail in message. Querying the wrong column made an earlier draft of
	// this test count zero rows unconditionally, which is to say pass whatever
	// the code did — the mutation run below is what caught it.
	countActorLogs := func() int64 {
		var n int64
		if err := ctx.db.Model(&models.LogEntry{}).
			Where("entity_name = ?", "Plugin actor unresolved").Count(&n).Error; err != nil {
			t.Fatalf("count log entries: %v", err)
		}
		return n
	}

	before := countActorLogs()
	for i := 0; i < 5; i++ {
		// A deleted account, five times over, standing in for a job that loops.
		if p := ctx.principalForPluginActor(4242); p == nil || p.Role != models.RoleGuest {
			t.Fatalf("deleted actor was not denied: %+v", p)
		}
		if p := ctx.principalForPluginActor(disabled.ID); p == nil || p.Role != models.RoleGuest {
			t.Fatalf("disabled actor was not denied: %+v", p)
		}
	}
	if after := countActorLogs(); after != before {
		t.Errorf("an expected refusal wrote %d log entries; a job looping over mah.db would fill the log with them",
			after-before)
	}
}

// An unscoped actor must keep the view it has always had. This is the regression
// guard on the fix: binding the principal properly must not accidentally confine
// an editor or an admin.
func TestHookInvocation_UnscopedPrincipalSeesEverything(t *testing.T) {
	ctx := newPluginHookTestContext(t, scopeProbePlugin())
	_, inside := scopeProbeFixture(t, ctx)

	editor, err := ctx.CreateUser(&UserInput{Username: "ed", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create editor: %v", err)
	}

	scoped := ctx.WithPrincipal(auth.FromUser(editor))
	if _, err := scoped.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "trigger", OwnerId: inside.ID},
	}); err != nil {
		t.Fatalf("editor could not create a note: %v", err)
	}

	for _, tc := range []struct {
		kind   string
		wanted string
	}{
		{"groups", "outside"},
		{"notes", "outside-note"},
		{"resources", "outside-res"},
		{"mrql", "outside"},
	} {
		saw := hookSaw(t, ctx, tc.kind)
		found := false
		for _, name := range saw {
			if name == tc.wanted {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: an editor's hook should still see %q (saw %v)", tc.kind, tc.wanted, saw)
		}
	}
}
