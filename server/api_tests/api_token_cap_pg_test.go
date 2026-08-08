//go:build postgres

package api_tests

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"

	"github.com/spf13/afero"
	"gorm.io/gorm"
)

func TestApiTokenCapPostgresConcurrency(t *testing.T) {
	const creators = 12
	tc := SetupPostgresTestEnv(t)
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "pg-concurrently-capped", Password: "password1", Role: models.RoleEditor,
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}

	// Use distinct application contexts, as independent server processes would,
	// while sharing the test database and its production-style connection pool.
	contexts := make([]*application_context.MahresourcesContext, creators)
	for i := range contexts {
		contexts[i] = application_context.NewMahresourcesContext(
			afero.NewMemMapFs(), tc.DB.Session(&gorm.Session{NewDB: true}), nil,
			&application_context.MahresourcesConfig{
				DbType: constants.DbTypePosgres, MaxUserTokens: 2,
			},
		)
	}
	if _, _, err := contexts[0].CreateApiToken(u.ID, "existing", nil); err != nil {
		t.Fatalf("create existing token: %v", err)
	}

	// Make the pre-fix read-before-write race deterministic. The fixed path
	// counts inside the owner-row locking transaction and bypasses this barrier.
	var nonTransactionalCounts atomic.Int32
	allNonTransactionalCountsDone := make(chan struct{})
	if err := tc.DB.Callback().Query().After("gorm:query").Register("test:api_token_cap_count_barrier", func(tx *gorm.DB) {
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
		t.Fatalf("register count barrier: %v", err)
	}

	ownerHeld := make(chan struct{})
	competitorAttempt := make(chan struct{})
	releaseOwner := make(chan struct{})
	var ownerObserved atomic.Bool
	var advisoryAttempts atomic.Int32
	if err := tc.DB.Callback().Raw().Before("gorm:raw").Register("test:api_token_advisory_competitor", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "pg_advisory_xact_lock") && advisoryAttempts.Add(1) == 2 {
			close(competitorAttempt)
		}
	}); err != nil {
		t.Fatalf("register advisory attempt barrier: %v", err)
	}
	if err := tc.DB.Callback().Query().After("gorm:query").Register("test:api_token_owner_row_held", func(tx *gorm.DB) {
		if tx.Error != nil || tx.Statement.Table != "users" {
			return
		}
		if _, locking := tx.Statement.Clauses["FOR"]; locking && ownerObserved.CompareAndSwap(false, true) {
			close(ownerHeld)
			<-releaseOwner
		}
	}); err != nil {
		t.Fatalf("register owner-row barrier: %v", err)
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
	case <-time.After(5 * time.Second):
		t.Fatal("transactional creator never held the PostgreSQL owner FOR UPDATE lock")
	}
	select {
	case <-competitorAttempt:
	case <-time.After(5 * time.Second):
		close(releaseOwner)
		t.Fatal("competitors never attempted capped creation while the owner lock was held")
	}
	select {
	case err := <-results:
		close(releaseOwner)
		t.Fatalf("creator completed before owner-row release: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseOwner)

	successes, capped := 0, 0
	for i := 0; i < creators; i++ {
		switch err := <-results; {
		case err == nil:
			successes++
		case errors.Is(err, application_context.ErrApiTokenLimitReached):
			capped++
		default:
			t.Fatalf("creator returned unexpected error: %v", err)
		}
	}
	if successes != 1 || capped != creators-1 {
		t.Fatalf("want one success and %d cap errors, got successes=%d capped=%d", creators-1, successes, capped)
	}
	if err := tc.DB.Callback().Query().Remove("test:api_token_cap_count_barrier"); err != nil {
		t.Fatalf("remove count barrier: %v", err)
	}
	_ = tc.DB.Callback().Raw().Remove("test:api_token_advisory_competitor")
	_ = tc.DB.Callback().Query().Remove("test:api_token_owner_row_held")

	var persisted int64
	if err := tc.DB.Model(&models.ApiToken{}).Where("user_id = ?", u.ID).Count(&persisted).Error; err != nil {
		t.Fatalf("count persisted tokens: %v", err)
	}
	if persisted != 2 {
		t.Fatalf("persisted token count = %d, want 2", persisted)
	}
}
