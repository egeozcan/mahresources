package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// roleBearer creates a user of the given role (with a scope group when required)
// and returns an "Authorization: Bearer ..." header value for it.
func roleBearer(t *testing.T, tc *TestContext, role models.Role) string {
	t.Helper()
	var scope *uint
	if role.AllowsScopeGroup() && role.RequiresScopeGroup() {
		g := &models.Group{Name: "scope-" + string(role)}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create scope group: %v", err)
		}
		scope = &g.ID
	}
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "rb_" + string(role), Password: "password1", Role: role, ScopeGroupId: scope,
	})
	if err != nil {
		t.Fatalf("create %s: %v", role, err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token for %s: %v", role, err)
	}
	return "Bearer " + raw
}

func TestAuthorizationMatrix(t *testing.T) {
	tc := setupAuthEnv(t)
	bearers := map[models.Role]string{
		models.RoleAdmin:  roleBearer(t, tc, models.RoleAdmin),
		models.RoleEditor: roleBearer(t, tc, models.RoleEditor),
		models.RoleUser:   roleBearer(t, tc, models.RoleUser),
		models.RoleGuest:  roleBearer(t, tc, models.RoleGuest),
	}

	cases := []struct {
		name, method, path, body string
		allowed                  map[models.Role]bool
	}{
		{
			name: "read notes (capRead)", method: http.MethodGet, path: "/v1/notes",
			allowed: map[models.Role]bool{models.RoleAdmin: true, models.RoleEditor: true, models.RoleUser: true, models.RoleGuest: true},
		},
		{
			name: "run MRQL (read-via-POST)", method: http.MethodPost, path: "/v1/mrql", body: `{"query":"resources | take 1"}`,
			allowed: map[models.Role]bool{models.RoleAdmin: true, models.RoleEditor: true, models.RoleUser: true, models.RoleGuest: true},
		},
		{
			name: "logout (session mgmt)", method: http.MethodPost, path: "/v1/auth/logout",
			allowed: map[models.Role]bool{models.RoleAdmin: true, models.RoleEditor: true, models.RoleUser: true, models.RoleGuest: true},
		},
		{
			name: "create note (capWrite)", method: http.MethodPost, path: "/v1/note", body: `{"name":"n"}`,
			allowed: map[models.Role]bool{models.RoleAdmin: true, models.RoleEditor: true, models.RoleUser: true, models.RoleGuest: false},
		},
		{
			name: "create note type (capEditor)", method: http.MethodPost, path: "/v1/note/noteType", body: `{"name":"nt"}`,
			allowed: map[models.Role]bool{models.RoleAdmin: true, models.RoleEditor: true, models.RoleUser: false, models.RoleGuest: false},
		},
		{
			name: "create category (capTaxonomy)", method: http.MethodPost, path: "/v1/category", body: `{"name":"c"}`,
			allowed: map[models.Role]bool{models.RoleAdmin: true, models.RoleEditor: false, models.RoleUser: false, models.RoleGuest: false},
		},
		{
			name: "admin data-stats (capSystem)", method: http.MethodGet, path: "/v1/admin/data-stats",
			allowed: map[models.Role]bool{models.RoleAdmin: true, models.RoleEditor: false, models.RoleUser: false, models.RoleGuest: false},
		},
	}

	for _, c := range cases {
		for role, bearer := range bearers {
			t.Run(c.name+"/"+string(role), func(t *testing.T) {
				headers := map[string]string{"Accept": "application/json", "Authorization": bearer}
				var body *strings.Reader
				if c.body != "" {
					headers["Content-Type"] = "application/json"
					body = strings.NewReader(c.body)
				}
				var rr = func() *http.Response {
					if body != nil {
						return doReq(tc, c.method, c.path, headers, nil, body).Result()
					}
					return doReq(tc, c.method, c.path, headers, nil, nil).Result()
				}()

				if c.allowed[role] {
					if rr.StatusCode == http.StatusForbidden {
						t.Errorf("%s %s as %s: expected allowed, got 403", c.method, c.path, role)
					}
				} else {
					if rr.StatusCode != http.StatusForbidden {
						t.Errorf("%s %s as %s: expected 403, got %d", c.method, c.path, role, rr.StatusCode)
					}
				}
			})
		}
	}
}
