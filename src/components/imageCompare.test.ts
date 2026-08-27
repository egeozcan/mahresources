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
    c.toggleAligning();
    c.swapSides();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    // The flipped view's bound for the 200px-rendered 800x1200 slot is +450.
    expect(c.trailOffset.dx).toBeCloseTo(450, 6);
    // Slot 1 confirms its stored size. The stale debt must not clamp +450 to
    // +300 -- the confirming report changed no geometry and nothing of its
    // own is owed.
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(450, 6);
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
    // The flipped view's bound for the 200px-rendered 800x600 slot is +850.
    expect(c.trailOffset.dx).toBeCloseTo(850, 6);
    // Slot 1 confirms its stored size. The stale debt must not clamp +850 to
    // +600 -- the confirming report changed no geometry and nothing of its
    // own is owed.
    c.noteSizeFrom(img(2, 1600, 1200));
    expect(c.trailOffset.dx).toBeCloseTo(850, 6);
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
    c.swapSides();
    c.zoomBy(-0.75);
    c.nudge(10000, 0);
    // The flipped bound for the 400px-rendered 1600x1200 slot is +900.
    expect(c.trailOffset.dx).toBeCloseTo(900, 6);
    // Slot 1 confirms. The old debt must not clamp +900 to +400.
    c.noteSizeFrom(img(2, 800, 600));
    expect(c.trailOffset.dx).toBeCloseTo(900, 6);
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
