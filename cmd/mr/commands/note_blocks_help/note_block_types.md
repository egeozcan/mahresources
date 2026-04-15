---
outputShape: Array of block type descriptors, each with type (string), defaultContent (object), defaultState (object), and optional plugin metadata (label, icon, description, plugin, pluginName)
exitCodes: 0 on success; 1 on any error
relatedCmds: note-block create, note-block update, note-block update-state
---

# Long

List every block type the server knows about, including built-in types
(`text`, `heading`, `todos`, `gallery`, `references`, `table`,
`calendar`, `divider`) and any types registered by active plugins. Each
entry includes `defaultContent` and `defaultState` — the canonical
empty-payload shapes you should extend when creating a block of that
type. Useful for discovering the content/state schema a given type
expects before calling `note-block create` or `note-block update`.

# Example

  # List all block types as a table (default)
  mr note-block types

  # List types as JSON and extract just the names
  mr note-block types --json | jq -r '.[].type'

  # mr-doctest: list types and assert the response is a non-empty array
  mr note-block types --json | jq -e 'type == "array" and length > 0'
