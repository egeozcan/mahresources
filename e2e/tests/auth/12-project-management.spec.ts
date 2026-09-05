import {test,expect,loginAs} from '../../fixtures/auth.fixture';

test('scoped PM guest sees styled badges and no edit controls',async ({page,authSeed}) => {
  await loginAs(page,authSeed.admin);
  const headers = {'X-CSRF-Token':(await (await page.request.get('/v1/auth/me')).json()).csrfToken};
  const enabled = await page.request.post('/v1/plugin/enable',{headers,form:{name:'project-management'}});
  expect(enabled.ok(),await enabled.text()).toBe(true);
  const setup = await page.request.post('/v1/plugins/project-management/api/setup',{headers,data:{}});
  expect(setup.ok(),await setup.text()).toBe(true);
  const tax = await setup.json();
  const opened = await page.request.post('/v1/plugin/scopedAccess',{headers,form:{name:'project-management',allowed:'true'}});
  expect(opened.ok(),await opened.text()).toBe(true);
  const note = await page.request.post('/v1/note',{headers,form:{name:'Guest PM Task',ownerId:String(authSeed.scopeGroupId),noteTypeId:String(tax.task_type_id)}});
  expect(note.ok(),await note.text()).toBe(true);
  const id = (await note.json()).ID;
  await loginAs(page,authSeed.guest);
  await page.goto(`/note?id=${id}`);
  await expect(page.getByTestId('pm-task-detail')).toContainText('To Do');
  await expect(page.locator('pm-status-control,pm-due-control,pm-owner-control')).toHaveCount(0);
  await expect(page.locator('meta-shortcode[data-editable="true"]')).toHaveCount(0);
  await expect(page.locator('.pm-pill').first()).toHaveCSS('border-radius','9999px');
});
