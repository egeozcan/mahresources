import { describe, expect, it } from 'vitest';
import { shouldUniquifyUpload } from '../../e2e/helpers/unique-upload';

describe('E2E upload byte uniquification', () => {
  it('only mutates trailing-byte-tolerant image fixtures by default', () => {
    expect(shouldUniquifyUpload('/tmp/sample.png')).toBe(true);
    expect(shouldUniquifyUpload('/tmp/sample.jpg')).toBe(true);
    expect(shouldUniquifyUpload('/tmp/sample.jpeg')).toBe(true);
    expect(shouldUniquifyUpload('/tmp/sample.gif')).toBe(true);

    expect(shouldUniquifyUpload('/tmp/sample.svg')).toBe(false);
    expect(shouldUniquifyUpload('/tmp/sample.mp4')).toBe(false);
    expect(shouldUniquifyUpload('/tmp/sample.txt')).toBe(false);
  });

  it('lets callers request exact bytes for otherwise tolerant formats', () => {
    expect(shouldUniquifyUpload('/tmp/sample.png', true)).toBe(false);
  });
});
