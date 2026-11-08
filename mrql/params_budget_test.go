package mrql

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// 500 repetitions of $p fit the IN-list and token limits. Each used to decode a
// quoted string parameter through a fresh lexer, retaining ~500 copies.
func TestBindParamsDecodesEachDistinctParameterOnce(t *testing.T) {
	big := `"` + strings.Repeat("x", 1<<20) + `"`
	refs := make([]string, 500)
	for i := range refs {
		refs[i] = "$p"
	}
	q, err := Parse("type = resource AND name IN (" + strings.Join(refs, ", ") + ")")
	if err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if err := BindParams(q, map[string]any{"p": big}); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 16<<20 {
		t.Errorf("binding allocated %d bytes; the 1 MiB parameter must be decoded once", grown)
	}
	in := q.Where.(*BinaryExpr).Right.(*InExpr)
	if len(in.Values) != 500 {
		t.Fatalf("values = %d", len(in.Values))
	}
	for i, v := range in.Values {
		s, ok := v.(*StringLiteral)
		if !ok || len(s.Value) != 1<<20 {
			t.Fatalf("value %d = %T", i, v)
		}
	}
}

func TestBindParamsKeepsPerOccurrenceTokens(t *testing.T) {
	q, err := Parse("type = resource AND name IN ($p, $p)")
	if err != nil {
		t.Fatal(err)
	}
	if err := BindParams(q, map[string]any{"p": "a b"}); err != nil {
		t.Fatal(err)
	}
	in := q.Where.(*BinaryExpr).Right.(*InExpr)
	a, b := in.Values[0].(*StringLiteral), in.Values[1].(*StringLiteral)
	if a == b {
		t.Error("occurrences share one node; each needs its own so positions stay distinct")
	}
	if a.Token.Pos == b.Token.Pos {
		t.Errorf("both occurrences report position %d", a.Token.Pos)
	}
}

func TestBindParamsAdmitsAggregateBytesAndCount(t *testing.T) {
	q, _ := Parse("type = resource AND name = $p")
	huge := map[string]any{"p": strings.Repeat("x", MaxParamBytes+1)}
	if err := BindParams(q, huge); err == nil {
		t.Error("parameter bytes past the allowance accepted")
	}
	many := map[string]any{"p": "x"}
	for i := 0; i < MaxParamCount+1; i++ {
		many[fmt.Sprintf("e%d", i)] = "x"
	}
	q, _ = Parse("type = resource AND name = $p")
	err := BindParams(q, many)
	if err == nil {
		t.Fatal("excess parameters accepted")
	}
	if len(err.Error()) > 4096 {
		t.Errorf("diagnostic is %d bytes; it must be bounded", len(err.Error()))
	}
}

func TestBindParamsUnknownDiagnosticIsBounded(t *testing.T) {
	q, _ := Parse("type = resource AND name = $p")
	params := map[string]any{"p": "x"}
	for i := 0; i < 200; i++ {
		params[fmt.Sprintf("e%03d", i)] = "x"
	}
	err := BindParams(q, params)
	if err == nil || !strings.Contains(err.Error(), "unknown parameters: $e000, $e001") {
		t.Fatalf("err = %v", err)
	}
	if len(err.Error()) > 512 {
		t.Errorf("diagnostic is %d bytes", len(err.Error()))
	}
	// A few extras keep the exact message.
	q, _ = Parse("type = resource AND name = $p")
	err = BindParams(q, map[string]any{"p": "x", "b": 1, "a": 2})
	if err == nil || err.Error() != "unknown parameters: $a, $b" {
		t.Errorf("err = %v", err)
	}
}

func TestBindParamsContextCancelled(t *testing.T) {
	refs := strings.Repeat("$p, ", 400) + "$p"
	q, _ := Parse("type = resource AND name IN (" + refs + ")")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := BindParamsContext(ctx, q, map[string]any{"p": "x"}); err == nil {
		t.Error("cancelled context ignored")
	}
}

func TestLikePatternEscapedOncePerValue(t *testing.T) {
	big := strings.Repeat("a%b_", 1<<18) // 1 MiB with characters that need escaping
	refs := make([]string, 200)
	for i := range refs {
		refs[i] = "name ~ $p"
	}
	q, err := Parse("type = resource AND (" + strings.Join(refs, " OR ") + ")")
	if err != nil {
		t.Fatal(err)
	}
	if err := BindParams(q, map[string]any{"p": big}); err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if _, err := TranslateWithOptions(q, StaticDB(false), TranslateOptions{}); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	// 200 occurrences x (escape chain ~ 6 copies of ~1.5 MiB) was ~1.8 GB.
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 400<<20 {
		t.Errorf("translation allocated %d bytes", grown)
	}
}

// Review r1: a memo with a cap let an operand used after the cap was full be
// lowercased once per occurrence.
func TestLowerMemoCoversEveryDistinctOperand(t *testing.T) {
	big := strings.ToUpper(strings.Repeat("x", 1<<20))
	var in []string
	for i := 0; i < 70; i++ { // more distinct small operands than the old cap
		in = append(in, fmt.Sprintf("\"s%d\"", i))
	}
	for i := 0; i < 400; i++ {
		in = append(in, "$p")
	}
	q, err := Parse("type = resource AND name IN (" + strings.Join(in, ", ") + ")")
	if err != nil {
		t.Fatal(err)
	}
	if err := BindParams(q, map[string]any{"p": big}); err != nil {
		t.Fatal(err)
	}
	grown := allocBytes(func() {
		if _, err := TranslateWithOptions(q, StaticDB(false), TranslateOptions{}); err != nil {
			t.Fatal(err)
		}
	})
	if grown > 64<<20 {
		t.Errorf("translation allocated %d bytes; 400 occurrences must share one lowered copy", grown)
	}
}

func TestLikeMemoCoversEveryDistinctOperand(t *testing.T) {
	big := strings.Repeat("a%", 1<<19)
	var parts []string
	for i := 0; i < 70; i++ {
		parts = append(parts, fmt.Sprintf("name ~ \"s%d\"", i))
	}
	for i := 0; i < 150; i++ {
		parts = append(parts, "name ~ $p")
	}
	q, err := Parse("type = resource AND (" + strings.Join(parts, " OR ") + ")")
	if err != nil {
		t.Fatal(err)
	}
	if err := BindParams(q, map[string]any{"p": big}); err != nil {
		t.Fatal(err)
	}
	grown := allocBytes(func() {
		if _, err := TranslateWithOptions(q, StaticDB(false), TranslateOptions{}); err != nil {
			t.Fatal(err)
		}
	})
	if grown > 200<<20 {
		t.Errorf("translation allocated %d bytes", grown)
	}
}

func TestBindParamsNamesCountAndAreBounded(t *testing.T) {
	q, _ := Parse("type = resource AND name = $p")
	err := BindParams(q, map[string]any{"p": "x", strings.Repeat("n", 5<<20): "x"})
	if err == nil || len(err.Error()) > 512 {
		t.Fatalf("err = %v", err)
	}
	// Names add to the aggregate. Every supplied name is referenced by the
	// query, so an unknown-parameter error cannot be what refuses it: only the
	// aggregate-byte admission can.
	params := map[string]any{"p": strings.Repeat("x", MaxParamBytes-100)}
	refs := []string{"$p"}
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("n%0100d", i)
		params[name] = "x"
		refs = append(refs, "$"+name)
	}
	q, err = Parse("type = resource AND name IN (" + strings.Join(refs, ", ") + ")")
	if err != nil {
		t.Fatal(err)
	}
	err = BindParams(q, params)
	if err == nil || !strings.Contains(err.Error(), "parameter values total more than") {
		t.Errorf("names were not counted in the aggregate: %v", err)
	}
	// Control: the same query with the big value shrunk binds.
	params["p"] = "x"
	q, _ = Parse("type = resource AND name IN (" + strings.Join(refs, ", ") + ")")
	if err := BindParams(q, params); err != nil {
		t.Errorf("control failed: %v", err)
	}
}

func allocBytes(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}
