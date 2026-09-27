import http from 'node:http';
import type { AddressInfo } from 'node:net';
import type { Page } from '@playwright/test';
import { test, expect, loginAs } from '../../fixtures/auth.fixture';

// Holds each request for a few seconds and then answers 500, so a download is
// running while a page is open on it and then fails with a reason.
async function startSlowFailingServer() {
  const server = http.createServer((request, response) => {
    const timer = setTimeout(() => {
      response.writeHead(500, { 'Content-Type': 'text/plain' });
      response.end('no');
    }, 4000);
    request.on('close', () => clearTimeout(timer));
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  return { server, base: `http://127.0.0.1:${(server.address() as AddressInfo).port}` };
}

async function csrfOf(page: Page): Promise<string> {
  const me = await page.request.get('/v1/auth/me');
  expect(me.ok()).toBe(true);
  return (await me.json()).csrfToken as string;
}

async function submit(page: Page, url: string, groupId: number): Promise<string> {
  const response = await page.request.post('/v1/download/submit', {
    headers: { 'X-CSRF-Token': await csrfOf(page) },
    data: { URL: url, OwnerId: groupId },
  });
  expect(response.status(), await response.text()).toBe(202);
  return (await response.json()).jobs[0].canonicalJobId as string;
}

// Every text any live region takes, with the region it was said in: the
// drawer's, the Job page's own, or another.
async function recordAnnouncements(page: Page) {
  await page.addInitScript(() => {
    const said: { text: string; region: Element }[] = [];
    (window as any).__announced = said;
    const last = new WeakMap<Element, string>();
    const isRegion = (element: Element) => element.hasAttribute('aria-live') || ['status', 'alert'].includes(element.getAttribute('role') || '');
    const regionOf = (node: Node | null) => {
      let element = node && node.nodeType === 1 ? node as Element : node?.parentElement || null;
      while (element && !isRegion(element)) element = element.parentElement;
      return element;
    };
    const check = (region: Element) => {
      const text = (region.textContent || '').replace(/\s+/g, ' ').trim();
      if (text === last.get(region)) return;
      last.set(region, text);
      if (text) said.push({ text, region });
    };
    const start = () => new MutationObserver(mutations => {
      const regions = new Set<Element>();
      for (const mutation of mutations) {
        const region = regionOf(mutation.target);
        if (region) regions.add(region);
        mutation.addedNodes.forEach(node => {
          if (node.nodeType === 1 && isRegion(node as Element)) regions.add(node as Element);
        });
      }
      regions.forEach(check);
    }).observe(document.documentElement, { subtree: true, childList: true, characterData: true });
    if (document.documentElement) start();
    else document.addEventListener('DOMContentLoaded', start);
  });
}

async function announcementsOf(page: Page, text: string) {
  return page.evaluate((wanted) => {
    const alpine = (window as any).Alpine;
    const drawer = alpine.$data(document.querySelector('[data-testid="job-panel-root"]'))?._liveRegion?.element;
    const center = alpine.$data(document.querySelector('[data-testid="job-detail"]'))?._liveRegion?.element;
    return ((window as any).__announced as { text: string; region: Element }[])
      .filter(entry => entry.text.includes(wanted))
      .map(entry => entry.region === drawer ? 'drawer' : entry.region === center ? 'page' : 'other');
  }, text);
}

test.describe('one announcement per Job state change', () => {
  test('an administrator on My jobs hears another account\'s Job once, from its page', async ({ browser, baseURL, authSeed }) => {
    const { server, base } = await startSlowFailingServer();
    const userContext = await browser.newContext({ baseURL });
    const adminContext = await browser.newContext({ baseURL });
    try {
      const user = await userContext.newPage();
      await loginAs(user, authSeed.user);
      const file = `theirs-${Date.now()}.bin`;
      const jobId = await submit(user, `${base}/${file}`, authSeed.scopeGroupId);

      const admin = await adminContext.newPage();
      await recordAnnouncements(admin);
      await loginAs(admin, authSeed.admin);
      await admin.goto(`/job?id=${encodeURIComponent(jobId)}`);
      await expect(admin.getByTestId('job-detail')).toBeVisible();

      await expect.poll(() => announcementsOf(admin, `${file} failed`), { timeout: 20_000 }).toEqual(['page']);
      await expect.poll(() => announcementsOf(admin, `${file} failed: HTTP 500`)).toEqual(['page']);
      await admin.waitForTimeout(3000);
      expect(await announcementsOf(admin, `${file} failed`)).toEqual(['page']);
    } finally {
      await userContext.close();
      await adminContext.close();
      server.close();
    }
  });

  test('a Job the drawer follows is heard once, from the drawer, and not again from its page', async ({ browser, baseURL, authSeed }) => {
    const { server, base } = await startSlowFailingServer();
    const adminContext = await browser.newContext({ baseURL });
    try {
      const admin = await adminContext.newPage();
      await recordAnnouncements(admin);
      await loginAs(admin, authSeed.admin);
      const file = `mine-${Date.now()}.bin`;
      const jobId = await submit(admin, `${base}/${file}`, authSeed.outsideGroupId);
      await admin.goto(`/job?id=${encodeURIComponent(jobId)}`);
      await expect(admin.getByTestId('job-detail')).toBeVisible();

      await expect.poll(() => announcementsOf(admin, `${file} failed`), { timeout: 20_000 }).toEqual(['drawer']);
      await expect.poll(() => announcementsOf(admin, `${file} failed: HTTP 500`)).toEqual(['drawer']);
      await admin.waitForTimeout(3000);
      expect(await announcementsOf(admin, `${file} failed`)).toEqual(['drawer']);
    } finally {
      await adminContext.close();
      server.close();
    }
  });
});
