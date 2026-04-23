---
title: mr CLI
description: Command-line reference for the mr tool
sidebar_label: CLI
---

# mr CLI reference

| Command | Short | |
|---------|-------|--|
| `mr admin` | Server administration commands | [Details](./admin/index.md) |
| `mr admin stats` | Show server and data statistics | [Details](./admin/stats.md) |
| `mr admin settings` | View and manage runtime configuration overrides | [Details](./admin/settings.md) |
| `mr admin settings get` | Show a single runtime setting by key | [Details](./admin/settings.md#get) |
| `mr admin settings list` | List all runtime settings | [Details](./admin/settings.md#list) |
| `mr admin settings reset` | Remove a runtime override and revert to boot default | [Details](./admin/settings.md#reset) |
| `mr admin settings set` | Override a runtime setting | [Details](./admin/settings.md#set) |
| `mr categories` | List group categories | [Details](./categories/index.md) |
| `mr categories list` | List categories | [Details](./categories/list.md) |
| `mr categories timeline` | Display a timeline of category activity | [Details](./categories/timeline.md) |
| `mr category` | Get, create, edit, or delete a group category | [Details](./category/index.md) |
| `mr category create` | Create a new category | [Details](./category/create.md) |
| `mr category delete` | Delete a category by ID | [Details](./category/delete.md) |
| `mr category edit-description` | Edit a category's description | [Details](./category/edit-description.md) |
| `mr category edit-name` | Edit a category's name | [Details](./category/edit-name.md) |
| `mr category get` | Get a category by ID | [Details](./category/get.md) |
| `mr docs` | Introspect and validate the mr CLI's own documentation | [Details](./docs/index.md) |
| `mr docs check-examples` | Run `# mr-doctest:` example blocks against a live server | [Details](./docs/check-examples.md) |
| `mr docs dump` | Emit the mr command tree as JSON or Markdown | [Details](./docs/dump.md) |
| `mr docs lint` | Validate every command's help against the template | [Details](./docs/lint.md) |
| `mr group` | Get, create, edit, delete, or clone a group | [Details](./group/index.md) |
| `mr group children` | List child groups (tree children) of a group | [Details](./group/children.md) |
| `mr group clone` | Clone a group | [Details](./group/clone.md) |
| `mr group create` | Create a new group | [Details](./group/create.md) |
| `mr group delete` | Delete a group by ID | [Details](./group/delete.md) |
| `mr group edit-description` | Edit a group's description | [Details](./group/edit-description.md) |
| `mr group edit-meta` | Edit a single metadata field by JSON path | [Details](./group/edit-meta.md) |
| `mr group edit-name` | Edit a group's name | [Details](./group/edit-name.md) |
| `mr group export` | Export one or more groups to a tar archive | [Details](./group/export.md) |
| `mr group get` | Get a group by ID | [Details](./group/get.md) |
| `mr group import` | Import a group export tar into this instance | [Details](./group/import.md) |
| `mr group parents` | List parent groups of a group | [Details](./group/parents.md) |
| `mr groups` | List, merge, or bulk-edit groups | [Details](./groups/index.md) |
| `mr groups add-meta` | Add metadata to multiple groups | [Details](./groups/add-meta.md) |
| `mr groups add-tags` | Add tags to multiple groups | [Details](./groups/add-tags.md) |
| `mr groups delete` | Delete multiple groups | [Details](./groups/delete.md) |
| `mr groups list` | List groups | [Details](./groups/list.md) |
| `mr groups merge` | Merge groups into a winner | [Details](./groups/merge.md) |
| `mr groups meta-keys` | List all unique metadata keys used across groups | [Details](./groups/meta-keys.md) |
| `mr groups remove-tags` | Remove tags from multiple groups | [Details](./groups/remove-tags.md) |
| `mr groups timeline` | Display a timeline of group activity | [Details](./groups/timeline.md) |
| `mr job` | Submit, cancel, pause, or retry a download job | [Details](./job/index.md) |
| `mr job cancel` | Cancel a job | [Details](./job/cancel.md) |
| `mr job pause` | Pause a job | [Details](./job/pause.md) |
| `mr job resume` | Resume a job | [Details](./job/resume.md) |
| `mr job retry` | Retry a failed job | [Details](./job/retry.md) |
| `mr job submit` | Submit URLs for download | [Details](./job/submit.md) |
| `mr jobs` | View the download job queue | [Details](./jobs/index.md) |
| `mr jobs list` | List the download queue | [Details](./jobs/list.md) |
| `mr log` | View a log entry or entity history | [Details](./log/index.md) |
| `mr log entity` | Get log entries for a specific entity | [Details](./log/entity.md) |
| `mr log get` | Get a log entry by ID | [Details](./log/get.md) |
| `mr logs` | List and filter audit log entries | [Details](./logs/index.md) |
| `mr logs list` | List log entries | [Details](./logs/list.md) |
| `mr mrql` | Execute and manage MRQL queries | [Details](./mrql/index.md) |
| `mr mrql delete` | Delete a saved MRQL query by ID | [Details](./mrql/delete.md) |
| `mr mrql list` | List saved MRQL queries | [Details](./mrql/list.md) |
| `mr mrql run` | Run a saved MRQL query by name or ID | [Details](./mrql/run.md) |
| `mr mrql save` | Save a MRQL query | [Details](./mrql/save.md) |
| `mr note` | Get, create, edit, delete, or share a note | [Details](./note/index.md) |
| `mr note create` | Create a new note | [Details](./note/create.md) |
| `mr note delete` | Delete a note by ID | [Details](./note/delete.md) |
| `mr note edit-description` | Edit a note's description | [Details](./note/edit-description.md) |
| `mr note edit-meta` | Edit a single metadata field by JSON path | [Details](./note/edit-meta.md) |
| `mr note edit-name` | Edit a note's name | [Details](./note/edit-name.md) |
| `mr note get` | Get a note by ID | [Details](./note/get.md) |
| `mr note share` | Generate a share token for a note | [Details](./note/share.md) |
| `mr note unshare` | Remove the share token from a note | [Details](./note/unshare.md) |
| `mr note-block` | Get, create, update, or delete a note block | [Details](./note-block/index.md) |
| `mr note-block create` | Create a new note block | [Details](./note-block/create.md) |
| `mr note-block delete` | Delete a note block by ID | [Details](./note-block/delete.md) |
| `mr note-block get` | Get a note block by ID | [Details](./note-block/get.md) |
| `mr note-block types` | Show available block types (text, table, calendar, etc.) | [Details](./note-block/types.md) |
| `mr note-block update` | Update a note block's content | [Details](./note-block/update.md) |
| `mr note-block update-state` | Update a note block's state | [Details](./note-block/update-state.md) |
| `mr note-blocks` | List, reorder, or rebalance note blocks | [Details](./note-blocks/index.md) |
| `mr note-blocks list` | List note blocks for a note | [Details](./note-blocks/list.md) |
| `mr note-blocks rebalance` | Rebalance note block positions | [Details](./note-blocks/rebalance.md) |
| `mr note-blocks reorder` | Reorder note blocks | [Details](./note-blocks/reorder.md) |
| `mr note-type` | Get, create, edit, or delete a note type | [Details](./note-type/index.md) |
| `mr note-type create` | Create a new note type | [Details](./note-type/create.md) |
| `mr note-type delete` | Delete a note type by ID | [Details](./note-type/delete.md) |
| `mr note-type edit` | Edit a note type | [Details](./note-type/edit.md) |
| `mr note-type edit-description` | Edit a note type's description | [Details](./note-type/edit-description.md) |
| `mr note-type edit-name` | Edit a note type's name | [Details](./note-type/edit-name.md) |
| `mr note-type get` | Get a note type by ID | [Details](./note-type/get.md) |
| `mr note-types` | List note types | [Details](./note-types/index.md) |
| `mr note-types list` | List note types | [Details](./note-types/list.md) |
| `mr notes` | List notes and bulk tag/group/meta operations | [Details](./notes/index.md) |
| `mr notes add-groups` | Add groups to multiple notes | [Details](./notes/add-groups.md) |
| `mr notes add-meta` | Add metadata to multiple notes | [Details](./notes/add-meta.md) |
| `mr notes add-tags` | Add tags to multiple notes | [Details](./notes/add-tags.md) |
| `mr notes delete` | Delete multiple notes | [Details](./notes/delete.md) |
| `mr notes list` | List notes | [Details](./notes/list.md) |
| `mr notes meta-keys` | List all unique metadata keys used across notes | [Details](./notes/meta-keys.md) |
| `mr notes remove-tags` | Remove tags from multiple notes | [Details](./notes/remove-tags.md) |
| `mr notes timeline` | Display a timeline of note activity | [Details](./notes/timeline.md) |
| `mr plugin` | Enable, disable, or configure a plugin | [Details](./plugin/index.md) |
| `mr plugin disable` | Disable a plugin | [Details](./plugin/disable.md) |
| `mr plugin enable` | Enable a plugin | [Details](./plugin/enable.md) |
| `mr plugin purge-data` | Purge all data for a plugin | [Details](./plugin/purge-data.md) |
| `mr plugin settings` | Update plugin settings (pass JSON via --data) | [Details](./plugin/settings.md) |
| `mr plugins` | List installed plugins | [Details](./plugins/index.md) |
| `mr plugins list` | List plugins and management info | [Details](./plugins/list.md) |
| `mr queries` | List saved queries | [Details](./queries/index.md) |
| `mr queries list` | List queries | [Details](./queries/list.md) |
| `mr queries timeline` | Display a timeline of query activity | [Details](./queries/timeline.md) |
| `mr query` | Get, create, run, or delete a saved query | [Details](./query/index.md) |
| `mr query create` | Create a new query | [Details](./query/create.md) |
| `mr query delete` | Delete a query by ID | [Details](./query/delete.md) |
| `mr query edit-description` | Edit a query's description | [Details](./query/edit-description.md) |
| `mr query edit-name` | Edit a query's name | [Details](./query/edit-name.md) |
| `mr query get` | Get a query by ID | [Details](./query/get.md) |
| `mr query run` | Run a query by ID | [Details](./query/run.md) |
| `mr query run-by-name` | Run a query by name | [Details](./query/run-by-name.md) |
| `mr query schema` | Show database table and column names for query building | [Details](./query/schema.md) |
| `mr relation` | Create, edit, or delete a group relation | [Details](./relation/index.md) |
| `mr relation create` | Create a new group relation | [Details](./relation/create.md) |
| `mr relation delete` | Delete a relation by ID | [Details](./relation/delete.md) |
| `mr relation edit-description` | Edit a relation's description | [Details](./relation/edit-description.md) |
| `mr relation edit-name` | Edit a relation's name | [Details](./relation/edit-name.md) |
| `mr relation-type` | Create, edit, or delete a relation type | [Details](./relation-type/index.md) |
| `mr relation-type create` | Create a new relation type | [Details](./relation-type/create.md) |
| `mr relation-type delete` | Delete a relation type by ID | [Details](./relation-type/delete.md) |
| `mr relation-type edit` | Edit a relation type | [Details](./relation-type/edit.md) |
| `mr relation-type edit-description` | Edit a relation type's description | [Details](./relation-type/edit-description.md) |
| `mr relation-type edit-name` | Edit a relation type's name | [Details](./relation-type/edit-name.md) |
| `mr relation-types` | List relation types | [Details](./relation-types/index.md) |
| `mr relation-types list` | List relation types | [Details](./relation-types/list.md) |
| `mr resource` | Upload, download, edit, or version a resource | [Details](./resource/index.md) |
| `mr resource delete` | Delete a resource by ID | [Details](./resource/delete.md) |
| `mr resource download` | Download a resource file | [Details](./resource/download.md) |
| `mr resource edit` | Edit a resource | [Details](./resource/edit.md) |
| `mr resource edit-description` | Edit a resource's description | [Details](./resource/edit-description.md) |
| `mr resource edit-meta` | Edit a single metadata field by JSON path | [Details](./resource/edit-meta.md) |
| `mr resource edit-name` | Edit a resource's name | [Details](./resource/edit-name.md) |
| `mr resource from-local` | Create a resource from a local server path | [Details](./resource/from-local.md) |
| `mr resource from-url` | Create a resource from a remote URL | [Details](./resource/from-url.md) |
| `mr resource get` | Get a resource by ID | [Details](./resource/get.md) |
| `mr resource preview` | Download a scaled thumbnail of a resource | [Details](./resource/preview.md) |
| `mr resource recalculate-dimensions` | Recalculate resource dimensions | [Details](./resource/recalculate-dimensions.md) |
| `mr resource rotate` | Rotate a resource image | [Details](./resource/rotate.md) |
| `mr resource upload` | Upload a file as a new resource | [Details](./resource/upload.md) |
| `mr resource version` | Get a specific version by ID | [Details](./resource/version.md) |
| `mr resource version-delete` | Delete a specific version | [Details](./resource/version-delete.md) |
| `mr resource version-download` | Download a specific version file | [Details](./resource/version-download.md) |
| `mr resource version-restore` | Restore a resource to a previous version | [Details](./resource/version-restore.md) |
| `mr resource version-upload` | Upload a new version of a resource | [Details](./resource/version-upload.md) |
| `mr resource versions` | List versions of a resource | [Details](./resource/versions.md) |
| `mr resource versions-cleanup` | Clean up old versions of a resource | [Details](./resource/versions-cleanup.md) |
| `mr resource versions-compare` | Compare two versions of a resource | [Details](./resource/versions-compare.md) |
| `mr resource-categories` | List resource categories | [Details](./resource-categories/index.md) |
| `mr resource-categories list` | List resource categories | [Details](./resource-categories/list.md) |
| `mr resource-category` | Get, create, edit, or delete a resource category | [Details](./resource-category/index.md) |
| `mr resource-category create` | Create a new resource category | [Details](./resource-category/create.md) |
| `mr resource-category delete` | Delete a resource category by ID | [Details](./resource-category/delete.md) |
| `mr resource-category edit-description` | Edit a resource category's description | [Details](./resource-category/edit-description.md) |
| `mr resource-category edit-name` | Edit a resource category's name | [Details](./resource-category/edit-name.md) |
| `mr resource-category get` | Get a resource category by ID | [Details](./resource-category/get.md) |
| `mr resources` | List, merge, or bulk-edit resources | [Details](./resources/index.md) |
| `mr resources add-groups` | Add groups to multiple resources | [Details](./resources/add-groups.md) |
| `mr resources add-meta` | Add metadata to multiple resources | [Details](./resources/add-meta.md) |
| `mr resources add-tags` | Add tags to multiple resources | [Details](./resources/add-tags.md) |
| `mr resources delete` | Delete multiple resources | [Details](./resources/delete.md) |
| `mr resources list` | List resources | [Details](./resources/list.md) |
| `mr resources merge` | Merge resources into a winner | [Details](./resources/merge.md) |
| `mr resources meta-keys` | List all unique metadata keys used across resources | [Details](./resources/meta-keys.md) |
| `mr resources remove-tags` | Remove tags from multiple resources | [Details](./resources/remove-tags.md) |
| `mr resources replace-tags` | Replace tags on multiple resources | [Details](./resources/replace-tags.md) |
| `mr resources set-dimensions` | Set dimensions on multiple resources | [Details](./resources/set-dimensions.md) |
| `mr resources timeline` | Display a timeline of resource activity | [Details](./resources/timeline.md) |
| `mr resources versions-cleanup` | Clean up old versions across resources | [Details](./resources/versions-cleanup.md) |
| `mr search` | Search across all entities | [Details](./search.md) |
| `mr series` | Manage resource series (list, create, edit, delete) | [Details](./series/index.md) |
| `mr series create` | Create a new series | [Details](./series/create.md) |
| `mr series delete` | Delete a series by ID | [Details](./series/delete.md) |
| `mr series edit` | Edit a series | [Details](./series/edit.md) |
| `mr series edit-name` | Edit a series name | [Details](./series/edit-name.md) |
| `mr series get` | Get a series by ID | [Details](./series/get.md) |
| `mr series list` | List series | [Details](./series/list.md) |
| `mr series remove-resource` | Remove a resource from its series | [Details](./series/remove-resource.md) |
| `mr tag` | Get, create, edit, or delete a tag | [Details](./tag/index.md) |
| `mr tag create` | Create a new tag | [Details](./tag/create.md) |
| `mr tag delete` | Delete a tag by ID | [Details](./tag/delete.md) |
| `mr tag edit-description` | Edit a tag's description | [Details](./tag/edit-description.md) |
| `mr tag edit-name` | Edit a tag's name | [Details](./tag/edit-name.md) |
| `mr tag get` | Get a tag by ID | [Details](./tag/get.md) |
| `mr tags` | List, merge, or bulk-delete tags | [Details](./tags/index.md) |
| `mr tags delete` | Delete multiple tags | [Details](./tags/delete.md) |
| `mr tags list` | List tags | [Details](./tags/list.md) |
| `mr tags merge` | Merge tags into a winner | [Details](./tags/merge.md) |
| `mr tags timeline` | Display a timeline of tag activity | [Details](./tags/timeline.md) |
