---
sidebar_position: 3
---

# First Steps

## The Data Model

Five main entity types form the core data model:

- **Resources** - Files you upload (images, documents, videos, etc.)
- **Notes** - Text content you create
- **Groups** - Collections that contain Resources, Notes, and other Groups
- **Tags** - Labels you attach to Resources, Notes, and Groups
- **Categories** - Types for your Groups (e.g., Person, Project, Topic)

## Step 1: Create a Category

Categories define what kind of thing a Group represents (e.g., "Person", "Project", "Location"). Groups can optionally have a Category, but creating a few early helps keep things organized.

1. Navigate to **Categories** under the **Admin** dropdown in the top navigation bar
2. Click the **Add** button
3. Enter a name like "Project"
4. Click **Save**

## Step 2: Create Some Tags

Tags are labels you can attach to Resources, Notes, and Groups.

1. Navigate to **Tags** in the top navigation bar
2. Click the **Add** button
3. Enter "In Progress" as the name
4. Add an optional description
5. Click **Save**

Repeat to create "Completed" and "On Hold" tags.

## Step 3: Create a Group

Groups hold related Resources, Notes, and other Groups.

1. Navigate to **Groups** in the top navigation bar
2. Click the **Add** button
3. Enter a name like "Research Project"
4. Add an optional description
5. Click **Save**

Set a parent Group to create a hierarchy.

## Step 4: Upload Resources

Add files to your Group.

1. Navigate to **Resources** in the top navigation bar
2. Click the **Create** button
3. Select one or more files to upload
4. Add a name and description
5. Under **Groups**, select "Research Project"
6. Under **Tags**, select "In Progress"
7. Click **Save**

Each Resource can belong to multiple Groups and have multiple Tags.

## Step 5: Create a Note

Add a Note linked to your Group.

1. Navigate to **Notes** in the top navigation bar
2. Click the **Create** button
3. Enter a title like "Initial Observations"
4. Write your note text
5. Optionally select a Note Type
6. Under **Groups**, select "Research Project"
7. Optionally link to specific Resources
8. Click **Save**

## Step 6: Use Global Search

Global search finds items across all entity types -- resources, notes, groups, tags, categories, and more.

1. Press **Cmd+K** (Mac) or **Ctrl+K** (Windows/Linux)
2. Start typing your search query
3. Results appear instantly, showing Resources, Notes, and Groups
4. Click a result to navigate directly to it

Results come from the FTS5 full-text index and appear as you type.

## Step 7: Explore Relationships

See how items connect to each other.

1. Open the "Research Project" group
2. See all Resources and Notes in this group
3. Click on a Resource to see its details, including all Groups and Tags
4. Use the related items to navigate between connected content

## Tips for Effective Organization

- **Use Groups for projects or themes** - Keep related items together
- **Use Tags for cross-cutting concerns** - Status, priority, type of content
- **Use Categories to type your Groups** - e.g., Person, Project, Location
- **Link Notes to Resources** - Connect your thoughts to source materials
- **Use global search often** - It's faster than clicking through menus

## What's Next?

From here, you can:

- Create **Saved Queries** to store and re-run searches
- Set up **Group Relations** to link Groups to each other with typed relationships
- Use the **JSON API** to script and automate operations
- Enable **Image Similarity** to find visually related images
