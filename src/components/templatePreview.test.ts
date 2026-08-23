import { afterEach, describe, expect, test, vi } from 'vitest';
import { templatePreview } from './templatePreview.js';

// The preview request carries the selected slot because that is the only thing
// that says whether `content` is markup or a stylesheet: with CustomCSS
// selected the editor sends that buffer as `content`, and a stylesheet has no
// <style> wrapper to say so. The server's lint of the same buffer turns on it,
// so dropping the field silently returns CustomCSS previews to being judged as
// markup — with every Go test still green.

function formWith(values: Record<string, string>) {
    return {
        querySelector: (selector: string) => {
            const match = /input\[name="(.+)"\]/.exec(selector);
            const name = match ? match[1] : '';
            return name in values ? { value: values[name] } : null;
        },
    } as unknown as HTMLFormElement;
}

function stubFetch(payload: Record<string, unknown> = {}) {
    const fetchMock = vi.fn(async () => ({
        ok: true,
        json: async () => ({ html: '', css: '', entity: null, issues: [], cssIssues: [], ...payload }),
    }));
    vi.stubGlobal('fetch', fetchMock);
    return fetchMock;
}

function sentBody(fetchMock: ReturnType<typeof stubFetch>) {
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, { body: string }];
    return JSON.parse(init.body);
}

describe('template preview request', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    test('names the selected slot so the server can judge CustomCSS as CSS', async () => {
        const fetchMock = stubFetch();
        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        component._form = formWith({
            CustomCSS: '.badge{color:red}',
            CustomHeader: '<h1>x</h1>',
        });
        component.$refs = {};
        component.entityId = 42;
        component.slot = 'CustomCSS';

        await component.refresh();

        const body = sentBody(fetchMock);
        expect(body.slot).toBe('CustomCSS');
        // With that slot selected the two buffers are one document, which is
        // what lets the server lint it once instead of reporting every issue
        // twice.
        expect(body.content).toBe(body.css);
    });

    test('names a markup slot too, so its content is not judged as a stylesheet', async () => {
        const fetchMock = stubFetch();
        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        component._form = formWith({
            CustomCSS: '.badge{color:red}',
            CustomHeader: '<h1>x</h1>',
        });
        component.$refs = {};
        component.entityId = 42;
        component.slot = 'CustomHeader';

        await component.refresh();

        const body = sentBody(fetchMock);
        expect(body.slot).toBe('CustomHeader');
        expect(body.content).toBe('<h1>x</h1>');
        expect(body.css).toBe('.badge{color:red}');
    });

    test('shows both issue lists, since the server splits them by buffer', async () => {
        // The response separates content findings from css findings because
        // their offsets index different buffers. The pane shows severity and
        // message, so it reads them as one list — and dropping either half
        // would silently hide half the diagnostics.
        const fetchMock = stubFetch({
            issues: [{ start: 0, end: 1, severity: 'warning', message: 'from content' }],
            cssIssues: [{ start: 0, end: 1, severity: 'warning', message: 'from css' }],
        });
        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        component._form = formWith({ CustomCSS: '.a{}', CustomHeader: '<h1>x</h1>' });
        component.$refs = {};
        component.entityId = 42;
        component.slot = 'CustomHeader';

        await component.refresh();

        expect(fetchMock).toHaveBeenCalledTimes(1);
        expect(component.issues.map((i: { message: string }) => i.message)).toEqual([
            'from content',
            'from css',
        ]);
    });

    test('carrier mode names its slot as well', async () => {
        const fetchMock = stubFetch();
        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        component._form = formWith({
            CustomCSS: '.badge{color:red}',
            CustomListHeader: '<header>x</header>',
        });
        component.$refs = {};
        component.slot = 'CustomListHeader';

        await component.refresh();

        const body = sentBody(fetchMock);
        expect(body.carrier).toBe(true);
        expect(body.slot).toBe('CustomListHeader');
    });
});

// The frame is one document built from two halves of the response: `css` goes
// into a <style> element, `html` into the body. Production's only sink for a
// CustomCSS buffer is the {% custom_css %} tag, which writes a <style> element
// and nothing else (custom_css_tag.go), so the frame has to treat that buffer
// the same way or it previews a page the saved template can never produce.
function frameStub() {
    return { srcdoc: '' } as unknown as HTMLIFrameElement;
}

// bodyOf returns the part of the built document from <body> on, which is the
// only part neither <style> element is in.
function bodyOf(srcdoc: string) {
    const at = srcdoc.indexOf('<body>');
    expect(at).toBeGreaterThan(-1);
    return srcdoc.slice(at);
}

describe('template preview frame', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    test('renders the CustomCSS buffer as a stylesheet, not also as body content', async () => {
        // With CustomCSS selected the pane sends one buffer as both `content`
        // and `css`, so the response carries the stylesheet twice. Rendering the
        // `html` half into the body printed the stylesheet's own source as page
        // text, which is the one thing saving the template cannot do.
        const css = '.badge{color:red}';
        const fetchMock = stubFetch({ html: css, css });
        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({ CustomCSS: css, CustomHeader: '<h1>x</h1>' });
        component.entityId = 42;
        component.slot = 'CustomCSS';

        await component.refresh();

        expect(fetchMock).toHaveBeenCalledTimes(1);
        expect(frame.srcdoc).toContain(`<style>${css}</style>`);
        expect(bodyOf(frame.srcdoc)).not.toContain(css);
    });

    test('a markup slot still renders its html in the body and its css in <style>', async () => {
        // The suppression is keyed on the selected slot, not on the two buffers
        // reading alike, so an ordinary slot keeps both halves.
        const fetchMock = stubFetch({ html: '<h1>rendered</h1>', css: '.badge{color:red}' });
        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({ CustomCSS: '.badge{color:red}', CustomHeader: '<h1>x</h1>' });
        component.entityId = 42;
        component.slot = 'CustomHeader';

        await component.refresh();

        expect(fetchMock).toHaveBeenCalledTimes(1);
        expect(frame.srcdoc).toContain('<style>.badge{color:red}</style>');
        expect(bodyOf(frame.srcdoc)).toContain('<h1>rendered</h1>');
    });
});
