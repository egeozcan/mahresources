---
outputShape: Created Series object with ID (uint), Name (string), Slug (string), Meta (object), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: series get, series edit-name, series list
---

# Long

Create a new series. `--name` is required. The server derives the slug
from the name at creation time; the slug never changes when the name is
later edited, so pick a name with care. On success prints a confirmation
line with the new ID; pass the global `--json` flag to emit the full
record for scripting (e.g., piping the new ID into follow-up commands).

# Example

  # Create a series with just a name
  mr series create --name "spring-2026-photos"

  # Create and capture the new ID via jq
  ID=$(mr series create --name "volume-1" --json | jq -r .ID)

  # mr-doctest: create a series, assert the returned ID is positive
  mr series create --name "doctest-create-$$-$RANDOM" --json | jq -e '.ID > 0'
