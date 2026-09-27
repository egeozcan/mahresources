// @vitest-environment happy-dom

import { afterEach, describe, expect, test, vi } from 'vitest';
import { jobPanel } from './jobPanel.js';

afterEach(() => {
    document.body.innerHTML = '';
    vi.unstubAllGlobals();
});

function row(id: string, title: string, controls = '') {
    return `<article data-job-id="${id}"><a id="job-panel-title-${id}" href="/job?id=${id}">${title}</a>${controls}</article>`;
}

function drawer() {
    // A frame as a browser paints one, rather than happy-dom's next tick.
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => setTimeout(() => callback(performance.now()), 16));
    document.body.innerHTML = `<div id="job-center-panel">
        <section data-job-panel-group="attention">${row('a', 'Commanded row', '<button data-command-key="forget">Forget</button>')}</section>
        <section data-job-panel-group="active">${row('b', 'Moving row')}</section>
        <section data-job-panel-group="finished"></section>
        <button data-job-panel-close>Close Jobs panel</button>
    </div>`;
    const element = document.querySelector('#job-center-panel') as HTMLElement;
    const panel = jobPanel() as any;
    panel.isOpen = true;
    panel.jobs = [{ id: 'a', state: 'failed' }, { id: 'b', state: 'running' }];
    // happy-dom lays nothing out, so every connected element counts as painted.
    for (const el of element.querySelectorAll('*')) (el as any).checkVisibility = () => true;
    panel.startFocusKeeper(element);
    return { element, panel };
}

const settle = () => new Promise(resolve => setTimeout(resolve, 40));

describe('Job Center drawer focus keeper', () => {
    test('follows the reader to another row when only the blur of the element they left is heard', async () => {
        const { element, panel } = drawer();
        const forget = element.querySelector('button[data-command-key="forget"]') as HTMLElement;
        forget.focus();
        // Row A's command is still running, so a restore for A waits for it.
        panel._commandFocusPending = { a: 1 };

        // The reader moves to B's title; the browser reports the blur of Forget
        // (whose relatedTarget names B's title) and no focusin for the title.
        const titleB = element.querySelector('#job-panel-title-b') as HTMLElement;
        forget.dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: titleB }));
        expect(panel._focusMemo?.jobId).toBe('b');

        // B finishes and is drawn anew in Finished; its old element goes.
        element.querySelector('[data-job-panel-group="active"]')!.innerHTML = '';
        const finished = element.querySelector('[data-job-panel-group="finished"]') as HTMLElement;
        finished.innerHTML = row('b', 'Moving row');
        for (const el of finished.querySelectorAll('*')) (el as any).checkVisibility = () => true;
        await settle();

        expect(document.activeElement?.id).toBe('job-panel-title-b');
        expect(document.activeElement?.closest('[data-job-panel-group]')?.getAttribute('data-job-panel-group')).toBe('finished');
        panel.stopFocusKeeper();
    });

    test('a restore that finds nothing to focus yet asks again once the row is drawn', async () => {
        const { element, panel } = drawer();
        const titleB = element.querySelector('#job-panel-title-b') as HTMLElement;
        titleB.focus();
        expect(panel._focusMemo?.jobId).toBe('b');

        // The row leaves; nothing else is drawn, and the drawer no longer
        // lists it, so there is no neighbour yet either.
        panel.jobs = [{ id: 'a', state: 'failed' }];
        element.querySelector('[data-job-panel-group="active"]')!.innerHTML = '';
        element.querySelector('[data-job-panel-group="attention"]')!.innerHTML = '';
        await settle();

        // A later render, with no further mutation after it, brings a row in.
        const finished = element.querySelector('[data-job-panel-group="finished"]') as HTMLElement;
        panel._focusObserver.disconnect();
        finished.innerHTML = row('c', 'Another row');
        for (const el of finished.querySelectorAll('*')) (el as any).checkVisibility = () => true;
        panel._focusObserver.observe(element, { childList: true, subtree: true });
        await settle();
        await settle();

        expect(document.activeElement?.id).toBe('job-panel-title-c');
        panel.stopFocusKeeper();
    });
});
