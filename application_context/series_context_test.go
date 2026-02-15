package application_context

import (
	"encoding/json"
	"mahresources/models/types"
	"testing"
)

func TestMergeMeta(t *testing.T) {
	tests := []struct {
		name    string
		base    types.JSON
		overlay types.JSON
		want    map[string]any
	}{
		{
			name:    "overlay wins on conflict",
			base:    types.JSON(`{"a":1,"b":2}`),
			overlay: types.JSON(`{"b":3,"c":4}`),
			want:    map[string]any{"a": float64(1), "b": float64(3), "c": float64(4)},
		},
		{
			name:    "empty overlay returns base",
			base:    types.JSON(`{"a":1}`),
			overlay: types.JSON(`{}`),
			want:    map[string]any{"a": float64(1)},
		},
		{
			name:    "empty base returns overlay",
			base:    types.JSON(`{}`),
			overlay: types.JSON(`{"a":1}`),
			want:    map[string]any{"a": float64(1)},
		},
		{
			name:    "null base returns overlay",
			base:    types.JSON("null"),
			overlay: types.JSON(`{"a":1}`),
			want:    map[string]any{"a": float64(1)},
		},
		{
			name:    "null overlay returns base",
			base:    types.JSON(`{"a":1}`),
			overlay: types.JSON("null"),
			want:    map[string]any{"a": float64(1)},
		},
		{
			name:    "both empty returns empty",
			base:    types.JSON(`{}`),
			overlay: types.JSON(`{}`),
			want:    map[string]any{},
		},
		{
			name:    "leave series scenario - resource values win",
			base:    types.JSON(`{"b":3,"c":4}`),
			overlay: types.JSON(`{"a":1,"b":2}`),
			want:    map[string]any{"a": float64(1), "b": float64(2), "c": float64(4)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeMeta(tt.base, tt.overlay)
			if err != nil {
				t.Fatalf("mergeMeta() error = %v", err)
			}

			var gotMap map[string]any
			if err := json.Unmarshal(got, &gotMap); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			if len(gotMap) != len(tt.want) {
				t.Errorf("mergeMeta() got %d keys, want %d keys", len(gotMap), len(tt.want))
			}

			for k, wantV := range tt.want {
				gotV, ok := gotMap[k]
				if !ok {
					t.Errorf("mergeMeta() missing key %q", k)
					continue
				}
				if gotV != wantV {
					t.Errorf("mergeMeta() key %q = %v, want %v", k, gotV, wantV)
				}
			}
		})
	}
}

func TestComputeOwnMeta(t *testing.T) {
	tests := []struct {
		name         string
		resourceMeta types.JSON
		seriesMeta   types.JSON
		want         map[string]any
	}{
		{
			name:         "strips common keys",
			resourceMeta: types.JSON(`{"a":1,"b":2,"c":3}`),
			seriesMeta:   types.JSON(`{"b":2,"c":5}`),
			want:         map[string]any{"a": float64(1), "c": float64(3)},
		},
		{
			name:         "all keys match series - empty own meta",
			resourceMeta: types.JSON(`{"a":1,"b":2}`),
			seriesMeta:   types.JSON(`{"a":1,"b":2}`),
			want:         map[string]any{},
		},
		{
			name:         "no overlap - all keys are own",
			resourceMeta: types.JSON(`{"x":1,"y":2}`),
			seriesMeta:   types.JSON(`{"a":1,"b":2}`),
			want:         map[string]any{"x": float64(1), "y": float64(2)},
		},
		{
			name:         "empty resource meta",
			resourceMeta: types.JSON(`{}`),
			seriesMeta:   types.JSON(`{"a":1}`),
			want:         map[string]any{},
		},
		{
			name:         "empty series meta - all keys are own",
			resourceMeta: types.JSON(`{"a":1,"b":2}`),
			seriesMeta:   types.JSON(`{}`),
			want:         map[string]any{"a": float64(1), "b": float64(2)},
		},
		{
			name:         "string values compared correctly",
			resourceMeta: types.JSON(`{"title":"hello","author":"me"}`),
			seriesMeta:   types.JSON(`{"title":"hello","author":"you"}`),
			want:         map[string]any{"author": "me"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := computeOwnMeta(tt.resourceMeta, tt.seriesMeta)
			if err != nil {
				t.Fatalf("computeOwnMeta() error = %v", err)
			}

			var gotMap map[string]any
			if err := json.Unmarshal(got, &gotMap); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			if len(gotMap) != len(tt.want) {
				t.Errorf("computeOwnMeta() got %d keys, want %d keys\ngot:  %v\nwant: %v", len(gotMap), len(tt.want), gotMap, tt.want)
			}

			for k, wantV := range tt.want {
				gotV, ok := gotMap[k]
				if !ok {
					t.Errorf("computeOwnMeta() missing key %q", k)
					continue
				}
				if gotV != wantV {
					t.Errorf("computeOwnMeta() key %q = %v, want %v", k, gotV, wantV)
				}
			}
		})
	}
}
