import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { execSync } from 'child_process';

// ---------------------------------------------------------------------------
// Shared types
// ---------------------------------------------------------------------------

interface Group {
  ID: number;
  Name: string;
  GUID?: string;
  Tags?: Array<{ ID: number; Name: string }>;
}

interface Tag {
  ID: number;
  Name: string;
}

interface Category {
  ID: number;
  Name: string;
}

// Shape of a plan JSON written by --plan-output (enough for decisions building)
interface ImportPlan {
  counts: { groups: number };
  mappings: {
    categories: MappingEntry[];
    note_types: MappingEntry[];
    resource_categories: MappingEntry[];
    tags: MappingEntry[];
    group_relation_types: MappingEntry[];
  };
  dangling_refs: Array<{ id: string }>;
  items: PlanItem[];
}

interface MappingEntry {
  decision_key: string;
  suggestion: string;
  destination_id?: number;
  ambiguous?: boolean;
}

interface PlanItem {
  export_id: string;
  shell?: boolean;
  children?: PlanItem[];
}

// Minimal ImportDecisions shape accepted by the server
interface ImportDecisions {
  resource_collision_policy: string;
  guid_collision_policy?: string;
  acknowledge_missing_hashes?: boolean;
  mapping_actions: Record<string, { include: boolean; action: string; destination_id?: number }>;
  dangling_actions: Record<string, { action: string }>;
  shell_group_actions: Record<string, { action: string }>;
  excluded_items: string[];
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Build an ImportDecisions JSON file from a plan.
 * Mirrors the logic in buildCLIDecisions in cmd/mr/commands/group_import.go,
 * but also lets the caller override guid_collision_policy.
 */
function buildDecisionsFromPlan(plan: ImportPlan, opts: {
  guidPolicy?: string;
  resourcePolicy?: string;
}): ImportDecisions {
  const d: ImportDecisions = {
    resource_collision_policy: opts.resourcePolicy ?? 'skip',
    guid_collision_policy: opts.guidPolicy,
    mapping_actions: {},
    dangling_actions: {},
    shell_group_actions: {},
    excluded_items: [],
  };

  const allMappings: MappingEntry[] = [
    ...plan.mappings.categories,
    ...plan.mappings.note_types,
    ...plan.mappings.resource_categories,
    ...plan.mappings.tags,
    ...plan.mappings.group_relation_types,
  ];

  for (const entry of allMappings) {
    const action = entry.suggestion || 'create';
    d.mapping_actions[entry.decision_key] = {
      include: true,
      action,
      ...(entry.destination_id != null ? { destination_id: entry.destination_id } : {}),
    };
  }

  for (const dr of plan.dangling_refs ?? []) {
    d.dangling_actions[dr.id] = { action: 'drop' };
  }

  function walkItems(items: PlanItem[]) {
    for (const item of items ?? []) {
      if (item.shell) {
        d.shell_group_actions[item.export_id] = { action: 'create' };
      }
      walkItems(item.children ?? []);
    }
  }
  walkItems(plan.items ?? []);

  return d;
}

/**
 * Export a group to a tar and return the tar path.
 * Caller must clean up the tmpDir.
 */
function exportGroup(cli: ReturnType<typeof createCliRunner>, groupId: number, tmpDir: string): string {
  const tarPath = path.join(tmpDir, 'export.tar');
  cli.runOrFail('group', 'export', String(groupId), '-o', tarPath);
  return tarPath;
}

/**
 * Run a dry-run import, capture the plan JSON, then apply with given decisions.
 */
function importWithDecisions(
  cli: ReturnType<typeof createCliRunner>,
  tarPath: string,
  decisions: ImportDecisions,
  tmpDir: string,
): string {
  const planPath = path.join(tmpDir, 'plan.json');
  const decisionsPath = path.join(tmpDir, 'decisions.json');

  // Dry-run to get the plan JSON
  cli.runOrFail('group', 'import', tarPath, '--dry-run', '--plan-output', planPath);

  // Build decisions and write to file
  const plan: ImportPlan = JSON.parse(fs.readFileSync(planPath, 'utf-8'));
  const enrichedDecisions = buildDecisionsFromPlan(plan, {
    guidPolicy: decisions.guid_collision_policy,
    resourcePolicy: decisions.resource_collision_policy,
  });

  fs.writeFileSync(decisionsPath, JSON.stringify(enrichedDecisions), 'utf-8');

  // Apply with the decisions file
  const result = cli.runOrFail(
    'group', 'import', tarPath,
    '--decisions', decisionsPath,
  );
  return result.stdout;
}

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

test.describe('GUID export/import round-trip', () => {
  const suffix = Date.now();
  let categoryId: number;
  let tag1Id: number;
  let tag2Id: number;

  // Shared category and tags for all sub-suites
  test.beforeAll(() => {
    const cli = createCliRunner();

    const cat = cli.runJson<Category>('category', 'create', '--name', `GUIDRoundTripCat_${suffix}`);
    categoryId = cat.ID;

    const t1 = cli.runJson<Tag>('tag', 'create', '--name', `guid-tag-alpha-${suffix}`);
    tag1Id = t1.ID;

    const t2 = cli.runJson<Tag>('tag', 'create', '--name', `guid-tag-beta-${suffix}`);
    tag2Id = t2.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('tag', 'delete', String(tag1Id));
    cli.run('tag', 'delete', String(tag2Id));
    cli.run('category', 'delete', String(categoryId));
  });

  // -------------------------------------------------------------------------
  // Test 1: exported entities have GUIDs in the manifest
  // -------------------------------------------------------------------------
  test.describe('Test 1: exported group has a GUID in the manifest', () => {
    let groupId: number;

    test.beforeAll(() => {
      const cli = createCliRunner();
      const g = cli.runJson<Group>('group', 'create', '--name', `GUIDTest1_${suffix}`);
      groupId = g.ID;
    });

    test.afterAll(() => {
      const cli = createCliRunner();
      cli.run('group', 'delete', String(groupId));
    });

    test('manifest entry has a non-empty GUID field', async ({ cli }) => {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-guid-test1-'));
      try {
        const tarPath = exportGroup(cli, groupId, tmpDir);
        expect(fs.statSync(tarPath).size).toBeGreaterThan(0);

        // Extract and inspect manifest.json
        execSync(`tar -xf ${JSON.stringify(tarPath)} -C ${JSON.stringify(tmpDir)} manifest.json`);
        const manifest = JSON.parse(fs.readFileSync(path.join(tmpDir, 'manifest.json'), 'utf-8'));

        expect(manifest.entries.groups).toHaveLength(1);
        const entry = manifest.entries.groups[0];
        expect(entry.guid).toBeTruthy();
        // UUID v4 format: 8-4-4-4-12 hex chars
        expect(entry.guid).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/);
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });
  });

  // -------------------------------------------------------------------------
  // Test 2: re-import with merge policy — incoming name wins, tags union-merged
  // -------------------------------------------------------------------------
  test.describe('Test 2: re-import with guid_collision_policy=merge', () => {
    let groupId: number;
    const originalName = `GUIDMerge_${suffix}`;
    const modifiedName = `GUIDMerge_MODIFIED_${suffix}`;

    test.beforeAll(() => {
      const cli = createCliRunner();
      const g = cli.runJson<Group>(
        'group', 'create',
        '--name', originalName,
        '--tags', String(tag1Id),
        '--category-id', String(categoryId),
      );
      groupId = g.ID;
    });

    test.afterAll(() => {
      const cli = createCliRunner();
      cli.run('group', 'delete', String(groupId));
    });

    test('name reverts to export name, tags are union of both', async ({ cli }) => {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-guid-merge-'));
      try {
        // 1. Export (captures: name=originalName, tags=[tag1])
        const tarPath = exportGroup(cli, groupId, tmpDir);

        // 2. Locally modify group: change name, add tag2
        cli.runOrFail('group', 'edit-name', String(groupId), modifiedName);
        cli.runOrFail('groups', 'add-tags', '--ids', String(groupId), '--tags', String(tag2Id));

        // Verify modification
        const modified = cli.runJson<Group>('group', 'get', String(groupId));
        expect(modified.Name).toBe(modifiedName);

        // 3. Re-import with merge policy
        const output = importWithDecisions(
          cli,
          tarPath,
          {
            resource_collision_policy: 'skip',
            guid_collision_policy: 'merge',
            mapping_actions: {},
            dangling_actions: {},
            shell_group_actions: {},
            excluded_items: [],
          },
          tmpDir,
        );
        expect(output).toContain('Import applied successfully');

        // 4. Verify: name reverted to export name (incoming wins on merge)
        const after = cli.runJson<Group>('group', 'get', String(groupId));
        expect(after.Name).toBe(originalName);

        // 5. Verify: union of tags — both tag1 and tag2 should be present
        const tagIds = (after.Tags ?? []).map(t => t.ID);
        expect(tagIds).toContain(tag1Id);
        expect(tagIds).toContain(tag2Id);
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });
  });

  // -------------------------------------------------------------------------
  // Test 3: re-import with skip policy — local changes preserved
  // -------------------------------------------------------------------------
  test.describe('Test 3: re-import with guid_collision_policy=skip', () => {
    let groupId: number;
    const originalName = `GUIDSkip_${suffix}`;
    const modifiedName = `GUIDSkip_MODIFIED_${suffix}`;

    test.beforeAll(() => {
      const cli = createCliRunner();
      const g = cli.runJson<Group>('group', 'create', '--name', originalName);
      groupId = g.ID;
    });

    test.afterAll(() => {
      const cli = createCliRunner();
      cli.run('group', 'delete', String(groupId));
    });

    test('local changes are preserved (skip leaves existing group untouched)', async ({ cli }) => {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-guid-skip-'));
      try {
        // 1. Export (captures: name=originalName)
        const tarPath = exportGroup(cli, groupId, tmpDir);

        // 2. Locally modify name
        cli.runOrFail('group', 'edit-name', String(groupId), modifiedName);

        // 3. Re-import with skip policy
        const output = importWithDecisions(
          cli,
          tarPath,
          {
            resource_collision_policy: 'skip',
            guid_collision_policy: 'skip',
            mapping_actions: {},
            dangling_actions: {},
            shell_group_actions: {},
            excluded_items: [],
          },
          tmpDir,
        );
        expect(output).toContain('Import applied successfully');

        // 4. Verify: local name unchanged (skip preserves existing)
        const after = cli.runJson<Group>('group', 'get', String(groupId));
        expect(after.Name).toBe(modifiedName);
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });
  });

  // -------------------------------------------------------------------------
  // Test 4: re-import with replace policy — tags exactly the export's tags
  // -------------------------------------------------------------------------
  test.describe('Test 4: re-import with guid_collision_policy=replace', () => {
    let groupId: number;
    const originalName = `GUIDReplace_${suffix}`;

    test.beforeAll(() => {
      const cli = createCliRunner();
      const g = cli.runJson<Group>(
        'group', 'create',
        '--name', originalName,
        '--tags', String(tag1Id),
      );
      groupId = g.ID;
    });

    test.afterAll(() => {
      const cli = createCliRunner();
      cli.run('group', 'delete', String(groupId));
    });

    test('extra tag added locally is removed on replace import', async ({ cli }) => {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-guid-replace-'));
      try {
        // 1. Export (captures: tags=[tag1])
        const tarPath = exportGroup(cli, groupId, tmpDir);

        // 2. Locally add tag2 (extra tag not in the export)
        cli.runOrFail('groups', 'add-tags', '--ids', String(groupId), '--tags', String(tag2Id));

        const withExtraTag = cli.runJson<Group>('group', 'get', String(groupId));
        const beforeTagIds = (withExtraTag.Tags ?? []).map(t => t.ID);
        expect(beforeTagIds).toContain(tag2Id);

        // 3. Re-import with replace policy
        const output = importWithDecisions(
          cli,
          tarPath,
          {
            resource_collision_policy: 'skip',
            guid_collision_policy: 'replace',
            mapping_actions: {},
            dangling_actions: {},
            shell_group_actions: {},
            excluded_items: [],
          },
          tmpDir,
        );
        expect(output).toContain('Import applied successfully');

        // 4. Verify: tags are exactly the export's tags — only tag1 present, tag2 removed
        const after = cli.runJson<Group>('group', 'get', String(groupId));
        const afterTagIds = (after.Tags ?? []).map(t => t.ID);
        expect(afterTagIds).toContain(tag1Id);
        expect(afterTagIds).not.toContain(tag2Id);
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });
  });

  // -------------------------------------------------------------------------
  // Test 5: default import (no explicit GUID policy) acts like merge
  // -------------------------------------------------------------------------
  test.describe('Test 5: default guid_collision_policy is merge', () => {
    let groupId: number;
    const originalName = `GUIDDefault_${suffix}`;
    const modifiedName = `GUIDDefault_MODIFIED_${suffix}`;

    test.beforeAll(() => {
      const cli = createCliRunner();
      const g = cli.runJson<Group>('group', 'create', '--name', originalName);
      groupId = g.ID;
    });

    test.afterAll(() => {
      const cli = createCliRunner();
      cli.run('group', 'delete', String(groupId));
    });

    test('re-importing without explicit policy reverts name (default=merge, incoming wins)', async ({ cli }) => {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mr-guid-default-'));
      try {
        // 1. Export (captures: name=originalName)
        const tarPath = exportGroup(cli, groupId, tmpDir);

        // 2. Locally modify name
        cli.runOrFail('group', 'edit-name', String(groupId), modifiedName);

        // 3. Re-import with NO explicit guid policy (CLI default applies, server defaults to merge)
        const result = cli.runOrFail(
          'group', 'import', tarPath,
          '--on-resource-conflict', 'skip',
        );
        expect(result.stdout).toContain('Import applied successfully');

        // 4. With default merge policy, incoming name overwrites local modification
        const after = cli.runJson<Group>('group', 'get', String(groupId));
        expect(after.Name).toBe(originalName);
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });
  });
});
