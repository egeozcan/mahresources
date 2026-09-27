package application_context

import "testing"

func TestSQLiteDSNFilePath(t *testing.T) {
	for dsn, want := range map[string]string{
		"mydb.db":                        "mydb.db",
		"/data/mydb.db?_busy_timeout=10": "/data/mydb.db",
		"file:mydb.db?_journal_mode=WAL": "mydb.db",
		"file:/data/mydb.db":             "/data/mydb.db",
		ephemeralDatabaseDSN("/tmp/odd?dir#part%41/1_2.db", "_journal_mode=WAL&mode=ro"): "/tmp/odd?dir#part%41/1_2.db",
	} {
		if got := sqliteDSNFilePath(dsn); got != want {
			t.Errorf("sqliteDSNFilePath(%q) = %q, want %q", dsn, got, want)
		}
	}
}
