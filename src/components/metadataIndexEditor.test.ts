import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { expect, test } from 'vitest';

const template = readFileSync(new URL('../../templates/partials/form/metadataIndexes.tpl', import.meta.url), 'utf8');
const script = template.match(/<script>([\s\S]*?)<\/script>/)![1];
const factory = runInNewContext(script + '\nwindow.metadataIndexEditor;', { window: {} });

test('category editor submits key and kind without a selectable entity', () => {
    const editor = factory('');
    editor.entries.push({ key: 'score', kind: 'numeric' });
    editor.entries.push({ key: 'camera.model', kind: 'text' });
    expect(JSON.parse(editor.serialized)).toEqual([
        { key: 'score', kind: 'numeric' }, { key: 'camera.model', kind: 'text' },
    ]);
    editor.entries.splice(0, 1);
    expect(JSON.parse(editor.serialized)).toEqual([{ key: 'camera.model', kind: 'text' }]);
    editor.entries.splice(0, 1);
    expect(editor.serialized).toBe('[]');
});

test('existing declarations load without losing their kinds or nested paths', () => {
    const raw = '[{"key":"camera.iso","kind":"numeric"}]';
    expect(factory(raw).serialized).toBe(raw);
});

test.each(['invalid', 'null', '[null]', '[{"key":"score","kind":"unknown"}]'])('invalid stored data %s is preserved until explicitly cleared', (raw) => {
    const editor = factory(raw);
    expect(editor.error).toContain('Invalid index definitions');
    expect(editor.serialized).toBe(raw);
    editor.entries = [];
    editor.error = '';
    expect(editor.serialized).toBe('[]');
});
