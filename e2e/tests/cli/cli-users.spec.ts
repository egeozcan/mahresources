import { test, expect } from '../../fixtures/cli.fixture';

// The CLI E2E server runs without auth, so every request is an implicit admin
// and the admin user-management endpoints are reachable. We use unique
// usernames so parallel/repeated runs against the same worker DB never collide.
function uniqueName(prefix: string): string {
  return `${prefix}_${Date.now()}_${Math.floor(Math.random() * 1e6)}`;
}

test.describe('mr user — lifecycle', () => {
  test('create, get, update, and delete a user', async ({ cli }) => {
    const username = uniqueName('cliuser');

    // Create
    const created = cli.runJson<{ ID: number; username: string; role: string }>(
      'user', 'create', '--username', username, '--password', 's3cret', '--role', 'editor'
    );
    expect(created.username).toBe(username);
    expect(created.role).toBe('editor');
    const id = created.ID;
    expect(id).toBeGreaterThan(0);

    // List shows it
    const listed = cli.runOrFail('user', 'list');
    expect(listed.stdout).toContain(username);

    // Get by id
    const got = cli.runJson<{ ID: number; username: string }>('user', 'get', String(id));
    expect(got.ID).toBe(id);
    expect(got.username).toBe(username);

    // Partial update: change role only; username is preserved.
    const updated = cli.runJson<{ username: string; role: string }>(
      'user', 'update', String(id), '--role', 'user'
    );
    expect(updated.role).toBe('user');
    expect(updated.username).toBe(username);

    // Disable, then re-enable.
    const disabled = cli.runJson<{ disabled: boolean }>('user', 'update', String(id), '--disabled');
    expect(disabled.disabled).toBe(true);
    const enabled = cli.runJson<{ disabled: boolean }>('user', 'update', String(id), '--enable');
    expect(enabled.disabled).toBe(false);

    // Delete
    const deleted = cli.runOrFail('user', 'delete', String(id));
    expect(deleted.stdout.toLowerCase()).toContain('deleted');

    // Gone
    cli.runExpectError('user', 'get', String(id));
  });
});

test.describe('mr user — validation', () => {
  test('create requires a role', async ({ cli }) => {
    const err = cli.runExpectError('user', 'create', '--username', uniqueName('norole'), '--password', 'x');
    expect(err.stderr.toLowerCase()).toContain('role');
  });

  test('guest requires a scope group', async ({ cli }) => {
    // Server-side validation: a guest must have a scope group.
    cli.runExpectError('user', 'create', '--username', uniqueName('guest'), '--password', 'x', '--role', 'guest');
  });

  test('delete of a non-existent user fails', async ({ cli }) => {
    cli.runExpectError('user', 'delete', '99999999');
  });
});
