import { execFileSync } from 'child_process';

export interface CliResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

const RETRYABLE_PATTERNS = [
  'database is locked',
  'SQLITE_BUSY',
  'database table is locked',
];

const MAX_RETRIES = 3;
const RETRY_DELAYS = [500, 1000, 2000];

function sleep(ms: number): void {
  const { execSync } = require('child_process');
  execSync(`sleep ${ms / 1000}`);
}

function isRetryable(result: CliResult): boolean {
  const combined = result.stdout + result.stderr;
  return RETRYABLE_PATTERNS.some(p => combined.includes(p));
}

export class CliRunner {
  constructor(
    private binaryPath: string,
    private serverUrl: string,
  ) {}

  run(...args: string[]): CliResult {
    const fullArgs = ['--server', this.serverUrl, ...args];

    for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
      let result: CliResult;
      try {
        const stdout = execFileSync(this.binaryPath, fullArgs, {
          timeout: 30000,
          encoding: 'utf-8',
          stdio: ['pipe', 'pipe', 'pipe'],
        });
        result = { stdout: stdout || '', stderr: '', exitCode: 0 };
      } catch (error: any) {
        result = {
          stdout: error.stdout?.toString() || '',
          stderr: error.stderr?.toString() || '',
          exitCode: error.status ?? 1,
        };
      }

      if (attempt < MAX_RETRIES && result.exitCode !== 0 && isRetryable(result)) {
        sleep(RETRY_DELAYS[attempt]);
        continue;
      }

      return result;
    }

    throw new Error('Retry loop exhausted');
  }

  runOrFail(...args: string[]): CliResult {
    const result = this.run(...args);
    if (result.exitCode !== 0) {
      throw new Error(
        `CLI command failed (exit ${result.exitCode}):\n` +
        `  args: ${args.join(' ')}\n` +
        `  stdout: ${result.stdout}\n` +
        `  stderr: ${result.stderr}`
      );
    }
    return result;
  }

  runJson<T = any>(...args: string[]): T {
    const result = this.runOrFail(...args, '--json');
    try {
      return JSON.parse(result.stdout) as T;
    } catch (parseError) {
      throw new Error(
        `Failed to parse CLI JSON output:\n` +
        `  args: ${args.join(' ')} --json\n` +
        `  stdout: ${result.stdout}\n` +
        `  stderr: ${result.stderr}`
      );
    }
  }

  runExpectError(...args: string[]): CliResult {
    const result = this.run(...args);
    if (result.exitCode === 0) {
      throw new Error(
        `Expected CLI command to fail but it succeeded:\n` +
        `  args: ${args.join(' ')}\n` +
        `  stdout: ${result.stdout}`
      );
    }
    return result;
  }
}
