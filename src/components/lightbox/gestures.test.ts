import { beforeEach, describe, expect, it, vi } from 'vitest';

import { gestureMethods, gestureState } from './gestures.js';

// A target whose closest() answers for the selectors a test says it sits inside.
function target(inside: string[] = [], video?: { bottom: number }) {
  return {
    closest(selector: string) {
      if (selector === 'video') {
        return video ? { getBoundingClientRect: () => ({ bottom: video.bottom }) } : null;
      }
      return selector.split(',').some(s => inside.includes(s.trim())) ? {} : null;
    },
  };
}

function touch(x: number, y: number) {
  return { clientX: x, clientY: y };
}

function makeStore() {
  return {
    ...gestureState,
    ...gestureMethods,
    zoomLevel: 1,
    minZoom: 1,
    panX: 0,
    panY: 0,
    isZoomed() { return this.zoomLevel > 1; },
    setZoomLevel(level: number) { this.zoomLevel = level; },
    isVideo: () => false,
    getCurrentItem: () => ({ contentType: 'image/png' }),
    getMediaElement: () => null,
    disableAnimations: vi.fn(),
    constrainPan: vi.fn(),
    announceZoom: vi.fn(),
    next: vi.fn(),
    prev: vi.fn(),
  } as any;
}

function swipe(store: any, start: ReturnType<typeof target>, from = 300, to = 100, y = 200) {
  store.handleTouchStart({ target: start, touches: [touch(from, y)], preventDefault: vi.fn() });
  store.handleTouchEnd({ touches: [], changedTouches: [touch(to, y)] });
}

beforeEach(() => {
  (globalThis as any).document = { querySelector: () => null };
  (globalThis as any).performance = { now: () => 0 };
});

describe('lightbox touch gestures', () => {
  it('swipes from the media area navigate', () => {
    const store = makeStore();
    swipe(store, target());
    expect(store.next).toHaveBeenCalledTimes(1);
  });

  it.each(['[data-edit-panel]', '[data-quick-tag-panel]', '#zoom-preset-popover'])(
    'ignores a swipe that starts inside %s', inside => {
      const store = makeStore();
      swipe(store, target([inside]));
      expect(store.next).not.toHaveBeenCalled();
      expect(store.prev).not.toHaveBeenCalled();
    },
  );

  it('ignores a swipe that starts on a video\'s control bar but not on the rest of it', () => {
    const onControls = makeStore();
    swipe(onControls, target([], { bottom: 220 }), 300, 100, 200);
    expect(onControls.next).not.toHaveBeenCalled();

    const onFrame = makeStore();
    swipe(onFrame, target([], { bottom: 600 }), 300, 100, 200);
    expect(onFrame.next).toHaveBeenCalledTimes(1);
  });

  it('announces a pinch only when it changed the zoom', () => {
    const store = makeStore();
    const twoFingers = (x: number) => [touch(x, 100), touch(x + 100, 100)];
    // A flat two-finger swipe: distance unchanged.
    store.handleTouchStart({ target: target(), touches: twoFingers(300), preventDefault: vi.fn() });
    store.handleTouchMove({ touches: twoFingers(100), preventDefault: vi.fn() });
    store.handleTouchEnd({ touches: [], changedTouches: [] });
    expect(store.next).toHaveBeenCalledTimes(1);
    expect(store.announceZoom).not.toHaveBeenCalled();

    // A real pinch-out.
    store._navDebounce = false;
    store.handleTouchStart({ target: target(), touches: [touch(200, 100), touch(300, 100)], preventDefault: vi.fn() });
    store.handleTouchMove({ touches: [touch(100, 100), touch(400, 100)], preventDefault: vi.fn() });
    store.handleTouchEnd({ touches: [], changedTouches: [] });
    expect(store.announceZoom).toHaveBeenCalledTimes(1);
  });
});

describe('lightbox mouse drags', () => {
  function drag(store: any, { button = 0, dx = -200, dy = 0, lastDx = dx, lastDy = dy } = {}) {
    const down = { button, clientX: 400, clientY: 300, target: target(), preventDefault: vi.fn() };
    store.handleMouseDown(down);
    // Two moves: the second sets the final velocity, which can differ from the whole drag.
    store.handleMouseMove({ clientX: 400 + dx - lastDx, clientY: 300 + dy - lastDy });
    (globalThis as any).performance.now = () => 10;
    store.handleMouseMove({ clientX: 400 + dx, clientY: 300 + dy });
    store.handleMouseUp({ clientX: 400 + dx, clientY: 300 + dy });
    (globalThis as any).performance.now = () => 0;
  }

  it('a horizontal left-button drag navigates', () => {
    const store = makeStore();
    drag(store);
    expect(store.next).toHaveBeenCalledTimes(1);
  });

  it('a right-button drag does not navigate', () => {
    const store = makeStore();
    drag(store, { button: 2 });
    expect(store.next).not.toHaveBeenCalled();
    expect(store.isDragging).toBe(false);
  });

  it('a mostly vertical drag that ends in a sideways flick does not navigate', () => {
    const store = makeStore();
    drag(store, { dx: -40, dy: 300, lastDx: -30, lastDy: 2 });
    expect(store.next).not.toHaveBeenCalled();
  });
});
