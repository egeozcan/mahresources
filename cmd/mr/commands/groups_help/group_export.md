---
outputShape: Tar archive written to stdout or --output path; when --no-wait, prints the job ID as plain text
exitCodes: 0 on success; 1 on any error
relatedCmds: group import, group clone, groups list
---

# Long

Export one or more Groups and their reachable entities to a portable
tar archive. Sends `POST /v1/groups/export`, polls the resulting job
until completion, then downloads the tar. Takes one or more Group IDs
as positional arguments; each ID becomes a root of the export tree.

The archive format follows the manifest schema v1 (see `archive/manifest.go`)
and is compatible with `mr group import` on any mahresources instance.
Scope and fidelity are controlled by paired `--include-*` / `--no-*`
flags (subtree, resources, notes, related, group-relations, blobs,
versions, previews, series). Schema-definition inclusion (categories,
tag defs, group-relation types) can be toggled individually or via the
`--schema-defs=all|none|selected` shortcut. Use `--gzip` to compress
the output and `--output <path>` (or `-o`) to write to a file rather
than stdout.

By default the command waits for the server-side job to finish before
downloading; pass `--no-wait` to print the job ID and exit immediately
so you can poll and download separately.

# Example

  # Export group 42 and its subtree to a tar file
  mr group export 42 --output /tmp/trip-2026.tar

  # Export two roots, compressed, with no resource blobs or related entities
  mr group export 42 43 --gzip --no-blobs --no-related --output /tmp/shell.tar.gz

  # Submit the job and print its ID without waiting
  mr group export 42 --no-wait

  # mr-doctest: create a group, export it to a tar file, assert the file exists and is non-empty
  ID=$(mr group create --name "doctest-export-$$-$RANDOM" --json | jq -r '.ID')
  OUT=/tmp/doctest-export-$$-$RANDOM.tar
  mr group export $ID -o $OUT
  test -s $OUT
  rm -f $OUT
