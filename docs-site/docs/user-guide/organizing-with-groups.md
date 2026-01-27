---
sidebar_position: 4
---

# Organizing with Groups

Groups are the primary organizational structure in Mahresources. They support hierarchical nesting, can own other entities, and form relationships with each other through typed relations.

## Understanding Groups

Groups serve multiple purposes:

- **Containers** - Hold collections of resources, notes, and sub-groups
- **Categories** - Organize items by topic, project, or any classification
- **Ownership** - Define who/what owns a piece of content
- **Relationships** - Connect to other groups with typed relations

### Owned vs Related

Every entity (resource, note, sub-group) can have two types of connections to a group:

| Connection Type | Description | Use Case |
|-----------------|-------------|----------|
| **Owned** | The group is the owner of the entity | Primary location, authorship, responsibility |
| **Related** | The group is associated with the entity | Cross-references, topics, secondary classifications |

An entity can have one owner but many relations.

## Creating Groups

### Basic Creation

1. Navigate to **Groups** in the top menu
2. Click **New Group**
3. Fill in required fields:
   - **Category** - The type of group (required for new groups)
   - **Name** - Display name (required)
4. Optionally add:
   - **Description** - Text describing the group
   - **URL** - External link associated with the group
   - **Tags** - Labels for the group itself
   - **Groups** - Related groups
   - **Owner** - Parent group that owns this one
   - **Meta** - Custom metadata
5. Click **Save**

### Creating Sub-Groups

Create groups nested under a parent:

1. Navigate to the parent group's detail page
2. In **Own Entities**, find **Sub-Groups**
3. Click **Add New**
4. The owner is pre-filled with the parent group
5. Fill in remaining fields and save

## Group Hierarchy

Groups form a hierarchical tree through ownership:

```
Project Alpha (Group)
  Documents (Sub-Group)
    Meeting Notes (Note)
    Spec Document (Resource)
  Media (Sub-Group)
    Photos (Sub-Sub-Group)
      photo1.jpg (Resource)
```

### Navigating Hierarchy

- **Breadcrumbs** - Show the path from root to current group
- **Own Entities > Sub-Groups** - Lists direct children
- **Owner** - Shows the parent group in the sidebar

### Viewing Group Contents

The group detail page shows content in expandable sections:

**Own Entities** (items this group owns)
- Notes - Text content owned by this group
- Sub-Groups - Child groups
- Resources - Files owned by this group

**Related Entities** (items associated with this group)
- Related Groups - Other groups linked to this one
- Related Resources - Files associated but not owned
- Related Notes - Notes associated but not owned

## Categories

Categories define the type of a group and can provide:

- A classification system for groups
- Custom metadata schemas
- Custom header/sidebar templates

### Using Categories

When creating a group, you must select a category. Categories help organize groups themselves:

```
Category: Person
  - John Smith
  - Jane Doe

Category: Project
  - Website Redesign
  - Mobile App
```

### Category Metadata Schemas

Categories can define JSON schemas for group metadata. When a category has a schema, the group edit form shows a structured form instead of free-form fields.

## Group Relations

Relations connect groups with typed, directional relationships.

### Understanding Relations

A relation has:
- **From Group** - The source group
- **To Group** - The target group
- **Relation Type** - Defines the nature of the connection
- **Name** (optional) - Specific instance name
- **Description** (optional) - Details about this relationship

Example: "John Smith" --[works at]--> "Acme Corp"

### Creating Relations

1. Navigate to a group's detail page
2. In the **Relations** section, click **Add New**
3. Select:
   - **Type** - The relation type
   - **From Group** - Source (may be pre-filled)
   - **To Group** - Target
4. Optionally add name and description
5. Click **Save**

### Viewing Relations

On a group's detail page, the **Relations** section shows:

- **Relations** - Outgoing relations (this group -> others)
- **Reverse Relations** - Incoming relations (others -> this group)

Each relation links to the connected group.

### Relation Types

Define relation types before creating relations:

1. Navigate to **Relation Types** > **New Relation Type**
2. Enter:
   - **Name** - Describes the relationship (e.g., "works at", "parent of")
   - **Category filters** - Restrict which categories can participate
3. Click **Save**

## Merging Groups

Combine multiple groups into one:

1. Navigate to the group that should remain (the "winner")
2. In the sidebar, find the **Merge** section
3. Use the autocomplete to select groups to merge
4. Click **Merge**

The merge operation:
- Moves all owned content to the winner group
- Updates all relations to point to the winner
- Deletes the merged ("loser") groups

:::warning

Merging is irreversible. The merged groups are permanently deleted.

:::

## Cloning Groups

Create a copy of a group:

1. Navigate to the group detail page
2. In the sidebar, find **Clone group?**
3. Click **Clone**

Cloning creates a new group with:
- The same name, description, and metadata
- The same category
- New unique ID

Note: Cloning does not copy owned content or relations.

## Group Metadata

### Schema-Based Metadata

When the group's category has a metadata schema defined, the edit form shows structured fields matching the schema. This ensures consistent data across groups of the same type.

### Free-Form Metadata

Without a schema, use the **Meta Data** section to add arbitrary key-value pairs:

1. Enter a key name
2. Enter a value
3. Add more fields as needed

## Working with Owned Content

### Adding Owned Items

From a group's detail page:

1. Expand **Own Entities**
2. Find the section (Notes, Sub-Groups, or Resources)
3. Click **Add New**
4. Complete the creation form (owner is pre-filled)

### Viewing All Owned Items

Click **See All** next to any owned entity section to view a filtered list of all items owned by this group.

### Transferring Ownership

To move content to a different group:

1. Edit the item (resource, note, or group)
2. Change the **Owner** field to the new group
3. Save

## Tips for Organization

### Hierarchical vs Flat

Choose based on your content:
- **Hierarchical** - When there's a natural parent-child relationship
- **Flat with Relations** - When items connect in complex, non-hierarchical ways

### Categories for Structure

Use categories to enforce structure:
- Create categories for different entity types (Person, Project, Topic)
- Define metadata schemas for consistent data capture
- Use category-specific templates for display customization

### Tags vs Groups

- **Tags** - Quick labels, many per item, no hierarchy
- **Groups** - Containers with hierarchy, ownership, and relations

Use tags for attributes; use groups for structure.
