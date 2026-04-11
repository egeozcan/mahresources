import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { execSync } from 'child_process';

const SAMPLE_IMAGE = path.resolve(__dirname, '../../test-assets/sample-image.png');

interface Resource {
  ID: number;
  Name: string;
}

interface Group {
  ID: number;
  Name: string;
}

interface Category {
  ID: number;
  Name: string;
}

test.describe('mr group export round-trip', () => {
  const suffix = Date.now();
  let categoryId: number;
  let rootId: number;
  let childId: number;
  let resourceId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const cat = cli.runJson<Category>('category', 'create', '--name', `CliExportCat_${suffix}`);
    categoryId = cat.ID;

    const root = cli.runJson<Group>('group', 'create', '--name', `CliRoot_${suffix}`, '--category-id', String(categoryId));
    rootId = root.ID;

    const child = cli.runJson<Group>('group', 'create', '--name', `CliChild_${suffix}`, '--owner-id', String(rootId), '--category-id', String(categoryId));
    childId = child.ID;

    // Upload a resource and associate it with the root group via --owner-id.
    const resRaw = cli.runJson<Resource | Resource[]>('resource', 'upload', SAMPLE_IMAGE, '--name', `cli-export-img-${suffix}`, '--owner-id', String(rootId));
    const res = Array.isArray(resRaw) ? resRaw[0] : resRaw;
    resourceId = res.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('resource', 'delete', String(resourceId));
    cli.run('group', 'delete', String(childId));
    cli.run('group', 'delete', String(rootId));
    cli.run('category', 'delete', String(categoryId));
  });

  test('produces a readable tar containing manifest, groups, and resources', async ({ cli }) => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-export-'));
    const outPath = path.join(tmpDir, 'out.tar');

    try {
      cli.runOrFail('group', 'export', String(rootId), '-o', outPath, '--include-subtree', '--include-resources');

      const stat = fs.statSync(outPath);
      expect(stat.size).toBeGreaterThan(0);

      // Spot-check the tar contents.
      const listing = execSync(`tar -tf ${JSON.stringify(outPath)}`).toString();
      expect(listing).toContain('manifest.json');
      expect(listing).toMatch(/groups\/g\d+\.json/);
      expect(listing).toMatch(/resources\/r\d+\.json/);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });
});

test.describe('mr group export --no-wait', () => {
  const suffix = Date.now();
  let categoryId: number;
  let groupId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const cat = cli.runJson<Category>('category', 'create', '--name', `AsyncCat_${suffix}`);
    categoryId = cat.ID;
    const g = cli.runJson<Group>('group', 'create', '--name', `AsyncRoot_${suffix}`, '--category-id', String(categoryId));
    groupId = g.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('group', 'delete', String(groupId));
    cli.run('category', 'delete', String(categoryId));
  });

  test('returns the job id immediately without writing a file', async ({ cli }) => {
    const result = cli.runOrFail('group', 'export', String(groupId), '--no-wait');
    // Output is a bare job ID followed by newline (no prefix).
    expect(result.stdout.trim()).toMatch(/^[a-zA-Z0-9_-]+$/);
  });
});
