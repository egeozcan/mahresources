package application_context

import "mahresources/server/interfaces"

// Compile-time interface compliance checks for MahresourcesContext.
// These ensure the context implements all required interfaces.
var (
	_ interfaces.ResourceMetaReader      = (*MahresourcesContext)(nil)
	_ interfaces.ResourceThumbnailLoader = (*MahresourcesContext)(nil)
	_ interfaces.GroupMetaReader         = (*MahresourcesContext)(nil)
	_ interfaces.NoteMetaReader          = (*MahresourcesContext)(nil)
	_ interfaces.NoteTypeReader          = (*MahresourcesContext)(nil)
)
