package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scores depend on tag names, so renaming a tag must drop cached resource
// results; otherwise the old (tag-less) ranking is served until the TTL.
func TestSearchTagRenameInvalidatesCachedResourceScores(t *testing.T) {
	tc := SetupTestEnv(t)

	sqlDB, err := tc.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	const term = "zztagrenameterm"
	tag := &models.Tag{Name: "placeholder"}
	require.NoError(t, tc.DB.Create(tag).Error)
	res := &models.Resource{
		Name: "plain", Description: "mentions " + term, Hash: "ccc333",
		HashType: "SHA1", Location: "/test/c.jpg",
	}
	require.NoError(t, tc.DB.Create(res).Error)
	require.NoError(t, tc.DB.Model(res).Association("Tags").Append(tag))

	scoreOf := func() int {
		out, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{Query: term, Limit: 50})
		require.NoError(t, err)
		for _, r := range out.Results {
			if r.Type == "resource" && r.ID == res.ID {
				return r.Score
			}
		}
		t.Fatalf("resource not found in results for %q", term)
		return 0
	}

	require.Equal(t, 40, scoreOf(), "description-only match before the rename")

	_, err = tc.AppCtx.UpdateTag(&query_models.TagCreator{ID: tag.ID, Name: term})
	require.NoError(t, err)

	assert.Equal(t, 90, scoreOf(), "exact tag match after the rename, not the cached score")
}

// Adding or removing a tag on an entity changes that entity's score just as a
// rename does, so every association path must drop the cached results too.
func TestSearchTagAssociationChangesInvalidateCachedScores(t *testing.T) {
	type target struct {
		kind   string
		create func(tc *TestContext, term string) uint
	}
	targets := []target{
		{"resource", func(tc *TestContext, term string) uint {
			r := &models.Resource{Name: "plain", Description: "mentions " + term, Hash: "assoc-" + term, HashType: "SHA1", Location: "/test/" + term + ".jpg"}
			require.NoError(t, tc.DB.Create(r).Error)
			return r.ID
		}},
		{"note", func(tc *TestContext, term string) uint {
			n := &models.Note{Name: "plain", Description: "mentions " + term}
			require.NoError(t, tc.DB.Create(n).Error)
			return n.ID
		}},
		{"group", func(tc *TestContext, term string) uint {
			g := &models.Group{Name: "plain", Description: "mentions " + term}
			require.NoError(t, tc.DB.Create(g).Error)
			return g.ID
		}},
	}
	bulk := func(ids []uint, tagID uint) *query_models.BulkEditQuery {
		return &query_models.BulkEditQuery{BulkQuery: query_models.BulkQuery{ID: ids}, EditedId: []uint{tagID}}
	}
	ops := map[string]struct {
		add, remove func(tc *TestContext, id, tagID uint) error
	}{
		"resource": {
			add: func(tc *TestContext, id, tagID uint) error {
				return tc.AppCtx.BulkAddTagsToResources(bulk([]uint{id}, tagID))
			},
			remove: func(tc *TestContext, id, tagID uint) error {
				return tc.AppCtx.BulkRemoveTagsFromResources(bulk([]uint{id}, tagID))
			},
		},
		"note": {
			add: func(tc *TestContext, id, tagID uint) error {
				return tc.AppCtx.BulkAddTagsToNotes(bulk([]uint{id}, tagID))
			},
			remove: func(tc *TestContext, id, tagID uint) error {
				return tc.AppCtx.BulkRemoveTagsFromNotes(bulk([]uint{id}, tagID))
			},
		},
		"group": {
			add: func(tc *TestContext, id, tagID uint) error {
				return tc.AppCtx.BulkAddTagsToGroups(bulk([]uint{id}, tagID))
			},
			remove: func(tc *TestContext, id, tagID uint) error {
				return tc.AppCtx.BulkRemoveTagsFromGroups(bulk([]uint{id}, tagID))
			},
		},
	}
	extra := map[string]struct {
		add, remove func(tc *TestContext, id, tagID uint) error
	}{
		"resource replace": {
			add: func(tc *TestContext, id, tagID uint) error {
				return tc.AppCtx.BulkReplaceTagsFromResources(bulk([]uint{id}, tagID))
			},
			remove: func(tc *TestContext, id, tagID uint) error {
				return tc.AppCtx.BulkReplaceTagsFromResources(&query_models.BulkEditQuery{BulkQuery: query_models.BulkQuery{ID: []uint{id}}})
			},
		},
		"note single": {
			add:    func(tc *TestContext, id, tagID uint) error { return tc.AppCtx.AddTagsToNote(id, []uint{tagID}) },
			remove: func(tc *TestContext, id, tagID uint) error { return tc.AppCtx.RemoveTagsFromNote(id, []uint{tagID}) },
		},
	}
	kindOf := map[string]string{"resource replace": "resource", "note single": "note"}
	for name, op := range extra {
		ops[name] = op
	}

	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			tc := SetupTestEnv(t)
			sqlDB, err := tc.DB.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)

			kind := name
			if k, ok := kindOf[name]; ok {
				kind = k
			}
			term := "zzassoc" + strings.ReplaceAll(name, " ", "")
			var create func(tc *TestContext, term string) uint
			for _, tg := range targets {
				if tg.kind == kind {
					create = tg.create
				}
			}
			id := create(tc, term)
			tag := &models.Tag{Name: term}
			require.NoError(t, tc.DB.Create(tag).Error)

			scoreOf := func() int {
				out, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{Query: term, Limit: 50})
				require.NoError(t, err)
				for _, r := range out.Results {
					if r.Type == kind && r.ID == id {
						return r.Score
					}
				}
				t.Fatalf("%s %d not found in results for %q", kind, id, term)
				return 0
			}

			untagged := scoreOf()
			require.NoError(t, op.add(tc, id, tag.ID))
			tagged := scoreOf()
			assert.Greater(t, tagged, untagged, "adding a matching tag must not serve the cached score")
			require.NoError(t, op.remove(tc, id, tag.ID))
			assert.Equal(t, untagged, scoreOf(), "removing the tag must not serve the cached score")
		})
	}
}
