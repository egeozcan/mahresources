package plugin_system

import (
	"fmt"
	"math"

	lua "github.com/yuin/gopher-lua"

	"mahresources/models/jobmetrics"
)

// maxJobProgressUnitBytes mirrors the Job Service's unit ceiling, which the
// host would otherwise refuse after the call returned.
const maxJobProgressUnitBytes = jobmetrics.MaxUnitBytes

// parseJobProgressTable reads the table form of mah.job_progress:
//
//	mah.job_progress(job_id, {
//	    percent = 40,                 -- or completed/total:
//	    completed = 120, total = 300, unit = "items",
//	    message = "Scanning",
//	    metrics = {
//	        { key = "rows", label = "Rows scanned", value = 1200, graph = true },
//	        { key = "skipped", label = "Skipped", value = 3, total = 300, unit = "items" },
//	    },
//	})
//
// A field the table omits keeps the value the previous report gave it, so a
// loop can report only what moved. metrics is the exception inside itself: when
// it is present it replaces the whole set, and an empty table clears it.
//
// Every refusal names the field, because the reader of the error is the
// plugin's author.
func parseJobProgressTable(tbl *lua.LTable, previous HostProgress) (HostProgress, error) {
	next := previous
	next.Metrics = jobmetrics.Clone(previous.Metrics)

	count := func(field string) (*int64, error) {
		value := tbl.RawGetString(field)
		if value == lua.LNil {
			return nil, nil
		}
		number, ok := value.(lua.LNumber)
		if !ok {
			return nil, fmt.Errorf("%s must be a number", field)
		}
		f := float64(number)
		if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > math.MaxInt64 {
			return nil, fmt.Errorf("%s must be a finite number of at least zero", field)
		}
		n := int64(f)
		return &n, nil
	}
	text := func(field string) (string, bool, error) {
		value := tbl.RawGetString(field)
		if value == lua.LNil {
			return "", false, nil
		}
		str, ok := value.(lua.LString)
		if !ok {
			return "", false, fmt.Errorf("%s must be a string", field)
		}
		return string(str), true, nil
	}

	for key := range luaTableKeys(tbl) {
		switch key {
		case "percent", "completed", "total", "unit", "message", "metrics":
		default:
			return HostProgress{}, fmt.Errorf("unknown field %q; expected percent, completed, total, unit, message or metrics", key)
		}
	}

	percent, err := count("percent")
	if err != nil {
		return HostProgress{}, err
	}
	completed, err := count("completed")
	if err != nil {
		return HostProgress{}, err
	}
	total, err := count("total")
	if err != nil {
		return HostProgress{}, err
	}
	if completed != nil {
		next.Completed = completed
	}
	if total != nil {
		next.Total = total
	}
	if unit, ok, err := text("unit"); err != nil {
		return HostProgress{}, err
	} else if ok {
		if len(unit) > maxJobProgressUnitBytes {
			return HostProgress{}, fmt.Errorf("unit is over the %d-byte ceiling", maxJobProgressUnitBytes)
		}
		next.Unit = unit
	}
	if message, ok, err := text("message"); err != nil {
		return HostProgress{}, err
	} else if ok {
		next.Message = message
	}

	switch {
	case percent != nil:
		value := *percent
		if value > 100 {
			value = 100
		}
		next.Percent = clampPercent(int(value))
	case next.Completed != nil && next.Total != nil && *next.Total > 0:
		// Counts imply a percent, which is what the compatibility panel and any
		// reader of the positional form still show.
		next.Percent = clampPercent(int(float64(*next.Completed) * 100 / float64(*next.Total)))
	}

	if raw := tbl.RawGetString("metrics"); raw != lua.LNil {
		metrics, err := parseJobMetrics(raw)
		if err != nil {
			return HostProgress{}, err
		}
		next.Metrics = metrics
	}
	return next, nil
}

func parseJobMetrics(raw lua.LValue) ([]jobmetrics.Metric, error) {
	list, ok := raw.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("metrics must be a list of tables")
	}
	length := list.Len()
	if length > jobmetrics.MaxMetrics {
		return nil, fmt.Errorf("%d metrics, over the %d-metric ceiling", length, jobmetrics.MaxMetrics)
	}
	for key := range luaTableKeys(list) {
		if index, ok := key.(float64); !ok || index < 1 || index > float64(length) || index != math.Trunc(index) {
			return nil, fmt.Errorf("metrics must be a list of tables, not a map")
		}
	}
	metrics := make([]jobmetrics.Metric, 0, length)
	for i := 1; i <= length; i++ {
		entry, ok := list.RawGetInt(i).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("metrics[%d] must be a table", i)
		}
		metric, err := parseJobMetric(entry, i)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	if err := jobmetrics.Validate(metrics); err != nil {
		return nil, err
	}
	if len(metrics) == 0 {
		return nil, nil
	}
	return metrics, nil
}

func parseJobMetric(entry *lua.LTable, index int) (jobmetrics.Metric, error) {
	var metric jobmetrics.Metric
	for key := range luaTableKeys(entry) {
		switch key {
		case "key", "label", "value", "total", "unit", "graph":
		default:
			return metric, fmt.Errorf("metrics[%d] has unknown field %q; expected key, label, value, total, unit or graph", index, key)
		}
	}
	str := func(field string, required bool) (string, error) {
		value := entry.RawGetString(field)
		if value == lua.LNil {
			if required {
				return "", fmt.Errorf("metrics[%d].%s is required", index, field)
			}
			return "", nil
		}
		s, ok := value.(lua.LString)
		if !ok {
			return "", fmt.Errorf("metrics[%d].%s must be a string", index, field)
		}
		return string(s), nil
	}
	num := func(field string) (*float64, error) {
		value := entry.RawGetString(field)
		if value == lua.LNil {
			return nil, nil
		}
		n, ok := value.(lua.LNumber)
		if !ok {
			return nil, fmt.Errorf("metrics[%d].%s must be a number", index, field)
		}
		f := float64(n)
		return &f, nil
	}
	var err error
	if metric.Key, err = str("key", true); err != nil {
		return metric, err
	}
	if metric.Label, err = str("label", false); err != nil {
		return metric, err
	}
	if metric.Label == "" {
		metric.Label = metric.Key
	}
	if metric.Unit, err = str("unit", false); err != nil {
		return metric, err
	}
	value, err := num("value")
	if err != nil {
		return metric, err
	}
	if value == nil {
		return metric, fmt.Errorf("metrics[%d].value is required", index)
	}
	metric.Value = *value
	if metric.Total, err = num("total"); err != nil {
		return metric, err
	}
	switch graph := entry.RawGetString("graph"); graph {
	case lua.LNil, lua.LFalse:
	case lua.LTrue:
		metric.Graph = true
	default:
		return metric, fmt.Errorf("metrics[%d].graph must be a boolean", index)
	}
	return metric, nil
}

// luaTableKeys yields a table's keys as Go values: strings for string keys and
// float64 for numeric ones, so a caller can refuse a key it does not know.
func luaTableKeys(tbl *lua.LTable) map[any]struct{} {
	keys := make(map[any]struct{})
	tbl.ForEach(func(key, _ lua.LValue) {
		switch k := key.(type) {
		case lua.LString:
			keys[string(k)] = struct{}{}
		case lua.LNumber:
			keys[float64(k)] = struct{}{}
		default:
			keys[k.String()] = struct{}{}
		}
	})
	return keys
}

func clampPercent(percent int) int {
	switch {
	case percent < 0:
		return 0
	case percent > 100:
		return 100
	}
	return percent
}
