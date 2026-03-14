import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';

interface Resource {
  ID: number;
  Name: string;
  FileSize: number;
  ContentType: string;
  Hash: string;
}

interface ResourceVersion {
  ID: number;
  ResourceID: number;
  VersionNumber: number;
  Hash: string;
  FileSize: number;
  ContentType: string;
  Width: number;
  Height: number;
  Comment: string;
  CreatedAt: string;
}

interface VersionComparison {
  SizeDelta: number;
  SameHash: boolean;
  SameType: boolean;
  DimensionsDiff: boolean;
}

const SAMPLE_DOC = path.resolve(__dirname, '../../test-assets/sample-document.txt');
const SAMPLE_IMAGE = path.resolve(__dirname, '../../test-assets/sample-image.png');

test.describe('Resource version lifecycle', () => {
  let resourceId: number;
  let firstVersionId: number;
  let secondVersionId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const result = cli.runJson<Resource | Resource[]>('resource', 'upload', SAMPLE_DOC);
    const res = Array.isArray(result) ? result[0] : result;
    resourceId = res.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('resource', 'delete', String(resourceId));
  });

  test('versions lists at least one version after upload', async ({ cli }) => {
    const versions = cli.runJson<ResourceVersion[]>('resource', 'versions', String(resourceId));
    expect(versions.length).toBeGreaterThanOrEqual(1);
    firstVersionId = versions[0].ID;
    expect(versions[0].ResourceID).toBe(resourceId);
    expect(versions[0].FileSize).toBeGreaterThan(0);
  });

  test('version-upload adds a new version', async ({ cli }) => {
    const result = cli.runOrFail('resource', 'version-upload', String(resourceId), SAMPLE_IMAGE);
    expect(result.exitCode).toBe(0);

    const versions = cli.runJson<ResourceVersion[]>('resource', 'versions', String(resourceId));
    expect(versions.length).toBeGreaterThanOrEqual(2);

    // Find the new version (not the first one)
    const newVersion = versions.find(v => v.ID !== firstVersionId);
    expect(newVersion).toBeDefined();
    secondVersionId = newVersion!.ID;
  });

  test('version get returns correct fields', async ({ cli }) => {
    const version = cli.runJson<ResourceVersion>('resource', 'version', String(firstVersionId));
    expect(version.ID).toBe(firstVersionId);
    expect(version.ResourceID).toBe(resourceId);
    expect(version.VersionNumber).toBeGreaterThanOrEqual(1);
    expect(version.FileSize).toBeGreaterThan(0);
    expect(version.CreatedAt).toBeTruthy();
  });

  test('version-download writes file to disk', async ({ cli }) => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'cli-test-'));
    try {
      const outPath = path.join(tmpDir, 'version-file');
      cli.runOrFail('resource', 'version-download', String(firstVersionId), '--output', outPath);

      expect(fs.existsSync(outPath)).toBe(true);
      const stat = fs.statSync(outPath);
      expect(stat.size).toBeGreaterThan(0);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  test('versions-compare compares two versions', async ({ cli }) => {
    const comparison = cli.runJson<VersionComparison>(
      'resource', 'versions-compare', String(resourceId),
      '--v1', String(firstVersionId),
      '--v2', String(secondVersionId),
    );
    // The two versions use different files (doc vs image), so they should differ
    expect(typeof comparison.SameHash).toBe('boolean');
    expect(typeof comparison.SameType).toBe('boolean');
    expect(typeof comparison.SizeDelta).toBe('number');
    expect(typeof comparison.DimensionsDiff).toBe('boolean');
  });

  test('version-restore restores a previous version', async ({ cli }) => {
    cli.runOrFail(
      'resource', 'version-restore',
      '--resource-id', String(resourceId),
      '--version-id', String(firstVersionId),
    );

    // After restore, there should be at least 3 versions (original, second, restored)
    const versions = cli.runJson<ResourceVersion[]>('resource', 'versions', String(resourceId));
    expect(versions.length).toBeGreaterThanOrEqual(3);
  });

  test('version-delete removes a version', async ({ cli }) => {
    // Get current version list before delete
    const versionsBefore = cli.runJson<ResourceVersion[]>('resource', 'versions', String(resourceId));
    const countBefore = versionsBefore.length;

    // Delete the second version
    cli.runOrFail(
      'resource', 'version-delete',
      '--resource-id', String(resourceId),
      '--version-id', String(secondVersionId),
    );

    const versionsAfter = cli.runJson<ResourceVersion[]>('resource', 'versions', String(resourceId));
    expect(versionsAfter.length).toBe(countBefore - 1);

    // The deleted version should not be in the list
    const deleted = versionsAfter.find(v => v.ID === secondVersionId);
    expect(deleted).toBeUndefined();
  });

  test('versions-cleanup for specific resource runs successfully', async ({ cli }) => {
    // Use --dry-run to avoid actually deleting remaining versions
    cli.runOrFail('resource', 'versions-cleanup', String(resourceId), '--keep', '1', '--dry-run');
  });
});

test.describe('Global versions-cleanup (plural resources)', () => {
  let resourceId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const result = cli.runJson<Resource | Resource[]>('resource', 'upload', SAMPLE_DOC);
    const res = Array.isArray(result) ? result[0] : result;
    resourceId = res.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('resource', 'delete', String(resourceId));
  });

  test('resources versions-cleanup runs without error', async ({ cli }) => {
    // Use --dry-run to safely verify the command works without side effects
    cli.runOrFail('resources', 'versions-cleanup', '--keep', '1', '--dry-run');
  });
});
