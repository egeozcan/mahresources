/**
 * Turn a drop's DataTransfer into a flat list of files that each remember the
 * folder chain they were found in.
 *
 * Two phases, because a DataTransfer is only readable synchronously inside the
 * drop handler: `snapshotDrop` grabs the entries right there, `walkDrop` is then
 * free to await.
 */

const IGNORED_NAMES = new Set(['thumbs.db', 'desktop.ini']);

/** Dotfiles and OS litter. Applied to what is found *inside* a dropped folder. */
export function isIgnoredName(name) {
  return name.startsWith('.') || IGNORED_NAMES.has(name.toLowerCase());
}

/**
 * Capture everything the drop carries, synchronously.
 * @param {DataTransfer} dataTransfer
 * @returns {{ entries: object[], loose: File[] }}
 */
export function snapshotDrop(dataTransfer) {
  const entries = [];
  const loose = [];
  if (dataTransfer?.items) {
    for (const item of dataTransfer.items) {
      if (item.kind !== 'file') continue;
      const entry = item.webkitGetAsEntry?.();
      if (entry) {
        entries.push(entry);
      } else {
        const file = item.getAsFile();
        if (file) loose.push(file);
      }
    }
  } else if (dataTransfer?.files) {
    loose.push(...dataTransfer.files);
  }
  return { entries, loose };
}

const fileOf = (entry) => new Promise((resolve, reject) => entry.file(resolve, reject));

// readEntries returns one batch per call (100 in Chromium) and an empty array at
// the end, so a single call silently drops the rest of a big folder.
async function readAll(directory) {
  const reader = directory.createReader();
  const all = [];
  for (;;) {
    const batch = await new Promise((resolve, reject) => reader.readEntries(resolve, reject));
    if (batch.length === 0) return all;
    all.push(...batch);
  }
}

/**
 * @param {{ entries: object[], loose: File[] }} snapshot
 * @returns {Promise<{ files: Array<{ file: File, dirPath: string[] }>, unreadable: number }>}
 *   `dirPath` is the folder chain from the dropped root down (`[]` for a loose
 *   file). A folder only shows up through the files beneath it, so folders that
 *   are empty or hold only ignored names produce nothing.
 */
export async function walkDrop({ entries, loose }) {
  const files = loose.map((file) => ({ file, dirPath: [] }));
  let unreadable = 0;

  async function visit(entry, dirPath, isRoot) {
    if (!isRoot && isIgnoredName(entry.name)) return;
    try {
      if (entry.isFile) {
        files.push({ file: await fileOf(entry), dirPath });
      } else if (entry.isDirectory) {
        const inner = [...dirPath, entry.name];
        for (const child of await readAll(entry)) await visit(child, inner, false);
      }
    } catch (_) {
      unreadable++;
    }
  }

  // A root is something the user picked on purpose, so a dot-name is kept.
  for (const entry of entries) await visit(entry, [], true);
  return { files, unreadable };
}
