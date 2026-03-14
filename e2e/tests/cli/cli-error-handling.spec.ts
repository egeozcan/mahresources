import { test, expect } from '../../fixtures/cli.fixture';
import { CliRunner } from '../../helpers/cli-runner';
import * as path from 'path';

test.describe('error handling', () => {
  test.describe('missing required flags', () => {
    test('tag create without --name fails', async ({ cli }) => {
      const result = cli.runExpectError('tag', 'create');
      expect(result.stderr).toContain('required');
    });

    test('note create without --name fails', async ({ cli }) => {
      const result = cli.runExpectError('note', 'create');
      expect(result.stderr).toContain('required');
    });

    test('query create without --text fails', async ({ cli }) => {
      const result = cli.runExpectError('query', 'create', '--name', 'test');
      expect(result.stderr).toContain('required');
    });
  });

  test.describe('invalid arguments', () => {
    test('tag get with non-numeric ID fails', async ({ cli }) => {
      const result = cli.runExpectError('tag', 'get', 'abc');
      const output = result.stderr + result.stdout;
      expect(output).toMatch(/invalid/i);
    });

    test('tag get without ID fails', async ({ cli }) => {
      const result = cli.runExpectError('tag', 'get');
      expect(result.stderr).toContain('accepts 1 arg');
    });

    test('tag edit-name with too few args fails', async ({ cli }) => {
      const result = cli.runExpectError('tag', 'edit-name', '1');
      expect(result.stderr).toContain('accepts 2 arg');
    });
  });

  test.describe('non-existent entities', () => {
    test('tag get with non-existent ID fails', async ({ cli }) => {
      const result = cli.runExpectError('tag', 'get', '999999');
      const output = result.stderr + result.stdout;
      expect(output).toMatch(/not found/i);
    });

    test('note get with non-existent ID fails', async ({ cli }) => {
      const result = cli.run('note', 'get', '999999');
      expect(result.exitCode).not.toBe(0);
    });

    test('category get with non-existent ID fails', async ({ cli }) => {
      const result = cli.runExpectError('category', 'get', '999999');
      const output = result.stderr + result.stdout;
      expect(output).toMatch(/not found/i);
    });
  });

  test.describe('server connectivity', () => {
    test('connection refused with bad server URL', async () => {
      const badCli = new CliRunner(
        process.env.CLI_PATH || path.resolve(__dirname, '../../../mr'),
        'http://localhost:1'
      );
      const result = badCli.run('tags', 'list');
      expect(result.exitCode).not.toBe(0);
      const output = result.stderr + result.stdout;
      expect(output).toMatch(/connect|refused|connection|dial/i);
    });
  });

  test.describe('edge cases', () => {
    test('special characters in name', async ({ cli }) => {
      const name = `cli-special-"quotes"-${Date.now()}`;
      const tag = cli.runJson('tag', 'create', '--name', name);
      expect(tag.Name).toBe(name);
      cli.run('tag', 'delete', String(tag.ID));
    });

    test('unicode in name', async ({ cli }) => {
      const name = `cli-unicode-\u00e9\u00e8\u00ea-${Date.now()}`;
      const tag = cli.runJson('tag', 'create', '--name', name);
      expect(tag.Name).toBe(name);
      cli.run('tag', 'delete', String(tag.ID));
    });

    test('very long name', async ({ cli }) => {
      const name = `cli-long-${'x'.repeat(200)}-${Date.now()}`;
      const tag = cli.runJson('tag', 'create', '--name', name);
      expect(tag.Name).toBe(name);
      cli.run('tag', 'delete', String(tag.ID));
    });
  });
});
