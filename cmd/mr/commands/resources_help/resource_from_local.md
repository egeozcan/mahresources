---
outputShape: Resource object with id
exitCodes: 0 on success; 1 on any error
relatedCmds: resource upload, resource from-url
---

# Long

Create a Resource from a file already present on the server's filesystem.
Differs from `upload` (which streams bytes over HTTP) in that the server
reads the file in place. Useful for bulk-importing existing files or
deploying pre-staged assets.

`--path` is required and is resolved inside the server's storage root, not
on the host: a file staged at `$FILE_SAVE_PATH/incoming/photo.jpg` is
`--path /incoming/photo.jpg` here. The name defaults to the file's base
name.

# Example

  # Create from a path under the server's storage root
  mr resource from-local --path /incoming/photo.jpg

  # With metadata
  mr resource from-local --path /imports/doc.pdf --name "Doc" --tags 3,7

  # mr-doctest: both doctest servers run -ephemeral so the storage filesystem is in memory and no path exists in it, skip-on=ephemeral|auth
  mr resource from-local --path /incoming/sample.jpg --name "from-local-test"
