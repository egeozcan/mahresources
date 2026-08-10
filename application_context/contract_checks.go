package application_context

import "mahresources/contracts"

// Compile-time interface compliance checks for MahresourcesContext.
// These ensure the context implements all required interfaces.
var (
	// Existing interfaces
	_ contracts.ResourceMetaReader      = (*MahresourcesContext)(nil)
	_ contracts.ResourceThumbnailLoader = (*MahresourcesContext)(nil)
	_ contracts.GroupMetaReader         = (*MahresourcesContext)(nil)
	_ contracts.NoteMetaReader          = (*MahresourcesContext)(nil)
	_ contracts.NoteTypeReader          = (*MahresourcesContext)(nil)

	// Template Partial interfaces
	_ contracts.TemplatePartialReader  = (*MahresourcesContext)(nil)
	_ contracts.TemplatePartialWriter  = (*MahresourcesContext)(nil)
	_ contracts.TemplatePartialDeleter = (*MahresourcesContext)(nil)

	// Download history (the durable record behind /downloads)
	_ contracts.DownloadHistoryReader = (*MahresourcesContext)(nil)
	_ contracts.DownloadHistoryWriter = (*MahresourcesContext)(nil)

	// Granular Resource interfaces
	_ contracts.ResourceCreator         = (*MahresourcesContext)(nil)
	_ contracts.ResourceEditor          = (*MahresourcesContext)(nil)
	_ contracts.BulkResourceTagEditor   = (*MahresourcesContext)(nil)
	_ contracts.BulkResourceGroupEditor = (*MahresourcesContext)(nil)
	_ contracts.BulkResourceMetaEditor  = (*MahresourcesContext)(nil)
	_ contracts.BulkResourceDeleter     = (*MahresourcesContext)(nil)
	_ contracts.ResourceMerger          = (*MahresourcesContext)(nil)
	_ contracts.ResourceMediaProcessor  = (*MahresourcesContext)(nil)
	_ contracts.ResourceWriter          = (*MahresourcesContext)(nil) // composite

	// Granular Group interfaces
	_ contracts.GroupCreator        = (*MahresourcesContext)(nil)
	_ contracts.GroupUpdater        = (*MahresourcesContext)(nil)
	_ contracts.BulkGroupTagEditor  = (*MahresourcesContext)(nil)
	_ contracts.BulkGroupMetaEditor = (*MahresourcesContext)(nil)
	_ contracts.GroupMerger         = (*MahresourcesContext)(nil)
	_ contracts.GroupDuplicator     = (*MahresourcesContext)(nil)
	_ contracts.GroupCRUD           = (*MahresourcesContext)(nil)
	_ contracts.GroupWriter         = (*MahresourcesContext)(nil) // composite
)
