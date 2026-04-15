---
outputShape: Group object with ID, Name, Description, Meta, OwnerId, CategoryId, CreatedAt/UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: group get, group edit-name, group edit-description, group edit-meta, groups list
---

# Long

Create a new Group. `--name` is required; all other fields are
optional. Use `--owner-id` to place the new Group under an existing
parent (forming a subtree); use `--category-id` to attach a Category;
pass a JSON blob via `--meta` for free-form custom metadata. Sends
`POST /v1/group` and returns the persisted record.

# Example

  # Create a top-level group
  mr group create --name "Trips 2026"

  # Create a child group with meta and a category
  mr group create --name "Berlin" --owner-id 5 --category-id 2 --meta '{"city":"Berlin"}'

  # mr-doctest: create a group and verify the returned ID is non-zero
  ID=$(mr group create --name "doctest-create-$$-$RANDOM" --json | jq -r '.ID')
  mr group get $ID --json | jq -e '.ID > 0'
