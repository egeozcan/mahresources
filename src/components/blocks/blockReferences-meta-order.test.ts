/**
 * Group metadata for a references block: a slower, older response must not
 * overwrite a newer one or mark a group it never asked about as unavailable.
 */
import { describe, it, expect, vi } from 'vitest';

const calls: Array<{ ids: number[]; resolve: (m: Record<number, unknown>) => void }> = [];
vi.mock('../picker/index.js', () => ({
  fetchEntityMeta: (_type: string, ids: number[]) =>
    new Promise((resolve) => calls.push({ ids: [...ids], resolve })),
}));

const { blockReferences } = await import('./blockReferences.js');
const flush = () => new Promise((r) => setTimeout(r, 0));

describe('blockReferences metadata ordering', () => {
  it('an older metadata response does not disable a newly added group', async () => {
    const refs: any = blockReferences({ id: 1, content: { groupIds: [1] } }, () => {}, () => true);
    refs.fetchGroupMeta();
    refs.groupIds = [1, 2];
    refs.fetchGroupMeta();
    calls[1].resolve({ 1: { name: 'one' }, 2: { name: 'two' } });
    await flush();
    calls[0].resolve({ 1: { name: 'one' } });
    await flush();
    expect(refs.getGroupDisplay(2).unavailable).toBe(false);
    expect(refs.groupMeta[2]).toEqual({ name: 'two' });
  });
});
