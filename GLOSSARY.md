# Mahresources

A personal information manager for files (Resources), Notes, Groups and the taxonomy that connects them.

## Language

### Entity text

**Description**:
The free-text body of an entity, written in markdown and shown rendered on the entity's detail page. A Note's "Text" is its Description.
_Avoid_: Text field, body, notes

**Text block**:
A block of markdown text inside a Note's block list. A Note's first Text block mirrors its Description, but a Text block is not a Description.
_Avoid_: Note text, paragraph

**Query Text**:
The SQL a saved Query runs. It is code, not prose, and is never rendered as markdown.
_Avoid_: Query description, query body
