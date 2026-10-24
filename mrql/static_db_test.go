package mrql

import (
	"strings"
	"testing"
)

func TestStaticDBTranslatesWithoutAConnection(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		db := StaticDB(postgres)
		want := "sqlite"
		if postgres {
			want = "postgres"
		}
		if got := db.Dialector.Name(); got != want {
			t.Fatalf("dialect %q, want %q", got, want)
		}
		if !db.DryRun || db.ConnPool != nil {
			t.Fatalf("StaticDB(%t) has DryRun=%t and a pool %v", postgres, db.DryRun, db.ConnPool)
		}
		q, err := Parse(`type = resource AND name ~ "a*" ORDER BY name LIMIT 5`)
		if err != nil {
			t.Fatal(err)
		}
		fts := true
		if _, err := TranslateWithOptions(q, db, TranslateOptions{FTSAvailable: &fts}); err != nil {
			t.Fatalf("translate on StaticDB(%t): %v", postgres, err)
		}
	}
}

func TestCheckScopeLiteral(t *testing.T) {
	for query, want := range map[string]string{
		`type = resource SCOPE 12`:      "",
		`type = resource SCOPE 0`:       "",
		`type = resource SCOPE "Books"`: "",
		`type = resource`:               "",
		`type = resource SCOPE 1.5`:     "integer group ID",
	} {
		q, err := Parse(query)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		err = CheckScopeLiteral(q)
		switch {
		case want == "" && err != nil:
			t.Errorf("%s: unexpected %v", query, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%s: want %q, got %v", query, want, err)
		}
	}
}

func TestFieldNamesListEveryField(t *testing.T) {
	for et, fields := range map[EntityType][]FieldDef{
		EntityUnspecified: commonFields,
		EntityResource:    resourceFields,
		EntityNote:        noteFields,
		EntityGroup:       groupFields,
	} {
		got := FieldNames(et)
		if len(got) != len(fields) {
			t.Fatalf("%s: %v", et, got)
		}
		for i, f := range fields {
			if got[i] != f.Name {
				t.Errorf("%s: field %d is %q, want %q", et, i, got[i], f.Name)
			}
		}
	}
}
