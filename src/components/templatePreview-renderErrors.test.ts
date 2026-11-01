// @vitest-environment happy-dom

import { afterEach, describe, expect, test, vi } from 'vitest';
// The bundle's user-settings module loads from the server on import once a
// document exists; answer it before the import runs, rather than letting it
// reach for a server that is not there.
vi.hoisted(() => {
    globalThis.fetch = (async () => ({ ok: false, status: 401, json: async () => ({}) })) as unknown as typeof fetch;
});

import { renderErrorsFrom, templatePreview } from './templatePreview.js';

// A shortcode that fails renders a short inline marker in the frame ("⚠
// plugin:data-views:pie-chart") with the actual reason only in its title
// attribute, a tooltip an author has to find by hovering. The pane lists every
// such failure under the frame in full.

describe('renderErrorsFrom', () => {
    test('reads the label and the title detail of a shortcode error marker', () => {
        const html =
            '<div><span class="shortcode-error text-red-700 font-mono" title="plugin &quot;data-views&quot; is not enabled">⚠ plugin:data-views:pie-chart</span></div>';
        expect(renderErrorsFrom(html)).toEqual([
            { label: 'plugin:data-views:pie-chart', detail: 'plugin "data-views" is not enabled', count: 1 },
        ]);
    });

    test('reads an mrql error, whose text is the whole message', () => {
        const html = '<div class="mrql-results mrql-error">parse error at 1:5: unexpected token</div>';
        expect(renderErrorsFrom(html)).toEqual([
            { label: 'mrql', detail: 'parse error at 1:5: unexpected token', count: 1 },
        ]);
    });

    test("decodes Go's html.EscapeString output, numeric quote entities included", () => {
        const html =
            '<span class="shortcode-error text-red-700 font-mono" title="bad &#34;x&#34; &amp; &#39;y&#39; &lt;z&gt;">⚠ plugin:a:b</span>';
        expect(renderErrorsFrom(html)).toEqual([{ label: 'plugin:a:b', detail: 'bad "x" & \'y\' <z>', count: 1 }]);
    });

    test('reads the inline [mrql value=] error span', () => {
        const html = '<p>Total: <span class="mrql-error text-red-700 font-mono">unknown field &#34;foo&#34;</span></p>';
        expect(renderErrorsFrom(html)).toEqual([{ label: 'mrql', detail: 'unknown field "foo"', count: 1 }]);
    });

    test('collapses repeats, as an [each] loop repeats the same failure per item', () => {
        const marker = '<span class="shortcode-error" title="boom">⚠ plugin:x:y</span>';
        expect(renderErrorsFrom(marker + marker + marker)).toEqual([
            { label: 'plugin:x:y', detail: 'boom', count: 3 },
        ]);
    });

    test('finds markers inside Alpine templates, which the frame renders later', () => {
        const html =
            '<template x-if="true"><p><template x-if="ok"><span class="shortcode-error" title="boom">⚠ plugin:x:y</span></template></p></template>';
        expect(renderErrorsFrom(html)).toEqual([{ label: 'plugin:x:y', detail: 'boom', count: 1 }]);
    });

    test('finds a marker the server wrote into a <style> element', () => {
        const html = '<style>.a{}<span class="shortcode-error" title="needs [/lazy]">⚠ lazy</span></style>';
        expect(renderErrorsFrom(html)).toEqual([{ label: 'lazy', detail: 'needs [/lazy]', count: 1 }]);
    });

    test('an SVG <template>, which has no content fragment, does not throw', () => {
        const html = '<svg><template></template></svg><span class="shortcode-error" title="t">⚠ lazy</span>';
        expect(renderErrorsFrom(html)).toEqual([{ label: 'lazy', detail: 't', count: 1 }]);
    });

    test('returns nothing for a clean render', () => {
        expect(renderErrorsFrom('<h1>fine</h1>')).toEqual([]);
        expect(renderErrorsFrom('')).toEqual([]);
    });
});

describe('template preview refresh', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    test('lists the render errors of the html it receives', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn(async () => ({
                ok: true,
                json: async () => ({
                    html: '<span class="shortcode-error" title="renderer failed: nope">⚠ plugin:a:b</span>',
                    css: '',
                    entity: null,
                    issues: [],
                    cssIssues: [],
                }),
            })),
        );
        const component = templatePreview({ entityType: 'group', previewPath: '/p', categoryId: 7 });
        component._form = {
            querySelector: () => ({ value: '[plugin:a:b]' }),
        } as unknown as HTMLFormElement;
        component.$refs = {};
        component.entityId = 1;
        component.slot = 'CustomHeader';

        await component.refresh();

        expect(component.renderErrors).toEqual([{ label: 'plugin:a:b', detail: 'renderer failed: nope', count: 1 }]);
    });

    function previewWith(respond: () => unknown) {
        vi.stubGlobal('fetch', vi.fn(async () => respond()));
        const component = templatePreview({ entityType: 'group', previewPath: '/p', categoryId: 7 });
        component._form = {
            querySelector: () => ({ value: 'x' }),
        } as unknown as HTMLFormElement;
        component.$refs = {};
        component.entityId = 1;
        return component;
    }

    const ok = (body: Record<string, unknown>) => ({
        ok: true,
        json: async () => ({ html: '', css: '', entity: null, issues: [], cssIssues: [], ...body }),
    });
    const marker = (name: string) => `<span class="shortcode-error" title="why">⚠ ${name}</span>`;

    test('lists failures in the CSS buffer too, once each on the CSS slot', async () => {
        const component = previewWith(() => ok({ html: marker('plugin:h:h'), css: marker('plugin:c:c') }));
        component.slot = 'CustomHeader';
        await component.refresh();
        expect(component.renderErrors.map((e) => e.label)).toEqual(['plugin:h:h', 'plugin:c:c']);

        // On a CSS slot the request sends one buffer as both halves.
        const css = previewWith(() => ok({ html: marker('plugin:c:c'), css: marker('plugin:c:c') }));
        css.slot = 'CustomCSS';
        await css.refresh();
        expect(css.renderErrors).toEqual([{ label: 'plugin:c:c', detail: 'why', count: 1 }]);
    });

    test('a failed refresh does not leave the previous render errors standing', async () => {
        let respond: () => unknown = () => ok({ html: marker('plugin:a:b') });
        const component = previewWith(() => respond());
        component.slot = 'CustomHeader';
        await component.refresh();
        expect(component.renderErrors).toHaveLength(1);

        respond = () => ({ ok: false, status: 500, json: async () => ({}) });
        await component.refresh();
        expect(component.renderErrors).toEqual([]);

        respond = () => ok({ html: marker('plugin:a:b') });
        await component.refresh();
        respond = () => {
            throw new Error('network');
        };
        await component.refresh();
        expect(component.renderErrors).toEqual([]);
    });
});
