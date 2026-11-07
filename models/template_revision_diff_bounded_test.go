package models

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func numberedLines(n int, tag string) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("%s line %d", tag, i)
	}
	return strings.Join(lines, "\n")
}

// 5,000 x 5,000 lines is 25 million cells: allowed by the unbounded diff, over
// the bounded variant's 1 million, so it takes the coarse delete-then-insert form.
func TestDiffLinesBoundedFallsBackPastTheCellBudget(t *testing.T) {
	a, b := numberedLines(5000, "a"), numberedLines(5000, "b")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	hunks, err := DiffLinesBounded(context.Background(), a, b)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if len(hunks) != 2 || hunks[0].Op != DiffOpDelete || hunks[1].Op != DiffOpInsert {
		t.Fatalf("hunks = %+v", summarize(hunks))
	}
	if len(hunks[0].Lines) != 5000 || len(hunks[1].Lines) != 5000 {
		t.Errorf("coarse form lost lines: %d/%d", len(hunks[0].Lines), len(hunks[1].Lines))
	}
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 8<<20 {
		t.Errorf("allocated %d bytes; the 25M-cell table must not be built", grown)
	}
}

func summarize(h []DiffHunk) []string {
	var out []string
	for _, x := range h {
		out = append(out, fmt.Sprintf("%s:%d", x.Op, len(x.Lines)))
	}
	return out
}

func TestDiffLinesBoundedMatchesDiffLinesWithinBudget(t *testing.T) {
	a := "one\ntwo\nthree\nfour\nfive"
	b := "one\n2\nthree\nfive\nsix"
	want := DiffLines(a, b)
	got, err := DiffLinesBounded(context.Background(), a, b)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(summarize(got)) != fmt.Sprint(summarize(want)) {
		t.Errorf("bounded %v != unbounded %v", summarize(got), summarize(want))
	}
}

// A cancelled context stops the computation instead of finishing the table.
func TestDiffLinesBoundedStopsOnCancel(t *testing.T) {
	a, b := numberedLines(900, "a"), numberedLines(900, "b") // 810,000 cells, within budget
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DiffLinesBounded(ctx, a, b); err == nil {
		t.Fatal("cancelled diff returned a result")
	}
}

func TestDiffLinesBoundedParallelStaysBounded(t *testing.T) {
	a, b := numberedLines(5000, "a"), numberedLines(5000, "b")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := DiffLinesBounded(context.Background(), a, b); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	runtime.ReadMemStats(&after)
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 64<<20 {
		t.Errorf("six parallel diffs allocated %d bytes", grown)
	}
}

func TestDiffTemplateSnapshotsBoundedStopsOnCancel(t *testing.T) {
	from := TemplateSnapshot{"CustomHeader": numberedLines(900, "a")}
	to := TemplateSnapshot{"CustomHeader": numberedLines(900, "b")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DiffTemplateSnapshotsBounded(ctx, "category", from, to); err == nil {
		t.Fatal("cancelled snapshot diff returned a result")
	}
	got, err := DiffTemplateSnapshotsBounded(context.Background(), "category", from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := DiffTemplateSnapshots("category", from, to)
	if len(got) != len(want) {
		t.Errorf("bounded %d slots, unbounded %d", len(got), len(want))
	}
}

// A pure deletion used to allocate one row per deleted line: the admission
// product was zero while the table was (n+1) x 1.
func TestDiffLinesBoundedDeletionOnlyAllocatesNoTable(t *testing.T) {
	a := numberedLines(2_000_000, "d")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	hunks, err := DiffLinesBounded(context.Background(), a, "")
	runtime.ReadMemStats(&after)
	if err != nil || len(hunks) != 1 || hunks[0].Op != DiffOpDelete || len(hunks[0].Lines) != 2_000_000 {
		t.Fatalf("hunks = %v, err = %v", summarize(hunks), err)
	}
	// The split lines themselves are the only large allocation (~2M string headers).
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 400<<20 {
		t.Errorf("allocated %d bytes", grown)
	}
}

func TestDiffLinesBoundedAdmitsRowOverhead(t *testing.T) {
	// 900 x 1,110 middle lines: the product is 999,000, but the table is
	// 901 x 1,111 cells plus row headers, over the 1,000,000 allowance. A shared
	// line in the middle shows whether an LCS ran: the coarse form has no equal hunk.
	mk := func(n int, tag string) string {
		return numberedLines(n/2, tag) + "\nCOMMON\n" + numberedLines(n/2-1, tag+"x")
	}
	hunks, err := DiffLinesBounded(context.Background(), mk(900, "a"), mk(1110, "b"))
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hunks {
		if h.Op == DiffOpEqual {
			t.Fatalf("an LCS table was built: %v", summarize(hunks))
		}
	}
}
