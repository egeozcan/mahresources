package application_context

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

func TestReductionColumnsCoverEveryColumnButTheLegacyDocuments(t *testing.T) {
	ctx := createTestContext(t)
	require.NoError(t, ctx.db.AutoMigrate(&models.ResourceReduction{}))
	types, err := ctx.db.Migrator().ColumnTypes(&models.ResourceReduction{})
	require.NoError(t, err)
	want := map[string]bool{}
	for _, c := range types {
		if c.Name() != "plan" && c.Name() != "extent" {
			want[c.Name()] = true
		}
	}
	got := map[string]bool{}
	for _, c := range strings.Split(reductionColumns, ",") {
		got[strings.TrimSpace(c)] = true
	}
	require.Equal(t, want, got)
}
