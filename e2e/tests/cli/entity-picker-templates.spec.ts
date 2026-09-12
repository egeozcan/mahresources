import { test, expect } from '../../fixtures/cli.fixture';

interface Carrier {
  ID: number;
  CustomEntityPickerResult: string;
  CustomEntityPickerResultCSS: string;
}

for (const command of ['category', 'note-type', 'resource-category']) {
  test(`${command} preserves picker template flags through a partial edit`, async ({ cli }) => {
    const html = '<b>[property path="Name"]</b>';
    const css = '.entity-picker-result b{color:red}';
    const created = cli.runJson<Carrier>(command, 'create', '--name', `picker-${command}-${Date.now()}`,
      '--custom-entity-picker-result', html, '--custom-entity-picker-result-css', css);
    try {
      const id = String(created.ID);
      cli.runOrFail(command, 'edit-description', id, 'Only the description changed');
      const saved = cli.runJson<Carrier>(command, 'get', id);
      expect(saved.CustomEntityPickerResult).toBe(html);
      expect(saved.CustomEntityPickerResultCSS).toBe(css);
      if (command === 'note-type') {
        cli.runOrFail(command, 'edit', '--id', id, '--custom-entity-picker-result', '', '--custom-entity-picker-result-css', '');
        const cleared = cli.runJson<Carrier>(command, 'get', id);
        expect(cleared.CustomEntityPickerResult).toBe('');
        expect(cleared.CustomEntityPickerResultCSS).toBe('');
      }
    } finally {
      cli.run(command, 'delete', String(created.ID));
    }
  });
}
