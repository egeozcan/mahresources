package api_handlers

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
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
func GetListVersionsHandler(ctx contracts.VersionReader) func(http.ResponseWriter, *http.Request) {
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
func GetVersionHandler(ctx contracts.VersionReader) func(http.ResponseWriter, *http.Request) {
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

// GetUploadVersionHandler returns handler for uploading a new version.
//
// BH-034: maxUploadSize is a getter read at request time (mirrors the
// pattern in GetResourceUploadHandler). 0 = unlimited. Oversize bodies are
// bounded by http.MaxBytesReader and surface as ParseMultipartForm errors.
func GetUploadVersionHandler(ctx contracts.VersionWriter, maxUploadSize func() int64) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID, err := strconv.ParseUint(r.URL.Query().Get("resourceId"), 10, 64)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("invalid resourceId"), w, r, http.StatusBadRequest)
			return
		}

		limit := int64(0)
		if maxUploadSize != nil {
			limit = maxUploadSize()
		}
		if limit > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
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
func GetRestoreVersionHandler(ctx contracts.VersionWriter) func(http.ResponseWriter, *http.Request) {
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
func GetDeleteVersionHandler(ctx contracts.VersionDeleter) func(http.ResponseWriter, *http.Request) {
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
func GetVersionFileHandler(ctx contracts.VersionFileServer) func(http.ResponseWriter, *http.Request) {
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
		// Keep the extension: a downloaded file named "v3_a3bf2ae2" has no type
		// for the OS to dispatch on and will not open by double-click.
		disposition := "attachment"
		if inlineVersionDisposition(r, version.ContentType) {
			// A framed attachment is not a viewer: the browser downloads it instead.
			// Inline is opt-in per request, safelisted by content type, and the only
			// case that relaxes the primary server's blanket X-Frame-Options: DENY.
			disposition = "inline"
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, versionDownloadFilename(version)))

		http.ServeContent(w, r, "", version.CreatedAt, file)
	}
}

// versionDownloadFilename builds the download name for a resource version:
// "v<n>_<hash prefix><ext>". The extension is taken from the version's own
// stored location, falling back to the content type, so the saved file keeps a
// type the OS can dispatch on.
func versionDownloadFilename(version *models.ResourceVersion) string {
	base := fmt.Sprintf("v%d_%s", version.VersionNumber, safeHashPrefix(version.Hash))

	if ext := path.Ext(version.Location); ext != "" {
		return base + ext
	}
	if exts, err := mime.ExtensionsByType(version.ContentType); err == nil && len(exts) > 0 {
		return base + exts[0]
	}
	return base
}

// safeHashPrefix returns the first 8 characters of a hash, tolerating hashes
// shorter than that rather than panicking on the slice.
func safeHashPrefix(hash string) string {
	if len(hash) < 8 {
		return hash
	}
	return hash[:8]
}

// GetCleanupVersionsHandler returns handler for cleaning up versions
func GetCleanupVersionsHandler(ctx contracts.VersionCleaner) func(http.ResponseWriter, *http.Request) {
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
func GetBulkCleanupVersionsHandler(ctx contracts.VersionCleaner) func(http.ResponseWriter, *http.Request) {
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
func GetCompareVersionsHandler(ctx contracts.VersionComparer) func(http.ResponseWriter, *http.Request) {
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

// inlineVersionDisposition reports whether this request asked for the version to
// be rendered in place, and whether its type is one we are willing to render.
//
// The safelist is the whole of the security argument. Version files are arbitrary
// user uploads, so serving one inline and same-origin-framable is stored XSS for
// anything the browser executes in a document context — text/html above all, and
// image/svg+xml, which is a document. That is why every version file is an
// attachment. PDFs are the exception worth making: the browser renders them in
// its own sandboxed viewer, which cannot reach the embedding origin's DOM. Do not
// add a type here without making that argument again for it.
func inlineVersionDisposition(r *http.Request, contentType string) bool {
	if r.URL.Query().Get("disposition") != "inline" {
		return false
	}
	// Strip any parameters ("application/pdf; charset=..."), and compare
	// case-insensitively, because the stored value came from sniffing or from
	// an upload header.
	base, _, _ := strings.Cut(contentType, ";")
	return strings.EqualFold(strings.TrimSpace(base), "application/pdf")
}
