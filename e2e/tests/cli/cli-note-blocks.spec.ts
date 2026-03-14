import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface Note {
  ID: number;
  Name: string;
}

interface NoteBlock {
  ID: number;
  NoteID: number;
  Type: string;
  Position: string;
  Content: any;
  State: any;
  CreatedAt: string;
  UpdatedAt: string;
}

test.describe('Note block types', () => {
  test('types returns a list of available block types', async ({ cli }) => {
    const types = cli.runJson<string[]>('note-block', 'types');
    expect(Array.isArray(types)).toBe(true);
    expect(types.length).toBeGreaterThan(0);
    expect(types).toContain('text');
  });
});

test.describe('Note block CRUD lifecycle', () => {
  const suffix = Date.now();
  let noteId: number;
  let blockId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const note = cli.runJson<Note>('note', 'create', '--name', `block-note-${suffix}`);
    noteId = note.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note', 'delete', String(noteId));
  });

  test('create a note block', async ({ cli }) => {
    const block = cli.runJson<NoteBlock>(
      'note-block', 'create',
      '--note-id', String(noteId),
      '--type', 'text',
      '--content', '{"text":"hello world"}'
    );
    expect(block.ID).toBeGreaterThan(0);
    expect(block.NoteID).toBe(noteId);
    expect(block.Type).toBe('text');
    blockId = block.ID;
  });

  test('get the created note block', async ({ cli }) => {
    const block = cli.runJson<NoteBlock>('note-block', 'get', String(blockId));
    expect(block.ID).toBe(blockId);
    expect(block.NoteID).toBe(noteId);
    expect(block.Type).toBe('text');
  });

  test('update note block content', async ({ cli }) => {
    cli.runOrFail('note-block', 'update', String(blockId), '--content', '{"text":"updated content"}');

    const block = cli.runJson<NoteBlock>('note-block', 'get', String(blockId));
    expect(block.Content).toBeDefined();
    // Content should reflect the update
    const content = typeof block.Content === 'string' ? JSON.parse(block.Content) : block.Content;
    expect(content.text).toBe('updated content');
  });

  test('update note block state', async ({ cli }) => {
    cli.runOrFail('note-block', 'update-state', String(blockId), '--state', '{"collapsed":true}');

    const block = cli.runJson<NoteBlock>('note-block', 'get', String(blockId));
    expect(block.State).toBeDefined();
    const state = typeof block.State === 'string' ? JSON.parse(block.State) : block.State;
    expect(state.collapsed).toBe(true);
  });

  test('delete note block', async ({ cli }) => {
    cli.runOrFail('note-block', 'delete', String(blockId));

    const result = cli.run('note-block', 'get', String(blockId), '--json');
    expect(result.exitCode).not.toBe(0);
  });
});

test.describe('Note blocks list', () => {
  const suffix = Date.now();
  let noteId: number;
  let blockId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const note = cli.runJson<Note>('note', 'create', '--name', `listblk-note-${suffix}`);
    noteId = note.ID;
    const block = cli.runJson<NoteBlock>(
      'note-block', 'create',
      '--note-id', String(noteId),
      '--type', 'text',
      '--content', '{"text":"list test"}'
    );
    blockId = block.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note-block', 'delete', String(blockId));
    cli.run('note', 'delete', String(noteId));
  });

  test('list note blocks returns the created block', async ({ cli }) => {
    const blocks = cli.runJson<NoteBlock[]>('note-blocks', 'list', '--note-id', String(noteId));
    expect(blocks.length).toBeGreaterThan(0);
    const match = blocks.find(b => b.ID === blockId);
    expect(match).toBeDefined();
  });
});

test.describe('Note blocks rebalance and reorder', () => {
  const suffix = Date.now();
  let noteId: number;
  let block1Id: number;
  let block2Id: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const note = cli.runJson<Note>('note', 'create', '--name', `reorder-note-${suffix}`);
    noteId = note.ID;
    const b1 = cli.runJson<NoteBlock>(
      'note-block', 'create',
      '--note-id', String(noteId),
      '--type', 'text',
      '--content', '{"text":"first"}'
    );
    block1Id = b1.ID;
    const b2 = cli.runJson<NoteBlock>(
      'note-block', 'create',
      '--note-id', String(noteId),
      '--type', 'text',
      '--content', '{"text":"second"}'
    );
    block2Id = b2.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note-block', 'delete', String(block1Id));
    cli.run('note-block', 'delete', String(block2Id));
    cli.run('note', 'delete', String(noteId));
  });

  test('rebalance updates positions', async ({ cli }) => {
    cli.runOrFail('note-blocks', 'rebalance', '--note-id', String(noteId));

    // After rebalancing, blocks should have ordered positions
    const blocks = cli.runJson<NoteBlock[]>('note-blocks', 'list', '--note-id', String(noteId));
    expect(blocks.length).toBe(2);

    // Positions should be non-empty after rebalance
    for (const block of blocks) {
      expect(block.Position).toBeTruthy();
    }
  });

  test('reorder with explicit positions', async ({ cli }) => {
    // Swap positions: put block2 first (position "a") and block1 second (position "b")
    const positions = JSON.stringify({
      [String(block2Id)]: 'a',
      [String(block1Id)]: 'b',
    });
    cli.runOrFail('note-blocks', 'reorder', '--note-id', String(noteId), '--positions', positions);

    // Verify positions were updated
    const blocks = cli.runJson<NoteBlock[]>('note-blocks', 'list', '--note-id', String(noteId));
    const b1 = blocks.find(b => b.ID === block1Id);
    const b2 = blocks.find(b => b.ID === block2Id);
    expect(b1).toBeDefined();
    expect(b2).toBeDefined();
    expect(b2!.Position).toBe('a');
    expect(b1!.Position).toBe('b');
  });
});

test.describe('Note block create without required flags', () => {
  test('create without --note-id fails', async ({ cli }) => {
    cli.runExpectError('note-block', 'create', '--type', 'text');
  });

  test('create without --type fails', async ({ cli }) => {
    cli.runExpectError('note-block', 'create', '--note-id', '1');
  });
});
