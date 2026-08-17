package application_context

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newRoleCapabilityTestContext builds a context with its own database. The
// AutoMigrate list is written out here rather than shared because it needs
// TemplatePartial, which the plugin-hook fixtures do not migrate.
func newRoleCapabilityTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.ResourceCategory{},
		&models.Series{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.TemplatePartial{},
		&models.LogEntry{},
		&models.User{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	sqlDB, _ := db.DB()
	return NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
}

var roleCapabilityNameCounter atomic.Uint64

// randomSuffix keeps names unique across the role × operation matrix, so a
// refusal is never confused with a unique-name conflict.
func randomSuffix() string {
	return fmt.Sprintf("%d", roleCapabilityNameCounter.Add(1))
}

// Role capability is decided in server/authz_policy.go and nowhere else, so an
// operation reached from below server/ — a plugin hook, a plugin action, a
// plugin page — has always been able to perform work the caller's own role
// forbids at the HTTP layer. Scope did not cover it: tags, categories, note
// types and relation types carry no owner, so scopeColumn maps none of them and
// a subtree filter has nothing to filter on.
//
// These tests pin the rule at the operations rather than at the tables, which is
// the distinction the whole guard turns on. A plain user's upload find-or-creates
// a Category (AddRemoteResource), and group import creates and renames them in
// bulk; both are legitimate at capWrite. What is admin-only is the *operation*
// the HTTP layer gates — CreateCategory — and that is what is guarded here.
func TestTaxonomyOperations_RequireTheRoleTheHTTPLayerRequires(t *testing.T) {
	ctx := newRoleCapabilityTestContext(t)

	// Seeded through the unbound singleton, which the guard leaves alone — so
	// the fixture itself is also the proof that an identity-less write works.
	catA, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: "seed-a"})
	if err != nil {
		t.Fatalf("seeding a category as the unbound singleton must work: %v", err)
	}
	catB, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: "seed-b"})
	if err != nil {
		t.Fatalf("seeding a category: %v", err)
	}
	for _, name := range []string{"group-a", "group-b"} {
		g := &models.Group{Name: name, CategoryId: &catA.ID}
		if err := ctx.db.Create(g).Error; err != nil {
			t.Fatalf("seeding group %s: %v", name, err)
		}
	}
	relType, err := ctx.AddRelationType(&query_models.RelationshipTypeEditorQuery{
		Name: "seed-type", FromCategory: catA.ID, ToCategory: catB.ID,
	})
	if err != nil {
		t.Fatalf("seeding a relation type as the unbound singleton must work: %v", err)
	}

	// Each operation is named with the capability server/ requires for the route
	// that calls it: admin for Category, ResourceCategory and TemplatePartial
	// (isTaxonomyPath), editor for NoteType, relation types and relation edges
	// (isEditorPath).
	//
	// Relation edges are here even though relationInScope already confines them,
	// because scope and capability answer different questions. A plain user is
	// refused POST /v1/relation outright; without this it could make the same
	// write through a plugin hook its own upload fired, and "the endpoints are
	// inside your subtree" is not an answer to "may you relate groups at all".
	operations := []struct {
		name      string
		adminOnly bool
		run       func(ctx *MahresourcesContext) error
	}{
		{"CreateCategory", true, func(c *MahresourcesContext) error {
			_, err := c.CreateCategory(&query_models.CategoryCreator{Name: "cat-" + randomSuffix()})
			return err
		}},
		{"UpdateCategory", true, func(c *MahresourcesContext) error {
			_, err := c.UpdateCategory(&query_models.CategoryEditor{
				CategoryCreator: query_models.CategoryCreator{Name: "renamed-" + randomSuffix()}, ID: 1,
			})
			return err
		}},
		{"DeleteCategory", true, func(c *MahresourcesContext) error { return c.DeleteCategory(1) }},
		{"CreateResourceCategory", true, func(c *MahresourcesContext) error {
			_, err := c.CreateResourceCategory(&query_models.ResourceCategoryCreator{Name: "rc-" + randomSuffix()})
			return err
		}},
		{"DeleteResourceCategory", true, func(c *MahresourcesContext) error { return c.DeleteResourceCategory(1) }},
		{"CreateOrUpdateTemplatePartial", true, func(c *MahresourcesContext) error {
			_, err := c.CreateOrUpdateTemplatePartial(&query_models.TemplatePartialEditor{
				Name: "tp-" + randomSuffix(), Content: "x",
			})
			return err
		}},
		{"DeleteTemplatePartial", true, func(c *MahresourcesContext) error { return c.DeleteTemplatePartial(1) }},
		{"CreateOrUpdateNoteType", false, func(c *MahresourcesContext) error {
			_, err := c.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{Name: "nt-" + randomSuffix()})
			return err
		}},
		{"DeleteNoteType", false, func(c *MahresourcesContext) error { return c.DeleteNoteType(1) }},
		{"AddRelationType", false, func(c *MahresourcesContext) error {
			_, err := c.AddRelationType(&query_models.RelationshipTypeEditorQuery{Name: "rt-" + randomSuffix()})
			return err
		}},
		{"EditRelationType", false, func(c *MahresourcesContext) error {
			_, err := c.EditRelationType(&query_models.RelationshipTypeEditorQuery{Id: relType.ID, Name: "rt2-" + randomSuffix()})
			return err
		}},
		{"DeleteRelationshipType", false, func(c *MahresourcesContext) error {
			return c.DeleteRelationshipType(relType.ID)
		}},
		{"AddRelation", false, func(c *MahresourcesContext) error {
			_, err := c.AddRelation(1, 2, relType.ID, "rel-"+randomSuffix(), "")
			return err
		}},
		{"EditRelation", false, func(c *MahresourcesContext) error {
			_, err := c.EditRelation(query_models.GroupRelationshipQuery{Id: 1, Name: "rel-" + randomSuffix()})
			return err
		}},
		{"DeleteRelationship", false, func(c *MahresourcesContext) error { return c.DeleteRelationship(1) }},
	}

	roles := []struct {
		name    string
		role    models.Role
		isAdmin bool
		isWrite bool // editor or better
	}{
		{"admin", models.RoleAdmin, true, true},
		{"editor", models.RoleEditor, false, true},
		{"user", models.RoleUser, false, false},
		{"guest", models.RoleGuest, false, false},
	}

	for _, op := range operations {
		for _, r := range roles {
			t.Run(op.name+"/"+r.name, func(t *testing.T) {
				bound := ctx.WithPrincipal(&auth.Principal{UserID: 1, Role: r.role})
				err := op.run(bound)

				allowed := r.isAdmin || (!op.adminOnly && r.isWrite)
				refused := errors.Is(err, ErrRoleCapability)

				if allowed && refused {
					t.Fatalf("%s must be available to %s, but it was refused: %v", op.name, r.name, err)
				}
				if !allowed && !refused {
					// The operation may still fail for an ordinary reason (a
					// missing row); what must not happen is that it gets past
					// the capability check.
					t.Fatalf("%s was not refused for %s (got %v), so role capability is not enforced below server/",
						op.name, r.name, err)
				}
			})
		}
	}
}

// A write that carries no identity at all — the singleton handle, a background
// worker, a startup seed — is unchanged. That is the same rule the scope
// mechanism already lives by, and it is what keeps the importer, the hash and
// thumbnail workers, and the bootstrap seeds working.
func TestTaxonomyOperations_UnboundHandleIsUnchanged(t *testing.T) {
	ctx := newRoleCapabilityTestContext(t)

	if _, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: "from-the-singleton"}); err != nil {
		t.Fatalf("an unbound (singleton) handle must keep working: %v", err)
	}
}

// Auth off means every request is an implicit administrator, and the principal
// built for it carries the root admin's real role. The guard must read that as
// admin rather than as "some bound principal, therefore suspicious".
func TestTaxonomyOperations_AuthOffRootPrincipalIsAllowed(t *testing.T) {
	ctx := newRoleCapabilityTestContext(t)

	bound := ctx.WithPrincipal(auth.SystemPrincipal())
	if _, err := bound.CreateCategory(&query_models.CategoryCreator{Name: "as-root"}); err != nil {
		t.Fatalf("the auth-off super-user must be allowed a taxonomy write: %v", err)
	}
}

// The identity a plugin call runs as when its actor cannot be resolved is a
// guest with no scope group. It was already deny-all for subtree-scoped data;
// with role enforced it is deny-all for global taxonomy too, which is what that
// principal's name has always claimed.
func TestTaxonomyOperations_UnresolvableActorCannotWriteTaxonomy(t *testing.T) {
	ctx := newRoleCapabilityTestContext(t)

	bound := ctx.WithPrincipal(deniedPluginPrincipal(4242))
	if _, err := bound.CreateCategory(&query_models.CategoryCreator{Name: "by-a-ghost"}); !errors.Is(err, ErrRoleCapability) {
		t.Fatalf("an unresolvable plugin actor must not be able to create a category, got %v", err)
	}
}

// taxonomyProbePlugin registers an after_note_create hook that tries to create a
// category, and reports what came back. The hook is the door this guard exists
// for: it fires from ordinary scoped CRUD that a plain user is entitled to
// perform, so it is not a URL path and the plugin-code deny cannot match it.
func taxonomyProbePlugin() string {
	return `plugin = { name = "hooktest", version = "1.0", description = "tries a taxonomy write from a hook" }
local last = "never ran"

function init()
    mah.on("after_note_create", function(data)
        local cat, err = mah.db.create_category({ name = "made-by-a-hook" })
        if cat then
            last = "created"
        else
            last = "refused: " .. tostring(err)
        end
        return data
    end)

    mah.inject("probe", function(ctx) return last end)
end
`
}

// The headline case. A plain user creates a note — a write its role allows —
// and the plugin woken by it attempts a write its role does not. Before the
// guard, that write succeeded: the invocation bound the caller's scope, but
// scope has nothing to say about a table with no owner.
func TestHookInvocation_PluginCannotWriteTaxonomyTheTriggeringRoleLacks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		role    models.Role
		allowed bool
	}{
		{"a plain user", models.RoleUser, false},
		{"an editor", models.RoleEditor, false}, // categories are admin-only
		{"an admin", models.RoleAdmin, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newPluginHookTestContext(t, taxonomyProbePlugin())

			group := &models.Group{Name: "g"}
			if err := ctx.db.Create(group).Error; err != nil {
				t.Fatalf("create group: %v", err)
			}
			user, err := ctx.CreateUser(&UserInput{
				Username: "trigger", Password: "password1", Role: tc.role,
			})
			if err != nil {
				t.Fatalf("create user: %v", err)
			}

			scoped := ctx.WithPrincipal(auth.FromUser(user))
			if _, err := scoped.CreateOrUpdateNote(&query_models.NoteEditor{
				NoteCreator: query_models.NoteCreator{Name: "trigger", OwnerId: group.ID},
			}); err != nil {
				t.Fatalf("%s could not create a note: %v", tc.name, err)
			}

			saw := ctx.PluginManager().RenderSlot(context.Background(), "probe", map[string]any{}, nil)
			if saw == "never ran" {
				t.Fatalf("the hook never fired, so this proves nothing")
			}

			var count int64
			ctx.db.Model(&models.Category{}).Where("name = ?", "made-by-a-hook").Count(&count)

			if tc.allowed {
				if count != 1 {
					t.Errorf("a hook triggered by %s must still be able to create a category (probe said %q)", tc.name, saw)
				}
				return
			}
			if count != 0 {
				t.Errorf("a hook triggered by %s created a category, which that role may not do at the HTTP layer (probe said %q)",
					tc.name, saw)
			}
			if !strings.Contains(saw, "refused") {
				t.Errorf("the plugin was not told why: probe said %q", saw)
			}
		})
	}
}
