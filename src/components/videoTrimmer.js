// Video trimmer Alpine component with dual-range slider.
//
// Provides a visual timeline slider with draggable start (green) and end (red) thumbs,
// plus text inputs for precise times. Submits via POST to /v1/resources/trim.
//
// The sidebar group is one **Video Actions…** button and a popup holding the
// trimmer, on the shared rules in sidebarPopup.js.
//
// The component stays mounted with the popup closed, which is why the x-data
// root is the sidebar group and only the UI is behind the x-if. A trim in
// flight finishes and navigates whether or not the reader closes the dialog on
// the way, and the times they had dialled in are still there when they reopen
// it. The slider's track ref is read at pointer time, not at init, so it does
// not care that the element is not in the document while the popup is shut.

import { sidebarPopup } from './sidebarPopup.js';

export function videoTrimmer({ resourceId, videoDuration = 0 }) {
  const duration = parseFloat(videoDuration) || 0;

  return {
    ...sidebarPopup(),
    resourceId,
    duration,
    start: duration > 0 ? 0 : null,
    end: duration > 0 ? duration : null,
    startText: '0',
    endText: duration > 0 ? duration.toFixed(1) : '',
    comment: '',
    isSubmitting: false,
    errorMessage: '',
    // True only while the reader asked for the range preview, never while they
    // are scrubbing the video by hand. See onTimeUpdate().
    previewing: false,

    // -- slider helpers --

    get sliderStartPct() {
      if (!this.duration) return 0;
      return (this.start / this.duration) * 100;
    },
    get sliderEndPct() {
      if (!this.duration) return 100;
      return (this.end / this.duration) * 100;
    },
    get sliderRangePct() {
      return this.sliderEndPct - this.sliderStartPct;
    },

    formatTime(seconds) {
      const s = parseFloat(seconds) || 0;
      const m = Math.floor(s / 60);
      const sec = (s % 60).toFixed(1);
      return m > 0 ? `${m}:${sec.padStart(4, '0')}` : `${sec}s`;
    },

    syncFromText(direction) {
      const s = parseFloat(this.startText);
      const e = parseFloat(this.endText);
      if (!isNaN(s) && s >= 0) this.start = this.duration > 0 ? Math.min(s, this.duration) : s;
      if (!isNaN(e) && e > 0) this.end = this.duration > 0 ? Math.min(e, this.duration) : e;
      if (this.start >= this.end) {
        if (direction === 'start') this.end = this.start + 0.1;
        else this.start = Math.max(0, this.end - 0.1);
      }
      this.startText = this.start.toFixed(1);
      this.endText = this.end.toFixed(1);
      this.seekPreview();
    },

    syncFromSlider() {
      this.startText = this.start.toFixed(1);
      this.endText = this.end.toFixed(1);
    },

    // -- preview --

    /**
     * Whether the playhead can be marked at all.
     *
     * `this.duration` and not the element's own `duration`, because `$refs` is
     * not reactive: a getter that reads only the element is evaluated on the
     * first render and never again, so the buttons would compute `false` when
     * the dialog opened before the metadata landed and stay disabled forever.
     * `adoptMetadata` writes this, so it recomputes exactly when it should.
     */
    get canMarkTime() {
      return this.duration > 0;
    },

    /**
     * Set the start (or the end) to wherever the playhead is now.
     *
     * Written through the text field and `syncFromText` rather than assigned to
     * `this.start`, so the ordering rule and the text fields the request posts
     * keep exactly one implementation each. Marking a start past the end pushes
     * the end along, which is what typing a start past the end does, and the
     * time is clamped to the duration because the end of a video is not a place
     * a trim can cut at.
     */
    markCurrentTime(which) {
      const player = this.$refs.player;
      const at = player && player.currentTime;
      if (!this.canMarkTime || !isFinite(at) || at < 0) return;
      if (which === 'start') this.startText = at.toFixed(1);
      else this.endText = at.toFixed(1);
      this.syncFromText(which);
    },

    /**
     * Take the duration from the media element when the server could not.
     *
     * `ProbeVideoDuration` needs a local filesystem and a working ffprobe, and it
     * answers 0 otherwise — which is every memory-fs deployment, so every
     * `-ephemeral` run, the e2e harness and any demo. The slider is gated on a
     * known duration, so a 0 used to hide it with nothing on screen saying why.
     * The element knows its own duration (the same number ffprobe reports), it
     * costs nothing, and it is what the trim endpoint is going to be given.
     *
     * A range the reader has already dialled in is theirs: only an untouched end
     * is completed here, because the metadata can land after they have started.
     */
    adoptMetadata(event) {
      // The event's target is the player. Not `event.target.duration ||
      // player.duration`: 0 and NaN are what an element reports before its
      // metadata has landed, and a falsy fallback would treat "not ready yet" as
      // "ask the ref instead" — which is the same element, saying the same
      // thing.
      const source = (event && event.target) || this.$refs.player;
      const reported = source ? source.duration : undefined;
      if (this.duration || !reported || !isFinite(reported) || reported <= 0) return;
      this.duration = reported;
      // Both ends, not one. The constructor seeds them from the server's
      // duration, so with none both are null, and a half-seeded range is worse
      // than none: the start thumb would have no aria-valuenow (a critical
      // aria-required-attr violation the moment the slider becomes visible),
      // the preview would have nowhere to seek to, and `end > start` would be
      // comparing against null. Only untouched ends are completed — a range
      // the reader has already dialled in is theirs, and the metadata can land
      // after they have started.
      if (this.start === null || this.start === undefined) {
        this.start = 0;
        this.startText = '0.0';
      }
      if (this.end === null || this.end === undefined) {
        this.end = this.duration;
        this.endText = this.duration.toFixed(1);
      }
    },

    /**
     * Play the range the trim would keep, and stop where it would stop.
     *
     * A preview that runs past the end is not a preview: the question a trimmer
     * exists to answer is "what am I about to keep", and a full-length playback
     * does not answer it.
     */
    previewRange() {
      const player = this.$refs.player;
      if (!player) return;
      this.seekToStart(player);
      this.previewing = true;
      const started = player.play();
      // A play() the browser refuses (autoplay policy, no decoder) must not
      // leave the button reading "Stop" over a video that never moved.
      if (started && typeof started.catch === 'function') {
        started.catch(() => {
          this.previewing = false;
        });
      }
    },

    stopPreview() {
      const player = this.$refs.player;
      if (player) player.pause();
      this.previewing = false;
    },

    togglePreview() {
      if (this.previewing) this.stopPreview();
      else this.previewRange();
    },

    onTimeUpdate(event) {
      // Only the range preview stops at the end. Someone watching the whole
      // video through the native controls is not trim-scoped, and cutting them
      // off at the trim point would be the dialog deciding what they asked for.
      if (!this.previewing) return;
      const current = (event && event.target && event.target.currentTime) || 0;
      if (this.end === null || this.end === undefined || current < this.end) return;
      const player = this.$refs.player;
      this.stopPreview();
      this.seekToStart(player);
    },

    /**
     * Move the paused preview to the start of the range, so moving a handle
     * shows the frame it would cut from.
     *
     * Not while it is playing: a preview that yanks the video away from someone
     * watching it is worse than one that lags. And not on every `pointermove` —
     * that is one seek per mouse event, and a queue of them.
     */
    seekPreview() {
      const player = this.$refs.player;
      if (!player || !player.paused) return;
      if (this.start === null || this.start === undefined) return;
      this.seekToStart(player);
    },

    seekToStart(player) {
      if (!player || this.start === null || this.start === undefined) return;
      try {
        player.currentTime = Math.max(0, this.start);
      } catch (_) {
        // Not seekable yet (no metadata, or a stream that cannot seek). The
        // slider still works; the preview just starts where it can.
      }
    },

    // -- slider thumb drag --

    startDrag: null, // 'start' | 'end' | null

    getTrackRect() {
      return this.$refs.sliderTrack ? this.$refs.sliderTrack.getBoundingClientRect() : null;
    },

    pctFromEvent(event) {
      const rect = this.getTrackRect();
      if (!rect || rect.width <= 0) return null;
      const px = Math.max(0, Math.min(rect.width, event.clientX - rect.left));
      return px / rect.width;
    },

    onThumbPointerDown(which, event) {
      if (event.button !== undefined && event.button !== 0) return;
      event.preventDefault();
      this.errorMessage = '';
      this.startDrag = which;
      if (event.target && event.target.setPointerCapture && event.pointerId !== undefined) {
        try { event.target.setPointerCapture(event.pointerId); } catch (_) { /* ignore */ }
      }
    },

    onTrackPointerDown(event) {
      if (event.button !== undefined && event.button !== 0) return;
      const pct = this.pctFromEvent(event);
      if (pct === null) return;

      const pos = pct * this.duration;
      // Move whichever thumb is closer to the click
      const distStart = Math.abs(this.start - pos);
      const distEnd = Math.abs(this.end - pos);
      if (distStart <= distEnd) {
        this.start = Math.max(0, Math.min(pos, this.end - 0.1));
        this.startDrag = 'start';
      } else {
        this.end = Math.min(this.duration, Math.max(pos, this.start + 0.1));
        this.startDrag = 'end';
      }
      this.syncFromSlider();

      if (event.target && event.target.setPointerCapture && event.pointerId !== undefined) {
        try { event.target.setPointerCapture(event.pointerId); } catch (_) { /* ignore */ }
      }
    },

    onPointerMove(event) {
      if (!this.startDrag || !this.duration) return;
      const pct = this.pctFromEvent(event);
      if (pct === null) return;
      const pos = pct * this.duration;
      if (this.startDrag === 'start') {
        this.start = Math.max(0, Math.min(pos, this.end - 0.1));
      } else {
        this.end = Math.min(this.duration, Math.max(pos, this.start + 0.1));
      }
      this.syncFromSlider();
    },

    onPointerUp(event) {
      this.startDrag = null;
      if (event && event.target && event.target.releasePointerCapture && event.pointerId !== undefined) {
        try { event.target.releasePointerCapture(event.pointerId); } catch (_) { /* ignore */ }
      }
      // End of a drag, not every frame of one: the preview lands on the frame
      // the handle was released over.
      this.seekPreview();
    },

    // -- submit --

    get validationError() {
      const s = parseFloat(this.startText);
      const e = parseFloat(this.endText);
      if (isNaN(s) || s < 0) return 'Start time must be a non-negative number.';
      if (isNaN(e) || e <= 0) return 'End time must be a positive number.';
      if (e <= s) return 'End must be after start.';
      return '';
    },

    hasEndAfterStart() {
      const s = parseFloat(this.startText);
      const e = parseFloat(this.endText);
      return !isNaN(s) && !isNaN(e) && e > s && s >= 0;
    },

    hasTimes() {
      // The text fields, not the slider state — because the text fields are
      // what submit() posts, and the slider is a view of them: every drag
      // writes them through syncFromSlider() and every keystroke syncs the
      // slider back through syncFromText(). Preferring the slider meant the
      // button could be live while the message under it read "End time must be
      // a positive number", which is reachable by anyone who clears End, and
      // which became common once a known duration stopped the slider hiding.
      return this.hasEndAfterStart();
    },

    async submit() {
      if (this.isSubmitting) return;
      if (!this.hasTimes()) {
        this.errorMessage = 'Start must be before end.';
        return;
      }
      this.errorMessage = '';
      this.isSubmitting = true;
      try {
        const body = new URLSearchParams();
        body.set('id', String(this.resourceId));
        body.set('start', this.startText.trim());
        body.set('end', this.endText.trim());
        if (this.comment && this.comment.trim()) body.set('comment', this.comment.trim());

        const response = await fetch('/v1/resources/trim', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            'Accept': 'application/json',
          },
          body: body.toString(),
        });

        if (!response.ok) {
          let message = `Trim failed (HTTP ${response.status})`;
          try {
            const data = await response.json();
            if (data.error) message = data.error;
          } catch (_) { /* ignore */ }
          this.errorMessage = message;
          this.isSubmitting = false;
          return;
        }

        window.location.reload();
      } catch (err) {
        this.errorMessage = err && err.message ? err.message : 'Trim failed.';
        this.isSubmitting = false;
      }
    },
  };
}
