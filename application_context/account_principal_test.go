package application_context

import (
	"fmt"
	"sync/atomic"
	"testing"

	"mahresources/auth"
	"mahresources/models"
)

var accountPrincipalSeq atomic.Uint64

// accountPrincipal stores a user account with this role and scope group and
// returns the principal a request by that account carries. A containment
// write re-reads the account (accountStillBound) and refuses a principal whose
// account is missing or no longer matches, so a scoped principal that writes
// must be backed by a stored account. tb may be nil for a helper that has no
// test handle; a failure then panics.
func accountPrincipal(tb testing.TB, ctx *MahresourcesContext, name string, role models.Role, scope *uint) *auth.Principal {
	if tb != nil {
		tb.Helper()
	}
	u := &models.User{
		Username:     fmt.Sprintf("%s-%d", name, accountPrincipalSeq.Add(1)),
		Role:         role,
		ScopeGroupId: scope,
	}
	// Some focused fixtures migrate no auth tables. Sessions and tokens are
	// there so an account change (which revokes them) works on the account.
	err := ctx.db.AutoMigrate(&models.User{}, &models.Session{}, &models.ApiToken{})
	if err == nil {
		err = ctx.db.Create(u).Error
	}
	if err != nil {
		if tb == nil {
			panic(fmt.Sprintf("store account %s: %v", u.Username, err))
		}
		tb.Fatalf("store account %s: %v", u.Username, err)
	}
	return auth.FromUser(u)
}
