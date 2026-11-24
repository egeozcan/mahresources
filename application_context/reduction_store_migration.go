package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mahresources/models"
)

// The one-time migration of a legacy JSON plan into the plan tables (design
// D12). Two steps:
//
//   - spool: one INSERT ... SELECT per Reduction parses the legacy plan and extent
//     once, in the database, into reduction_migration_rows. It is the only
//     statement that writes more than a chunk. On SQLite it holds the writer lock,
//     so the caller runs it at startup, before serving.
//   - process: the spool is read by keyset (cluster_ord) and written as Clusters
//     and members in fenced transactions, then published in one transaction that
//     also nulls the legacy columns.
//
// The run's generation, writer token, lease and checkpoint live in its
// reduction_migrations row, never on the Reduction row that still carries the
// blob.

const (
	reductionMigrationLease  = 10 * time.Minute
	reductionMigrationStatus = "interrupted by an upgrade; compute again"
	// staleReasonTooLarge is the reason a legacy Cluster over the member cap carries.
	staleReasonTooLarge = "too large: compute again"
	// migrationBatchPrefix names the synthetic outcome batch of a migrated apply record.
	migrationBatchPrefix = "migrated-"
)

var errReductionMigrationBusy = errors.New("reduction migration is held by another runner")

// Test seams, nil outside a test. migrationAfterResourceLock runs once a chunk holds
// the Resources it read (their ids), migrationAfterChunk after each committed chunk
// of an over-cap Cluster, spoolAfterList between listing the Reductions to spool
// and locking the first.
var (
	migrationAfterResourceLock func(locked []uint)
	migrationAfterChunk        func() error
	spoolAfterList             func()
)

// ---- spool ---------------------------------------------------------------------

const spoolColumns = `reduction_id, generation, kind, cluster_ord, member_ord, cluster_key, tier, state, winner_id,
 checked, reviewed, oversized, decided_by, margin, undecided, keeps_smaller, lossy, stale_reason, merged, claim_id,
 applied_at, resource_id, hash, in_extent, ejected, ejected_reason, distance, owner_at_merge, changed, rejected`

// An id in a legacy document is valid when it is a positive integer that fits an
// int64. Anything else (beyond int64, negative, fractional, not a number) is
// spooled as 0 with rejected set, so it is skipped and counted, never clamped or
// allowed to fail the statement.

// sqliteSpoolID is x (a json_extract or json_each value) when it is a valid id, else 0.
func sqliteSpoolID(x string) string {
	return "(CASE WHEN typeof(" + x + ") = 'integer' AND " + x + " > 0 THEN " + x + " ELSE 0 END)"
}

// pgSpoolID is the same for a jsonb expression. 19 digits compare as text against
// the largest int64, which is the same order for equal-length digit strings.
func pgSpoolID(x string) string {
	t := "((" + x + ") #>> '{}')"
	return "(CASE WHEN jsonb_typeof(" + x + ") = 'number' AND " + t + " ~ '^[0-9]{1,19}$' THEN " +
		"(CASE WHEN length(" + t + ") < 19 OR " + t + " <= '9223372036854775807' THEN " + t + "::bigint ELSE 0 END) ELSE 0 END)"
}

// spoolMemberSelectSQLite parses clusters and members with json_each. A JSON null
// where an array is expected (an empty apply writes clusters: null) expands to
// nothing.
var spoolMemberSelectSQLite = `SELECT r.id, ?, 'member', CAST(c.key AS INTEGER), COALESCE(CAST(m.key AS INTEGER), 0),
 COALESCE(json_extract(c.value, '$.id'), ''), COALESCE(json_extract(c.value, '$.tier'), ''), COALESCE(json_extract(c.value, '$.state'), ''),
 ` + sqliteSpoolID("json_extract(c.value, '$.winnerId')") + `,
 COALESCE(json_extract(c.value, '$.checked'), 0), COALESCE(json_extract(c.value, '$.reviewed'), 0), COALESCE(json_extract(c.value, '$.oversized'), 0),
 COALESCE(json_extract(c.value, '$.decidedBy'), ''), COALESCE(json_extract(c.value, '$.margin'), ''), COALESCE(json_extract(c.value, '$.undecided'), 0),
 COALESCE(json_extract(c.value, '$.keepsSmaller'), 0), json_extract(c.value, '$.lossy'), COALESCE(json_extract(c.value, '$.staleReason'), ''),
 COALESCE(json_extract(c.value, '$.merged'), 0), COALESCE(json_extract(c.value, '$.claimId'), ''), json_extract(c.value, '$.appliedAt'),
 ` + sqliteSpoolID("json_extract(m.value, '$.resourceId')") + `, COALESCE(json_extract(m.value, '$.hash'), ''),
 COALESCE(json_extract(m.value, '$.inExtent'), 0), COALESCE(json_extract(m.value, '$.ejected'), 0), COALESCE(json_extract(m.value, '$.ejectedReason'), ''),
 (CASE WHEN typeof(json_extract(m.value, '$.distance')) = 'integer' AND json_extract(m.value, '$.distance') BETWEEN 0 AND 2147483647 THEN json_extract(m.value, '$.distance') END),
 ` + sqliteSpoolID(`json_extract(c.value, '$.loserOwners."' || json_extract(m.value, '$.resourceId') || '"')`) + `,
 COALESCE((SELECT 1 FROM json_each(c.value, '$.changedMembers') cm WHERE json_type(c.value, '$.changedMembers') = 'array' AND cm.value = json_extract(m.value, '$.resourceId') LIMIT 1), 0),
 (CASE WHEN m.value IS NOT NULL AND ` + sqliteSpoolID("json_extract(m.value, '$.resourceId')") + ` = 0 THEN 1 ELSE 0 END)
 FROM resource_reductions r
 JOIN json_each(CASE WHEN json_type(r.plan, '$.clusters') = 'array' THEN json_extract(r.plan, '$.clusters') ELSE '[]' END) c
 LEFT JOIN json_each(c.value, '$.members') m ON json_type(c.value, '$.members') = 'array' AND m.type = 'object'
 WHERE r.id = ?`

var spoolMemberSelectPG = `SELECT r.id, ?, 'member', (c.ord - 1)::bigint, COALESCE((m.ord - 1)::bigint, 0),
 COALESCE(c.v->>'id', ''), COALESCE(c.v->>'tier', ''), COALESCE(c.v->>'state', ''), ` + pgSpoolID("c.v->'winnerId'") + `,
 COALESCE((c.v->>'checked')::boolean, false), COALESCE((c.v->>'reviewed')::boolean, false), COALESCE((c.v->>'oversized')::boolean, false),
 COALESCE(c.v->>'decidedBy', ''), COALESCE(c.v->>'margin', ''), COALESCE((c.v->>'undecided')::boolean, false),
 COALESCE((c.v->>'keepsSmaller')::boolean, false), c.v->'lossy', COALESCE(c.v->>'staleReason', ''),
 COALESCE((c.v->>'merged')::boolean, false), COALESCE(c.v->>'claimId', ''), (c.v->>'appliedAt')::timestamptz,
 ` + pgSpoolID("m.v->'resourceId'") + `, COALESCE(m.v->>'hash', ''),
 COALESCE((m.v->>'inExtent')::boolean, false), COALESCE((m.v->>'ejected')::boolean, false), COALESCE(m.v->>'ejectedReason', ''),
 (CASE WHEN jsonb_typeof(m.v->'distance') = 'number' AND (m.v->>'distance') ~ '^[0-9]{1,9}$' THEN (m.v->>'distance')::bigint END),
 ` + pgSpoolID("c.v->'loserOwners'->(m.v->>'resourceId')") + `,
 COALESCE(CASE WHEN jsonb_typeof(c.v->'changedMembers') = 'array' THEN (c.v->'changedMembers') @> (m.v->'resourceId') END, false),
 (m.v IS NOT NULL AND ` + pgSpoolID("m.v->'resourceId'") + ` = 0)
 FROM resource_reductions r
 CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(r.plan::jsonb->'clusters') = 'array' THEN r.plan::jsonb->'clusters' ELSE '[]'::jsonb END) WITH ORDINALITY AS c(v, ord)
 LEFT JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(c.v->'members') = 'array' THEN c.v->'members' ELSE '[]'::jsonb END) WITH ORDINALITY AS m(v, ord) ON jsonb_typeof(m.v) = 'object'
 WHERE r.id = ?`

// listSpoolSelect expands one id list of the legacy documents into spool rows of
// the given kind: the list's own ordinal is member_ord and the value is the
// resource_id (extent kinds, 0 and rejected when it is not a valid id) or
// cluster_key (apply_cluster).
func listSpoolSelect(postgres bool, kind, document, path string) string {
	valueCol := "0"
	keyCol := "''"
	rejected := "false"
	if kind == "apply_cluster" {
		keyCol = "e.value"
		if postgres {
			keyCol = "e.v #>> '{}'"
		}
	} else {
		valueCol = sqliteSpoolID("e.value")
		rejected = "(" + valueCol + " = 0)"
		if postgres {
			valueCol = pgSpoolID("e.v")
			rejected = "(" + valueCol + " = 0)"
		}
	}
	if postgres {
		jsonPath := "r." + document + "::jsonb" + path
		return fmt.Sprintf(`SELECT r.id, ?, '%s', 0, (e.ord - 1)::bigint, %s, '', '', 0, false, false, false, '', '', false, false, NULL, '', false, '', NULL, %s, '', false, false, '', NULL, 0, false, %s
 FROM resource_reductions r
 CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(%s) = 'array' THEN %s ELSE '[]'::jsonb END) WITH ORDINALITY AS e(v, ord)
 WHERE r.id = ?`, kind, keyCol, valueCol, rejected, jsonPath, jsonPath)
	}
	sqlitePath := "$" + strings.ReplaceAll(strings.ReplaceAll(path, "->", "."), "'", "")
	return fmt.Sprintf(`SELECT r.id, ?, '%s', 0, CAST(e.key AS INTEGER), %s, '', '', 0, 0, 0, 0, '', '', 0, 0, NULL, '', 0, '', NULL, %s, '', 0, 0, '', NULL, 0, 0, %s
 FROM resource_reductions r
 JOIN json_each(CASE WHEN json_type(r.%s, '%s') = 'array' THEN json_extract(r.%s, '%s') ELSE '[]' END) e
 WHERE r.id = ?`, kind, keyCol, valueCol, rejected, document, sqlitePath, document, sqlitePath)
}

// spoolReduction parses one legacy Reduction into the spool and returns the
// counts of clusters and members the pass read. One statement per document.
func (s *reductionStore) spoolReduction(tx *gorm.DB, id uint, generation string) (clusters, members, rejected int64, err error) {
	pg := tx.Dialector.Name() == "postgres"
	member := spoolMemberSelectSQLite
	if pg {
		member = spoolMemberSelectPG
	}
	statements := []string{member,
		listSpoolSelect(pg, "extent_resource", "extent", "->'resourceIds'"),
		listSpoolSelect(pg, "extent_group", "extent", "->'groupIds'"),
		listSpoolSelect(pg, "apply_cluster", "plan", "->'lastApply'->'clusterIds'")}
	if !pg {
		statements[1] = listSpoolSelect(false, "extent_resource", "extent", ".resourceIds")
		statements[2] = listSpoolSelect(false, "extent_group", "extent", ".groupIds")
		statements[3] = listSpoolSelect(false, "apply_cluster", "plan", ".lastApply.clusterIds")
	}
	for _, sel := range statements {
		if err := tx.Exec("INSERT INTO reduction_migration_rows ("+spoolColumns+") "+sel, generation, id).Error; err != nil {
			return 0, 0, 0, fmt.Errorf("spool reduction %d: %w", id, err)
		}
	}
	if err := tx.Raw(`SELECT COUNT(DISTINCT cluster_ord) FROM reduction_migration_rows WHERE reduction_id = ? AND generation = ? AND kind = 'member'`, id, generation).Scan(&clusters).Error; err != nil {
		return 0, 0, 0, err
	}
	if err := tx.Raw(`SELECT COUNT(*) FROM reduction_migration_rows WHERE reduction_id = ? AND generation = ? AND kind = 'member' AND resource_id > 0`, id, generation).Scan(&members).Error; err != nil {
		return 0, 0, 0, err
	}
	if err := tx.Raw(`SELECT COUNT(*) FROM reduction_migration_rows WHERE reduction_id = ? AND generation = ? AND rejected = ?`, id, generation, true).Scan(&rejected).Error; err != nil {
		return 0, 0, 0, err
	}
	return clusters, members, rejected, nil
}

// SpoolReductionMigrations is the first step: every plan_schema = 0 Reduction
// without a migration row is spooled in its own transaction, which also creates
// its reduction_migrations row. Run it at startup before serving (design D12).
func (ctx *MahresourcesContext) SpoolReductionMigrations() error {
	s := ctx.reductionStore()
	var ids []uint
	if err := s.db.Raw(`SELECT id FROM resource_reductions WHERE plan_schema = 0 AND NOT EXISTS
		(SELECT 1 FROM reduction_migrations m WHERE m.reduction_id = resource_reductions.id) ORDER BY id`).Scan(&ids).Error; err != nil {
		return err
	}
	if spoolAfterList != nil {
		spoolAfterList()
	}
	// One Reduction the database cannot parse fails alone; it keeps its legacy
	// plan and the others are still spooled.
	var firstErr error
	for _, id := range ids {
		var rejected int64
		err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := lockReductionRowNarrow(tx, id); err != nil {
				return err
			}
			// Another instance may have published it since the list was read.
			if legacy, err := reductionStillLegacy(tx, id); err != nil || !legacy {
				return err
			}
			var exists int64
			if err := tx.Raw(`SELECT COUNT(*) FROM reduction_migrations WHERE reduction_id = ?`, id).Scan(&exists).Error; err != nil || exists > 0 {
				return err
			}
			generation := uuid.NewString()
			clusters, members, rej, err := s.spoolReduction(tx, id, generation)
			if err != nil {
				return err
			}
			rejected = rej
			return tx.Create(&models.ReductionMigration{ReductionID: id, Generation: generation,
				PassClusters: clusters, PassMembers: members}).Error
		})
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			ctx.Logger().Warning(models.LogActionSystem, "", nil, "", fmt.Sprintf("reduction %d spool failed, its legacy plan is kept: %v", id, err), nil)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if rejected > 0 {
			ctx.Logger().Warning(models.LogActionSystem, "", nil, "", fmt.Sprintf("reduction %d migration skipped %d ids that are not positive integers within int64", id, rejected), nil)
		}
	}
	return firstErr
}

// reductionStillLegacy reads plan_schema under the row lock the caller holds.
func reductionStillLegacy(tx *gorm.DB, id uint) (bool, error) {
	var schema []int
	if err := tx.Raw("SELECT plan_schema FROM resource_reductions WHERE id = ?", id).Scan(&schema).Error; err != nil {
		return false, err
	}
	return len(schema) == 1 && schema[0] == 0, nil
}

// lockReductionRowNarrow takes the Reduction row's lock on Postgres without
// reading the legacy blob; SQLite's writer lock (BEGIN IMMEDIATE) already
// excludes other writers. A missing row is gorm.ErrRecordNotFound.
func lockReductionRowNarrow(tx *gorm.DB, id uint) error {
	suffix := ""
	if tx.Dialector.Name() == "postgres" {
		suffix = " FOR UPDATE"
	}
	var got []uint
	if err := tx.Raw("SELECT id FROM resource_reductions WHERE id = ?"+suffix, id).Scan(&got).Error; err != nil {
		return err
	}
	if len(got) == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ---- process -------------------------------------------------------------------

// MigrateReductionPlans runs both steps over every plan_schema = 0 Reduction.
// Safe to run again: a published Reduction is skipped, a half-done one resumes
// from its checkpoint.
func (ctx *MahresourcesContext) MigrateReductionPlans() error {
	firstErr := ctx.SpoolReductionMigrations()
	s := ctx.reductionStore()
	var ids []uint
	if err := s.db.Raw(`SELECT reduction_id FROM reduction_migrations ORDER BY reduction_id`).Scan(&ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.migrateReduction(id); err != nil && !errors.Is(err, errReductionMigrationBusy) {
			ctx.Logger().Warning(models.LogActionSystem, "", nil, "", fmt.Sprintf("reduction %d migration: %v", id, err), nil)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// takeOverMigration rotates the writer token and renews the lease, refusing a
// live lease held by another token. It returns the row as it now stands.
func (s *reductionStore) takeOverMigration(id uint) (*models.ReductionMigration, error) {
	token := uuid.NewString()
	var out models.ReductionMigration
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockReductionRowNarrow(tx, id); err != nil {
			return err
		}
		if legacy, err := reductionStillLegacy(tx, id); err != nil {
			return err
		} else if !legacy {
			return gorm.ErrRecordNotFound
		}
		now := time.Now()
		res := tx.Exec(`UPDATE reduction_migrations SET writer_token = ?, lease_until = ? WHERE reduction_id = ? AND (lease_until < ? OR writer_token = '')`,
			token, now.Add(reductionMigrationLease).UnixNano(), id, now.UnixNano())
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errReductionMigrationBusy
		}
		return tx.Where("reduction_id = ?", id).First(&out).Error
	})
	return &out, err
}

// fence proves, inside a transaction, that this runner still owns the migration:
// the Reduction row locked first, then the migration row's token and lease, with
// a clock read after the lock. Its write is also what starts the transaction.
func (s *reductionStore) fenceMigration(tx *gorm.DB, mig *models.ReductionMigration) error {
	if err := lockReductionRowNarrow(tx, mig.ReductionID); err != nil {
		return err
	}
	res := tx.Exec(`UPDATE reduction_migrations SET lease_until = ? WHERE reduction_id = ? AND generation = ? AND writer_token = ? AND lease_until >= ?`,
		time.Now().Add(reductionMigrationLease).UnixNano(), mig.ReductionID, mig.Generation, mig.WriterToken, time.Now().UnixNano())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errReductionMigrationBusy
	}
	return nil
}

// legacySettingsProbe is whether a legacy plan recorded no settings and the Winner
// Rule to derive keeps-smaller from when it did not.
type legacySettingsProbe struct {
	none bool
	rule []string
}

type spoolHead struct {
	ClusterOrd int
	N          int64
}

func (s *reductionStore) migrateReduction(id uint) error {
	mig, err := s.takeOverMigration(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	for {
		var heads []spoolHead
		if err := s.db.Raw(`SELECT cluster_ord, COUNT(CASE WHEN resource_id > 0 THEN 1 END) AS n FROM reduction_migration_rows
			WHERE reduction_id = ? AND generation = ? AND kind = 'member' AND cluster_ord >= ?
			GROUP BY cluster_ord ORDER BY cluster_ord LIMIT 1`, id, mig.Generation, mig.Checkpoint).Scan(&heads).Error; err != nil {
			return err
		}
		if len(heads) == 0 {
			break
		}
		if err := s.migrateCluster(mig, heads[0]); err != nil {
			return err
		}
		mig.Checkpoint = int64(heads[0].ClusterOrd) + 1
	}
	return s.publishMigration(mig)
}

// spoolRow is one spooled member with its Cluster's scalars (the table's own model).
type spoolRow = models.ReductionMigrationRow

func clusterRowFromSpool(r *spoolRow, reductionID uint, generation string) models.ReductionClusterRow {
	row := models.ReductionClusterRow{
		ReductionID: reductionID, Generation: generation, Key: r.ClusterKey, Position: int64(r.ClusterOrd),
		Tier: r.Tier, State: r.State, WinnerID: r.WinnerID, Checked: r.Checked, Reviewed: r.Reviewed, Oversized: r.Oversized,
		DecidedBy: r.DecidedBy, Margin: r.Margin, Undecided: r.Undecided, KeepsSmaller: r.KeepsSmaller,
		StaleReason: r.StaleReason, Merged: r.Merged, ClaimID: r.ClaimID, AppliedAt: r.AppliedAt,
	}
	if len(r.Lossy) > 0 && string(r.Lossy) != "null" {
		row.Lossy = r.Lossy
	}
	return row
}

// migrateCluster writes one spooled Cluster and advances the checkpoint, in one
// fenced transaction.
func (s *reductionStore) migrateCluster(mig *models.ReductionMigration, head spoolHead) error {
	var first spoolRow
	if err := s.db.Where("reduction_id = ? AND generation = ? AND kind = 'member' AND cluster_ord = ?", mig.ReductionID, mig.Generation, head.ClusterOrd).
		Order("member_ord").Limit(1).Take(&first).Error; err != nil {
		return err
	}
	row := clusterRowFromSpool(&first, mig.ReductionID, mig.Generation)
	// Old plans keep the legacy cluster-level fields only when decoded here, and a
	// merged Cluster's claim (inactive claims are normalized, design D3).
	if row.State != models.ReductionClusterApplied || row.Merged {
		if row.State != models.ReductionClusterApplied {
			row.ClaimID = ""
		}
	}
	if head.N > int64(reductionMemberCap) {
		return s.migrateOverCapCluster(mig, head, &row)
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.fenceMigration(tx, mig); err != nil {
			return err
		}
		if err := s.writeSmallCluster(tx, mig, head, &row); err != nil {
			return err
		}
		return tx.Exec(`UPDATE reduction_migrations SET checkpoint = ?, member_checkpoint = 0 WHERE reduction_id = ? AND writer_token = ?`,
			head.ClusterOrd+1, mig.ReductionID, mig.WriterToken).Error
	})
}

func (s *reductionStore) writeSmallCluster(tx *gorm.DB, mig *models.ReductionMigration, head spoolHead, row *models.ReductionClusterRow) error {
	var spool []spoolRow
	if head.N > 0 {
		if err := tx.Where("reduction_id = ? AND generation = ? AND kind = 'member' AND cluster_ord = ? AND resource_id > 0", mig.ReductionID, mig.Generation, head.ClusterOrd).
			Order("member_ord").Find(&spool).Error; err != nil {
			return err
		}
	}
	ids := make([]uint, 0, len(spool))
	for _, m := range spool {
		ids = append(ids, m.ResourceID)
	}
	type fact struct {
		ID       uint
		FileSize int64
		Width    uint
		Height   uint
	}
	facts := map[uint]fact{}
	if len(ids) > 0 {
		// Share-locked in ascending id (Postgres), so a delete either committed
		// before this read (gone) or waits for these rows to commit, when the
		// deletion hook sees them (D4): gone is never stale.
		var found []fact
		if err := tx.Raw(`SELECT id, file_size, width, height FROM resources WHERE id IN ? ORDER BY id`+shareLock(tx), ids).Scan(&found).Error; err != nil {
			return err
		}
		locked := make([]uint, 0, len(found))
		for _, f := range found {
			facts[f.ID] = f
			locked = append(locked, f.ID)
		}
		if migrationAfterResourceLock != nil {
			migrationAfterResourceLock(locked)
		}
	}
	c := &storedCluster{Row: *row}
	for _, m := range spool {
		f, ok := facts[m.ResourceID]
		c.Members = append(c.Members, models.ReductionMemberRow{
			ReductionID: mig.ReductionID, Generation: mig.Generation, ResourceID: m.ResourceID, Hash: m.Hash,
			FileSize: f.FileSize, InExtent: m.InExtent, Ejected: m.Ejected, EjectedReason: m.EjectedReason,
			Distance: m.Distance, OwnerAtMerge: m.OwnerAtMerge, Gone: !ok, Changed: m.Changed,
		})
	}
	// A plan stored before settings were recorded has no keeps-smaller flag; derive it
	// from the Winner Rule the way the old read did (deleted Resources tolerated).
	if first := spoolRow(spool0(spool)); first.State != models.ReductionClusterApplied {
		if has, rule, err := s.legacyHasNoSettings(tx, mig.ReductionID); err != nil {
			return err
		} else if has {
			var cands []clusterCandidate
			for i, m := range c.Members {
				if f, ok := facts[m.ResourceID]; ok && !m.Ejected {
					cands = append(cands, clusterCandidate{WinnerCandidate: models.WinnerCandidate{Resource: &models.Resource{ID: f.ID, Width: f.Width, Height: f.Height}}})
				}
				_ = i
			}
			c.Row.KeepsSmaller = keepsSmaller(cands, c.Row.WinnerID, rule, c.Row.DecidedBy)
		}
	}
	for i := range c.Members {
		c.Members[i].Position = i
	}
	deriveClusterColumns(c)
	if err := tx.Create(&c.Row).Error; err != nil {
		return err
	}
	for i := range c.Members {
		c.Members[i].ClusterID = c.Row.ID
	}
	if len(c.Members) == 0 {
		return nil
	}
	return tx.CreateInBatches(c.Members, reductionRowChunk).Error
}

func spool0(rows []spoolRow) spoolRow {
	if len(rows) == 0 {
		return spoolRow{}
	}
	return rows[0]
}

// legacyHasNoSettings reports whether the legacy plan recorded no settings, and
// the Winner Rule to derive keeps-smaller from, reading only those two small
// values out of the document in SQL. The answer is read once per Reduction: a plan
// with 100,000 small Clusters does not re-parse its settings for each.
func (s *reductionStore) legacyHasNoSettings(tx *gorm.DB, id uint) (bool, []string, error) {
	if p, ok := s.settingsProbe[id]; ok {
		return p.none, p.rule, nil
	}
	pg := tx.Dialector.Name() == "postgres"
	expr := "json_type(plan, '$.settings')"
	if pg {
		expr = "jsonb_typeof(plan::jsonb->'settings')"
	}
	var row struct {
		T          *string
		WinnerRule []byte
	}
	if err := tx.Raw("SELECT "+expr+" AS t, winner_rule FROM resource_reductions WHERE id = ?", id).Scan(&row).Error; err != nil {
		return false, nil, err
	}
	probe := legacySettingsProbe{none: row.T == nil || *row.T != "object"}
	if probe.none {
		probe.rule = DecodeWinnerRule(row.WinnerRule)
	}
	if s.settingsProbe == nil {
		s.settingsProbe = map[uint]legacySettingsProbe{}
	}
	s.settingsProbe[id] = probe
	return probe.none, probe.rule, nil
}

// shareLock is the locking suffix of a read whose rows must not be deleted until
// the transaction commits. SQLite's writer lock already serializes against deletes.
func shareLock(tx *gorm.DB) string {
	if tx.Dialector.Name() == "postgres" {
		return " FOR SHARE"
	}
	return ""
}

// migrateOverCapCluster writes a legacy Cluster over the member cap without
// hydrating it, in fenced transactions of one chunk each: the Cluster row with the
// first, then reductionRowChunk members per transaction by keyset on member_ord,
// each committing the (cluster_ord, member_ord) checkpoint it reached, so an
// interruption resumes inside the Cluster and no transaction holds SQLite's writer
// lock across the whole of it. The aggregates come from SQL after the last chunk.
// One that is not applied and merged becomes a read-only stale Cluster ("too
// large: compute again").
func (s *reductionStore) migrateOverCapCluster(mig *models.ReductionMigration, head spoolHead, row *models.ReductionClusterRow) error {
	history := row.State == models.ReductionClusterApplied && row.Merged
	row.ReadOnly = true
	if !history {
		row.State = models.ReductionClusterStale
		row.StaleReason = staleReasonTooLarge
		row.Checked = false
		row.ClaimID = ""
	}
	if int64(head.ClusterOrd) != mig.Checkpoint {
		mig.MemberCheckpoint = 0
	}
	const spoolCluster = `s.reduction_id = ? AND s.generation = ? AND s.kind = 'member' AND s.cluster_ord = ? AND s.resource_id > 0`
	for {
		finished := false
		next := mig.MemberCheckpoint
		err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := s.fenceMigration(tx, mig); err != nil {
				return err
			}
			var existing []uint
			if err := tx.Raw(`SELECT id FROM reduction_clusters WHERE reduction_id = ? AND generation = ? AND "key" = ?`,
				mig.ReductionID, mig.Generation, row.Key).Scan(&existing).Error; err != nil {
				return err
			}
			if len(existing) == 0 {
				row.ID = 0
				if err := tx.Create(row).Error; err != nil {
					return err
				}
			} else {
				row.ID = existing[0]
			}
			var hi int64
			if err := tx.Raw(`SELECT COALESCE(MAX(member_ord), -1) FROM (SELECT s.member_ord FROM reduction_migration_rows s WHERE `+spoolCluster+`
				AND s.member_ord >= ? ORDER BY s.member_ord LIMIT ?) chunk`,
				mig.ReductionID, mig.Generation, head.ClusterOrd, mig.MemberCheckpoint, reductionRowChunk).Scan(&hi).Error; err != nil {
				return err
			}
			if hi < 0 {
				finished = true
				if err := s.finishOverCapCluster(tx, row); err != nil {
					return err
				}
				return tx.Exec(`UPDATE reduction_migrations SET checkpoint = ?, member_checkpoint = 0 WHERE reduction_id = ? AND writer_token = ?`,
					head.ClusterOrd+1, mig.ReductionID, mig.WriterToken).Error
			}
			rangeArgs := []any{mig.ReductionID, mig.Generation, head.ClusterOrd, mig.MemberCheckpoint, hi}
			var locked []uint
			if err := tx.Raw(`SELECT r.id FROM resources r WHERE r.id IN (SELECT s.resource_id FROM reduction_migration_rows s WHERE `+spoolCluster+`
				AND s.member_ord >= ? AND s.member_ord <= ?) ORDER BY r.id`+shareLock(tx), rangeArgs...).Scan(&locked).Error; err != nil {
				return err
			}
			if migrationAfterResourceLock != nil {
				migrationAfterResourceLock(locked)
			}
			if err := tx.Exec(`INSERT INTO reduction_members (cluster_id, reduction_id, generation, resource_id, position, hash, file_size, in_extent, ejected, ejected_reason, distance, owner_at_merge, gone, changed)
				SELECT ?, s.reduction_id, s.generation, s.resource_id, s.member_ord, s.hash, COALESCE(r.file_size, 0), s.in_extent, s.ejected, s.ejected_reason, s.distance, s.owner_at_merge,
				 (r.id IS NULL), s.changed
				FROM reduction_migration_rows s LEFT JOIN resources r ON r.id = s.resource_id
				WHERE `+spoolCluster+` AND s.member_ord >= ? AND s.member_ord <= ?
				ORDER BY s.member_ord`, append([]any{row.ID}, rangeArgs...)...).Error; err != nil {
				return err
			}
			next = hi + 1
			return tx.Exec(`UPDATE reduction_migrations SET member_checkpoint = ? WHERE reduction_id = ? AND writer_token = ?`,
				next, mig.ReductionID, mig.WriterToken).Error
		})
		if err != nil {
			return err
		}
		if finished {
			mig.MemberCheckpoint = 0
			return nil
		}
		mig.MemberCheckpoint = next
		if migrationAfterChunk != nil {
			if err := migrationAfterChunk(); err != nil {
				return err
			}
		}
	}
}

// finishOverCapCluster derives an over-cap Cluster's columns with SQL aggregates
// over its member rows, never a hydrated Cluster.
func (s *reductionStore) finishOverCapCluster(tx *gorm.DB, row *models.ReductionClusterRow) error {
	var agg struct {
		N, Losers, Gone int
		Size            int64
	}
	if err := tx.Raw(`SELECT COUNT(*) AS n, COALESCE(SUM(CASE WHEN ejected = ? AND resource_id <> ? THEN 1 ELSE 0 END), 0) AS losers,
		COALESCE(SUM(CASE WHEN ejected = ? AND gone = ? THEN 1 ELSE 0 END), 0) AS gone, COALESCE(SUM(file_size), 0) AS size
		FROM reduction_members WHERE cluster_id = ?`, false, row.WinnerID, false, true, row.ID).Scan(&agg).Error; err != nil {
		return err
	}
	fingerprint := ""
	return tx.Model(&models.ReductionClusterRow{}).Where("id = ?", row.ID).Updates(map[string]any{
		"member_count": agg.N, "loser_count": agg.Losers, "gone_members": agg.Gone, "total_size": agg.Size,
		"fingerprint": fingerprint, "needs_attention": lossyFieldCount(row.Lossy) > 0 || row.Oversized || row.KeepsSmaller,
	}).Error
}

// ---- publication ---------------------------------------------------------------

func (s *reductionStore) publishMigration(mig *models.ReductionMigration) error {
	id := mig.ReductionID
	// Extent ids and the apply record's Cluster ids move before publication, in
	// chunks, each idempotent (a resumed run repeats them harmlessly).
	if err := s.moveSpooledLists(mig); err != nil {
		return err
	}
	pg := s.db.Dialector.Name() == "postgres"
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.fenceMigration(tx, mig); err != nil {
			return err
		}
		// The member rows are counted, not the sum of the counts the Clusters claim: a
		// missing member row must stop publication, which nulls the legacy plan.
		var written struct{ Clusters, Members int64 }
		if err := tx.Raw(`SELECT (SELECT COUNT(*) FROM reduction_clusters WHERE reduction_id = ? AND generation = ?) AS clusters,
			(SELECT COUNT(*) FROM reduction_members WHERE reduction_id = ? AND generation = ?) AS members`, id, mig.Generation, id, mig.Generation).Scan(&written).Error; err != nil {
			return err
		}
		var pass models.ReductionMigration
		if err := tx.Where("reduction_id = ?", id).First(&pass).Error; err != nil {
			return err
		}
		if written.Clusters != pass.PassClusters || written.Members != pass.PassMembers {
			return fmt.Errorf("migration of reduction %d wrote %d clusters and %d members, the spool read %d and %d", id, written.Clusters, written.Members, pass.PassClusters, pass.PassMembers)
		}
		// Small documents only: the legacy plan stays in the database.
		var docs struct {
			Coverage, Settings, LastApply []byte
			All                           bool
			Status                        string
		}
		if pg {
			err := tx.Raw(`SELECT plan::jsonb->'coverage' AS coverage, plan::jsonb->'settings' AS settings,
				CASE WHEN jsonb_typeof(plan::jsonb->'lastApply') = 'object' THEN (plan::jsonb->'lastApply') - 'clusterIds' END AS last_apply,
				COALESCE((extent::jsonb->>'all')::boolean, false) AS "all", status FROM resource_reductions WHERE id = ?`, id).Scan(&docs).Error
			if err != nil {
				return err
			}
		} else {
			err := tx.Raw(`SELECT json_extract(plan, '$.coverage') AS coverage, json_extract(plan, '$.settings') AS settings,
				CASE WHEN json_type(plan, '$.lastApply') = 'object' THEN json_remove(json_extract(plan, '$.lastApply'), '$.clusterIds') END AS last_apply,
				COALESCE(json_extract(extent, '$.all'), 0) AS "all", status FROM resource_reductions WHERE id = ?`, id).Scan(&docs).Error
			if err != nil {
				return err
			}
		}
		lastApply := []byte(nil)
		if len(docs.LastApply) > 0 && string(docs.LastApply) != "null" {
			var rec map[string]any
			if err := json.Unmarshal(docs.LastApply, &rec); err != nil {
				return err
			}
			rec["batchId"] = migrationBatchPrefix + mig.Generation
			lastApply, _ = json.Marshal(rec)
		}
		updates := map[string]any{
			"plan_schema": 1, "plan_generation": mig.Generation, "extent_all": docs.All,
			"plan": nil, "extent": nil,
			"coverage": nullJSON(docs.Coverage), "plan_settings": nullJSON(docs.Settings), "last_apply": nullJSON(lastApply),
		}
		if docs.Status == models.ReductionStatusComputing {
			updates["status"] = models.ReductionStatusFailed
			updates["compute_error"] = reductionMigrationStatus
		}
		res := tx.Model(&models.ResourceReduction{}).Where("id = ? AND plan_schema = 0", id).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return fmt.Errorf("migration of reduction %d: publication matched %d rows", id, res.RowsAffected)
		}
		return tx.Exec(`DELETE FROM reduction_migrations WHERE reduction_id = ? AND generation = ? AND writer_token = ?`, id, mig.Generation, mig.WriterToken).Error
	})
	if err != nil {
		return err
	}
	return s.deleteSpool(id, mig.Generation)
}

// nullJSON is nil for an empty or JSON-null document, so the column stays NULL.
func nullJSON(b []byte) any {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
}

// moveSpooledLists moves the legacy Extent ids and the apply record's Cluster
// ids out of the spool in chunks of reductionRowChunk, fenced.
func (s *reductionStore) moveSpooledLists(mig *models.ReductionMigration) error {
	steps := []struct{ kind, sql string }{
		{"extent_resource", `INSERT INTO reduction_extent_resources (reduction_id, resource_id)
			SELECT reduction_id, resource_id FROM reduction_migration_rows WHERE reduction_id = ? AND generation = ? AND kind = 'extent_resource' AND resource_id > 0 AND member_ord >= ? AND member_ord < ?
			ON CONFLICT DO NOTHING`},
		{"extent_group", `INSERT INTO reduction_extent_groups (reduction_id, group_id)
			SELECT reduction_id, resource_id FROM reduction_migration_rows WHERE reduction_id = ? AND generation = ? AND kind = 'extent_group' AND resource_id > 0 AND member_ord >= ? AND member_ord < ?
			ON CONFLICT DO NOTHING`},
		{"apply_cluster", `UPDATE reduction_clusters SET outcome_batch = '` + migrationBatchPrefix + mig.Generation + `'
			WHERE reduction_id = ? AND generation = ? AND "key" IN
			 (SELECT cluster_key FROM reduction_migration_rows WHERE reduction_id = ? AND generation = ? AND kind = 'apply_cluster' AND member_ord >= ? AND member_ord < ?)`},
	}
	for _, step := range steps {
		var max *int64
		if err := s.db.Raw(`SELECT MAX(member_ord) FROM reduction_migration_rows WHERE reduction_id = ? AND generation = ? AND kind = ?`, mig.ReductionID, mig.Generation, step.kind).Scan(&max).Error; err != nil {
			return err
		}
		if max == nil {
			continue
		}
		for from := int64(0); from <= *max; from += reductionRowChunk {
			err := s.db.Transaction(func(tx *gorm.DB) error {
				if err := s.fenceMigration(tx, mig); err != nil {
					return err
				}
				if step.kind == "apply_cluster" {
					return tx.Exec(step.sql, mig.ReductionID, mig.Generation, mig.ReductionID, mig.Generation, from, from+reductionRowChunk).Error
				}
				return tx.Exec(step.sql, mig.ReductionID, mig.Generation, from, from+reductionRowChunk).Error
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteSpool removes a finished generation's spool rows in chunks.
func (s *reductionStore) deleteSpool(id uint, generation string) error {
	// Branch before executing: on Postgres a failed statement aborts the caller's
	// transaction, so the other dialect's row address must never be tried first.
	statement := `DELETE FROM reduction_migration_rows WHERE rowid IN (SELECT rowid FROM reduction_migration_rows WHERE reduction_id = ? AND generation = ? LIMIT ?)`
	if s.db.Dialector.Name() == "postgres" {
		statement = `DELETE FROM reduction_migration_rows WHERE ctid IN (SELECT ctid FROM reduction_migration_rows WHERE reduction_id = ? AND generation = ? LIMIT ?)`
	}
	for {
		res := s.db.Exec(statement, id, generation, reductionRowChunk)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
	}
}
