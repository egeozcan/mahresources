package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroupCategoryIdNullWhenZero(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("Create group without category stores NULL not zero", func(t *testing.T) {
		// Create a group with CategoryId=0 (no category selected)
		payload := query_models.GroupCreator{
			Name:       "No Category Group",
			CategoryId: 0,
		}
		resp := tc.MakeRequest(http.MethodPost, "/v1/group", payload)
		assert.Equal(t, http.StatusOK, resp.Code)

		var created models.Group
		json.Unmarshal(resp.Body.Bytes(), &created)

		// Verify in the DB that CategoryId is NULL, not a pointer to 0
		var check models.Group
		tc.DB.First(&check, created.ID)
		assert.Nil(t, check.CategoryId,
			"CategoryId should be NULL when no category is specified, not a pointer to 0")
	})

	t.Run("Update group to remove category stores NULL not zero", func(t *testing.T) {
		// First create a group WITH a category
		cat := &models.Category{Name: "Temp Cat"}
		tc.DB.Create(cat)

		createPayload := query_models.GroupCreator{
			Name:       "Will Remove Category",
			CategoryId: cat.ID,
		}
		createResp := tc.MakeRequest(http.MethodPost, "/v1/group", createPayload)
		assert.Equal(t, http.StatusOK, createResp.Code)
		var group models.Group
		json.Unmarshal(createResp.Body.Bytes(), &group)

		// Now update with CategoryId=0 to remove the category
		updatePayload := query_models.GroupEditor{
			ID: group.ID,
			GroupCreator: query_models.GroupCreator{
				Name:       "Will Remove Category",
				CategoryId: 0,
			},
		}
		updateResp := tc.MakeRequest(http.MethodPost, "/v1/group", updatePayload)
		assert.Equal(t, http.StatusOK, updateResp.Code)

		var check models.Group
		tc.DB.First(&check, group.ID)
		assert.Nil(t, check.CategoryId,
			"CategoryId should be NULL after update with CategoryId=0, not a pointer to 0")
	})
}

func TestMergeGroupsRedirectsToGroup(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	cat := &models.Category{Name: "Merge Cat"}
	tc.DB.Create(cat)

	winner := &models.Group{Name: "Winner Group", CategoryId: &cat.ID}
	tc.DB.Create(winner)
	loser := &models.Group{Name: "Loser Group", CategoryId: &cat.ID}
	tc.DB.Create(loser)

	payload := query_models.MergeQuery{
		Winner: winner.ID,
		Losers: []uint{loser.ID},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, "/v1/groups/merge", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/html")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusSeeOther, rr.Code)
	location := rr.Header().Get("Location")
	expected := fmt.Sprintf("/group?id=%d", winner.ID)
	assert.Equal(t, expected, location,
		"merge groups should redirect to /group?id=..., not /resource?id=...")
}

func TestMergeGroupsMetaWinnerTakesPrecedence(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create winner with meta key "color" = "blue"
	winner := &models.Group{Name: "Winner", Meta: []byte(`{"color":"blue","only_winner":"yes"}`)}
	tc.DB.Create(winner)
	// Create loser with meta key "color" = "red" (conflicting) and a unique key
	loser := &models.Group{Name: "Loser", Meta: []byte(`{"color":"red","only_loser":"yes"}`)}
	tc.DB.Create(loser)

	err := tc.AppCtx.MergeGroups(winner.ID, []uint{loser.ID})
	assert.NoError(t, err)

	// Reload the winner from DB
	var merged models.Group
	tc.DB.First(&merged, winner.ID)

	var meta map[string]interface{}
	json.Unmarshal(merged.Meta, &meta)

	// Winner's value should take precedence for conflicting keys (consistent with Postgres and resource merge)
	assert.Equal(t, "blue", meta["color"],
		"Winner's meta should take precedence for conflicting keys, got loser's value instead")

	// Both unique keys should be preserved
	assert.Equal(t, "yes", meta["only_winner"], "Winner's unique key should be preserved")
	assert.Equal(t, "yes", meta["only_loser"], "Loser's unique key should be preserved")
}

func TestMergeGroupsPreservesRelationTypeId(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create category (needed for relation types)
	cat := &models.Category{Name: "Rel Test Cat"}
	tc.DB.Create(cat)

	// Create winner, loser, and a target group
	winner := &models.Group{Name: "Winner", CategoryId: &cat.ID}
	tc.DB.Create(winner)
	loser := &models.Group{Name: "Loser", CategoryId: &cat.ID}
	tc.DB.Create(loser)
	target := &models.Group{Name: "Target", CategoryId: &cat.ID}
	tc.DB.Create(target)

	// Create a relation type
	relType := &models.GroupRelationType{
		Name:           "depends-on",
		FromCategoryId: &cat.ID,
		ToCategoryId:   &cat.ID,
	}
	tc.DB.Create(relType)

	// Create a relation from loser -> target with the relation type
	rel := &models.GroupRelation{
		FromGroupId:    &loser.ID,
		ToGroupId:      &target.ID,
		RelationTypeId: &relType.ID,
		Name:           "test relation",
	}
	tc.DB.Create(rel)

	// Merge loser into winner
	err := tc.AppCtx.MergeGroups(winner.ID, []uint{loser.ID})
	assert.NoError(t, err)

	// The relation should have been transferred to the winner with relation_type_id preserved
	var transferred models.GroupRelation
	tc.DB.Where("from_group_id = ? AND to_group_id = ?", winner.ID, target.ID).First(&transferred)

	assert.NotZero(t, transferred.ID, "Relation should have been transferred to winner")
	assert.NotNil(t, transferred.RelationTypeId,
		"RelationTypeId must be preserved when transferring relations during merge")
	if transferred.RelationTypeId != nil {
		assert.Equal(t, relType.ID, *transferred.RelationTypeId)
	}
}

func TestGroupCreateWithoutURLStoresNull(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group without specifying a URL
	payload := query_models.GroupCreator{
		Name: "No URL Group",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/group", payload)
	assert.Equal(t, http.StatusOK, resp.Code)

	var created models.Group
	json.Unmarshal(resp.Body.Bytes(), &created)

	// Check the database directly — URL should be NULL, not empty string
	var dbValue *string
	tc.DB.Raw("SELECT url FROM groups WHERE id = ?", created.ID).Scan(&dbValue)

	assert.Nil(t, dbValue,
		"URL should be NULL when not specified, not an empty string")
}

func TestGroupSearchChildrenNoDuplicates(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a parent group
	parent := tc.CreateDummyGroup("Parent")

	// Create two children whose names match the search term
	child1 := &models.Group{Name: "Matching Child A", OwnerId: &parent.ID}
	tc.DB.Create(child1)
	child2 := &models.Group{Name: "Matching Child B", OwnerId: &parent.ID}
	tc.DB.Create(child2)

	// Search for "Matching" with SearchChildrenForName enabled.
	// The parent should appear at most once (found via its children), not duplicated.
	resp := tc.MakeRequest(http.MethodGet, "/v1/groups?Name=Matching&SearchChildrenForName=true", nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var groups []models.Group
	json.Unmarshal(resp.Body.Bytes(), &groups)

	// Count how many times the parent appears
	parentCount := 0
	for _, g := range groups {
		if g.ID == parent.ID {
			parentCount++
		}
	}

	assert.LessOrEqual(t, parentCount, 1,
		"Parent group should appear at most once when multiple children match, not be duplicated per child")
}

func TestGroupFilterByGroupsDoesNotBypassNameFilter(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a parent group
	parent := tc.CreateDummyGroup("Parent Group")

	// Create two child groups: one related via junction table, one owned
	junctionChild := &models.Group{Name: "Junction Child"}
	tc.DB.Create(junctionChild)
	tc.DB.Exec("INSERT INTO group_related_groups (group_id, related_group_id) VALUES (?, ?)", parent.ID, junctionChild.ID)

	ownedChild := &models.Group{Name: "Owned Child", OwnerId: &parent.ID}
	tc.DB.Create(ownedChild)

	// Filter by parent group AND name="Owned" — should only return the owned child.
	// The junction child ("Junction Child") should NOT appear because it doesn't match "Owned".
	url := fmt.Sprintf("/v1/groups?Groups=%d&Name=Owned", parent.ID)
	resp := tc.MakeRequest(http.MethodGet, url, nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var groups []models.Group
	json.Unmarshal(resp.Body.Bytes(), &groups)

	assert.Equal(t, 1, len(groups),
		"Groups filter combined with Name filter should return only groups matching BOTH; junction-table matches must not bypass Name filter")
	if len(groups) > 0 {
		assert.Equal(t, ownedChild.ID, groups[0].ID)
	}
}

func TestGroupEndpoints(t *testing.T) {
	tc := SetupTestEnv(t)

	// Setup Helper
	createGroup := func(name string, catID uint) *models.Group {
		payload := query_models.GroupCreator{
			Name:       name,
			CategoryId: catID,
		}
		resp := tc.MakeRequest(http.MethodPost, "/v1/group", payload)
		assert.Equal(t, http.StatusOK, resp.Code)
		var g models.Group
		json.Unmarshal(resp.Body.Bytes(), &g)
		return &g
	}

	// 1. Categories needed for groups
	catResp := tc.MakeRequest(http.MethodPost, "/v1/category", query_models.CategoryCreator{Name: "Main Cat"})
	var mainCat models.Category
	json.Unmarshal(catResp.Body.Bytes(), &mainCat)

	t.Run("Create and List Groups", func(t *testing.T) {
		g1 := createGroup("Group 1", mainCat.ID)
		assert.NotZero(t, g1.ID)

		resp := tc.MakeRequest(http.MethodGet, "/v1/groups", nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var groups []models.Group
		json.Unmarshal(resp.Body.Bytes(), &groups)
		assert.NotEmpty(t, groups)
	})

	t.Run("Get Group", func(t *testing.T) {
		g := createGroup("Group to Get", mainCat.ID)
		url := fmt.Sprintf("/v1/group?id=%d", g.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)
	})

	t.Run("Group Hierarchy", func(t *testing.T) {
		parent := createGroup("Parent", mainCat.ID)
		child := createGroup("Child", mainCat.ID)

		// Add child to parent (update child's owner)
		// Assuming OwnerId creates the hierarchy
		payload := query_models.GroupEditor{
			ID: child.ID,
			GroupCreator: query_models.GroupCreator{
				Name:       child.Name,
				CategoryId: mainCat.ID,
				OwnerId:    parent.ID,
			},
		}
		tc.MakeRequest(http.MethodPost, "/v1/group", payload)

		// Check parents endpoint
		url := fmt.Sprintf("/v1/group/parents?id=%d", child.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var parents []models.Group
		json.Unmarshal(resp.Body.Bytes(), &parents)
		assert.NotEmpty(t, parents)
		assert.Equal(t, parent.ID, parents[0].ID)
	})

	t.Run("Delete Group", func(t *testing.T) {
		g := createGroup("To Delete", mainCat.ID)
		url := fmt.Sprintf("/v1/group/delete?Id=%d", g.ID)
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var check models.Group
		assert.Error(t, tc.DB.First(&check, g.ID).Error)
	})
}

func TestGroupUpdateCategoryId(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two categories
	resp1 := tc.MakeRequest(http.MethodPost, "/v1/category", query_models.CategoryCreator{Name: "Cat A"})
	assert.Equal(t, http.StatusOK, resp1.Code)
	var catA models.Category
	json.Unmarshal(resp1.Body.Bytes(), &catA)

	resp2 := tc.MakeRequest(http.MethodPost, "/v1/category", query_models.CategoryCreator{Name: "Cat B"})
	assert.Equal(t, http.StatusOK, resp2.Code)
	var catB models.Category
	json.Unmarshal(resp2.Body.Bytes(), &catB)

	// Create a group with Cat A
	createPayload := query_models.GroupCreator{
		Name:       "Test Group",
		CategoryId: catA.ID,
	}
	createResp := tc.MakeRequest(http.MethodPost, "/v1/group", createPayload)
	assert.Equal(t, http.StatusOK, createResp.Code)
	var group models.Group
	json.Unmarshal(createResp.Body.Bytes(), &group)

	// Verify initial category
	var check models.Group
	tc.DB.First(&check, group.ID)
	assert.NotNil(t, check.CategoryId)
	assert.Equal(t, catA.ID, *check.CategoryId, "group should initially have Cat A")

	// Update the group to Cat B
	updatePayload := query_models.GroupEditor{
		ID: group.ID,
		GroupCreator: query_models.GroupCreator{
			Name:       "Test Group",
			CategoryId: catB.ID,
		},
	}
	updateResp := tc.MakeRequest(http.MethodPost, "/v1/group", updatePayload)
	assert.Equal(t, http.StatusOK, updateResp.Code)

	// Verify category was updated to Cat B
	tc.DB.First(&check, group.ID)
	assert.NotNil(t, check.CategoryId, "category should not be nil after update")
	assert.Equal(t, catB.ID, *check.CategoryId, "group category should be updated to Cat B")
}

func TestResourceEditOwnerIdZeroStoresNull(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group to use as initial owner
	group := tc.CreateDummyGroup("Owner")

	// Create a resource owned by the group
	res := &models.Resource{Name: "Owned Resource", OwnerId: &group.ID}
	tc.DB.Create(res)

	// Verify initial owner is set
	var check models.Resource
	tc.DB.First(&check, res.ID)
	assert.NotNil(t, check.OwnerId)
	assert.Equal(t, group.ID, *check.OwnerId)

	// Edit the resource with OwnerId=0 (removing owner)
	editPayload := query_models.ResourceEditor{
		ID: res.ID,
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "Owned Resource",
		},
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/edit", editPayload)
	assert.Equal(t, http.StatusOK, resp.Code)

	// Verify OwnerId is NULL, not a pointer to 0
	tc.DB.First(&check, res.ID)
	assert.Nil(t, check.OwnerId,
		"OwnerId should be NULL when edited with OwnerId=0, not a pointer to 0")
}

func TestResourceEditUpdatesWidthHeight(t *testing.T) {
	tc := SetupTestEnv(t)

	// Insert a resource with known dimensions directly in DB
	res := &models.Resource{
		Name:   "test image",
		Width:  100,
		Height: 200,
	}
	tc.DB.Create(res)

	// Verify initial dimensions
	var check models.Resource
	tc.DB.First(&check, res.ID)
	assert.Equal(t, uint(100), check.Width)
	assert.Equal(t, uint(200), check.Height)

	// Edit the resource with new dimensions via the API
	editPayload := query_models.ResourceEditor{
		ID: res.ID,
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:   "test image",
			Width:  800,
			Height: 600,
		},
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/edit", editPayload)
	assert.Equal(t, http.StatusOK, resp.Code)

	// Verify dimensions were updated
	tc.DB.First(&check, res.ID)
	assert.Equal(t, uint(800), check.Width, "width should be updated to 800")
	assert.Equal(t, uint(600), check.Height, "height should be updated to 600")
}

func TestResourceNotesFilterRequiresAllNotes(t *testing.T) {
	tc := SetupTestEnv(t)

	note1 := tc.CreateDummyNote("Note A")
	note2 := tc.CreateDummyNote("Note B")

	// Resource linked to BOTH notes
	resBoth := &models.Resource{Name: "Has Both Notes"}
	tc.DB.Create(resBoth)
	tc.DB.Model(resBoth).Association("Notes").Append(&[]*models.Note{{ID: note1.ID}, {ID: note2.ID}})

	// Resource linked to only note1
	resOne := &models.Resource{Name: "Has One Note"}
	tc.DB.Create(resOne)
	tc.DB.Model(resOne).Association("Notes").Append(&[]*models.Note{{ID: note1.ID}})

	// Filter by BOTH notes — should return only the resource that has both (AND semantics),
	// matching how Tags filtering works
	url := fmt.Sprintf("/v1/resources?Notes=%d&Notes=%d", note1.ID, note2.ID)
	resp := tc.MakeRequest(http.MethodGet, url, nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var resources []models.Resource
	json.Unmarshal(resp.Body.Bytes(), &resources)

	assert.Equal(t, 1, len(resources),
		"filtering by two notes should return only resources linked to BOTH notes (AND), not ANY (OR)")
	if len(resources) == 1 {
		assert.Equal(t, resBoth.ID, resources[0].ID)
	}
}

func TestResourceGroupFilterDoesNotDoubleCount(t *testing.T) {
	tc := SetupTestEnv(t)

	group := tc.CreateDummyGroup("Owner Group")

	// Create a resource that is BOTH owned by the group AND related via the junction table.
	// This can happen when a user adds a group as both owner and related group.
	res := &models.Resource{Name: "Double Linked", OwnerId: &group.ID}
	tc.DB.Create(res)
	tc.DB.Exec("INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)", group.ID, res.ID)

	// Filter resources by this group — should find the resource despite dual linkage
	url := fmt.Sprintf("/v1/resources?Groups=%d", group.ID)
	resp := tc.MakeRequest(http.MethodGet, url, nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var resources []models.Resource
	json.Unmarshal(resp.Body.Bytes(), &resources)

	assert.Equal(t, 1, len(resources),
		"Resource both owned by and related to a group should appear in results, not be excluded by double-counting")
	if len(resources) > 0 {
		assert.Equal(t, res.ID, resources[0].ID)
	}
}

func TestResourceEndpoints(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("Upload Resource (Mock)", func(t *testing.T) {
		// Mocking multipart upload is complex in this helper,
		// but we can test the `ResourceFromRemoteCreator` flow or just basic model logic if we can insert directly.
		// Let's rely on `ResourceFromLocalCreator` logic which mimics "Local" add.

		// payload := query_models.ResourceFromLocalCreator{
		// 	ResourceQueryBase: query_models.ResourceQueryBase{
		// 		Name: "Local File",
		// 	},
		// 	LocalPath: "/tmp/fake/file.txt",
		// 	PathName: "file.txt",
		// }

		// This endpoint usually moves files. In test env with memfs, it might fail if source doesn't exist.
		// For now, let's verify listing empty resources works.
		resp := tc.MakeRequest(http.MethodGet, "/v1/resources", nil)
		assert.Equal(t, http.StatusOK, resp.Code)
	})

	t.Run("Resource Metadata", func(t *testing.T) {
		// Manually insert resource
		res := &models.Resource{Name: "Manual Resource", Location: "/loc"}
		tc.DB.Create(res)

		// Test Edit Name
		url := fmt.Sprintf("/v1/resource/editName?id=%d", res.ID)
		tc.MakeRequest(http.MethodPost, url, map[string]string{"Name": "New Res Name"})

		var updated models.Resource
		tc.DB.First(&updated, res.ID)
		assert.Equal(t, "New Res Name", updated.Name)
	})
}
