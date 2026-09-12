import { test, expect } from '../../fixtures/cli.fixture';
import { mkdirSync, writeFileSync } from 'node:fs';

interface Carrier {
  ID: number;
  CustomEntityPickerResult: string;
  CustomEntityPickerResultCSS: string;
}

for (const command of ['category', 'note-type', 'resource-category']) {
  test(`${command} reads picker HTML and CSS from files on create and edit`, async ({ cli }, info) => {
    mkdirSync(info.outputDir, { recursive: true });
    const htmlPath = info.outputPath('picker.html'), cssPath = info.outputPath('picker.css');
    const html = '<b>[property path="Name"]</b>\n', css = '.entity-picker-result b { color: red; }\n';
    writeFileSync(htmlPath, html);writeFileSync(cssPath, css);
    const created = cli.runJson<Carrier>(command, 'create', '--name', `picker-file-${command}-${Date.now()}`,
      '--custom-entity-picker-result-file', htmlPath, '--custom-entity-picker-result-css-file', cssPath);
    try {
      expect(created.CustomEntityPickerResult).toBe(html);expect(created.CustomEntityPickerResultCSS).toBe(css);
      writeFileSync(htmlPath, '');writeFileSync(cssPath, '');
      cli.runOrFail(command, 'edit', '--id', String(created.ID), '--custom-entity-picker-result-file', htmlPath, '--custom-entity-picker-result-css-file', cssPath);
      const saved = cli.runJson<Carrier>(command, 'get', String(created.ID));
      expect(saved.CustomEntityPickerResult).toBe('');expect(saved.CustomEntityPickerResultCSS).toBe('');
    } finally {cli.run(command, 'delete', String(created.ID));}
  });
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
      cli.runOrFail(command, 'edit', '--id', id, '--custom-entity-picker-result', '', '--custom-entity-picker-result-css', '');
      const cleared = cli.runJson<Carrier>(command, 'get', id);
      expect(cleared.CustomEntityPickerResult).toBe('');
      expect(cleared.CustomEntityPickerResultCSS).toBe('');
    } finally {
      cli.run(command, 'delete', String(created.ID));
    }
  });
}
