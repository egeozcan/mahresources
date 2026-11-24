package application_context

import (
	"fmt"
	"testing"

	"mahresources/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- 3: revalidation reads through the one-bind subtree ----------------------------

func TestStoreNarrowResourcesOnAHugeSubtreeSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	home := &models.Group{Name: "home"}
	require.NoError(t, ctx.db.Create(home).Error)
	kids := make([]models.Group, 33000) // past SQLite's 32,766 bind limit
	for i := range kids {
		kids[i] = models.Group{Name: fmt.Sprintf("k%d", i), OwnerId: &home.ID}
	}
	require.NoError(t, ctx.db.CreateInBatches(kids, 500).Error)
	inside := makeResource(t, ctx.db, "in", opsRes{Hash: "h", Owner: &home.ID})
	outside := makeResource(t, ctx.db, "out", opsRes{Hash: "h2"})

	have, err := scopedTo(ctx, home).narrowResources([]uint{inside, outside})
	require.NoError(t, err)
	assert.Equal(t, map[uint]string{inside: "h"}, have, "the Resource outside the subtree is invisible, and 33,000 descendants do not overflow the binds")
}

// ---- 4: a winner's name is withheld once the account has changed -------------------

func TestStoreWinnerNameIsWithheldWhenTheAccountChangedSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	home := &models.Group{Name: "home"}
	require.NoError(t, ctx.db.Create(home).Error)
	winner := makeResource(t, ctx.db, "the-winner", opsRes{Hash: "h", Owner: &home.ID})
	p := accountPrincipal(t, ctx, "viewer", models.RoleUser, &home.ID)
	viewer := ctx.WithPrincipal(p)

	assert.Equal(t, "the-winner", viewer.storeWinnerName(winner))

	require.NoError(t, ctx.db.Exec("UPDATE users SET disabled = ? WHERE id = ?", true, p.UserID).Error)
	assert.Equal(t, "", viewer.storeWinnerName(winner), "a disabled account is told no names")
}
