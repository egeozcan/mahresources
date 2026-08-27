import { describe, expect, test } from 'vitest';
// @ts-expect-error -- plain JS module with no type declarations
import { imageCompare } from './imageCompare.js';

/**
 * The compare page's overlay modes place two images in one coordinate space.
 * Everything about whether that space is right reduces to `_sizes`, so these
 * tests are about how `_sizes` is filled and what is derived from it.
 *
 * The e2e suite (`compare-registration-scale.spec.ts`) measures the rendered
 * result; this measures the arithmetic, including the cases a browser will not
 * hand you on demand -- a flip mid-load, an image that reports zeros, the same
 * size arriving from all four modes at once.
 */

function component(leftSize: unknown, rightSize: unknown) {
  return imageCompare({
    leftUrl: '/v1/resource/version/file?versionId=1',
    rightUrl: '/v1/resource/version/file?versionId=2',
    leftLabel: 'Version 1',
    rightLabel: 'Version 2',
    leftSize,
    rightSize,
  });
}

/** The two properties `noteSizeFrom` reads off a real `<img>`, and nothing else. */
function img(side: 'lead' | 'trail', naturalWidth: number, naturalHeight: number) {
  return { naturalWidth, naturalHeight, dataset: { compareSide: side } };
}

describe('filling sizes from the loaded images', () => {
  test('a loaded image overrides the dimensions the server stored', () => {
    // The EXIF case: Go's DecodeConfig ignores orientation and stores 800x600,
    // every browser paints and reports 600x800. Trusting the stored pair builds
    // an 800x800 box for two images the reader sees as identically shaped.
    const c = component({ w: 800, h: 600 }, { w: 600, h: 800 });
    expect(c.overlayRatio).toBe('800 / 800');

    c.noteSizeFrom(img('lead', 600, 800));
    expect(c.overlayRatio).toBe('600 / 800');
    expect(c.leadScale).toEqual(c.trailScale);
  });

  test('an image with no dimensions of its own leaves the stored ones alone', () => {
    // A dimensionless SVG, a HEIC no browser renders, a fetch that failed.
    const c = component({ w: 800, h: 600 }, { w: 400, h: 300 });
    c.noteSizeFrom(img('lead', 0, 0));
    c.noteSizeFrom(img('trail', 400, 0));
    expect(c.overlayRatio).toBe('800 / 600');
  });

  test('a pair the server had no dimensions for is registered entirely from the browser', () => {
    // AVIF: an accepted content type with no Go decoder anywhere in the tree.
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    expect(c.overlayRatio).toBeNull();
    expect(c.leadScale).toEqual({ width: '', height: '' });
    expect(c.overlayBoxStyle).toEqual({ aspectRatio: '' });

    c.noteSizeFrom(img('lead', 400, 300));
    // Still nothing: one known size cannot describe a shared box, and a half-
    // filled pair must keep rendering the way an empty one does.
    expect(c.overlayRatio).toBeNull();

    c.noteSizeFrom(img('trail', 800, 600));
    expect(c.overlayRatio).toBe('800 / 600');
    expect(c.overlayBoxStyle).toEqual({ aspectRatio: '800 / 600' });
    expect(c.leadScale).toEqual({ width: '50%', height: '50%' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%' });
  });

  test('a size is recorded against the version, not the side it was showing on', () => {
    // Eight <img> elements report into two slots, and a flip re-fires `load` on
    // all of them. Recording against the side would transpose the whole box.
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    c.swapped = true;
    c.noteSizeFrom(img('lead', 400, 300));
    c.noteSizeFrom(img('trail', 800, 600));

    // Swapped: the lead slot showed version 2, the trail slot version 1.
    expect(c._sizes).toEqual([{ w: 800, h: 600 }, { w: 400, h: 300 }]);
    // And the sizes survive the flip back, still attached to their own version.
    c.swapped = false;
    expect(c.leadScale).toEqual({ width: '100%', height: '100%' });
    expect(c.trailScale).toEqual({ width: '50%', height: '50%' });
  });

  test('repeated reports of a size already known change nothing', () => {
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    c.noteSizeFrom(img('lead', 400, 300));
    c.noteSizeFrom(img('trail', 800, 600));
    const settled = c._sizes;

    // All four modes hold both images, so every side reports up to four times.
    for (let i = 0; i < 4; i++) {
      c.noteSizeFrom(img('lead', 400, 300));
      c.noteSizeFrom(img('trail', 800, 600));
    }
    expect(c._sizes).toBe(settled);
  });

  test('anything that is not an image is ignored rather than thrown at', () => {
    const c = component({ w: 800, h: 600 }, { w: 400, h: 300 });
    for (const value of [null, undefined, {}, { naturalWidth: '400', naturalHeight: '300' }]) {
      expect(() => c.noteSizeFrom(value)).not.toThrow();
    }
    expect(c.overlayRatio).toBe('800 / 600');
  });
});

describe('styles are objects, because x-show writes to the same attribute', () => {
  // A string `x-bind:style` replaces the whole style attribute, and `x-show`
  // hides an element by writing `display: none` onto it. Every styled element
  // in this component is also x-shown, so a string binding that re-renders
  // un-hides it. Nothing about the rendered page distinguishes the two forms;
  // only the type does, so the type is what is asserted.
  test('every style a re-render can change is emitted as an object', () => {
    const c = component({ w: 400, h: 300 }, { w: 800, h: 600 });
    for (const style of [c.leadScale, c.trailScale, c.overlayBoxStyle]) {
      expect(typeof style).toBe('object');
    }
  });

  test('a property that stops applying is emitted empty rather than dropped', () => {
    // Alpine only touches the keys an object names, so a key present in one
    // render and absent from the next keeps its old value forever.
    const known = component({ w: 400, h: 300 }, { w: 800, h: 600 });
    const unknown = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    expect(Object.keys(unknown.leadScale).sort()).toEqual(Object.keys(known.leadScale).sort());
    expect(Object.keys(unknown.overlayBoxStyle).sort()).toEqual(Object.keys(known.overlayBoxStyle).sort());
  });
});
