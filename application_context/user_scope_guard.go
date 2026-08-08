package application_context

import (
	"errors"

	"mahresources/models"

	"gorm.io/gorm"
)

// ErrGroupIsUserScope is returned when deleting a group would remove the root
// of one or more users' authorization scope.
var ErrGroupIsUserScope = errors.New("group is assigned as a user scope")

// rejectGroupDeletionIfUserScope must run on the caller's transaction before
// any destructive write involving groupID.
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
