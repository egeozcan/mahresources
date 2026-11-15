package api_handlers

import "testing"

func TestPreviewTextFillsMetaValues(t *testing.T) {
	in := `<div class="h"><h1>Apollo: <meta-shortcode data-path="status" data-value="&#34;active&#34;"></meta-shortcode></h1>` +
		`<p>Size <meta-shortcode data-value="42"></meta-shortcode>, empty <meta-shortcode data-value="null"></meta-shortcode>.</p>` +
		`<style>.h{color:red}</style><script>x()</script><ul><li>one</li><li>two</li></ul></div>`
	want := "Apollo: active\nSize 42, empty .\none\ntwo"
	if got := previewText(in); got != want {
		t.Fatalf("previewText =\n%q\nwant\n%q", got, want)
	}
}
