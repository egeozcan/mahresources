package api_handlers

import (
	"encoding/json"
	"net/http"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/server/http_utils"
)

// GetJobMigrationReadinessHandler exposes only fixed blocker codes and counts.
// The route is admin-only and remains available during the migration release,
// before the public Job Center cutover.
func GetJobMigrationReadinessHandler(ctx interface {
	GetJobMigrationReadiness() (application_context.JobMigrationReadiness, error)
}) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		report, err := ctx.GetJobMigrationReadiness()
		if err != nil {
			http.Error(writer, "Job migration readiness unavailable", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(report)
	}
}

func GetServerStatsHandler(ctx AdminStatsContext) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		stats, err := ctx.GetServerStats()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(stats)
	}
}

func GetDataStatsHandler(ctx AdminStatsContext) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		stats, err := ctx.GetDataStats()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(stats)
	}
}

func GetExpensiveStatsHandler(ctx AdminStatsContext) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		stats, err := ctx.GetExpensiveStats()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(stats)
	}
}

// GetRecomputeSimilaritiesHandler submits a background job to rebuild all v2
// similarity pairs from stored hashes (no image decode). Returns the job ID.
func GetRecomputeSimilaritiesHandler(ctx AdminStatsContext) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		jobID, err := ctx.RecomputeSimilarities()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusConflict)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{"jobId": jobID})
	}
}

// GetRetryFailedHashesHandler resets failed image_hashes rows so the backfill
// worker retries them. Returns the number of rows reset.
func GetRetryFailedHashesHandler(ctx AdminStatsContext) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		reset, err := ctx.RetryFailedHashes()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{"reset": reset})
	}
}
