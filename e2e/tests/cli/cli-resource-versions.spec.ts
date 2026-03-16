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

// ResourceVersion API uses camelCase JSON field names
interface ResourceVersion {
  id: number;
  resourceId: number;
  versionNumber: number;
  hash: string;
  fileSize: number;
  contentType: string;
  width: number;
  height: number;
  comment: string;
  createdAt: string;
}

interface VersionComparison {
  sizeDelta: number;
  sameHash: boolean;
  sameType: boolean;
  dimensionsDiff: boolean;
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
    firstVersionId = versions[0].id;
    expect(versions[0].resourceId).toBe(resourceId);
    expect(versions[0].fileSize).toBeGreaterThan(0);
  });

  test('version-upload adds a new version', async ({ cli }) => {
    cli.runOrFail('resource', 'version-upload', String(resourceId), SAMPLE_IMAGE);
    const versions = cli.runJson<ResourceVersion[]>('resource', 'versions', String(resourceId));
    expect(versions.length).toBeGreaterThanOrEqual(2);
    const newVersion = versions.find(v => v.id !== firstVersionId);
    expect(newVersion).toBeDefined();
    secondVersionId = newVersion!.id;
  });

  test('version get returns correct fields', async ({ cli }) => {
    test.skip(!firstVersionId, 'No version ID available');
    const version = cli.runJson<ResourceVersion>('resource', 'version', String(firstVersionId));
    expect(version.id).toBe(firstVersionId);
    expect(version.resourceId).toBe(resourceId);
    expect(version.versionNumber).toBeGreaterThanOrEqual(1);
    expect(version.fileSize).toBeGreaterThan(0);
    expect(version.createdAt).toBeTruthy();
  });

  test('version-download writes file to disk', async ({ cli }) => {
    test.skip(!firstVersionId, 'No version ID available');
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
    test.skip(!secondVersionId, 'Second version not available (version-upload failed)');
    const comparison = cli.runJson<VersionComparison>(
      'resource', 'versions-compare', String(resourceId),
      '--v1', String(firstVersionId),
      '--v2', String(secondVersionId),
    );
    expect(typeof comparison.sameHash).toBe('boolean');
    expect(typeof comparison.sameType).toBe('boolean');
    expect(typeof comparison.sizeDelta).toBe('number');
    expect(typeof comparison.dimensionsDiff).toBe('boolean');
  });

  test('version-restore restores a previous version', async ({ cli }) => {
    test.skip(!firstVersionId, 'No version ID available');
    test.skip(!secondVersionId, 'Second version not available');
    cli.runOrFail(
      'resource', 'version-restore',
      '--resource-id', String(resourceId),
      '--version-id', String(firstVersionId),
    );

    const versions = cli.runJson<ResourceVersion[]>('resource', 'versions', String(resourceId));
    expect(versions.length).toBeGreaterThanOrEqual(3);
  });

  test('version-delete removes a version', async ({ cli }) => {
    test.skip(!secondVersionId, 'Second version not available');
    const versionsBefore = cli.runJson<ResourceVersion[]>('resource', 'versions', String(resourceId));
    const countBefore = versionsBefore.length;

    cli.runOrFail(
      'resource', 'version-delete',
      '--resource-id', String(resourceId),
      '--version-id', String(secondVersionId),
    );

    const versionsAfter = cli.runJson<ResourceVersion[]>('resource', 'versions', String(resourceId));
    expect(versionsAfter.length).toBe(countBefore - 1);

    const deleted = versionsAfter.find(v => v.id === secondVersionId);
    expect(deleted).toBeUndefined();
  });

  test('versions-cleanup for specific resource runs successfully', async ({ cli }) => {
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
    cli.runOrFail('resources', 'versions-cleanup', '--keep', '1', '--dry-run');
  });
});
