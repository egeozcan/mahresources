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
