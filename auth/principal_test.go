package auth

import (
	"testing"

	"mahresources/models"
)

func TestPrincipalCapabilityMatrix(t *testing.T) {
	cases := []struct {
		name                                     string
		p                                        *Principal
		admin, system, taxonomy, write, readOnly bool
	}{
		{"nil", nil, false, false, false, false, true},
		{"superuser", SystemPrincipal(), true, true, true, true, false},
		{"admin", &Principal{Role: models.RoleAdmin}, true, true, true, true, false},
		{"editor", &Principal{Role: models.RoleEditor}, false, false, false, true, false},
		{"user", &Principal{Role: models.RoleUser}, false, false, false, true, false},
		{"guest", &Principal{Role: models.RoleGuest}, false, false, false, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.p.IsAdmin(); got != c.admin {
				t.Errorf("IsAdmin = %v, want %v", got, c.admin)
			}
			if got := c.p.CanManageSystem(); got != c.system {
				t.Errorf("CanManageSystem = %v, want %v", got, c.system)
			}
			if got := c.p.CanManageTaxonomy(); got != c.taxonomy {
				t.Errorf("CanManageTaxonomy = %v, want %v", got, c.taxonomy)
			}
			if got := c.p.CanWrite(); got != c.write {
				t.Errorf("CanWrite = %v, want %v", got, c.write)
			}
			if got := c.p.IsReadOnly(); got != c.readOnly {
				t.Errorf("IsReadOnly = %v, want %v", got, c.readOnly)
			}
		})
	}
}

func TestPrincipalScoping(t *testing.T) {
	gid := uint(7)

	scopedUser := &Principal{Role: models.RoleUser, ScopeGroupID: &gid}
	if !scopedUser.IsScoped() {
		t.Error("a user with a scope group should be scoped")
	}
	if scopedUser.RequiresScope() {
		t.Error("a user does not require a scope group")
	}

	unscopedUser := &Principal{Role: models.RoleUser}
	if unscopedUser.IsScoped() {
		t.Error("a user without a scope group should not be scoped")
	}

	guest := &Principal{Role: models.RoleGuest}
	if !guest.RequiresScope() {
		t.Error("a guest must require a scope group")
	}
	if guest.IsScoped() {
		t.Error("a guest without a configured scope group is not yet scoped")
	}

	// Superuser ignores any configured scope.
	super := SystemPrincipal()
	super.ScopeGroupID = &gid
	if super.IsScoped() || super.RequiresScope() {
		t.Error("superuser must never be scoped")
	}
}

func TestFromUser(t *testing.T) {
	gid := uint(3)
	u := &models.User{Username: "alice", Role: models.RoleGuest, ScopeGroupId: &gid}
	u.ID = 42
	p := FromUser(u)
	if p.UserID != 42 || p.Username != "alice" || p.Role != models.RoleGuest || p.ScopeGroupID == nil || *p.ScopeGroupID != 3 {
		t.Errorf("FromUser produced unexpected principal: %+v", p)
	}
	if FromUser(nil) != nil {
		t.Error("FromUser(nil) should be nil")
	}
}
