package application_context

import (
	"mahresources/constants"
	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// stampedModels is the set of content models carrying CreatedByUserId. Kept in
// one place so DeleteUser's referential cleanup stays in sync with the models
// that the stamp callback writes.
func stampedModels() []any {
	return []any{
		&models.Resource{},
		&models.Note{},
		&models.Group{},
		&models.Tag{},
		&models.Category{},
		&models.ResourceCategory{},
		&models.NoteType{},
		&models.Series{},
		&models.Query{},
		&models.SavedMRQLQuery{},
		&models.NoteBlock{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ResourceVersion{},
		&models.TemplatePartial{},
		// Not content, but it carries the same column and for the same reason: a
		// deleted user must not leave a dangling id behind on their download
		// history. The rows survive with a NULL creator, which the visibility
		// predicate then hides from every non-admin.
		&models.DownloadHistoryEntry{},
		// Likewise not content: the owner is the identity a deferred plugin
		// download is re-validated against before it is submitted. Nulling it on
		// user deletion makes the claim predicate fail, so the row stops rather
		// than firing as root or as a deleted account.
		&models.ScheduledDownload{},
		// Likewise not content, and here the sweep is load-bearing rather than
		// tidy: the owner of a schedule is the identity its Lua executes as, so
		// "the operator was deleted" has to resolve to "this stops" rather than
		// to a run binding an account that no longer exists. Nulling the column
		// does exactly that, because a row with no owner is never claimed.
		&models.PluginSchedule{},
		// A Reduction is a pending proposal to delete files, and its visibility is
		// owner-restricted. Nulling the column on user deletion is what makes it
		// nobody's — which is the fail-closed arm of the visibility predicate, so
		// the sweep is load-bearing here rather than tidy.
		&models.ResourceReduction{},
	}
}

// nullCreatorReferences nulls created_by_user_id on every stamped content table
// for the given user, so deleting the user leaves their content intact with a
// NULL creator rather than a dangling id. Runs inside the DeleteUser transaction.
// Correct on SQLite + Postgres (both accept UPDATE ... SET col = NULL).
func nullCreatorReferences(tx *gorm.DB, userID uint) error {
	for _, m := range stampedModels() {
		if err := tx.Model(m).
			Where("created_by_user_id = ?", userID).
			Update("created_by_user_id", nil).Error; err != nil {
			return err
		}
	}
	return nil
}

// lockEnabledAdmins locks the enabled-admin row set (Postgres FOR UPDATE) so
// concurrent last-admin mutations serialize: under read-committed two txns could
// otherwise each observe two enabled admins and each remove a different one down
// to zero. A no-op on SQLite, which serializes writers within a write
// transaction (and where the conditional mutation below is the first write).
func lockEnabledAdmins(ctx *MahresourcesContext, tx *gorm.DB) error {
	if ctx.Config.DbType != constants.DbTypePosgres {
		return nil
	}
	var ids []uint
	return tx.Model(&models.User{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("role = ? AND disabled = ?", models.RoleAdmin, false).
		Order("id").
		Pluck("id", &ids).Error
}
