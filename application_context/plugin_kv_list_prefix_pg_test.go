//go:build postgres && json1 && fts5

package application_context

import (
	"reflect"
	"testing"
)

// PluginKVList builds `key LIKE ? ESCAPE '\'`. SQLite and Postgres both accept
// the explicit ESCAPE clause, but only a run against each shows that a prefix
// holding the escape character itself, or a LIKE wildcard, is matched literally.
func TestPluginKVListPG_PrefixMetacharactersAreLiteral(t *testing.T) {
	ctx := newPostgresKVContext(t)
	const plugin = "like-prefix"

	for _, k := range []string{`a\b1`, `a\b2`, `ab3`, `a%b`, `axb`, `a_b`, `azb`} {
		if err := ctx.PluginKVSet(plugin, k, "v"); err != nil {
			t.Fatalf("set %q: %v", k, err)
		}
	}

	cases := []struct {
		prefix string
		want   []string
	}{
		{`a\b`, []string{`a\b1`, `a\b2`}},
		{`a%`, []string{`a%b`}},
		{`a_`, []string{`a_b`}},
		{`a\`, []string{`a\b1`, `a\b2`}},
	}
	for _, tc := range cases {
		got, err := ctx.PluginKVList(plugin, tc.prefix)
		if err != nil {
			t.Fatalf("list %q: %v", tc.prefix, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("prefix %q: got %q, want %q", tc.prefix, got, tc.want)
		}
	}
}
