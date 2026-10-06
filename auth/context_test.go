package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/auth"
	"mahresources/models"
)

func TestDescribeContext_NoPrincipal(t *testing.T) {
	assert.Nil(t, auth.DescribeContext(context.Background()))
}

// Plugins receive this map as a Lua table and authorize on it, so the exact key
// set and value types are the contract: comparing the whole map catches a
// renamed key, an added key, a wrong type (e.g. a *uint leaking through for
// scopeGroupId) or a wrong isAdmin derivation.
func TestDescribeContext_Principal(t *testing.T) {
	scope := uint(42)
	cases := []struct {
		name string
		p    *auth.Principal
		want map[string]any
	}{
		{
			"user without scope",
			&auth.Principal{UserID: 1, Username: "u", Role: models.RoleUser},
			map[string]any{"userId": uint(1), "username": "u", "role": string(models.RoleUser), "isAdmin": false, "scopeGroupId": nil, "superUser": false},
		},
		{
			"scoped editor",
			&auth.Principal{UserID: 2, Username: "e", Role: models.RoleEditor, ScopeGroupID: &scope},
			map[string]any{"userId": uint(2), "username": "e", "role": string(models.RoleEditor), "isAdmin": false, "scopeGroupId": uint(42), "superUser": false},
		},
		{
			"admin",
			&auth.Principal{UserID: 3, Username: "a", Role: models.RoleAdmin},
			map[string]any{"userId": uint(3), "username": "a", "role": string(models.RoleAdmin), "isAdmin": true, "scopeGroupId": nil, "superUser": false},
		},
		{
			"superuser with no role is admin",
			&auth.Principal{UserID: 4, Username: "s", SuperUser: true},
			map[string]any{"userId": uint(4), "username": "s", "role": "", "isAdmin": true, "scopeGroupId": nil, "superUser": true},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := auth.DescribeContext(auth.WithPrincipal(context.Background(), c.p))
			assert.Equal(t, c.want, got)
		})
	}
}
