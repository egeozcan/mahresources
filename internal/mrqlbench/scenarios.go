package mrqlbench

import (
	"fmt"
	"sort"
)

var standardProfiles = map[string]Profile{
	"100k": profile("100k", 100_000),
	"1m":   profile("1m", 1_000_000),
	"3m":   profile("3m", 3_000_000),
}

func profile(id string, resources int) Profile {
	groups := resources / 500
	if groups < 32 {
		groups = 32
	}
	if groups > 5_000 {
		groups = 5_000
	}
	return Profile{ID: id, Resources: resources, Notes: (resources + 3) / 4, Groups: groups, Tags: 64, Seed: 1}
}

func Profiles() []Profile {
	ids := make([]string, 0, len(standardProfiles))
	for id := range standardProfiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Profile, 0, len(ids))
	for _, id := range ids {
		out = append(out, standardProfiles[id])
	}
	return out
}

func ProfileByID(id string) (Profile, bool) {
	p, ok := standardProfiles[id]
	return p, ok
}

var scenarioCatalog = []Scenario{
	{ID: "scalar-selective", Family: "scalar", Description: "Selective scalar filter and top-N ordering", Query: `type = "resource" AND fileSize > 10000000 ORDER BY created DESC LIMIT 50`, Mode: "direct", ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 1, MinimumRows: 1, MaximumRows: 50},
	{ID: "relation-common-tag", Family: "relation", Description: "Many-to-many relation filter", Query: `type = "resource" AND tags = "common" ORDER BY created DESC LIMIT 50`, Mode: "direct", ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 1},
	{ID: "scope-shallow", Family: "scope", Description: "Explicit shallow subtree", Query: `type = "resource" SCOPE 1 ORDER BY created DESC LIMIT 50`, Mode: "direct", ScopeClass: "shallow", MinimumSQLStatements: 1, MaximumSQLStatements: 2},
	{ID: "scope-deep", Family: "scope", Description: "Explicit deep subtree", Query: `type = "resource" SCOPE 2 ORDER BY created DESC LIMIT 50`, Mode: "direct", ScopeClass: "deep", MinimumSQLStatements: 1, MaximumSQLStatements: 2},
	{ID: "scope-empty", Family: "scope", Description: "Empty subtree", Query: `type = "resource" SCOPE 3 LIMIT 50`, Mode: "direct", ScopeClass: "empty", MinimumSQLStatements: 1, MaximumSQLStatements: 2},
	{ID: "hierarchy-descendants", Family: "hierarchy", Description: "Recursive descendant predicate", Query: `type = "group" AND descendants.name = "benchmark-leaf" LIMIT 50`, Mode: "direct", ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 1},
	{ID: "metadata-numeric", Family: "metadata", Description: "Numeric JSON metadata predicate", Query: `type = "resource" AND meta.rating >= 3 LIMIT 50`, Mode: "direct", ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 1},
	{ID: "fts-common", Family: "fts", Description: "Common full-text term with rank ordering", Query: `type = "resource" AND TEXT ~ "benchmarkneedle" ORDER BY RANK LIMIT 50`, Mode: "direct", ScopeClass: "global", RequiredFeatures: []string{"fts"}, MinimumSQLStatements: 1, MaximumSQLStatements: 1},
	{ID: "similarity", Family: "similarity", Description: "Precomputed perceptual similarity", Query: `type = "resource" AND SIMILAR TO resource(1) WITHIN 10 LIMIT 50`, Mode: "direct", ScopeClass: "global", RequiredFeatures: []string{"similarity"}, MinimumSQLStatements: 1, MaximumSQLStatements: 1},
	{ID: "page-first", Family: "pagination", Description: "First ordered page", Query: `type = "resource" ORDER BY created DESC`, Mode: "direct", Page: 1, ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 1},
	{ID: "page-middle", Family: "pagination", Description: "Middle ordered page", Query: `type = "resource" ORDER BY created DESC`, Mode: "direct", Page: 10, ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 1},
	{ID: "page-deep", Family: "pagination", Description: "Deep ordered page", Query: `type = "resource" ORDER BY created DESC`, Mode: "direct", Page: 100, ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 1},
	{ID: "aggregate-content-type", Family: "aggregate", Description: "Set-based grouped aggregate", Query: `type = "resource" GROUP BY contentType COUNT() ORDER BY count DESC LIMIT 20`, Mode: "grouped", ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 1},
	{ID: "bucket-content-type", Family: "bucket", Description: "Bucket discovery and item fan-out", Query: `type = "resource" GROUP BY contentType LIMIT 10`, Mode: "grouped", ScopeClass: "global", MinimumSQLStatements: 2, MaximumSQLStatements: 201},
	{ID: "cross-entity-top-n", Family: "cross-entity", Description: "Three-branch global top-N", Query: `name ~ "benchmark" ORDER BY created DESC LIMIT 50`, Mode: "direct", ScopeClass: "global", MinimumSQLStatements: 3, MaximumSQLStatements: 3},
	{ID: "cross-entity-random", Family: "cross-entity", Description: "Random cross-entity sampling with conditional counts", Query: `name ~ "benchmark" ORDER BY RANDOM() LIMIT 50`, Mode: "direct", ScopeClass: "global", MinimumSQLStatements: 3, MaximumSQLStatements: 6, StochasticOutput: true, MinimumOutputBytes: 20000, MaximumOutputBytes: 100000},
	{ID: "raw-json", Family: "rendering", Description: "HTTP JSON execution and encoding", Query: `type = "resource" ORDER BY created DESC LIMIT 20`, Mode: "http", RenderMode: "raw-json", ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 1},
	{ID: "compact-render", Family: "rendering", Description: "Compact shortcode rendering", Query: `type = "resource" ORDER BY created DESC LIMIT 20`, Mode: "shortcode", RenderMode: "compact", ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 8},
	{ID: "table-render", Family: "rendering", Description: "Table shortcode rendering", Query: `type = "resource" ORDER BY created DESC LIMIT 20`, Mode: "shortcode", RenderMode: "table", ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 8},
	{ID: "custom-render", Family: "rendering", Description: "Custom category template rendering", Query: `type = "resource" ORDER BY created DESC LIMIT 20`, Mode: "shortcode", RenderMode: "custom", ScopeClass: "global", MinimumSQLStatements: 1, MaximumSQLStatements: 8},
	{ID: "nested-mrql", Family: "rendering", Description: "Entity-correlated nested MRQL rendering", Query: `type = "group" ORDER BY created DESC LIMIT 20`, Mode: "shortcode", RenderMode: "nested", ScopeClass: "global", MinimumSQLStatements: 2, MaximumSQLStatements: 30},
}

var scenarioRowBounds = map[string][2]int64{
	"scalar-selective": {1, 50}, "relation-common-tag": {1, 50},
	"scope-shallow": {1, 50}, "scope-deep": {1, 50}, "scope-empty": {0, 0},
	"hierarchy-descendants": {1, 100}, "metadata-numeric": {1, 50}, "fts-common": {1, 50}, "similarity": {1, 50},
	"page-first": {50, 50}, "page-middle": {50, 50}, "page-deep": {50, 50},
	"aggregate-content-type": {1, 20}, "bucket-content-type": {2, 2010},
	"cross-entity-top-n": {3, 150}, "cross-entity-random": {3, 1000},
	"raw-json": {1, 20}, "compact-render": {1, 2000}, "table-render": {1, 2000},
	"custom-render": {1, 2000}, "nested-mrql": {1, 5000},
}

func Scenarios() []Scenario {
	out := append([]Scenario(nil), scenarioCatalog...)
	for i := range out {
		out[i].RequiredFeatures = append([]string(nil), out[i].RequiredFeatures...)
		if bounds, ok := scenarioRowBounds[out[i].ID]; ok {
			out[i].CheckRows, out[i].MinimumRows, out[i].MaximumRows = true, bounds[0], bounds[1]
		}
		if out[i].TimingPolicy.RelativePercent == 0 {
			out[i].TimingPolicy = TimingRegressionPolicy{RelativePercent: 15, MinimumNanos: 1_000_000}
		}
	}
	return out
}

func ScenarioByID(id string) (Scenario, bool) {
	for _, scenario := range Scenarios() {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return Scenario{}, false
}

func ValidateCatalog() error {
	seen := map[string]bool{}
	for _, scenario := range Scenarios() {
		if scenario.ID == "" || scenario.Family == "" || scenario.Query == "" || scenario.Mode == "" {
			return fmt.Errorf("scenario has incomplete identity: %#v", scenario)
		}
		if seen[scenario.ID] {
			return fmt.Errorf("duplicate scenario %q", scenario.ID)
		}
		seen[scenario.ID] = true
		if scenario.MinimumSQLStatements < 0 || scenario.MaximumSQLStatements < scenario.MinimumSQLStatements {
			return fmt.Errorf("scenario %q has invalid SQL bounds", scenario.ID)
		}
		if !scenario.CheckRows || scenario.MaximumRows < scenario.MinimumRows {
			return fmt.Errorf("scenario %q has missing or invalid row bounds", scenario.ID)
		}
	}
	return nil
}
