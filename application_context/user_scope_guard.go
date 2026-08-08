package application_context

import (
	"errors"

	"mahresources/constants"
	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrGroupIsUserScope is returned when deleting a group would remove the root
// of one or more users' authorization scope.
var ErrGroupIsUserScope = errors.New("group is assigned as a user scope")

// lockScopeGroup is the shared serialization protocol for deleting a group and
// assigning it as a user scope. SQLite's no-op UPDATE is deliberately the first
// transaction statement, acquiring its writer lock before validation. Postgres
// uses a row lock on the same groups row on both sides.
func (ctx *MahresourcesContext) lockScopeGroup(db *gorm.DB, groupID uint, operation string) (*models.Group, error) {
	var group models.Group
	if ctx.Config.DbType == constants.DbTypeSqlite {
		res := db.Exec("UPDATE groups SET id = id WHERE id = ?", groupID)
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 0 {
			return nil, gorm.ErrRecordNotFound
		}
		if err := db.First(&group, groupID).Error; err != nil {
			return nil, err
		}
	} else {
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&group, groupID).Error; err != nil {
			return nil, err
		}
	}
	if ctx.scopeLockBarrier != nil {
		ctx.scopeLockBarrier(operation, groupID)
	}
	return &group, nil
}

// rejectGroupDeletionIfUserScope must run on the caller's transaction after
// lockScopeGroup has locked the target row.
func rejectGroupDeletionIfUserScope(db *gorm.DB, groupID uint) error {
	// Some focused test and migration contexts intentionally predate the auth
	// tables. With no users table there cannot be a scope reference to preserve.
	if !db.Migrator().HasTable(&models.User{}) {
		return nil
	}

	var count int64
	if err := db.Model(&models.User{}).
		Where("scope_group_id = ?", groupID).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrGroupIsUserScope
	}
	return nil
}

// validateAndLockScopeGroup applies role rules and locks a supplied group until
// the caller's user create/update transaction commits.
func (ctx *MahresourcesContext) validateAndLockScopeGroup(db *gorm.DB, role models.Role, scope *uint) error {
	scope = normalizeScopeGroup(role, scope)
	if scope == nil {
		if role.RequiresScopeGroup() {
			return ErrScopeGroupRequired
		}
		return nil
	}
	if _, err := ctx.lockScopeGroup(db, *scope, "assign"); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrScopeGroupMissing
		}
		return err
	}
	return nil
}

// transferUserScopeReferences repoints every user scoped to loserID. The caller
// is responsible for passing its active transaction so the transfer rolls back
// together with the merge.
func transferUserScopeReferences(db *gorm.DB, loserID, winnerID uint) error {
	if !db.Migrator().HasTable(&models.User{}) {
		return nil
	}
	return db.Model(&models.User{}).
		Where("scope_group_id = ?", loserID).
		UpdateColumn("scope_group_id", winnerID).Error
}
