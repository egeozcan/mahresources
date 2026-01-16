import { test, expect } from '../fixtures/base.fixture';

test.describe('RelationType CRUD Operations', () => {
  let personCategoryId: number;
  let companyCategoryId: number;
  let createdRelationTypeId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create categories needed for relation types
    const personCategory = await apiClient.createCategory('Person RT Test', 'Person category for relation type tests');
    personCategoryId = personCategory.ID;

    const companyCategory = await apiClient.createCategory('Company RT Test', 'Company category for relation type tests');
    companyCategoryId = companyCategory.ID;
  });

  test('should create a new relation type', async ({ relationTypePage }) => {
    createdRelationTypeId = await relationTypePage.create({
      name: 'Works At',
      description: 'Employment relationship',
      fromCategoryName: 'Person RT Test',
      toCategoryName: 'Company RT Test',
    });
    expect(createdRelationTypeId).toBeGreaterThan(0);
  });

  test('should display the created relation type', async ({ relationTypePage, page }) => {
    expect(createdRelationTypeId, 'RelationType must be created first').toBeGreaterThan(0);
    await relationTypePage.gotoDisplay(createdRelationTypeId);
    await expect(page.locator('h1, .title')).toContainText('Works At');
  });

  test('should update the relation type', async ({ relationTypePage, page }) => {
    expect(createdRelationTypeId, 'RelationType must be created first').toBeGreaterThan(0);
    await relationTypePage.update(createdRelationTypeId, {
      name: 'Employed By',
      description: 'Updated employment relationship',
    });
    await expect(page.locator('h1, .title')).toContainText('Employed By');
  });

  test('should list the relation type', async ({ relationTypePage }) => {
    await relationTypePage.verifyRelationTypeInList('Employed By');
  });

  test('should delete the relation type', async ({ relationTypePage }) => {
    expect(createdRelationTypeId, 'RelationType must be created first').toBeGreaterThan(0);
    await relationTypePage.delete(createdRelationTypeId);
    await relationTypePage.verifyRelationTypeNotInList('Employed By');
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

test.describe('RelationType without Categories', () => {
  let genericRelationTypeId: number;

  test('should create relation type without category constraints', async ({ relationTypePage }) => {
    genericRelationTypeId = await relationTypePage.create({
      name: 'Related To',
      description: 'Generic relationship without category constraints',
    });
    expect(genericRelationTypeId).toBeGreaterThan(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (genericRelationTypeId) {
      await apiClient.deleteRelationType(genericRelationTypeId);
    }
  });
});
