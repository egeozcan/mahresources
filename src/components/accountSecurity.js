function responseMessage(data, fallback) {
  if (Array.isArray(data?.errors) && data.errors.length) {
    return data.errors.map((entry) => entry?.message).filter(Boolean).join(', ') || fallback;
  }
  return data?.error || data?.message || fallback;
}

async function readResponse(response) {
  const contentType = response.headers?.get?.('Content-Type') || '';
  if (contentType.includes('application/json')) {
    return response.json();
  }
  const text = await response.text();
  return text ? { message: text } : {};
}

function normalizeToken(token) {
  const lastUsedAt = token.lastUsedAt ?? token.LastUsedAt ?? null;
  return {
    id: token.id ?? token.ID,
    name: token.name ?? token.Name ?? '',
    prefix: token.prefix ?? token.Prefix ?? '',
    lastUsedAt,
    lastUsedAtDisplay: lastUsedAt
      ? new Date(lastUsedAt).toLocaleString(undefined, {
        year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
      })
      : 'never',
  };
}

function tokensFromPage() {
  if (typeof document === 'undefined') return [];
  const element = document.getElementById('account-token-data');
  if (!element?.textContent) return [];
  try {
    const parsed = JSON.parse(element.textContent);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

/**
 * Self-service credential state for /account.
 *
 * Raw API tokens live only in pendingRawToken and are discarded on explicit
 * dismissal or page navigation. A pending raw token intentionally blocks minting
 * another one so rapid clicks cannot create credentials the user never sees.
 */
export function accountSecurity(initialTokens = null) {
  return {
    tokenCreateBusy: false,
    tokenRefreshBusy: false,
    passwordBusy: false,
    revokingTokenId: null,
    get busy() {
      return this.tokenCreateBusy || this.tokenRefreshBusy || this.passwordBusy || this.revokingTokenId !== null;
    },
    error: '',
    pendingRawToken: '',
    tokens: (initialTokens ?? tokensFromPage()).map(normalizeToken),
    name: '',
    currentPassword: '',
    newPassword: '',
    confirmPassword: '',
    passwordError: '',
    passwordSuccess: '',
    _pageHideHandler: null,
    _tokenMutationGeneration: 0,
    _tokenRefreshGeneration: 0,

    init() {
      if (typeof window === 'undefined') return;
      this._pageHideHandler = () => this.dismissToken();
      window.addEventListener('pagehide', this._pageHideHandler, { once: true });
    },

    destroy() {
      this.dismissToken();
      if (this._pageHideHandler && typeof window !== 'undefined') {
        window.removeEventListener('pagehide', this._pageHideHandler);
      }
    },

    async createToken() {
      if (this.tokenCreateBusy || this.pendingRawToken) return;
      this.tokenCreateBusy = true;
      this.error = '';
      try {
        const response = await fetch('/v1/account/tokens', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({ name: this.name.trim() || 'mr cli' }),
        });
        const data = await readResponse(response);
        if (!response.ok) {
          this.error = responseMessage(data, `Could not create token (${response.status}).`);
          return;
        }
        if (!data.token) {
          this.error = 'The server did not return the one-time token.';
          return;
        }

        this._tokenMutationGeneration += 1;
        this.pendingRawToken = data.token;
        this.tokens = [normalizeToken(data), ...this.tokens.filter((token) => token.id !== data.id)];
        this.name = '';
      } catch (err) {
        this.error = err instanceof Error ? err.message : 'Could not create token.';
      } finally {
        this.tokenCreateBusy = false;
      }
    },

    dismissToken() {
      this.pendingRawToken = '';
    },

    async refreshTokens() {
      if (this.tokenRefreshBusy) return;
      const refreshGeneration = ++this._tokenRefreshGeneration;
      const mutationGeneration = this._tokenMutationGeneration;
      this.tokenRefreshBusy = true;
      this.error = '';
      try {
        const response = await fetch('/v1/account/tokens', { headers: { Accept: 'application/json' } });
        const data = await readResponse(response);
        const isCurrent = () => refreshGeneration === this._tokenRefreshGeneration
          && mutationGeneration === this._tokenMutationGeneration;
        if (!response.ok) {
          if (isCurrent()) {
            this.error = responseMessage(data, `Could not refresh tokens (${response.status}).`);
          }
          return;
        }
        if (isCurrent()) {
          this.tokens = Array.isArray(data) ? data.map(normalizeToken) : [];
        }
      } catch (err) {
        if (refreshGeneration === this._tokenRefreshGeneration && mutationGeneration === this._tokenMutationGeneration) {
          this.error = err instanceof Error ? err.message : 'Could not refresh tokens.';
        }
      } finally {
        this.tokenRefreshBusy = false;
      }
    },

    async revokeToken(token) {
      if (this.revokingTokenId !== null || !token?.id) return;
      this.revokingTokenId = token.id;
      this.error = '';
      try {
        const response = await fetch(`/v1/account/tokens/delete?id=${encodeURIComponent(token.id)}`, {
          method: 'POST',
          headers: { Accept: 'application/json' },
        });
        const data = await readResponse(response);
        if (!response.ok) {
          this.error = responseMessage(data, `Could not revoke token (${response.status}).`);
          return;
        }
        this._tokenMutationGeneration += 1;
        this.tokens = this.tokens.filter((row) => row.id !== token.id);
      } catch (err) {
        this.error = err instanceof Error ? err.message : 'Could not revoke token.';
      } finally {
        this.revokingTokenId = null;
      }
    },

    async changePassword() {
      if (this.passwordBusy) return;
      this.passwordError = '';
      this.passwordSuccess = '';
      if (this.newPassword !== this.confirmPassword) {
        this.passwordError = 'New password and confirmation do not match.';
        return;
      }

      this.passwordBusy = true;
      try {
        const response = await fetch('/v1/account/password', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({
            currentPassword: this.currentPassword,
            newPassword: this.newPassword,
          }),
        });
        const data = await readResponse(response);
        if (!response.ok) {
          this.passwordError = responseMessage(data, `Could not update password (${response.status}).`);
          return;
        }
        this.currentPassword = '';
        this.newPassword = '';
        this.confirmPassword = '';
        this.passwordSuccess = 'Password updated. Other browser sessions were signed out; this session remains active.';
      } catch (err) {
        this.passwordError = err instanceof Error ? err.message : 'Could not update password.';
      } finally {
        this.passwordBusy = false;
      }
    },
  };
}
