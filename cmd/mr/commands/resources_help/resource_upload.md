---
outputShape: Resource object with id, name
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit, resource from-url, resource from-local, resources list
---

# Long

Upload a local file as a new Resource. Sends the file via multipart form
to `POST /v1/resource`. The Resource's name defaults to the source
filename if `--name` is not set. Use `--meta` for a JSON blob of custom
metadata that is merged into the new record.

# Example

  # Basic upload (name defaults to the filename)
  mr resource upload ./photo.jpg

  # Upload with ownership and meta JSON
  mr resource upload ./photo.jpg --owner-id 3 --meta '{"camera":"Pixel"}'

  # mr-doctest: upload a fixture and verify the returned id
  GRP=$(mr group create --name "doctest-upload-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "upload-test-$$" --json | jq -r '.[0].ID')
  mr resource get $ID --json | jq -e '.ID > 0'
