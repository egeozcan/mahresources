# Lightbox Fullscreen & Zoom Design

## Overview

Add fullscreen mode, pinch zoom, and improved gesture navigation to the custom Alpine.js lightbox.

## Features

### 1. Fullscreen Mode

**Triggers:**
- Enter key toggles fullscreen (guarded by `canNavigate()` - won't trigger when focused on inputs)
- Dedicated button in bottom bar

**Behavior:**
- Uses native Fullscreen API (`requestFullscreen()` / `exitFullscreen()`)
- Listens for `fullscreenchange` event to sync state
- Exiting lightbox also exits fullscreen if active
- Fullscreen state resets when lightbox closes

**Escape key priority (layered):**
1. If edit panel open → close edit panel
2. Else if fullscreen → exit fullscreen
3. Else → close lightbox

### 2. Keyboard Shortcuts

All guarded by `canNavigate()` to avoid conflicts with input fields:

| Key | Action |
|-----|--------|
| Enter | Toggle fullscreen |
| e | Toggle edit panel |
| F2 | Toggle edit panel |
| Escape | Close (layered: edit panel → fullscreen → lightbox) |
| Arrow keys | Navigate (existing) |

### 3. Pinch Zoom

**State:**
```javascript
zoomLevel: 1,      // Current zoom (1-5)
minZoom: 1,
maxZoom: 5,
panX: 0,           // Pan offset X
panY: 0,           // Pan offset Y
```

**Behavior:**
- Free zoom between 1x and 5x
- Zoom centers on pinch point (where fingers are)
- Pinching below 1x snaps back to 1x on release
- Trackpad pinch detected via `wheel` event with `ctrlKey` modifier
- Double-tap / double-click toggles between 1x and 2x (centered on tap point)
- Zoom resets to 1x when navigating to next/prev image

**Zoom indicator:**
- Bottom-left corner, shows "2.3x" format
- Hidden when zoom is exactly 1x
- Auto-hides 1.5s after zoom activity stops
- Same styling as counter (bg-black/50, rounded, white text)

### 4. Pan & Navigation Gestures

**When NOT zoomed (zoomLevel === 1):**

| Input | Action |
|-------|--------|
| Two-finger swipe (touch) | Navigate prev/next |
| Two-finger swipe (trackpad) | Navigate prev/next |
| Mouse drag | Navigate prev/next |
| Single-finger swipe (touch) | Navigate prev/next (existing) |

**When zoomed (zoomLevel > 1):**

| Input | Action |
|-------|--------|
| Two-finger swipe (touch) | Pan around image |
| Two-finger swipe (trackpad) | Pan around image |
| Mouse drag | Pan around image |
| Single-finger swipe (touch) | Pan around image |

**Navigation feel:**
- Momentum/inertia based - gesture velocity determines trigger
- Quick flick feels snappy

**Pan boundaries:**
- Constrained so you can't pan beyond image edges
- When zoomed image is smaller than viewport in one dimension, that axis is centered

### 5. UI Changes

**Bottom bar layout:**
```
[1/24] [⛶ Fullscreen] [Resource name...] [Edit]
```

**Fullscreen button:**
- Expand icon (↗↙) when not fullscreen
- Compress icon (↙↗) when fullscreen
- Tooltip: "Enter fullscreen" / "Exit fullscreen"

**Zoom indicator (bottom-left):**
- Shows current zoom level (e.g., "2.3x")
- Hidden at 1x, fades in/out
- Auto-hides after 1.5s of inactivity

### 6. Edge Cases

- **Fullscreen API not supported:** Button hidden, Enter key no-op
- **Video content:** Zoom/pan disabled (native controls handle this)
- **SVG content:** Zoom/pan works normally
- **Edit panel open:** Zoom works, pan constrained to not overlap panel on desktop

### 7. Accessibility

- Zoom level announced to screen readers on change
- Fullscreen state announced ("Entered fullscreen", "Exited fullscreen")
- No +/- keyboard zoom (avoids browser zoom conflicts)

### 8. Performance

- CSS `transform: scale() translate()` for zoom/pan (GPU accelerated)
- Throttle pinch/drag events to ~60fps via requestAnimationFrame
- No image re-rendering at different zoom levels

## State Additions to Lightbox Store

```javascript
// Fullscreen
isFullscreen: false,

// Zoom
zoomLevel: 1,
minZoom: 1,
maxZoom: 5,
panX: 0,
panY: 0,
zoomIndicatorVisible: false,
zoomIndicatorTimeout: null,

// Pinch tracking
pinchStartDistance: null,
pinchStartZoom: null,
pinchCenterX: null,
pinchCenterY: null,

// Mouse drag tracking
isDragging: false,
dragStartX: null,
dragStartY: null,
dragStartPanX: null,
dragStartPanY: null,

// Navigation momentum
lastDragVelocity: 0,
lastDragTime: null,
```

## Files to Modify

1. `src/components/lightbox.js` - Add state, methods for zoom/pan/fullscreen
2. `templates/partials/lightbox.tpl` - Add fullscreen button, zoom indicator, gesture bindings, transform styles
