// Video trimmer Alpine component with dual-range slider.
//
// Provides a visual timeline slider with draggable start (green) and end (red) thumbs,
// plus text inputs for precise times. Submits via POST to /v1/resources/trim.

export function videoTrimmer({ resourceId, videoDuration = 0 }) {
  const duration = parseFloat(videoDuration) || 0;

  return {
    resourceId,
    duration,
    start: duration > 0 ? 0 : null,
    end: duration > 0 ? duration : null,
    startText: '0',
    endText: duration > 0 ? duration.toFixed(1) : '',
    comment: '',
    isSubmitting: false,
    errorMessage: '',
    _validationError: '',

    // Validation computed reactively on every input change
    updateValidation() {
      const s = parseFloat(this.startText);
      const e = parseFloat(this.endText);
      if (isNaN(s) || s < 0) {
        this._validationError = 'Start must be a non-negative number.';
      } else if (isNaN(e) || e <= 0) {
        this._validationError = 'End must be a positive number.';
      } else if (e <= s) {
        this._validationError = 'End must be after start.';
      } else {
        this._validationError = '';
      }
    },

    get validationError() {
      return this._validationError || '';
    },

    // -- slider helpers --

    init() {
      this.updateValidation();
    },

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
    },

    syncFromSlider() {
      this.startText = this.start.toFixed(1);
      this.endText = this.end.toFixed(1);
      this.updateValidation();
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
      // If we have a duration, use slider state
      if (this.duration > 0) {
        return this.end > this.start && this.start >= 0;
      }
      // Otherwise check text inputs
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
