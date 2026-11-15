// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { dismissGenerationNotice, noticeGenerationEnd } from './generationEndNotice.js';

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden });
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => (hidden ? 'hidden' : 'visible') });
}

function target(onScreen: boolean) {
  const el = document.createElement('section');
  el.tabIndex = -1;
  el.getBoundingClientRect = () => ({ top: onScreen ? 10 : 5000, bottom: onScreen ? 200 : 5200, left: 0, right: 100, width: 100, height: 190, x: 0, y: 0, toJSON() {} }) as DOMRect;
  el.scrollIntoView = vi.fn();
  document.body.appendChild(el);
  return el;
}

const notice = () => document.querySelector('[data-testid="generation-end-notice"]');

describe('the end-of-generation notice', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    document.title = 'New Category - mahresources';
    setHidden(false);
  });
  afterEach(() => {
    dismissGenerationNotice();
    vi.useRealTimers();
  });

  it('shows nothing extra when the result is already on screen', () => {
    const el = target(true);
    noticeGenerationEnd({ kind: 'ready', heading: 'Template draft ready', detail: 'Review it, then Apply.', locate: () => el, immediate: true });
    expect(notice()).toBeNull();
    expect(document.title).toBe('New Category - mahresources');
  });

  it('shows a notice whose Review button brings the result into view and focuses it', () => {
    const el = target(false);
    noticeGenerationEnd({ kind: 'ready', heading: 'Template draft ready', detail: 'Review it, then Apply.', locate: () => el, immediate: true });
    expect(notice()?.textContent).toContain('Template draft ready');
    (notice()!.querySelector('[data-testid="generation-end-notice-show"]') as HTMLButtonElement).click();
    expect(el.scrollIntoView).toHaveBeenCalled();
    expect(document.activeElement).toBe(el);
    expect(notice()).toBeNull();
  });

  it('opens a closed panel the result sits in before moving to it', () => {
    const details = document.createElement('details');
    document.body.appendChild(details);
    const el = target(false);
    details.appendChild(el);
    noticeGenerationEnd({ kind: 'failed', heading: 'Generation failed', detail: 'x', locate: () => el, immediate: true });
    (notice()!.querySelector('[data-testid="generation-end-notice-show"]') as HTMLButtonElement).click();
    expect(details.open).toBe(true);
  });

  it('marks the tab title while the page is in the background and clears it on return', () => {
    setHidden(true);
    const el = target(true);
    noticeGenerationEnd({ kind: 'ready', heading: 'Template draft ready', detail: '', locate: () => el, immediate: true });
    expect(document.title).toBe('(Draft ready) New Category - mahresources');
    expect(notice()).not.toBeNull();
    setHidden(false);
    document.dispatchEvent(new Event('visibilitychange'));
    expect(document.title).toBe('New Category - mahresources');
  });

  it('replaces an earlier title mark instead of stacking them', () => {
    setHidden(true);
    const el = target(true);
    noticeGenerationEnd({ kind: 'ready', heading: 'a', detail: '', locate: () => el, immediate: true });
    noticeGenerationEnd({ kind: 'failed', heading: 'b', detail: '', locate: () => el, immediate: true });
    expect(document.title).toBe('(Generation failed) New Category - mahresources');
    expect(document.querySelectorAll('[data-testid="generation-end-notice"]')).toHaveLength(1);
  });

  it('goes away by itself, by Dismiss, and when dismissed from code', () => {
    vi.useFakeTimers();
    const el = target(false);
    noticeGenerationEnd({ kind: 'review', heading: 'h', detail: '', locate: () => el, immediate: true });
    vi.advanceTimersByTime(20000);
    expect(notice()).toBeNull();
    noticeGenerationEnd({ kind: 'review', heading: 'h', detail: '', locate: () => el, immediate: true });
    (notice()!.querySelector('[data-testid="generation-end-notice-dismiss"]') as HTMLButtonElement).click();
    expect(notice()).toBeNull();
    noticeGenerationEnd({ kind: 'review', heading: 'h', detail: '', locate: () => el, immediate: true });
    dismissGenerationNotice();
    expect(notice()).toBeNull();
  });

  it('counts a result inside a closed panel as out of sight', () => {
    const details = document.createElement('details');
    document.body.appendChild(details);
    const el = target(true);
    details.appendChild(el);
    noticeGenerationEnd({ kind: 'ready', heading: 'h', detail: '', locate: () => el, immediate: true });
    expect(notice()).not.toBeNull();
  });

  it('drops a notice still waiting for its frames when dismissed first', () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => { frames.push(cb); return frames.length; });
    const el = target(false);
    noticeGenerationEnd({ kind: 'ready', heading: 'h', detail: '', locate: () => el });
    dismissGenerationNotice();
    while (frames.length) frames.shift()!(0);
    expect(notice()).toBeNull();
    vi.unstubAllGlobals();
  });

  it('stays while focus is in it even after the pointer leaves', () => {
    vi.useFakeTimers();
    const el = target(false);
    noticeGenerationEnd({ kind: 'ready', heading: 'h', detail: '', locate: () => el, immediate: true });
    const box = notice()!;
    box.dispatchEvent(new Event('mouseenter'));
    (box.querySelector('[data-testid="generation-end-notice-show"]') as HTMLButtonElement).focus();
    box.dispatchEvent(new Event('mouseleave'));
    vi.advanceTimersByTime(20000);
    expect(notice()).not.toBeNull();
  });

  it('returns focus to where it was when Dismiss removes the focused notice', () => {
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    const el = target(false);
    noticeGenerationEnd({ kind: 'ready', heading: 'h', detail: '', locate: () => el, immediate: true });
    const dismiss = notice()!.querySelector('[data-testid="generation-end-notice-dismiss"]') as HTMLButtonElement;
    dismiss.focus();
    dismiss.click();
    expect(document.activeElement).toBe(input);
  });

  it('closes on Escape pressed anywhere, but leaves Escape to an open dialog', () => {
    const el = target(false);
    noticeGenerationEnd({ kind: 'ready', heading: 'h', detail: '', locate: () => el, immediate: true });
    const dialog = document.createElement('div');
    dialog.setAttribute('aria-modal', 'true');
    dialog.getClientRects = () => [{}] as unknown as DOMRectList;
    document.body.appendChild(dialog);
    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    expect(notice()).not.toBeNull();
    dialog.remove();
    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    expect(notice()).toBeNull();
  });

  it('does not lead behind an open dialog', () => {
    const el = target(false);
    noticeGenerationEnd({ kind: 'ready', heading: 'h', detail: '', locate: () => el, immediate: true });
    const dialog = document.createElement('div');
    dialog.setAttribute('aria-modal', 'true');
    dialog.getClientRects = () => [{}] as unknown as DOMRectList;
    document.body.appendChild(dialog);
    (notice()!.querySelector('[data-testid="generation-end-notice-show"]') as HTMLButtonElement).click();
    expect(el.scrollIntoView).not.toHaveBeenCalled();
    expect(notice()).not.toBeNull();
  });

  it('shows no notice when the tab returns with the result on screen, but keeps the title mark until then', () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => { frames.push(cb); return frames.length; });
    setHidden(true);
    const el = target(true);
    noticeGenerationEnd({ kind: 'ready', heading: 'h', detail: '', locate: () => el });
    expect(document.title.startsWith('(Draft ready) ')).toBe(true);
    setHidden(false);
    while (frames.length) frames.shift()!(0);
    expect(notice()).toBeNull();
    vi.unstubAllGlobals();
  });
});
