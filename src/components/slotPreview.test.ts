import { afterEach, describe, expect, test, vi } from 'vitest';
import { slotPreview } from './slotPreview.js';
import { templatePreviewContext } from './templatePreview.js';

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

    test('names a markup slot too, so its content is not judged as a stylesheet', async () => {
        const fetchMock = stubFetch();
        const component = slotPreview({
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
        component.entityReady = true;
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
        const component = slotPreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        component._form = formWith({ CustomCSS: '.a{}', CustomHeader: '<h1>x</h1>' });
        component.$refs = {};
        component.entityId = 42;
        component.entityReady = true;
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
        const component = slotPreview({
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

    test('a markup slot still renders its html in the body and its css in <style>', async () => {
        // The suppression is keyed on the selected slot, not on the two buffers
        // reading alike, so an ordinary slot keeps both halves.
        const fetchMock = stubFetch({ html: '<h1>rendered</h1>', css: '.badge{color:red}' });
        const component = slotPreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({ CustomCSS: '.badge{color:red}', CustomHeader: '<h1>x</h1>' });
        component.entityId = 42;
        component.entityReady = true;
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
        const component = slotPreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({ CustomCSS: 'body{background:#000}', CustomHeader: '' });
        component.entityId = 42;
        component.entityReady = true;
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

        const component = slotPreview({
            entityType: 'group',
            previewPath: '/v1/category/previewTemplate',
            categoryId: 7,
        });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({ CustomCSS: '', CustomHeader: '<h1>x</h1>' });
        component.entityId = 42;
        component.entityReady = true;
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

        const component = slotPreview({
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
        component.entityReady = true;
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

describe('slot preview startup', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    test('waits for the picker instead of claiming there is nothing to preview', async () => {
        const fetchMock = stubFetch();
        const component = slotPreview({ entityType: 'group', previewPath: '/v1/category/previewTemplate', categoryId: 7 });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({ CustomHeader: '<h1>x</h1>' });

        await component.refresh();

        expect(fetchMock).not.toHaveBeenCalled();
        expect(component.waiting).toBe(true);
        expect(component.error).toBe('');
        // Still stale, so the picker's publish renders it.
        expect(component.stale).toBe(true);
    });

    test('explains an empty deployment instead of leaving the frame a void', async () => {
        // The picker settled on nothing: a fresh deployment, or a category whose
        // members were deleted. Say so and blank the frame (WS6 finding 29).
        const fetchMock = stubFetch();
        const component = slotPreview({ entityType: 'group', previewPath: '/v1/category/previewTemplate', categoryId: 7 });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({ CustomHeader: '<h1>x</h1>' });
        component.entityReady = true;

        await component.refresh();

        expect(fetchMock).not.toHaveBeenCalled();
        expect(component.waiting).toBe(false);
        expect(component.error).toContain('Nothing to preview against yet');
        expect(frame.srcdoc).toContain('<body>');
    });

    test('an invalidation retires the render in flight', async () => {
        let release: () => void = () => {};
        const pending = new Promise<void>((resolve) => {
            release = resolve;
        });
        let calls = 0;
        vi.stubGlobal(
            'fetch',
            vi.fn(async () => {
                calls++;
                if (calls === 1) await pending;
                return {
                    ok: true,
                    json: async () => ({ html: calls === 1 ? '<h1>stale</h1>' : '<h1>fresh</h1>', css: '', entity: null, issues: [], cssIssues: [] }),
                };
            }),
        );
        const component = slotPreview({ entityType: 'group', previewPath: '/p', categoryId: 7 });
        const frame = frameStub();
        component.$refs = { frame };
        component._form = formWith({ CustomHeader: 'x' });
        component.entityId = 1;
        component.entityReady = true;
        component.visible = true;

        const first = component.refresh();
        component.invalidate(false);
        await new Promise((r) => setTimeout(r, 0));
        release();
        await first;
        expect(bodyOf(frame.srcdoc)).toContain('fresh');
        expect(bodyOf(frame.srcdoc)).not.toContain('stale');
    });

    test('off screen, an invalidation only marks the preview stale', () => {
        const fetchMock = stubFetch();
        const component = slotPreview({ entityType: 'group', previewPath: '/p', categoryId: 7 });
        component.stale = false;
        component.visible = false;
        component.invalidate(false);
        expect(component.stale).toBe(true);
        expect(fetchMock).not.toHaveBeenCalled();
    });
});

describe('preview context', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    function stubStore() {
        const stored: Record<string, Record<string, unknown>> = {};
        vi.stubGlobal('window', {
            Alpine: {
                store: (name: string, value?: Record<string, unknown>) => {
                    if (value !== undefined) stored[name] = value;
                    return stored[name];
                },
            },
        });
        return stored;
    }

    test('publishes a restored entity to the store the previews and generate buttons read', async () => {
        const stored = stubStore();
        vi.stubGlobal('localStorage', {
            getItem: () => JSON.stringify({ id: 42, label: 'Remembered' }),
            setItem: () => {},
        });
        const fetchMock = vi.fn();
        vi.stubGlobal('fetch', fetchMock);

        const component = templatePreviewContext({ entityType: 'group', categoryId: 7 });
        await component.init();

        expect(component.entityId).toBe(42);
        expect(stored.templatePreview).toMatchObject({ entityId: 42, ready: true, categoryId: 7 });
        expect(fetchMock).not.toHaveBeenCalled();
    });

    test('a default lookup answering after a pick does not replace the pick', async () => {
        const stored = stubStore();
        vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {} });
        let release: () => void = () => {};
        const pending = new Promise<void>((resolve) => {
            release = resolve;
        });
        vi.stubGlobal(
            'fetch',
            vi.fn(async () => {
                await pending;
                return { ok: true, json: async () => [{ ID: 1, Name: 'Default' }] };
            }),
        );

        const component = templatePreviewContext({ entityType: 'group', categoryId: 7 });
        const starting = component.init();
        component.pick({ ID: 9, Name: 'Chosen' });
        expect(stored.templatePreview).toMatchObject({ entityId: 9, ready: true });
        release();
        await starting;

        expect(component.entityId).toBe(9);
        expect(stored.templatePreview).toMatchObject({ entityId: 9 });
    });
});

// Equal-specificity companion rules must override shared rules exactly as on saved pages.
describe('companion CSS cascade', () => {
    afterEach(() => vi.unstubAllGlobals());
    test.each(['group', 'resource', 'note'])('picker preview uses only shared and picker styles for %s', async (entityType) => {
        const fetchMock = stubFetch();
        const component = slotPreview({ entityType, previewPath: '/preview', categoryId: 7 });
        component._form = formWith({
            CustomEntityPickerResult: '<p class="card">Picker</p>',
            CustomCSS: '.card{color:red}',
            CustomHeaderCSS: '.card{color:green}',
            CustomEntityPickerResultCSS: '.card{color:blue}',
        });
        component.$refs = {};
        component.entityId = 42;
        component.entityReady = true;
        component.slot = 'CustomEntityPickerResult';
        await component.refresh();
        expect(sentBody(fetchMock).content).toBe('<p class="card">Picker</p>');
        expect(sentBody(fetchMock).css).toBe('.card{color:red}\n.card{color:blue}');
    });
    test.each(['group', 'resource', 'note'])('shared CSS comes first for %s', async (entityType) => {
        const fetchMock = stubFetch();
        const component = slotPreview({ entityType, previewPath: '/preview', categoryId: 7 });
        component._form = formWith({
            CustomHeader: '<p class="card">Card</p>',
            CustomCSS: '.card{color:red}',
            CustomHeaderCSS: '.card{color:green}',
            CustomMRQLResultCSS: '.card{color:blue}',
        });
        component.$refs = {};
        component.entityId = 42;
        component.entityReady = true;
        component.slot = 'CustomHeader';
        await component.refresh();
        expect(sentBody(fetchMock).css).toBe('.card{color:red}\n.card{color:green}\n.card{color:blue}');
    });
});
