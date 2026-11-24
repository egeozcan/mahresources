package application_context

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// Review fixes to the D12 migration. Every body takes a context so the SQLite and
// Postgres wrappers run the same assertions.

func createLegacyReduction(t *testing.T, ctx *MahresourcesContext, plan, extent string) *models.ResourceReduction {
	t.Helper()
	legacy := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "legacy", Status: models.ReductionStatusReady, MatchingMode: "both"}, Plan: []byte(plan)}
	if extent != "" {
		legacy.Extent = []byte(extent)
	}
	require.NoError(t, ctx.db.Create(legacy).Error)
	return &legacy.ResourceReduction
}

func legacyColumnsAreNull(t *testing.T, ctx *MahresourcesContext, id uint) bool {
	t.Helper()
	var legacy struct{ Plan, Extent *string }
	require.NoError(t, ctx.db.Raw("SELECT plan, extent FROM resource_reductions WHERE id = ?", id).Scan(&legacy).Error)
	return legacy.Plan == nil && legacy.Extent == nil
}

func makeResources(t *testing.T, ctx *MahresourcesContext, n int) []uint {
	t.Helper()
	var ids []uint
	for i := 0; i < n; i++ {
		res := &models.Resource{Name: fmt.Sprintf("m%d", i), Hash: "h", FileSize: int64(10 * (i + 1))}
		require.NoError(t, ctx.db.Create(res).Error)
		ids = append(ids, res.ID)
	}
	return ids
}

// (3) A JSON null where lastApply, coverage or settings should be an object must
// not fail publication: Postgres cannot subtract a key from a jsonb null.
func testReductionMigrationJSONNullDocuments(t *testing.T, ctx *MahresourcesContext) {
	ids := makeResources(t, ctx, 2)
	red := createLegacyReduction(t, ctx, fmt.Sprintf(
		`{"clusters":[{"id":"c1","tier":"identical","winnerId":%d,"state":"open","members":[{"resourceId":%d,"hash":"h"},{"resourceId":%d,"hash":"h"}]}],`+
			`"coverage":null,"settings":null,"lastApply":null}`, ids[0], ids[0], ids[1]), "")
	require.NoError(t, ctx.MigrateReductionPlans())
	var got models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&got, red.ID).Error)
	require.Equal(t, 1, got.PlanSchema)
	require.Empty(t, got.LastApply)
	require.True(t, legacyColumnsAreNull(t, ctx, red.ID))
}

// (4) Free text copied from the legacy plan is not bounded by a varchar: a plugin
// veto reason or a margin can be longer than the old limits.
func testReductionMigrationKeepsLongLegacyText(t *testing.T, ctx *MahresourcesContext) {
	ids := makeResources(t, ctx, 2)
	long := strings.Repeat("r", 500)
	red := createLegacyReduction(t, ctx, fmt.Sprintf(
		`{"clusters":[{"id":"c1","tier":"identical","winnerId":%d,"state":"stale","staleReason":%q,"margin":%q,"decidedBy":%q,`+
			`"members":[{"resourceId":%d,"hash":"h"},{"resourceId":%d,"hash":"h","ejected":true,"ejectedReason":%q}]},`+
			`{"id":"c2","tier":"near","winnerId":%d,"state":"applied","merged":true,"claimId":%q,"members":[{"resourceId":%d,"hash":"h"}]}],"coverage":{}}`,
		ids[0], long, long, long, ids[0], ids[1], long, ids[0], long, ids[0]), "")
	require.NoError(t, ctx.MigrateReductionPlans())
	var got models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&got, red.ID).Error)
	require.Equal(t, 1, got.PlanSchema, "the long legacy text must not fail the migration")
	var c models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ? AND \"key\" = ?", red.ID, "c1").First(&c).Error)
	require.Equal(t, long, c.StaleReason)
	require.Equal(t, long, c.Margin)
	require.Equal(t, long, c.DecidedBy)
	var m models.ReductionMemberRow
	require.NoError(t, ctx.db.Where("cluster_id = ? AND ejected = ?", c.ID, true).First(&m).Error)
	require.Equal(t, long, m.EjectedReason)
}

// (5) An id beyond int64 is skipped and counted, never clamped to the largest
// int64, and the Reduction still migrates.
func testReductionMigrationSkipsOutOfRangeIDs(t *testing.T, ctx *MahresourcesContext) {
	ids := makeResources(t, ctx, 2)
	const huge = "9223372036854775808"
	red := createLegacyReduction(t, ctx, fmt.Sprintf(
		`{"clusters":[{"id":"c1","tier":"identical","winnerId":%d,"state":"open","members":[{"resourceId":%d,"hash":"h"},{"resourceId":%s,"hash":"h"},{"resourceId":%d,"hash":"h"}]}],"coverage":{}}`,
		ids[0], ids[0], huge, ids[1]),
		fmt.Sprintf(`{"resourceIds":[%d,%s,-4],"groupIds":[3,%s]}`, ids[0], huge, huge))

	require.NoError(t, ctx.SpoolReductionMigrations())
	var rejected int64
	require.NoError(t, ctx.db.Model(&models.ReductionMigrationRow{}).Where("reduction_id = ? AND rejected = ?", red.ID, true).Count(&rejected).Error)
	require.EqualValues(t, 4, rejected, "one member, two Resource ids and one Group id were refused")

	require.NoError(t, ctx.MigrateReductionPlans())
	var got models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&got, red.ID).Error)
	require.Equal(t, 1, got.PlanSchema)
	var clamped int64
	require.NoError(t, ctx.db.Model(&models.ReductionExtentResource{}).Where("reduction_id = ? AND resource_id NOT IN ?", red.ID, []uint{ids[0]}).Count(&clamped).Error)
	require.EqualValues(t, 0, clamped, "no id was clamped into a row")
	require.NoError(t, ctx.db.Model(&models.ReductionExtentGroup{}).Where("reduction_id = ? AND group_id <> 3", red.ID).Count(&clamped).Error)
	require.EqualValues(t, 0, clamped)
	var members int64
	require.NoError(t, ctx.db.Model(&models.ReductionMemberRow{}).Where("reduction_id = ?", red.ID).Count(&members).Error)
	require.EqualValues(t, 2, members, "the two valid members only")
}

// (6) Publication compares the member rows actually written, not the sum of the
// counts the Clusters claim.
func testReductionMigrationPublicationCountsMemberRows(t *testing.T, ctx *MahresourcesContext) {
	red, _ := seedLegacyReduction(t, ctx, false)
	require.NoError(t, ctx.SpoolReductionMigrations())
	s := ctx.reductionStore()
	mig, err := s.takeOverMigration(red.ID)
	require.NoError(t, err)
	require.NoError(t, s.migrateCluster(mig, spoolHead{ClusterOrd: 0, N: 3}))
	require.NoError(t, s.migrateCluster(mig, spoolHead{ClusterOrd: 1, N: 2}))
	require.NoError(t, ctx.db.Exec("DELETE FROM reduction_members WHERE id = (SELECT MIN(id) FROM reduction_members WHERE reduction_id = ?)", red.ID).Error)

	require.Error(t, s.publishMigration(mig), "a member row went missing: publication must refuse")
	var got models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&got, red.ID).Error)
	require.Equal(t, 0, got.PlanSchema)
	require.False(t, legacyColumnsAreNull(t, ctx, red.ID), "the legacy plan is kept")
}

// (7) A Resource deleted after the migration read it but before its member
// committed must not land as live: the chunk share-locks the Resources it reads.
func testReductionMigrationShareLocksTheResourcesItReads(t *testing.T, ctx *MahresourcesContext) {
	red, ids := seedLegacyReduction(t, ctx, true)
	var attempts, blocked atomic.Int32
	orig := migrationAfterResourceLock
	t.Cleanup(func() { migrationAfterResourceLock = orig })
	migrationAfterResourceLock = func(locked []uint) {
		for _, id := range locked {
			attempts.Add(1)
			err := ctx.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec("SET LOCAL lock_timeout = '300ms'").Error; err != nil {
					return err
				}
				return tx.Exec("DELETE FROM resources WHERE id = ?", id).Error
			})
			if err != nil {
				blocked.Add(1)
			}
		}
	}
	require.NoError(t, ctx.MigrateReductionPlans())
	require.Positive(t, attempts.Load())
	require.Equal(t, attempts.Load(), blocked.Load(), "every Resource a chunk read was locked against a delete until it committed")
	var n int64
	require.NoError(t, ctx.db.Model(&models.Resource{}).Where("id IN ?", ids).Count(&n).Error)
	require.EqualValues(t, len(ids), n)
	_ = red
}

// (8) A Cluster over the cap is written in fenced chunks of its own, each committed
// with its checkpoint, so an interruption resumes inside the Cluster.
func testReductionMigrationOverCapClusterResumesMidCluster(t *testing.T, ctx *MahresourcesContext) {
	ids := makeResources(t, ctx, 3)
	var members []string
	members = append(members, fmt.Sprintf(`{"resourceId":%d,"hash":"h"}`, ids[0]))
	for i := 0; i < 1199; i++ {
		members = append(members, fmt.Sprintf(`{"resourceId":%d,"hash":"h"}`, 300000+i))
	}
	red := createLegacyReduction(t, ctx, fmt.Sprintf(
		`{"clusters":[{"id":"big","tier":"identical","winnerId":%d,"state":"open","members":[%s]},`+
			`{"id":"after","tier":"identical","winnerId":%d,"state":"open","members":[{"resourceId":%d,"hash":"h"},{"resourceId":%d,"hash":"h"}]}],"coverage":{}}`,
		ids[0], strings.Join(members, ","), ids[1], ids[1], ids[2]), "")

	var chunks atomic.Int32
	orig := migrationAfterChunk
	t.Cleanup(func() { migrationAfterChunk = orig })
	migrationAfterChunk = func() error {
		if chunks.Add(1) == 1 {
			return errors.New("interrupted after the first chunk")
		}
		return nil
	}
	require.Error(t, ctx.MigrateReductionPlans())

	var written int64
	require.NoError(t, ctx.db.Model(&models.ReductionMemberRow{}).Where("reduction_id = ?", red.ID).Count(&written).Error)
	require.EqualValues(t, reductionRowChunk, written, "the first chunk committed on its own")
	var mig models.ReductionMigration
	require.NoError(t, ctx.db.Where("reduction_id = ?", red.ID).First(&mig).Error)
	require.EqualValues(t, 0, mig.Checkpoint, "the Cluster is not finished")
	require.EqualValues(t, reductionRowChunk, mig.MemberCheckpoint, "the checkpoint is inside the Cluster")

	require.NoError(t, ctx.db.Exec("UPDATE reduction_migrations SET lease_until = 1 WHERE reduction_id = ?", red.ID).Error)
	require.NoError(t, ctx.MigrateReductionPlans())
	var got models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&got, red.ID).Error)
	require.Equal(t, 1, got.PlanSchema)
	var big models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ? AND \"key\" = ?", red.ID, "big").First(&big).Error)
	require.Equal(t, 1200, big.MemberCount)
	require.Equal(t, 1199, big.GoneMembers)
	require.True(t, big.ReadOnly)
	var distinct int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(DISTINCT position) FROM reduction_members WHERE cluster_id = ?", big.ID).Scan(&distinct).Error)
	require.EqualValues(t, 1200, distinct, "resuming duplicated or skipped no member")
	var after models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ? AND \"key\" = ?", red.ID, "after").First(&after).Error)
	require.Equal(t, 2, after.MemberCount)
}

// (9) A Reduction another instance published between this one listing it and
// locking it gets no new migration state.
func testReductionMigrationSkipsAReductionPublishedMeanwhile(t *testing.T, ctx *MahresourcesContext) {
	red, _ := seedLegacyReduction(t, ctx, false)
	orig := spoolAfterList
	t.Cleanup(func() { spoolAfterList = orig })
	spoolAfterList = func() {
		spoolAfterList = nil
		require.NoError(t, ctx.MigrateReductionPlans())
	}
	require.NoError(t, ctx.SpoolReductionMigrations())
	var n int64
	require.NoError(t, ctx.db.Model(&models.ReductionMigration{}).Where("reduction_id = ?", red.ID).Count(&n).Error)
	require.EqualValues(t, 0, n, "no migration row for a published Reduction")
	require.NoError(t, ctx.db.Model(&models.ReductionMigrationRow{}).Where("reduction_id = ?", red.ID).Count(&n).Error)
	require.EqualValues(t, 0, n, "no spool rows either")
}

// (10) The legacy settings are read once per Reduction, not once per Cluster.
func testReductionMigrationReadsSettingsOncePerReduction(t *testing.T, ctx *MahresourcesContext) {
	ids := makeResources(t, ctx, 2)
	var clusters []string
	for i := 0; i < 30; i++ {
		clusters = append(clusters, fmt.Sprintf(`{"id":"c%d","tier":"identical","winnerId":%d,"state":"open","members":[{"resourceId":%d,"hash":"h"},{"resourceId":%d,"hash":"h"}]}`, i, ids[0], ids[0], ids[1]))
	}
	createLegacyReduction(t, ctx, `{"clusters":[`+strings.Join(clusters, ",")+`],"coverage":{}}`, "")
	var reads atomic.Int32
	require.NoError(t, ctx.db.Callback().Row().Before("gorm:row").Register("count_settings_reads", func(db *gorm.DB) {
		if strings.Contains(db.Statement.SQL.String(), "winner_rule FROM resource_reductions") {
			reads.Add(1)
		}
	}))
	require.NoError(t, ctx.MigrateReductionPlans())
	require.LessOrEqual(t, reads.Load(), int32(1), "settings are parsed once, not per Cluster")
}

// (13) A Cluster over the cap whose only warning is a lossy field still needs
// attention.
func testReductionMigrationOverCapLossyNeedsAttention(t *testing.T, ctx *MahresourcesContext) {
	ids := makeResources(t, ctx, 1)
	var members []string
	members = append(members, fmt.Sprintf(`{"resourceId":%d,"hash":"h"}`, ids[0]))
	for i := 0; i < 250; i++ {
		members = append(members, fmt.Sprintf(`{"resourceId":%d,"hash":"h"}`, 400000+i))
	}
	red := createLegacyReduction(t, ctx, fmt.Sprintf(
		`{"clusters":[{"id":"big","tier":"identical","winnerId":%d,"state":"open","lossy":["description"],"members":[%s]}],"coverage":{},"settings":{}}`,
		ids[0], strings.Join(members, ",")), "")
	require.NoError(t, ctx.MigrateReductionPlans())
	var big models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ? AND \"key\" = ?", red.ID, "big").First(&big).Error)
	require.True(t, big.NeedsAttention, "lossy alone sets needs_attention, as deriveClusterColumns does")
}

// (14) Deleting the spool inside a caller's transaction must not run another
// dialect's statement first: on Postgres that aborts the transaction.
func testReductionDeleteSpoolInsideATransaction(t *testing.T, ctx *MahresourcesContext) {
	red, _ := seedLegacyReduction(t, ctx, false)
	require.NoError(t, ctx.SpoolReductionMigrations())
	var gen string
	require.NoError(t, ctx.db.Raw("SELECT generation FROM reduction_migrations WHERE reduction_id = ?", red.ID).Scan(&gen).Error)
	require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
		s := &reductionStore{db: tx}
		if err := s.deleteSpool(red.ID, gen); err != nil {
			return err
		}
		return tx.Exec("SELECT 1").Error
	}))
	var n int64
	require.NoError(t, ctx.db.Model(&models.ReductionMigrationRow{}).Where("reduction_id = ?", red.ID).Count(&n).Error)
	require.EqualValues(t, 0, n)
}

func TestReductionMigrationJSONNullDocumentsSQLite(t *testing.T) {
	testReductionMigrationJSONNullDocuments(t, newReductionSQLiteContext(t))
}
func TestReductionMigrationKeepsLongLegacyTextSQLite(t *testing.T) {
	testReductionMigrationKeepsLongLegacyText(t, newReductionSQLiteContext(t))
}
func TestReductionMigrationSkipsOutOfRangeIDsSQLite(t *testing.T) {
	testReductionMigrationSkipsOutOfRangeIDs(t, newReductionSQLiteContext(t))
}
func TestReductionMigrationPublicationCountsMemberRowsSQLite(t *testing.T) {
	testReductionMigrationPublicationCountsMemberRows(t, newReductionSQLiteContext(t))
}
func TestReductionMigrationOverCapClusterResumesMidClusterSQLite(t *testing.T) {
	testReductionMigrationOverCapClusterResumesMidCluster(t, newReductionSQLiteContext(t))
}
func TestReductionMigrationSkipsAReductionPublishedMeanwhileSQLite(t *testing.T) {
	testReductionMigrationSkipsAReductionPublishedMeanwhile(t, newReductionSQLiteContext(t))
}
func TestReductionMigrationReadsSettingsOncePerReductionSQLite(t *testing.T) {
	testReductionMigrationReadsSettingsOncePerReduction(t, newReductionSQLiteContext(t))
}
func TestReductionMigrationOverCapLossyNeedsAttentionSQLite(t *testing.T) {
	testReductionMigrationOverCapLossyNeedsAttention(t, newReductionSQLiteContext(t))
}
func TestReductionDeleteSpoolInsideATransactionSQLite(t *testing.T) {
	testReductionDeleteSpoolInsideATransaction(t, newReductionSQLiteContext(t))
}

// One Reduction whose plan the database cannot parse fails alone: the others
// still migrate and its legacy plan is kept. (SQLite stores the plan as text, so
// only it can hold a malformed document.)
func TestReductionMigrationOneBadPlanDoesNotBlockTheRest(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	bad := createLegacyReduction(t, ctx, `{"clusters": [`, "")
	good, _ := seedLegacyReduction(t, ctx, false)
	require.Error(t, ctx.MigrateReductionPlans(), "the failure is reported")
	var got models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&got, good.ID).Error)
	require.Equal(t, 1, got.PlanSchema, "the good Reduction migrated")
	var badRow models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&badRow, bad.ID).Error)
	require.Equal(t, 0, badRow.PlanSchema)
	require.False(t, legacyColumnsAreNull(t, ctx, bad.ID), "the bad plan is kept")
}
