package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"mahresources/auth"
	"mahresources/deferredtoken"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/mrql"
)

func TestMRQLSnapshotKeepsRandomMassEditTargets(t *testing.T) {
	f := newMassEditFixture(t, "snapshot-targets", 12)
	query := `type = resource AND name ~ $name ORDER BY RANDOM() LIMIT 4 OFFSET 1`
	params := map[string]any{"name": "snapshot-targets"}
	parsed, err := parseSnapshotQuery(query, params)
	require.NoError(t, err)
	result, err := f.ctx.ExecuteMRQLParsed(context.Background(), parsed, 0, 0)
	require.NoError(t, err)
	require.Len(t, result.Resources, 4)
	token, err := f.ctx.IssueMRQLSnapshot(context.Background(), query, params, result)
	require.NoError(t, err)
	var expected []uint
	for _, r := range result.Resources {
		expected = append(expected, r.ID)
	}
	for range 3 {
		resolved, err := f.ctx.ResolveMRQLSnapshot(context.Background(), query, params, token)
		require.NoError(t, err)
		var actual []uint
		for _, r := range resolved.Resources {
			actual = append(actual, r.ID)
		}
		require.Equal(t, expected, actual)
	}
	q := &query_models.MassEditQuery{Target: "mrql", MRQLQuery: query, MRQLParams: `{"name":"snapshot-targets"}`,
		MRQLSnapshot: token, TagsOp: "add", TagIds: []uint{f.tag.ID}, DryRun: true}
	probe, err := f.ctx.MassEditResources(q)
	require.NoError(t, err)
	require.EqualValues(t, 4, probe.Matched)
	q.DryRun, q.ExpectedCount = false, uip(4)
	_, err = f.ctx.MassEditResources(q)
	require.NoError(t, err)
	var tagged []uint
	require.NoError(t, f.ctx.db.Table("resource_tags").Where("tag_id = ?", f.tag.ID).Pluck("resource_id", &tagged).Error)
	require.ElementsMatch(t, expected, tagged)
	q.MRQLSnapshot = ""
	_, err = f.ctx.MassEditResources(q)
	require.ErrorIs(t, err, ErrInvalidMRQLSnapshot)
}

func TestMRQLSnapshotRejectsTamperingExpiryAndChangedBinding(t *testing.T) {
	f := newMassEditFixture(t, "snapshot-binding", 2)
	query := `type = resource AND name ~ $name ORDER BY RANDOM() LIMIT 2`
	params := map[string]any{"name": "snapshot-binding"}
	parsed, err := parseSnapshotQuery(query, params)
	require.NoError(t, err)
	result, err := f.ctx.ExecuteMRQLParsed(context.Background(), parsed, 0, 0)
	require.NoError(t, err)
	token, err := f.ctx.IssueMRQLSnapshot(context.Background(), query, params, result)
	require.NoError(t, err)
	for name, resolve := range map[string]func() error{
		"tampered": func() error {
			_, err := f.ctx.ResolveMRQLSnapshot(context.Background(), query, params, "x"+token[1:])
			return err
		},
		"query": func() error {
			_, err := f.ctx.ResolveMRQLSnapshot(context.Background(), query+" ", params, token)
			return err
		},
		"params": func() error {
			_, err := f.ctx.ResolveMRQLSnapshot(context.Background(), query, map[string]any{"name": "other"}, token)
			return err
		},
		"principal": func() error {
			other := f.ctx.WithPrincipal(&auth.Principal{UserID: 222, Role: models.RoleAdmin})
			_, err := other.ResolveMRQLSnapshot(context.Background(), query, params, token)
			return err
		},
		"scope": func() error {
			other := scopedMassEditContext(f.ctx, f.owner)
			_, err := other.ResolveMRQLSnapshot(context.Background(), query, params, token)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) { require.ErrorIs(t, resolve(), ErrInvalidMRQLSnapshot) })
	}
	typ, id, body, ok := deferredtoken.Open(f.ctx.mrqlSnapshotKey(), token)
	require.True(t, ok)
	var expired mrqlSnapshot
	require.NoError(t, json.Unmarshal([]byte(body), &expired))
	expired.Expires = time.Now().Add(-time.Second).Unix()
	raw, err := json.Marshal(expired)
	require.NoError(t, err)
	token = deferredtoken.Seal(f.ctx.mrqlSnapshotKey(), typ, id, string(raw))
	_, err = f.ctx.ResolveMRQLSnapshot(context.Background(), query, params, token)
	require.ErrorIs(t, err, ErrInvalidMRQLSnapshot)
}

func TestMRQLSnapshotRechecksLiveScopeAndCount(t *testing.T) {
	f := newMassEditFixture(t, "snapshot-live", 3)
	require.NoError(t, f.ctx.db.Model(&models.Resource{}).Where("id IN ?", []uint{f.res[0].ID, f.res[1].ID, f.res[2].ID}).Update("owner_id", f.owner.ID).Error)
	scoped := scopedMassEditContext(f.ctx, f.owner)
	query := `type = resource ORDER BY RANDOM() LIMIT 3`
	parsed, err := parseSnapshotQuery(query, nil)
	require.NoError(t, err)
	result, err := scoped.ExecuteMRQLParsed(context.Background(), parsed, 0, 0)
	require.NoError(t, err)
	token, err := scoped.IssueMRQLSnapshot(context.Background(), query, nil, result)
	require.NoError(t, err)
	require.NoError(t, f.ctx.db.Model(&models.Resource{}).Where("id = ?", f.res[0].ID).Update("owner_id", nil).Error)
	require.NoError(t, f.ctx.db.Delete(&models.Resource{}, f.res[1].ID).Error)
	resolved, err := scoped.ResolveMRQLSnapshot(context.Background(), query, nil, token)
	require.NoError(t, err)
	require.Len(t, resolved.Resources, 1)
	require.Equal(t, f.res[2].ID, resolved.Resources[0].ID)
	_, err = scoped.MassEditResources(&query_models.MassEditQuery{Target: "mrql", MRQLQuery: query, MRQLSnapshot: token,
		TagsOp: "add", TagIds: []uint{f.tag.ID}, ExpectedCount: uip(3)})
	require.ErrorIs(t, err, ErrMassEditSetChanged)
}

func TestMRQLMassEditContinuesDuplicateHeavyBuckets(t *testing.T) {
	ctx := createAdminTestContext(t, "mrql_duplicate_bucket_cap")
	notes := make([]models.Note, 1001)
	for i := range notes {
		notes[i].Name = fmt.Sprintf("duplicate-bucket-%04d", i)
	}
	require.NoError(t, ctx.db.CreateInBatches(&notes, 100).Error)
	tags := make([]models.Tag, 11)
	for i := range tags {
		tags[i].Name = fmt.Sprintf("duplicate-tag-%02d", i)
	}
	require.NoError(t, ctx.db.Create(&tags).Error)
	type join struct{ NoteId, TagId uint }
	var joins []join
	for _, n := range notes {
		for _, tag := range tags {
			joins = append(joins, join{n.ID, tag.ID})
		}
	}
	require.NoError(t, ctx.db.Table("note_tags").CreateInBatches(&joins, 100).Error)
	query := `type = note GROUP BY tags ORDER BY name ASC LIMIT 1001`
	parsed, err := mrql.Parse(query)
	require.NoError(t, err)
	parsed.EntityType = mrql.EntityNote
	firstPage, err := ctx.ExecuteMRQLGrouped(context.Background(), parsed)
	require.NoError(t, err)
	require.NotEmpty(t, firstPage.Warnings)
	require.NotNil(t, firstPage.NextOffset)
	result, err := ctx.MassEditNotes(&query_models.MassEditQuery{Target: "mrql", MRQLQuery: query,
		TagsOp: "add", TagIds: []uint{tags[0].ID}, DryRun: true})
	require.NoError(t, err)
	require.EqualValues(t, 1001, result.Matched)
}
