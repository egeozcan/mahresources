package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/auth"
	"mahresources/models"
)

func TestDescribeContext(t *testing.T) {
	t.Run("nil principal returns nil", func(t *testing.T) {
		ctx := context.Background()
		res := auth.DescribeContext(ctx)
		assert.Nil(t, res)
	})

	t.Run("principal without scopeGroupID", func(t *testing.T) {
		p := &auth.Principal{
			UserID:   1,
			Username: "testuser",
			Role:     models.RoleUser,
		}
		ctx := auth.WithPrincipal(context.Background(), p)

		res := auth.DescribeContext(ctx)

		assert.NotNil(t, res)
		assert.Equal(t, uint(1), res["userId"])
		assert.Equal(t, "testuser", res["username"])
		assert.Equal(t, string(models.RoleUser), res["role"])
		assert.False(t, res["isAdmin"].(bool))
		assert.Nil(t, res["scopeGroupId"])
		assert.False(t, res["superUser"].(bool))
	})

	t.Run("principal with scopeGroupID", func(t *testing.T) {
		scopeGroupID := uint(42)
		p := &auth.Principal{
			UserID:       2,
			Username:     "editoruser",
			Role:         models.RoleEditor,
			ScopeGroupID: &scopeGroupID,
		}
		ctx := auth.WithPrincipal(context.Background(), p)

		res := auth.DescribeContext(ctx)

		assert.NotNil(t, res)
		assert.Equal(t, uint(2), res["userId"])
		assert.Equal(t, "editoruser", res["username"])
		assert.Equal(t, string(models.RoleEditor), res["role"])
		assert.False(t, res["isAdmin"].(bool))
		assert.Equal(t, uint(42), res["scopeGroupId"])
		assert.False(t, res["superUser"].(bool))
	})

	t.Run("admin principal", func(t *testing.T) {
		p := &auth.Principal{
			UserID:   3,
			Username: "adminuser",
			Role:     models.RoleAdmin,
		}
		ctx := auth.WithPrincipal(context.Background(), p)

		res := auth.DescribeContext(ctx)

		assert.NotNil(t, res)
		assert.Equal(t, uint(3), res["userId"])
		assert.Equal(t, "adminuser", res["username"])
		assert.Equal(t, string(models.RoleAdmin), res["role"])
		assert.True(t, res["isAdmin"].(bool))
		assert.Nil(t, res["scopeGroupId"])
		assert.False(t, res["superUser"].(bool))
	})

	t.Run("superuser principal", func(t *testing.T) {
		p := &auth.Principal{
			UserID:    4,
			Username:  "superuser",
			SuperUser: true,
		}
		ctx := auth.WithPrincipal(context.Background(), p)

		res := auth.DescribeContext(ctx)

		assert.NotNil(t, res)
		assert.Equal(t, uint(4), res["userId"])
		assert.Equal(t, "superuser", res["username"])
		assert.Equal(t, "", res["role"]) // Role not set
		assert.True(t, res["isAdmin"].(bool))
		assert.Nil(t, res["scopeGroupId"])
		assert.True(t, res["superUser"].(bool))
	})
}
