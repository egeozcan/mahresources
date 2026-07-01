import fs from 'fs';
import os from 'os';
import path from 'path';

/**
 * Defeats the server's global content-hash (SHA1) deduplication in tests.
 *
 * `AddResource` (application_context/resource_upload_context.go) dedupes uploads by a
 * global SHA1 across the whole DB. Each Playwright worker reuses ONE ephemeral server
 * (base.fixture.ts, scope:'worker') across every spec it runs, so two specs uploading the
 * same `test-assets/sample-image-N.png` collide: the second upload either 409s or silently
 * resolves to the first spec's resource (wrong owner/name/tag state). See the
 * `project_known_flaky_e2e` memory. Giving each upload unique bytes removes the collision.
 */

// Monotonic per-process counter. A Playwright worker is a single process whose ephemeral
// server + DB SURVIVE test retries, so a deterministic per-spec/per-image seed would
// re-collide with its own attempt-1 residue on retry — reintroducing the exact flake.
// A process-lifetime counter guarantees every upload (including on retry) gets a distinct SHA1.
let _seq = 0;

/** A short ASCII token unique for every call within this worker process. */
export function uniqueMarker(): string {
  _seq += 1;
  return `e2e-uniq-${process.pid}-${Date.now()}-${_seq}`;
}

/**
 * Returns `buffer` with a unique ASCII marker appended. The SHA1 changes (defeating dedup)
 * but the file still decodes: PNG/JPEG/GIF ignore trailing bytes after their end marker, and
 * text/SVG stay text because the tail is ASCII — a random *binary* tail would flip the
 * server's mimetype sniffing to application/octet-stream. Pixels are untouched, so perceptual
 * hashes are unaffected.
 *
 * NOT safe for strict container formats (e.g. tar) — do not use for import archives.
 */
export function uniquifyBuffer(buffer: Buffer): Buffer {
  return Buffer.concat([buffer, Buffer.from(`\n${uniqueMarker()}\n`, 'ascii')]);
}

/**
 * Writes a temp copy of `srcPath` with a unique ASCII marker appended and returns its path.
 * For UI uploads (`setInputFiles`) that must read a real file from disk. Files land in the OS
 * temp dir; callers need not clean up.
 */
export function uniqueAssetFile(srcPath: string): string {
  const ext = path.extname(srcPath);
  const stem = path.basename(srcPath, ext);
  const tmpPath = path.join(os.tmpdir(), `mahres-e2e-${stem}-${uniqueMarker()}${ext}`);
  fs.writeFileSync(tmpPath, uniquifyBuffer(fs.readFileSync(srcPath)));
  return tmpPath;
}
