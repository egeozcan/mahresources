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
// into a <style> element, `html` into the body. Every production sink for a
// CustomCSS buffer renders it as stylesheet content and nothing else -- the
// custom_css tag, the per-card block the MRQL paths prepend, the share page's
// head block -- so the frame has to treat that buffer the same way or it
// previews a page the saved template can never produce. See CSS_SLOT.
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

    test("emits the frame's own reset before the author CSS, as the real pages do", async () => {
        // base.tpl links the app stylesheets and only then renders the head
        // block, where the custom_css tag writes its <style>, so nothing of the
        // page's own outranks the author at equal specificity. The frame's
        // reset is preview chrome with no counterpart there; emitted after the
        // author CSS it silently outranked them, and a `body` rule the author
        // wrote previewed as having no effect at all. What the ordering does
        // not do is reproduce a real page, whose body carries classes that beat
        // a bare `body` selector either way; see _renderFrame.
        const fetchMock = stubFetch({ html: '', css: 'body{background:#000}' });
        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({ CustomCSS: 'body{background:#000}', CustomHeader: '' });
        component.entityId = 42;
        component.slot = 'CustomHeader';

        await component.refresh();

        expect(fetchMock).toHaveBeenCalledTimes(1);
        const reset = frame.srcdoc.indexOf('body{margin:0');
        const author = frame.srcdoc.indexOf('body{background:#000}');
        expect(reset).toBeGreaterThan(-1);
        expect(author).toBeGreaterThan(-1);
        expect(reset).toBeLessThan(author);
    });

    test('a refusal that sends no request retires the one already in flight', async () => {
        // The two branches that answer without a request - an unsaved carrier,
        // and no entity to render against - blank the frame and say why. They
        // have to claim the sequence number too, or the response for the slot
        // just left lands afterwards and overwrites the explanation with a
        // render the pane has already disowned. Both branches are exercised:
        // this one takes the missing-entity door, and the case below takes the
        // carrier door, because the two are separate copies of the same rule.
        let release: () => void = () => {};
        const pending = new Promise<void>((resolve) => {
            release = resolve;
        });
        const fetchMock = vi.fn(async () => {
            await pending;
            return {
                ok: true,
                json: async () => ({
                    html: '<h1>stale</h1>',
                    css: '',
                    entity: null,
                    issues: [],
                    cssIssues: [],
                }),
            };
        });
        vi.stubGlobal('fetch', fetchMock);

        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({ CustomCSS: '', CustomHeader: '<h1>x</h1>' });
        component.entityId = 42;
        component.slot = 'CustomHeader';

        const inFlight = component.refresh();
        // The chosen entity goes away while that request is out.
        component.entityId = null;
        await component.refresh();
        expect(component.error).toContain('Nothing to preview against yet');
        expect(bodyOf(frame.srcdoc)).not.toContain('stale');

        release();
        await inFlight;

        expect(bodyOf(frame.srcdoc)).not.toContain('stale');
        expect(component.error).toContain('Nothing to preview against yet');
        expect(component.loading).toBe(false);
    });

    test('the unsaved-carrier refusal retires an in-flight request too', async () => {
        // The carrier door is a separate copy of the same rule, so a cleanup
        // that tidied one and not the other would regress unnoticed.
        let release: () => void = () => {};
        const pending = new Promise<void>((resolve) => {
            release = resolve;
        });
        const fetchMock = vi.fn(async () => {
            await pending;
            return {
                ok: true,
                json: async () => ({
                    html: '<h1>stale</h1>',
                    css: '',
                    entity: null,
                    issues: [],
                    cssIssues: [],
                }),
            };
        });
        vi.stubGlobal('fetch', fetchMock);

        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({
            CustomCSS: '',
            CustomHeader: '<h1>x</h1>',
            CustomListHeader: '<header>x</header>',
        });
        component.entityId = 42;
        component.slot = 'CustomHeader';

        const inFlight = component.refresh();
        // Switched to a list-header slot on a form with no saved carrier.
        component.categoryId = null;
        component.slot = 'CustomListHeader';
        await component.refresh();
        expect(component.error).toContain('Save this category first');
        expect(bodyOf(frame.srcdoc)).not.toContain('stale');

        release();
        await inFlight;

        expect(bodyOf(frame.srcdoc)).not.toContain('stale');
        expect(component.error).toContain('Save this category first');
        expect(component.loading).toBe(false);
    });
});

describe('template preview startup', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    test('explains an empty deployment instead of leaving the frame a void', async () => {
        // init() used to call refresh() only when an entity had been found, so
        // the one case that has nothing to render against reached neither the
        // explanation nor the blanking -- an unexplained 384px void at exactly
        // the moment WS6 finding 29 was raised about.
        const fetchMock = vi.fn(async (url: string) => ({
            ok: true,
            json: async () => [] as unknown[],
            requested: url,
        }));
        vi.stubGlobal('fetch', fetchMock);
        vi.stubGlobal('localStorage', {
            getItem: () => null,
            setItem: () => {},
        });
        // No Alpine on it, so _publishStore returns without a store.
        vi.stubGlobal('window', {});

        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        const frame = frameStub();
        component.$refs = { frame };
        component.$root = { closest: () => null } as unknown as HTMLElement;

        await component.init();

        expect(component.entityId).toBe(null);
        expect(component.error).toContain('Nothing to preview against yet');
        // Blanked rather than left at whatever it last held.
        expect(frame.srcdoc).toContain('<body>');
        // The list lookups happened; no preview request was sent for them.
        expect(fetchMock.mock.calls.every(([url]) => String(url).startsWith('/v1/groups'))).toBe(
            true,
        );
    });

    test('publishes a restored entity to the store the generate buttons read', async () => {
        // A restore writes entityId directly rather than through _selectEntity,
        // which is the one path that does not republish, so the store kept
        // saying 0 while the pane showed a remembered entity by name.
        const stored: Record<string, unknown> = {};
        vi.stubGlobal('window', {
            Alpine: {
                store: (name: string, value?: unknown) => {
                    if (value !== undefined) stored[name] = value;
                    return stored[name];
                },
            },
        });
        vi.stubGlobal('localStorage', {
            getItem: () => JSON.stringify({ id: 42, label: 'Remembered' }),
            setItem: () => {},
        });
        const fetchMock = vi.fn(async () => ({
            ok: true,
            json: async () => ({ html: '', css: '', entity: null, issues: [], cssIssues: [] }),
        }));
        vi.stubGlobal('fetch', fetchMock);

        const component = templatePreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        component.$refs = { frame: frameStub() };
        component.$root = { closest: () => null } as unknown as HTMLElement;

        await component.init();

        expect(component.entityId).toBe(42);
        expect((stored.templatePreview as { entityId: number }).entityId).toBe(42);
    });
});
