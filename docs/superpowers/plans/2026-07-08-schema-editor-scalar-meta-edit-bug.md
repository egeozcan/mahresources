# Schema Editor Scalar Meta Edit Bug

Date: 2026-07-08
Status: open

## Summary

Editable `[meta]` shortcodes backed by a scalar schema can open in edit mode with
`[object Object]` in the input, even though the shortcode's `data-value` is a
valid JSON scalar string.

This is separate from the Alpine morph corruption fixed in the controlled
shortcode morph work. The morph fix makes editable meta shortcodes inside
custom/deferred template content usable, which makes this older schema-editor
value-shape bug easier to hit.

## Symptom

For a resource category with a schema property like:

```json
{
  "type": "object",
  "properties": {
    "notes": { "title": "Notes", "type": "string" }
  }
}
```

and a custom template containing:

```html
[meta path="notes" editable="true"]
```

clicking the edit pencil can render a text input whose value is:

```text
[object Object]
```

The shortcode element still has the correct attribute state before edit mode:

```html
<meta-shortcode
  data-path="notes"
  data-schema='{"title":"Notes","type":"string"}'
  data-value='"Initial notes for image 3"'>
</meta-shortcode>
```

## Reproduction Notes

The bug was reproduced on the local ephemeral test server at:

```text
http://127.0.0.1:18181/resources?ResourceCategoryId=2
```

Steps:

1. Open a resource category whose custom summary includes editable `[meta]`.
2. Open a deferred `[details]` block containing `Notes: [meta path="notes" editable="true"]`.
3. Click the edit pencil for the Notes meta shortcode.
4. The edit input displays `[object Object]` instead of the stored notes string.

Observed live state after opening edit mode:

```js
schemaEditor.value === '"Initial notes for image 3"'
schemaFormMode.value === 'Initial notes for image 3'
schemaFormMode._data === {}
input.value === '[object Object]'
```

## Likely Root Cause

There are two scalar/object assumptions fighting each other:

1. `src/schema-editor/schema-editor.ts` parses `.value` from JSON before passing it
   to `<schema-form-mode>`. For a JSON string value, that produces the scalar
   string `Initial notes for image 3`.
2. `src/schema-editor/modes/form-mode.ts` then treats any string `value` as if it
   were a JSON string that still needs parsing:

```ts
this._data = this.value != null
  ? (typeof this.value === 'string' ? this._safeParse(this.value) : structuredClone(this.value))
  : {};
```

`_safeParse("Initial notes for image 3")` fails and returns `{}`. The string
field renderer then passes that object into an `<input>`, which the browser
coerces to `[object Object]`.

This parsing behavior exists in the committed baseline, so it should be treated
as a pre-existing schema-editor bug.

## Candidate Fix

Make `schema-form-mode` preserve already-parsed scalar values.

Possible approach:

```ts
private _normalizeValue(value: any): any {
  if (value == null) return {};
  if (typeof value !== 'string') return structuredClone(value);

  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}
```

Then use `_normalizeValue(this.value)` in `willUpdate`.

Check whether root object schemas still need `{}` as the empty fallback. A safer
version may use the current schema type:

- object schema + null/undefined/invalid JSON -> `{}`
- array schema + null/undefined/invalid JSON -> `[]`
- scalar schema + failed JSON parse -> original string

## Suggested Tests

Add unit coverage around `schema-form-mode` or `schema-editor` for:

1. String schema with JSON string value renders the raw scalar text, not
   `[object Object]`.
2. String schema with plain string property value passed directly to
   `schema-form-mode` remains a string.
3. Object schema still parses JSON object strings and still falls back to `{}` on
   invalid object payloads.
4. Editable `[meta path="notes"]` with `data-schema='{"type":"string"}'` opens
   edit mode with the stored string.

## Related Files

- `src/schema-editor/schema-editor.ts`
- `src/schema-editor/modes/form-mode.ts`
- `src/webcomponents/meta-shortcode.ts`
