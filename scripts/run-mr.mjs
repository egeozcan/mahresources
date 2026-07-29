import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import path from 'node:path';

const exe = process.platform === 'win32' ? '.exe' : '';
const binary = path.resolve(process.cwd(), `mr${exe}`);

if (!existsSync(binary)) {
  console.error(`mr CLI not found at ${binary}. Run 'npm run build-cli' first.`);
  process.exit(1);
}

const child = spawn(binary, process.argv.slice(2), { stdio: 'inherit' });
child.on('exit', (code, signal) => {
  process.exit(signal ? 1 : (code ?? 0));
});
