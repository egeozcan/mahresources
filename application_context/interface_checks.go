package application_context

import "mahresources/server/interfaces"

// Compile-time interface compliance checks for MahresourcesContext.
// These ensure the context implements all required interfaces.
var (
	// Existing interfaces
	_ interfaces.ResourceMetaReader      = (*MahresourcesContext)(nil)
	_ interfaces.ResourceThumbnailLoader = (*MahresourcesContext)(nil)
	_ interfaces.GroupMetaReader         = (*MahresourcesContext)(nil)
	_ interfaces.NoteMetaReader          = (*MahresourcesContext)(nil)
	_ interfaces.NoteTypeReader          = (*MahresourcesContext)(nil)

	// Granular Resource interfaces
	_ interfaces.ResourceCreator        = (*MahresourcesContext)(nil)
	_ interfaces.ResourceEditor         = (*MahresourcesContext)(nil)
	_ interfaces.BulkResourceTagEditor  = (*MahresourcesContext)(nil)
	_ interfaces.BulkResourceGroupEditor = (*MahresourcesContext)(nil)
	_ interfaces.BulkResourceMetaEditor = (*MahresourcesContext)(nil)
	_ interfaces.BulkResourceDeleter    = (*MahresourcesContext)(nil)
	_ interfaces.ResourceMerger         = (*MahresourcesContext)(nil)
	_ interfaces.ResourceMediaProcessor = (*MahresourcesContext)(nil)
	_ interfaces.ResourceWriter         = (*MahresourcesContext)(nil) // composite

	// Granular Group interfaces
	_ interfaces.GroupCreator       = (*MahresourcesContext)(nil)
	_ interfaces.GroupUpdater       = (*MahresourcesContext)(nil)
	_ interfaces.BulkGroupTagEditor = (*MahresourcesContext)(nil)
	_ interfaces.BulkGroupMetaEditor = (*MahresourcesContext)(nil)
	_ interfaces.GroupMerger        = (*MahresourcesContext)(nil)
	_ interfaces.GroupDuplicator    = (*MahresourcesContext)(nil)
	_ interfaces.GroupCRUD          = (*MahresourcesContext)(nil)
	_ interfaces.GroupWriter        = (*MahresourcesContext)(nil) // composite
)
