---
outputShape: Resource object with id
exitCodes: 0 on success; 1 on any error
relatedCmds: resource upload, resource from-local
---

# Long

Create a Resource by having the server fetch a remote URL. Useful when
you have a public asset that shouldn't be proxied through your local
machine. The `--url` flag is required; the server downloads, stores, and
indexes the file. Optional `--tags` / `--groups` attach relationships at
creation.

# Example

  # Create from a URL
  mr resource from-url --url https://example.com/photo.jpg

  # With metadata and groups
  mr resource from-url --url https://example.com/doc.pdf --name "Paper" --meta '{"source":"arxiv"}' --groups 5

  # mr-doctest: ephemeral server has no outbound access, skip-on=ephemeral
  mr resource from-url --url https://example.com/tiny.jpg --name "from-url-test"
