/**
 * Regression test: deleting a group must NOT delete its owned resources.
 *
 * Background: DeleteGroup uses Select(...) to cascade-delete join-table
 * associations (tags, related resources, etc.). OwnResources is a has-many
 * relationship that must be excluded from the cascade, otherwise the actual
 * resource records (and their files) are destroyed — irreversible data loss.
 */
import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe.serial('Group Deletion Preserves Owned Resources', () => {
  let categoryId: number;
  let groupId: number;
  let resourceId: number;
  const testRunId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    // Create a category (required for groups)
    const category = await apiClient.createCategory(
      `Preserve Resources Category ${testRunId}`,
      'Category for group-delete-preserves-resources test'
    );
    categoryId = category.ID;

    // Create a group that will own a resource
    const group = await apiClient.createGroup({
      name: `Owner Group ${testRunId}`,
      description: 'This group will be deleted; its resource must survive',
      categoryId: category.ID,
    });
    groupId = group.ID;

    // Create a resource owned by the group
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-10.png'),
      name: `Owned Resource ${testRunId}`,
      description: 'This resource is owned by the group and must survive deletion',
      ownerId: group.ID,
    });
    resourceId = resource.ID;
  });

  test('should confirm resource is owned by the group before deletion', async ({ apiClient }) => {
    const resource = await apiClient.getResource(resourceId);
    expect(resource).toBeTruthy();
    expect(resource.Name).toContain(`Owned Resource ${testRunId}`);
  });

  test('should delete the group successfully', async ({ apiClient }) => {
    await apiClient.deleteGroup(groupId);

    // Verify the group is gone
    try {
      await apiClient.getGroup(groupId);
      throw new Error('Group should have been deleted');
    } catch (err) {
      expect(String(err)).toContain('error');
    }
  });

  test('should still be able to fetch the resource after group deletion', async ({ apiClient }) => {
    // This is the critical assertion: the resource must survive
    const resource = await apiClient.getResource(resourceId);
    expect(resource).toBeTruthy();
    expect(resource.ID).toBe(resourceId);
    expect(resource.Name).toContain(`Owned Resource ${testRunId}`);
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up the resource (it should still exist)
    try {
      if (resourceId) await apiClient.deleteResource(resourceId);
    } catch { /* ignore */ }
    // Group already deleted by the test
    try {
      if (categoryId) await apiClient.deleteCategory(categoryId);
    } catch { /* ignore */ }
  });
});
