package application_context

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// User-management and authentication errors.
var (
	ErrUserNotFound     = errors.New("user not found")
	ErrUsernameRequired = errors.New("username is required")
	ErrUsernameTaken    = errors.New("username already taken")
	ErrPasswordRequired = errors.New("password is required")
	// ErrPasswordTooShort / ErrPasswordTooLong alias the auth-package policy
	// errors so callers and userErrorStatus can match them via the
	// application_context namespace.
	ErrPasswordTooShort   = auth.ErrPasswordTooShort
	ErrPasswordTooLong    = auth.ErrPasswordTooLong
	ErrInvalidRole        = errors.New("invalid role")
	ErrScopeGroupRequired = errors.New("this role must be limited to a group")
	ErrScopeGroupMissing  = errors.New("scope group does not exist")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserDisabled       = errors.New("user account is disabled")
	// ErrLastAdmin is returned when a delete/demote/disable would remove the last
	// enabled admin. Mapped to HTTP 409 Conflict.
	ErrLastAdmin = errors.New("cannot remove the last enabled administrator")
)

// dummyHash is a valid bcrypt hash used to equalize timing when authenticating
// a non-existent username, mitigating user-enumeration via response time. It is
// the hash of a random throwaway string; it matches no real password.
const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// UserInput is the create/update payload for a user account.
type UserInput struct {
	Username     string
	DisplayName  string
	Password     string // optional on update; required on create
	Role         models.Role
	ScopeGroupId *uint
	Disabled     bool
}

// normalizeScopeGroup forces the scope group to nil for roles that are never
// scoped (admin/editor), and otherwise returns the requested scope unchanged.
func normalizeScopeGroup(role models.Role, scope *uint) *uint {
	if !role.AllowsScopeGroup() {
		return nil
	}
	return scope
}

// validateScopeGroup enforces role scoping rules and verifies the referenced
// group exists when one is supplied.
func (ctx *MahresourcesContext) validateScopeGroup(role models.Role, scope *uint) error {
	scope = normalizeScopeGroup(role, scope)
	if scope == nil {
		if role.RequiresScopeGroup() {
			return ErrScopeGroupRequired
		}
		return nil
	}
	var count int64
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", *scope).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrScopeGroupMissing
	}
	return nil
}

// CreateUser validates and persists a new user account.
func (ctx *MahresourcesContext) CreateUser(input *UserInput) (*models.User, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" {
		return nil, ErrUsernameRequired
	}
	if !input.Role.IsValid() {
		return nil, ErrInvalidRole
	}
	if input.Password == "" {
		return nil, ErrPasswordRequired
	}
	if err := auth.ValidatePassword(input.Password); err != nil {
		return nil, err
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username:     username,
		DisplayName:  strings.TrimSpace(input.DisplayName),
		PasswordHash: hash,
		Role:         input.Role,
		ScopeGroupId: normalizeScopeGroup(input.Role, input.ScopeGroupId),
		Disabled:     input.Disabled,
	}
	if err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.validateAndLockScopeGroup(tx, input.Role, user.ScopeGroupId); err != nil {
			return err
		}
		if err := tx.Create(user).Error; err != nil {
			if isUniqueConstraintError(err) {
				return ErrUsernameTaken
			}
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// Best-effort re-warm: a new admin may become (or shift) the root actor.
	ctx.refreshRootAdmin()
	return user, nil
}

// GetUser loads a single user by ID.
func (ctx *MahresourcesContext) GetUser(id uint) (*models.User, error) {
	var user models.User
	if err := ctx.db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByUsername loads a single user by username.
func (ctx *MahresourcesContext) GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	if err := ctx.db.Where("username = ?", strings.TrimSpace(username)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUsers lists users ordered by username.
func (ctx *MahresourcesContext) GetUsers(offset, limit int) ([]models.User, error) {
	var users []models.User
	q := ctx.db.Order("username asc").Offset(offset)
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// CountUsers returns the total number of user accounts. Used to detect whether
// an instance has been bootstrapped.
func (ctx *MahresourcesContext) CountUsers() (int64, error) {
	var count int64
	if err := ctx.db.Model(&models.User{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountEnabledAdmins returns the number of enabled admin accounts. The
// last-admin guard and startup auto-create both key off this.
func (ctx *MahresourcesContext) CountEnabledAdmins() (int64, error) {
	var count int64
	if err := ctx.db.Model(&models.User{}).
		Where("role = ? AND disabled = ?", models.RoleAdmin, false).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// RootAdmin returns the "root" user: the oldest enabled admin, ordered by
// created_at then id (a deterministic tiebreak for rows created in the same
// instant). Returns gorm.ErrRecordNotFound when no enabled admin exists.
func (ctx *MahresourcesContext) RootAdmin() (*models.User, error) {
	var user models.User
	if err := ctx.db.
		Where("role = ? AND disabled = ?", models.RoleAdmin, false).
		Order("created_at asc, id asc").
		First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// DeleteUser removes a user along with their sessions and API tokens, and nulls
// the creator on any content they stamped. Runs in a single transaction so the
// last-admin guard and the referential cleanup are atomic. The dependent rows
// are deleted explicitly so removal works regardless of whether the database
// enforces ON DELETE CASCADE (SQLite leaves FK enforcement off by default).
//
// Last-admin guard: the delete is a conditional statement that only removes the
// row when the target is not the last enabled admin, verified via RowsAffected.
// On Postgres the enabled-admin row set is additionally locked FOR UPDATE so two
// concurrent deletions of different admins serialize (read-committed would
// otherwise let both succeed down to zero admins). On SQLite the conditional
// DELETE is the first write, so writers serialize on it.
func (ctx *MahresourcesContext) DeleteUser(id uint) error {
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if lockErr := lockEnabledAdmins(ctx, tx); lockErr != nil {
			return lockErr
		}

		// Referential cleanup (also the first write → serializes SQLite writers).
		if cErr := nullCreatorReferences(tx, id); cErr != nil {
			return cErr
		}
		if sErr := tx.Where("user_id = ?", id).Delete(&models.Session{}).Error; sErr != nil {
			return sErr
		}
		if tErr := tx.Where("user_id = ?", id).Delete(&models.ApiToken{}).Error; tErr != nil {
			return tErr
		}
		// Per-user settings are owner-keyed (not creator-attributed), so drop them
		// outright rather than nulling — a settings row with no owner is meaningless.
		if uErr := tx.Where("user_id = ?", id).Delete(&models.UserSetting{}).Error; uErr != nil {
			return uErr
		}

		// Conditional delete: allowed unless the target is an enabled admin with no
		// other enabled admin remaining.
		res := tx.Exec(
			"DELETE FROM users WHERE id = ? AND (role <> ? OR disabled = ? OR EXISTS (SELECT 1 FROM users u2 WHERE u2.role = ? AND u2.disabled = ? AND u2.id <> ?))",
			id, models.RoleAdmin, true, models.RoleAdmin, false, id,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Distinguish "not found" from "last admin".
			var count int64
			if cErr := tx.Model(&models.User{}).Where("id = ?", id).Count(&count).Error; cErr != nil {
				return cErr
			}
			if count == 0 {
				return ErrUserNotFound
			}
			return ErrLastAdmin
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Re-warm after the commit so the resolver sees the post-delete state.
	ctx.refreshRootAdmin()
	return nil
}

// AuthenticateUser verifies a username/password pair and returns the user on
// success. It performs a constant-cost compare even for unknown usernames to
// avoid leaking account existence through timing.
func (ctx *MahresourcesContext) AuthenticateUser(username, password string) (*models.User, error) {
	user, err := ctx.GetUserByUsername(username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Equalize timing against the user-exists path.
			auth.CheckPassword(dummyHash, password)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if !auth.CheckPassword(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	if user.Disabled {
		return nil, ErrUserDisabled
	}
	return user, nil
}

// EnsureAdminUser creates an admin account with the given credentials, or, if
// the username already exists, resets its password and ensures it is an enabled
// admin. Idempotent; used for headless bootstrap from a flag/env.
var errIdentityReclassified = errors.New("identity changed while awaiting mutation lock")

func (ctx *MahresourcesContext) EnsureAdminUser(username, password string) (*models.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, ErrUsernameRequired
	}
	if password == "" {
		return nil, ErrPasswordRequired
	}
	if err := auth.ValidatePassword(password); err != nil {
		return nil, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 5; attempt++ {
		existing, getErr := ctx.GetUserByUsername(username)
		if getErr != nil && !errors.Is(getErr, ErrUserNotFound) {
			return nil, getErr
		}
		if existing == nil {
			created, createErr := ctx.CreateUser(&UserInput{Username: username, Password: password, Role: models.RoleAdmin})
			if errors.Is(createErr, ErrUsernameTaken) {
				continue
			}
			return created, createErr
		}
		if ctx.identityReuseBarrier != nil {
			ctx.identityReuseBarrier("ensure-admin", existing)
		}
		promoted, promoteErr := ctx.promoteExpectedAdmin(existing, username, hash)
		if errors.Is(promoteErr, errIdentityReclassified) {
			continue
		}
		if promoteErr != nil {
			return nil, promoteErr
		}
		ctx.refreshRootAdmin()
		return promoted, nil
	}
	return nil, fmt.Errorf("could not stabilize bootstrap identity for %q", username)
}

func (ctx *MahresourcesContext) promoteExpectedAdmin(expected *models.User, username, hash string) (*models.User, error) {
	var promoted models.User
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("id = ? AND username = ? AND role = ?", expected.ID, username, expected.Role)
		if ctx.Config.DbType == constants.DbTypeSqlite {
			res := tx.Exec("UPDATE users SET id = id WHERE id = ? AND username = ? AND role = ?", expected.ID, username, expected.Role)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return errIdentityReclassified
			}
		} else if err := query.Clauses(clause.Locking{Strength: "UPDATE"}).First(&promoted).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errIdentityReclassified
			}
			return err
		}
		res := tx.Model(&models.User{}).
			Where("id = ? AND username = ? AND role = ?", expected.ID, username, expected.Role).
			Updates(map[string]any{
				"password_hash": hash, "password_auto_generated": false,
				"role": models.RoleAdmin, "disabled": false, "scope_group_id": nil,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errIdentityReclassified
		}
		if err := deleteUserSessions(tx, expected.ID, nil); err != nil {
			return err
		}
		if err := deleteUserAPITokens(tx, expected.ID); err != nil {
			return err
		}
		if err := tx.First(&promoted, expected.ID).Error; err != nil {
			return err
		}
		if promoted.Username != username || promoted.Role != models.RoleAdmin || promoted.Disabled {
			return errIdentityReclassified
		}
		return nil
	})
	return &promoted, err
}

// TouchUserLogin records the time of a successful login.
func (ctx *MahresourcesContext) TouchUserLogin(id uint) {
	now := time.Now()
	ctx.db.Model(&models.User{}).Where("id = ?", id).Update("last_login_at", now)
}
