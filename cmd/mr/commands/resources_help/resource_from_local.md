---
outputShape: Resource object with id
exitCodes: 0 on success; 1 on any error
relatedCmds: resource upload, resource from-url
---

# Long

Create a Resource from a file already present on the server's filesystem.
Differs from `upload` (which streams bytes over HTTP) in that the server
reads the file in place. The `--path` flag is required and must resolve
on the target server. Useful for bulk-importing existing files or
deploying pre-staged assets.

# Example

  # Create from a server-local path
  mr resource from-local --path /var/mahresources/incoming/photo.jpg

  # With metadata
  mr resource from-local --path /srv/imports/doc.pdf --name "Doc" --tags 3,7

  # mr-doctest: path only valid on the real target server, skip-on=ephemeral
  mr resource from-local --path /tmp/sample.jpg --name "from-local-test"
