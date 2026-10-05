import { describe, expect, it } from 'vitest';
import { isIgnoredName, snapshotDrop, walkDrop } from './dropEntries.js';

function fileEntry(name: string, body = 'x') {
  const file = new File([body], name);
  return {
    isFile: true,
    isDirectory: false,
    name,
    file: (ok: (f: File) => void) => ok(file),
  };
}

// readEntries hands back at most `batch` entries per call and an empty array
// once exhausted, which is what Chromium does at 100.
function dirEntry(name: string, children: any[], batch = 100) {
  return {
    isFile: false,
    isDirectory: true,
    name,
    createReader() {
      let at = 0;
      return {
        readEntries(ok: (e: any[]) => void) {
          const slice = children.slice(at, at + batch);
          at += slice.length;
          ok(slice);
        },
      };
    },
  };
}

function transfer(entries: any[]) {
  return {
    items: entries.map((entry) => ({
      kind: 'file',
      webkitGetAsEntry: () => entry,
      getAsFile: () => null,
    })),
    files: [],
  } as unknown as DataTransfer;
}

describe('isIgnoredName', () => {
  it('ignores dotfiles and OS litter, keeps ordinary names', () => {
    expect(isIgnoredName('.DS_Store')).toBe(true);
    expect(isIgnoredName('.git')).toBe(true);
    expect(isIgnoredName('Thumbs.db')).toBe(true);
    expect(isIgnoredName('desktop.ini')).toBe(true);
    expect(isIgnoredName('photo.jpg')).toBe(false);
    expect(isIgnoredName('my.notes.txt')).toBe(false);
  });
});

describe('walkDrop', () => {
  it('returns a loose file with an empty dirPath', async () => {
    const { files } = await walkDrop(snapshotDrop(transfer([fileEntry('a.txt')])));
    expect(files.map((f) => [f.file.name, f.dirPath])).toEqual([['a.txt', []]]);
  });

  it('records the folder chain for nested files', async () => {
    const tree = dirEntry('photos', [
      fileEntry('top.jpg'),
      dirEntry('2024', [fileEntry('a.jpg'), dirEntry('raw', [fileEntry('b.cr2')])]),
    ]);
    const { files } = await walkDrop(snapshotDrop(transfer([tree])));
    expect(files.map((f) => [f.file.name, f.dirPath])).toEqual([
      ['top.jpg', ['photos']],
      ['a.jpg', ['photos', '2024']],
      ['b.cr2', ['photos', '2024', 'raw']],
    ]);
  });

  it('keeps reading until the reader is exhausted', async () => {
    const many = Array.from({ length: 250 }, (_, i) => fileEntry(`f${i}.txt`));
    const { files } = await walkDrop(snapshotDrop(transfer([dirEntry('big', many, 100)])));
    expect(files).toHaveLength(250);
  });

  it('skips ignored files, and folders that hold only ignored content', async () => {
    const tree = dirEntry('mixed', [
      fileEntry('.DS_Store'),
      fileEntry('keep.txt'),
      dirEntry('junk', [fileEntry('Thumbs.db'), fileEntry('.hidden')]),
      dirEntry('empty', []),
      dirEntry('.git', [fileEntry('config')]),
    ]);
    const { files } = await walkDrop(snapshotDrop(transfer([tree])));
    expect(files.map((f) => [f.file.name, f.dirPath])).toEqual([['keep.txt', ['mixed']]]);
  });

  it('keeps an explicitly dropped dotfile or dot-folder root', async () => {
    const { files } = await walkDrop(
      snapshotDrop(transfer([fileEntry('.env'), dirEntry('.config', [fileEntry('a.txt')])])),
    );
    expect(files.map((f) => [f.file.name, f.dirPath])).toEqual([
      ['.env', []],
      ['a.txt', ['.config']],
    ]);
  });

  it('tells apart two roots that share a name', async () => {
    const { files } = await walkDrop(
      snapshotDrop(transfer([dirEntry('photos', [fileEntry('1')]), dirEntry('photos', [fileEntry('2')])])),
    );
    expect(files.map((f) => [f.file.name, f.root])).toEqual([['1', 0], ['2', 1]]);
  });

  it('gives each dropped root its own drop-local identity', async () => {
    const { files } = await walkDrop(
      snapshotDrop(transfer([dirEntry('a', [fileEntry('1')]), dirEntry('b', [fileEntry('2')])])),
    );
    expect(new Set(files.map((f) => f.dirPath[0]))).toEqual(new Set(['a', 'b']));
  });

  it('counts entries it could not read instead of throwing', async () => {
    const broken = {
      isFile: true,
      isDirectory: false,
      name: 'bad.bin',
      file: (_ok: unknown, fail: (e: Error) => void) => fail(new Error('gone')),
    };
    const { files, unreadable } = await walkDrop(
      snapshotDrop(transfer([broken, fileEntry('ok.txt')])),
    );
    expect(files.map((f) => f.file.name)).toEqual(['ok.txt']);
    expect(unreadable).toBe(1);
  });

  it('falls back to dataTransfer.files when the entry API is missing', async () => {
    const f = new File(['x'], 'plain.txt');
    const dt = { items: undefined, files: [f] } as unknown as DataTransfer;
    const { files } = await walkDrop(snapshotDrop(dt));
    expect(files.map((x) => [x.file.name, x.dirPath])).toEqual([['plain.txt', []]]);
  });
});
