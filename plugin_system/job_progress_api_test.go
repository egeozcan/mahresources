package plugin_system

import (
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func luaProgressTable(t *testing.T, source string) *lua.LTable {
	t.Helper()
	L := lua.NewState()
	t.Cleanup(L.Close)
	if err := L.DoString("result = " + source); err != nil {
		t.Fatalf("lua %q: %v", source, err)
	}
	tbl, ok := L.GetGlobal("result").(*lua.LTable)
	if !ok {
		t.Fatalf("lua %q did not produce a table", source)
	}
	return tbl
}

func TestParseJobProgressTableKeepsOmittedFields(t *testing.T) {
	previous := HostProgress{Percent: 10, Message: "scanning", Unit: "items",
		Completed: ptrInt64(1), Total: ptrInt64(10)}
	next, err := parseJobProgressTable(luaProgressTable(t, `{ completed = 5 }`), previous)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if next.Message != "scanning" || next.Unit != "items" || *next.Total != 10 || *next.Completed != 5 {
		t.Fatalf("next = %+v; want only completed changed", next)
	}
	if next.Percent != 50 {
		t.Fatalf("percent = %d; want 50 derived from the counts", next.Percent)
	}

	explicit, err := parseJobProgressTable(luaProgressTable(t, `{ completed = 5, percent = 7 }`), previous)
	if err != nil || explicit.Percent != 7 {
		t.Fatalf("explicit percent = %d, %v; want the plugin's own 7", explicit.Percent, err)
	}

	withMetrics, err := parseJobProgressTable(luaProgressTable(t,
		`{ metrics = { { key = "rows", label = "Rows", value = 3, graph = true }, { key = "left", value = 2, total = 9, unit = "items" } } }`), previous)
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if len(withMetrics.Metrics) != 2 || !withMetrics.Metrics[0].Graph || withMetrics.Metrics[1].Label != "left" ||
		withMetrics.Metrics[1].Total == nil || *withMetrics.Metrics[1].Total != 9 {
		t.Fatalf("metrics = %+v", withMetrics.Metrics)
	}
	cleared, err := parseJobProgressTable(luaProgressTable(t, `{ metrics = {} }`), withMetrics)
	if err != nil || cleared.Metrics != nil {
		t.Fatalf("an empty metrics table left %+v, %v; want the set cleared", cleared.Metrics, err)
	}
	kept, err := parseJobProgressTable(luaProgressTable(t, `{ message = "next" }`), withMetrics)
	if err != nil || len(kept.Metrics) != 2 {
		t.Fatalf("omitting metrics dropped them: %+v, %v", kept.Metrics, err)
	}
}

func TestParseJobProgressTableRefusalsNameTheField(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{`{ percnt = 5 }`, `unknown field "percnt"`},
		{`{ completed = -1 }`, "completed must be a finite number"},
		{`{ total = "ten" }`, "total must be a number"},
		{`{ unit = string.rep("u", 21) }`, "unit is over"},
		{`{ message = 5 }`, "message must be a string"},
		{`{ metrics = "rows" }`, "metrics must be a list"},
		{`{ metrics = { rows = { key = "rows", value = 1 } } }`, "not a map"},
		{`{ metrics = { "rows" } }`, "metrics[1] must be a table"},
		{`{ metrics = { { label = "x", value = 1 } } }`, "metrics[1].key is required"},
		{`{ metrics = { { key = "rows" } } }`, "metrics[1].value is required"},
		{`{ metrics = { { key = "rows", value = 1, graph = "yes" } } }`, "metrics[1].graph must be a boolean"},
		{`{ metrics = { { key = "rows", value = 1, colour = "red" } } }`, `unknown field "colour"`},
		{`{ metrics = { { key = "Rows", value = 1 } } }`, "may only use a-z"},
		{`{ metrics = { { key = "rows", value = -1 } } }`, "finite number of at least zero"},
		{`{ metrics = { { key = "a", value = 1, graph = true }, { key = "b", value = 1, graph = true },
		   { key = "c", value = 1, graph = true }, { key = "d", value = 1, graph = true } } }`, "graphed metrics"},
	}
	for _, tt := range tests {
		_, err := parseJobProgressTable(luaProgressTable(t, tt.source), HostProgress{})
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: error = %v; want it to mention %q", tt.source, err, tt.want)
		}
	}
}

func ptrInt64(v int64) *int64 { return &v }
