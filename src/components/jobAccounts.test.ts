import { describe, expect, test } from 'vitest';
import { jobAccountText } from './jobCenter.js';
import { panelOwnerText } from './jobPanel.js';

describe('Job owner and actor names', () => {
    test('the detail names an account, says when it was deleted, and falls back to its number', () => {
        expect(jobAccountText({ ownerUserId: 3, ownerName: 'Alice Liddell (alice)' }, 'owner')).toBe('Alice Liddell (alice)');
        expect(jobAccountText({ ownerUserId: null, ownerDeleted: true }, 'owner')).toBe('Deleted account');
        expect(jobAccountText({ actorUserId: 7 }, 'actor')).toBe('Account 7');
        expect(jobAccountText({ ownerUserId: null }, 'owner')).toBe('');
        expect(jobAccountText({}, 'actor')).toBe('');
    });

    test("an administrator's drawer names the owner of somebody else's job only", () => {
        const viewer = 1;
        expect(panelOwnerText({ ownerUserId: 3, ownerName: 'alice' }, viewer)).toBe('Owner: alice');
        expect(panelOwnerText({ ownerUserId: 1, ownerName: 'root' }, viewer)).toBe('');
        expect(panelOwnerText({ ownerUserId: null, ownerDeleted: true }, viewer)).toBe('Owner: deleted account');
        expect(panelOwnerText({ ownerUserId: null }, viewer)).toBe('');
        expect(panelOwnerText({ ownerUserId: 4 }, viewer)).toBe('Owner: account 4');
        // Anyone else sees only their own jobs, and is told nothing.
        expect(panelOwnerText({ ownerUserId: 3, ownerName: 'alice' }, 0)).toBe('');
    });
});
