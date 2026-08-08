//go:build postgres

package api_tests

import (
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

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

	var persisted int64
	if err := tc.DB.Model(&models.ApiToken{}).Where("user_id = ?", u.ID).Count(&persisted).Error; err != nil {
		t.Fatalf("count persisted tokens: %v", err)
	}
	if persisted != 2 {
		t.Fatalf("persisted token count = %d, want 2", persisted)
	}
}
