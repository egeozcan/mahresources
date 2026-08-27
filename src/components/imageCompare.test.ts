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
    expect(c.leadScale).toEqual({ width: '', height: '', objectFit: '', margin: '' });
    expect(c.overlayBoxStyle).toEqual({ aspectRatio: '' });

    c.noteSizeFrom(img(1, 400, 300));
    // Still nothing: one known size cannot describe a shared box, and a half-
    // filled pair must keep rendering the way an empty one does.
    expect(c.overlayRatio).toBeNull();

    c.noteSizeFrom(img(2, 800, 600));
    expect(c.overlayRatio).toBe('800 / 600');
    expect(c.overlayBoxStyle).toEqual({ aspectRatio: '800 / 600' });
    expect(c.leadScale).toEqual({ width: '50%', height: '50%', objectFit: '', margin: '' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '' });
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
    expect(c.leadScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '' });
    expect(c.trailScale).toEqual({ width: '50%', height: '50%', objectFit: '', margin: '' });
    // And they survive the flip back, still attached to their own version.
    c.swapped = false;
    expect(c.leadScale).toEqual({ width: '50%', height: '50%', objectFit: '', margin: '' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '' });
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
    expect(c.leadScale).toEqual({ width: '66.66666666666666%', height: '37.5%', objectFit: '', margin: '' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '' });
  });

  test('fit grows each version until an edge touches the frame', () => {
    // 400x300 into a 600x800 box: width-limited, so 600x450 -- 100% wide and
    // 56.25% tall. The taller version already touches both edges.
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    c.setScale('fit');
    expect(c.leadScale).toEqual({ width: '100%', height: '56.25%', objectFit: '', margin: '' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '' });
  });

  test('fit registers a pure resolution change exactly', () => {
    // The case the package exists for: one aspect ratio, two resolutions. Under
    // relative scale the rescan draws at double size and lines up with nothing.
    const c = component({ w: 800, h: 600 }, { w: 1600, h: 1200 });
    expect(c.leadScale).toEqual({ width: '50%', height: '50%', objectFit: '', margin: '' });
    c.setScale('fit');
    expect(c.leadScale).toEqual(c.trailScale);
    expect(c.leadScale).toEqual({ width: '100%', height: '100%', objectFit: '', margin: '' });
  });

  test('stretch distorts both versions onto the whole frame', () => {
    const c = component({ w: 400, h: 300 }, { w: 600, h: 800 });
    c.setScale('stretch');
    expect(c.leadScale).toEqual({ width: '100%', height: '100%', objectFit: 'fill', margin: '' });
    expect(c.trailScale).toEqual({ width: '100%', height: '100%', objectFit: 'fill', margin: '' });
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
    expect(c.leadScale).toEqual({ width: '100%', height: '56.25%', objectFit: '', margin: '0' });
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
