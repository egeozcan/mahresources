package application_context

import (
	"errors"
	"strings"
	"time"

	"mahresources/auth"
	"mahresources/models"

	"gorm.io/gorm"
)

// User-management and authentication errors.
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUsernameRequired   = errors.New("username is required")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrPasswordRequired   = errors.New("password is required")
	ErrInvalidRole        = errors.New("invalid role")
	ErrScopeGroupRequired = errors.New("this role must be limited to a group")
	ErrScopeGroupMissing  = errors.New("scope group does not exist")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserDisabled       = errors.New("user account is disabled")
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
	if err := ctx.validateScopeGroup(input.Role, input.ScopeGroupId); err != nil {
		return nil, err
	}
	if input.Password == "" {
		return nil, ErrPasswordRequired
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
	if err := ctx.db.Create(user).Error; err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}
	return user, nil
}

// UpdateUser updates an existing user's mutable fields. A blank Password leaves
// the existing password unchanged.
func (ctx *MahresourcesContext) UpdateUser(id uint, input *UserInput) (*models.User, error) {
	user, err := ctx.GetUser(id)
	if err != nil {
		return nil, err
	}
	username := strings.TrimSpace(input.Username)
	if username == "" {
		return nil, ErrUsernameRequired
	}
	if !input.Role.IsValid() {
		return nil, ErrInvalidRole
	}
	if err := ctx.validateScopeGroup(input.Role, input.ScopeGroupId); err != nil {
		return nil, err
	}

	user.Username = username
	user.DisplayName = strings.TrimSpace(input.DisplayName)
	user.Role = input.Role
	user.ScopeGroupId = normalizeScopeGroup(input.Role, input.ScopeGroupId)
	user.Disabled = input.Disabled

	if input.Password != "" {
		hash, err := auth.HashPassword(input.Password)
		if err != nil {
			return nil, err
		}
		user.PasswordHash = hash
	}

	if err := ctx.db.Save(user).Error; err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}
	return user, nil
}

// SetUserPassword replaces a user's password.
func (ctx *MahresourcesContext) SetUserPassword(id uint, newPassword string) error {
	if newPassword == "" {
		return ErrPasswordRequired
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	res := ctx.db.Model(&models.User{}).Where("id = ?", id).Update("password_hash", hash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
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

// DeleteUser removes a user along with their sessions and API tokens. The
// dependent rows are deleted explicitly so removal works regardless of whether
// the database enforces ON DELETE CASCADE (SQLite leaves FK enforcement off by
// default).
func (ctx *MahresourcesContext) DeleteUser(id uint) error {
	if err := ctx.RevokeUserSessions(id); err != nil {
		return err
	}
	if err := ctx.RevokeUserApiTokens(id); err != nil {
		return err
	}
	res := ctx.db.Delete(&models.User{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUserNotFound
	}
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
func (ctx *MahresourcesContext) EnsureAdminUser(username, password string) (*models.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, ErrUsernameRequired
	}
	if password == "" {
		return nil, ErrPasswordRequired
	}

	existing, err := ctx.GetUserByUsername(username)
	if err != nil && !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}
	if existing != nil {
		hash, hErr := auth.HashPassword(password)
		if hErr != nil {
			return nil, hErr
		}
		existing.PasswordHash = hash
		existing.Role = models.RoleAdmin
		existing.Disabled = false
		existing.ScopeGroupId = nil
		if sErr := ctx.db.Save(existing).Error; sErr != nil {
			return nil, sErr
		}
		return existing, nil
	}

	return ctx.CreateUser(&UserInput{
		Username: username,
		Password: password,
		Role:     models.RoleAdmin,
	})
}

// TouchUserLogin records the time of a successful login.
func (ctx *MahresourcesContext) TouchUserLogin(id uint) {
	now := time.Now()
	ctx.db.Model(&models.User{}).Where("id = ?", id).Update("last_login_at", now)
}
