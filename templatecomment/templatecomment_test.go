package templatecomment

import (
	"strings"
	"testing"
	"time"
)

func TestStripMappedOffsets(t *testing.T) {
	text := "ab[comment]xx[/comment]cd[comment]y[/comment][comment]z[/comment]ef"
	result := StripMapped(text)
	stripped, toOriginal := result.Text, result.ToOriginal
	if stripped != "abcdef" {
		t.Fatalf("stripped = %q", stripped)
	}
	for i := range len(stripped) + 1 {
		o := toOriginal(i)
		if i < len(stripped) && text[o] != stripped[i] {
			t.Errorf("offset %d maps to %d (%q), want %q", i, o, text[o], stripped[i])
		}
	}
	if got := toOriginal(len(stripped)); got != len(text) {
		t.Errorf("end maps to %d, want %d", got, len(text))
	}
}

func TestScanIsLinearOnUnfinishedOpeners(t *testing.T) {
	start := time.Now()
	ScanText(strings.Repeat("[comment ", 200000))
	ScanText("[comment " + strings.Repeat(`"`, 1000000))
	if d := time.Since(start); d > time.Second {
		t.Errorf("scan took %v", d)
	}
}

func TestRepeatedRemovalIsBounded(t *testing.T) {
	n := 20000
	text := strings.Repeat("[/comm", n) + "[/comment]" + strings.Repeat("ent]", n)
	start := time.Now()
	Strip(text)
	StripMapped(text)
	if d := time.Since(start); d > time.Second {
		t.Errorf("strip took %v", d)
	}
}

func TestStripMappedReportsTagsFormedByAPass(t *testing.T) {
	text := "[comm[comment][/comment]ent]VISIBLE"
	result := StripMapped(text)
	if result.Text != "" {
		t.Fatalf("Text = %q", result.Text)
	}
	if len(result.Unclosed) != 1 {
		t.Fatalf("Unclosed = %v", result.Unclosed)
	}
	if u := result.Unclosed[0]; text[u.Start:u.End] != "[comm[comment][/comment]ent]" {
		t.Errorf("unclosed opener maps to %q", text[u.Start:u.End])
	}
}
