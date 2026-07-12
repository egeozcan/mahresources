import { describe, expect, it, vi } from 'vitest';
import { cardActionMenu } from './cardActionMenu.js';

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
