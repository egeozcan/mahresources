package mrql

import (
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// TestNoRawGroupFKReadWhenConfined is the structural guard for groupFK: with
// VisibleRootID set, every read of a row's owner_id (a resource's or note's
// owner, a group's parent) goes through groupFK, which reads a key to an
// invisible group as NULL. It translates a corpus that reaches owner_id every
// way the translator can, removes what is allowed, and fails on any owner_id
// that is left:
//
//   - groupFK's own CASE expression and the visible derived tables;
//   - the main row's scope filter (ApplyScopeCTE), which bounds the main table;
//   - reverse reads, a child's owner_id compared with or collected for its
//     parent's id: the child is read from a visible table, or its key is only
//     matched against a visible row's id, so it cannot name a hidden group;
//   - a forward LEFT JOIN onto the visible groups table, where an invisible
//     key joins nothing and so reads as a missing parent;
//   - the ancestors./descendants. walks, whose seed set is visible-only.
func TestNoRawGroupFKReadWhenConfined(t *testing.T) {
	db := setupTestDB(t)
	const root = 7
	opts := TranslateOptions{VisibleRootID: root, ScopeGroupID: root}
	tc := &translateContext{visibleRoot: root}

	queries := []string{
		`type = group AND parent = 3`, `type = group AND parent != 3`, `type = group AND NOT (parent = 3)`,
		`type = group AND parent IS NULL`, `type = group AND parent IS NOT EMPTY`, `type = resource AND owner IS NULL`,
		`type = resource AND owner = 3`, `type = note AND owner != 3`,
		`type = group AND parent.name = "x"`, `type = group AND parent.parent.name = "x"`, `type = group AND NOT (parent.parent.meta.k = "x")`,
		`type = group AND parent.name IS NULL`, `type = group AND parent.parent IS NULL`, `type = group AND parent.parent IS NOT NULL`,
		`type = resource AND owner.parent.name = "x"`, `type = resource AND owner.parent IS NULL`, `type = note AND owner.parent.parent.name != "x"`,
		`type = group AND parent.tags = "x"`, `type = resource AND owner.parent.tags = "x"`,
		`type = group AND children.name = "x"`, `type = group AND children.parent.name = "x"`, `type = group AND children.name IS NULL`,
		`type = group AND children IS EMPTY`, `type = group AND children.count > 1`, `type = group AND parent.children.name = "x"`,
		`type = group AND ancestors.name = "x"`, `type = resource AND NOT (ancestors.name = "x")`, `type = group AND descendants.tags = "x"`,
		`type = resource AND groups = "x"`, `type = note AND resources IS NOT EMPTY`, `type = resource AND groups.count > 0`,
	}
	grouped := []string{
		`type = group GROUP BY parent COUNT()`, `type = group GROUP BY parent`, `type = group GROUP BY parent.name COUNT()`,
		`type = group GROUP BY parent.parent.name COUNT()`, `type = resource GROUP BY owner COUNT()`, `type = resource GROUP BY owner.parent.meta.k COUNT()`,
		`type = group GROUP BY children COUNT()`, `type = note GROUP BY groups COUNT()`, `type = group GROUP BY parent.tags COUNT()`,
	}

	caseFK := regexp.MustCompile(`\(CASE WHEN ([\w."]+) IN \(SELECT v\.id FROM VISIBLE_groups v\) THEN [\w."]+ END\)`)
	walks := regexp.MustCompile(`WITH RECURSIVE _mrql_(anc|desc)\(id, depth\) AS \(.*?\) SELECT id FROM _mrql_(anc|desc)`)
	allowed := []*regexp.Regexp{
		regexp.MustCompile(`"?(resources|notes|groups)"?\."?owner_id"? IN \(SCOPE_IDS\)`), // the main row's scope
		regexp.MustCompile(`\bc\.owner_id\b`),                                             // children subqueries read a child's key
		regexp.MustCompile(`SELECT owner_id FROM groups WHERE owner_id IS NOT NULL`),      // the set of groups that have children
		regexp.MustCompile(`SELECT _t\d+\.owner_id FROM`),                                 // a children step selects the child's key
		regexp.MustCompile(`_t\d+\.owner_id IS NOT NULL AND`),                             // ... and drops NULL keys
		regexp.MustCompile(`ON \w+\.owner_id = [\w."]+\.id`),                              // a reverse join to a child
		// A forward join whose target is the visible table: an invisible key
		// joins nothing, so the joined columns are NULL, as for no parent.
		regexp.MustCompile(`JOIN VISIBLE_groups \w+ ON \w+\.id = [\w."]+\.owner_id`),
	}
	check := func(query, sql string) {
		t.Helper()
		for _, table := range []string{"groups", "resources", "notes"} {
			sql = strings.ReplaceAll(sql, tc.visibleTable(table), "VISIBLE_"+table)
		}
		sql = strings.ReplaceAll(sql, tc.visibleGroupIDs(), "VISIBLE_IDS")
		sql = strings.ReplaceAll(sql, scopeCTE, "SCOPE_IDS")
		sql = caseFK.ReplaceAllString(sql, "GROUP_FK")
		sql = walks.ReplaceAllString(sql, "WALK")
		for _, re := range allowed {
			sql = re.ReplaceAllString(sql, "ALLOWED")
		}
		if strings.Contains(sql, "owner_id") {
			t.Errorf("%s reads owner_id outside groupFK:\n%s", query, sql)
		}
	}
	dry := func() *gorm.DB { return db.Session(&gorm.Session{DryRun: true}) }
	for _, input := range queries {
		q := parseValidated(t, input)
		built, err := TranslateWithOptions(q, dry(), opts)
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		var rows []map[string]any
		check(input, built.Find(&rows).Statement.SQL.String())
	}
	for _, input := range grouped {
		q := parseValidated(t, input)
		q.EntityType = ExtractEntityType(q)
		var built *gorm.DB
		var err error
		if len(q.GroupBy.Aggregates) > 0 {
			built, err = BuildAggregatedGroupBy(q, dry(), opts)
		} else {
			built, err = BuildGroupByKeys(q, dry(), opts)
		}
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		var rows []map[string]any
		check(input, built.Find(&rows).Statement.SQL.String())
	}
}

func parseValidated(t *testing.T, input string) *Query {
	t.Helper()
	q, err := Parse(input)
	if err == nil {
		err = Validate(q)
	}
	if err != nil {
		t.Fatalf("%s: %v", input, err)
	}
	return q
}
