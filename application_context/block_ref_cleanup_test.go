package application_context

import (
	"encoding/json"
	"testing"
)

func TestScrubResourceFromBlockContent_Gallery(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		resourceID  uint
		wantChanged bool
		wantIDs     []uint
	}{
		{
			name:        "removes target ID from middle",
			content:     `{"resourceIds":[1,2,3]}`,
			resourceID:  2,
			wantChanged: true,
			wantIDs:     []uint{1, 3},
		},
		{
			name:        "removes target ID from start",
			content:     `{"resourceIds":[1,2,3]}`,
			resourceID:  1,
			wantChanged: true,
			wantIDs:     []uint{2, 3},
		},
		{
			name:        "removes target ID from end",
			content:     `{"resourceIds":[1,2,3]}`,
			resourceID:  3,
			wantChanged: true,
			wantIDs:     []uint{1, 2},
		},
		{
			name:        "no change when ID not present",
			content:     `{"resourceIds":[1,3]}`,
			resourceID:  2,
			wantChanged: false,
			wantIDs:     []uint{1, 3},
		},
		{
			name:        "empties array when only ID present",
			content:     `{"resourceIds":[5]}`,
			resourceID:  5,
			wantChanged: true,
			wantIDs:     []uint{},
		},
		{
			name:        "no resourceIds field",
			content:     `{"other":"data"}`,
			resourceID:  1,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := scrubResourceFromBlockContent(tt.content, tt.resourceID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}
			if !tt.wantChanged {
				return
			}
			// Verify the IDs in the output
			var raw map[string]any
			if err := json.Unmarshal([]byte(got), &raw); err != nil {
				t.Fatalf("result is not valid JSON: %v", err)
			}
			ids, _ := raw["resourceIds"].([]any)
			if len(ids) != len(tt.wantIDs) {
				t.Errorf("resourceIds length = %d, want %d (got: %v)", len(ids), len(tt.wantIDs), ids)
				return
			}
			for i, want := range tt.wantIDs {
				if toUint(ids[i]) != want {
					t.Errorf("resourceIds[%d] = %v, want %d", i, ids[i], want)
				}
			}
		})
	}
}

func TestScrubResourceFromBlockContent_Calendar(t *testing.T) {
	tests := []struct {
		name              string
		content           string
		resourceID        uint
		wantChanged       bool
		wantResourceIDNil bool // true if the resourceId key should be absent after scrub
	}{
		{
			name:              "removes resourceId from calendar source",
			content:           `{"calendars":[{"id":"c1","name":"c1","color":"#ff0000","source":{"type":"resource","resourceId":5}}]}`,
			resourceID:        5,
			wantChanged:       true,
			wantResourceIDNil: true,
		},
		{
			name:        "no change when resourceId differs",
			content:     `{"calendars":[{"id":"c1","name":"c1","color":"#ff0000","source":{"type":"resource","resourceId":5}}]}`,
			resourceID:  99,
			wantChanged: false,
		},
		{
			name:        "no change when source is url type",
			content:     `{"calendars":[{"id":"c1","name":"c1","color":"#ff0000","source":{"type":"url","url":"https://example.com"}}]}`,
			resourceID:  5,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := scrubResourceFromBlockContent(tt.content, tt.resourceID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}
			if !tt.wantChanged {
				return
			}
			var raw map[string]any
			if err := json.Unmarshal([]byte(got), &raw); err != nil {
				t.Fatalf("result is not valid JSON: %v", err)
			}
			cals, _ := raw["calendars"].([]any)
			if len(cals) == 0 {
				t.Fatal("calendars array must not be empty after scrub")
			}
			cal0 := cals[0].(map[string]any)
			source := cal0["source"].(map[string]any)
			_, hasResID := source["resourceId"]
			if tt.wantResourceIDNil && hasResID {
				t.Errorf("resourceId should be absent after scrub but is still present: %v", source)
			}
		})
	}
}

func TestScrubGroupFromBlockContent(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		groupID     uint
		wantChanged bool
		wantIDs     []uint
	}{
		{
			name:        "removes target group ID",
			content:     `{"groupIds":[10,20,30]}`,
			groupID:     20,
			wantChanged: true,
			wantIDs:     []uint{10, 30},
		},
		{
			name:        "no change when ID not present",
			content:     `{"groupIds":[10,30]}`,
			groupID:     20,
			wantChanged: false,
		},
		{
			name:        "no groupIds field",
			content:     `{"other":"data"}`,
			groupID:     1,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := scrubGroupFromBlockContent(tt.content, tt.groupID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}
			if !tt.wantChanged {
				return
			}
			var raw map[string]any
			if err := json.Unmarshal([]byte(got), &raw); err != nil {
				t.Fatalf("result is not valid JSON: %v", err)
			}
			ids, _ := raw["groupIds"].([]any)
			if len(ids) != len(tt.wantIDs) {
				t.Errorf("groupIds length = %d, want %d", len(ids), len(tt.wantIDs))
				return
			}
			for i, want := range tt.wantIDs {
				if toUint(ids[i]) != want {
					t.Errorf("groupIds[%d] = %v, want %d", i, ids[i], want)
				}
			}
		})
	}
}

func TestScrubQueryFromBlockContent(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		queryID     uint
		wantChanged bool
	}{
		{
			name:        "removes matching queryId",
			content:     `{"queryId":7}`,
			queryID:     7,
			wantChanged: true,
		},
		{
			name:        "no change when queryId differs",
			content:     `{"queryId":7}`,
			queryID:     99,
			wantChanged: false,
		},
		{
			name:        "no change when no queryId",
			content:     `{"columns":[],"rows":[]}`,
			queryID:     7,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := scrubQueryFromBlockContent(tt.content, tt.queryID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}
			if !tt.wantChanged {
				return
			}
			var raw map[string]any
			if err := json.Unmarshal([]byte(got), &raw); err != nil {
				t.Fatalf("result is not valid JSON: %v", err)
			}
			if _, ok := raw["queryId"]; ok {
				t.Errorf("queryId should be absent after scrub")
			}
		})
	}
}
