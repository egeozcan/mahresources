package application_context

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/query_models"
)

// commitAfterEveryFirstRead has another connection try to commit right after each
// transaction's first read, and counts the attempts.
//
// That is the moment a deferred BEGIN is exposed: the read has taken a WAL snapshot,
// and once anything commits on top of it the transaction's first write fails at once
// with SQLITE_BUSY_SNAPSHOT, which SQLite never puts through the busy handler. The
// server's driver begins every write transaction by taking the writer lock, so the
// attempt is refused and the transaction goes on to commit.
func commitAfterEveryFirstRead(t *testing.T, ctx *MahresourcesContext) *atomic.Int32 {
	t.Helper()
	competitor := noWaitConnection(t, ctx.db)
	var attempts atomic.Int32
	var mu sync.Mutex
	seen := map[gorm.ConnPool]bool{}
	competingCommit := func(tx *gorm.DB) {
		if _, inTransaction := tx.Statement.ConnPool.(gorm.TxCommitter); !inTransaction {
			return
		}
		mu.Lock()
		first := !seen[tx.Statement.ConnPool]
		seen[tx.Statement.ConnPool] = true
		mu.Unlock()
		if !first {
			return
		}
		attempts.Add(1)
		if _, err := competitor.Exec(`UPDATE resource_categories SET description = description || '.' WHERE id = 1`); err != nil && !strings.Contains(err.Error(), "database is locked") {
			t.Errorf("competing commit: %v", err)
		}
	}
	const queryName, rowName = "test:commit_after_first_read_query", "test:commit_after_first_read_row"
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(queryName, competingCommit))
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register(rowName, competingCommit))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Query().Remove(queryName)
		_ = ctx.db.Callback().Row().Remove(rowName)
	})
	return &attempts
}

// requireMappedJob reports a Resource Reduction source whose mapping does not name
// jobID: a write that returned nil without landing.
func requireMappedJob(ctx *MahresourcesContext, sourceID, jobID string) error {
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationReduction, sourceID).First(&mapping).Error; err != nil {
		return err
	}
	if mapping.JobID != jobID {
		return fmt.Errorf("the mapping of %s names %q, want %q", sourceID, mapping.JobID, jobID)
	}
	return nil
}

// Each of these writers reads inside its transaction before it writes. None needs to
// order its statements for SQLite: the driver makes the whole transaction one writer
// from its BEGIN. (Merging resources and recording a dual-published source still
// open with a no-op write, which predates the driver and is redundant under it.)
func TestTransactionsThatReadBeforeTheyWriteSurviveACommitAfterTheirRead(t *testing.T) {
	// Each case seeds what its writer needs and returns the write.
	cases := []struct {
		name  string
		setup func(t *testing.T, ctx *MahresourcesContext) func() error
	}{
		{"creating a group under an owner", func(t *testing.T, ctx *MahresourcesContext) func() error {
			owner := &models.Group{Name: "owner"}
			require.NoError(t, ctx.db.Create(owner).Error)
			return func() error {
				_, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "child", OwnerId: owner.ID})
				return err
			}
		}},
		{"tagging resources in bulk", func(t *testing.T, ctx *MahresourcesContext) func() error {
			resource := &models.Resource{Name: "tagged"}
			tag := &models.Tag{Name: "tag"}
			require.NoError(t, ctx.db.Create(resource).Error)
			require.NoError(t, ctx.db.Create(tag).Error)
			return func() error {
				return ctx.BulkAddTagsToResources(&query_models.BulkEditQuery{
					BulkQuery: query_models.BulkQuery{ID: []uint{resource.ID}}, EditedId: []uint{tag.ID},
				})
			}
		}},
		{"merging tags", func(t *testing.T, ctx *MahresourcesContext) func() error {
			winner, loser := &models.Tag{Name: "winner"}, &models.Tag{Name: "loser"}
			require.NoError(t, ctx.db.Create(winner).Error)
			require.NoError(t, ctx.db.Create(loser).Error)
			return func() error { return ctx.MergeTags(winner.ID, []uint{loser.ID}) }
		}},
		{"editing a relation type", func(t *testing.T, ctx *MahresourcesContext) func() error {
			from, to := &models.Category{Name: "From"}, &models.Category{Name: "To"}
			require.NoError(t, ctx.db.Create(from).Error)
			require.NoError(t, ctx.db.Create(to).Error)
			relationType := &models.GroupRelationType{Name: "links to", FromCategoryId: &from.ID, ToCategoryId: &to.ID}
			require.NoError(t, ctx.db.Create(relationType).Error)
			return func() error {
				_, err := ctx.EditRelationType(&query_models.RelationshipTypeEditorQuery{Id: relationType.ID, Name: "renamed"})
				return err
			}
		}},
		{"renaming a series", func(t *testing.T, ctx *MahresourcesContext) func() error {
			series := &models.Series{Name: "series", Slug: "series"}
			require.NoError(t, ctx.db.Create(series).Error)
			return func() error {
				_, err := ctx.UpdateSeries(&query_models.SeriesEditor{ID: series.ID, Name: "renamed"})
				return err
			}
		}},
		{"merging resources", func(t *testing.T, ctx *MahresourcesContext) func() error {
			winner := &models.Resource{Name: "winner", Hash: "hash-winner", Location: "loc-winner"}
			loser := &models.Resource{Name: "loser", Hash: "hash-loser", Location: "loc-loser"}
			require.NoError(t, ctx.db.Create(winner).Error)
			require.NoError(t, ctx.db.Create(loser).Error)
			return func() error {
				if err := ctx.MergeResources(winner.ID, []uint{loser.ID}, false); err != nil {
					return err
				}
				var remaining int64
				if err := ctx.db.Model(&models.Resource{}).Where("id = ?", loser.ID).Count(&remaining).Error; err != nil {
					return err
				}
				if remaining != 0 {
					return fmt.Errorf("the merge reported success and kept its loser")
				}
				return nil
			}
		}},
		{"recording a new dual-published source", func(t *testing.T, ctx *MahresourcesContext) func() error {
			require.NoError(t, ctx.db.AutoMigrate(&models.JobSourceMapping{}))
			return func() error {
				if err := ctx.recordDualPublishedSource(jobMigrationReduction, "new", "job-1", "hash-1", false, time.Now()); err != nil {
					return err
				}
				return requireMappedJob(ctx, "new", "job-1")
			}
		}},
		{"refreshing a dual-published source", func(t *testing.T, ctx *MahresourcesContext) func() error {
			require.NoError(t, ctx.db.AutoMigrate(&models.JobSourceMapping{}))
			require.NoError(t, ctx.recordDualPublishedSource(jobMigrationReduction, "existing", "job-0", "hash-0", false, time.Now()))
			return func() error {
				if err := ctx.recordDualPublishedSource(jobMigrationReduction, "existing", "job-1", "hash-1", false, time.Now()); err != nil {
					return err
				}
				return requireMappedJob(ctx, "existing", "job-1")
			}
		}},
		{"editing a resource", func(t *testing.T, ctx *MahresourcesContext) func() error {
			resource := &models.Resource{Name: "before"}
			require.NoError(t, ctx.db.Create(resource).Error)
			return func() error {
				_, err := ctx.EditResource(&query_models.ResourceEditor{
					ID: resource.ID, ResourceQueryBase: query_models.ResourceQueryBase{Name: "after"},
				})
				return err
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newWALTestContext(t, 0)
			require.NoError(t, ctx.db.AutoMigrate(&models.Category{}, &models.GroupRelationType{}))
			write := tc.setup(t, ctx)
			attempts := commitAfterEveryFirstRead(t, ctx)
			require.NoError(t, write())
			require.NotZero(t, attempts.Load(), "the writer read nothing inside a transaction, so the interleave never happened")
		})
	}
}
