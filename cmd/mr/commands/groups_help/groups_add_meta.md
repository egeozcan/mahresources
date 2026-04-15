---
outputShape: Status object with ok (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: group edit-meta, groups meta-keys, group get
---

# Long

Merge a Meta JSON object onto multiple Groups at once. Both arguments
are required: `--ids` selects the target Groups (comma-separated) and
`--meta` is a JSON object string that is deep-merged onto each target's
existing Meta. Existing keys are overwritten by the incoming value;
keys not present in `--meta` are preserved.

To edit a single path on a single group, prefer `group edit-meta` which
takes a dotted path + JSON literal. This bulk variant is best for
stamping the same set of keys across many Groups.

# Example

  # Stamp one Meta key across three groups
  mr groups add-meta --ids 10,11,12 --meta '{"reviewed":true}'

  # Merge multiple keys
  mr groups add-meta --ids 10 --meta '{"season":"winter","owner":"alice"}'

  # mr-doctest: stamp a meta key and verify it appears on the target group
  GID=$(mr group create --name "doctest-addmeta-$$-$RANDOM" --json | jq -r '.ID')
  KEY="probe_$RANDOM"
  mr groups add-meta --ids=$GID --meta="{\"$KEY\":42}"
  mr group get $GID --json | jq --arg k "$KEY" -e '.Meta[$k] == 42'
