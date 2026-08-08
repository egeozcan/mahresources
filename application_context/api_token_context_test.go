package application_context

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/models"

	"github.com/spf13/afero"
	"gorm.io/gorm"
)

func TestCreateAndValidateApiToken(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "tok", Password: "password1", Role: models.RoleEditor})

	raw, token, err := ctx.CreateApiToken(u.ID, "ci", nil)
	if err != nil {
		t.Fatalf("CreateApiToken: %v", err)
	}
	if raw == "" || token.TokenHash == raw {
		t.Error("raw token must be returned and differ from stored hash")
	}
	if token.Prefix == "" {
		t.Error("token prefix should be set for display")
	}

	got, _, err := ctx.ValidateApiToken(raw)
	if err != nil {
		t.Fatalf("ValidateApiToken: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("validated wrong user: %d", got.ID)
	}

	if _, _, err := ctx.ValidateApiToken("nope"); !errors.Is(err, ErrApiTokenInvalid) {
		t.Errorf("bogus token: got %v", err)
	}
}

func TestApiTokenDisabledAndExpired(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "te", Password: "password1", Role: models.RoleEditor})

	// Disabled token.
	rawDisabled, tokDisabled, _ := ctx.CreateApiToken(u.ID, "d", nil)
	ctx.db.Model(&models.ApiToken{}).Where("id = ?", tokDisabled.ID).Update("disabled", true)
	if _, _, err := ctx.ValidateApiToken(rawDisabled); !errors.Is(err, ErrApiTokenInvalid) {
		t.Errorf("disabled token: got %v", err)
	}

	// Expired token.
	past := time.Now().Add(-time.Hour)
	rawExpired, _, _ := ctx.CreateApiToken(u.ID, "e", &past)
	if _, _, err := ctx.ValidateApiToken(rawExpired); !errors.Is(err, ErrApiTokenInvalid) {
		t.Errorf("expired token: got %v", err)
	}

	// Disabled user invalidates an otherwise-valid token.
	rawOK, _, _ := ctx.CreateApiToken(u.ID, "ok", nil)
	u.Disabled = true
	ctx.db.Save(u)
	if _, _, err := ctx.ValidateApiToken(rawOK); !errors.Is(err, ErrUserDisabled) {
		t.Errorf("disabled user token: got %v", err)
	}
}

func TestRevokeApiTokenScopedToOwner(t *testing.T) {
	ctx := newAuthTestContext(t)
	owner, _ := ctx.CreateUser(&UserInput{Username: "owner", Password: "password1", Role: models.RoleEditor})
	other, _ := ctx.CreateUser(&UserInput{Username: "other", Password: "password1", Role: models.RoleEditor})

	raw, token, _ := ctx.CreateApiToken(owner.ID, "k", nil)

	// Another user cannot revoke this token by ID.
	if err := ctx.RevokeApiToken(token.ID, other.ID); !errors.Is(err, ErrApiTokenNotFound) {
		t.Errorf("cross-user revoke should fail, got %v", err)
	}
	if _, _, err := ctx.ValidateApiToken(raw); err != nil {
		t.Errorf("token should still be valid after failed cross-user revoke: %v", err)
	}

	// Owner can revoke.
	if err := ctx.RevokeApiToken(token.ID, owner.ID); err != nil {
		t.Fatalf("owner revoke: %v", err)
	}
	if _, _, err := ctx.ValidateApiToken(raw); !errors.Is(err, ErrApiTokenInvalid) {
		t.Errorf("revoked token should be invalid, got %v", err)
	}

	if tokens, _ := ctx.ListApiTokens(owner.ID); len(tokens) != 0 {
		t.Errorf("owner should have no tokens left, got %d", len(tokens))
	}
}

func TestCreateApiTokenPerUserCap(t *testing.T) {
	ctx := newAuthTestContext(t)
	ctx.Config.MaxUserTokens = 2
	u, _ := ctx.CreateUser(&UserInput{Username: "capped", Password: "password1", Role: models.RoleEditor})
	other, _ := ctx.CreateUser(&UserInput{Username: "uncapped-peer", Password: "password1", Role: models.RoleEditor})

	if _, _, err := ctx.CreateApiToken(u.ID, "a", nil); err != nil {
		t.Fatalf("first token: %v", err)
	}
	if _, _, err := ctx.CreateApiToken(u.ID, "b", nil); err != nil {
		t.Fatalf("second token: %v", err)
	}
	// Third token exceeds the cap.
	if _, _, err := ctx.CreateApiToken(u.ID, "c", nil); !errors.Is(err, ErrApiTokenLimitReached) {
		t.Fatalf("third token should hit the cap, got %v", err)
	}
	// The cap is per-user: a different user is unaffected.
	if _, _, err := ctx.CreateApiToken(other.ID, "a", nil); err != nil {
		t.Fatalf("peer's token should not be blocked by another user's cap: %v", err)
	}
	// Revoking frees a slot.
	tokens, _ := ctx.ListApiTokens(u.ID)
	if err := ctx.RevokeApiToken(tokens[0].ID, u.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, err := ctx.CreateApiToken(u.ID, "c", nil); err != nil {
		t.Fatalf("token after freeing a slot should succeed: %v", err)
	}
}

func TestCreateApiTokenUnlimitedByDefault(t *testing.T) {
	ctx := newAuthTestContext(t) // MaxUserTokens defaults to 0 = unlimited
	u, _ := ctx.CreateUser(&UserInput{Username: "many", Password: "password1", Role: models.RoleEditor})
	for i := 0; i < 25; i++ {
		if _, _, err := ctx.CreateApiToken(u.ID, "k", nil); err != nil {
			t.Fatalf("token %d should succeed with no cap: %v", i, err)
		}
	}
}

func TestUnlimitedApiTokenCreationSerializesPasswordReset(t *testing.T) {
	contexts := newConcurrentApiTokenTestContexts(t, 2)
	for _, testCtx := range contexts {
		testCtx.Config.MaxUserTokens = 0
	}
	user, err := contexts[0].CreateUser(&UserInput{Username: "unlimited-race", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}

	insertReady := make(chan struct{})
	releaseInsert := make(chan struct{})
	var insertOnce sync.Once
	if err := contexts[0].db.Callback().Create().Before("gorm:create").Register("test:pause_unlimited_token_insert", func(tx *gorm.DB) {
		if tx.Statement.Table == "api_tokens" {
			insertOnce.Do(func() {
				close(insertReady)
				<-releaseInsert
			})
		}
	}); err != nil {
		t.Fatalf("register token insert barrier: %v", err)
	}

	resetAttempted := make(chan struct{})
	var resetOnce sync.Once
	if err := contexts[1].db.Callback().Raw().Before("gorm:raw").Register("test:observe_unlimited_reset_lock", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "UPDATE users SET id = id") {
			resetOnce.Do(func() { close(resetAttempted) })
		}
	}); err != nil {
		t.Fatalf("register reset barrier: %v", err)
	}

	type tokenResult struct {
		raw string
		err error
	}
	tokenDone := make(chan tokenResult, 1)
	go func() {
		raw, _, err := contexts[0].CreateApiToken(user.ID, "racing token", nil)
		tokenDone <- tokenResult{raw: raw, err: err}
	}()
	<-insertReady

	resetDone := make(chan error, 1)
	go func() { resetDone <- contexts[1].SetUserPassword(user.ID, "password2") }()
	<-resetAttempted
	close(releaseInsert)

	created := <-tokenDone
	if created.err != nil {
		t.Fatalf("create token: %v", created.err)
	}
	if err := <-resetDone; err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if _, _, err := contexts[0].ValidateApiToken(created.raw); !errors.Is(err, ErrApiTokenInvalid) {
		t.Fatalf("token created by an operation racing reset remained valid: %v", err)
	}
}

func TestCreateApiTokenRejectsMissingOwner(t *testing.T) {
	for _, limit := range []int{0, 2} {
		t.Run(fmt.Sprintf("limit-%d", limit), func(t *testing.T) {
			ctx := newAuthTestContext(t)
			ctx.Config.MaxUserTokens = limit
			if raw, token, err := ctx.CreateApiToken(999999, "orphan", nil); !errors.Is(err, ErrUserNotFound) {
				t.Fatalf("CreateApiToken missing owner: raw=%q token=%v err=%v, want ErrUserNotFound", raw, token, err)
			}
		})
	}
}

// newConcurrentApiTokenTestContexts opens separate production-configured SQLite
// pools against one on-disk database. Using the production connection factory
// matters here: every connection gets WAL mode and the busy timeout used by a
// real deployment, so a raw "database is locked" error is a regression rather
// than an artifact of the test driver.
func newConcurrentApiTokenTestContexts(t *testing.T, count int) []*MahresourcesContext {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "api-token-cap.db")
	contexts := make([]*MahresourcesContext, 0, count)
	for i := 0; i < count; i++ {
		db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, dsn, "", 0)
		if err != nil {
			t.Fatalf("open shared SQLite database %d: %v", i, err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("get shared SQLite pool %d: %v", i, err)
		}
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		if i == 0 {
			if err := db.AutoMigrate(&models.User{}, &models.Session{}, &models.ApiToken{}); err != nil {
				t.Fatalf("migrate shared SQLite database: %v", err)
			}
		}
		contexts = append(contexts, NewMahresourcesContext(
			afero.NewMemMapFs(), db, nil,
			&MahresourcesConfig{DbType: constants.DbTypeSqlite, MaxUserTokens: 2},
		))
	}
	return contexts
}

func TestApiTokenConcurrentCapSQLite(t *testing.T) {
	const creators = 12
	contexts := newConcurrentApiTokenTestContexts(t, creators)
	u, err := contexts[0].CreateUser(&UserInput{
		Username: "concurrently-capped", Password: "password1", Role: models.RoleEditor,
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if _, _, err := contexts[0].CreateApiToken(u.ID, "existing", nil); err != nil {
		t.Fatalf("create existing token: %v", err)
	}

	// Coordinate the original read-before-write implementation so this test is a
	// deterministic regression rather than a scheduler lottery. A correct capped
	// creation performs its count inside the locking transaction and deliberately
	// bypasses this test-only barrier.
	var nonTransactionalCounts atomic.Int32
	allNonTransactionalCountsDone := make(chan struct{})
	for i, creatorCtx := range contexts {
		callbackName := fmt.Sprintf("test:api_token_cap_count_barrier:%d", i)
		if err := creatorCtx.db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Table != "api_tokens" || tx.Error != nil {
				return
			}
			if _, inTransaction := tx.Statement.ConnPool.(*sql.Tx); inTransaction {
				return
			}
			if nonTransactionalCounts.Add(1) == creators {
				close(allNonTransactionalCountsDone)
			}
			<-allNonTransactionalCountsDone
		}); err != nil {
			t.Fatalf("register count barrier %d: %v", i, err)
		}
	}

	// Prove the fixed path itself holds the owner write lock before counting.
	// Competitors must reach the transaction's early serialization write while
	// that owner lock is held; deleting lockApiTokenOwner makes this barrier fail.
	ownerHeld := make(chan struct{})
	competitorAttempt := make(chan struct{})
	releaseOwner := make(chan struct{})
	var ownerObserved atomic.Bool
	var mutationAttempts atomic.Int32
	for i, creatorCtx := range contexts {
		callbackName := fmt.Sprintf("test:api_token_owner_lock_barrier:%d", i)
		if err := creatorCtx.db.Callback().Raw().Before("gorm:raw").Register(callbackName+":attempt", func(tx *gorm.DB) {
			if strings.Contains(tx.Statement.SQL.String(), "UPDATE users SET id = id") && mutationAttempts.Add(1) == 2 {
				close(competitorAttempt)
			}
		}); err != nil {
			t.Fatalf("register mutation attempt barrier %d: %v", i, err)
		}
		if err := creatorCtx.db.Callback().Raw().After("gorm:raw").Register(callbackName+":held", func(tx *gorm.DB) {
			sqlText := tx.Statement.SQL.String()
			if tx.Error == nil && strings.Contains(sqlText, "UPDATE users SET id = id WHERE id = ?") && ownerObserved.CompareAndSwap(false, true) {
				close(ownerHeld)
				<-releaseOwner
			}
		}); err != nil {
			t.Fatalf("register owner-held barrier %d: %v", i, err)
		}
	}

	start := make(chan struct{})
	results := make(chan error, creators)
	for i := 0; i < creators; i++ {
		go func(i int) {
			<-start
			raw, token, err := contexts[i].CreateApiToken(u.ID, fmt.Sprintf("creator-%d", i), nil)
			if err == nil && (raw == "" || token == nil || token.TokenHash == raw) {
				err = fmt.Errorf("successful creation violated one-time raw-token semantics")
			}
			results <- err
		}(i)
	}
	close(start)
	select {
	case <-ownerHeld:
	case <-time.After(3 * time.Second):
		t.Fatal("transactional creator never held the SQLite owner lock")
	}
	select {
	case <-competitorAttempt:
	case <-time.After(3 * time.Second):
		close(releaseOwner)
		t.Fatal("competitors never attempted capped creation while the owner lock was held")
	}
	select {
	case err := <-results:
		close(releaseOwner)
		t.Fatalf("creator completed before owner-lock release: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseOwner)

	successes, capped := 0, 0
	for i := 0; i < creators; i++ {
		switch err := <-results; {
		case err == nil:
			successes++
		case errors.Is(err, ErrApiTokenLimitReached):
			capped++
		default:
			t.Fatalf("creator returned unexpected error: %v", err)
		}
	}
	if successes != 1 || capped != creators-1 {
		t.Fatalf("want one success and %d cap errors, got successes=%d capped=%d", creators-1, successes, capped)
	}
	for i, creatorCtx := range contexts {
		callbackName := fmt.Sprintf("test:api_token_cap_count_barrier:%d", i)
		if err := creatorCtx.db.Callback().Query().Remove(callbackName); err != nil {
			t.Fatalf("remove count barrier %d: %v", i, err)
		}
		ownerCallback := fmt.Sprintf("test:api_token_owner_lock_barrier:%d", i)
		_ = creatorCtx.db.Callback().Raw().Remove(ownerCallback + ":attempt")
		_ = creatorCtx.db.Callback().Raw().Remove(ownerCallback + ":held")
	}

	var persisted int64
	if err := contexts[0].db.Model(&models.ApiToken{}).Where("user_id = ?", u.ID).Count(&persisted).Error; err != nil {
		t.Fatalf("count persisted tokens: %v", err)
	}
	if persisted != 2 {
		t.Fatalf("persisted token count = %d, want 2", persisted)
	}
}
