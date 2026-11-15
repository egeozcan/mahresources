package api_handlers

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPreviewTextFillsMetaValues(t *testing.T) {
	in := `<div class="h"><h1>Apollo: <meta-shortcode data-path="status" data-value="&#34;active&#34;"></meta-shortcode></h1>` +
		`<p>Size <meta-shortcode data-value="42"></meta-shortcode>, empty <meta-shortcode data-path="note" data-value="null"></meta-shortcode>.</p>` +
		`<style>.h{color:red}</style><script>x()</script><ul><li>one</li><li>two</li></ul></div>`
	want := "Apollo: active\nSize 42, empty Note: —.\none\ntwo"
	if got := previewText(in); got != want {
		t.Fatalf("previewText =\n%q\nwant\n%q", got, want)
	}
}

func TestPreviewTextFollowsTheMetaElementEmptyRules(t *testing.T) {
	cases := map[string]string{
		`<meta-shortcode data-path="owner" data-default="Unassigned"></meta-shortcode>`:                                 "Unassigned",
		`<meta-shortcode data-path="owner" data-value="null" data-hide-empty="true" data-default="x"></meta-shortcode>`: "",
		`<meta-shortcode data-path="due_date" data-value="&#34;  &#34;"></meta-shortcode>`:                              "Due Date: —",
		`<meta-shortcode data-path="a.ownerName" data-schema="{&#34;title&#34;:&#34;Owner&#34;}"></meta-shortcode>`:     "Owner: —",
		`<meta-shortcode data-path="a.ownerName"></meta-shortcode>`:                                                     "Owner Name: —",
		`<meta-shortcode data-path="tags" data-value="[&#34;a&#34;,&#34;b&#34;]"></meta-shortcode>`:                     `["a","b"]`,
	}
	for in, want := range cases {
		if got := previewText(in); got != want {
			t.Errorf("previewText(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestPreviewTextIsCapped(t *testing.T) {
	long := "<p>" + strings.Repeat("é", 8000) + "</p>"
	got := previewText(long)
	if len(got) > previewTextLimit+len("…") || !strings.HasSuffix(got, "…") || !utf8.ValidString(got) {
		t.Fatalf("len=%d suffix=%q valid=%v", len(got), got[len(got)-6:], utf8.ValidString(got))
	}
	if short := previewText("<p>short</p>"); short != "short" {
		t.Fatalf("an uncut preview was marked cut: %q", short)
	}
}

func TestPreviewTextUsesSchemaDisplay(t *testing.T) {
	cases := map[string]string{
		`<meta-shortcode data-path="approved" data-value="true" data-schema="{&#34;type&#34;:&#34;boolean&#34;}"></meta-shortcode>`:                                                          "Yes",
		`<meta-shortcode data-path="s" data-value="&#34;wip&#34;" data-schema="{&#34;oneOf&#34;:[{&#34;const&#34;:&#34;wip&#34;,&#34;title&#34;:&#34;In progress&#34;}]}"></meta-shortcode>`: "In progress",
		`<meta-shortcode data-path="n" data-value="3" data-schema="{&#34;oneOf&#34;:[{&#34;const&#34;:3,&#34;title&#34;:&#34;High&#34;}]}"></meta-shortcode>`:                                "High",
	}
	for in, want := range cases {
		if got := previewText(in); got != want {
			t.Errorf("previewText(%s) = %q, want %q", in, got, want)
		}
	}
}
