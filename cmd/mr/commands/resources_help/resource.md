---
exitCodes: 0 on success; 1 on any error
relatedCmds: resources list, group list, tag list
---

# Long

Resources are files stored in mahresources. A Resource has a name,
content bytes, MIME type, optional dimensions, perceptual hash, and
free-form meta JSON. Resources relate many-to-many to Tags, Notes, and
Groups, and support versioned edits (see `versions`, `version-upload`).

Use the `resource` subcommands to operate on a single resource by ID:
fetch metadata, upload a file, rotate an image, or manage its version
history. Use `resources list` to discover resources matching filters.
