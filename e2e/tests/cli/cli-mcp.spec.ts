import { spawn, ChildProcessWithoutNullStreams } from 'child_process';
import * as path from 'path';
import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

/**
 * Drives `mr mcp` the way an MCP client does: JSON-RPC over the binary's stdin
 * and stdout. The client speaks protocol 2025-11-25 and advertises form
 * elicitation, so a destructive call reaches it as an elicitation/create
 * request from the bridge, which the test answers.
 */
class StdioClient {
  private proc: ChildProcessWithoutNullStreams;
  private buf = '';
  private nextId = 1;
  private pending = new Map<number, (msg: any) => void>();
  elicitations: string[] = [];

  constructor(serverUrl: string, private answer: 'accept' | 'decline') {
    const binary = process.env.CLI_PATH || path.resolve(__dirname, '../../../mr');
    this.proc = spawn(binary, ['--server', serverUrl, 'mcp'], { stdio: ['pipe', 'pipe', 'pipe'] });
    this.proc.stdout.setEncoding('utf-8');
    this.proc.stdout.on('data', (chunk: string) => this.onData(chunk));
  }

  private onData(chunk: string) {
    this.buf += chunk;
    let nl: number;
    while ((nl = this.buf.indexOf('\n')) >= 0) {
      const line = this.buf.slice(0, nl).trim();
      this.buf = this.buf.slice(nl + 1);
      if (!line) continue;
      const msg = JSON.parse(line);
      if (msg.method === 'elicitation/create') {
        this.elicitations.push(msg.params.message);
        const result = this.answer === 'accept'
          ? { action: 'accept', content: { confirm: true } }
          : { action: 'decline' };
        this.send({ jsonrpc: '2.0', id: msg.id, result });
      } else if (msg.id !== undefined && this.pending.has(msg.id)) {
        this.pending.get(msg.id)!(msg);
        this.pending.delete(msg.id);
      }
    }
  }

  private send(msg: object) {
    this.proc.stdin.write(JSON.stringify(msg) + '\n');
  }

  request(method: string, params: object = {}): Promise<any> {
    const id = this.nextId++;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`timeout waiting for ${method}`)), 20000);
      this.pending.set(id, (msg) => { clearTimeout(timer); resolve(msg); });
      this.send({ jsonrpc: '2.0', id, method, params });
    });
  }

  async initialize() {
    const init = await this.request('initialize', {
      protocolVersion: '2025-11-25',
      capabilities: { elicitation: { form: {} } },
      clientInfo: { name: 'cli-e2e', version: '1' },
    });
    this.send({ jsonrpc: '2.0', method: 'notifications/initialized' });
    return init;
  }

  close() {
    this.proc.stdin.end();
    this.proc.kill();
  }
}

test.describe('mr mcp', () => {
  test('lists tools and forwards a read', async ({ workerServer }) => {
    const cli = createCliRunner();
    const name = `mcp-e2e-read-${Date.now()}`;
    const note = JSON.parse(cli.runOrFail('note', 'create', '--name', name, '--json').stdout);

    const client = new StdioClient(`http://127.0.0.1:${workerServer.port}`, 'accept');
    try {
      const init = await client.initialize();
      expect(init.result.serverInfo.name).toBe('mahresources');

      const list = await client.request('tools/list');
      const names = list.result.tools.map((t: any) => t.name);
      expect(names).toEqual(expect.arrayContaining(['search', 'get_note', 'delete', 'call_operation']));

      const got = await client.request('tools/call', { name: 'get_note', arguments: { id: note.ID } });
      expect(got.result.isError).toBeFalsy();
      expect(got.result.content[0].text).toContain(name);
    } finally {
      client.close();
    }
  });

  test('a delete runs only after the user confirms', async ({ workerServer }) => {
    const cli = createCliRunner();
    const declinedName = `mcp-e2e-declined-${Date.now()}`;
    const acceptedName = `mcp-e2e-accepted-${Date.now()}`;
    const declined = JSON.parse(cli.runOrFail('note', 'create', '--name', declinedName, '--json').stdout);
    const accepted = JSON.parse(cli.runOrFail('note', 'create', '--name', acceptedName, '--json').stdout);
    const url = `http://127.0.0.1:${workerServer.port}`;

    const no = new StdioClient(url, 'decline');
    try {
      await no.initialize();
      const res = await no.request('tools/call', { name: 'delete', arguments: { entity: 'note', id: declined.ID } });
      expect(res.result.content[0].text).toContain('Cancelled');
      expect(no.elicitations).toHaveLength(1);
      expect(no.elicitations[0]).toContain(declinedName);
    } finally {
      no.close();
    }
    expect(cli.run('note', 'get', String(declined.ID), '--json').exitCode).toBe(0);

    const yes = new StdioClient(url, 'accept');
    try {
      await yes.initialize();
      const res = await yes.request('tools/call', { name: 'delete', arguments: { entity: 'note', id: accepted.ID } });
      expect(res.result.isError).toBeFalsy();
      expect(yes.elicitations[0]).toContain(acceptedName);
    } finally {
      yes.close();
    }
    expect(cli.run('note', 'get', String(accepted.ID), '--json').exitCode).not.toBe(0);
  });
});
