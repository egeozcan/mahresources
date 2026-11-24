package application_context

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"

	"mahresources/models"
)

// sqlRecorder is a gorm logger that keeps every statement in order.
type sqlRecorder struct {
	mu   sync.Mutex
	sqls []string
}

func (r *sqlRecorder) LogMode(logger.LogLevel) logger.Interface      { return r }
func (r *sqlRecorder) Info(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Warn(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Error(context.Context, string, ...interface{}) {}
func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	r.sqls = append(r.sqls, sql)
	r.mu.Unlock()
}

// (10) D3's lock order is Resources first, then the Cluster row. An over-cap
// Cluster's first chunk used to insert the Cluster row and only then share-lock the
// member Resources, the reverse of writeSmallCluster and of a Resource delete's hook.
func testReductionOverCapLocksResourcesBeforeInsertingTheCluster(t *testing.T, ctx *MahresourcesContext) {
	first := seedResources(t, ctx, 260, `'oc' || n`, `'h'`, `NULL`)
	var members []any
	for i := 0; i < 260; i++ {
		members = append(members, map[string]any{"resourceId": first + uint(i), "hash": "h", "inExtent": true})
	}
	plan, err := json.Marshal(map[string]any{"clusters": []any{map[string]any{"id": "big", "tier": "identical", "winnerId": first, "state": "open", "members": members}},
		"settings": map[string]any{"matchingMode": "both", "winnerRule": []string{"pixels"}}})
	require.NoError(t, err)
	legacy := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "legacy", Status: models.ReductionStatusReady, MatchingMode: "both"}, Plan: plan, Extent: []byte(`{"all":true}`)}
	require.NoError(t, ctx.db.Create(legacy).Error)
	require.NoError(t, ctx.SpoolReductionMigrations())

	rec := &sqlRecorder{}
	previous := ctx.db.Config.Logger
	ctx.db.Config.Logger = rec
	t.Cleanup(func() { ctx.db.Config.Logger = previous })
	require.NoError(t, ctx.MigrateReductionPlans())
	ctx.db.Config.Logger = previous
	require.Equal(t, 1, planSchemaOf(t, ctx, legacy.ID))

	lock, insert := -1, -1
	for i, sql := range rec.sqls {
		if lock < 0 && strings.Contains(sql, "SELECT r.id FROM resources r WHERE r.id IN (SELECT s.resource_id FROM reduction_migration_rows") {
			lock = i
		}
		if insert < 0 && (strings.Contains(sql, "INSERT INTO `reduction_clusters`") || strings.Contains(sql, `INSERT INTO "reduction_clusters"`)) {
			insert = i
		}
	}
	require.GreaterOrEqual(t, lock, 0, "the member Resources were locked")
	require.GreaterOrEqual(t, insert, 0, "the Cluster row was inserted")
	require.Less(t, lock, insert, "Resources are locked before the Cluster row is inserted (D3)")
}

func TestReductionOverCapLocksResourcesBeforeInsertingTheClusterSQLite(t *testing.T) {
	testReductionOverCapLocksResourcesBeforeInsertingTheCluster(t, newReductionSQLiteContext(t))
}
