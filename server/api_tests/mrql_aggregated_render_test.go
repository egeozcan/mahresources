package api_tests

import (
	"context"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// TestMRQLAggregatedValuesRenderAsValues: GROUP BY aggregate rows reach the
// shortcodes as the values themselves, so a table cell, a value= scalar and a
// [conditional] aggregate show and compare the value, not a pointer to it.
func TestMRQLAggregatedValuesRenderAsValues(t *testing.T) {
	runMRQLAggregatedValuesRenderAsValues(t, SetupTestEnv(t))
}

func runMRQLAggregatedValuesRenderAsValues(t *testing.T, tc *TestContext) {
	group := &models.Group{Name: "Owner"}
	if err := tc.DB.Create(group).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := tc.DB.Create(&models.Note{Name: "plan", OwnerId: &group.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	meta := template_filters.BuildMetaContextForEntity(group, tc.AppCtx)
	render := func(markup string) string {
		return shortcodes.Process(context.Background(), markup, *meta, nil, template_filters.BuildQueryExecutor(tc.AppCtx))
	}
	table := render(`[mrql query='type = note GROUP BY name COUNT()' scope="global" format="table"]`)
	if strings.Contains(table, "0x") || !strings.Contains(table, ">plan<") || !strings.Contains(table, ">2<") {
		t.Errorf("aggregated table:\n%s", table)
	}
	if bucketed := render(`[mrql query='type = note GROUP BY name' scope="global"]`); strings.Contains(bucketed, "0x") || !strings.Contains(bucketed, "plan") {
		t.Errorf("bucketed headings:\n%s", bucketed)
	}
	if got := render(`[mrql query='type = note GROUP BY name COUNT()' scope="global" value="name"]`); got != "plan" {
		t.Errorf("value= scalar: %q", got)
	}
	if got := render(`[conditional mrql='type = note GROUP BY name COUNT()' scope="global" aggregate="name" eq="plan"]two[else]other[/conditional]`); got != "two" {
		t.Errorf("conditional aggregate: %q", got)
	}
}
