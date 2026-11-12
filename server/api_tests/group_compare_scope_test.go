package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
)

type cmpEdgeFixture struct {
	f      scopeFixture
	typeID uint
	outFwd uint // child -> outside
	outBck uint // outside -> child
}

func buildCompareEdgeFixture(t *testing.T, tc *TestContext) cmpEdgeFixture {
	t.Helper()
	f := buildScopingFixture(t, tc)
	rt := &models.GroupRelationType{Name: "cmp-probe-type"}
	if err := tc.DB.Create(rt).Error; err != nil {
		t.Fatal(err)
	}
	mk := func(from, to uint, name, desc string) uint {
		e := &models.GroupRelation{Name: name, Description: desc, FromGroupId: &from, ToGroupId: &to, RelationTypeId: &rt.ID}
		if err := tc.DB.Create(e).Error; err != nil {
			t.Fatal(err)
		}
		return e.ID
	}
	fw := mk(f.childID, f.outsideID, "fwd-secret-name", "fwd-secret-desc")
	bk := mk(f.outsideID, f.childID, "back-secret-name", "back-secret-desc")
	mk(f.rootID, f.childID, "ctrl-fwd", "")
	mk(f.childID, f.rootID, "ctrl-back", "")
	return cmpEdgeFixture{f: f, typeID: rt.ID, outFwd: fw, outBck: bk}
}

func cmpBearer(t *testing.T, tc *TestContext, in *application_context.UserInput) string {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(in)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + raw
}

func TestGroupCompare_ScopedPrincipalSeesNoOutOfSubtreeEdges(t *testing.T) {
	tc := setupAuthEnv(t)
	fx := buildCompareEdgeFixture(t, tc)
	f := fx.f
	guest := cmpBearer(t, tc, &application_context.UserInput{Username: "sguest", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &f.rootID})
	principals := map[string]string{"scoped-user": f.bearer, "scoped-guest": guest}

	for name, bearer := range principals {
		for _, suffix := range []string{".json", "", ".body"} {
			t.Run(name+suffix, func(t *testing.T) {
				h := map[string]string{"Authorization": bearer, "Accept": "text/html"}
				rr := doReq(tc, http.MethodGet, fmt.Sprintf("/group/compare%s?g1=%d", suffix, f.childID), h, nil, nil)
				if rr.Code != http.StatusOK {
					t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
				}
				body := rr.Body.String()
				for _, s := range []string{"fwd-secret-name", "fwd-secret-desc", "back-secret-name", "back-secret-desc",
					fmt.Sprintf("/relation?id=%d", fx.outFwd), fmt.Sprintf("/relation?id=%d", fx.outBck),
					fmt.Sprintf("forward:%d:%d", fx.typeID, f.outsideID), fmt.Sprintf("reverse:%d:%d", fx.typeID, f.outsideID)} {
					if strings.Contains(body, s) {
						t.Errorf("response leaks %q", s)
					}
				}
				if suffix == ".json" {
					if !strings.Contains(body, "ctrl-back") {
						t.Errorf("positive control ctrl-back missing")
					}
					var out struct {
						Comparison struct{ Group1, Group2 models.Group } `json:"comparison"`
					}
					if err := json.Unmarshal([]byte(body), &out); err != nil {
						t.Fatal(err)
					}
					for _, g := range []models.Group{out.Comparison.Group1, out.Comparison.Group2} {
						for _, e := range g.Relationships {
							if e.ToGroupId != nil && *e.ToGroupId == f.outsideID {
								t.Errorf("Relationships has outside edge")
							}
						}
						for _, e := range g.BackRelations {
							if e.FromGroupId != nil && *e.FromGroupId == f.outsideID {
								t.Errorf("BackRelations has outside edge")
							}
						}
					}
				}
			})
		}
	}
}

func TestGroupCompare_CrossGroupScopedNoOutsideEdges(t *testing.T) {
	tc := setupAuthEnv(t)
	fx := buildCompareEdgeFixture(t, tc)
	f := fx.f
	h := map[string]string{"Authorization": f.bearer, "Accept": "application/json"}
	rr := doReq(tc, http.MethodGet, fmt.Sprintf("/group/compare.json?g1=%d&g2=%d", f.childID, f.rootID), h, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	body := rr.Body.String()
	for _, s := range []string{"fwd-secret", "back-secret", fmt.Sprintf(":%d:%d", fx.typeID, f.outsideID)} {
		if strings.Contains(body, s) {
			t.Errorf("leaks %q", s)
		}
	}
	for _, s := range []string{"ctrl-fwd", "ctrl-back"} {
		if !strings.Contains(body, s) {
			t.Errorf("control %q missing", s)
		}
	}
}

func TestGroupCompare_UnrestrictedRolesStillSeeEverything(t *testing.T) {
	tc := setupAuthEnv(t)
	fx := buildCompareEdgeFixture(t, tc)
	f := fx.f
	editor := cmpBearer(t, tc, &application_context.UserInput{Username: "ed", Password: "password1", Role: models.RoleEditor})
	h := map[string]string{"Authorization": editor, "Accept": "application/json"}
	rr := doReq(tc, http.MethodGet, fmt.Sprintf("/group/compare.json?g1=%d", f.childID), h, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, s := range []string{"fwd-secret-name", "back-secret-name", fmt.Sprintf("forward:%d:%d", fx.typeID, f.outsideID)} {
		if !strings.Contains(body, s) {
			t.Errorf("unscoped editor should see %q", s)
		}
	}
}

func TestCompareGroupsCross_ScopedFailsClosedOnEdges(t *testing.T) {
	tc := setupAuthEnv(t)
	fx := buildCompareEdgeFixture(t, tc)
	f := fx.f
	scoped := tc.AppCtx.WithPrincipal(&auth.Principal{UserID: 1, Role: models.RoleUser, ScopeGroupID: &f.rootID})
	cmp, err := scoped.CompareGroupsCross(f.childID, f.childID)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(cmp.Group1.Relationships); n != 1 {
		t.Errorf("Relationships = %d, want 1 (ctrl-back only)", n)
	}
	if len(cmp.Group1.BackRelations) != 1 || cmp.Group1.BackRelations[0].Name != "ctrl-fwd" {
		t.Errorf("BackRelations should hold only the root edge: %+v", cmp.Group1.BackRelations)
	}
	if cmp.ForwardRelations.TotalCount != 1 {
		t.Errorf("forward TotalCount = %d, want 1", cmp.ForwardRelations.TotalCount)
	}
}
