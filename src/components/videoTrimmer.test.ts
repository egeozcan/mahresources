import { describe, expect, test, vi, beforeEach } from 'vitest';
import { videoTrimmer } from './videoTrimmer.js';

/**
 * The trim preview, and the slider that comes with it.
 *
 * The slider was never removed — it is gated on `x-show="duration > 0"`, and
 * `duration` came from `ProbeVideoDuration`, which needs a local filesystem and
 * a working ffprobe. It answers 0 otherwise, so the slider silently did not
 * render on every memory-fs deployment: every `-ephemeral` run, the e2e harness,
 * any demo. The preview is what fixes that, because the media element knows its
 * own duration, so `adoptMetadata` is the load-bearing line in this file.
 *
 * The rest is what makes a preview a preview rather than a second video: it
 * plays the range the trim will keep and stops where the trim will stop, it
 * follows the selection by seeking the start, and it does neither while the
 * reader is watching something by hand.
 */

let component: any;
let player: any;

function fakePlayer(overrides: Record<string, any> = {}) {
    return {
        duration: 0,
        currentTime: 0,
        paused: true,
        play: vi.fn(() => Promise.resolve()),
        pause: vi.fn(),
        ...overrides,
    };
}

function timeUpdate(currentTime: number) {
    return { target: { currentTime } };
}

beforeEach(() => {
    player = fakePlayer({ duration: 6.083333 });
    // No videoDuration from the server: the case this was written for.
    component = videoTrimmer({ resourceId: 7 });
    component.$refs = { player };
});

describe('adopting the duration from the preview', () => {
    test('takes it from the media element when the server had none', () => {
        expect(component.duration).toBe(0);

        component.adoptMetadata({ target: { duration: 6.083333 } });

        expect(component.duration).toBe(6.083333);
    });

    test('completes an untouched range, which is what the slider needs', () => {
        component.adoptMetadata({ target: { duration: 6.083333 } });

        // Both ends, not one: a half-seeded range gives the start thumb no
        // aria-valuenow, which axe reports as a critical violation the moment
        // the slider becomes visible, and leaves the preview with nowhere to
        // seek to.
        expect(component.start).toBe(0);
        expect(component.startText).toBe('0.0');
        expect(component.end).toBe(6.083333);
        expect(component.endText).toBe('6.1');
        // The whole video selected by default is a range, so the trim button is
        // live the moment the metadata lands rather than after a correction.
        expect(component.validationError).toBe('');
        expect(component.hasTimes()).toBe(true);
    });

    test('never clobbers a range the reader already dialled in', () => {
        // Metadata can land after they have started typing.
        component.startText = '1';
        component.endText = '3';
        component.syncFromText('end');

        component.adoptMetadata({ target: { duration: 6.083333 } });

        expect(component.start).toBe(1);
        expect(component.end).toBe(3);
        expect(component.duration).toBe(6.083333);
    });

    test('a probed duration wins, and the element cannot raise it', () => {
        const probed = videoTrimmer({ resourceId: 7, videoDuration: 120 });
        probed.$refs = { player: fakePlayer({ duration: 6 }) };

        probed.adoptMetadata({ target: { duration: 6 } });

        expect(probed.duration).toBe(120);
    });

    test('ignores a missing, zero, infinite or negative duration', () => {
        for (const reported of [0, NaN, Infinity, -1, undefined]) {
            component.adoptMetadata({ target: { duration: reported } });
            expect(component.duration).toBe(0);
        }
    });

    test('a live element with no loaded metadata changes nothing', () => {
        component.adoptMetadata({ target: { duration: NaN } });

        expect(component.duration).toBe(0);
        expect(component.start).toBeNull();
        expect(component.end).toBeNull();
    });
});

describe('previewing the range', () => {
    beforeEach(() => {
        component.adoptMetadata({ target: { duration: 6.083333 } });
    });

    test('plays from the start of the range', () => {
        component.startText = '1';
        component.endText = '3';
        component.syncFromText('end');
        player.currentTime = 0;

        component.previewRange();

        expect(player.currentTime).toBe(1);
        expect(player.play).toHaveBeenCalled();
        expect(component.previewing).toBe(true);
    });

    test('stops where the trim would stop, and rewinds to the start', () => {
        component.startText = '1';
        component.endText = '3';
        component.syncFromText('end');
        component.previewRange();

        component.onTimeUpdate(timeUpdate(2.5));
        expect(player.pause).not.toHaveBeenCalled();

        component.onTimeUpdate(timeUpdate(3));
        expect(player.pause).toHaveBeenCalled();
        // Back to the start, so pressing Preview again replays the same range
        // instead of resuming a quarter of a second in.
        expect(player.currentTime).toBe(1);
        expect(component.previewing).toBe(false);
    });

    test('does not stop a reader watching the video by hand', () => {
        // `previewing` is false for the native controls, and the trim point must
        // not become the dialog's opinion about when their playback ends.
        component.startText = '1';
        component.endText = '3';
        component.syncFromText('end');

        component.onTimeUpdate(timeUpdate(4));

        expect(player.pause).not.toHaveBeenCalled();
        expect(component.previewing).toBe(false);
    });

    test('a refused play does not leave the button reading Stop', () => {
        player.play = vi.fn(() => Promise.reject(new Error('NotAllowedError')));

        component.previewRange();
        return Promise.resolve().then(() => {
            expect(component.previewing).toBe(false);
        });
    });

    test('toggle is idempotent in both directions', () => {
        component.togglePreview();
        expect(component.previewing).toBe(true);
        component.togglePreview();
        expect(component.previewing).toBe(false);
        expect(player.pause).toHaveBeenCalledTimes(1);
    });

    test('no player, no preview', () => {
        component.$refs = {};
        component.previewRange();
        expect(player.play).not.toHaveBeenCalled();
    });
});

describe('the preview following the selection', () => {
    beforeEach(() => {
        component.adoptMetadata({ target: { duration: 6.083333 } });
    });

    test('typing a start seeks the paused preview to it', () => {
        component.startText = '2';
        component.endText = '5';
        component.syncFromText('end');

        expect(player.currentTime).toBe(2);
    });

    test('leaves the video alone while it is playing', () => {
        // A preview that yanks the video away from someone watching it is worse
        // than one that lags.
        player.paused = false;
        player.currentTime = 4;

        component.startText = '2';
        component.endText = '5';
        component.syncFromText('end');

        expect(player.currentTime).toBe(4);
    });

    test('a seek the player refuses is not an error', () => {
        Object.defineProperty(player, 'currentTime', {
            get: () => 0,
            set: () => {
                throw new Error('no seekable range');
            },
            configurable: true,
        });

        expect(() => {
            component.startText = '2';
            component.endText = '5';
            component.syncFromText('end');
        }).not.toThrow();
    });

    test('an unset start does not seek', () => {
        component.$refs = { player };
        component.duration = 10;
        component.start = null;
        component.end = 10;

        component.seekPreview();

        expect(player.currentTime).toBe(0);
    });
});

describe('the button and the message agree', () => {
    // `hasTimes()` used to prefer the slider state whenever a duration was
    // known, and `submit()` posts the text fields. Clearing End therefore left
    // a live button under a "End time must be a positive number" message, and a
    // known duration made that the normal case rather than the rare one.
    beforeEach(() => {
        component.adoptMetadata({ target: { duration: 6.083333 } });
    });

    test('a cleared End disables the trim button', () => {
        component.startText = '5';
        component.endText = '';

        expect(component.validationError).toBe('End time must be a positive number.');
        expect(component.hasTimes()).toBe(false);
    });

    test('an End before Start disables it', () => {
        component.startText = '4';
        component.endText = '2';

        expect(component.validationError).toBe('End must be after start.');
        expect(component.hasTimes()).toBe(false);
    });

    test('what a drag wrote into the text is what the button reads', () => {
        // Dragging keeps the two in step, so following the text cannot
        // contradict the slider the reader just used.
        component.start = 2;
        component.end = 4;
        component.syncFromSlider();

        expect(component.hasTimes()).toBe(true);
        expect(component.validationError).toBe('');
    });
});

describe('marking the range off the playhead', () => {
    beforeEach(() => {
        component.adoptMetadata({ target: { duration: 6.083333 } });
    });

    test('Set Start takes the current time', () => {
        player.currentTime = 2.5;

        component.markCurrentTime('start');

        expect(component.start).toBe(2.5);
        expect(component.startText).toBe('2.5');
    });

    test('Set End takes the current time', () => {
        // The order a reader marks in: set the start, then scrub the preview,
        // then mark the end. Typing the start moves the paused preview to it,
        // so setting the playhead before that would be marked instead.
        component.startText = '1';
        component.syncFromText('start');
        expect(player.currentTime).toBe(1);
        player.currentTime = 4;

        component.markCurrentTime('end');

        expect(component.end).toBe(4);
        expect(component.endText).toBe('4.0');
    });

    test('both text fields are rewritten, because the slider is a view of them', () => {
        player.currentTime = 3;

        component.markCurrentTime('start');

        // A range that disagrees between the two representations is the thing
        // hasTimes() just stopped trusting.
        expect(component.endText).toBe(component.end.toFixed(1));
        expect(component.startText).toBe(component.start.toFixed(1));
    });

    test('marking a start past the end pushes the end along', () => {
        // The same rule typing a start past the end gets, because it goes
        // through syncFromText rather than a second implementation.
        component.startText = '1';
        component.syncFromText('start');
        player.currentTime = 5;

        component.markCurrentTime('start');

        expect(component.start).toBe(5);
        expect(component.end).toBeGreaterThan(component.start);
        expect(component.validationError).toBe('');
    });

    test('marking an end before the start pulls the start back', () => {
        component.startText = '4';
        component.syncFromText('start');
        player.currentTime = 1;

        component.markCurrentTime('end');

        expect(component.end).toBe(1);
        expect(component.start).toBeLessThan(component.end);
        expect(component.validationError).toBe('');
    });

    test('the end of a video is not a place a trim can cut at', () => {
        player.currentTime = 999;

        component.markCurrentTime('end');

        expect(component.end).toBe(6.083333);
    });

    test('nothing happens before the element has metadata', () => {
        const fresh = videoTrimmer({ resourceId: 7 });
        fresh.$refs = { player: fakePlayer({ duration: 0, currentTime: 3 }) };

        expect(fresh.canMarkTime).toBe(false);
        fresh.markCurrentTime('start');

        // Marking 0.0 because the playhead is nowhere would look like it worked.
        expect(fresh.start).toBeNull();
        expect(fresh.startText).toBe('0');
    });

    test('a missing player is not an error', () => {
        // A known duration, no element: the dialog can be mid-teardown, and
        // marking must not throw or write a mark it could not have read.
        component.$refs = {};
        expect(component.canMarkTime).toBe(true);
        expect(() => component.markCurrentTime('start')).not.toThrow();
        expect(component.start).toBe(0);
    });

    test('canMarkTime follows the reactive duration, not the element', () => {
        // This is what a unit test cannot see: `canMarkTime` used to read
        // $refs.player.duration, which Alpine has no reason to recompute for, so
        // `:disabled="!canMarkTime"` evaluated once at first render and the
        // buttons stayed disabled for the life of the dialog. Reading this
        // reactive property is the fix, and the e2e that clicks the buttons is
        // what pins it.
        const fresh = videoTrimmer({ resourceId: 7 });
        const late = fakePlayer({ duration: 0 });
        fresh.$refs = { player: late };

        expect(fresh.canMarkTime).toBe(false);
        late.duration = 6.083333;
        fresh.adoptMetadata({ target: late });
        expect(fresh.canMarkTime).toBe(true);
    });
});
