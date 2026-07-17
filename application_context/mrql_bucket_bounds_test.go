package application_context

import (
	"context"
	"fmt"
	"testing"

	"mahresources/models"
	"mahresources/mrql"
)

func TestBucketedMRQLBoundsQueryFanoutWithContinuation(t *testing.T) {
	ctx, db := setupMRQLRenderDataTest(t)
	if err := db.Create(&models.ResourceCategory{ID: 1, Name: "default"}).Error; err != nil {
		t.Fatal(err)
	}
	resources := make([]models.Resource, maxBucketQueries+1)
	for i := range resources {
		resources[i] = models.Resource{Name: fmt.Sprintf("bucket-%03d", i), ResourceCategoryId: 1}
	}
	if err := db.CreateInBatches(&resources, 100).Error; err != nil {
		t.Fatal(err)
	}
	q, err := mrql.Parse(`type = "resource" GROUP BY name LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	if err := mrql.Validate(q); err != nil {
		t.Fatal(err)
	}
	q.EntityType = mrql.EntityResource
	result, err := ctx.ExecuteMRQLGrouped(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != maxBucketQueries {
		t.Fatalf("groups=%d, want bounded %d", len(result.Groups), maxBucketQueries)
	}
	if result.NextOffset == nil || *result.NextOffset != maxBucketQueries {
		t.Fatalf("nextOffset=%v, want %d", result.NextOffset, maxBucketQueries)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected bucket fan-out warning")
	}
}
