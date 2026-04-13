import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { execSync } from 'child_process';

interface Group {
  ID: number;
  Name: string;
}

interface Category {
  ID: number;
  Name: string;
}

test.describe('CLI: group import (full apply)', () => {
  const suffix = Date.now();
  let categoryId: number;
  let groupId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();

    const cat = cli.runJson<Category>('category', 'create', '--name', `ImportApplyCliCat_${suffix}`);
    categoryId = cat.ID;

    const g = cli.runJson<Group>(
      'group', 'create',
      '--name', `ImportApplyCliGroup_${suffix}`,
      '--category-id', String(categoryId),
    );
    groupId = g.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('group', 'delete', String(groupId));
    cli.run('category', 'delete', String(categoryId));
  });

  test('export then import round-trip', async ({ cli }) => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-import-apply-'));
    const tarPath = path.join(tmpDir, 'export.tar');

    try {
      // Export the group to a temp tar file
      cli.runOrFail(
        'group', 'export',
        String(groupId),
        '-o', tarPath,
        '--include-subtree',
        '--include-resources',
      );

      const stat = fs.statSync(tarPath);
      expect(stat.size).toBeGreaterThan(0);

      // Verify the tar looks sane
      const listing = execSync(`tar -tf ${JSON.stringify(tarPath)}`).toString();
      expect(listing).toContain('manifest.json');

      // Run full apply import with --auto-map (default) and --on-resource-conflict=duplicate
      // to avoid skip-by-hash since the same resource already exists on this server.
      // Use --guid-collision-policy=skip so GUID-matched groups are wired for relationships
      // but not re-created (since they already exist on this server).
      const result = cli.runOrFail(
        'group', 'import', tarPath,
        '--on-resource-conflict', 'duplicate',
        '--guid-collision-policy', 'skip',
      );

      // Verify output indicates success. Groups: 0 created because the group already
      // exists on this server and is matched by GUID (skip policy keeps it as-is).
      expect(result.stdout).toContain('Import applied successfully');
      expect(result.stdout).toMatch(/Groups:\s+0 created/);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  test('import with --on-resource-conflict=skip skips hash matches', async ({ cli }) => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-import-skip-'));
    const tarPath = path.join(tmpDir, 'export.tar');

    try {
      cli.runOrFail(
        'group', 'export',
        String(groupId),
        '-o', tarPath,
        '--include-subtree',
        '--include-resources',
      );

      // Use --on-resource-conflict=skip and --guid-collision-policy=skip.
      // The group already exists on this server and will be matched by GUID,
      // so it will not be re-created (0 created).
      const result = cli.runOrFail(
        'group', 'import', tarPath,
        '--on-resource-conflict', 'skip',
        '--guid-collision-policy', 'skip',
      );

      expect(result.stdout).toContain('Import applied successfully');
      expect(result.stdout).toMatch(/Groups:\s+0 created/);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });
});
