import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { accountSecurity } from './accountSecurity.js';

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

beforeEach(() => {
  vi.stubGlobal('document', { getElementById: () => null });
});

afterEach(() => vi.unstubAllGlobals());

describe('accountSecurity token creation state', () => {
  test('rapid repeated create calls issue only one request', async () => {
    let resolve!: (response: Response) => void;
    const pending = new Promise<Response>((done) => { resolve = done; });
    const fetchMock = vi.fn().mockReturnValue(pending);
    vi.stubGlobal('fetch', fetchMock);
    const state = accountSecurity([]);
    state.name = 'laptop';

    const first = state.createToken();
    const second = state.createToken();

    expect(state.busy).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    resolve(jsonResponse({ token: 'mr_once', id: 9, name: 'laptop', prefix: 'mr_on' }));
    await Promise.all([first, second]);
  });

  test('a non-2xx response exposes an alert message and recovers', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      jsonResponse({ errors: [{ message: 'token limit reached' }] }, 409),
    ));
    const state = accountSecurity([]);

    await state.createToken();

    expect(state.error).toBe('token limit reached');
    expect(state.busy).toBe(false);
    expect(state.pendingRawToken).toBe('');
  });

  test('a pending one-time token blocks creation until dismissed', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ token: 'mr_first', id: 1, name: 'first', prefix: 'mr_fi' }))
      .mockResolvedValueOnce(jsonResponse({ token: 'mr_second', id: 2, name: 'second', prefix: 'mr_se' }));
    vi.stubGlobal('fetch', fetchMock);
    const state = accountSecurity([]);

    await state.createToken();
    await state.createToken();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(state.pendingRawToken).toBe('mr_first');

    state.dismissToken();
    await state.createToken();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(state.pendingRawToken).toBe('mr_second');
  });

  test('successful metadata is immediately visible in the token list', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      jsonResponse({ token: 'mr_raw', id: 42, name: 'automation', prefix: 'mr_aut' }),
    ));
    const state = accountSecurity([]);

    await state.createToken();

    expect(state.tokens).toEqual([
      expect.objectContaining({ id: 42, name: 'automation', prefix: 'mr_aut' }),
    ]);
    expect(state.pendingRawToken).toBe('mr_raw');
    expect(state.name).toBe('');
  });
});

describe('accountSecurity refresh and revoke state', () => {
  test('refresh busy state does not block or relabel unrelated operations', async () => {
    let resolveRefresh!: (response: Response) => void;
    const refreshPending = new Promise<Response>((done) => { resolveRefresh = done; });
    const fetchMock = vi.fn((url: string) => {
      if (url === '/v1/account/tokens') {
        const request = fetchMock.mock.calls.length === 1;
        if (request) return refreshPending;
      }
      return Promise.resolve(jsonResponse({ token: 'mr_new', id: 8, name: 'new', prefix: 'mr_ne' }));
    });
    vi.stubGlobal('fetch', fetchMock);
    const state = accountSecurity([]);

    const refresh = state.refreshTokens();
    expect(state.tokenRefreshBusy).toBe(true);
    expect(state.tokenCreateBusy).toBe(false);
    expect(state.passwordBusy).toBe(false);

    await state.createToken();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(state.pendingRawToken).toBe('mr_new');
    expect(state.tokenRefreshBusy).toBe(true);

    resolveRefresh(jsonResponse([]));
    await refresh;
    expect(state.tokenRefreshBusy).toBe(false);
  });

  test('revoke removes only the successful token row', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({ ok: true })));
    const state = accountSecurity([
      { ID: 3, name: 'desktop', prefix: 'mr_des' },
      { ID: 4, name: 'automation', prefix: 'mr_aut' },
    ]);

    await state.revokeToken(state.tokens[1]);

    expect(state.tokens).toEqual([
      expect.objectContaining({ id: 3, name: 'desktop' }),
    ]);
    expect(state.error).toBe('');
    expect(state.busy).toBe(false);
  });

  test('refresh replaces token metadata without retaining raw tokens', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      jsonResponse([{ ID: 3, name: 'desktop', prefix: 'mr_des' }]),
    ));
    const state = accountSecurity([]);

    await state.refreshTokens();

    expect(state.tokens).toEqual([
      expect.objectContaining({ id: 3, name: 'desktop', prefix: 'mr_des' }),
    ]);
    expect(state.pendingRawToken).toBe('');
  });
});
