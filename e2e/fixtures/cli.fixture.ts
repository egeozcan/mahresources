import { test as base, expect } from '@playwright/test';
import { CliRunner } from '../helpers/cli-runner';
import * as path from 'path';

export function createCliRunner(): CliRunner {
  const binaryPath = process.env.CLI_PATH || path.resolve(__dirname, '../../mr');
  const serverUrl = process.env.CLI_BASE_URL || process.env.BASE_URL || 'http://localhost:8181';
  return new CliRunner(binaryPath, serverUrl);
}

type CliFixtures = {
  cli: CliRunner;
};

export const test = base.extend<CliFixtures>({
  cli: async ({}, use) => {
    await use(createCliRunner());
  },
});

export { expect };
