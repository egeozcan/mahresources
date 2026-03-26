package api_handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
)

// versionErrorStatus maps a version operation error to the appropriate HTTP status code.
// "not found" errors become 404, ownership/constraint violations become 400 or 409,
// and truly unexpected failures remain 500.
func versionErrorStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		return http.StatusNotFound
	case strings.Contains(msg, "does not belong"):
		return http.StatusBadRequest
	case strings.Contains(msg, "cannot delete current version"),
		strings.Contains(msg, "cannot delete last version"):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// GetListVersionsHandler returns handler for listing versions
func GetListVersionsHandler(ctx interfaces.VersionReader) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID, err := strconv.ParseUint(r.URL.Query().Get("resourceId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid resourceId"), w, r, http.StatusBadRequest)
			return
		}

		versions, err := ctx.GetVersions(uint(resourceID))
		if err != nil {
			http_utils.HandleError(err, w, r, versionErrorStatus(err))
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(versions)
	}
}

// GetVersionHandler returns handler for getting a single version
func GetVersionHandler(ctx interfaces.VersionReader) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid version id"), w, r, http.StatusBadRequest)
			return
		}

		version, err := ctx.GetVersion(uint(versionID))
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(version)
	}
}

// GetUploadVersionHandler returns handler for uploading a new version
func GetUploadVersionHandler(ctx interfaces.VersionWriter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID, err := strconv.ParseUint(r.URL.Query().Get("resourceId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid resourceId"), w, r, http.StatusBadRequest)
			return
		}

		if err := r.ParseMultipartForm(100 << 20); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http_utils.HandleError(fmt.Errorf("file required"), w, r, http.StatusBadRequest)
			return
		}
		defer file.Close()

		comment := r.FormValue("comment")

		version, err := ctx.UploadNewVersion(uint(resourceID), file, header, comment)
		if err != nil {
			http_utils.HandleError(err, w, r, versionErrorStatus(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(w, r, fmt.Sprintf("/resource?id=%v", resourceID)) {
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(version)
	}
}

// GetRestoreVersionHandler returns handler for restoring a version
func GetRestoreVersionHandler(ctx interfaces.VersionWriter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var query query_models.VersionRestoreQuery
		if err := tryFillStructValuesFromRequest(&query, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		version, err := ctx.RestoreVersion(query.ResourceID, query.VersionID, query.Comment)
		if err != nil {
			http_utils.HandleError(err, w, r, versionErrorStatus(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(w, r, fmt.Sprintf("/resource?id=%v", query.ResourceID)) {
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(version)
	}
}

// GetDeleteVersionHandler returns handler for deleting a version
func GetDeleteVersionHandler(ctx interfaces.VersionDeleter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID, err := strconv.ParseUint(r.URL.Query().Get("resourceId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid resourceId"), w, r, http.StatusBadRequest)
			return
		}

		versionID, err := strconv.ParseUint(r.URL.Query().Get("versionId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid versionId"), w, r, http.StatusBadRequest)
			return
		}

		if resourceID == 0 || versionID == 0 {
			http_utils.HandleError(fmt.Errorf("resourceId and versionId are required"), w, r, http.StatusBadRequest)
			return
		}

		if err := ctx.DeleteVersion(uint(resourceID), uint(versionID)); err != nil {
			http_utils.HandleError(err, w, r, versionErrorStatus(err))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(w, r, fmt.Sprintf("/resource?id=%v", resourceID)) {
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}

// GetVersionFileHandler returns handler for downloading version file
func GetVersionFileHandler(ctx interfaces.VersionFileServer) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, err := strconv.ParseUint(r.URL.Query().Get("versionId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid versionId"), w, r, http.StatusBadRequest)
			return
		}

		version, err := ctx.GetVersion(uint(versionID))
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusNotFound)
			return
		}

		fs, err := ctx.GetFsForStorageLocation(version.StorageLocation)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}

		file, err := fs.Open(version.Location)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusNotFound)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", version.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"v%d_%s\"", version.VersionNumber, version.Hash[:8]))

		http.ServeContent(w, r, "", version.CreatedAt, file)
	}
}

// GetCleanupVersionsHandler returns handler for cleaning up versions
func GetCleanupVersionsHandler(ctx interfaces.VersionCleaner) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var query query_models.VersionCleanupQuery
		if err := tryFillStructValuesFromRequest(&query, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		deletedIDs, err := ctx.CleanupVersions(&query)
		if err != nil {
			http_utils.HandleError(err, w, r, versionErrorStatus(err))
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deletedVersionIds": deletedIDs,
			"count":             len(deletedIDs),
		})
	}
}

// GetBulkCleanupVersionsHandler returns handler for bulk cleanup
func GetBulkCleanupVersionsHandler(ctx interfaces.VersionCleaner) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var query query_models.BulkVersionCleanupQuery
		if err := tryFillStructValuesFromRequest(&query, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		result, err := ctx.BulkCleanupVersions(&query)
		if err != nil {
			http_utils.HandleError(err, w, r, versionErrorStatus(err))
			return
		}

		totalDeleted := 0
		for _, ids := range result {
			totalDeleted += len(ids)
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deletedByResource": result,
			"totalDeleted":      totalDeleted,
		})
	}
}

// GetCompareVersionsHandler returns handler for comparing versions
func GetCompareVersionsHandler(ctx interfaces.VersionComparer) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID, err := strconv.ParseUint(r.URL.Query().Get("resourceId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid resourceId"), w, r, http.StatusBadRequest)
			return
		}

		v1, err := strconv.ParseUint(r.URL.Query().Get("v1"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid v1"), w, r, http.StatusBadRequest)
			return
		}

		v2, err := strconv.ParseUint(r.URL.Query().Get("v2"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid v2"), w, r, http.StatusBadRequest)
			return
		}

		comparison, err := ctx.CompareVersions(uint(resourceID), uint(v1), uint(v2))
		if err != nil {
			http_utils.HandleError(err, w, r, versionErrorStatus(err))
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(comparison)
	}
}
