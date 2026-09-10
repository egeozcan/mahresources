package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestReductionExternalResources(t *testing.T) {
	for _, tier := range []string{models.ReductionTierIdentical, models.ReductionTierNear} {
		for _, selection := range []string{"resources", "groups", "owned", "owned subtree"} {
			t.Run(tier+"/"+selection, func(t *testing.T) {
				tc := SetupTestEnv(t)
				inside := addImage(t, tc, "inside.jpg", 400, 400)
				twin := addImage(t, tc, "twin.jpg", 200, 200)
				outside := addImage(t, tc, "outside.jpg", 800, 800)
				if tier == models.ReductionTierIdentical {
					require.NoError(t, tc.DB.Model(&models.Resource{}).
						Where("id IN ?", []uint{inside.ID, twin.ID, outside.ID}).Update("hash", "shared").Error)
				} else {
					pairThem(t, tc, inside, twin, 2)
					pairThem(t, tc, inside, outside, 3)
					// No outside-to-twin pair: excluding the external winner must
					// preserve the inside-to-twin cluster before rejustification.
				}

				root := &models.Group{Name: "Root"}
				require.NoError(t, tc.DB.Create(root).Error)
				child := &models.Group{Name: "Child", OwnerId: &root.ID}
				require.NoError(t, tc.DB.Create(child).Error)
				creator := query_models.ResourceReductionCreator{Name: "External matching"}
				switch selection {
				case "resources":
					creator.ResourceIds = []uint{inside.ID, twin.ID}
				case "groups":
					creator.GroupIds = []uint{root.ID}
					require.NoError(t, tc.DB.Model(inside).Update("owner_id", child.ID).Error)
					require.NoError(t, tc.DB.Model(child).Association("RelatedResources").Append(twin))
				case "owned", "owned subtree":
					creator.OwnerId = root.ID
					ownerID := root.ID
					if selection == "owned subtree" {
						creator.IncludeDescendants = true
						ownerID = child.ID
					}
					require.NoError(t, tc.DB.Model(&models.Resource{}).
						Where("id IN ?", []uint{inside.ID, twin.ID}).Update("owner_id", ownerID).Error)
				}

				for _, exclude := range []bool{false, true} {
					t.Run(fmt.Sprintf("exclude=%t", exclude), func(t *testing.T) {
						creator.ExcludeExternalResources = exclude
						response := tc.MakeRequest(http.MethodPost, "/v1/reduction", creator)
						require.Equal(t, http.StatusOK, response.Code, response.Body.String())
						var created struct{ ID uint }
						require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
						red, err := tc.AppCtx.GetResourceReduction(created.ID, nil, false)
						require.NoError(t, err)
						assert.Equal(t, exclude, red.ExcludeExternalResources)

						for run := 0; run < 2; run++ {
							plan := computeReduction(t, tc, red.ID)
							require.Len(t, plan.Clusters, 1)
							cluster := plan.Clusters[0]
							assert.Equal(t, tier, cluster.Tier)
							assert.Equal(t, 2, plan.Coverage.ExtentSize)
							if exclude {
								assert.Equal(t, inside.ID, cluster.WinnerID)
								assert.ElementsMatch(t, []uint{inside.ID, twin.ID}, memberIDs(cluster))
								for _, member := range cluster.Members {
									assert.True(t, member.InExtent)
								}
							} else {
								assert.Equal(t, outside.ID, cluster.WinnerID)
							}
						}

						// Extending retains the original choice even if a caller
						// sends a different creation setting.
						widened, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
							ID: red.ID, ResourceIds: []uint{outside.ID}, ExcludeExternalResources: !exclude,
						}, nil, false)
						require.NoError(t, err)
						assert.Equal(t, exclude, widened.ExcludeExternalResources)
						plan := computeReduction(t, tc, red.ID)
						require.NotNil(t, clusterFor(plan, outside.ID))
						assert.Equal(t, outside.ID, clusterFor(plan, outside.ID).WinnerID,
							"a newly selected resource becomes eligible")
					})
				}
			})
		}
	}
}

func TestReductionExcludesOnlyExternalMatch(t *testing.T) {
	for _, tier := range []string{models.ReductionTierIdentical, models.ReductionTierNear} {
		t.Run(tier, func(t *testing.T) {
			tc := SetupTestEnv(t)
			inside := addImage(t, tc, "inside.jpg", 200, 200)
			outside := addImage(t, tc, "outside.jpg", 800, 800)
			if tier == models.ReductionTierIdentical {
				require.NoError(t, tc.DB.Model(&models.Resource{}).
					Where("id IN ?", []uint{inside.ID, outside.ID}).Update("hash", "shared").Error)
			} else {
				pairThem(t, tc, inside, outside, 2)
			}
			red, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
				ResourceIds: []uint{inside.ID}, ExcludeExternalResources: true,
			}, nil, false)
			require.NoError(t, err)
			assert.Empty(t, computeReduction(t, tc, red.ID).Clusters)
		})
	}
}
