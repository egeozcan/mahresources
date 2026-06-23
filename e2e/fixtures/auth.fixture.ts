/**
 * Auth-enabled E2E fixture.
 *
 * Starts a dedicated, auth-enabled ephemeral server per worker (bootstrapped
 * admin), then seeds the other three roles plus an in-scope and an
 * out-of-scope group so per-role specs can exercise access boundaries and
 * group-subtree confinement. Used by the `auth` Playwright project only; the
 * default suites continue to run against an auth-off server.
 */
import { test as base, expect, request as pwRequest, APIRequestContext, Page } from '@playwright/test';
import {
  ServerInfo,
  startServer,
  stopServer,
  AUTH_ADMIN_USERNAME,
  AUTH_ADMIN_PASSWORD,
} from './server-manager';

export interface RoleCreds {
  username: string;
  password: string;
  role: 'admin' | 'editor' | 'user' | 'guest';
}

export interface AuthSeed {
  admin: RoleCreds;
  editor: RoleCreds;
  user: RoleCreds; // confined to scopeGroupId
  guest: RoleCreds; // confined to scopeGroupId
  scopeGroupId: number;
  scopeGroupName: string;
  outsideGroupId: number;
  outsideGroupName: string;
}

// Per-worker process cache (each worker is its own process).
let _seed: AuthSeed | null = null;

async function adminLogin(baseURL: string): Promise<{ ctx: APIRequestContext; csrf: string }> {
  const ctx = await pwRequest.newContext({ baseURL });
  const login = await ctx.post('/v1/auth/login', {
    data: { username: AUTH_ADMIN_USERNAME, password: AUTH_ADMIN_PASSWORD },
  });
  if (!login.ok()) {
    throw new Error(`auth seed: admin login failed (${login.status()})`);
  }
  const me = await (await ctx.get('/v1/auth/me')).json();
  const csrf = me.csrfToken as string;
  if (!csrf) throw new Error('auth seed: /v1/auth/me returned no csrfToken');
  return { ctx, csrf };
}

async function seedRoles(baseURL: string): Promise<AuthSeed> {
  const { ctx, csrf } = await adminLogin(baseURL);
  const stamp = `${Date.now()}_${Math.floor(Math.random() * 1e6)}`;
  const header = { 'X-CSRF-Token': csrf };

  // Category (groups require one).
  const catRes = await ctx.post('/v1/category', { headers: header, form: { name: `authcat_${stamp}` } });
  if (!catRes.ok()) throw new Error(`auth seed: category create failed (${catRes.status()})`);
  const categoryId = (await catRes.json()).ID as number;

  const mkGroup = async (name: string): Promise<{ id: number; name: string }> => {
    const res = await ctx.post('/v1/group', { headers: header, form: { name, categoryId: String(categoryId) } });
    if (!res.ok()) throw new Error(`auth seed: group ${name} create failed (${res.status()})`);
    const g = await res.json();
    return { id: g.ID as number, name: g.Name as string };
  };
  const scope = await mkGroup(`scope_${stamp}`);
  const outside = await mkGroup(`outside_${stamp}`);

  const mkUser = async (creds: RoleCreds, scopeGroupId?: number) => {
    const data: Record<string, unknown> = { ...creds };
    if (scopeGroupId !== undefined) data.scopeGroupId = scopeGroupId;
    const res = await ctx.post('/v1/users', { headers: header, data });
    if (!res.ok()) throw new Error(`auth seed: user ${creds.username} create failed (${res.status()}): ${await res.text()}`);
  };

  const editor: RoleCreds = { username: `editor_${stamp}`, password: 'pw', role: 'editor' };
  const user: RoleCreds = { username: `user_${stamp}`, password: 'pw', role: 'user' };
  const guest: RoleCreds = { username: `guest_${stamp}`, password: 'pw', role: 'guest' };
  await mkUser(editor);
  await mkUser(user, scope.id);
  await mkUser(guest, scope.id);

  await ctx.dispose();

  return {
    admin: { username: AUTH_ADMIN_USERNAME, password: AUTH_ADMIN_PASSWORD, role: 'admin' },
    editor,
    user,
    guest,
    scopeGroupId: scope.id,
    scopeGroupName: scope.name,
    outsideGroupId: outside.id,
    outsideGroupName: outside.name,
  };
}

type AuthWorkerFixtures = {
  authServer: ServerInfo;
  authSeed: AuthSeed;
};

export const test = base.extend<{}, AuthWorkerFixtures>({
  authServer: [
    async ({}, use) => {
      const server = await startServer(3, { auth: true });
      await use(server);
      await stopServer(server.proc);
    },
    { scope: 'worker', auto: true },
  ],

  authSeed: [
    async ({ authServer }, use) => {
      if (!_seed) {
        _seed = await seedRoles(`http://127.0.0.1:${authServer.port}`);
      }
      await use(_seed);
    },
    { scope: 'worker' },
  ],

  baseURL: async ({ authServer }, use) => {
    await use(`http://127.0.0.1:${authServer.port}`);
  },
});

export { expect };

/**
 * Log in through the browser login form and wait for the post-login navigation.
 * Returns once the session cookie is set and the destination page has loaded.
 */
export async function loginAs(page: Page, creds: RoleCreds): Promise<void> {
  await page.goto('/login');
  await page.fill('input[name="username"]', creds.username);
  await page.fill('input[name="password"]', creds.password);
  await Promise.all([
    page.waitForURL((url) => !url.pathname.startsWith('/login'), { timeout: 15000 }),
    page.click('button[type="submit"]'),
  ]);
}
