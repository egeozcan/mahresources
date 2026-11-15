package mcpserver

// Safety is how an operation is exposed over MCP.
type Safety int

const (
	// ReadOnly operations change nothing. Every GET is ReadOnly unless it is
	// Excluded; a POST is ReadOnly only when it is listed in readOnlyPosts.
	ReadOnly Safety = iota + 1
	// Mutating operations change data and run without a confirmation.
	Mutating
	// Destructive operations delete, merge, overwrite in bulk, publish, or change
	// system configuration. They never run without a confirmation the client
	// showed to its user (see confirm.go).
	Destructive
	// Excluded operations are not reachable over MCP at all.
	Excluded
)

func (s Safety) String() string {
	switch s {
	case ReadOnly:
		return "read-only"
	case Mutating:
		return "mutating"
	case Destructive:
		return "destructive"
	case Excluded:
		return "excluded"
	}
	return "unclassified"
}

// readOnlyPosts are POST operations that are semantically reads.
var readOnlyPosts = map[string]bool{
	"executeMRQL":                        true,
	"completeMRQL":                       true,
	"explainMRQL":                        true,
	"exportMRQL":                         true,
	"validateMRQL":                       true,
	"runSavedMRQLQuery":                  true,
	"runQuery":                           true,
	"estimateGroupExport":                true,
	"lintShortcodes":                     true,
	"previewCategoryDescription":         true,
	"previewGroupDescription":            true,
	"previewNoteDescription":             true,
	"previewNoteTypeDescription":         true,
	"previewQueryDescription":            true,
	"previewRelationDescription":         true,
	"previewRelationTypeDescription":     true,
	"previewResourceDescription":         true,
	"previewResourceCategoryDescription": true,
	"previewTagDescription":              true,
	"previewTemplatePartialDescription":  true,
	"previewCategoryTemplate":            true,
	"previewNoteTypeTemplate":            true,
	"previewResourceCategoryTemplate":    true,
}

// excluded lists every operation MCP does not expose, with the reason.
var excluded = map[string]string{
	// Sessions belong to browsers.
	"login":  "starts a browser session",
	"logout": "ends a browser session",

	// Credential issuance. A token or password minted here would be usable
	// against the REST API directly, outside every MCP confirmation.
	"createOwnToken":    "mints a credential usable outside MCP",
	"changeOwnPassword": "sets a credential usable outside MCP",
	"createUser":        "sets a credential usable outside MCP",
	"updateUser":        "can set a credential or role usable outside MCP",

	// Streams and browser fragments.
	"downloadEvents":           "server-sent event stream",
	"streamCanonicalJobEvents": "server-sent event stream",
	"renderPluginBlock":        "HTML fragment for the browser",
	"renderPluginBlocks":       "HTML fragments for the browser",
	"renderPluginDisplay":      "HTML fragment for the browser",
	"renderDeferredShortcode":  "HTML fragment for the browser",
	"browseEntities":           "browser entity-picker widget",
	"resolvePickerEntities":    "browser entity-picker widget",

	// Legacy aliases of the canonical /v1/jobs routes.
	"submitDownload":   "legacy alias of jobsSubmitDownload",
	"cancelDownload":   "legacy alias of jobsCancel",
	"pauseDownload":    "legacy alias of jobsPause",
	"resumeDownload":   "legacy alias of jobsResume",
	"retryDownload":    "legacy alias of jobsRetry",
	"getDownloadQueue": "legacy alias of jobsGetQueue",
}

// destructive operations need a confirmation. Every delete, merge, bulk
// replacement, publication and system-configuration change is here.
var destructive = map[string]bool{
	// Account
	"deleteSavedSearch": true,
	"revokeOwnToken":    true,

	// Administration
	"updateRuntimeSettings": true,
	"setRuntimeSetting":     true,
	"resetRuntimeSetting":   true,
	"bulkRevokeShares":      true,
	"deleteUser":            true,
	"jobsClearCompleted":    true,
	"deleteDownloadHistory": true,

	// Plugins
	"enablePlugin":          true,
	"disablePlugin":         true,
	"purgePluginData":       true,
	"setPluginScopedAccess": true,
	"updatePluginSettings":  true,

	// Jobs. A command can be "forget", which erases a job's stored inputs;
	// the jobs package itself advertises it as destructive.
	"executeCanonicalJobCommand":     true,
	"executeBulkCanonicalJobCommand": true,

	// Bulk meta "add" merges into existing meta and overwrites the keys it names.
	"bulkAddMetaToGroups":    true,
	"bulkAddMetaToNotes":     true,
	"bulkAddMetaToResources": true,

	// Groups
	"applyGroupImport":  true, // a "replace" collision policy overwrites existing entities
	"deleteGroup":       true,
	"bulkDeleteGroups":  true,
	"mergeGroups":       true,
	"massEditGroups":    true,
	"deleteGroupImport": true,

	// Notes
	"deleteNote":      true,
	"bulkDeleteNotes": true,
	"massEditNotes":   true,
	"deleteBlock":     true,
	"deleteBlockPost": true,
	"shareNote":       true,
	"unshareNote":     true,

	// Resources
	"deleteResource":             true,
	"bulkDeleteResources":        true,
	"mergeResources":             true,
	"massEditResources":          true,
	"bulkReplaceTagsOfResources": true,
	"deleteVersion":              true,
	"deleteVersionPost":          true,
	"cleanupVersions":            true,
	"bulkCleanupVersions":        true,
	"applyResourceReduction":     true,
	// Clearing previews also deletes an uploaded custom thumbnail, which
	// cannot be regenerated.
	"clearResourceThumbnail":        true,
	"clearResourceThumbnailViaPost": true,
	"deleteResourceReduction":       true,

	// Side-effect destruction. Each of these deletes stored previews,
	// including an uploaded custom thumbnail, which a version does not keep.
	"setCustomResourceThumbnail": true,
	"rotateResource":             true,
	"cropResource":               true,
	"trimResourceVideo":          true,
	"uploadVersion":              true,
	"restoreVersion":             true,
	// Removing the last member deletes the Series. editResource does the same
	// when SeriesId is sent as 0; see effectiveSafety in tools.go.
	"removeResourceFromSeries": true,
	// Note type edits can turn on ApplyTemplatesToShares, which publishes the
	// type's templates on every shared note of that type. createNoteType
	// updates an existing type when ID is set.
	"editNoteType":            true,
	"createNoteType":          true,
	"restoreNoteTypeRevision": true,
	// Plugin code can delete or replace plugin and application data, and an
	// action declares no safety MCP could enforce.
	"runPluginAction":      true,
	"runPluginScheduleNow": true,

	// Taxonomy and other entities
	"deleteTag":              true,
	"bulkDeleteTags":         true,
	"mergeTags":              true,
	"deleteCategory":         true,
	"deleteResourceCategory": true,
	"deleteNoteType":         true,
	"deleteRelation":         true,
	"deleteRelationType":     true,
	"deleteSeries":           true,
	"deleteQuery":            true,
	"deleteSavedMRQLQuery":   true,
	"deleteTemplatePartial":  true,
}

// mutating lists every non-GET operation that changes data without needing a
// confirmation. It is explicit, rather than "everything else", so that a new
// route has to be classified before it is reachable (see the drift test).
var mutating = map[string]bool{
	"createSavedSearch": true, "updateSavedSearch": true,
	"setOwnSetting": true, "deleteOwnSetting": true,
	"recomputeSimilarities": true, "retryFailedHashes": true,

	"createOrUpdateCategory": true, "editCategoryDescription": true, "editCategoryName": true,
	"generateCategoryTemplate": true, "finishCategoryTemplateRun": true,
	"labelCategoryRevision": true, "restoreCategoryRevision": true,

	"retryDownloadHistory": true,

	"createOrUpdateGroup": true, "cloneGroup": true, "editGroupDescription": true,
	"editGroupMeta": true, "editGroupName": true,
	"bulkAddTagsToGroups": true, "bulkRemoveTagsFromGroups": true,
	"submitGroupExport": true, "parseGroupImport": true,

	"jobsCancel":         true,
	"jobsSubmitDownload": true, "jobsPause": true, "jobsResume": true, "jobsRetry": true,
	"exportCanonicalJobSummary": true,

	"generateMRQL": true, "createSavedMRQLQuery": true, "updateSavedMRQLQuery": true,

	"createOrUpdateNote": true, "createBlock": true, "updateBlockContent": true,
	"updateBlockState": true, "rebalanceBlocks": true, "reorderBlocks": true,
	"editNoteDescription": true, "editNoteMeta": true, "editNoteName": true,
	"editNoteTypeDescription": true,
	"editNoteTypeName":        true, "generateNoteTypeTemplate": true, "finishNoteTypeTemplateRun": true,
	"labelNoteTypeRevision": true, "bulkAddGroupsToNotes": true, "bulkAddTagsToNotes": true,
	"bulkRemoveTagsFromNotes": true,

	"cancelPluginCommandRun": true, "cancelPluginScheduledDownload": true,

	"createOrUpdateQuery": true, "editQueryDescription": true, "editQueryName": true,

	"createResourceReduction": true, "overrideResourceReductionCluster": true,
	"computeResourceReduction": true, "editResourceReduction": true,

	"createOrUpdateRelation": true, "editRelationDescription": true, "editRelationName": true,
	"createRelationType": true, "editRelationType": true, "editRelationTypeDescription": true,
	"editRelationTypeName": true,

	"createResource": true, "editResource": true, "editResourceDescription": true,
	"editResourceMeta": true, "editResourceName": true, "addLocalResource": true,
	"bulkRecalculateDimensions": true,
	"addRemoteResource":         true, "bulkAddGroupsToResources": true,
	"bulkAddTagsToResources": true, "bulkRemoveTagsFromResources": true,
	"setResourceDimensions": true, "createOrUpdateResourceCategory": true, "editResourceCategoryDescription": true,
	"editResourceCategoryName": true, "generateResourceCategoryTemplate": true,
	"finishResourceCategoryTemplateRun": true, "labelResourceCategoryRevision": true,
	"restoreResourceCategoryRevision": true,

	"updateSeries": true, "createSeries": true, "editSeriesName": true,

	"createOrUpdateTag": true, "editTagDescription": true, "editTagName": true,

	"createOrUpdateTemplatePartial": true, "updateTemplatePartial": true,
	"editTemplatePartialDescription": true,
}

// openWorld operations reach outside mahresources: they fetch URLs, read the
// server's filesystem, call an AI provider, or run plugin code that may.
var openWorld = map[string]bool{
	"createResource":                   true,
	"addRemoteResource":                true,
	"addLocalResource":                 true,
	"jobsSubmitDownload":               true,
	"retryDownloadHistory":             true,
	"generateMRQL":                     true,
	"generateCategoryTemplate":         true,
	"generateNoteTypeTemplate":         true,
	"generateResourceCategoryTemplate": true,
	"runPluginAction":                  true,
	"runPluginScheduleNow":             true,
}

// classify returns the Safety of an operation, or 0 when a non-GET operation
// has not been classified.
func classify(operationID, method string) Safety {
	if _, ok := excluded[operationID]; ok {
		return Excluded
	}
	if method == "GET" {
		return ReadOnly
	}
	switch {
	case readOnlyPosts[operationID]:
		return ReadOnly
	case destructive[operationID]:
		return Destructive
	case mutating[operationID]:
		return Mutating
	}
	return 0
}

// ClassifiedOperationIDs returns every operation ID named in the safety tables.
func ClassifiedOperationIDs() []string {
	var ids []string
	for _, table := range []map[string]bool{readOnlyPosts, destructive, mutating, openWorld} {
		for id := range table {
			ids = append(ids, id)
		}
	}
	for id := range excluded {
		ids = append(ids, id)
	}
	for id := range linkOnly {
		ids = append(ids, id)
	}
	for id := range inlineImages {
		ids = append(ids, id)
	}
	return ids
}
