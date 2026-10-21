import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cardActionMenu, resetCardActionDefinitions } from './cardActionMenu.js';

function menuWithItems(count = 3) {
    const items = Array.from({ length: count }, () => ({ focus: vi.fn() }));
    const menu = { querySelectorAll: vi.fn(() => items) };
    const trigger = { focus: vi.fn() };
    const component = Object.assign(cardActionMenu(), {
        $refs: { menu, trigger },
        $nextTick: (callback: () => void) => callback(),
    });
    return { component, items, trigger };
}

describe('cardActionMenu keyboard navigation', () => {
    it('opens from the trigger and moves focus into the menu', () => {
        const { component, items } = menuWithItems();

        component.toggle();

        expect(component.open).toBe(true);
        expect(items[0].focus).toHaveBeenCalledOnce();
    });

    it('opens and focuses the requested edge of the menu', () => {
        const { component, items } = menuWithItems();

        component.openAndFocus('first');
        expect(component.open).toBe(true);
        expect(items[0].focus).toHaveBeenCalledOnce();

        component.openAndFocus('last');
        expect(items[2].focus).toHaveBeenCalledOnce();
    });

    it('wraps focus with the arrow keys', () => {
        const { component, items } = menuWithItems();
        const preventDefault = vi.fn();

        component.onMenuKeydown({ key: 'ArrowDown', target: items[2], preventDefault });
        expect(items[0].focus).toHaveBeenCalledOnce();

        component.onMenuKeydown({ key: 'ArrowUp', target: items[0], preventDefault });
        expect(items[2].focus).toHaveBeenCalledOnce();
        expect(preventDefault).toHaveBeenCalledTimes(2);
    });

    it('closes on Escape and restores focus to the trigger', () => {
        const { component, items, trigger } = menuWithItems();
        component.open = true;
        const preventDefault = vi.fn();

        component.onMenuKeydown({ key: 'Escape', target: items[0], preventDefault });

        expect(component.open).toBe(false);
        expect(trigger.focus).toHaveBeenCalledOnce();
        expect(preventDefault).toHaveBeenCalledOnce();
    });
});

// Every card used to inline each action's whole definition into its @click, which was
// most of a 16 MB /resources page and one compiled handler per card per action. The
// buttons now name the action and the menu looks its definition up once per page.
describe('cardActionMenu action lookup', () => {
    const colorize = {
        plugin_name: 'fal-ai', id: 'colorize', label: 'Colorize', description: 'd', entity: 'resource',
        async: true, params: [{ name: 'model' }], confirm: null, filters: { content_types: ['image/png'] }, bulk_max: 5,
    };

    function menuFor(entityType = 'resource') {
        const trigger = { focus: vi.fn(), dataset: { actionEntityType: entityType } };
        const component = Object.assign(cardActionMenu(), {
            $refs: { menu: { querySelectorAll: () => [] }, trigger },
            $nextTick: (callback: () => void) => callback(),
            $selection: undefined,
        });
        return { component, trigger };
    }

    function button(id = 'colorize', entityId = '42') {
        return { dataset: { actionPlugin: 'fal-ai', actionId: id, actionEntityId: entityId } };
    }

    beforeEach(() => {
        resetCardActionDefinitions();
        vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify([colorize]))));
        vi.stubGlobal('window', { dispatchEvent: vi.fn() });
    });

    afterEach(() => {
        vi.unstubAllGlobals();
    });

    it('dispatches the looked-up definition for the clicked card', async () => {
        const { component, trigger } = menuFor();
        await component.runAction(button());

        expect(fetch).toHaveBeenCalledWith('/v1/plugin/actions?entity=resource');
        const event = vi.mocked(window.dispatchEvent).mock.calls[0][0] as CustomEvent;
        expect(event.type).toBe('plugin-action-open');
        expect(event.detail).toMatchObject({
            returnFocusTo: trigger, plugin: 'fal-ai', action: 'colorize', label: 'Colorize', description: 'd',
            entityIds: [42], entityType: 'resource', async: true, params: colorize.params,
            filters: colorize.filters, bulk_max: 5,
        });
    });

    it('fetches the definitions once per entity type, starting when a menu opens', async () => {
        const first = menuFor();
        first.component.openAndFocus('first');
        expect(fetch).toHaveBeenCalledOnce();

        await first.component.runAction(button());
        await menuFor().component.runAction(button('colorize', '7'));
        expect(fetch).toHaveBeenCalledOnce();

        await menuFor('note').component.runAction(button());
        expect(fetch).toHaveBeenCalledTimes(2);
    });

    it('lets only the latest activation open a dialog when lookups answer out of order', async () => {
        let answerResource!: (r: Response) => void;
        vi.mocked(fetch).mockImplementation(async (url) => String(url).includes('entity=resource')
            ? new Promise<Response>(resolve => { answerResource = resolve; })
            : new Response(JSON.stringify([{ ...colorize, entity: 'note' }])));

        const older = menuFor('resource').component.runAction(button());
        await menuFor('note').component.runAction(button());
        answerResource(new Response(JSON.stringify([colorize])));
        await older;

        expect(window.dispatchEvent).toHaveBeenCalledOnce();
        expect((vi.mocked(window.dispatchEvent).mock.calls[0][0] as CustomEvent).detail.entityType).toBe('note');
    });

    it('returns focus to the menu button and announces it when the lookup fails', async () => {
        vi.mocked(fetch).mockResolvedValueOnce(new Response('nope', { status: 500 }));
        vi.spyOn(console, 'error').mockImplementation(() => {});
        const announce = vi.fn();
        const { component, trigger } = menuFor();

        await component.runAction(button(), { announce });

        expect(trigger.focus).toHaveBeenCalledOnce();
        expect(announce).toHaveBeenCalledWith(expect.stringContaining('could not be opened'));
    });

    it('retries a failed fetch on the next click instead of caching the failure', async () => {
        vi.mocked(fetch).mockResolvedValueOnce(new Response('nope', { status: 500 }));
        vi.spyOn(console, 'error').mockImplementation(() => {});
        const { component } = menuFor();

        await component.runAction(button(), { announce: vi.fn() });
        expect(window.dispatchEvent).not.toHaveBeenCalled();

        await component.runAction(button());
        expect(window.dispatchEvent).toHaveBeenCalledOnce();
    });
});
