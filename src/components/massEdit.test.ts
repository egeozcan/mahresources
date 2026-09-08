// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { massEditModal } from './massEdit.js';

describe('Mass Edit selection isolation', () => {
    it.each([{ids:[73]}, {ids:[]}])('uses the active type’s selection when switching from all results to IDs: $ids', ({ids}) => {
        const modal = Object.assign(massEditModal(), { $nextTick: () => {} });
        modal.open({entityType:'note', target:'ids', selection:{selectedIds:new Set([42])}});
        modal.close();
        modal.open({entityType:'resource', target:'filter', selection:{selectedIds:new Set(ids)}});
        modal.target = 'ids';

        const { payload } = modal.buildPayload(document.createElement('form'), {dryRun:true, expectedCount:null});
        expect(payload.getAll('ID')).toEqual(ids.map(String));
        expect(payload.getAll('ID')).not.toContain('42');
    });
});
