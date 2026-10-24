package application_context

import (
	"strings"
	"testing"

	"mahresources/shortcodes"
)

func TestCheckTemplateMRQL(t *testing.T) {
	sqlite := MRQLStaticEnv{FTS: true}
	postgres := MRQLStaticEnv{Postgres: true, FTS: true}
	noFTS := MRQLStaticEnv{}
	limit := func(n int) shortcodes.MRQLCheckOptions { return shortcodes.MRQLCheckOptions{Limit: n, Buckets: 5} }
	params := func(kv ...string) shortcodes.MRQLCheckOptions {
		o := limit(20)
		o.Params = map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			o.Params[kv[i]] = kv[i+1]
		}
		return o
	}
	cases := []struct {
		name, query string
		opts        shortcodes.MRQLCheckOptions
		env         MRQLStaticEnv
		wantErr     string // "" means valid
	}{
		{"flat", `type = resource AND tags = "x"`, limit(20), sqlite, ""},
		{"syntax", `type = resource AND`, limit(20), sqlite, "parse"},
		{"bound param", `type = note AND name = $n`, params("n", "x"), sqlite, ""},
		{"missing param", `type = note AND name = $n`, limit(20), sqlite, "n"},
		{"extra param", `type = note`, params("n", "x"), sqlite, "n"},
		{"invented self variable", `type = group AND parent = $self`, limit(20), sqlite, "self"},
		{"type mismatch", `type = resource AND width = "wide"`, limit(20), sqlite, "width"},
		{"category name as type", `type = "book"`, limit(20), sqlite, "book"},
		{"hyphenated meta key", `type = note AND meta.due-date > "2026-01-01"`, limit(20), sqlite, "due"},
		{"regex on sqlite", `type = resource AND name ~* "^a"`, limit(20), sqlite, "PostgreSQL"},
		{"regex on postgres", `type = resource AND name ~* "^a"`, limit(20), postgres, ""},
		{"rank with fts", `type = note AND TEXT ~ "plan" ORDER BY RANK`, limit(20), sqlite, ""},
		{"rank without fts", `type = note AND TEXT ~ "plan" ORDER BY RANK`, limit(20), noFTS, "RANK"},
		{"scope id", `type = resource SCOPE 3`, limit(20), sqlite, ""},
		{"scope name", `type = resource SCOPE "Books"`, limit(20), sqlite, ""},
		{"fractional scope", `type = resource SCOPE 1.5`, limit(20), sqlite, "integer"},
		{"aggregated", `type = resource GROUP BY contentType COUNT()`, limit(20), sqlite, ""},
		{"bucketed", `type = resource GROUP BY contentType`, limit(20), sqlite, ""},
		{"group by without type", `name = "x" GROUP BY name`, limit(20), sqlite, "entity type"},
		{"cross-entity", `name = "x"`, limit(20), sqlite, ""},
		{"cross-entity resource field", `contentType ~ "image/*"`, limit(20), sqlite, "contentType"},
		{"limit over the bound", `type = resource`, limit(10001), sqlite, "limit"},
		{"limit replaces a large LIMIT", `type = resource LIMIT 50000`, limit(20), sqlite, ""},
		{"buckets over the bound", `type = resource GROUP BY contentType`, shortcodes.MRQLCheckOptions{Limit: 20, Buckets: 1001}, sqlite, "bucket"},
		{"offset over the bound", `type = resource OFFSET 20000`, limit(20), sqlite, "offset"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckTemplateMRQL(c.query, c.opts, c.env)
			switch {
			case c.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case c.wantErr != "" && err == nil:
				t.Fatalf("want an error mentioning %q, got none", c.wantErr)
			case c.wantErr != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(c.wantErr)):
				t.Fatalf("error %q does not mention %q", err, c.wantErr)
			}
		})
	}
}
