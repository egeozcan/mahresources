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

func TestMergeGroupsPreservesReverseRelatedGroups(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create winner, loser, and an observer group
	winner := &models.Group{Name: "Winner"}
	tc.DB.Create(winner)
	loser := &models.Group{Name: "Loser"}
	tc.DB.Create(loser)
	observer := &models.Group{Name: "Observer"}
	tc.DB.Create(observer)

	// Observer has loser as a related group (observer → loser)
	tc.DB.Exec("INSERT INTO group_related_groups (group_id, related_group_id) VALUES (?, ?)", observer.ID, loser.ID)

	// Merge loser into winner
	err := tc.AppCtx.MergeGroups(winner.ID, []uint{loser.ID})
	assert.NoError(t, err)

	// Observer should now have winner as a related group (observer → winner)
	var count int64
	tc.DB.Raw("SELECT COUNT(*) FROM group_related_groups WHERE group_id = ? AND related_group_id = ?", observer.ID, winner.ID).Scan(&count)
	assert.Equal(t, int64(1), count,
		"Reverse relationships (other→loser) should be transferred to winner during merge")
}

func TestDeleteGroupDoesNotDeleteChildGroups(t *testing.T) {
	tc := SetupTestEnv(t)

	parent := tc.CreateDummyGroup("ParentToDelete")
	child := &models.Group{Name: "ChildGroup", OwnerId: &parent.ID}
	tc.DB.Create(child)

	err := tc.AppCtx.DeleteGroup(parent.ID)
	assert.NoError(t, err)

	// Child should survive with OwnerId set to NULL, NOT be cascade-deleted
	var check models.Group
	result := tc.DB.First(&check, child.ID)
	assert.NoError(t, result.Error,
		"Child group should survive parent deletion (SET NULL), not be cascade-deleted")
	assert.Nil(t, check.OwnerId,
		"Child OwnerId should be NULL after parent is deleted")
}

func TestDeleteGroupNullsOwnedResourceOwner(t *testing.T) {
	tc := SetupTestEnv(t)

	group := tc.CreateDummyGroup("ResourceOwnerGroup")
	res := &models.Resource{Name: "Owned Resource", OwnerId: &group.ID}
	tc.DB.Create(res)

	err := tc.AppCtx.DeleteGroup(group.ID)
	assert.NoError(t, err)

	// Resource should survive with OwnerId set to NULL
	var check models.Resource
	result := tc.DB.First(&check, res.ID)
	assert.NoError(t, result.Error,
		"Owned resource should survive group deletion")
	assert.Nil(t, check.OwnerId,
		"Resource OwnerId should be NULL after owner group is deleted")
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

func TestGroupSearchChildrenCountMatchesList(t *testing.T) {
	tc := SetupTestEnv(t)

	parent := tc.CreateDummyGroup("CountParent")

	child1 := &models.Group{Name: "CountMatch A", OwnerId: &parent.ID}
	tc.DB.Create(child1)
	child2 := &models.Group{Name: "CountMatch B", OwnerId: &parent.ID}
	tc.DB.Create(child2)

	// The count from GetGroupsCount should match the actual number of unique groups
	// returned by GetGroups when SearchChildrenForName is enabled
	query := &query_models.GroupQuery{
		Name:                  "CountMatch",
		SearchChildrenForName: true,
	}

	groups, err := tc.AppCtx.GetGroups(0, 50, query)
	assert.NoError(t, err)

	count, err := tc.AppCtx.GetGroupsCount(query)
	assert.NoError(t, err)

	assert.Equal(t, int64(len(groups)), count,
		"GetGroupsCount should return the same number as len(GetGroups) when child joins produce duplicates")
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

func TestGroupMetaSortWithChildJoinUsesCorrectTable(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create parent with meta rank=2
	parent := &models.Group{Name: "SortParent", Meta: []byte(`{"rank":"2"}`)}
	tc.DB.Create(parent)

	// Create child with a matching name so the child join activates
	child := &models.Group{Name: "SortChild", OwnerId: &parent.ID, Meta: []byte(`{"rank":"1"}`)}
	tc.DB.Create(child)

	// Search with SearchChildrenForName + meta sort — this activates the child JOIN.
	// The meta sort must reference the main table (groups.meta), not the ambiguous bare "meta".
	url := "/v1/groups?Name=Sort&SearchChildrenForName=true&SortBy=meta->>'rank'"
	resp := tc.MakeRequest(http.MethodGet, url, nil)
	assert.Equal(t, http.StatusOK, resp.Code,
		"meta sort with child join should not cause ambiguous column error")

	var groups []models.Group
	json.Unmarshal(resp.Body.Bytes(), &groups)

	// Both parent (matched via child name) and child (matched directly) should be returned
	assert.GreaterOrEqual(t, len(groups), 1, "should return results when sorting by meta with child join")
}

func TestGroupChildMetaFilterWithMultipleMatchingChildren(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a parent group
	parent := &models.Group{Name: "MetaParent", Meta: []byte(`{}`)}
	tc.DB.Create(parent)

	// Create two children that BOTH match the meta filter
	child1 := &models.Group{Name: "Child1", OwnerId: &parent.ID, Meta: []byte(`{"color":"red"}`)}
	tc.DB.Create(child1)
	child2 := &models.Group{Name: "Child2", OwnerId: &parent.ID, Meta: []byte(`{"color":"red"}`)}
	tc.DB.Create(child2)

	// Query groups using app context with child meta filter directly
	query := &query_models.GroupQuery{
		MetaQuery: []query_models.ColumnMeta{
			{Key: "child.color", Value: "red", Operation: "EQ"},
		},
	}
	groups, err := tc.AppCtx.GetGroups(0, 50, query)
	assert.NoError(t, err)

	// Parent should be included even though MULTIPLE children match
	found := false
	for _, g := range groups {
		if g.ID == parent.ID {
			found = true
			break
		}
	}
	assert.True(t, found,
		"Parent should be found when multiple children match child.meta filter, not excluded by count(*) = 1")
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

func TestGroupUpdatePartialJSONPreservesOtherFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group with both name and description populated
	group := tc.CreateDummyGroup("Original Name")
	group.Description = "Original Desc"
	group.Meta = []byte(`{"key":"value"}`)
	tc.DB.Save(group)

	// Send a partial JSON body that only changes the description
	// (simulates CLI: mr group edit ID --description "New")
	partialBody := map[string]any{
		"ID":          group.ID,
		"Description": "Updated Desc",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/group", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	// The name should be preserved, not cleared to empty
	var check models.Group
	tc.DB.First(&check, group.ID)
	assert.Equal(t, "Updated Desc", check.Description)
	assert.Equal(t, "Original Name", check.Name,
		"Editing only description should not clear the name — partial JSON must preserve unset fields")
	assert.JSONEq(t, `{"key":"value"}`, string(check.Meta),
		"Editing only description should not reset meta to default")
}

func TestGroupUpdatePartialJSONPreservesTagAssociations(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create tags
	tag1 := &models.Tag{Name: "Tag Alpha"}
	tag2 := &models.Tag{Name: "Tag Beta"}
	tc.DB.Create(tag1)
	tc.DB.Create(tag2)

	// Create a group with tags via API (full update with all fields)
	createResp := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]any{
		"Name": "Tagged Group",
		"Tags": []uint{tag1.ID, tag2.ID},
	})
	assert.Equal(t, http.StatusOK, createResp.Code)
	var created models.Group
	json.Unmarshal(createResp.Body.Bytes(), &created)

	// Verify tags were assigned
	var checkBefore models.Group
	tc.DB.Preload("Tags").First(&checkBefore, created.ID)
	assert.Equal(t, 2, len(checkBefore.Tags), "group should start with 2 tags")

	// Send a partial JSON update that only changes the description
	// Tags field is NOT included in the request body
	partialBody := map[string]any{
		"ID":          created.ID,
		"Description": "Updated description",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/group", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	// The tags should be preserved, not cleared
	var checkAfter models.Group
	tc.DB.Preload("Tags").First(&checkAfter, created.ID)
	assert.Equal(t, "Updated description", checkAfter.Description)
	assert.Equal(t, 2, len(checkAfter.Tags),
		"Editing only description should not clear tag associations — partial JSON must preserve unset arrays")
}

func TestResourceEditPartialJSONPreservesTagAssociations(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create tags
	tag1 := &models.Tag{Name: "Res Tag A"}
	tag2 := &models.Tag{Name: "Res Tag B"}
	tc.DB.Create(tag1)
	tc.DB.Create(tag2)

	// Create a resource and attach tags directly in DB
	res := &models.Resource{Name: "Tagged Resource", Meta: []byte(`{}`), OwnMeta: []byte(`{}`)}
	tc.DB.Create(res)
	tc.DB.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", res.ID, tag1.ID)
	tc.DB.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", res.ID, tag2.ID)

	// Verify tags exist
	var checkBefore models.Resource
	tc.DB.Preload("Tags").First(&checkBefore, res.ID)
	assert.Equal(t, 2, len(checkBefore.Tags), "resource should start with 2 tags")

	// Send a partial JSON edit that only changes the description
	partialBody := map[string]any{
		"ID":          res.ID,
		"Description": "Updated description",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/edit", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	// The tags should be preserved, not cleared
	var checkAfter models.Resource
	tc.DB.Preload("Tags").First(&checkAfter, res.ID)
	assert.Equal(t, "Updated description", checkAfter.Description)
	assert.Equal(t, 2, len(checkAfter.Tags),
		"Editing only description should not clear tag associations — partial JSON must preserve unset arrays")
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

func TestResourceEditPartialJSONPreservesOtherFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource with a name and description
	res := &models.Resource{Name: "Original Name", Description: "Original Desc", Meta: []byte(`{"key":"value"}`)}
	tc.DB.Create(res)

	// Send a partial JSON body that only changes the description (simulates CLI: mr resource edit --description "New")
	partialBody := map[string]any{
		"ID":          res.ID,
		"Description": "Updated Desc",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/edit", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	// The name should be preserved, not cleared to empty
	var check models.Resource
	tc.DB.First(&check, res.ID)
	assert.Equal(t, "Updated Desc", check.Description)
	assert.Equal(t, "Original Name", check.Name,
		"Editing only description should not clear the name — partial JSON must preserve unset fields")
}

func TestResourceEditPartialJSONPreservesResourceCategoryId(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource category
	rc := &models.ResourceCategory{Name: "Photo"}
	tc.DB.Create(rc)

	// Create a resource and assign the category via direct SQL to avoid GORM association issues
	res := &models.Resource{Name: "Categorized Resource", Meta: []byte(`{}`), OwnMeta: []byte(`{}`)}
	tc.DB.Create(res)
	tc.DB.Model(res).Update("resource_category_id", rc.ID)

	// Verify category is set
	var before models.Resource
	tc.DB.First(&before, res.ID)
	if !assert.NotNil(t, before.ResourceCategoryId, "setup: resource should have ResourceCategoryId") {
		return
	}

	// Partial JSON edit — only change description
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/edit", map[string]any{
		"ID":          res.ID,
		"Description": "Updated",
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var after models.Resource
	tc.DB.First(&after, res.ID)
	assert.Equal(t, "Updated", after.Description)
	if assert.NotNil(t, after.ResourceCategoryId,
		"Editing only description should not clear ResourceCategoryId") {
		assert.Equal(t, rc.ID, *after.ResourceCategoryId)
	}
}

func TestResourceEditPartialJSONPreservesDimensions(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource with known dimensions
	res := &models.Resource{
		Name:   "Image with dimensions",
		Width:  1920,
		Height: 1080,
		Meta:   []byte(`{}`),
	}
	tc.DB.Create(res)

	// Send a partial JSON edit that only changes the description
	partialBody := map[string]any{
		"ID":          res.ID,
		"Description": "Updated",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/edit", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	var check models.Resource
	tc.DB.First(&check, res.ID)
	assert.Equal(t, "Updated", check.Description)
	assert.Equal(t, uint(1920), check.Width,
		"Editing only description should not clear Width")
	assert.Equal(t, uint(1080), check.Height,
		"Editing only description should not clear Height")
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
