package api_handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

const (
	defaultTimelineGranularity = "monthly"
	defaultTimelineColumns     = 15
	maxTimelineColumns         = 60
)

// parseTimelineParams extracts the timeline-specific query parameters from
// the request: granularity (default "monthly"), anchor (default today),
// and columns (default 15, max 60).
func parseTimelineParams(request *http.Request) (string, time.Time, int) {
	granularity := request.URL.Query().Get("granularity")
	switch granularity {
	case "yearly", "monthly", "weekly":
		// valid
	default:
		granularity = defaultTimelineGranularity
	}

	anchor := time.Now().UTC()
	if anchorStr := request.URL.Query().Get("anchor"); anchorStr != "" {
		if parsed, err := time.Parse("2006-01-02", anchorStr); err == nil {
			anchor = parsed
		}
	}

	columns := defaultTimelineColumns
	if colVal := request.URL.Query().Get("columns"); colVal != "" {
		if c, err := strconv.Atoi(colVal); err == nil && c > 0 && c <= maxTimelineColumns {
			columns = c
		}
	}

	return granularity, anchor, columns
}

// bucketContainsToday reports whether the given bucket boundary contains
// the current date (in UTC).
func bucketContainsToday(b application_context.BucketBoundary) bool {
	now := time.Now().UTC()
	return !now.Before(b.Start) && now.Before(b.End)
}

// computeHasMore determines the hasMore flags for a timeline response.
// hasMore.left is always true (there is always older data to scroll to).
// hasMore.right is false when the rightmost bucket contains today.
func computeHasMore(boundaries []application_context.BucketBoundary) models.TimelineHasMore {
	hasMore := models.TimelineHasMore{
		Left:  true,
		Right: true,
	}
	if len(boundaries) > 0 {
		rightmost := boundaries[len(boundaries)-1]
		if bucketContainsToday(rightmost) {
			hasMore.Right = false
		}
	}
	return hasMore
}

// GetResourceTimelineHandler returns an HTTP handler that produces timeline
// bucket counts for resources, filtered by the standard resource query params.
func GetResourceTimelineHandler(ctx *application_context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.ResourceSearchQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		granularity, anchor, columns := parseTimelineParams(request)
		boundaries := application_context.GenerateBucketBoundaries(granularity, anchor, columns)

		buckets, err := ctx.GetResourceTimelineCounts(&query, boundaries)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(models.TimelineResponse{
			Buckets: buckets,
			HasMore: computeHasMore(boundaries),
		})
	}
}

// GetNoteTimelineHandler returns an HTTP handler that produces timeline
// bucket counts for notes, filtered by the standard note query params.
func GetNoteTimelineHandler(ctx *application_context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.NoteQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		granularity, anchor, columns := parseTimelineParams(request)
		boundaries := application_context.GenerateBucketBoundaries(granularity, anchor, columns)

		buckets, err := ctx.GetNoteTimelineCounts(&query, boundaries)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(models.TimelineResponse{
			Buckets: buckets,
			HasMore: computeHasMore(boundaries),
		})
	}
}

// GetGroupTimelineHandler returns an HTTP handler that produces timeline
// bucket counts for groups, filtered by the standard group query params.
func GetGroupTimelineHandler(ctx *application_context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.GroupQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		granularity, anchor, columns := parseTimelineParams(request)
		boundaries := application_context.GenerateBucketBoundaries(granularity, anchor, columns)

		buckets, err := ctx.GetGroupTimelineCounts(&query, boundaries)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(models.TimelineResponse{
			Buckets: buckets,
			HasMore: computeHasMore(boundaries),
		})
	}
}

// GetTagTimelineHandler returns an HTTP handler that produces timeline
// bucket counts for tags, filtered by the standard tag query params.
func GetTagTimelineHandler(ctx *application_context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.TagQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		granularity, anchor, columns := parseTimelineParams(request)
		boundaries := application_context.GenerateBucketBoundaries(granularity, anchor, columns)

		buckets, err := ctx.GetTagTimelineCounts(&query, boundaries)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(models.TimelineResponse{
			Buckets: buckets,
			HasMore: computeHasMore(boundaries),
		})
	}
}

// GetCategoryTimelineHandler returns an HTTP handler that produces timeline
// bucket counts for categories, filtered by the standard category query params.
func GetCategoryTimelineHandler(ctx *application_context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.CategoryQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		granularity, anchor, columns := parseTimelineParams(request)
		boundaries := application_context.GenerateBucketBoundaries(granularity, anchor, columns)

		buckets, err := ctx.GetCategoryTimelineCounts(&query, boundaries)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(models.TimelineResponse{
			Buckets: buckets,
			HasMore: computeHasMore(boundaries),
		})
	}
}

// GetQueryTimelineHandler returns an HTTP handler that produces timeline
// bucket counts for queries, filtered by the standard query query params.
func GetQueryTimelineHandler(ctx *application_context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.QueryQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		granularity, anchor, columns := parseTimelineParams(request)
		boundaries := application_context.GenerateBucketBoundaries(granularity, anchor, columns)

		buckets, err := ctx.GetQueryTimelineCounts(&query, boundaries)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(models.TimelineResponse{
			Buckets: buckets,
			HasMore: computeHasMore(boundaries),
		})
	}
}
