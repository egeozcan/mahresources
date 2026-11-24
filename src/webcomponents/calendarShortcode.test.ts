// @vitest-environment happy-dom

// Month navigation of the [calendar] shortcode: what a click sends, what
// replaces the host, where focus lands and what the reader is told, for each
// way the round trip can end. reloadInto itself is covered in
// deferredReload.test.ts; here it is a fake whose outcome each test picks.
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

const reloadInto = vi.fn();
vi.mock('./deferredShortcodes.js', () => ({ reloadInto: (...args: any[]) => reloadInto(...args) }));

let announce: ReturnType<typeof vi.fn>;

beforeAll(async () => {
  await import('./calendarShortcode.js');
});

beforeEach(() => {
  announce = vi.fn();
  (window as any).mahAnnounce = announce;
  reloadInto.mockReset();
});

afterEach(() => {
  document.body.innerHTML = '';
});

function calendar(month: string, { prev = true, next = true } = {}) {
  return (
    `<calendar-shortcode role="group" aria-labelledby="c-${month}"` +
    (prev ? ` data-prev-token="prev-of-${month}"` : '') +
    (next ? ` data-next-token="next-of-${month}"` : '') +
    `><div class="calendar-shortcode-nav">` +
    (prev ? `<button type="button" data-calendar-nav="prev">Previous</button>` : '') +
    (next ? `<button type="button" data-calendar-nav="next">Next</button>` : '') +
    `<button type="button" class="foreign">Other</button>` +
    `</div><table><caption id="c-${month}">${month} <span>(no items)</span></caption></table></calendar-shortcode>`
  );
}

function mount(month = 'March 2026') {
  document.body.innerHTML = calendar(month);
  return document.querySelector('calendar-shortcode') as HTMLElement;
}

// reloadInto as it behaves when a swap lands: beforeSwap, then the new markup.
function landing(html: string) {
  reloadInto.mockImplementation(async (host: HTMLElement, content: HTMLElement, _token: string, _stale: any, beforeSwap: () => void) => {
    beforeSwap();
    content.innerHTML = html;
    return true;
  });
}

const settle = () => new Promise((r) => setTimeout(r, 0));

describe('calendar-shortcode', () => {
  it('sends the token of the direction clicked', async () => {
    const host = mount();
    landing(calendar('April 2026'));
    host.querySelector<HTMLButtonElement>('[data-calendar-nav="next"]')!.click();
    await settle();
    expect(reloadInto).toHaveBeenCalledTimes(1);
    expect(reloadInto.mock.calls[0][0]).toBe(host);
    expect(reloadInto.mock.calls[0][1]).toBe(host);
    expect(reloadInto.mock.calls[0][2]).toBe('next-of-March 2026');
  });

  it('replaces itself with the new month, keeps focus on the same button and announces the caption', async () => {
    const host = mount();
    landing(calendar('April 2026'));
    host.querySelector<HTMLButtonElement>('[data-calendar-nav="next"]')!.click();
    await settle();

    const all = document.querySelectorAll('calendar-shortcode');
    expect(all).toHaveLength(1);
    const fresh = all[0] as HTMLElement;
    expect(fresh).not.toBe(host);
    expect(fresh.dataset.nextToken).toBe('next-of-April 2026');
    expect(document.activeElement).toBe(fresh.querySelector('[data-calendar-nav="next"]'));
    expect(announce).toHaveBeenCalledWith('April 2026 (no items)', { assertive: false });

    // The new host listens exactly once.
    landing(calendar('May 2026'));
    fresh.querySelector<HTMLButtonElement>('[data-calendar-nav="next"]')!.click();
    await settle();
    expect(reloadInto).toHaveBeenCalledTimes(2);
    expect(reloadInto.mock.calls[1][2]).toBe('next-of-April 2026');
  });

  it('parks focus on the new month when its same-direction button is gone at a bound', async () => {
    const host = mount();
    landing(calendar('9999-12', { next: false }));
    host.querySelector<HTMLButtonElement>('[data-calendar-nav="next"]')!.click();
    await settle();
    const fresh = document.querySelector('calendar-shortcode') as HTMLElement;
    expect(fresh.contains(document.activeElement)).toBe(true);
  });

  it('puts the old month back when the response holds no calendar', async () => {
    const host = mount();
    landing('<span class="shortcode-error">⚠ calendar</span>');
    const next = host.querySelector<HTMLButtonElement>('[data-calendar-nav="next"]')!;
    next.click();
    await settle();
    expect(document.querySelector('calendar-shortcode')).toBe(host);
    expect(host.querySelector('caption')!.textContent).toContain('March 2026');
    expect(host.querySelector('.shortcode-error')).toBeNull();
    expect(document.activeElement).toBe(next);
    expect(announce).toHaveBeenCalledWith('Could not load the month', { assertive: true });

    landing(calendar('April 2026'));
    next.click();
    await settle();
    expect(reloadInto).toHaveBeenCalledTimes(2);
  });

  it('keeps the month and recovers when the request fails', async () => {
    const host = mount();
    reloadInto.mockRejectedValue(new Error('500'));
    const prev = host.querySelector<HTMLButtonElement>('[data-calendar-nav="prev"]')!;
    prev.click();
    await settle();
    expect(document.querySelector('calendar-shortcode')).toBe(host);
    expect(host.querySelector('caption')!.textContent).toContain('March 2026');
    expect(announce).toHaveBeenCalledWith('Could not load the month', { assertive: true });
    prev.click();
    await settle();
    expect(reloadInto).toHaveBeenCalledTimes(2);
  });

  it('says nothing when the swap was abandoned, and is not left busy', async () => {
    const host = mount();
    reloadInto.mockResolvedValue(false);
    const next = host.querySelector<HTMLButtonElement>('[data-calendar-nav="next"]')!;
    next.click();
    await settle();
    expect(announce).not.toHaveBeenCalled();
    next.click();
    await settle();
    expect(reloadInto).toHaveBeenCalledTimes(2);
  });

  it('sends one request for a double click', async () => {
    const host = mount();
    let release!: (v: boolean) => void;
    reloadInto.mockImplementation(() => new Promise((r) => (release = r)));
    const next = host.querySelector<HTMLButtonElement>('[data-calendar-nav="next"]')!;
    next.click();
    next.click();
    expect(reloadInto).toHaveBeenCalledTimes(1);
    release(false);
    await settle();
  });

  it('ignores other buttons inside the host', async () => {
    const host = mount();
    host.querySelector<HTMLButtonElement>('.foreign')!.click();
    await settle();
    expect(reloadInto).not.toHaveBeenCalled();
  });

  it('does nothing for a calendar connected inside another one', async () => {
    const host = mount();
    host.insertAdjacentHTML('beforeend', calendar('April 2026'));
    const inner = host.querySelector(':scope > calendar-shortcode') as any;
    expect(inner._init).toBeUndefined();
    inner.querySelector('[data-calendar-nav="next"]').click();
    await settle();
    expect(reloadInto).not.toHaveBeenCalled();
  });
});
