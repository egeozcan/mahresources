import { describe, expect, test, vi } from 'vitest';
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

/**
 * The three properties `noteSizeFrom` reads off a real `<img>`, and nothing else.
 *
 * `version` is 1 or 2 and becomes `currentSrc` -- the URL of the image data the
 * element is *currently displaying*, which is what decides the slot. Deliberately
 * not the side the element sits on: that is the bug these tests exist to catch.
 */
function img(version: 1 | 2, naturalWidth: number, naturalHeight: number) {
  return {
    naturalWidth,
    naturalHeight,
    currentSrc: `/v1/resource/version/file?versionId=${version}`,
  };
}

describe('filling sizes from the loaded images', () => {
  test('a loaded image overrides the dimensions the server stored', () => {
    // The EXIF case: Go's DecodeConfig ignores orientation and stores 800x600,
    // every browser paints and reports 600x800. Trusting the stored pair builds
    // an 800x800 box for two images the reader sees as identically shaped.
    const c = component({ w: 800, h: 600 }, { w: 600, h: 800 });
    expect(c.overlayRatio).toBe('800 / 800');

    c.noteSizeFrom(img(1, 600, 800));
    expect(c.overlayRatio).toBe('600 / 800');
    expect(c.leadScale).toEqual(c.trailScale);
  });

  test('an image with no dimensions of its own leaves the stored ones alone', () => {
    // A dimensionless SVG, a HEIC no browser renders, a fetch that failed.
    const c = component({ w: 800, h: 600 }, { w: 400, h: 300 });
    c.noteSizeFrom(img(1, 0, 0));
    c.noteSizeFrom(img(2, 400, 0));
    expect(c.overlayRatio).toBe('800 / 600');
  });

  test('a pair the server had no dimensions for is registered entirely from the browser', () => {
    // AVIF: an accepted content type with no Go decoder anywhere in the tree.
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    expect(c.overlayRatio).toBeNull();
    expect(c.leadScale).toEqual({ width: '', height: '', objectFit: '', margin: '', transform: '', transformOrigin: '' });
    expect(c.overlayBoxStyle).toEqual({ aspectRatio: '' });

    c.noteSizeFrom(img(1, 400, 300));
    // Still nothing: one known size cannot describe a shared box, and a half-
    // filled pair must keep rendering the way an empty one does.
    expect(c.overlayRatio).toBeNull();

    c.noteSizeFrom(img(2, 800, 600));
    expect(c.overlayRatio).toBe('800 / 600');
    expect(c.overlayBoxStyle).toEqual({ aspectRatio: '800 / 600' });
    expect(c.leadScale).toEqual({ width: '50%', height: '50%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
  });

  test('a size is recorded against the version whose bytes were measured', () => {
    // Eight <img> elements report into two slots, and a flip re-fires `load` on
    // all of them.
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    c.swapped = true;
    c.noteSizeFrom(img(1, 400, 300));
    c.noteSizeFrom(img(2, 800, 600));

    expect(c._sizes).toEqual([{ w: 400, h: 300 }, { w: 800, h: 600 }]);
    // Swapped, the lead side shows version 2.
    expect(c.leadScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
    expect(c.trailScale).toEqual({ width: '50%', height: '50%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
    // And they survive the flip back, still attached to their own version.
    c.swapped = false;
    expect(c.leadScale).toEqual({ width: '50%', height: '50%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
  });

  test('a load that lands after a flip is still credited to its own version', () => {
    // The interleave, injected rather than raced for. A `load` queued for the
    // previous `src` runs after `swapped` has already changed, and the element
    // still reports the old image's dimensions because the new request has not
    // completed. Asking `swapped` which slot the lead side means would file
    // version 1's measurement under version 2 -- and leave it there, since the
    // replacement load may never arrive.
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    c.noteSizeFrom(img(2, 800, 600));

    c.swapped = true;
    c.noteSizeFrom(img(1, 400, 300));

    expect(c._sizes).toEqual([{ w: 400, h: 300 }, { w: 800, h: 600 }]);
    expect(c.overlayRatio).toBe('800 / 600');
  });

  test('an image showing neither version is ignored', () => {
    const c = component({ w: 800, h: 600 }, { w: 400, h: 300 });
    c.noteSizeFrom({ naturalWidth: 999, naturalHeight: 999, currentSrc: '/v1/resource/preview?id=7' });
    c.noteSizeFrom({ naturalWidth: 999, naturalHeight: 999, currentSrc: '' });
    expect(c.overlayRatio).toBe('800 / 600');
  });

  test('repeated reports of a size already known change nothing', () => {
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    c.noteSizeFrom(img(1, 400, 300));
    c.noteSizeFrom(img(2, 800, 600));
    const settled = c._sizes;

    // All four modes hold both images, so every side reports up to four times.
    for (let i = 0; i < 4; i++) {
      c.noteSizeFrom(img(1, 400, 300));
      c.noteSizeFrom(img(2, 800, 600));
    }
    expect(c._sizes).toBe(settled);
  });

  test('anything that is not an image is ignored rather than thrown at', () => {
    const c = component({ w: 800, h: 600 }, { w: 400, h: 300 });
    for (const value of [null, undefined, {}, { naturalWidth: '400', naturalHeight: '300' }, { naturalWidth: 400, naturalHeight: 300 }]) {
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

describe('scale policy', () => {
  /** `onRadiogroupKeydown` reaches for two Alpine affordances and nothing else. */
  function arrowRight(c: any) {
    c.$nextTick = (fn: () => void) => fn();
    c.onScaleKeydown({
      key: 'ArrowRight',
      preventDefault: () => {},
      currentTarget: { querySelector: () => null },
    });
  }

  test('relative scale draws each version at its true size against the other', () => {
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    expect(c.scale).toBe('relative');
    expect(c.leadScale).toEqual({ width: '66.66666666666666%', height: '37.5%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
  });

  test('fit grows each version until an edge touches the frame', () => {
    // 400x300 into a 600x800 box: width-limited, so 600x450 -- 100% wide and
    // 56.25% tall. The taller version already touches both edges.
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    c.setScale('fit');
    expect(c.leadScale).toEqual({ width: '100%', height: '56.25%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
  });

  test('fit registers a pure resolution change exactly', () => {
    // The case the package exists for: one aspect ratio, two resolutions. Under
    // relative scale the rescan draws at double size and lines up with nothing.
    const c = component({ w: 800, h: 600 }, { w: 1600, h: 1200 });
    expect(c.leadScale).toEqual({ width: '50%', height: '50%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
    c.setScale('fit');
    expect(c.leadScale).toEqual(c.trailScale);
    expect(c.leadScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '', transform: '', transformOrigin: '' });
  });

  test('stretch distorts both versions onto the whole frame', () => {
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    c.setScale('stretch');
    expect(c.leadScale).toEqual({ width: '100%', height: '100%', objectFit: 'fill', margin: '', transform: '', transformOrigin: '' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%', objectFit: 'fill', margin: '', transform: '', transformOrigin: '' });
  });

  test('leaving stretch takes the distortion back off', () => {
    // `object-fit` is only named by one of the three modes, so it has to be
    // cleared by the others rather than omitted: Alpine leaves a key it is not
    // given at whatever it was last set to.
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    c.setScale('stretch');
    c.setScale('relative');
    expect(c.leadScale.objectFit).toBe('');
  });

  test('a refused group still swallows the keys the pattern owns', () => {
    // Returning early without preventDefault hands ArrowDown, Home and End to
    // the browser's default scrolling. The checked radio is still focusable and
    // is the group's tab stop, so pressing Home on a control that just said it
    // cannot act would jump the reader to the top of the page.
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    for (const key of ['ArrowRight', 'ArrowLeft', 'ArrowUp', 'ArrowDown', 'Home', 'End']) {
      let prevented = false;
      c.onScaleKeydown({ key, preventDefault: () => { prevented = true; }, currentTarget: { querySelector: () => null } });
      expect(prevented, key).toBe(true);
    }
    // And leaves every other key alone -- Tab above all, which has to keep
    // moving focus out of the group.
    for (const key of ['Tab', 'Enter', ' ', 'a', 'PageDown']) {
      let prevented = false;
      c.onScaleKeydown({ key, preventDefault: () => { prevented = true; }, currentTarget: { querySelector: () => null } });
      expect(prevented, key).toBe(false);
    }
    expect(c.scale).toBe('relative');
  });

  test('a pair with nothing to scale against refuses by mouse and by keyboard', () => {
    // Every mode returns the same empty style here and the CSS fallback draws
    // the pair, so the control announces itself disabled. A guard on the click
    // alone would leave the group fully working for anyone using arrow keys.
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    expect(c.scaleAvailable).toBe(false);

    c.setScale('fit');
    expect(c.scale).toBe('relative');
    arrowRight(c);
    expect(c.scale).toBe('relative');

    // And it starts working the moment the browser supplies the dimensions.
    c.noteSizeFrom(img(1, 400, 300));
    c.noteSizeFrom(img(2, 600, 800));
    expect(c.scaleAvailable).toBe(true);
    arrowRight(c);
    expect(c.scale).toBe('fit');
  });
});

describe('anchoring', () => {
  test('the top-left anchor zeroes the margin that centres an image', () => {
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    expect(c.leadScale.margin).toBe('');
    c.toggleAnchor();
    expect(c.anchor).toBe('top-left');
    expect(c.leadScale.margin).toBe('0');
    expect(c.trailScale.margin).toBe('0');
  });

  test('one anchor mechanism serves relative and fit alike', () => {
    // The reason Fit sizes the element rather than leaning on object-fit: with
    // the element equal to the painted rectangle, both modes anchor by margin.
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    c.toggleAnchor();
    c.setScale('fit');
    expect(c.leadScale).toEqual({ width: '100%', height: '56.25%', objectFit: '', margin: '0', transform: '', transformOrigin: '' });
  });

  test('stretch leaves no slack, so the anchor refuses there', () => {
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    c.setScale('stretch');
    expect(c.anchorAvailable).toBe(false);
    c.toggleAnchor();
    expect(c.anchor).toBe('center');
  });

  test('an anchor already chosen survives a trip through stretch', () => {
    // Refusing to *change* the anchor is not the same as discarding it: coming
    // back out of stretch has to return the reader to the view they left.
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    c.toggleAnchor();
    c.setScale('stretch');
    expect(c.leadScale.margin).toBe('');
    c.setScale('relative');
    expect(c.anchor).toBe('top-left');
    expect(c.leadScale.margin).toBe('0');
  });

  test('a pair with nothing to scale against cannot be anchored either', () => {
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    expect(c.anchorAvailable).toBe(false);
    c.toggleAnchor();
    expect(c.anchor).toBe('center');
  });
});

describe('a refused scale press does not leave focus on a dead radio', () => {
  /**
   * A refused click, with control over where focus currently sits.
   *
   * `focusedInGroup` is what `document.activeElement` will report: the refused
   * radio (the invariant broken), the checked one (already correct), or null for
   * focus resting somewhere else entirely.
   */
  function clickOn(checked: any, focusedInGroup: any) {
    const group = {
      querySelector: () => checked,
      contains: (node: any) => node === checked || node === focusedInGroup,
    };
    (globalThis as any).document = { activeElement: focusedInGroup };
    return { currentTarget: { closest: () => group } };
  }

  const refusedRadio = { getAttribute: () => 'false' };

  test('activation restores focus to the checked radio, whatever moved it', () => {
    // Guarding mousedown alone leaves the hole open for programmatic focus, for
    // an assistive technology moving focus directly, and for a touch stack that
    // focuses without a compatibility mousedown -- Enter or Space then produces
    // a refused click with focus left on a tabindex="-1", aria-checked="false"
    // radio. Every one of those paths ends in this handler.
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    let focused = 0;
    const checked = { focus: () => { focused += 1; }, getAttribute: () => 'true' };
    c.setScale('fit', clickOn(checked, refusedRadio));
    expect(c.scale).toBe('relative');
    expect(focused).toBe(1);
  });

  test('a refusal does not steal focus from outside the group', () => {
    // The commonest path of all: a pointer press on a dimmed radio, where
    // refuseFocusIfUnavailable already stopped focus from moving. The reader is
    // still in whatever field or control they were using, and restoring
    // unconditionally would yank them into a group they only clicked at.
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    let focused = 0;
    const checked = { focus: () => { focused += 1; }, getAttribute: () => 'true' };
    c.setScale('fit', clickOn(checked, null));
    expect(focused).toBe(0);
  });

  test('a refusal with focus already on the checked radio changes nothing', () => {
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    let focused = 0;
    const checked = { focus: () => { focused += 1; }, getAttribute: () => 'true' };
    c.setScale('fit', clickOn(checked, checked));
    expect(focused).toBe(0);
  });

  test('an accepted press moves the selection and touches no focus', () => {
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    let focused = 0;
    const checked = { focus: () => { focused += 1; }, getAttribute: () => 'true' };
    c.setScale('fit', clickOn(checked, refusedRadio));
    expect(c.scale).toBe('fit');
    expect(focused).toBe(0);
  });

  test('a refusal with no event, and one outside a group, are survivable', () => {
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    expect(() => c.setScale('fit')).not.toThrow();
    expect(() => c.setScale('fit', { currentTarget: { closest: () => null } })).not.toThrow();
    expect(c.scale).toBe('relative');
  });

  test('mousedown is defaulted-prevented only while the control cannot act', () => {
    // Cosmetic rather than the mechanism: mousedown and click are separate tasks
    // with a frame between them, so without this the pointer path paints a focus
    // ring on a dead control before the click handler takes it back.
    const unavailable = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    let prevented = false;
    unavailable.refuseFocusIfUnavailable({ preventDefault: () => { prevented = true; } });
    expect(prevented).toBe(true);

    const available = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    let touched = false;
    available.refuseFocusIfUnavailable({ preventDefault: () => { touched = true; } });
    expect(touched).toBe(false);
  });
});

describe('two sides showing one version', () => {
  test('a load fills every slot that version occupies', () => {
    // `sameVersion` blocks a same-resource self-comparison from rendering this
    // component, but it is `!crossResource && v1 == v2` -- a cross-resource URL
    // naming one version on both sides gets through with two identical URLs.
    // Filling only the first slot leaves the second unknown, and for a format
    // that stores no dimensions the scale controls then stay refused while both
    // images have decoded perfectly well.
    const c = imageCompare({
      leftUrl: '/v1/resource/version/file?versionId=7',
      rightUrl: '/v1/resource/version/file?versionId=7',
      leftLabel: 'A', rightLabel: 'B',
      leftSize: { w: 0, h: 0 }, rightSize: { w: 0, h: 0 },
    });
    expect(c.scaleAvailable).toBe(false);

    c.noteSizeFrom({
      naturalWidth: 400,
      naturalHeight: 300,
      currentSrc: '/v1/resource/version/file?versionId=7',
    });

    expect(c._sizes).toEqual([{ w: 400, h: 300 }, { w: 400, h: 300 }]);
    expect(c.scaleAvailable).toBe(true);
    expect(c.overlayRatio).toBe('400 / 300');
  });
});

/**
 * Package 2: manual alignment.
 *
 * Package 1's three scale policies are whole-pair decisions computed from
 * intrinsic dimensions. None of them can act on a pair that is the same size
 * and simply not in register. This is the offset the reader drives, and almost
 * everything that can go wrong with it is arithmetic: which slot carries the
 * transform, what a percentage is a percentage *of*, and what a flip does to a
 * correction the reader already made.
 */
describe('manual alignment', () => {
  /** The percentages out of a `translate(x%, y%) scale(k)`, as numbers. */
  function transformOf(style: any): { tx: number; ty: number; k: number } | null {
    if (!style.transform) return null;
    const m = /^translate\((-?[\d.]+)%, (-?[\d.]+)%\) scale\(([\d.]+)\)$/.exec(style.transform);
    if (!m) throw new Error(`unparseable transform: ${style.transform}`);
    return { tx: Number(m[1]), ty: Number(m[2]), k: Number(m[3]) };
  }

  function key(k: string, shiftKey = false) {
    let prevented = false;
    return { key: k, shiftKey, preventDefault() { prevented = true; }, get prevented() { return prevented; } };
  }

  test('a nudge moves the trailing version and leaves the leading one alone', () => {
    const c = component({ w: 800, h: 600 }, { w: 400, h: 300 });
    c.toggleAligning();
    c.nudge(12, -4);
    expect(transformOf(c.leadScale)).toBeNull();
    // The trail element is 400x300 *box pixels* wide, and a CSS percentage
    // translate resolves against the element's own border box -- so twelve box
    // pixels is 3% of this element and would be 1.5% of the other one.
    expect(transformOf(c.trailScale)).toEqual({ tx: 3, ty: -4 / 300 * 100, k: 1 });
  });

  test('the offset is in box pixels, so it means the same at any element size', () => {
    // Same twelve-pixel correction, expressed against two differently-sized
    // elements: under Fit the trail grows to fill the box, so the *same* offset
    // has to come out as a smaller percentage of a larger element.
    const c = component({ w: 800, h: 600 }, { w: 400, h: 300 });
    c.toggleAligning();
    c.nudge(12, 0);
    expect(transformOf(c.trailScale)!.tx).toBe(3);
    c.setScale('fit');
    expect(transformOf(c.trailScale)!.tx).toBe(12 / 800 * 100);
  });

  test('a flip inverts the correction rather than dropping it', () => {
    // The reader aligned the trail onto the lead. A flip exchanges the two, so
    // preserving what they did means applying the inverse to the other image:
    // translate(-d/k) scale(1/k), which is T inverse for T(p) = k*p + d.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.nudge(40, 0);
    c.zoomBy(0.25);
    expect(transformOf(c.trailScale)).toEqual({ tx: 5, ty: 0, k: 1.25 });

    c.swapSides();
    // Slot 0 is the trailing side now and carries the inverse; slot 1 is clean.
    expect(transformOf(c.trailScale)).toEqual({ tx: -4, ty: 0, k: 0.8 });
    expect(transformOf(c.leadScale)).toBeNull();
  });

  test('flipping twice returns exactly the correction that was made', () => {
    // `_offset` is the durable state and the trail transform is derived from
    // it, so a flip mutates nothing and there is no drift to accumulate.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.nudge(37, -11);
    c.zoomBy(0.07);
    const before = c.trailScale.transform;
    c.swapSides();
    c.swapSides();
    expect(c.trailScale.transform).toBe(before);
  });

  test('the translation stops with a quarter of the version still in frame', () => {
    // `nudgeSlider` clamps to 1-99 rather than 0-100 for the same reason: a
    // state that shows nothing at all reads as a broken page, not as a choice.
    // A congruent pair fills its own box, so a quarter of it left in frame is
    // three quarters of the box travelled.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.nudge(10000, -10000);
    expect(transformOf(c.trailScale)).toEqual({ tx: 75, ty: -75, k: 1 });
    expect(c.offsetLabel).toBe('+600, -450, 100%');
  });

  test('a small version anchored to the corner cannot be pushed out of the frame', () => {
    // The case a half-the-box bound silently fails. A 100x100 version in a
    // 1000x1000 box is a tenth of the frame; anchored to the corner it starts
    // at the edge, so half a box of travel clears the frame entirely and leaves
    // the reader looking at nothing.
    const c = component({ w: 1000, h: 1000 }, { w: 100, h: 100 });
    c.toggleAligning();
    c.toggleAnchor();
    c.zoomBy(-0.75);
    c.nudge(-10000, 0);

    // 25 rendered box pixels of it, so it may travel 18.75 before only a
    // quarter is left -- not the 500 a fraction of the box would have allowed.
    expect(c.trailOffset.dx).toBeCloseTo(-18.75, 6);
    const { tx } = transformOf(c.trailScale)!;
    // Still overlapping the frame: the rendered rect runs from `dx` to
    // `dx + 25`, and a quarter of it is inside.
    expect(c.trailOffset.dx + 100 * 0.25) .toBeGreaterThan(0);
    expect(tx).toBeCloseTo(-18.75, 6);
  });

  test('a nudge after a flip moves by its own step, never by the whole bound', () => {
    // A flip derives the inverse, and the inverse of an extreme correction is
    // legitimately outside this side's bound: dx 600 at 25% inverts to -2400 at
    // 400%. Clamping that on the first arrow press would jump the image by the
    // whole difference and destroy an alignment the reader had made.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    const forward = c.trailOffset.dx;

    c.swapSides();
    const inverted = c.trailOffset.dx;
    expect(inverted).toBeCloseTo(-forward / 0.25, 6);

    c.handleAlignKey(key('ArrowRight'));
    expect(c.trailOffset.dx).toBeCloseTo(inverted + 1, 6);
    // And it refuses to go further out from there.
    c.handleAlignKey(key('ArrowLeft'));
    expect(c.trailOffset.dx).toBeCloseTo(inverted + 1, 6);
  });

  test('a nudge from outside stops at the range, never through it', () => {
    // "Reduce the distance to the range" is not enough: from 100 against a
    // range of [-10, 10], a delta of -150 reduces the distance and lands at
    // -50, still outside and now on the far side. A large drag after a flip
    // that derived an out-of-range inverse is exactly that gesture.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    c.swapSides();
    const outside = c.trailOffset.dx;
    expect(outside).toBeLessThan(-1000);

    // One enormous inward drag: it must land inside the bound, not beyond it,
    // and not be refused for overshooting.
    c.nudge(100000, 0);
    const box = 800;
    const rendered = box * c.trailOffset.k;
    const rest = (box - rendered) / 2;
    expect(c.trailOffset.dx).toBeCloseTo(box - rendered * 0.25 - rest, 6);
  });

  test('zooming, rescaling and anchoring bring the offset back into the frame', () => {
    // The bound is a function of the version's rendered size and of the anchor,
    // so all three move it while leaving the offset where it was. Without this,
    // dx 600 in an 800 box renders at x = 900..1100 the moment the reader zooms
    // out to 25% -- entirely outside the frame they are looking at.
    const c = component({ w: 800, h: 800 }, { w: 800, h: 800 });
    c.toggleAligning();
    c.nudge(10000, 0);
    expect(c.trailOffset.dx).toBe(600);

    c.zoomBy(-0.75);
    // 200 rendered, resting at 300: it may reach 800 - 50 - 300 = 450.
    expect(c.trailOffset.dx).toBeCloseTo(450, 6);

    c.zoomBy(0.75);
    c.nudge(10000, 0);
    c.toggleAnchor();
    // Anchored, the version rests at the frame's edge, so it may travel
    // 800 - 200 - 0 = 600 -- unchanged here, and the point is that the bound is
    // re-applied rather than that this case moves.
    expect(c.trailOffset.dx).toBeCloseTo(600, 6);

    // A scale changed by arrow key assigns through the generic radiogroup
    // handler and never touches `setScale`, so this is a second path to the
    // same hazard and it had no rebound at all.
    const e = component({ w: 800, h: 600 }, { w: 400, h: 300 });
    e.toggleAligning();
    e.setScale('fit');
    e.nudge(10000, 0);
    // Fit grows the 400x300 trail to fill the 800x600 box, so it may travel
    // 800 - 200 = 600.
    expect(e.trailOffset.dx).toBeCloseTo(600, 6);

    e.$nextTick = (fn: () => void) => fn();
    e.onScaleKeydown({ key: 'ArrowLeft', preventDefault() {}, currentTarget: { querySelector: () => null } });
    expect(e.scale).toBe('relative');
    // At its true size the trail is 400 wide, resting at 200: it may reach
    // 800 - 100 - 200 = 500, and the offset comes back to it.
    expect(e.trailOffset.dx).toBeCloseTo(500, 6);

    // The anchor bites in the other direction: anchored, a small version rests
    // at the frame's edge and may travel almost the whole frame; centred, it
    // rests in the middle and the same offset carries it clean out. The
    // rebound only ever pulls in, so this is the order that shows it.
    const d = component({ w: 1000, h: 1000 }, { w: 100, h: 100 });
    d.toggleAligning();
    d.toggleAnchor();
    d.nudge(10000, 0);
    // Resting at 0, 100 wide: it stops with 25 of itself still inside.
    expect(d.trailOffset.dx).toBeCloseTo(975, 6);

    d.toggleAnchor();
    // Centred it rests at 450, so 975 would put it at 1425..1525 -- nowhere
    // near the frame. It comes back to 1000 - 25 - 450.
    expect(d.trailOffset.dx).toBeCloseTo(525, 6);
  });

  test('re-selecting the scale already selected leaves a flipped correction alone', () => {
    // A rebound corrects what its operation broke, never what was already true.
    // After a flip the derived inverse is legitimately outside this side's
    // bound, so a rebound triggered by an operation that changed nothing would
    // clamp it and silently rewrite the reader's original number.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    const chosen = c._offset.dx;

    c.swapSides();
    expect(c.offsetWithinBound).toBe(false);
    // Relative is already selected and Fit-then-back is not: this is the
    // activation that changes nothing.
    c.setScale('relative');
    expect(c._offset.dx).toBeCloseTo(chosen, 6);
    c.$nextTick = (fn: () => void) => fn();
    c.onScaleKeydown({ key: 'Home', preventDefault() {}, currentTarget: { querySelector: () => null } });
    expect(c.scale).toBe('relative');
    expect(c._offset.dx).toBeCloseTo(chosen, 6);

    // And flipping back returns exactly what was chosen.
    c.swapSides();
    expect(c.trailOffset.dx).toBeCloseTo(chosen, 6);
  });

  test('an operation that does move the bound still pulls a flipped offset in', () => {
    // The other half of the same rule, and the reason it is not "never rebound
    // while flipped": a reader looking at the flipped view who zooms is owed
    // the same guarantee as one looking at the original.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    c.swapSides();
    const before = c.trailOffset.dx;

    c.toggleAnchor();
    // Anchored, the flipped rendered rect starts at the frame's edge, so this
    // offset is legal there and nothing moves.
    expect(c.trailOffset.dx).toBeCloseTo(before, 6);
    c.toggleAnchor();
    // Centred it is not, so it comes back in -- which costs this pathological
    // round trip its exactness, and is the price of the guarantee holding in
    // the view the reader is actually looking at.
    expect(c.trailOffset.dx).toBeCloseTo(-1200, 6);
  });

  test('a load-time rebound waits for both versions to report', () => {
    // The box is `max` of the two, so while one side is a real measurement and
    // the other still the stored placeholder it describes a pair that does not
    // exist. Clamping against that transient makes the same two files land on a
    // different offset depending on which decoded first.
    const order = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 400, h: 300 }, { w: 400, h: 300 });
      c.toggleAligning();
      c.nudge(10000, 0);
      c.noteSizeFrom(img(first, 800, 600));
      c.noteSizeFrom(img(second, 800, 600));
      return c.trailOffset.dx;
    };
    expect(order(1, 2)).toBeCloseTo(order(2, 1), 6);

    // A version whose load merely confirms the stored size still counts as
    // measured, or the rebound would wait for a change that never comes.
    const c = component({ w: 400, h: 300 }, { w: 400, h: 300 });
    c.toggleAligning();
    c.nudge(10000, 0);
    c.noteSizeFrom(img(2, 400, 300));
    c.noteSizeFrom(img(1, 800, 600));
    // Slot 0 grew, so the frame did while the trail did not: the offset is
    // converted to 600 and the bound is now 800 - 100 - 200 = 500.
    expect(c.trailOffset.dx).toBeCloseTo(500, 6);

    // The reversed order: the *lead* grows first and the trail then confirms
    // its stored (smaller) size. The confirming report flips `_measured` to
    // both-true without changing any size, so the early return skips the
    // rebound unless a report that only confirms still owes it -- otherwise
    // the converted offset (600) stays against a bound (500) the smaller trail
    // leaves, and the version sits entirely outside the frame.
    const d = component({ w: 400, h: 300 }, { w: 400, h: 300 });
    d.toggleAligning();
    d.nudge(10000, 0);
    d.noteSizeFrom(img(1, 800, 600));
    d.noteSizeFrom(img(2, 400, 300));
    expect(d.trailOffset.dx).toBeCloseTo(500, 6);
  });

  test('a report that only confirms does not arm a later rebound', () => {
    // The rebound debt exists for a size *change* moving an offset that was
    // inside. A report that confirms a stored size changes nothing, so it
    // must not arm it -- otherwise a flip made after a confirm report would
    // have its legitimately-outside inverse clamped by the next confirm,
    // rewriting the reader's original correction.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    // Slot 0 confirms its stored size before anything has happened.
    c.noteSizeFrom(img(1, 800, 600));
    c.toggleAligning();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    c.swapSides();
    // The flip-derived inverse is legitimately outside this side's bound.
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    // Slot 1 confirms too: it completes the pair but changes no size, so it
    // must not clamp the flipped offset.
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
  });

  test('a report that fills both slots still reaches the rebound', () => {
    // One image showing on both sides (two identical URLs) fills both slots in
    // a single report, so the "both measured" moment is also the size change
    // -- and the rebound has to happen then, or a converted offset is left
    // outside the bound the corrected box implies.
    const c = imageCompare({
      leftUrl: '/v1/resource/version/file?versionId=9',
      rightUrl: '/v1/resource/version/file?versionId=9',
      leftLabel: 'A', rightLabel: 'B',
      leftSize: { w: 400, h: 800 }, rightSize: { w: 400, h: 800 },
    });
    c.toggleAligning();
    c.nudge(0, 10000);
    // Stored 400x800: dy is bounded to 600 (800 - a quarter of 800).
    expect(c.trailOffset.dy).toBeCloseTo(600, 6);
    // The browser reports the real dimensions: 800x400, width doubled. The
    // offset converts to 1200 and the new bound allows only 300 -- without a
    // rebound here, the version would sit entirely outside the frame.
    c.noteSizeFrom({ naturalWidth: 800, naturalHeight: 400, currentSrc: '/v1/resource/version/file?versionId=9' });
    expect(c.trailOffset.dy).toBeCloseTo(300, 6);
  });

  test('a deferred rebound is paid in the orientation that incurred it', () => {
    // The debt is armed by a size change in whatever orientation the reader is
    // in. Paying it later in a *different* orientation -- a flip landed
    // between the change and the confirming load -- clamps the inverse and
    // rewrites the canonical offset differently than the same measurements
    // without the flip would have: the same two files land on a different
    // correction depending on when the reader happened to flip, which is the
    // order-dependence the load-time rebound exists to remove.
    const before = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    before.toggleAligning();
    before.zoomBy(-0.75);
    before.nudge(10000, 0);
    before.noteSizeFrom(img(1, 1600, 1200));
    before.noteSizeFrom(img(2, 800, 600));
    // Both measured before the flip: the canonical offset rebounds to 850
    // (1600 box, 200 rendered trail: 1600 - 50 - 700), and the flip then
    // derives the inverse without touching it.
    expect(before.trailOffset.dx).toBeCloseTo(850, 6);
    before.swapSides();
    expect(before.trailOffset.dx).toBeCloseTo(-3400, 6);

    const flipped = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    flipped.toggleAligning();
    flipped.zoomBy(-0.75);
    flipped.nudge(10000, 0);
    // Slot 0 grows to 1600x1200: the conversion doubles dx to 900 and arms
    // the debt, both while unflipped.
    flipped.noteSizeFrom(img(1, 1600, 1200));
    expect(flipped.trailOffset.dx).toBeCloseTo(900, 6);
    // The reader flips to check, and the confirming load lands while flipped.
    flipped.swapSides();
    flipped.noteSizeFrom(img(2, 800, 600));
    // The pay must land on the same canonical correction the flip-free order
    // did, not clamp the flipped inverse to -2400 and rewrite it to 600.
    expect(flipped._offset.dx).toBeCloseTo(850, 6);
    expect(flipped.trailOffset.dx).toBeCloseTo(-3400, 6);
  });

  test('a debt armed at identity never rewrites an alignment made later', () => {
    // The debt exists for a size change *moving an offset that was inside*.
    // With the offset at identity there is nothing to move, so a size change
    // must not arm it -- otherwise a later confirming report fires the stale
    // debt and rewrites alignment the reader made after the change, which is
    // a rebound correcting what no operation of its own broke.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    // Slot 0 reports 800x1200 while the offset is identity: nothing moved.
    c.noteSizeFrom(img(1, 800, 1200));
    // Which is the rule, stated where it is decided: at identity no axis can
    // cross, because zero is inside every range `translateRange` produces.
    expect(c._sizeOwedX).toBe(false);
    expect(c._sizeOwedY).toBe(false);
    c.toggleAligning();
    c.swapSides();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    // The flipped view's bound for the 200px-rendered 800x1200 slot is +450.
    expect(c.trailOffset.dx).toBeCloseTo(450, 6);
    // Slot 1 confirms its stored size, and the window's request resolves --
    // canonically, as every deferred resolution does, so a correction that
    // landed on the flipped bound comes back to the canonical one. That is
    // the exactness the flipped window trades for a result that does not
    // depend on which version decoded first; it is not the stale debt acting,
    // which would need a claim, and there is none.
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(300, 6);
    expect(c._sizeOwedX).toBe(false);
  });

  test('a control used between the two reports does not make the result order-dependent', () => {
    // Pressing Anchor (or zooming, or changing scale) while one slot has
    // reported and the other has not clamps against a transient geometry --
    // the trail is whichever slot decoded first -- and the later width
    // conversion scales the already-clamped value, so the same two files land
    // on a different canonical offset depending on decode order. The clamp is
    // deferred to the both-measured moment.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.zoomBy(-0.75);
      c.nudge(-430, 0);
      c.noteSizeFrom(img(first, 600, 800));
      c.toggleAnchor();
      c.noteSizeFrom(img(second, 600, 800));
      return c.trailOffset.dx;
    };
    // Both orders land on the final bound: 600x800 box, 150px-rendered trail,
    // anchored top-left, so min is -(0 + 150 - 37.5).
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(-112.5, 6);
  });

  test('a keyboard scale change during the load window defers like the mouse one', () => {
    // The keyboard path assigns `this.scale` straight through the generic
    // radiogroup handler, so it needs the same deferral the click path has:
    // rebounding against the transient geometry lands a scale changed by
    // arrow key between the two reports on a different offset than the same
    // change by click.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 100, h: 100 }, { w: 100, h: 100 });
      c.toggleAligning();
      c.toggleAnchor();
      c.zoomBy(1);
      c.nudge(0, 50);
      c.noteSizeFrom(img(first, first === 1 ? 400 : 200, first === 1 ? 300 : 800));
      c.$nextTick = (fn: () => void) => fn();
      c.onScaleKeydown({ key: 'ArrowRight', preventDefault() {}, currentTarget: { querySelector: () => null } });
      c.noteSizeFrom(img(second, second === 1 ? 400 : 200, second === 1 ? 300 : 800));
      return c.trailOffset.dy;
    };
    // 400x300 box and a 200x800 trail at 200% anchored top-left: the final
    // y-range is [-1200, 400], so the converted 200 stays.
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(200, 6);
  });

  test('a no-op scale activation arms no deferred debt', () => {
    // Pressing the already-selected policy changes no geometry, so it must
    // not arm the deferred rebound, exactly as it does not reapply the bound
    // outside the load window: nothing was broken, so nothing is owed.
    const c = component({ w: 100, h: 100 }, { w: 100, h: 100 });
    c.toggleAligning();
    c.zoomBy(3);
    c.nudge(-10000, 0);
    // At k=4 the 100x100 trail renders 400 wide: the bound is -150.
    expect(c.trailOffset.dx).toBeCloseTo(-150, 6);
    // Slot 0 confirms its stored size: the pair is half-measured now.
    c.noteSizeFrom(img(1, 100, 100));
    // Clicking the already-selected Relative changes nothing and must not
    // claim anything: it moves no bound, so no axis crosses one.
    c.setScale('relative');
    expect(c._sizeOwedX).toBe(false);
    expect(c._sizeOwedY).toBe(false);
    c.swapSides();
    c.nudge(-10000, 0);
    // The flipped bound for the 25px-rendered slot is -56.25.
    expect(c.trailOffset.dx).toBeCloseTo(-56.25, 6);
    // Slot 1 confirms and the window's request resolves against the canonical
    // bound, as every deferred resolution does. Nothing here is the no-op's
    // doing -- it claimed no axis, and a claim is what would license the
    // rebound to repair one.
    c.noteSizeFrom(img(2, 100, 100));
    expect(c.trailOffset.dx).toBeCloseTo(-37.5, 6);
    expect(c._sizeOwedX).toBe(false);
  });

  test('a zoom clamped at the boundary arms no deferred debt', () => {
    // Pressing + at 400% changes no geometry -- the clamp holds k -- so
    // nothing is owed, exactly as a no-op scale activation arms nothing.
    const c = component({ w: 100, h: 100 }, { w: 100, h: 100 });
    c.toggleAligning();
    c.zoomBy(3);
    c.nudge(-10000, 0);
    c.noteSizeFrom(img(1, 100, 100));
    c.zoomBy(0.01);
    expect(c._sizeReboundDue).toBe(false);
    // A real zoom changes geometry and is owed in the load window.
    c.zoomBy(-0.5);
    expect(c._sizeReboundDue).toBe(true);
  });

  test('a nudge during the load window defers its clamp, so it cannot bind the final result', () => {
    // Nudging while exactly one version is measured obeys a transient bound --
    // the trail is whichever slot decoded first -- but only on display: what
    // the clamp swallowed rides onto the outstanding request, and the deferred
    // rebound resolves that request against the final bound. Clamping the
    // increment alone would let the later width conversion scale the
    // already-clamped value, so the same two files and input landed on a
    // different canonical offset per decode order.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.noteSizeFrom(img(first, 600, 800));
      c.nudge(-10000, 0);
      // Display continuity holds at every instant: the image sits at the
      // transient bound -- slot 0 first puts an 800-wide trail in an 800 box,
      // slot 1 first a 600-wide one -- each keeping its own quarter visible.
      const mid = c.trailOffset.dx;
      c.noteSizeFrom(img(second, 600, 800));
      return { mid, final: c.trailOffset.dx };
    };
    const order0First = run(1, 2);
    const order1First = run(2, 1);
    expect(order0First.mid).toBeCloseTo(-600, 6);
    expect(order1First.mid).toBeCloseTo(-550, 6);
    // Both orders resolve to the final bound: 600x800 box, 600px-rendered
    // trail, centred, so min is -(0 + 600 - 150).
    expect(order0First.final).toBeCloseTo(order1First.final, 6);
    expect(order0First.final).toBeCloseTo(-450, 6);
  });

  test('a reset retires an unmet in-window remainder with the correction', () => {
    // What a half-measured nudge's clamp deferred lives only while the
    // correction it belongs to does. A stale remainder surviving Reset would
    // drag every later alignment made before the pair completes over to the
    // bound. The completing report here confirms its stored size, changing no
    // geometry of its own, so the modest nudge must land as asked.
    const c = component({ w: 600, h: 800 }, { w: 600, h: 800 });
    c.toggleAligning();
    c.noteSizeFrom(img(1, 600, 800));
    c.nudge(-10000, 0);
    expect(c.trailOffset.dx).toBeCloseTo(-450, 6);
    c.resetAlignment();
    c.nudge(30, 0);
    c.noteSizeFrom(img(2, 600, 800));
    expect(c.trailOffset.dx).toBeCloseTo(30, 6);
  });

  test('an outstanding remainder converts with the box even when the display walked back to identity', () => {
    // Display and request diverge exactly here: the reader ran the image out,
    // walked it back home, and the completing report resized the box. At
    // identity the display needs no conversion -- but the request is still a
    // displacement measured in the old box's pixels, and skipping its
    // conversion binds the deferred resolution to decode order.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.noteSizeFrom(img(first, 600, 800));
      c.nudge(-1100, 0);
      c.nudge(600, 0);
      // Whatever the walk-back left showing, the ask that remains does not
      // depend on which slot happened to be standing under it.
      c.noteSizeFrom(img(second, 600, 800));
      return c.trailOffset.dx;
    };
    // -500 outstanding scales by the 0.75 width ratio into the smaller pair:
    // interior of the final bound, so what arrives IS the scaled request.
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(-375, 6);
  });

  test('a free in-window nudge lands on the same offset in either decode order', () => {
    // The transient box is `max` of one real measurement and one stored
    // placeholder, so it differs per decode order when the corrected widths
    // differ -- and a step taken against it inherits that difference unless
    // the step means something order-independent. It means stored-box pixels:
    // the frame both orders share before any image has spoken. Resolving the
    // request then converts it with the ratio of the final box to the stored
    // one, so the same files and the same key land on the same canonical
    // offset whichever slot decoded first.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      // Actual slot 0 is 600x800, actual slot 1 is 1200x400: the stored box
      // is 800 wide, the final one 1200.
      c.noteSizeFrom(img(first, first === 1 ? 600 : 1200, first === 1 ? 800 : 400));
      c.nudge(10, 0);
      c.noteSizeFrom(img(second, second === 1 ? 600 : 1200, second === 1 ? 800 : 400));
      return c._offset.dx;
    };
    // Ten stored-box pixels against a box that grows to 1200/800 of that.
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(15, 6);
  });

  test('a nudge on one axis never resolves the other axis out of its flip-derived position', () => {
    // The request is created whole from the displayed offset, both axes --
    // but a key that moved only y must not hand the resolver a licence to
    // clamp x. The flip-derived inverse sits legitimately outside the bound,
    // and `boundedNudge` widens the display's range around it for exactly
    // that reason; resolving an untouched axis has to preserve it too, or
    // one ArrowDown rewrites the canonical offset by 150 (450 -> 300).
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.zoomBy(-0.75);
    c.nudge(450, 0);
    c.swapSides();
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    c.noteSizeFrom(img(1, 800, 600));
    c.nudge(0, -1);
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    // Canonical, slot 1 relative to slot 0: still the correction made.
    expect(c._offset.dx).toBeCloseTo(450, 6);
    expect(c._offset.dy).toBeCloseTo(0.25, 6);
  });

  test('a drag during the load window covers the same screen distance in either decode order', () => {
    // A keyboard step is an abstract unit, and the load window redefines it as
    // stored-box pixels so both decode orders agree. A drag increment is not
    // abstract: it is a physical distance the reader's hand covered on the
    // screen, already converted into the pixels of whichever transient box was
    // standing. Recording *that* raw changes what it measures -- the same 60px
    // drag becomes 60 screen pixels one way and 90 the other, and the image
    // visibly jumps when the second version decodes. So a drag increment is
    // recorded converted, and the resolution's conversion telescopes it back
    // to the physical distance.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const dom = fakeDom();
      try {
        // Stored 800x600 both; actual slot 0 is 600x800 and slot 1 is 1200x400,
        // so the transient box is 800 wide one way and 1200 the other.
        const size = (v: 1 | 2): [number, number] => (v === 1 ? [600, 800] : [1200, 400]);
        const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
        c.toggleAligning();
        c.noteSizeFrom(img(first, ...size(first)));
        // The box is rendered 600 CSS pixels wide, and the hand moves 60 of them.
        c.startAlignDrag(press(100, 100, { closest: () => null }, frame(600, 600)));
        dom.fire('mousemove', { clientX: 160, clientY: 100 });
        dom.fire('mouseup', {});
        c.noteSizeFrom(img(second, ...size(second)));
        return c._offset.dx;
      } finally {
        dom.restore();
      }
    };
    // The final box is 1200 wide and still rendered 600 across, so 60 screen
    // pixels are 120 box pixels -- whichever version decoded first.
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(120, 6);
  });

  test('a request pays the rebound the completing report incurs, rather than preserving it', () => {
    // An axis the display legitimately occupies outside its bound -- a flip's
    // inverse -- is preserved through the resolution. An axis the *completing
    // report* pushed outside is the opposite: that report shrank the trailing
    // version, and the debt it armed is exactly what the deferred rebound
    // exists to pay. Widening around wherever the display happens to sit at
    // resolution cannot tell the two apart, so it preserves the decode-order
    // artifact instead: one order leaves the image entirely out of frame, the
    // other leaves less than the quarter the bound promises.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 800, h: 200 }, { w: 1200, h: 100 });
      c.toggleAligning();
      const size = (v: 1 | 2): [number, number] => (v === 1 ? [1200, 800] : [100, 400]);
      c.noteSizeFrom(img(first, ...size(first)));
      // Far past either transient bound: the display stops at it, and every
      // press beyond rides onto the request whole.
      for (let i = 0; i < 100; i += 1) c.nudge(-10, 0);
      c.noteSizeFrom(img(second, ...size(second)));
      return c._offset.dx;
    };
    // 1200-wide box, a 100-wide trail centred in it: rest 550, a quarter of
    // 100 kept, so the boundary is -(550 + 100 - 25).
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(-625, 6);
  });

  test('a request converts with the box even when the pair had no stored size', () => {
    // With no stored dimensions there is no stored box to denominate the
    // request in, and no second decode order to diverge from either -- only
    // the version that reported first can open a nudgeable window. What is
    // still owed is physical fidelity: the step was taken against the box
    // standing at the request's creation, so the corrected box has to carry it
    // the same distance the displayed offset travels. Falling back to a
    // conversion factor of 1 makes the correction visibly shrink the moment
    // the real width arrives.
    const c = component({ w: 0, h: 0 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.noteSizeFrom(img(1, 400, 300));
    c.nudge(10, 0);
    expect(c.trailOffset.dx).toBeCloseTo(10, 6);
    // The box grows 800 -> 1200, so ten of its pixels become fifteen.
    c.noteSizeFrom(img(2, 1200, 600));
    expect(c.trailOffset.dx).toBeCloseTo(15, 6);
  });

  test('an outside position a report created is not laundered into a legitimate one by a later nudge', () => {
    // The other half of the same rule, and the reason the provenance is
    // snapshotted at the nudge rather than read off the display: here the
    // report lands *first*, shrinking the trailing version until the existing
    // correction hangs outside the new bound, and the nudge that follows sees
    // exactly the picture a flip-derived inverse paints. What separates them
    // is that a bound-mover armed a debt, so the outside-ness is owed and the
    // nudge records no anchor -- the deferred rebound pays it instead of
    // preserving it.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.nudge(450, 0);
    // Slot 1 is the trail, and it turns out to be 100 wide rather than 800:
    // the bound closes to -425..425 with the correction sitting at 450.
    c.noteSizeFrom(img(2, 100, 600));
    expect(c.trailOffset.dx).toBeCloseTo(450, 6);
    c.nudge(10, 0);
    c.noteSizeFrom(img(1, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(425, 6);
  });

  test('a swallowed outward nudge from a flip-derived outside stops there rather than snapping in', () => {
    // The anchor's preserving half. The display sits at the inverse of an
    // extreme correction, legitimately outside this side's own bound, and the
    // reader pushes it further out: the display refuses to travel and the
    // whole increment rides onto the request. Resolving that request against
    // the plain bound would answer a refused outward gesture by moving the
    // image 600 pixels inward -- so the range is widened to the anchor, and
    // the request stops exactly where the display already was.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.zoomBy(-0.75);
    c.nudge(450, 0);
    c.swapSides();
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    c.noteSizeFrom(img(1, 800, 600));
    c.nudge(-100, 0);
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    expect(c._offset.dx).toBeCloseTo(450, 6);
  });

  test('a bound moved while nothing is displaced arms no debt, so a later flip keeps its inverse', () => {
    // The same rule a size report already obeys -- arm only what the operation
    // actually broke -- applied to the control operations the load window
    // defers. A zero translation sits inside every bound `translateRange` can
    // produce, so moving the bound underneath it breaks nothing; arming there
    // records that the display's position is the operation's doing, and the
    // next nudge then reads a legitimately-outside axis as owed and lets the
    // resolution snap it. Anchor at identity, correct to +600, flip, and one
    // ArrowDown rewrote the canonical correction to +300.
    const c = component({ w: 400, h: 300 }, { w: 800, h: 600 });
    c.noteSizeFrom(img(1, 400, 300));
    c.toggleAnchor();
    c.toggleAligning();
    c.nudge(600, 0);
    c.swapSides();
    expect(c.trailOffset.dx).toBeCloseTo(-600, 6);
    c.nudge(0, 1);
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(-600, 6);
    expect(c._offset.dx).toBeCloseTo(600, 6);
    expect(c._offset.dy).toBeCloseTo(-1, 6);
  });

  test('a size report that lands before any correction arms no debt either', () => {
    // The same predicate, at the other place that arms it. The first report
    // corrects the box while the reader has not moved anything yet, so it
    // broke nothing -- but recording it as a bound-mover's doing outlived the
    // whole window: zoom, correct to the bound, flip, and the inverse the flip
    // derives sits legitimately outside, where the next press must preserve it
    // and instead found the display's position already claimed by a report
    // that had displaced nothing. Canonical 450 came back as 300.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    // Slot 0 turns out to be taller than stored: the box moves, the offset
    // does not, because there is no offset.
    c.noteSizeFrom(img(1, 800, 900));
    c.zoomBy(-0.75);
    c.nudge(450, 0);
    c.swapSides();
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    c.nudge(0, -1);
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    expect(c._offset.dx).toBeCloseTo(450, 6);
  });

  test('an operation repairs the axis it broke even while the other is legitimately outside', () => {
    // Repair is per axis for the same reason arming is. Here the flip leaves x
    // legitimately outside and y comfortably inside, and the anchor change
    // trades them: x becomes legal and y is thrown 1000 pixels down a frame
    // 800 tall, which is the state the whole bound exists to prevent -- an
    // image with nothing of it on screen. A whole-offset "was it inside"
    // question answers no, because of x, and the repair never runs; asked per
    // axis it repairs y and leaves x exactly where the reader's flip put it.
    const c = component({ w: 800, h: 800 }, { w: 800, h: 800 });
    c.toggleAligning();
    c.zoomBy(-0.75);
    c.nudge(450, -250);
    c.swapSides();
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    expect(c.trailOffset.dy).toBeCloseTo(1000, 6);
    c.toggleAnchor();
    // Anchored top-left the range is -2400..0 on both axes: x was already
    // outside and stays, y was inside and comes back to the near edge.
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    expect(c.trailOffset.dy).toBeCloseTo(0, 6);
  });

  test('a nudge joins an outstanding debt rather than moving it to its own orientation', () => {
    // The debt is paid in the orientation that incurred it, and a flip landing
    // between the size change and the confirming load must not change the
    // canonical result. A nudge arriving in that gap is a second arming of a
    // debt that already exists, so it has no orientation of its own to
    // record: rewriting the orientation pays the report's clamp against the
    // other version's bound, and one ArrowDown then rewrote the x correction
    // from 850 to 600 -- but only when a flip happened to intervene, which is
    // the flip-invariance the paid-in-its-own-orientation rule exists for.
    const run = (flip: boolean) => {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.zoomBy(-0.75);
      c.nudge(450, 0);
      // Slot 0 is twice its stored size: the box doubles, the correction
      // converts to 900, and the bound it now sits outside is 850.
      c.noteSizeFrom(img(1, 1600, 1200));
      if (flip) c.swapSides();
      c.nudge(0, 1);
      c.noteSizeFrom(img(2, 800, 600));
      return c._offset.dx;
    };
    expect(run(true)).toBeCloseTo(run(false), 6);
    expect(run(true)).toBeCloseTo(850, 6);
  });

  test('a flip between the two reports cannot change the canonical correction', () => {
    // The reports are the browser's, the flips are the reader's, and a decode
    // order must not decide what a flip means. The bound question a report
    // asks is therefore asked of the canonical correction and the canonical
    // trail -- the frame a flip does not touch -- rather than of whatever is
    // on screen when the bytes happen to arrive. Asked of the display, the
    // width conversion here lands while flipped in one order, where a 600-wide
    // version in a 900-tall box has room for it, and while unflipped in the
    // other, where the 400-tall trail does not: the same files and the same
    // presses finish 125 box pixels apart.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.nudge(0, -450);
      // Actual slot 0 is 600x900 and slot 1 is 1200x400: the box grows from
      // 800x600 to 1200x900 and the correction converts to -675.
      const size = (v: 1 | 2): [number, number] => (v === 1 ? [600, 900] : [1200, 400]);
      c.noteSizeFrom(img(first, ...size(first)));
      c.swapSides();
      c.noteSizeFrom(img(second, ...size(second)));
      c.swapSides();
      return c._offset.dy;
    };
    // A 400-tall trail centred in a 900-tall box: rest 250, a quarter of 400
    // kept, so the boundary is -(250 + 400 - 100).
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(-550, 6);
  });

  test('a control operation during the load window claims what it broke in the frame the pay uses', () => {
    // A repair the reader watches happen belongs in the view they are looking
    // at. A repair *deferred* to the both-measured moment does not: by then
    // the view may be the other one, and the question the operation asked has
    // to survive until it is answered. Asked of the display, an axis a flip
    // already left outside hides the crossing the operation makes canonically
    // -- here Anchor throws the correction past a canonical bound it was
    // sitting exactly on -- and the completing report then finds it already
    // outside and claims nothing either. Nothing repairs it, and the two
    // decode orders finish 150 box pixels apart with one of them showing
    // nothing of the image at all.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.zoomBy(-0.75);
      c.nudge(-450, 0);
      c.swapSides();
      c.noteSizeFrom(img(first, 400, 400));
      c.toggleAnchor();
      c.noteSizeFrom(img(second, 400, 400));
      return c._offset.dx;
    };
    // A 100-wide trail anchored top-left in a 400 box: the near edge is
    // -(100 - 25).
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(-75, 6);
  });

  test('a report that claims an axis retires the anchor a nudge recorded before it', () => {
    // An anchor means the axis was legitimately outside at nudge time *and*
    // nothing has claimed it since. A report arriving afterwards and pushing
    // that axis out of a bound it was inside is exactly such a claim: widening
    // the resolution around the older anchor launders the outside-ness the
    // report created, which is the debt the rebound exists to pay.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.zoomBy(-0.75);
      c.nudge(450, 0);
      c.swapSides();
      const size = (v: 1 | 2): [number, number] => (v === 1 ? [600, 600] : [400, 600]);
      c.noteSizeFrom(img(first, ...size(first)));
      // Refused outward: the display does not move and the whole increment
      // rides onto the request, with the flip-derived outside as its anchor.
      c.nudge(-1, 0);
      c.noteSizeFrom(img(second, ...size(second)));
      c.swapSides();
      return c._offset.dx;
    };
    // A 100-wide trail centred in a 600 box: rest 250, a quarter kept, so the
    // boundary is 600 - 25 - 250.
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(325, 6);
  });

  test('a report that brings an axis back inside does not hand it to the next one', () => {
    // A claim is the reports' doing measured against the position the reader
    // chose. Measured instead against whatever the previous report left, a
    // report that happens to widen a bound past a legitimately-outside axis
    // makes that axis inside again for a moment, and the completing report
    // then finds it crossing outward and clamps a correction nothing broke.
    // Which report widens and which crosses is the decode order, so the same
    // two files and the same presses keep the reader's alignment in one order
    // and snap it to zero in the other.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      // Flipped and zoomed out, ten box pixels left: well inside the bound the
      // reader is looking at, and outside the canonical one, which is what a
      // flip-derived inverse is.
      c.swapSides();
      c.zoomBy(-0.75);
      c.nudge(-10, 0);
      c.toggleAnchor();
      c.noteSizeFrom(img(first, 400, 400));
      c.noteSizeFrom(img(second, 400, 400));
      return c._offset.dx;
    };
    // The box halves, so the correction halves with it and the reader still
    // sees the ten pixels they asked for, as five.
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(20, 6);
  });

  test('the canonical correction never depends on which version decoded first', () => {
    // Every load-window defect this component has had is one shape: a reader's
    // script interleaved with the two decodes, converging in one order and not
    // the other. Found one repro at a time, that shape gets sampled, never
    // covered -- a steady find rate across rounds is the signature. So the
    // interleavings are enumerated instead: each script is replayed with the
    // two reports inserted at every pair of positions in it, under both decode
    // orders, and the two canonical offsets must agree.
    //
    // Convergence only. Containment is deliberately not asserted: a
    // flip-derived inverse is legitimately outside this side's bound, and the
    // whole design is built to preserve it rather than clamp it.
    const step = (name: string, run: (c: any) => void) => ({ name, run });
    const SCRIPTS = [
      [step('nudge -450', (c) => c.nudge(-450, 0)), step('flip', (c) => c.swapSides()),
        step('anchor', (c) => c.toggleAnchor())],
      [step('zoom 25%', (c) => c.zoomBy(-0.75)), step('nudge +450', (c) => c.nudge(450, 0)),
        step('flip', (c) => c.swapSides()), step('nudge y', (c) => c.nudge(0, 1))],
      [step('nudge -450y', (c) => c.nudge(0, -450)), step('flip', (c) => c.swapSides()),
        step('nudge', (c) => c.nudge(37, -11)), step('flip', (c) => c.swapSides())],
      [step('zoom 25%', (c) => c.zoomBy(-0.75)), step('nudge -1000', (c) => c.nudge(-1000, 0)),
        step('anchor', (c) => c.toggleAnchor()), step('flip', (c) => c.swapSides()),
        step('nudge y', (c) => c.nudge(0, 50))],
      [step('nudge', (c) => c.nudge(120, -40)), step('fit', (c) => c.setScale('fit')),
        step('flip', (c) => c.swapSides()), step('zoom +', (c) => c.zoomBy(0.5))],
      [step('anchor', (c) => c.toggleAnchor()), step('nudge', (c) => c.nudge(-77, 210)),
        step('flip', (c) => c.swapSides()), step('stretch', (c) => c.setScale('stretch')),
        step('nudge', (c) => c.nudge(5, 5))],
      [step('zoom +', (c) => c.zoomBy(1)), step('flip', (c) => c.swapSides()),
        step('nudge', (c) => c.nudge(-10000, 0)), step('flip', (c) => c.swapSides()),
        step('nudge', (c) => c.nudge(0, 10000))],
      [step('nudge', (c) => c.nudge(450, 450)), step('flip', (c) => c.swapSides()),
        step('anchor', (c) => c.toggleAnchor()), step('zoom 25%', (c) => c.zoomBy(-0.75)),
        step('nudge', (c) => c.nudge(-3, 7))],
      [step('flip', (c) => c.swapSides()), step('zoom 25%', (c) => c.zoomBy(-0.75)),
        step('nudge', (c) => c.nudge(-10, 0)), step('anchor', (c) => c.toggleAnchor())],
    ];
    // Pairs whose corrected widths differ from the stored ones and from each
    // other, so the transient box differs per decode order -- the condition
    // every one of these defects needed.
    const PAIRS = [
      { stored: [{ w: 800, h: 600 }, { w: 800, h: 600 }], actual: [[400, 400], [400, 400]] },
      { stored: [{ w: 800, h: 600 }, { w: 800, h: 600 }], actual: [[600, 900], [1200, 400]] },
      { stored: [{ w: 800, h: 200 }, { w: 1200, h: 100 }], actual: [[1200, 800], [100, 400]] },
      { stored: [{ w: 800, h: 600 }, { w: 800, h: 600 }], actual: [[600, 600], [400, 600]] },
      { stored: [{ w: 800, h: 600 }, { w: 800, h: 600 }], actual: [[600, 800], [1200, 400]] },
      { stored: [{ w: 4000, h: 397 }, { w: 100, h: 3000 }], actual: [[397, 4000], [3000, 100]] },
      { stored: [{ w: 1000, h: 1000 }, { w: 1000, h: 1000 }], actual: [[100, 4000], [4000, 100]] },
      { stored: [{ w: 300, h: 900 }, { w: 900, h: 300 }], actual: [[900, 300], [300, 900]] },
    ];

    const play = (pair: typeof PAIRS[number], script: typeof SCRIPTS[number],
      at: number, then: number, first: 1 | 2) => {
      const c = component(pair.stored[0], pair.stored[1]);
      c.toggleAligning();
      const second: 1 | 2 = first === 1 ? 2 : 1;
      const report = (v: 1 | 2) => c.noteSizeFrom(img(v, pair.actual[v - 1][0], pair.actual[v - 1][1]));
      for (let k = 0; k <= script.length; k += 1) {
        if (k === at) report(first);
        if (k === then) report(second);
        if (k < script.length) script[k].run(c);
      }
      return c._offset;
    };

    const apart = (a: any, b: any) => Math.abs(a.dx - b.dx) > 1e-6
      || Math.abs(a.dy - b.dy) > 1e-6 || Math.abs(a.k - b.k) > 1e-9;
    const diverged: string[] = [];
    const detail: string[] = [];
    let runs = 0;
    PAIRS.forEach((pair, p) => {
      SCRIPTS.forEach((script, sc) => {
        for (let at = 0; at <= script.length; at += 1) {
          for (let then = at; then <= script.length; then += 1) {
            runs += 1;
            const slot0First = play(pair, script, at, then, 1);
            const slot1First = play(pair, script, at, then, 2);
            if (apart(slot0First, slot1First)) {
              diverged.push(`p${p}/s${sc}/${at},${then}`);
              const where = script.map((st, i) => (i === at || i === then ? `[load] ${st.name}` : st.name));
              detail.push(`p${p}/s${sc}/${at},${then} (${where.join(' -> ')}): `
                + `${JSON.stringify(slot0First)} vs ${JSON.stringify(slot1First)}`);
            }
          }
        }
      });
    });
    expect(runs).toBeGreaterThan(1000);
    // Asserted as an exact inventory rather than as "none". Scripts 0-4 and
    // pairs 0-4 -- the shapes every reported repro has taken -- converge. The
    // longer scripts below them do not everywhere, and each such interleaving
    // is listed rather than left out of the corpus, so this list is the whole
    // of what is accepted: a twenty-first divergence fails this test, and so
    // does closing one of these twenty. Two families, both with the completing
    // report arriving after the whole script -- script 6 accumulates one
    // request across two flips, and script 7 takes its last nudge in the
    // window the completing report closes. The plan records why they stand.
    const ACCEPTED_INTERLEAVINGS = [
      'p0/s6/0,5', 'p0/s6/1,5', 'p0/s6/2,5', 'p0/s7/4,5',
      'p1/s6/0,5', 'p1/s6/1,5', 'p1/s6/2,5', 'p1/s7/4,5',
      'p3/s6/0,5', 'p3/s6/1,5', 'p3/s6/2,5', 'p3/s7/4,5',
      'p4/s6/0,5', 'p4/s6/1,5', 'p4/s6/2,5', 'p4/s7/4,5',
      'p6/s6/0,5', 'p6/s6/1,5', 'p6/s6/2,5', 'p7/s7/4,5',
    ];
    expect({ diverging: diverged, detail: detail.slice(0, 3) })
      .toEqual({ diverging: ACCEPTED_INTERLEAVINGS, detail: detail.slice(0, 3) });
  });

  test('a report that moves one axis leaves the other axis its legitimate outside', () => {
    // Provenance is a fact about an axis, not about the component. A report
    // that corrects only the height moves the y bound and, because the offset
    // converts through the *width* ratio alone, moves neither the x bound nor
    // the x value -- so an x sitting legitimately outside after a flip is
    // still nobody's doing but the flip's, and the next press must anchor it.
    // One boolean for both axes let a height-only report claim x as well: an
    // ArrowUp then resolved x from -1800 to -1200, and the two decode orders
    // came apart, 300 against 450.
    const run = (first: 1 | 2, second: 1 | 2) => {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.zoomBy(-0.75);
      c.nudge(450, 0);
      c.swapSides();
      expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
      // Slot 0 is taller than stored; every width in the pair is 800.
      const size = (v: 1 | 2): [number, number] => (v === 1 ? [800, 900] : [800, 600]);
      c.noteSizeFrom(img(first, ...size(first)));
      c.nudge(0, 1);
      c.noteSizeFrom(img(second, ...size(second)));
      return c._offset.dx;
    };
    expect(run(1, 2)).toBeCloseTo(run(2, 1), 6);
    expect(run(1, 2)).toBeCloseTo(450, 6);
  });

  test('a control operation claims only the axes it actually pushed out of bounds', () => {
    // A zoom moves both bounds, but moving a bound is not the same as
    // breaking something: the correction here is at the edge of the bound
    // before the zoom and at the edge of the smaller one after it, inside
    // both times. Claiming the axis anyway outlives the operation and
    // suppresses the anchor of the outside the reader's own flip derives a
    // moment later -- canonical 450 came back as 300.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.noteSizeFrom(img(1, 800, 600));
    c.nudge(450, 0);
    c.zoomBy(-0.75);
    expect(c.trailOffset.dx).toBeCloseTo(450, 6);
    c.swapSides();
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    c.nudge(0, 1);
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(-1800, 6);
    expect(c._offset.dx).toBeCloseTo(450, 6);
  });

  test('a pending remainder keeps Reset actionable while the readout sits at identity', () => {
    // `offsetIsIdentity` describes only what is shown; the half-measured
    // window can leave an unshown remainder underneath it. Drawing Reset
    // aria-disabled while pressing it would clear that remainder says the
    // control cannot act exactly when it can, so idle means no displayed
    // correction AND no outstanding request.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.noteSizeFrom(img(1, 600, 800));
    c.nudge(-1100, 0);
    c.nudge(600, 0);
    expect(c.offsetIsIdentity).toBe(true);
    expect(c.resetIdle).toBe(false);
    // And clearing it is the control's own work: the readout was already at
    // identity, but activating Reset now still announces, because something
    // actually changed.
    let announced = false;
    vi.spyOn(c, 'announceOffset').mockImplementation(() => { announced = true; });
    c.resetAlignment();
    expect(c._requestedOffset).toBeNull();
    expect(announced).toBe(true);
    expect(c.resetIdle).toBe(true);
  });

  test('a report that moves neither the offset nor its bound arms no debt', () => {
    // The debt is owed when a size report moves the offset or the bound it
    // sits in. A report that only resizes the *leading* version without
    // overtaking the box moves neither -- and arming anyway lets a later
    // confirming report rewrite alignment the reader made after it.
    const c = component({ w: 1600, h: 1200 }, { w: 1600, h: 1200 });
    c.toggleAligning();
    c.nudge(1, 0);
    // Slot 0 reports 800x600: slot 1 still determines the 1600x1200 box and
    // is the trail, so nothing the offset depends on changed.
    c.noteSizeFrom(img(1, 800, 600));
    c.swapSides();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    // Nothing the offset depends on changed, so no axis crossed a bound.
    expect(c._sizeOwedX).toBe(false);
    expect(c._sizeOwedY).toBe(false);
    // The flipped view's bound for the 200px-rendered 800x600 slot is +850.
    expect(c.trailOffset.dx).toBeCloseTo(850, 6);
    // Slot 1 confirms, and the window's request resolves canonically. The
    // leading version's own resize claimed nothing, which is the rule; the
    // move from +850 is the canonical resolution the flipped window is always
    // charged.
    c.noteSizeFrom(img(2, 1600, 1200));
    expect(c.trailOffset.dx).toBeCloseTo(600, 6);
    expect(c._sizeOwedX).toBe(false);
  });

  test('reset retires the debt with the correction that earned it', () => {
    // The debt is the promise that a specific correction will be brought back
    // once both versions have reported. Reset discards that correction, so
    // the promise must die with it -- left armed, a later confirming report
    // would rewrite alignment the reader makes *after* the reset.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    // Slot 0 grows to 1600x1200: conversion to 900, debt armed.
    c.noteSizeFrom(img(1, 1600, 1200));
    expect(c.trailOffset.dx).toBeCloseTo(900, 6);
    // The reader resets, then builds a fresh alignment in the flipped view.
    c.resetAlignment();
    // The promise died with the correction that earned it.
    expect(c._sizeReboundDue).toBe(false);
    expect(c._sizeOwedX).toBe(false);
    expect(c._sizeOwedY).toBe(false);
    c.swapSides();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    // The flipped bound for the 400px-rendered 1600x1200 slot is +900.
    expect(c.trailOffset.dx).toBeCloseTo(900, 6);
    // Slot 1 confirms and the fresh window's own request resolves against the
    // canonical bound. What must not happen is the *old* claim acting, and
    // there is none: Reset cleared it with the correction it belonged to.
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(400, 6);
    expect(c._sizeOwedX).toBe(false);
  });

  test('a tap is left alone, so an armed reader can still switch versions', () => {
    // preventDefault on touchstart suppresses the synthesized click, so while
    // armed a tap on toggle mode's button would do nothing at all.
    const dom = fakeDom();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      let prevented = false;
      c.startAlignDrag({
        type: 'touchstart',
        currentTarget: frame(),
        target: { closest: () => null },
        touches: [{ clientX: 100, clientY: 100 }],
        preventDefault() { prevented = true; },
      });
      expect(prevented).toBe(false);
      // The gesture still runs -- a touch drag aligns, it just does not cancel
      // the tap that a touch without movement produces.
      expect(dom.listening('touchmove')).toBe(1);
      dom.fire('touchmove', { touches: [{ clientX: 140, clientY: 100 }] });
      expect(c.trailOffset.dx).toBeCloseTo(40, 6);
    } finally {
      dom.restore();
    }
  });

  test('a corrected box keeps an existing offset the same distance on screen', () => {
    // The offset is in box pixels. A reader who nudged against the stored
    // placeholder and then saw the browser report the real dimensions would
    // otherwise watch their correction change size on its own.
    const c = component({ w: 400, h: 300 }, { w: 400, h: 300 });
    c.toggleAligning();
    c.nudge(40, -30);
    expect(transformOf(c.trailScale)).toEqual({ tx: 10, ty: -10, k: 1 });

    c.noteSizeFrom(img(1, 800, 600));
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(80, 6);
    expect(transformOf(c.trailScale)).toEqual({ tx: 10, ty: -10, k: 1 });
  });

  test('a correction to the box height alone moves nothing on screen', () => {
    // Both components scale by the *width* ratio, because the box is
    // width-driven: it takes the container's width and derives its height from
    // `aspect-ratio`, so one box pixel is the same distance on screen in either
    // axis. Scaling each axis by its own ratio looks right and is not -- this
    // correction changes no physical distance at all, and would double `dy`.
    const c = component({ w: 400, h: 300 }, { w: 400, h: 300 });
    c.toggleAligning();
    c.nudge(0, -30);

    c.noteSizeFrom(img(1, 400, 600));
    c.noteSizeFrom(img(2, 400, 600));
    expect(c.trailOffset.dy).toBeCloseTo(-30, 6);
    // The percentage halves precisely because the element is twice as tall on
    // screen: -10% of 0.75R and -5% of 1.5R are the same number of pixels.
    expect(transformOf(c.trailScale)!.ty).toBeCloseTo(-5, 6);
  });

  test('the zoom range is reciprocal, so a flip never lands outside it', () => {
    // 0.25 is 1/4 exactly. If the two ends were not reciprocal, one flip would
    // derive a scale the reader could not have chosen.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.zoomBy(100);
    expect(c.trailOffset.k).toBe(4);
    c.swapSides();
    expect(c.trailOffset.k).toBe(0.25);
    c.swapSides();
    expect(c.trailOffset.k).toBe(4);
  });

  test('the zoom is clamped to a quarter and four times', () => {
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.zoomBy(100);
    expect(transformOf(c.trailScale)!.k).toBe(4);
    c.zoomBy(-100);
    expect(transformOf(c.trailScale)!.k).toBe(0.25);
  });

  test('the zoom scales from wherever the anchor holds the image', () => {
    // Anchor pressed means the reader has asserted "these two line up at the
    // corner". Scaling from the centre then walks that corner off by half the
    // scale change and undoes the thing they just said.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.zoomBy(0.1);
    expect(c.trailScale.transformOrigin).toBe('');
    c.toggleAnchor();
    expect(c.trailScale.transformOrigin).toBe('0 0');
  });

  test('a change of scale policy leaves the correction alone', () => {
    // The offset is in box pixels, so it still means "shift slot 1 by twelve"
    // after the switch. Silently discarding work the reader did is the worse
    // failure, and Anchor already changes the sizing without resetting anything.
    const c = component({ w: 800, h: 600 }, { w: 400, h: 300 });
    c.toggleAligning();
    c.nudge(12, -4);
    for (const policy of ['fit', 'stretch', 'relative']) c.setScale(policy);
    expect(c.offsetLabel).toBe('+12, -4, 100%');
  });

  test('stretch still carries the correction, unlike the anchor', () => {
    // Anchor refuses under Stretch because it provably cannot act -- there is
    // no slack to take up. A translate acts under every policy, so refusing it
    // would be extending that rule past its reason.
    const c = component({ w: 800, h: 600 }, { w: 400, h: 300 });
    c.toggleAligning();
    c.setScale('stretch');
    c.nudge(80, 0);
    expect(c.anchorAvailable).toBe(false);
    expect(c.alignAvailable).toBe(true);
    // Under stretch the element *is* the box, so eighty box pixels is 10%.
    expect(transformOf(c.trailScale)).toEqual({ tx: 10, ty: 0, k: 1 });
  });

  test('a pair with nothing to scale against cannot be aligned either', () => {
    // Under the no-dimensions fallback the lead is `position: static` and the
    // trail is pinned to the top with `margin: 0`: the two are not in a shared
    // coordinate space, and the formats that reach it paint nothing to align.
    const c = component({ w: 0, h: 0 }, { w: 0, h: 0 });
    expect(c.alignAvailable).toBe(false);
    c.toggleAligning();
    expect(c.aligning).toBe(false);
    c.nudge(12, -4);
    expect(c.offsetLabel).toBe('0, 0, 100%');
    expect(c.trailScale.transform).toBe('');
  });

  test('reset clears the translation and the zoom together, and stays armed', () => {
    // A reset that left a 103% zoom behind would be the same invisible state in
    // a smaller box. The arming survives because the reason you reset is almost
    // always that you are about to try again.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.nudge(12, -4);
    c.zoomBy(0.03);
    expect(c.offsetLabel).toBe('+12, -4, 103%');
    expect(c.offsetIsIdentity).toBe(false);

    c.resetAlignment();
    expect(c.offsetLabel).toBe('0, 0, 100%');
    expect(c.offsetIsIdentity).toBe(true);
    expect(c.trailScale.transform).toBe('');
    expect(c.aligning).toBe(true);
  });

  test('the keys do nothing at all until the reader arms the mode', () => {
    // The arrow keys are already spent -- `_keyHandler` gives them to the
    // slider position and the onion opacity by mode. Arming is what makes them
    // unambiguously the alignment's.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    const e = key('ArrowRight');
    expect(c.handleAlignKey(e)).toBe(false);
    expect(e.prevented).toBe(false);
    expect(c.offsetIsIdentity).toBe(true);

    c.toggleAligning();
    expect(c.handleAlignKey(key('ArrowRight'))).toBe(true);
    expect(c.offsetLabel).toBe('+1, 0, 100%');
  });

  test('shift takes the bigger step in both axes and in the zoom', () => {
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.handleAlignKey(key('ArrowDown', true));
    c.handleAlignKey(key('ArrowLeft'));
    c.handleAlignKey(key('=', true));
    expect(c.offsetLabel).toBe('-1, +10, 110%');
  });

  test('R resets, and only while armed', () => {
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    c.nudge(12, 0);
    expect(c.handleAlignKey(key('r'))).toBe(true);
    expect(c.offsetIsIdentity).toBe(true);

    c.nudge(12, 0);
    c.toggleAligning();
    expect(c.handleAlignKey(key('r'))).toBe(false);
    expect(c.offsetIsIdentity).toBe(false);
  });

  test('+ - and R reach the alignment from a focused button, while the arrows stay its own', () => {
    // The container handler skips what a focusable element answers itself: the
    // radiogroups navigate with arrows, the Align button carries its own
    // handler, the slider handle binds its own keys. But no button answers
    // `+`, `-` or `R` -- so guarding those too meant a reader who armed Align
    // and then clicked Flip or Anchor could not press R to reset, because
    // focus had moved to the button. The guard now splits: text fields keep
    // every key, arrows and Home/End stay on the focused control, and the
    // alignment's remaining keys pass.
    const g = globalThis as any;
    const previous = g.HTMLElement;
    class FakeElement {}
    g.HTMLElement = FakeElement;
    // `closest` answers the selectors the guard asks about, so a fake target
    // can be "a button" or "a text input" without a DOM.
    const focused = (kind: 'button' | 'input') => Object.assign(new FakeElement(), {
      closest: (sel: string) => (sel.includes(kind) ? {} : null),
    });
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.$el = { addEventListener() {}, querySelectorAll: () => [] };
      c.init();
      const fire = (key: string, target: any) => {
        const e: any = { key, shiftKey: false, target, defaultPrevented: false };
        e.preventDefault = () => { e.defaultPrevented = true; };
        c._keyHandler(e);
      };

      c.nudge(12, 0);
      // Focus on the Flip button, which answers no keys of its own: R resets.
      fire('r', focused('button'));
      expect(c.offsetIsIdentity).toBe(true);

      // ...and `+` zooms from there too.
      c.nudge(12, 0);
      fire('=', focused('button'));
      expect(c.trailOffset.k).toBeCloseTo(1.01, 6);

      // The arrows stay the focused control's own, or the radiogroup would
      // double-step: a nudge from a button is still refused.
      c._offset = { dx: 0, dy: 0, k: 1 };
      fire('ArrowRight', focused('button'));
      expect(c.offsetIsIdentity).toBe(true);

      // A text field owns every key: R there means the letter, not a reset.
      c.nudge(12, 0);
      fire('r', focused('input'));
      expect(c.offsetIsIdentity).toBe(false);
    } finally {
      g.HTMLElement = previous;
    }
  });

  test('a range input keeps its arrows but lets + - and R through', () => {
    // The onion-opacity slider is an <input type="range">: it answers the
    // arrows and Home/End itself, but `+`, `-` and `R` are not its keys. The
    // text-field guard must not treat it as one, or an armed reader focused
    // on the slider could not reset or zoom.
    const g = globalThis as any;
    const previous = g.HTMLElement;
    class FakeElement {}
    g.HTMLElement = FakeElement;
    const element = (kind: 'text' | 'range') => Object.assign(new FakeElement(), {
      closest: (sel: string) => {
        if (sel.includes('input:not([type="range"])')) return kind === 'text' ? {} : null;
        if (sel.includes('input') || sel.includes('button')) return {};
        return null;
      },
    });
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.$el = { addEventListener() {}, querySelectorAll: () => [] };
      c.init();
      const fire = (key: string, target: any) => {
        const e: any = { key, shiftKey: false, target, defaultPrevented: false };
        e.preventDefault = () => { e.defaultPrevented = true; };
        c._keyHandler(e);
      };

      c.nudge(12, 0);
      // On the range: R resets and = zooms...
      fire('r', element('range'));
      expect(c.offsetIsIdentity).toBe(true);
      c.nudge(12, 0);
      fire('=', element('range'));
      expect(c.trailOffset.k).toBeCloseTo(1.01, 6);
      // ...while the arrows stay the slider's own.
      c._offset = { dx: 0, dy: 0, k: 1 };
      fire('ArrowRight', element('range'));
      expect(c.offsetIsIdentity).toBe(true);

      // A real text field still owns every key.
      c.nudge(12, 0);
      fire('r', element('text'));
      expect(c.offsetIsIdentity).toBe(false);
    } finally {
      g.HTMLElement = previous;
    }
  });

  test('modified keys stay the browser\'s, even while armed', () => {
    // The alignment's shortcuts are the plain keys, Shift steps them up, and
    // Ctrl/Cmd/Alt are the browser's: Ctrl+R reloads, Ctrl+/- zooms the page,
    // Alt+ArrowLeft goes back. Stealing them while armed -- resetting on
    // Ctrl+R, zooming the image on Ctrl+- -- leaves the reader unable to
    // reload or magnify until they disarm.
    const g = globalThis as any;
    const previous = g.HTMLElement;
    class FakeElement {}
    g.HTMLElement = FakeElement;
    const plain = Object.assign(new FakeElement(), { closest: () => null });
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.nudge(12, 0);
      c.$el = { addEventListener() {}, querySelectorAll: () => [] };
      c.init();
      const fire = (key: string, mods: any = {}) => {
        const e: any = { key, shiftKey: false, target: plain, defaultPrevented: false, ...mods };
        e.preventDefault = () => { e.defaultPrevented = true; };
        c._keyHandler(e);
        return e;
      };

      // Ctrl+R is reload, not a reset; Ctrl+- is page zoom, not the
      // alignment's zoom-out; Alt+ArrowLeft is back, not a nudge. None are
      // defaulted-prevented either, so the browser's own action proceeds.
      fire('r', { ctrlKey: true });
      expect(c.offsetIsIdentity).toBe(false);
      fire('-', { metaKey: true });
      expect(c.trailOffset.k).toBe(1);
      fire('ArrowLeft', { altKey: true });
      expect(c.trailOffset.dx).toBe(12);
      expect(fire('=', { ctrlKey: true }).defaultPrevented).toBe(false);

      // handleAlignKey itself refuses too: the Align button calls it directly,
      // bypassing the container handler.
      expect(c.handleAlignKey({ key: 'r', ctrlKey: true, shiftKey: false, preventDefault() {} })).toBe(false);

      // The plain keys still work.
      fire('r');
      expect(c.offsetIsIdentity).toBe(true);
    } finally {
      g.HTMLElement = previous;
    }
  });

  test('the wheel leaves the browser its own magnification and its sideways swipe', () => {
    // Ctrl+wheel is page zoom, and a trackpad pinch arrives as exactly that:
    // taking it leaves the page unzoomable for as long as the reader is
    // aligning. A sideways swipe reports deltaY 0, which `deltaY < 0` reads as
    // "not up" and would answer by zooming out.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();

    const wheel = (init: any) => {
      let prevented = false;
      const e = { deltaY: 0, ctrlKey: false, metaKey: false, ...init, preventDefault() { prevented = true; } };
      c.onAlignWheel(e);
      return () => prevented;
    };

    expect(wheel({ deltaY: -120, ctrlKey: true })()).toBe(false);
    expect(c.trailOffset.k).toBe(1);
    expect(wheel({ deltaY: 0 })()).toBe(false);
    expect(c.trailOffset.k).toBe(1);

    expect(wheel({ deltaY: -120 })()).toBe(true);
    expect(c.trailOffset.k).toBeCloseTo(1.01, 6);
    expect(wheel({ deltaY: 120 })()).toBe(true);
    expect(c.trailOffset.k).toBeCloseTo(1, 6);
  });

  test('a wheel burst announces once it stops, not once per event', () => {
    // One notch delivers a burst of events. Announcing per event is the
    // pointermove mistake in another shape.
    vi.useFakeTimers();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      for (let i = 0; i < 5; i++) {
        c.onAlignWheel({ deltaY: -120, ctrlKey: false, metaKey: false, preventDefault() {} });
        vi.advanceTimersByTime(50);
      }
      expect(c.offsetAnnouncement).toBe('');
      vi.advanceTimersByTime(250);
      expect(c.offsetAnnouncement).toContain('105%');
    } finally {
      vi.useRealTimers();
    }
  });

  /**
   * The drag path, with the handful of DOM entry points it actually uses.
   *
   * This suite runs with no DOM, so the drag was reachable only end-to-end --
   * which left its own rules (announce once at the end, arm the toggle-click
   * suppression only where that click can happen) pinned by nothing that runs
   * in a second. The component touches `addEventListener` on `document` and
   * `window`, `document.body.style`, and the box's geometry; that is the whole
   * surface, so stubbing it is faithful rather than a mock of the behaviour.
   */
  function fakeDom() {
    // Separate registries for the two targets: the component registers its
    // drag listeners on `document` and the blur ender on `window`, and the
    // teardown assertions are only meaningful if removing from the wrong
    // target is visible as a still-installed listener.
    const handlers: Record<string, Record<string, Function[]>> = { document: {}, window: {} };
    const add = (target: string, type: string, fn: Function) => { (handlers[target][type] ||= []).push(fn); };
    const remove = (target: string, type: string, fn: Function) => {
      handlers[target][type] = (handlers[target][type] || []).filter((h) => h !== fn);
    };
    const g = globalThis as any;
    const previous = { document: g.document, window: g.window };
    g.document = {
      addEventListener: (t: string, fn: Function) => add('document', t, fn),
      removeEventListener: (t: string, fn: Function) => remove('document', t, fn),
      body: { style: {} },
    };
    g.window = {
      addEventListener: (t: string, fn: Function) => add('window', t, fn),
      removeEventListener: (t: string, fn: Function) => remove('window', t, fn),
    };
    return {
      fire(type: string, event: any = {}, target: 'document' | 'window' = 'document') {
        (handlers[target][type] || []).slice().forEach((h) => h({ preventDefault() {}, type, ...event }));
      },
      listening(type: string) {
        return (handlers.document[type] || []).length + (handlers.window[type] || []).length;
      },
      restore() { g.document = previous.document; g.window = previous.window; },
    };
  }

  const frame = (width = 800, height = 600, contains = true) => ({
    getBoundingClientRect: () => ({ left: 0, top: 0, width, height }),
    clientWidth: width,
    clientHeight: height,
    // Whether a release on this frame lands *inside* it -- the click that
    // follows a drag only fires when the release is on the button.
    contains: () => contains,
  });

  const press = (x: number, y: number, from: any = { closest: () => null }, frameOverride?: any) => ({
    currentTarget: frameOverride ?? frame(), target: from, clientX: x, clientY: y, preventDefault() {},
  });

  test('a drag announces once, at the end', () => {
    // Announcing per move is the mistake the whole split exists to avoid: a
    // gesture is hundreds of events and a live region reads every one of them.
    const dom = fakeDom();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.startAlignDrag(press(100, 100));

      dom.fire('mousemove', { clientX: 120, clientY: 110 });
      dom.fire('mousemove', { clientX: 140, clientY: 130 });
      expect(c.offsetLabel).toBe('+40, +30, 100%');
      expect(c.offsetAnnouncement).toBe('');

      dom.fire('mouseup', {});
      expect(c.offsetAnnouncement).toContain('+40, +30');
      // And every listener it installed is gone.
      for (const type of ['mousemove', 'mouseup', 'touchmove', 'touchend', 'touchcancel', 'blur']) {
        expect(dom.listening(type)).toBe(0);
      }
    } finally {
      dom.restore();
    }
  });

  test('disarming mid-drag ends it: nothing moves while disarmed', () => {
    // Space on the focused Align button toggles it while the mouse is still
    // held, and the gesture's movement has to end with the arming -- decision
    // 1 is "nothing nudges while disarmed".
    const dom = fakeDom();
    try {
      // Disarm, then move: the move must not nudge.
      const stayed = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      stayed.toggleAligning();
      stayed.startAlignDrag(press(100, 100));
      dom.fire('mousemove', { clientX: 120, clientY: 100 });
      expect(stayed.trailOffset.dx).toBeCloseTo(20, 6);
      stayed.toggleAligning();
      dom.fire('mousemove', { clientX: 200, clientY: 100 });
      expect(stayed.trailOffset.dx).toBeCloseTo(20, 6);

      // Disarm and re-arm *without moving in between*, then move: the
      // disarm removed the move listeners, so a re-arm cannot resurrect the
      // gesture.
      const rearmed = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      rearmed.toggleAligning();
      rearmed.startAlignDrag(press(100, 100));
      dom.fire('mousemove', { clientX: 120, clientY: 100 });
      expect(rearmed.trailOffset.dx).toBeCloseTo(20, 6);
      rearmed.toggleAligning();
      rearmed.toggleAligning();
      dom.fire('mousemove', { clientX: 300, clientY: 100 });
      expect(rearmed.trailOffset.dx).toBeCloseTo(20, 6);

      // The release ends whatever is left of each gesture; then every
      // listener it installed is gone.
      dom.fire('mouseup', {});
      dom.fire('mouseup', {});
      for (const type of ['mousemove', 'mouseup', 'touchmove', 'touchend', 'touchcancel', 'blur']) {
        expect(dom.listening(type)).toBe(0);
      }
    } finally {
      dom.restore();
    }
  });

  test('a non-primary button press is not a drag', () => {
    // Right-dragging the image must not move it -- and must not arm the
    // toggle-click suppression, because a right-button release generates no
    // click at all, so a later deliberate left click would be eaten. The
    // context menu has to survive too: preventDefault on a right mousedown
    // would suppress it.
    const dom = fakeDom();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.mode = 'toggle';
      c.toggleAligning();
      let prevented = false;
      c.startAlignDrag({ ...press(100, 100), type: 'mousedown', button: 2, preventDefault() { prevented = true; } });
      expect(prevented).toBe(false);
      expect(dom.listening('mousemove')).toBe(0);
      expect(c.trailOffset.dx).toBe(0);

      // A left click right after is the reader's own: nothing was stamped.
      c.toggleSide({ detail: 1 });
      expect(c.showLeft).toBe(false);
    } finally {
      dom.restore();
    }
  });

  test('a load converting the offset mid-press is not a drag', () => {
    // The suppression is for the click ending a *drag*. A press with no
    // movement is a tap, even if a load converts the offset while the button
    // is held -- the click is the reader's own, not the end of a gesture.
    const dom = fakeDom();
    try {
      const c = component({ w: 400, h: 300 }, { w: 400, h: 300 });
      c.mode = 'toggle';
      c.toggleAligning();
      c.nudge(40, 0);
      c.startAlignDrag(press(100, 100));
      // A load lands while the button is held: the offset is converted but no
      // drag movement happened.
      c.noteSizeFrom(img(1, 800, 600));
      expect(c.trailOffset.dx).toBeCloseTo(80, 6);
      dom.fire('mouseup', {});
      // The click is a tap, not the end of a drag: it switches versions, and
      // the release announces nothing -- the conversion was a load's doing,
      // not the gesture's, and the live region reads only what the reader
      // did.
      expect(c.offsetAnnouncement).toBe('');
      c.toggleSide({ detail: 1 });
      expect(c.showLeft).toBe(false);
    } finally {
      dom.restore();
    }
  });

  test('the drag teardown assertions can tell which target a listener was removed from', () => {
    // fakeDom keeps the document and window registries apart, so "every
    // listener it installed is gone" would catch a regression that removed a
    // mousemove handler from window instead of document.
    const dom = fakeDom();
    try {
      const g = globalThis as any;
      const fn = () => {};
      g.document.addEventListener('mousemove', fn);
      // Removing the same handler from the *window* is a no-op, as with real
      // addEventListener/removeEventListener targets...
      g.window.removeEventListener('mousemove', fn);
      // ...so the listener is still installed, and the assertion sees it.
      expect(dom.listening('mousemove')).toBe(1);
    } finally {
      dom.restore();
    }
  });

  test('a press that moves nothing announces nothing and suppresses nothing', () => {
    const dom = fakeDom();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.mode = 'toggle';
      c.toggleAligning();
      c.startAlignDrag(press(100, 100));
      dom.fire('mouseup', {});

      expect(c.offsetAnnouncement).toBe('');
      c.toggleSide({ detail: 1 });
      expect(c.showLeft).toBe(false);
    } finally {
      dom.restore();
    }
  });

  test('only a drag that ends in toggle mode arms the click suppression', () => {
    // Toggle mode's frame is a button and the click ending a drag across it
    // would also switch versions. A drag in onion skin produces no such click,
    // so stamping there swallowed the first click after switching modes.
    const dom = fakeDom();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.mode = 'onion';
      c.toggleAligning();
      c.startAlignDrag(press(100, 100));
      dom.fire('mousemove', { clientX: 140, clientY: 100 });
      dom.fire('mouseup', {});

      c.mode = 'toggle';
      c.toggleSide({ detail: 1 });
      expect(c.showLeft).toBe(false);

      c.startAlignDrag(press(100, 100));
      dom.fire('mousemove', { clientX: 160, clientY: 100 });
      dom.fire('mouseup', {});
      c.toggleSide({ detail: 1 });
      expect(c.showLeft).toBe(false);
    } finally {
      dom.restore();
    }
  });

  test('a touch drag never arms the click suppression', () => {
    // preventDefault on the touchmove cancels that gesture's compatibility
    // click, so no click can follow a touch drag -- and stamping the window
    // anyway would eat the deliberate tap that comes next, which is a tap the
    // reader meant for the version switch.
    const dom = fakeDom();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.mode = 'toggle';
      c.toggleAligning();
      c.startAlignDrag({
        type: 'touchstart',
        currentTarget: frame(),
        target: { closest: () => null },
        touches: [{ clientX: 100, clientY: 100 }],
        preventDefault() {},
      });
      dom.fire('touchmove', { touches: [{ clientX: 140, clientY: 100 }] });
      dom.fire('touchend', {});
      // The drag did move the offset, and the tap right after it is not that
      // drag's click -- the touchmove's preventDefault suppressed it.
      c.toggleSide({ detail: 1 });
      expect(c.showLeft).toBe(false);
    } finally {
      dom.restore();
    }
  });

  test('a drag released outside the toggle button arms no suppression', () => {
    // The click that would follow the drag is the button's -- but only if the
    // release landed on the button. Released outside, no click follows, and a
    // stamp would eat the reader's next deliberate click inside the window.
    const dom = fakeDom();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.mode = 'toggle';
      c.toggleAligning();
      c.startAlignDrag(press(100, 100, { closest: () => null }, frame(800, 600, false)));
      dom.fire('mousemove', { clientX: 140, clientY: 100 });
      dom.fire('mouseup', {});
      // The release was outside the button, so no click followed the drag --
      // and this tap is the reader's own, not the drag's.
      c.toggleSide({ detail: 1 });
      expect(c.showLeft).toBe(false);
    } finally {
      dom.restore();
    }
  });

  test('a two-finger gesture is the browser\'s, not a drag', () => {
    // `touch-action: manipulation` keeps pinch zoom available, so a gesture
    // with two fingers must not move the image and must not be cancelled by
    // a preventDefault -- that would cancel the browser's own magnification,
    // the touch equivalent of stealing Ctrl+wheel.
    const dom = fakeDom();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      // A pinch that starts with two fingers never starts a drag at all.
      c.startAlignDrag({
        type: 'touchstart',
        currentTarget: frame(),
        target: { closest: () => null },
        touches: [{ clientX: 100, clientY: 100 }, { clientX: 120, clientY: 120 }],
        preventDefault() {},
      });
      expect(dom.listening('touchmove')).toBe(0);

      // A second finger landing mid-drag stops it without preventing: the
      // pinch is the gesture now.
      c.startAlignDrag({
        type: 'touchstart',
        currentTarget: frame(),
        target: { closest: () => null },
        touches: [{ clientX: 100, clientY: 100 }],
        preventDefault() {},
      });
      dom.fire('touchmove', { touches: [{ clientX: 140, clientY: 100 }] });
      expect(c.trailOffset.dx).toBeCloseTo(40, 6);
      let prevented = false;
      dom.fire('touchmove', {
        touches: [{ clientX: 150, clientY: 100 }, { clientX: 130, clientY: 100 }],
        preventDefault() { prevented = true; },
      });
      expect(prevented).toBe(false);
      expect(c.trailOffset.dx).toBeCloseTo(40, 6);

      // And it stays the browser's. A finger lifting does not hand the
      // gesture back: the move listener is gone, so the rest of the pinch
      // neither moves the image nor is cancelled. Resuming would also move it
      // by the distance the remaining finger covered *during* the pinch --
      // `last` is only written by a move this handler processed -- on top of
      // whatever the browser's magnification did to what a client coordinate
      // means.
      expect(dom.listening('touchmove')).toBe(0);
      let laterPrevented = false;
      dom.fire('touchmove', {
        touches: [{ clientX: 170, clientY: 100 }],
        preventDefault() { laterPrevented = true; },
      });
      expect(laterPrevented).toBe(false);
      expect(c.trailOffset.dx).toBeCloseTo(40, 6);

      // The enders stay, as they do for a mid-gesture disarm: the release
      // still finishes the gesture and announces the one move it made.
      dom.fire('touchend', {});
      expect(c.offsetAnnouncement).toContain('+40');
      for (const type of ['mousemove', 'mouseup', 'touchmove', 'touchend', 'touchcancel', 'blur']) {
        expect(dom.listening(type)).toBe(0);
      }
    } finally {
      dom.restore();
    }
  });

  test('a drag converts both axes through the box width, keeping a box pixel isotropic', () => {
    // The box is width-driven: one box pixel is the same screen distance in
    // either axis (renderedWidth / box.w). Converting the y-axis through the
    // rendered *height* -- a rounded integer -- breaks that for extreme aspect
    // ratios: a 4000x397 box rendered 800px wide is 79.4px tall, reports 79,
    // and a 10px vertical drag becomes 50.25 box pixels instead of 50.
    const dom = fakeDom();
    try {
      const c = component({ w: 4000, h: 397 }, { w: 4000, h: 397 });
      c.toggleAligning();
      c.startAlignDrag(press(100, 100, { closest: () => null }, frame(800, 79)));
      dom.fire('mousemove', { clientX: 110, clientY: 110 });
      dom.fire('mouseup', {});
      expect(c.trailOffset.dx).toBeCloseTo(50, 6);
      expect(c.trailOffset.dy).toBeCloseTo(50, 6);
    } finally {
      dom.restore();
    }
  });

  test('a press that came from the slider handle is not a drag at all', () => {
    // The handle's own mousedown bubbles to the box this handler is on, so
    // without the refusal an armed drag on it moves the reveal position and the
    // trailing version at the same time.
    const dom = fakeDom();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.startAlignDrag(press(100, 100, { closest: (s: string) => (s === '.compare-slider-handle' ? {} : null) }));
      expect(dom.listening('mousemove')).toBe(0);
    } finally {
      dom.restore();
    }
  });

  test('a drag suppresses only the click that ends it', () => {
    // Toggle mode's box is a button, so the click ending a drag across it would
    // also switch versions. Bounded in time rather than by a flag the next
    // click clears: a drag in onion skin, a disarm, then a click here is an
    // unrelated click, and a sticky flag eats it.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    expect(c.showLeft).toBe(true);

    c._alignDragEndedAt = Date.now();
    c.toggleSide({ detail: 1 });
    expect(c.showLeft).toBe(true);

    // Keyboard activation reports detail 0 and cannot have followed a drag.
    c._alignDragEndedAt = Date.now();
    c.toggleSide({ detail: 0 });
    expect(c.showLeft).toBe(false);

    // And a drag from some earlier interaction does not reach forward.
    c._alignDragEndedAt = Date.now() - 5000;
    c.toggleSide({ detail: 1 });
    expect(c.showLeft).toBe(true);

    // The suppression is spent on the one click it was for: a second click
    // inside the same window is the reader asking again.
    c.showLeft = true;
    c._alignDragEndedAt = Date.now();
    c.toggleSide({ detail: 1 });
    expect(c.showLeft).toBe(true);
    c.toggleSide({ detail: 1 });
    expect(c.showLeft).toBe(false);
  });

  test('a wheel settle pending when a key lands does not announce afterwards', () => {
    // The keyboard announcement is the current value; letting the wheel's
    // pending one fire after it announces a value the reader has moved past.
    vi.useFakeTimers();
    try {
      const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
      c.toggleAligning();
      c.onAlignWheel({ deltaY: -120, ctrlKey: false, metaKey: false, preventDefault() {} });
      c.handleAlignKey(key('ArrowRight'));
      const said = c.offsetAnnouncement;
      expect(said).toContain('+1');
      vi.advanceTimersByTime(1000);
      expect(c.offsetAnnouncement).toBe(said);
    } finally {
      vi.useRealTimers();
    }
  });

  test('nudge and announceOffset are separate, which is what lets a drag stay quiet', () => {
    // A live region written on every pointermove produces a queue a screen
    // reader spends minutes reading. The visible readout updates continuously;
    // the announcement is made on each key and once at the end of a drag.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();

    c.nudge(5, 0);
    expect(c.offsetAnnouncement).toBe('');

    c.handleAlignKey(key('ArrowRight'));
    expect(c.offsetAnnouncement).toContain('+6');

    c.nudge(5, 0);
    expect(c.offsetAnnouncement).toContain('+6');
    c.announceOffset();
    expect(c.offsetAnnouncement).toContain('+11');
  });

  test('no two announcements in a row are the same string', () => {
    // `aria-live` fires on a text *change*, so nudging away from a value and
    // back would land on the text already in the region and announce nothing.
    // A zero-width mark is alternated to make each announcement a change --
    // alternated rather than accumulated, because consecutive difference is the
    // whole property and an ever-growing string is not.
    const c = component({ w: 800, h: 600 }, { w: 800, h: 600 });
    c.toggleAligning();
    const said = [];
    for (const k of ['ArrowRight', 'ArrowLeft', 'ArrowRight', 'ArrowRight']) {
      c.handleAlignKey(key(k));
      said.push(c.offsetAnnouncement);
    }
    for (let i = 1; i < said.length; i++) expect(said[i]).not.toBe(said[i - 1]);
    // And the reader hears the number, not the mark.
    expect(said[3]).toContain('+2');
    expect(said.map((t) => t.replace(/\u200B/g, ''))).toEqual([
      'Offset +1, 0, 100%',
      'Offset 0, 0, 100%',
      'Offset +1, 0, 100%',
      'Offset +2, 0, 100%',
    ]);
  });
});
