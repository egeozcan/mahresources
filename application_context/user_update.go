package application_context

import (
	"errors"
	"strings"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserField distinguishes an omitted update property from a property explicitly
// set to its zero value.
type UserField[T any] struct {
	Set   bool
	Value T
}

// UserUpdate is the presence-aware mutation payload for an existing user.
type UserUpdate struct {
	Username     UserField[string]
	DisplayName  UserField[string]
	Password     UserField[string]
	Role         UserField[models.Role]
	ScopeGroupID UserField[*uint]
	Disabled     UserField[bool]
}

// FullUserUpdate converts the historical full-body input into a presence-aware
// update. It keeps Go callers that intentionally replace every mutable identity
// field explicit while HTTP partial requests can mark only supplied properties.
func FullUserUpdate(input *UserInput) *UserUpdate {
	return &UserUpdate{
		Username:     UserField[string]{Set: true, Value: input.Username},
		DisplayName:  UserField[string]{Set: true, Value: input.DisplayName},
		Password:     UserField[string]{Set: input.Password != "", Value: input.Password},
		Role:         UserField[models.Role]{Set: true, Value: input.Role},
		ScopeGroupID: UserField[*uint]{Set: true, Value: input.ScopeGroupId},
		Disabled:     UserField[bool]{Set: true, Value: input.Disabled},
	}
}

// UpdateUser updates only fields marked present. Identity mutation, last-admin
// enforcement, password replacement, and credential revocation commit or roll
// back together.
func (ctx *MahresourcesContext) UpdateUser(id uint, update *UserUpdate) (*models.User, error) {
	if update == nil {
		update = &UserUpdate{}
	}

	updates := make(map[string]any)
	if update.Username.Set {
		username := strings.TrimSpace(update.Username.Value)
		if username == "" {
			return nil, ErrUsernameRequired
		}
		updates["username"] = username
	}
	if update.DisplayName.Set {
		updates["display_name"] = strings.TrimSpace(update.DisplayName.Value)
	}
	if update.Role.Set {
		if !update.Role.Value.IsValid() {
			return nil, ErrInvalidRole
		}
		updates["role"] = update.Role.Value
	}

	passwordChanged := update.Password.Set && update.Password.Value != ""
	if passwordChanged {
		if err := auth.ValidatePassword(update.Password.Value); err != nil {
			return nil, err
		}
		hash, err := auth.HashPassword(update.Password.Value)
		if err != nil {
			return nil, err
		}
		updates["password_hash"] = hash
		updates["password_auto_generated"] = false
	}
	if update.Disabled.Set {
		updates["disabled"] = update.Disabled.Value
	}

	// Updates that cannot affect role, scope, or enabled-admin membership do not
	// need a read-before-write classification. Updating their explicit columns
	// directly is what prevents a stale display-name/username write from restoring
	// a concurrently replaced password hash.
	if !update.Role.Set && !update.ScopeGroupID.Set && !update.Disabled.Set {
		if len(updates) == 0 {
			return ctx.GetUser(id)
		}
		err := ctx.db.Transaction(func(tx *gorm.DB) error {
			if err := ctx.lockUserManagementMutation(tx); err != nil {
				return err
			}
			res := tx.Model(&models.User{}).Where("id = ?", id).Updates(updates)
			if res.Error != nil {
				if isUniqueConstraintError(res.Error) {
					return ErrUsernameTaken
				}
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrUserNotFound
			}
			if passwordChanged {
				if err := deleteUserSessions(tx, id, nil); err != nil {
					return err
				}
				if err := deleteUserAPITokens(tx, id); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		result, err := ctx.GetUser(id)
		if err != nil {
			return nil, err
		}
		ctx.refreshRootAdmin()
		return result, nil
	}

	// A pre-read is required to know which scope group (if any) must be the
	// transaction's first lock. The row is deliberately re-read under lock below;
	// it is only a lock-order hint and never the authority for last-admin checks.
	// If its role/scope changes before the user lock, retry from a fresh hint.
	for attempt := 0; attempt < 5; attempt++ {
		observed, err := ctx.GetUser(id)
		if err != nil {
			return nil, err
		}
		_, observedScope := effectiveUserScope(observed, update)

		var result models.User
		err = ctx.db.Transaction(func(tx *gorm.DB) error {
			if err := ctx.lockUserManagementMutation(tx); err != nil {
				return err
			}
			// Scope-bearing identity changes lock/revalidate the target group before
			// any user row. MergeGroups uses the same groups-before-users order.
			if observedScope != nil {
				if _, lockErr := ctx.lockScopeGroup(tx, *observedScope, "assign"); lockErr != nil {
					if errors.Is(lockErr, gorm.ErrRecordNotFound) {
						return ErrScopeGroupMissing
					}
					return lockErr
				}
			}

			// SQLite has no row-level FOR UPDATE. This no-op user write follows
			// the optional group no-op write, preserving the same lock order while
			// serializing identity writers before authoritative classification.
			if ctx.Config.DbType == constants.DbTypeSqlite {
				res := tx.Exec(sqliteUserRowLockStatement, id)
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected == 0 {
					return ErrUserNotFound
				}
			}
			if err := lockEnabledAdmins(ctx, tx); err != nil {
				return err
			}

			var current models.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrUserNotFound
				}
				return err
			}

			effectiveRole, effectiveScope := effectiveUserScope(&current, update)
			if !sameScopeGroup(observedScope, effectiveScope) {
				return errIdentityReclassified
			}
			if effectiveScope == nil && effectiveRole.RequiresScopeGroup() {
				return ErrScopeGroupRequired
			}
			if update.Role.Set || update.ScopeGroupID.Set {
				updates["scope_group_id"] = effectiveScope
			}

			effectiveDisabled := current.Disabled
			if update.Disabled.Set {
				effectiveDisabled = update.Disabled.Value
			}
			dangerous := current.Role == models.RoleAdmin && !current.Disabled &&
				(effectiveRole != models.RoleAdmin || effectiveDisabled)

			if len(updates) > 0 {
				query := tx.Model(&models.User{}).Where("id = ?", id)
				if dangerous {
					query = query.Where(
						"EXISTS (SELECT 1 FROM users u2 WHERE u2.role = ? AND u2.disabled = ? AND u2.id <> ?)",
						models.RoleAdmin, false, id,
					)
				}
				res := query.Updates(updates)
				if res.Error != nil {
					if isUniqueConstraintError(res.Error) {
						return ErrUsernameTaken
					}
					return res.Error
				}
				if dangerous && res.RowsAffected == 0 {
					return ErrLastAdmin
				}
			}

			// An administrator-set password reset and a disabled account must not
			// retain credentials minted under the prior identity state.
			if passwordChanged || (update.Disabled.Set && update.Disabled.Value) {
				if err := deleteUserSessions(tx, id, nil); err != nil {
					return err
				}
				if err := deleteUserAPITokens(tx, id); err != nil {
					return err
				}
			}

			return tx.First(&result, id).Error
		})
		if errors.Is(err, errIdentityReclassified) {
			continue
		}
		if err != nil {
			return nil, err
		}
		ctx.refreshRootAdmin()
		return &result, nil
	}
	return nil, errIdentityReclassified
}

func effectiveUserScope(current *models.User, update *UserUpdate) (models.Role, *uint) {
	role := current.Role
	if update.Role.Set {
		role = update.Role.Value
	}
	scope := current.ScopeGroupId
	if update.ScopeGroupID.Set {
		scope = update.ScopeGroupID.Value
	}
	return role, normalizeScopeGroup(role, scope)
}

func sameScopeGroup(left, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
