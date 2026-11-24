package models

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Design D1's index columns, in order. The apply loop reads by (position, id)
// and the bulk marks by id, so id is part of those indexes on every database
// (SQLite's implicit rowid does not stand in for it on Postgres).
func TestReductionTableIndexesHaveTheDesignColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:rtidx?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ReductionClusterRow{}, &ReductionSeed{}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"idx_rc_batch":    "reduction_id, generation, apply_batch, position, id",
		"idx_rc_bulk":     "reduction_id, generation, bulk_op, id",
		"idx_rseed_rank": "reduction_id, generation, key_prefix, tie_rank, resource_id",
	}
	for name, cols := range want {
		var got []string
		if err := db.Raw("SELECT name FROM pragma_index_info(?) ORDER BY seqno", name).Scan(&got).Error; err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, ", ") != cols {
			t.Errorf("%s = (%s), want (%s)", name, strings.Join(got, ", "), cols)
		}
	}
}
