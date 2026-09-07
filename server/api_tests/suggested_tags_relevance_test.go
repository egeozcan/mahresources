package api_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/models"
)

func TestSuggestedTags_HTTP_Relevance(t *testing.T) {
	assertSuggestedTagRelevance(t, SetupTestEnv(t))
}

// Shared with the PostgreSQL wrapper so the same public contract exercises both dialects.
func assertSuggestedTagRelevance(t *testing.T, tc *TestContext) {
	owner := &models.Group{Name: "relevance"}
	require.NoError(t, tc.DB.Create(owner).Error)
	seed := &models.Tag{Name: "seed"}
	related := &models.Tag{Name: "related"}
	require.NoError(t, tc.DB.Create(seed).Error)
	require.NoError(t, tc.DB.Create(related).Error)
	target := &models.Resource{Name: "target", OwnerId: &owner.ID, Meta: []byte("{}"), OwnMeta: []byte("{}")}
	peer := &models.Resource{Name: "peer", OwnerId: &owner.ID, Meta: []byte("{}"), OwnMeta: []byte("{}")}
	require.NoError(t, tc.DB.Create(target).Error)
	require.NoError(t, tc.DB.Create(peer).Error)
	require.NoError(t, tc.DB.Model(target).Association("Tags").Append(seed))
	require.NoError(t, tc.DB.Model(peer).Association("Tags").Append(seed, related))
	require.NoError(t, tc.DB.Create(&models.ResourceSimilarity{ResourceID1: target.ID, ResourceID2: peer.ID, HammingDistance: 3}).Error)
	rr := tc.MakeRequest(http.MethodGet, "/v1/resource/suggestedTags?id="+itoa(int(target.ID)), nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	got := decodeSuggestions(t, rr.Body.String())
	require.Len(t, got.Suggestions, 1)
	s := got.Suggestions[0]
	require.Equal(t, related.ID, s.ID)
	require.Equal(t, []string{"similar", "cooccurrence", "group"}, s.Sources)
	require.InDelta(t, .5*.5/2.5+.3/6+.2/6, s.Score, 1e-12)
}
