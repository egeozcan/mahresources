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

### Templates

**Template comment**:
A region of a template or a Description that is removed before it renders. Nothing inside it runs, and none of it is rendered.
_Avoid_: HTML comment (that one is sent to the browser), commented-out shortcode

### Template generation

**Generation run**:
One user request to have an AI provider draft template content, from the approved Disclosure to the returned Draft. It is made of one or more Turns.
_Avoid_: generation, session, job, conversation

**Turn**:
One exchange with the AI provider inside a Generation run: the model either asks for a Tool or submits a Draft.
_Avoid_: call, step, round, iteration

**Tool**:
A read-only operation the model may ask the server to perform during a Turn, such as linting or previewing its own Draft, or reading something inside the Envelope.
_Avoid_: function, action, command

**Disclosure**:
The statement, shown to the user for approval before a Generation run starts, of where the run sends data and what it may send.
_Avoid_: consent dialog, prompt preview

**Envelope**:
What a Generation run's Tools may read and do, as approved in the Disclosure: everything the account can read, plus whichever opt-ins the user ticked for that run. A Tool asking for anything outside it is refused.
_Avoid_: scope (that is the account's subtree permission, which bounds the Envelope), context, grounding

**Seed**:
The entities and editor content a Generation run's first Turn carries without the model asking for them. It is where the model starts, not a limit on what it may read.
_Avoid_: samples (those are one part of the Seed), grounding

**Related entity**:
An entity one step from another along ownership (up to the owner, down to what it owns) or along a link (Group Relations, related Resources, shared Groups and Notes).
_Avoid_: child (that is only the downward direction), neighbour, linked entity

**Draft**:
Template content the model submitted at the end of a Generation run. It is held for the user's review as text and reaches the editors only when the user applies it.
_Avoid_: generation, result, suggestion, output
