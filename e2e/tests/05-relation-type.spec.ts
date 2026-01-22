import { test, expect } from '../fixtures/base.fixture';

test.describe('RelationType CRUD Operations', () => {
  let personCategoryId: number;
  let companyCategoryId: number;
  let createdRelationTypeId: number;
  let testRunId: string;

  test.beforeAll(async ({ apiClient }) => {
    // Generate unique ID at beforeAll time to handle retries
    testRunId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    // Create categories needed for relation types
    const personCategory = await apiClient.createCategory(`Person RT Test ${testRunId}`, 'Person category for relation type tests');
    personCategoryId = personCategory.ID;

    const companyCategory = await apiClient.createCategory(`Company RT Test ${testRunId}`, 'Company category for relation type tests');
    companyCategoryId = companyCategory.ID;
  });

  test('should create a new relation type', async ({ relationTypePage }) => {
    createdRelationTypeId = await relationTypePage.create({
      name: `Works At ${testRunId}`,
      description: 'Employment relationship',
      fromCategoryName: `Person RT Test ${testRunId}`,
      toCategoryName: `Company RT Test ${testRunId}`,
    });
    expect(createdRelationTypeId).toBeGreaterThan(0);
  });

  test('should display the created relation type', async ({ relationTypePage, page }) => {
    expect(createdRelationTypeId, 'RelationType must be created first').toBeGreaterThan(0);
    await relationTypePage.gotoDisplay(createdRelationTypeId);
    await expect(page.locator('h1, .title')).toContainText(`Works At ${testRunId}`);
  });

  test('should update the relation type', async ({ relationTypePage, page }) => {
    expect(createdRelationTypeId, 'RelationType must be created first').toBeGreaterThan(0);
    await relationTypePage.update(createdRelationTypeId, {
      name: `Employed By ${testRunId}`,
      description: 'Updated employment relationship',
    });
    await expect(page.locator('h1, .title')).toContainText(`Employed By ${testRunId}`);
  });

  test('should list the relation type', async ({ relationTypePage }) => {
    await relationTypePage.verifyRelationTypeInList(`Employed By ${testRunId}`);
  });

  test('should delete the relation type', async ({ relationTypePage }) => {
    expect(createdRelationTypeId, 'RelationType must be created first').toBeGreaterThan(0);
    await relationTypePage.delete(createdRelationTypeId);
    await relationTypePage.verifyRelationTypeNotInList(`Employed By ${testRunId}`);
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up categories
    if (personCategoryId) {
      await apiClient.deleteCategory(personCategoryId);
    }
    if (companyCategoryId) {
      await apiClient.deleteCategory(companyCategoryId);
    }
  });
});

test.describe('RelationType with same category', () => {
  let genericRelationTypeId: number;
  let genericCategoryId: number;
  let testRunId: string;

  test.beforeAll(async ({ apiClient }) => {
    // Generate unique ID at beforeAll time to handle retries
    testRunId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    // Create a generic category for self-referential relations
    const category = await apiClient.createCategory(`Generic RT Cat ${testRunId}`, 'Generic category for relation type');
    genericCategoryId = category.ID;
  });

  test('should create relation type with same from/to category', async ({ relationTypePage }) => {
    // Relation types require categories, so we test with the same category for from/to
    genericRelationTypeId = await relationTypePage.create({
      name: `Related To ${testRunId}`,
      description: 'Generic relationship with same category for from/to',
      fromCategoryName: `Generic RT Cat ${testRunId}`,
      toCategoryName: `Generic RT Cat ${testRunId}`,
    });
    expect(genericRelationTypeId).toBeGreaterThan(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (genericRelationTypeId) {
      await apiClient.deleteRelationType(genericRelationTypeId);
    }
    if (genericCategoryId) {
      await apiClient.deleteCategory(genericCategoryId);
    }
  });
});
