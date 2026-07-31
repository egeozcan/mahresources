import { describe, it, expect } from 'vitest';
import { mrqlEditor } from './mrqlEditor.js';

// The bucketed half of the /mrql column-ordering finding (docs/todo.md, WS11
// round 3 + Batch 13). `9ab65b12` fixed the aggregated table header and left the
// bucket key badges iterating `bucket.key` — Object.keys over an object Go
// marshals from a map, and encoding/json sorts map keys, so the order the author
// wrote was discarded before the browser ever saw it.
//
// The server now sends `keyColumns`. These cover the ordering function on its
// own; the badge markup that consumes it is in templates/mrql.tpl.

function editor() {
  // mrqlEditor is an Alpine.data factory; its pure state works standalone as
  // long as init() is never called (that is what needs the DOM and CodeMirror).
  return mrqlEditor();
}

describe('bucketKeyOrder', () => {
  it('orders badges by the authored GROUP BY, not by the object key order', () => {
    const e = editor();
    // What the wire actually looks like: encoding/json emitted the key object
    // sorted, and keyColumns carries the order the query was written in.
    e.result = { mode: 'bucketed', keyColumns: ['width', 'height', 'contentType'] };
    const bucket = { key: { contentType: 'image/png', height: 50, width: 100 } };

    expect(e.bucketKeyOrder(bucket)).toEqual(['width', 'height', 'contentType']);
  });

  it('follows the query when it is written the other way round', () => {
    const e = editor();
    e.result = { mode: 'bucketed', keyColumns: ['contentType', 'height', 'width'] };
    // The same sorted payload — this is the whole point: two different queries
    // used to be indistinguishable on screen.
    const bucket = { key: { contentType: 'image/png', height: 50, width: 100 } };

    expect(e.bucketKeyOrder(bucket)).toEqual(['contentType', 'height', 'width']);
  });

  it('keeps the relation id badge the server adds for disambiguation', () => {
    const e = editor();
    // GROUP BY owner: the server renames its internal _gbid_owner to owner_id so
    // two groups with the same name stay distinguishable. It is not a group-by
    // column, so it is not in keyColumns — and it must still be rendered.
    e.result = { mode: 'bucketed', keyColumns: ['owner'] };
    const bucket = { key: { owner: 'Photos', owner_id: 12 } };

    expect(e.bucketKeyOrder(bucket)).toEqual(['owner', 'owner_id']);
  });

  it('falls back to the object key order when the server sent no keyColumns', () => {
    // An older cached response, or a shape this build does not know about,
    // renders as it always did rather than blank.
    const e = editor();
    e.result = { mode: 'bucketed' };
    const bucket = { key: { contentType: 'image/png', width: 100 } };

    expect(e.bucketKeyOrder(bucket)).toEqual(['contentType', 'width']);
  });

  it('ignores a named column that is not in this bucket', () => {
    const e = editor();
    e.result = { mode: 'bucketed', keyColumns: ['width', 'missing', 'height'] };
    const bucket = { key: { height: 50, width: 100 } };

    expect(e.bucketKeyOrder(bucket)).toEqual(['width', 'height']);
  });

  it('returns nothing for a bucket with no key', () => {
    const e = editor();
    e.result = { mode: 'bucketed', keyColumns: ['width'] };

    expect(e.bucketKeyOrder({})).toEqual([]);
    expect(e.bucketKeyOrder(undefined)).toEqual([]);
  });
});
