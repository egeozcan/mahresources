package application_context

import (
	"encoding/json"
	"testing"
)

func TestImportPlan_RoundTripJSON(t *testing.T) {
	plan := &ImportPlan{
		JobID:            "abc123",
		SchemaVersion:    1,
		SourceInstanceID: "src-1",
		Counts: ImportPlanCounts{
			Groups: 2, Notes: 3, Resources: 5, Series: 1,
		},
		Items: []ImportPlanItem{
			{ExportID: "g0001", Kind: "group", Name: "Books", OwnerRef: "", Children: []ImportPlanItem{
				{ExportID: "g0002", Kind: "group", Name: "Fiction", OwnerRef: "g0001",
					ResourceCount: 3, NoteCount: 1,
					DescendantResourceCount: 3, DescendantNoteCount: 1},
			}, ResourceCount: 2, NoteCount: 0, DescendantResourceCount: 5, DescendantNoteCount: 1},
		},
		Mappings: ImportMappings{
			Categories: []MappingEntry{
				{DecisionKey: "category:c0001", SourceKey: "Books", SourceExportID: "c0001", HasPayload: true, Suggestion: "map", DestinationID: uintPtr(3)},
			},
			GroupRelationTypes: []MappingEntry{
				{DecisionKey: "grt:DerivedFrom|Books|Archive", SourceKey: "DerivedFrom", SourceExportID: "grt0001", HasPayload: true,
					FromCategoryName: "Books", ToCategoryName: "Archive",
					Suggestion: "create"},
			},
		},
		SeriesInfo: []SeriesMapping{
			{ExportID: "s0001", Name: "Volumes", Slug: "volumes", Action: "reuse_existing", DestID: uintPtr(7), DestName: "Volumes"},
		},
		DanglingRefs: []DanglingRefPlan{
			{ID: "dr0001", Kind: "related_group", FromExportID: "g0001", FromName: "Books",
				StubSourceID: 88, StubName: "Archive"},
		},
		Conflicts: ConflictSummary{
			ResourceHashMatches: 2,
		},
		ManifestOnlyMissingHashes: 0,
		Warnings:                  []string{"test warning"},
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ImportPlan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.JobID != plan.JobID {
		t.Errorf("JobID = %q, want %q", decoded.JobID, plan.JobID)
	}
	if len(decoded.Items) != 1 || len(decoded.Items[0].Children) != 1 {
		t.Errorf("item tree not preserved")
	}
	if decoded.Items[0].DescendantResourceCount != 5 {
		t.Errorf("rolled-up descendant resource count = %d, want 5", decoded.Items[0].DescendantResourceCount)
	}
	if decoded.Items[0].Children[0].OwnerRef != "g0001" {
		t.Errorf("child OwnerRef = %q, want 'g0001'", decoded.Items[0].Children[0].OwnerRef)
	}
	if len(decoded.Mappings.Categories) != 1 || decoded.Mappings.Categories[0].Suggestion != "map" {
		t.Errorf("category mapping not preserved")
	}
	if len(decoded.Mappings.GroupRelationTypes) != 1 {
		t.Fatalf("expected 1 GRT mapping, got %d", len(decoded.Mappings.GroupRelationTypes))
	}
	grt := decoded.Mappings.GroupRelationTypes[0]
	if grt.FromCategoryName != "Books" || grt.ToCategoryName != "Archive" {
		t.Errorf("GRT composite key not preserved: from=%q to=%q", grt.FromCategoryName, grt.ToCategoryName)
	}
	if grt.DecisionKey != "grt:DerivedFrom|Books|Archive" {
		t.Errorf("GRT decision key = %q, want 'grt:DerivedFrom|Books|Archive'", grt.DecisionKey)
	}
	if len(decoded.SeriesInfo) != 1 || decoded.SeriesInfo[0].Action != "reuse_existing" {
		t.Errorf("series info not preserved")
	}
}

func TestImportDecisions_RoundTripJSON(t *testing.T) {
	decisions := &ImportDecisions{
		ParentGroupID:          uintPtr(42),
		ResourceCollisionPolicy: "skip",
		AcknowledgeMissingHashes: true,
		MappingActions: map[string]MappingAction{
			"category:c0001": {Include: true, Action: "map", DestinationID: uintPtr(3)},
			"tag:t0001":      {Include: true, Action: "create"},
		},
		DanglingActions: map[string]DanglingAction{
			"dr0001": {Action: "drop"},
			"dr0002": {Action: "map", DestinationID: uintPtr(88)},
		},
		ExcludedItems: []string{"g0002"},
	}

	data, err := json.Marshal(decisions)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ImportDecisions
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if *decoded.ParentGroupID != 42 {
		t.Errorf("ParentGroupID = %d, want 42", *decoded.ParentGroupID)
	}
	if decoded.ResourceCollisionPolicy != "skip" {
		t.Errorf("policy = %q, want 'skip'", decoded.ResourceCollisionPolicy)
	}
	if len(decoded.MappingActions) != 2 {
		t.Errorf("mapping actions count = %d, want 2", len(decoded.MappingActions))
	}
	catAction := decoded.MappingActions["category:c0001"]
	if !catAction.Include || catAction.Action != "map" {
		t.Errorf("category action = %+v, want include=true action=map", catAction)
	}
	if len(decoded.ExcludedItems) != 1 || decoded.ExcludedItems[0] != "g0002" {
		t.Errorf("excluded items = %v, want [g0002]", decoded.ExcludedItems)
	}
}

func TestDecisionKeyFor(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		entry    MappingEntry
		want     string
	}{
		{
			name:     "category with export ID",
			typeName: "category",
			entry:    MappingEntry{SourceExportID: "c0001", SourceKey: "Books"},
			want:     "category:c0001",
		},
		{
			name:     "tag without export ID",
			typeName: "tag",
			entry:    MappingEntry{SourceKey: "MyTag"},
			want:     "tag:MyTag",
		},
		{
			name:     "GRT composite key",
			typeName: "grt",
			entry:    MappingEntry{SourceKey: "DerivedFrom", FromCategoryName: "Books", ToCategoryName: "Archive"},
			want:     "grt:DerivedFrom|Books|Archive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecisionKeyFor(tt.typeName, tt.entry)
			if got != tt.want {
				t.Errorf("DecisionKeyFor(%q, ...) = %q, want %q", tt.typeName, got, tt.want)
			}
		})
	}
}

func TestValidateForApply_ShellGroupMapWithoutDest(t *testing.T) {
	plan := &ImportPlan{}
	decisions := &ImportDecisions{
		ResourceCollisionPolicy: "skip",
		MappingActions:          map[string]MappingAction{},
		DanglingActions:         map[string]DanglingAction{},
		ShellGroupActions: map[string]ShellGroupAction{
			"g0005": {Action: "map_to_existing", DestinationID: nil},
		},
	}
	err := plan.ValidateForApply(decisions)
	if err == nil {
		t.Fatal("expected validation error for map_to_existing without destination_id")
	}
}

func uintPtr(v uint) *uint { return &v }
