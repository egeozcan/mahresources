package application_context

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The participant lock must not drag every full name through the lock query (a
// name may be a megabyte, 199 Losers a Cluster), and what the merge keeps of a name
// for version comments and staged hook payloads is cut to 1,024 bytes on a rune
// boundary.
func testLockResourcesCutsNames(t *testing.T, ctx *MahresourcesContext) {
	long := strings.Repeat("日本語é", 5000) // 3- and 2-byte runes, far past the cut
	a := makeResource(t, ctx.db, long, opsRes{Hash: "a"})
	b := makeResource(t, ctx.db, "short", opsRes{Hash: "b"})

	capture := &rowsCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	locked, err := lockResourcesAfterSeries(spy.db, []uint{a, b})
	require.NoError(t, err)

	got := locked[a].Name
	assert.LessOrEqual(t, len(got), 1024)
	assert.Greater(t, len(got), 1000, "the cut keeps nearly the whole budget")
	assert.True(t, utf8.ValidString(got), "the cut lands on a rune boundary")
	assert.True(t, strings.HasPrefix(long, got))
	assert.Equal(t, "short", locked[b].Name)

	bare := regexp.MustCompile(`(?i)\bname\b`)
	capture.mu.Lock()
	defer capture.mu.Unlock()
	checked := 0
	for _, sql := range capture.sqls {
		if !strings.HasPrefix(sql, "SELECT") || !strings.Contains(sql, "resources") {
			continue
		}
		checked++
		withoutCut := regexp.MustCompile(`(?i)SUBSTR\(\s*\W?name\W?\s*,\s*1\s*,\s*\d+\s*\)`).ReplaceAllString(sql, "")
		if strings.Contains(sql, "FOR UPDATE") || strings.Contains(sql, "series_id") {
			assert.False(t, bare.MatchString(withoutCut), "no statement reads the full name: %s", sql)
		}
	}
	require.Greater(t, checked, 0)
}

func TestLockResourcesCutsNamesSQLite(t *testing.T) {
	testLockResourcesCutsNames(t, withSettings(newReductionSQLiteContext(t)))
}
