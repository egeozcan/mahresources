//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestTemplateRunRelatedReadersPostgres(t *testing.T) {
	runRelatedReaders(t, newPostgresUserJobContext(t))
}
