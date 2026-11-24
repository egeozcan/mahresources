// @vitest-environment happy-dom

import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { registerReductionReviewStore } from './reductionReview.js';

// The pieces of the review store that read and write the Cluster card in place: the
// "Show more members" paging, and the oversized-approval gate that depends on it.

type Store = Record<string, any>;

function makeStore(): Store {
  let store: Store = {};
  registerReductionReviewStore({ store: (_name: string, value: Store) => { store = value; } });
  store.reductionId = 7;
  return store;
}

function cardHtml(opts: { rendered?: number; memberCount?: number; readOnly?: boolean; stale?: boolean } = {}): string {
  const rendered = opts.rendered ?? 1;
  const members = Array.from({ length: rendered }, (_, i) =>
    `<div class="reduction-member" tabindex="-1" data-testid="reduction-member" data-resource-id="${i + 1}"></div>`).join('');
  return `
    <article data-cluster-id="c1" tabindex="-1" data-member-count="${opts.memberCount ?? 5}" data-cluster-state="${opts.stale ? 'stale' : 'open'}">
      <h3 id="cluster-c1-heading">Near-Identical</h3>
      ${opts.readOnly ? '<div data-testid="cluster-stale-reason" data-cluster-read-only="true"></div>' : ''}
      <div class="reduction-cluster-members">${members}</div>
      <div data-testid="cluster-more-members-wrap">
        <p data-testid="cluster-shown-count">Showing ${rendered} of ${opts.memberCount ?? 5} copies.</p>
        <button type="button" data-testid="cluster-more-members" data-members-after="">Show more members</button>
      </div>
      <button type="button" id="elsewhere">elsewhere</button>
    </article>`;
}

function member(resourceId: number, resource: object | null = { ID: resourceId, Name: `r${resourceId}`, FileSize: 10, ContentType: 'image/png' }) {
  return { resourceId, Resource: resource, IsWinner: false, TagCount: 0, NoteCount: 0, GroupCount: 0 };
}

// A members endpoint that answers when the test says so.
function slowMembers(page: { members: unknown[]; after?: string }) {
  let release: () => void = () => {};
  const gate = new Promise<void>((resolve) => { release = resolve; });
  vi.stubGlobal('fetch', vi.fn(async () => {
    await gate;
    return { ok: true, status: 200, json: async () => page };
  }));
  return release;
}

beforeEach(() => {
  document.body.innerHTML = '';
  (window as any).mahAnnounce = vi.fn();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('Show more members focus', () => {
  test('leaves focus alone when the reviewer tabbed elsewhere during the request', async () => {
    document.body.innerHTML = cardHtml();
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const elsewhere = document.getElementById('elsewhere')!;
    trigger.focus();
    const release = slowMembers({ members: [member(2), member(3)], after: 'next' });

    const pending = store.loadMoreMembers('c1', trigger);
    elsewhere.focus();
    release();
    await pending;

    expect(document.activeElement).toBe(elsewhere);
  });

  test('moves focus to the first added copy when it is still on the trigger', async () => {
    document.body.innerHTML = cardHtml();
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    trigger.focus();
    const release = slowMembers({ members: [member(2), member(3)], after: 'next' });

    const pending = store.loadMoreMembers('c1', trigger);
    release();
    await pending;

    expect((document.activeElement as HTMLElement).dataset.resourceId).toBe('2');
  });

  test('lands on the first appended focusable thing when the trigger goes and the first copy is a placeholder', async () => {
    document.body.innerHTML = cardHtml();
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    trigger.focus();
    const release = slowMembers({ members: [member(2, null), member(3)] });

    const pending = store.loadMoreMembers('c1', trigger);
    release();
    await pending;

    expect(document.activeElement).not.toBe(trigger);
    expect(document.activeElement).not.toBe(document.body);
    expect(document.activeElement!.isConnected).toBe(true);
    expect(document.querySelector('article')!.contains(document.activeElement)).toBe(true);
    expect(document.querySelector('[data-testid="cluster-more-members"]')).toBeNull();
  });
});

describe('appended copies of a Cluster that takes no decisions', () => {
  test('a read-only Cluster gets no Keep this instead or Not-removing buttons', async () => {
    document.body.innerHTML = cardHtml({ readOnly: true });
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const release = slowMembers({ members: [member(2), member(3)] });

    const pending = store.loadMoreMembers('c1', trigger);
    release();
    await pending;

    expect(document.querySelectorAll('[data-testid="reduction-member"]').length).toBe(3);
    expect(document.querySelector('[data-testid="member-promote"]')).toBeNull();
    expect(document.querySelector('[data-testid="member-eject"]')).toBeNull();
  });

  test('an open Cluster still gets them', async () => {
    document.body.innerHTML = cardHtml();
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const release = slowMembers({ members: [member(2)] });

    const pending = store.loadMoreMembers('c1', trigger);
    release();
    await pending;

    expect(document.querySelector('[data-testid="member-promote"]')).not.toBeNull();
    expect(document.querySelector('[data-testid="member-eject"]')).not.toBeNull();
  });
});

describe('appended copies of a stale Cluster', () => {
  function staleCard(winnerChanged: boolean): string {
    return cardHtml({ stale: true }).replace('data-cluster-state="stale"', `data-cluster-state="stale" data-cluster-winner-changed="${winnerChanged}"`);
  }

  test('a copy that was deleted can be left out, as on the server-rendered card', async () => {
    document.body.innerHTML = staleCard(false);
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const release = slowMembers({ members: [member(2, null)] });

    const pending = store.loadMoreMembers('c1', trigger);
    release();
    await pending;

    const gone = document.querySelector('[data-testid="reduction-member-withheld"]')!;
    expect(gone.textContent).toContain('Removed elsewhere.');
    const eject = gone.querySelector<HTMLButtonElement>('[data-testid="member-eject"]')!;
    expect(eject).not.toBeNull();
    const act = vi.fn();
    store.act = act;
    eject.click();
    expect(act).toHaveBeenCalledWith('c1', 'eject', 2);
  });

  test('offers nothing when the kept copy is the one that changed', async () => {
    document.body.innerHTML = staleCard(true);
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const release = slowMembers({ members: [member(2, null), member(3)] });

    const pending = store.loadMoreMembers('c1', trigger);
    release();
    await pending;

    expect(document.querySelector('[data-testid="member-eject"]')).toBeNull();
  });

  test('a deleted winner or an already left out copy gets no button', async () => {
    document.body.innerHTML = staleCard(false);
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const release = slowMembers({ members: [{ ...member(2, null), ejected: true }, { ...member(3, null), IsWinner: true }] });

    const pending = store.loadMoreMembers('c1', trigger);
    release();
    await pending;

    expect(document.querySelector('[data-testid="member-eject"]')).toBeNull();
  });
});

describe('appended copies of a Near-Identical Cluster', () => {
  test('a loser carries its distance badge and a Compare link to the kept copy', async () => {
    document.body.innerHTML = cardHtml().replace('<div class="reduction-cluster-members">',
      '<div class="reduction-cluster-members"><div data-testid="reduction-member" data-winner="true" data-resource-id="1" tabindex="-1"></div>')
      .replace('<div class="reduction-member" tabindex="-1" data-testid="reduction-member" data-resource-id="1"></div>', '');
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const release = slowMembers({ members: [{ ...member(2), IsLoser: true, distance: 5 }, { ...member(3), IsLoser: true, distance: 1 }] });

    const pending = store.loadMoreMembers('c1', trigger);
    release();
    await pending;

    const badges = [...document.querySelectorAll('[data-testid="member-distance"]')];
    expect(badges.map((b) => b.textContent)).toEqual(['Close (perceptual distance 5)', 'Very close (perceptual distance 1)']);
    expect(badges[0].getAttribute('title')).toBe('perceptual distance 5');
    const compare = document.querySelector<HTMLAnchorElement>('[data-testid="member-compare"]')!;
    expect(compare).not.toBeNull();
    expect(compare.getAttribute('href')).toBe('/resource/compare?r1=1&r2=2&from=reduction:7');
    // The click records which Cluster the reviewer came from, as the server-rendered link does.
    compare.addEventListener('click', (event) => event.preventDefault());
    compare.click();
    expect(compare.getAttribute('href')).toContain('fa=c1');
  });

  test('an Identical copy has neither a distance nor a Compare link without a loser flag', async () => {
    document.body.innerHTML = cardHtml();
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const release = slowMembers({ members: [member(2)] });

    const pending = store.loadMoreMembers('c1', trigger);
    release();
    await pending;

    expect(document.querySelector('[data-testid="member-distance"]')).toBeNull();
    expect(document.querySelector('[data-testid="member-compare"]')).toBeNull();
  });
});

describe('approving a large Cluster whose copies are not all on the page', () => {
  test('stays unavailable until Show more has rendered every copy, and then names how many it showed', async () => {
    document.body.innerHTML = cardHtml({ rendered: 1, memberCount: 3 });
    const store = makeStore();
    store.expand('c1');

    // Expanded, but only the kept copy is on the page.
    expect(store.isReviewable('c1', true, true)).toBe(false);
    // A Cluster whose copies were all rendered, or one that is not large, is not held back.
    expect(store.isReviewable('c1', true, false)).toBe(true);
    expect(store.isReviewable('c1', false, true)).toBe(true);

    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const release = slowMembers({ members: [member(2), member(3)] });
    const pending = store.loadMoreMembers('c1', trigger);
    release();
    await pending;
    expect(store.isReviewable('c1', true, true)).toBe(true);

    const act = vi.fn();
    store.act = act;
    const button = document.createElement('button');
    button.setAttribute('aria-pressed', 'false');
    store.toggleApproval('c1', true, { currentTarget: button });
    expect(act).toHaveBeenCalledWith('c1', 'check', 0, { acknowledgeOversized: true, membersSeen: 3 });
  });

  test('a page refresh that morphs the appended copies away makes the Cluster unavailable again', () => {
    document.body.innerHTML = cardHtml({ rendered: 1, memberCount: 3 });
    const store = makeStore();
    store.expand('c1');
    store.allMembersShown = { c1: true };
    store.forgetShownMembers();
    expect(store.isReviewable('c1', true, true)).toBe(false);
  });
});
