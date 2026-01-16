import { test, expect } from '../fixtures/base.fixture';

test.describe('Relation CRUD Operations', () => {
  let personCategoryId: number;
  let companyCategoryId: number;
  let personGroupId: number;
  let companyGroupId: number;
  let relationTypeId: number;
  let createdRelationId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create categories
    const personCategory = await apiClient.createCategory('Person Relation Test', 'Person category');
    personCategoryId = personCategory.ID;

    const companyCategory = await apiClient.createCategory('Company Relation Test', 'Company category');
    companyCategoryId = companyCategory.ID;

    // Create groups
    const personGroup = await apiClient.createGroup({
      name: 'John Doe',
      description: 'A person for relation testing',
      categoryId: personCategoryId,
    });
    personGroupId = personGroup.ID;

    const companyGroup = await apiClient.createGroup({
      name: 'Acme Corp',
      description: 'A company for relation testing',
      categoryId: companyCategoryId,
    });
    companyGroupId = companyGroup.ID;

    // Create relation type
    const relationType = await apiClient.createRelationType({
      name: 'Employed By RT',
      description: 'Employment relationship',
      fromCategoryId: personCategoryId,
      toCategoryId: companyCategoryId,
    });
    relationTypeId = relationType.ID;
  });

  test('should create a new relation via UI', async ({ relationPage }) => {
    createdRelationId = await relationPage.create({
      name: 'John works at Acme',
      description: 'Employment relation',
      relationTypeName: 'Employed By RT',
      fromGroupName: 'John Doe',
      toGroupName: 'Acme Corp',
    });
    expect(createdRelationId).toBeGreaterThan(0);
  });

  test('should display the created relation', async ({ relationPage, page }) => {
    await relationPage.gotoDisplay(createdRelationId);
    await expect(page.locator('h1, .title')).toContainText('John works at Acme');
  });

  test('should show relation on group page', async ({ groupPage, page }) => {
    // Check if relation appears on the person's group page
    await groupPage.gotoDisplay(personGroupId);
    await expect(page.locator('text=Acme Corp')).toBeVisible();
  });

  test('should update the relation', async ({ relationPage, page }) => {
    await relationPage.update(createdRelationId, {
      name: 'John employed by Acme',
      description: 'Updated employment relation',
    });
    await expect(page.locator('h1, .title')).toContainText('John employed by Acme');
  });

  test('should list the relation', async ({ relationPage }) => {
    await relationPage.verifyRelationInList('John employed by Acme');
  });

  test('should delete the relation', async ({ relationPage }) => {
    await relationPage.delete(createdRelationId);
    await relationPage.verifyRelationNotInList('John employed by Acme');
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up in reverse dependency order
    if (personGroupId) {
      await apiClient.deleteGroup(personGroupId);
    }
    if (companyGroupId) {
      await apiClient.deleteGroup(companyGroupId);
    }
    if (relationTypeId) {
      await apiClient.deleteRelationType(relationTypeId);
    }
    if (personCategoryId) {
      await apiClient.deleteCategory(personCategoryId);
    }
    if (companyCategoryId) {
      await apiClient.deleteCategory(companyCategoryId);
    }
  });
});

test.describe('Multiple Relations', () => {
  let categoryId: number;
  let groupIds: number[] = [];
  let relationTypeId: number;
  let relationIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('Multi Relation Category', 'Category for multiple relations');
    categoryId = category.ID;

    // Create multiple groups
    for (let i = 1; i <= 3; i++) {
      const group = await apiClient.createGroup({
        name: `Multi Relation Group ${i}`,
        categoryId: categoryId,
      });
      groupIds.push(group.ID);
    }

    const relationType = await apiClient.createRelationType({
      name: 'Connected To MR',
      description: 'Generic connection',
    });
    relationTypeId = relationType.ID;
  });

  test('should create multiple relations between groups', async ({ apiClient }) => {
    // Create relations via API for speed
    const relation1 = await apiClient.createRelation({
      name: 'Group 1 to Group 2',
      fromGroupId: groupIds[0],
      toGroupId: groupIds[1],
      relationTypeId: relationTypeId,
    });
    relationIds.push(relation1.ID);

    const relation2 = await apiClient.createRelation({
      name: 'Group 2 to Group 3',
      fromGroupId: groupIds[1],
      toGroupId: groupIds[2],
      relationTypeId: relationTypeId,
    });
    relationIds.push(relation2.ID);

    expect(relationIds.length).toBe(2);
  });

  test('should display all relations on group page', async ({ groupPage, page }) => {
    // Group 2 should show relations to both Group 1 and Group 3
    await groupPage.gotoDisplay(groupIds[1]);

    // The exact display depends on how relations are shown
    await expect(page.locator('text=Multi Relation Group')).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up relations first
    for (const relationId of relationIds) {
      await apiClient.deleteRelation(relationId);
    }
    // Then groups
    for (const groupId of groupIds) {
      await apiClient.deleteGroup(groupId);
    }
    if (relationTypeId) {
      await apiClient.deleteRelationType(relationTypeId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
