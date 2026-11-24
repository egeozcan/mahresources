package application_context

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// "Arrived since" must cost the arrivals, not the history: a Group related to many old
// Resources must not have those relationship rows read to count a few new Resources.
func testStoreArrivalsDoNotReadRelationshipHistory(t *testing.T, ctx *MahresourcesContext) {
	group := &models.Group{Name: "G"}
	require.NoError(t, ctx.db.Create(group).Error)
	computed := time.Now().Add(-time.Hour)
	old := computed.Add(-48 * time.Hour)

	var oldResources []models.Resource
	for i := 0; i < 60; i++ {
		oldResources = append(oldResources, models.Resource{Name: "old", Hash: "h", CreatedAt: old})
	}
	require.NoError(t, ctx.db.CreateInBatches(&oldResources, 30).Error)
	for _, res := range oldResources {
		require.NoError(t, ctx.db.Exec("INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)", group.ID, res.ID).Error)
	}
	owned := models.Resource{Name: "owned", Hash: "h", OwnerId: &group.ID}
	related := models.Resource{Name: "related", Hash: "h"}
	stranger := models.Resource{Name: "stranger", Hash: "h"}
	for _, res := range []*models.Resource{&owned, &related, &stranger} {
		require.NoError(t, ctx.db.Create(res).Error)
	}
	require.NoError(t, ctx.db.Exec("INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)", group.ID, related.ID).Error)

	r := tableReduction(t, ctx)
	require.NoError(t, ctx.db.Create(&models.ReductionExtentGroup{ReductionID: r.ID, GroupID: group.ID}).Error)

	// A tiny chunk so the count crosses chunk boundaries.
	defer func(old int) { reductionArrivalChunk = old }(reductionArrivalChunk)
	reductionArrivalChunk = 1

	capture := &rowsCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	n, _, err := spy.reductionStore().enteredSince(r, computed)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "the owned arrival and the related one; the stranger and the old copies are not arrivals")

	capture.mu.Lock()
	defer capture.mu.Unlock()
	for _, sql := range capture.sqls {
		if strings.Contains(sql, "groups_related_resources") {
			assert.Regexp(t, `x\.id IN \(\d`, sql, "the relationship table is only probed for a chunk of arrivals: %s", sql)
		}
	}
}

func TestStoreArrivalsDoNotReadRelationshipHistorySQLite(t *testing.T) {
	testStoreArrivalsDoNotReadRelationshipHistory(t, newReductionSQLiteContext(t))
}
