import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { selectorFormParameters } from './selectorFormParameters.js';

const controls: Record<string, Array<{ value: string }>> = {};

describe('selector form parameters', () => {
    beforeEach(() => {
        for (const key of Object.keys(controls)) delete controls[key];
        vi.stubGlobal('document', {
            querySelectorAll: (query: string) => {
                const name = query.replace('input[name=', '').replace(']', '');
                return controls[name] || [];
            },
        });
    });

    afterEach(() => {
        vi.unstubAllGlobals();
    });

    test('reads the live control values on every call', () => {
        controls.RelationSideFrom = [{ value: '0' }];
        const read = () => selectorFormParameters({ RelationSide: 'RelationSideFrom' });

        expect(read()).toEqual({ RelationSide: '0' });

        controls.RelationSideFrom[0].value = '1';
        expect(read()).toEqual({ RelationSide: '1' });
    });

    test('the last control of a name wins, so a selected field still sends its empty control', () => {
        controls.GroupRelationTypeId = [{ value: '7' }, { value: '' }];

        expect(selectorFormParameters({ RelationTypeId: 'GroupRelationTypeId' }))
            .toEqual({ RelationTypeId: '' });
    });

    test('a control name that matches nothing contributes no parameter', () => {
        controls.RelationSideFrom = [{ value: '0' }];

        expect(selectorFormParameters({
            RelationTypeId: 'GroupRelationTypeId',
            RelationSide: 'RelationSideFrom',
        })).toEqual({ RelationSide: '0' });
    });
});
