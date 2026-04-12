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

test.describe('mr group import --dry-run', () => {
  const suffix = Date.now();
  let categoryId: number;
  let groupId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();

    const cat = cli.runJson<Category>('category', 'create', '--name', `ImportDryRunCat_${suffix}`);
    categoryId = cat.ID;

    const g = cli.runJson<Group>(
      'group', 'create',
      '--name', `ImportDryRunRoot_${suffix}`,
      '--category-id', String(categoryId),
    );
    groupId = g.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('group', 'delete', String(groupId));
    cli.run('category', 'delete', String(categoryId));
  });

  test('parses tar and prints plan without applying', async ({ cli }) => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-import-test-'));
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

      // Verify the tar looks sane before trying to import it
      const listing = execSync(`tar -tf ${JSON.stringify(tarPath)}`).toString();
      expect(listing).toContain('manifest.json');

      // Run dry-run import — should exit 0 and print the plan summary
      const result = cli.runOrFail('group', 'import', tarPath, '--dry-run');

      expect(result.stdout).toContain('Import Plan');
      expect(result.stdout).toContain('Groups:');
      // Our exported group should appear in the count
      expect(result.stdout).toMatch(/Groups:\s+1/);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  test('--plan-output writes plan JSON to a file', async ({ cli }) => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-import-plan-'));
    const tarPath = path.join(tmpDir, 'export.tar');
    const planPath = path.join(tmpDir, 'plan.json');

    try {
      cli.runOrFail(
        'group', 'export',
        String(groupId),
        '-o', tarPath,
        '--include-subtree',
      );

      cli.runOrFail(
        'group', 'import', tarPath,
        '--dry-run',
        '--plan-output', planPath,
      );

      // plan.json must exist and contain valid JSON with the expected shape
      const raw = fs.readFileSync(planPath, 'utf-8');
      const plan = JSON.parse(raw);
      expect(plan).toHaveProperty('counts');
      expect(plan.counts.groups).toBeGreaterThanOrEqual(1);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });
});
