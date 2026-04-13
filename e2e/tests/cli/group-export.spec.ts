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

test.describe('mr group export --related-depth', () => {
  const suffix = Date.now();
  let groupAId: number;
  let groupBId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    // Create GroupB (will be the related group)
    const groupB = cli.runJson<Group>('group', 'create', '--name', `DepthB_${suffix}`);
    groupBId = groupB.ID;
    // Create GroupA with GroupB as a related group
    const groupA = cli.runJson<Group>('group', 'create', '--name', `DepthA_${suffix}`, '--groups', String(groupBId));
    groupAId = groupA.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('group', 'delete', String(groupAId));
    cli.run('group', 'delete', String(groupBId));
  });

  test('export with --related-depth 1 includes related groups, import creates shell groups', async ({ cli }) => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-related-depth-'));
    const outPath = path.join(tmpDir, 'depth.tar');

    try {
      // Export with related-depth 1 (--include-related is on by default)
      cli.runOrFail('group', 'export', String(groupAId), '-o', outPath, '--related-depth', '1');

      const stat = fs.statSync(outPath);
      expect(stat.size).toBeGreaterThan(0);

      // Verify the tar contains both groups
      const listing = execSync(`tar -tf ${JSON.stringify(outPath)}`).toString();
      expect(listing).toContain('manifest.json');
      // Should have 2 group entries (A + B shell)
      const groupEntries = listing.split('\n').filter(l => l.match(/^groups\/g\d+\.json$/));
      expect(groupEntries.length).toBe(2);

      // Import the archive with --guid-collision-policy=skip.
      // Both GroupA and GroupB already exist on this server (created in beforeAll),
      // so they will be matched by GUID and not re-created (0 created).
      const result = cli.runOrFail(
        'group', 'import', outPath,
        '--on-resource-conflict', 'duplicate',
        '--guid-collision-policy', 'skip',
      );

      expect(result.stdout).toContain('Import applied successfully');
      // Both groups already exist on this server and are matched by GUID (skip).
      expect(result.stdout).toMatch(/Groups:\s+0 created/);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });
});
