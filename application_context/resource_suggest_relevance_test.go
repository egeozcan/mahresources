package application_context

import (
	"context"
	"fmt"
	"gorm.io/gorm/logger"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/models"
)

func suggestionByName(t *testing.T, suggestions []contracts.SuggestedTag, name string) contracts.SuggestedTag {
	t.Helper()
	for _, s := range suggestions {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("missing %q in %+v", name, suggestions)
	return contracts.SuggestedTag{}
}

func TestSuggestedTags_DistanceAndSupport(t *testing.T) {
	ctx := newSuggestTestContext(t)
	near := suggestMakeTag(t, ctx, "near")
	far := suggestMakeTag(t, ctx, "far")
	target := suggestMakeResource(t, ctx, "target", nil)
	for i, row := range []struct {
		tag      *models.Tag
		distance uint8
	}{{near, 0}, {far, 3}, {nil, 0}} {
		var tags []*models.Tag
		if row.tag != nil {
			tags = append(tags, row.tag)
		}
		r := suggestMakeResource(t, ctx, fmt.Sprint(i), nil, tags...)
		suggestLinkSimilar(t, ctx, target.ID, r.ID, row.distance)
	}
	got, err := ctx.GetSuggestedTags(target.ID, 8)
	require.NoError(t, err)
	require.Equal(t, []string{"near", "far"}, suggestNames(got))
	require.InDelta(t, .5/3.5, got[0].Score, 1e-12)
	require.InDelta(t, .25/3.5, got[1].Score, 1e-12)
	peer := suggestMakeResource(t, ctx, "another", nil, near)
	suggestLinkSimilar(t, ctx, target.ID, peer.ID, 0)
	more, err := ctx.GetSuggestedTags(target.ID, 8)
	require.NoError(t, err)
	require.Greater(t, more[0].Score, got[0].Score)
}

func TestSuggestedTags_WeakSimilarityAndLegacyFallback(t *testing.T) {
	t.Run("weak match does not overwhelm group", func(t *testing.T) {
		ctx := newSuggestTestContext(t)
		owner := suggestMakeGroup(t, ctx, "owner", nil)
		common := suggestMakeTag(t, ctx, "common")
		weak := suggestMakeTag(t, ctx, "weak")
		target := suggestMakeResource(t, ctx, "target", &owner.ID)
		for i := 0; i < 20; i++ {
			suggestMakeResource(t, ctx, fmt.Sprint(i), &owner.ID, common)
		}
		peer := suggestMakeResource(t, ctx, "peer", nil, weak)
		suggestLinkSimilar(t, ctx, target.ID, peer.ID, 9)
		got, err := ctx.GetSuggestedTags(target.ID, 8)
		require.NoError(t, err)
		require.Equal(t, "common", got[0].Name)
		require.InDelta(t, .2*20/25, got[0].Score, 1e-12)
		require.InDelta(t, .5*.125/2.125, got[1].Score, 1e-12)
	})
	t.Run("unavailable distance", func(t *testing.T) {
		ctx := newSuggestTestContext(t)
		tag := suggestMakeTag(t, ctx, "legacy")
		target := suggestMakeResource(t, ctx, "target", nil)
		peer := suggestMakeResource(t, ctx, "peer", nil, tag)
		for _, id := range []uint{target.ID, peer.ID} {
			require.NoError(t, ctx.db.Create(&models.ImageHash{ResourceId: &id, DHash: "abcdef"}).Error)
		}
		got, err := ctx.GetSuggestedTags(target.ID, 8)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.InDelta(t, .5*.25/2.25, got[0].Score, 1e-12)
	})
}

func TestSuggestedTags_ExclusionBeforeBothCandidateLimits(t *testing.T) {
	ctx := newSuggestTestContext(t)
	owner := suggestMakeGroup(t, ctx, "owner", nil)
	var applied []*models.Tag
	for i := 0; i < 20; i++ {
		applied = append(applied, suggestMakeTag(t, ctx, fmt.Sprintf("applied%02d", i)))
	}
	next := suggestMakeTag(t, ctx, "zzz-next")
	target := suggestMakeResource(t, ctx, "target", &owner.ID, applied...)
	suggestMakeResource(t, ctx, "peer", &owner.ID, append(applied, next)...)
	got, err := ctx.GetSuggestedTags(target.ID, 8)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, next.ID, got[0].ID)
	require.Equal(t, []string{"cooccurrence", "group"}, got[0].Sources)
	// Target is excluded from both denominators, and sharing 20 seeds is one vote.
	require.InDelta(t, .3/6+.2/6, got[0].Score, 1e-12)
}

func TestSuggestedTags_CooccurrenceLocalThresholdAndAnySeed(t *testing.T) {
	for _, n := range []int{4, 5} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			ctx := newSuggestTestContext(t)
			owner := suggestMakeGroup(t, ctx, "owner", nil)
			seeds := []*models.Tag{suggestMakeTag(t, ctx, "seed-a"), suggestMakeTag(t, ctx, "seed-b")}
			local := suggestMakeTag(t, ctx, "local")
			global := suggestMakeTag(t, ctx, "global")
			target := suggestMakeResource(t, ctx, "target", &owner.ID, seeds...)
			for i := 0; i < n; i++ {
				suggestMakeResource(t, ctx, fmt.Sprint(i), &owner.ID, seeds[i%2], local)
			}
			// Ownerless peers are included in the unrestricted fallback, counted once.
			suggestMakeResource(t, ctx, "outside", nil, seeds[0], seeds[1], global)
			got, err := ctx.GetSuggestedTags(target.ID, 8)
			require.NoError(t, err)
			if n == 4 {
				s := suggestionByName(t, got, "global")
				require.InDelta(t, .3/10, s.Score, 1e-12)
			} else {
				require.False(t, suggestHas(got, "global"))
			}
			count := n
			if n == 4 {
				count++
			}
			require.InDelta(t, .3*float64(n)/float64(count+5)+.2*float64(n)/float64(n+5), suggestionByName(t, got, "local").Score, 1e-12)
		})
	}
}

func TestSuggestedTags_OwnerlessCooccurrenceAndSparseSmoothing(t *testing.T) {
	ctx := newSuggestTestContext(t)
	seed := suggestMakeTag(t, ctx, "seed")
	paired := suggestMakeTag(t, ctx, "paired")
	target := suggestMakeResource(t, ctx, "target", nil, seed)
	suggestMakeResource(t, ctx, "peer", nil, seed, paired)
	got, err := ctx.GetSuggestedTags(target.ID, 8)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []string{"cooccurrence"}, got[0].Sources)
	require.InDelta(t, .3/6, got[0].Score, 1e-12)
}

func TestSuggestedTags_CountsBeyondSourceCandidateLimit(t *testing.T) {
	for _, source := range []string{"group", "cooccurrence"} {
		t.Run(source, func(t *testing.T) {
			ctx := newSuggestTestContext(t)
			owner := suggestMakeGroup(t, ctx, "owner", nil)
			seed := suggestMakeTag(t, ctx, "seed")
			tail := suggestMakeTag(t, ctx, "zzz-tail")
			var hot []*models.Tag
			for i := 0; i < 21; i++ {
				hot = append(hot, suggestMakeTag(t, ctx, fmt.Sprintf("hot%02d", i)))
			}
			target := suggestMakeResource(t, ctx, "target", &owner.ID, seed)
			for i := 0; i < 5; i++ {
				suggestMakeResource(t, ctx, fmt.Sprint(i), &owner.ID, seed)
			}
			if source == "cooccurrence" {
				suggestMakeResource(t, ctx, "co-hot-1", &owner.ID, append(hot, seed, tail)...)
				suggestMakeResource(t, ctx, "co-hot-2", &owner.ID, append(hot, seed)...)
				for i := 0; i < 3; i++ {
					suggestMakeResource(t, ctx, fmt.Sprint("tail", i), &owner.ID, tail)
				}
			} else {
				for i := 0; i < 2; i++ {
					suggestMakeResource(t, ctx, fmt.Sprint("hot", i), &owner.ID, hot...)
				}
				suggestMakeResource(t, ctx, "co-tail", &owner.ID, seed, tail)
			}
			got, err := ctx.GetSuggestedTags(target.ID, 100)
			require.NoError(t, err)
			s := suggestionByName(t, got, "zzz-tail")
			require.Equal(t, []string{"cooccurrence", "group"}, s.Sources)
			if source == "cooccurrence" {
				require.InDelta(t, .3/12+.2*4/15, s.Score, 1e-12)
			} else {
				require.InDelta(t, .3/11+.2/13, s.Score, 1e-12)
			}
		})
	}
}

func TestSuggestedTags_ScopedEvidenceAndNeighborWindow(t *testing.T) {
	ctx := newSuggestTestContext(t)
	root := suggestMakeGroup(t, ctx, "root", nil)
	child := suggestMakeGroup(t, ctx, "child", &root.ID)
	outside := suggestMakeGroup(t, ctx, "outside", nil)
	seed := suggestMakeTag(t, ctx, "seed")
	inside := suggestMakeTag(t, ctx, "inside")
	secret := suggestMakeTag(t, ctx, "secret")
	target := suggestMakeResource(t, ctx, "target", &child.ID, seed)
	for i := 0; i < 51; i++ {
		r := suggestMakeResource(t, ctx, fmt.Sprint("outside", i), &outside.ID, seed, secret)
		suggestLinkSimilar(t, ctx, target.ID, r.ID, 0)
	}
	peer := suggestMakeResource(t, ctx, "visible", &root.ID, seed, inside)
	suggestLinkSimilar(t, ctx, target.ID, peer.ID, 3)
	scoped := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, ScopeGroupID: &root.ID})
	got, err := scoped.GetSuggestedTags(target.ID, 8)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, inside.ID, got[0].ID)
	require.Equal(t, []string{"similar", "cooccurrence"}, got[0].Sources)
	require.InDelta(t, .5*.5/2.5+.3/6, got[0].Score, 1e-12)
}

func TestSuggestedTags_AllSignalsAndStableDenominators(t *testing.T) {
	ctx := newSuggestTestContext(t)
	owner := suggestMakeGroup(t, ctx, "owner", nil)
	seed := suggestMakeTag(t, ctx, "seed")
	candidate := suggestMakeTag(t, ctx, "candidate")
	target := suggestMakeResource(t, ctx, "target", &owner.ID, seed)
	peer := suggestMakeResource(t, ctx, "peer", &owner.ID, seed, candidate)
	suggestLinkSimilar(t, ctx, target.ID, peer.ID, 0)
	got, err := ctx.GetSuggestedTags(target.ID, 8)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []string{"similar", "cooccurrence", "group"}, got[0].Sources)
	require.InDelta(t, .5/3+.3/6+.2/6, got[0].Score, 1e-12)
}

func TestSuggestedTags_SourceFailureDoesNotWidenOrLeak(t *testing.T) {
	ctx := newSuggestTestContext(t)
	owner := suggestMakeGroup(t, ctx, "owner", nil)
	seed := suggestMakeTag(t, ctx, "seed")
	outside := suggestMakeTag(t, ctx, "outside")
	target := suggestMakeResource(t, ctx, "target", &owner.ID, seed)
	suggestMakeResource(t, ctx, "peer", nil, seed, outside)
	var localFailures, fallbackQueries int
	require.NoError(t, ctx.db.Callback().Query().Before("gorm:query").Register("test:co-error", func(db *gorm.DB) {
		where := fmt.Sprint(db.Statement.Clauses["WHERE"])
		if !strings.Contains(where, "SELECT resource_id FROM resource_tags WHERE tag_id IN") {
			return
		}
		if strings.Contains(where, "resources.owner_id =") {
			localFailures++
			db.AddError(fmt.Errorf("local population unavailable"))
		} else {
			fallbackQueries++
		}
	}))
	got, err := ctx.GetSuggestedTags(target.ID, 8)
	require.NoError(t, err)
	require.Empty(t, got)
	require.Equal(t, 1, localFailures)
	require.Zero(t, fallbackQueries)
}

func TestSuggestedTags_CandidateTieDeterminism(t *testing.T) {
	ctx := newSuggestTestContext(t)
	owner := suggestMakeGroup(t, ctx, "owner", nil)
	target := suggestMakeResource(t, ctx, "target", &owner.ID)
	for i := 24; i >= 0; i-- {
		tag := suggestMakeTag(t, ctx, fmt.Sprintf("tag%02d", i))
		suggestMakeResource(t, ctx, tag.Name, &owner.ID, tag)
	}
	got, err := ctx.GetSuggestedTags(target.ID, 100)
	require.NoError(t, err)
	require.Len(t, got, 20)
	for i, s := range got {
		require.Equal(t, fmt.Sprintf("tag%02d", i), s.Name)
		require.False(t, math.IsNaN(s.Score))
	}
}

// Capture real queries so EXPLAIN inspects the production query shape, and the
// constant-query assertion catches accidental per-candidate lookups.
type suggestionQueryRecorder struct {
	logger.Interface
	queries []string
}

func (r *suggestionQueryRecorder) Trace(_ context.Context, _ time.Time, sql func() (string, int64), _ error) {
	query, _ := sql()
	r.queries = append(r.queries, query)
}

func TestSuggestedTags_LargePopulationQueryPlan(t *testing.T) {
	ctx := newSuggestTestContext(t)
	require.NoError(t, ctx.db.AutoMigrate(&models.LogEntry{}))
	require.NoError(t, models.EnsureSupplementalIndexes(ctx.db))
	seed := suggestMakeTag(t, ctx, "seed")
	paired := suggestMakeTag(t, ctx, "paired")
	background := suggestMakeTag(t, ctx, "background")
	target := suggestMakeResource(t, ctx, "target", nil, seed)
	require.NoError(t, ctx.db.Exec(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<10000)
  INSERT INTO resources (name, meta, own_meta) SELECT 'fixture-' || n, '{}', '{}' FROM seq`).Error)
	require.NoError(t, ctx.db.Exec("INSERT INTO resource_tags (resource_id, tag_id) SELECT id, ? FROM resources WHERE id <> ?", background.ID, target.ID).Error)
	for _, tagID := range []uint{seed.ID, paired.ID} {
		require.NoError(t, ctx.db.Exec("INSERT INTO resource_tags (resource_id, tag_id) SELECT id, ? FROM resources WHERE id <> ? ORDER BY id LIMIT 100", tagID, target.ID).Error)
	}
	require.NoError(t, ctx.db.Exec("ANALYZE").Error)
	recorder := &suggestionQueryRecorder{Interface: logger.Discard}
	ctx.db = ctx.db.Session(&gorm.Session{Logger: recorder})
	got, err := ctx.GetSuggestedTags(target.ID, 8)
	require.NoError(t, err)
	require.Len(t, got, 2)
	queryCount := len(recorder.queries)
	queries := append([]string(nil), recorder.queries...)
	var explained int
	for _, query := range queries {
		if !strings.Contains(query, "JOIN resource_tags st") {
			continue
		}
		var plan []struct{ Detail string }
		require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+query).Scan(&plan).Error)
		var details []string
		for _, row := range plan {
			details = append(details, row.Detail)
		}
		joined := strings.Join(details, "\n")
		require.Regexp(t, `SEARCH resource_tags USING (COVERING )?INDEX .*\(tag_id=\?\)`, joined)
		require.NotContains(t, joined, "SCAN resources")
		t.Logf("candidate query plan:\n%s", joined)
		explained++
	}
	require.Equal(t, 2, explained)
	// Growing the candidate set must not grow the number of database round trips.
	for i := 0; i < 40; i++ {
		tag := suggestMakeTag(t, ctx, fmt.Sprintf("extra%02d", i))
		require.NoError(t, ctx.db.Exec("INSERT INTO resource_tags (resource_id, tag_id) SELECT id, ? FROM resources WHERE id <> ? ORDER BY id LIMIT 100", tag.ID, target.ID).Error)
	}
	recorder.queries = nil
	got, err = ctx.GetSuggestedTags(target.ID, 8)
	require.NoError(t, err)
	require.Len(t, got, 8)
	require.Len(t, recorder.queries, queryCount)
	t.Logf("10000 resources, 100 matching peers: %d queries for both 2 and 20 candidates", queryCount)
}

func TestSuggestedTags_ManyDistinctTagsAvoidCrossProduct(t *testing.T) {
	ctx := newSuggestTestContext(t)
	seed := suggestMakeTag(t, ctx, "seed")
	target := suggestMakeResource(t, ctx, "target", nil, seed)
	require.NoError(t, ctx.db.Exec(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<20000)
  INSERT INTO tags (name) SELECT 'fixture-tag-' || n FROM seq`).Error)
	require.NoError(t, ctx.db.Exec(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<100000)
  INSERT INTO resources (name, meta, own_meta) SELECT 'fixture-' || n, '{}', '{}' FROM seq`).Error)
	require.NoError(t, ctx.db.Exec("INSERT INTO resource_tags (resource_id, tag_id) SELECT id, 2 + (id % 20000) FROM resources WHERE id <> ?", target.ID).Error)
	require.NoError(t, ctx.db.Exec("INSERT INTO resource_tags (resource_id, tag_id) SELECT id, ? FROM resources WHERE id <> ? ORDER BY id LIMIT 10000", seed.ID, target.ID).Error)
	require.NoError(t, ctx.db.Exec("ANALYZE").Error)
	recorder := &suggestionQueryRecorder{Interface: logger.Discard}
	ctx.db = ctx.db.Session(&gorm.Session{Logger: recorder})
	got, err := ctx.GetSuggestedTags(target.ID, 8)
	require.NoError(t, err)
	require.Len(t, got, 8)
	queries := append([]string(nil), recorder.queries...)
	var explained int
	for _, query := range queries {
		if !strings.Contains(query, "JOIN resource_tags st") {
			continue
		}
		var plan []struct{ Detail string }
		require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+query).Scan(&plan).Error)
		var details []string
		for _, row := range plan {
			details = append(details, row.Detail)
		}
		joined := strings.Join(details, "\n")
		require.NotRegexp(t, `SCAN t(?:\s|$)`, joined, "do not cross every tag with every matching resource")
		t.Logf("100000 resources, 20000 tags, 10000 seed matches:\n%s", joined)
		explained++
	}
	require.Equal(t, 2, explained)
}
