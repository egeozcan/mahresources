package application_context

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/afero"

	"mahresources/hls"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The media operations mah.media is built on: read what a file contains, and
// take a frame out of it. Both are host work by necessity -- the plugin VM has
// no filesystem and no process execution, deliberately -- and both are shaped
// so that what a plugin names is a resource id, never a path and never an
// ffmpeg argument.
//
// Every one of them reads the resource through ctx, which for a plugin call is
// bound to the acting principal. A group-limited caller therefore cannot probe
// or cut a resource outside its subtree, and that is a property of the handle
// rather than of a check somebody has to remember to write.

// mediaProbeTimeout bounds one ffprobe invocation. Probing reads headers, so a
// file that takes longer than this is pathological rather than large.
const mediaProbeTimeout = 30 * time.Second

// maxFrameWidth caps what a plugin may ask for when extracting a frame. The
// frame comes back as a base64 data URI through the Lua VM, and an 8K still is
// tens of megabytes of string.
const maxFrameWidth = 4096

// ProbeMedia returns what ffprobe reports about a resource: its format and its
// streams, as the nested structure ffprobe itself produces.
//
// The whole document is returned rather than a curated selection because the
// useful field varies by caller -- duration, codec, rotation, channel layout,
// frame rate -- and a curated list is a list somebody has to keep extending.
func (ctx *MahresourcesContext) ProbeMedia(reqCtx context.Context, resourceId uint) (map[string]any, error) {
	// Before the resource read, the gate and -- on a filesystem with no local
	// path -- a copy of the whole file. A server with no ffprobe should say so
	// rather than spend all of that and return an exec error naming a binary
	// the caller never chose.
	if _, err := exec.LookPath(ctx.ffprobePath()); err != nil {
		return nil, fmt.Errorf("%w (ffprobe was not found either)", ErrFfmpegUnavailable)
	}

	var resource models.Resource
	if err := ctx.db.First(&resource, resourceId).Error; err != nil {
		return nil, err
	}

	fs, err := ctx.GetFsForStorageLocation(resource.StorageLocation)
	if err != nil {
		return nil, err
	}

	if reqCtx == nil {
		reqCtx = context.Background()
	}

	var stdout, stderr bytes.Buffer
	// Through the same gate as every other ffmpeg-family call on a resource.
	// Probing is cheap per call and a plugin can make it in a loop, so an
	// ungated probe is an unbounded number of concurrent processes reached
	// through the cheapest surface in the API.
	runErr, gated := ctx.runVideoTool(reqCtx, resource.ID, mediaProbeTimeout, func(runCtx context.Context) error {
		// Inside the gate, and under its deadline: on a filesystem with no
		// local path this copies the whole file, and doing that before taking a
		// slot meant an unbounded number of concurrent copies, each of which
		// could outlive the caller's own budget.
		path, cleanup, err := localOrTempPath(runCtx, fs, resource.GetCleanLocation())
		if err != nil {
			return err
		}
		defer cleanup()

		cmd := exec.CommandContext(runCtx, ctx.ffprobePath(),
			"-v", "error",
			"-print_format", "json",
			"-show_format",
			"-show_streams",
			path,
		)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			if runCtx.Err() != nil {
				return fmt.Errorf("probing the file timed out after %s", mediaProbeTimeout)
			}
			return fmt.Errorf("could not probe the file: %w (ffprobe: %s)", err, truncateStderr(stderr.String(), 500))
		}
		return nil
	})
	if !gated {
		return nil, errors.New("timed out waiting for a video processing slot")
	}
	if runErr != nil {
		return nil, runErr
	}

	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("could not read what ffprobe reported: %w", err)
	}
	return out, nil
}

// ExtractVideoFrame returns a single frame of a video as JPEG bytes.
//
// It is a sibling of createThumbFromVideoFileAtTime rather than a caller of it:
// that one exists to make a thumbnail, so it rounds the timestamp to a whole
// second and scales to a fixed 640px. Both are right for a thumbnail and wrong
// for an API whose caller asked for a particular moment.
//
// The video thumbnail lock and its timeout are shared with thumbnail
// generation, so plugins cannot start more concurrent ffmpeg processes than the
// deployment already allows for the same work.
func (ctx *MahresourcesContext) ExtractVideoFrame(reqCtx context.Context, resourceId uint, atSeconds float64, maxWidth int) ([]byte, error) {
	if atSeconds < 0 {
		return nil, errors.New("the timestamp cannot be negative")
	}
	if maxWidth < 0 || maxWidth > maxFrameWidth {
		return nil, fmt.Errorf("the frame width must be between 1 and %d", maxFrameWidth)
	}
	if maxWidth == 0 {
		// "The video's own size" still has a ceiling. Without this the default
		// -- the value a caller passes by saying nothing -- was the one case
		// that could return a 8K still as a base64 string, which is the exact
		// thing the cap exists to prevent. The scale filter never upscales, so
		// this changes nothing for anything smaller.
		maxWidth = maxFrameWidth
	}
	// Refused above the resource read and the lock, so a request that cannot
	// succeed neither reads the file nor holds the lock while failing. Same
	// order as TrimVideo, for the same reason.
	if err := ctx.requireFfmpeg(); err != nil {
		return nil, fmt.Errorf("cannot extract a frame: %w", err)
	}

	var resource models.Resource
	if err := ctx.db.First(&resource, resourceId).Error; err != nil {
		return nil, err
	}
	if !resource.IsVideo() {
		return nil, errors.New("resource is not a video")
	}

	fs, err := ctx.GetFsForStorageLocation(resource.StorageLocation)
	if err != nil {
		return nil, err
	}

	if reqCtx == nil {
		reqCtx = context.Background()
	}

	var frame []byte
	runErr, gated := ctx.runVideoTool(reqCtx, resource.ID,
		orDefault(ctx.Config.VideoThumbnailTimeout, defaultVideoRunTimeout),
		func(runCtx context.Context) error {
			// Inside the gate: see ProbeMedia.
			path, cleanup, err := localOrTempPath(runCtx, fs, resource.GetCleanLocation())
			if err != nil {
				return err
			}
			defer cleanup()

			args := []string{
				"-nostdin",
				// Before -i: input seeking, which jumps rather than decoding
				// everything up to the timestamp.
				"-ss", fmt.Sprintf("%.3f", atSeconds),
				"-i", path,
				"-frames:v", "1",
			}
			if maxWidth > 0 {
				// -1 keeps the aspect ratio; min() leaves a smaller video alone
				// rather than upscaling it into a blurrier, larger frame.
				args = append(args, "-vf", fmt.Sprintf("scale='min(%d,iw)':-1", maxWidth))
			}
			args = append(args, "-c:v", "mjpeg", "-q:v", "3", "-f", "image2pipe", "pipe:1")

			var out, stderr bytes.Buffer
			cmd := exec.CommandContext(runCtx, ctx.Config.FfmpegPath, args...)
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("could not extract the frame: %w (ffmpeg: %s)", err, truncateStderr(stderr.String(), 500))
			}
			if out.Len() == 0 {
				// ffmpeg exits 0 when the seek lands past the end of the file,
				// having written nothing.
				return fmt.Errorf("there is no frame at %.3fs in this video", atSeconds)
			}
			frame = out.Bytes()
			return nil
		})
	if !gated {
		return nil, errors.New("timed out waiting for a video processing slot")
	}
	if runErr != nil {
		if errors.Is(runErr, context.DeadlineExceeded) {
			return nil, errors.New("extracting the frame timed out")
		}
		return nil, runErr
	}
	return frame, nil
}

// FrameDataURI is ExtractVideoFrame as the data URI mah.image already speaks,
// so a frame can be handed straight to it.
func (ctx *MahresourcesContext) FrameDataURI(reqCtx context.Context, resourceId uint, atSeconds float64, maxWidth int) (string, error) {
	frame, err := ctx.ExtractVideoFrame(reqCtx, resourceId, atSeconds, maxWidth)
	if err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(frame), nil
}

// runVideoTool runs one ffmpeg-family process under the deployment's video
// concurrency gate, with a deadline that actually reaches the process.
//
// The deadline is the part worth stating. RunWithLockTimeout's own run timeout
// bounds only how long *it* waits: it returns while the function keeps running,
// so an exec.CommandContext built on the caller's context would leave a wedged
// ffmpeg alive with nothing left holding a reference to it. The command is
// therefore built on a context derived here, so the timeout kills the process
// rather than merely abandoning it.
//
// gated is false when the lock could not be taken in time, which is "the server
// is busy", not "this failed".
func (ctx *MahresourcesContext) runVideoTool(reqCtx context.Context, resourceID uint, timeout time.Duration, fn func(context.Context) error) (err error, gated bool) {
	if reqCtx == nil {
		reqCtx = context.Background()
	}
	if !ctx.acquireVideoSlot(reqCtx, resourceID) {
		if err := reqCtx.Err(); err != nil {
			return err, true
		}
		return nil, false
	}
	defer ctx.locks.VideoThumbnailGenerationLock.Release(resourceID)

	// Started after the lock: waiting for a slot is not running. A deadline
	// taken outside would be spent by the queue in front of this call, so a
	// request that waited 25 of its 60 permitted seconds would give ffmpeg
	// five -- and one that waited the full lock timeout would hand it a
	// context already cancelled.
	runCtx, cancel := context.WithTimeout(reqCtx, timeout)
	defer cancel()
	return fn(runCtx), true
}

// trimVideoGated is TrimVideo under the deployment's video concurrency gate.
//
// TrimVideo takes only the per-resource version lock, so N concurrent trims of
// N different resources start N ffmpeg processes -- and unlike a probe or a
// frame, a trim is a full re-encode. That is the existing behaviour of the trim
// button, where a person is doing one at a time; a plugin loop is not, so the
// plugin's door takes the same global gate every other ffmpeg-family call here
// takes.
//
// The gate is taken *around* TrimVideo rather than inside it: this is the
// plugin surface's own bound, and putting it inside would change the HTTP
// endpoint's behaviour too. It cannot deadlock against the lock TrimVideo takes
// (VersionUploadLock) -- a different lock -- and nothing under TrimVideo takes
// this one, since thumbnails are generated lazily on request rather than at
// write time.
//
// No run timeout: a trim is a transcode, and the thirty seconds that bounds a
// frame extraction would refuse every clip longer than a few minutes. What is
// bounded is concurrency, and the caller's own context still cancels the work.
func (ctx *MahresourcesContext) trimVideoGated(reqCtx context.Context, resourceID uint, start, end, comment string) error {
	if !ctx.acquireVideoSlot(reqCtx, resourceID) {
		if err := reqCtx.Err(); err != nil {
			return err
		}
		return errors.New("timed out waiting for a video processing slot")
	}
	defer ctx.locks.VideoThumbnailGenerationLock.Release(resourceID)
	return ctx.TrimVideo(reqCtx, resourceID, start, end, comment)
}

// TrimVideoToNewResource cuts a clip and files it as a resource of its own,
// leaving the source untouched.
//
// The other half of what a plugin can ask for. TrimVideo replaces the
// resource's current content with the clip, which is right when the clip *is*
// the thing you wanted; it is wrong when the source is a two-hour recording you
// are pulling five clips out of, and a plugin doing that would otherwise have
// to trim, read the version back, create a resource from it and restore the
// original.
//
// The clip inherits the source's owner group and storage location, because a
// clip belongs where its source does. It does *not* copy the source's other
// group memberships or its tags: those describe the whole recording, and a
// caller who wants them can add them to the resource this hands back.
func (ctx *MahresourcesContext) TrimVideoToNewResource(reqCtx context.Context, resourceID uint, start, end, name string) (*models.Resource, error) {
	startSec, endSec, err := parseTrimRange(start, end)
	if err != nil {
		return nil, err
	}
	// Above the resource read and the gate, so a request that cannot succeed
	// neither reads the file nor holds a slot while failing.
	if err := ctx.requireFfmpeg(); err != nil {
		return nil, fmt.Errorf("cannot trim video: %w", err)
	}

	var resource models.Resource
	if err := ctx.db.First(&resource, resourceID).Error; err != nil {
		return nil, err
	}
	if !resource.IsVideo() {
		return nil, errors.New("resource must be a video")
	}

	if !ctx.acquireVideoSlot(reqCtx, resourceID) {
		if err := reqCtx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("timed out waiting for a video processing slot")
	}
	defer ctx.locks.VideoThumbnailGenerationLock.Release(resourceID)

	clip, err := ctx.trimVideoBytes(reqCtx, &resource, start, endSec-startSec)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%s (%s-%s)", resource.Name, start, end)
	}
	// ffmpeg always writes an MP4 container here, whatever the source was, so
	// the name says mp4 rather than inheriting a .webm the bytes are not.
	name = hls.OutputName(TrimEntityName(name))

	query := &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:             name,
			OwnerId:          ownerOrZero(resource.OwnerId),
			OriginalName:     resource.OriginalName,
			OriginalLocation: resource.OriginalLocation,
		},
	}
	if resource.StorageLocation != nil {
		query.PathName = *resource.StorageLocation
	}
	return ctx.AddResource(io.NopCloser(bytes.NewReader(clip)), name, query)
}

func ownerOrZero(id *uint) uint {
	if id == nil {
		return 0
	}
	return *id
}

// cancelableReader stops a copy when its context ends. afero readers take no
// context, so this is the only place to ask.
type cancelableReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *cancelableReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// acquireVideoSlot takes the video gate, giving up when the caller does.
//
// idlock's wait takes no context, so this races it against the caller's own
// cancellation. Losing that race is not the end of it: the acquire may still
// succeed afterwards, so a watcher releases the slot it was handed rather than
// leaking one out of the pool for the rest of the process's life.
func (ctx *MahresourcesContext) acquireVideoSlot(reqCtx context.Context, resourceID uint) bool {
	if reqCtx.Err() != nil {
		return false
	}
	lock := ctx.locks.VideoThumbnailGenerationLock
	wait := ctx.videoLockWait(reqCtx)

	got := make(chan bool, 1)
	go func() { got <- lock.AcquireWithTimeout(resourceID, wait) }()

	select {
	case acquired := <-got:
		return acquired
	case <-reqCtx.Done():
		go func() {
			if <-got {
				lock.Release(resourceID)
			}
		}()
		return false
	}
}

// videoLockWait is how long to wait for a video slot, never longer than the
// caller has left.
//
// The lock's own wait takes no context, so a cancelled request -- or an async
// job whose deadline has passed -- would otherwise sit in it for the full
// configured timeout, doing nothing anybody is waiting for. Under contention
// that is a minute a schedule handler spends outside its own budget, and the
// claim TTL that bounds a schedule is derived from budgets like it.
func (ctx *MahresourcesContext) videoLockWait(reqCtx context.Context) time.Duration {
	wait := orDefault(ctx.Config.VideoThumbnailLockTimeout, defaultVideoLockTimeout)
	deadline, ok := reqCtx.Deadline()
	if !ok {
		return wait
	}
	if remaining := time.Until(deadline); remaining < wait {
		if remaining < 0 {
			return 0
		}
		return remaining
	}
	return wait
}

// The fallbacks for a context built from a bare config, which is what the CLI
// and every test construct. Zero there means "nobody said", and passing it
// through as a timeout would mean "already expired" -- every frame refused,
// with a message about a timeout nobody configured. They match the documented
// defaults of -video-thumb-timeout and -video-thumb-lock-timeout.
const (
	defaultVideoRunTimeout  = 30 * time.Second
	defaultVideoLockTimeout = 60 * time.Second
)

func orDefault(v, fallback time.Duration) time.Duration {
	if v <= 0 {
		return fallback
	}
	return v
}

// localOrTempPath gives ffmpeg and ffprobe a real file to open.
//
// Both need to seek, so a pipe is not a general substitute: an MP4 whose moov
// atom sits at the end cannot be read from stdin at all. On a local filesystem
// this hands back the file itself and cleanup does nothing; on any other
// (memory, or an alternative backend) it copies to a temp file, which costs a
// copy and always works.
func localOrTempPath(ctx context.Context, fs afero.Fs, location string) (string, func(), error) {
	noop := func() {}
	if path, ok := resolveLocalFilePath(fs, location); ok {
		return path, noop, nil
	}

	src, err := fs.Open(location)
	if err != nil {
		return "", noop, err
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "mahresources-media-")
	if err != nil {
		return "", noop, err
	}
	cleanup := func() {
		tmp.Close()
		_ = os.Remove(tmp.Name())
	}
	// Through the context, so a caller whose budget runs out mid-copy stops
	// rather than finishing a copy nobody is waiting for any more.
	if _, err := io.Copy(tmp, &cancelableReader{ctx: ctx, r: src}); err != nil {
		cleanup()
		return "", noop, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", noop, err
	}
	return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, nil
}
