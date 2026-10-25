package api_tests

import (
	"fmt"
	"strings"
	"testing"

	"mahresources/auth"
	"mahresources/models"
)

func TestMRQLHiddenParentBehavesLikeNull(t *testing.T) {
	runMRQLHiddenParentBehavesLikeNull(t, SetupTestEnv(t))
}

// runMRQLHiddenParentBehavesLikeNull: for a confined principal, a parent it
// cannot see must be indistinguishable from no parent at all, under every
// comparison, NOT, double NOT and OR, and at every hop of a traversal. Two
// subtrees with the same shape differ only there: tree A's root has a parent
// outside every scope, tree B's root has none. Each query runs for a guest
// confined to A and one confined to B, and the two results, with their
// prefixes stripped, must be identical.
func runMRQLHiddenParentBehavesLikeNull(t *testing.T, tc *TestContext) {
	create := func(v any) {
		t.Helper()
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	hidden := &models.Group{Name: "HIDDEN-parent"}
	create(hidden)
	tree := func(prefix string, parent *models.Group) *models.Group {
		root := &models.Group{Name: prefix + "root"}
		if parent != nil {
			root.OwnerId = &parent.ID
		}
		create(root)
		child := &models.Group{Name: prefix + "child", OwnerId: &root.ID}
		create(child)
		grand := &models.Group{Name: prefix + "grand", OwnerId: &child.ID}
		create(grand)
		create(&models.Resource{Name: prefix + "res-root", OwnerId: &root.ID, Hash: prefix + "r1", Location: prefix + "r1"})
		create(&models.Resource{Name: prefix + "res-grand", OwnerId: &grand.ID, Hash: prefix + "r3", Location: prefix + "r3"})
		create(&models.Note{Name: prefix + "note-child", OwnerId: &child.ID})
		return root
	}
	rootA := tree("a-", hidden)
	rootB := tree("b-", nil)
	confinedTo := func(root *models.Group) *auth.Principal {
		return &auth.Principal{UserID: 1, Username: "guest", Role: models.RoleGuest, ScopeGroupID: &root.ID}
	}
	a := tc.AppCtx.WithPrincipal(confinedTo(rootA))
	b := tc.AppCtx.WithPrincipal(confinedTo(rootB))

	p := hidden.ID
	queries := []string{
		fmt.Sprintf(`type = group AND parent = %d`, p),
		fmt.Sprintf(`type = group AND NOT (parent = %d)`, p),
		fmt.Sprintf(`type = group AND NOT (NOT (parent = %d))`, p),
		fmt.Sprintf(`type = group AND parent != %d`, p),
		fmt.Sprintf(`type = group AND NOT (parent != %d)`, p),
		fmt.Sprintf(`type = group AND (parent = %d OR name ~ "*child")`, p),
		fmt.Sprintf(`type = group AND NOT (parent = %d OR name ~ "*child")`, p),
		fmt.Sprintf(`type = group AND (NOT (parent = %d) OR name ~ "*grand")`, p),
		`type = group AND parent IS NULL`,
		`type = group AND NOT (parent IS NULL)`,
		`type = group AND parent IS EMPTY`,
		`type = group AND NOT (parent IS NOT EMPTY)`,
		`type = group AND parent.name = "HIDDEN-parent"`,
		`type = group AND NOT (parent.name = "HIDDEN-parent")`,
		`type = group AND NOT (parent.name != "HIDDEN-parent")`,
		`type = group AND parent.name IS NULL`,
		`type = group AND NOT (parent.name IS NULL)`,
		// Two, three and four hops (IS NULL takes one hop past the first).
		`type = group AND parent.parent IS NULL`,
		`type = group AND parent.parent IS NOT NULL`,
		`type = group AND NOT (parent.parent IS NULL)`,
		`type = group AND parent.parent.parent.name = "HIDDEN-parent"`,
		`type = group AND NOT (parent.parent.parent.name = "HIDDEN-parent")`,
		`type = group AND parent.parent.name = "HIDDEN-parent"`,
		`type = group AND NOT (parent.parent.name = "HIDDEN-parent")`,
		`type = resource AND owner.parent IS NULL`,
		`type = resource AND NOT (owner.parent IS NULL)`,
		`type = resource AND owner.parent.name = "HIDDEN-parent"`,
		`type = resource AND NOT (owner.parent.parent.name = "HIDDEN-parent")`,
		`type = resource AND NOT (owner.parent.parent.parent.name != "zz")`,
		`type = resource AND NOT (owner.parent.name = "HIDDEN-parent")`,
		`type = note AND NOT (owner.parent.name = "HIDDEN-parent")`,
		`type = note AND NOT (owner.parent.parent.name = "HIDDEN-parent")`,
		`type = group AND ancestors.name = "HIDDEN-parent"`,
		`type = group AND NOT (ancestors.name = "HIDDEN-parent")`,
		`type = resource AND NOT (ancestors.name = "HIDDEN-parent")`,
	}
	strip := func(summary, prefix string) string {
		var out []string
		for _, name := range strings.Split(summary, ",") {
			out = append(out, strings.TrimPrefix(name, prefix))
		}
		return strings.Join(out, ",")
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			gotA := strip(runConfinedMRQL(t, a, q), "a-")
			gotB := strip(runConfinedMRQL(t, b, q), "b-")
			sortedA, sortedB := sortedCSV(gotA), sortedCSV(gotB)
			if sortedA != sortedB {
				t.Errorf("a hidden parent differs from no parent:\n hidden %q\n   null %q", sortedA, sortedB)
			}
			if strings.Contains(sortedA, "HIDDEN") {
				t.Errorf("the hidden parent appears: %q", sortedA)
			}
		})
	}
}

func sortedCSV(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		for j := i + 1; j < len(parts); j++ {
			if parts[j] < parts[i] {
				parts[i], parts[j] = parts[j], parts[i]
			}
		}
	}
	return strings.Join(parts, ",")
}
