package application_context

import (
	"fmt"
	"strings"
	"sync/atomic"
)

var testSQLiteDatabaseSequence atomic.Uint64

func testSQLiteMemoryDSN(prefix string) string {
	prefix = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, prefix)
	if prefix == "" {
		prefix = "test"
	}
	return fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", prefix, testSQLiteDatabaseSequence.Add(1))
}
