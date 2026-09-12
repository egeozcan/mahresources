---
outputShape: Updated Category object with ID and template fields under --json; confirmation otherwise
exitCodes: 0 on success; 1 on any error
relatedCmds: category create, category get, category edit-name, category edit-description
---

# Long

Partially edit a Category. `--id` is required and must be positive. Only
explicit flags change stored values; an explicit empty string clears a field.
Supports `--name`, `--description`, `--meta-schema`, `--section-config` and
all category `--custom-*` template flags, including
`--custom-entity-picker-result` and `--custom-entity-picker-result-css`.
The picker template supplies content, not selection controls.

Use `--custom-entity-picker-result-file` or
`--custom-entity-picker-result-css-file` to read UTF-8 HTML or CSS from a file.
Each file flag is mutually exclusive with its corresponding inline flag;
an empty file clears the slot. Files are read before any HTTP request.

# Example

  # mr-doctest: edit and clear a category picker template
  ID=$(mr category create --name "picker-edit-$$-$RANDOM" --json | jq -r .ID)
  mr category edit --id "$ID" --custom-entity-picker-result '<b>[property path="Name"]</b>'
  mr category edit --id "$ID" --custom-entity-picker-result ''
  mr category get "$ID" --json | jq -e '.CustomEntityPickerResult == ""'
  mr category delete "$ID"

  # mr-doctest: load picker content from a file
  ID=$(mr category create --name "picker-file-$$-$RANDOM" --json | jq -r .ID)
  FILE=$(mktemp)
  printf '%s' '<b>From file</b>' > "$FILE"
  mr category edit --id "$ID" --custom-entity-picker-result-file "$FILE"
  mr category get "$ID" --json | jq -e '.CustomEntityPickerResult == "<b>From file</b>"'
  rm "$FILE"
  mr category delete "$ID"
