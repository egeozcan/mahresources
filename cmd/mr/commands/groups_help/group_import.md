---
outputShape: ImportPlan (dry-run) or ImportApplyResult object with CreatedGroups, CreatedResources, SkippedByHash, CreatedNotes, CreatedGroupIDs arrays, etc.
exitCodes: 0 on success; 1 on any error
relatedCmds: group export, group create, groups list
---

# Long

Upload a group export tar, parse it into an import plan, and optionally
apply it. Takes the path to a tar file (produced by `mr group export`
or the `/v1/groups/export` API) as its single positional argument.

The command runs a two-phase job pipeline: first a `parse` job uploads
the tar, validates the manifest schema version, and produces an
`ImportPlan` (counts, mappings, conflicts, dangling refs). Then — unless
`--dry-run` is set — an `apply` job actually creates the groups and
related entities.

Use `--dry-run` to inspect the plan without mutating state. Use
`--plan-output <file>` to save the parsed plan JSON. Use
`--parent-group <id>` to graft imported top-level groups under an
existing parent. Use `--on-resource-conflict=skip|duplicate` and
`--guid-collision-policy=merge|skip|replace` to steer conflict
resolution. For full manual control over every mapping/dangling/shell
decision, pass `--decisions <json-file>` produced from a prior dry-run.

When the server plan reports resources without bytes in the tar,
`--acknowledge-missing-hashes` is required to proceed.

# Example

  # Dry-run an import and print the plan
  mr group import /tmp/trip-2026.tar --dry-run

  # Import, grafting top-level groups under an existing parent
  mr group import /tmp/trip-2026.tar --parent-group 17

  # Dry-run to JSON file for review
  mr group import /tmp/trip-2026.tar --dry-run --plan-output /tmp/plan.json

  # mr-doctest: roundtrip export + dry-run import of the same tar, assert the plan is a JSON object
  ID=$(mr group create --name "doctest-roundtrip-$$-$RANDOM" --json | jq -r '.ID')
  OUT=/tmp/doctest-roundtrip-$$-$RANDOM.tar
  mr group export $ID -o $OUT
  mr group import $OUT --dry-run --json 2>/dev/null | jq -e '.schema_version == 1 and (.counts.groups >= 1)'
  rm -f $OUT
