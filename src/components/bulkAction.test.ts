import { describe, expect, it } from 'vitest';
import { bulkAction, matchesActionFilters } from './bulkAction.js';
import { replaceMRQLSort } from '../utils/mrqlListQuery.js';

describe('shared action eligibility', () => {
    it('uses the serialized resource category field for eligibility', () => {
        expect(matchesActionFilters({resourceCategoryId:7}, {category_ids:[7]})).toBe(true);
        expect(matchesActionFilters({resourceCategoryId:8}, {category_ids:[7]})).toBe(false);
    });
    it('requires every selected entity to match plugin filters', () => {
        const action = Object.assign(bulkAction({Min:1, Filters:{content_types:['image/png']}}), {
            $selection: {selectedIds:new Set([1,2]),selectedEntities:()=>[{ContentType:'image/png'},{ContentType:'application/pdf'}]},
        });
        expect(action.unavailableReason()).toContain('Every selected item');
        expect(matchesActionFilters({ContentType:'image/png'}, {content_types:['image/png']})).toBe(true);
        expect(matchesActionFilters({ContentType:'image/png'}, {content_types:['image/*']})).toBe(false);
        expect(matchesActionFilters({ContentType:''}, {content_types:['']})).toBe(false);
    });
    it('keeps Compare unavailable for three selections', () => {
        const action=Object.assign(bulkAction({Min:2,Max:2,Label:'Compare'}),{$selection:{selectedIds:new Set([1,2,3])}});
        expect(action.unavailableReason()).toContain('exactly 2');
    });
});

describe('MRQL list sorting', () => {
    it('preserves quoted keywords, parameters, and explicit bounds', () => {
        expect(replaceMRQLSort('name = "ORDER BY x LIMIT 8" AND tags = $tag ORDER BY created DESC LIMIT 100 OFFSET 20','name ASC'))
            .toBe('name = "ORDER BY x LIMIT 8" AND tags = $tag ORDER BY name ASC LIMIT 100 OFFSET 20');
    });
});
