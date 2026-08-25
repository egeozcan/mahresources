import { describe, it, expect } from 'vitest';
import { compareView } from './compareView';

/** The component with navigation stubbed, since every path ends in one. */
function view(state: { r1: number; v1: number; r2: number; v2: number }) {
    const component: any = compareView(state);
    component.navigations = 0;
    component.updateUrl = function () { this.navigations += 1; };
    return component;
}

const pick = (id: number) => ({ added: [{ raw: { ID: id, Name: `R${id}` } }], removed: [] });

describe('compareView picker selection', () => {
    it('moves only the side that was picked', () => {
        const v = view({ r1: 1, v1: 2, r2: 5, v2: 4 });
        v.onSideSelected('right', pick(9));

        expect(v.r2).toBe(9);
        expect(v.v2).toBe(0);
        expect(v.r1).toBe(1);
        expect(v.v1).toBe(2);
        expect(v.navigations).toBe(1);
    });

    // The untouched side's resource has not moved, so its version is as valid as
    // it was. Clearing it threw away a choice the reader had already made and
    // answered with the page's previous-versus-current default instead.
    it('keeps the untouched version when both sides converge on one resource', () => {
        const v = view({ r1: 1, v1: 1, r2: 5, v2: 5 });
        v.onSideSelected('right', pick(1));

        expect(v.r1).toBe(1);
        expect(v.r2).toBe(1);
        expect(v.v1).toBe(1);
        expect(v.v2).toBe(0);
    });

    it('does nothing when the picked resource is already on that side', () => {
        const v = view({ r1: 1, v1: 2, r2: 5, v2: 4 });
        v.onSideSelected('left', pick(1));

        expect(v.v1).toBe(2);
        expect(v.navigations).toBe(0);
    });
});
