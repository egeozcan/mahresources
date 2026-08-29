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
	"time"

	"github.com/spf13/afero"

	"mahresources/models"
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
	var resource models.Resource
	if err := ctx.db.First(&resource, resourceId).Error; err != nil {
		return nil, err
	}

	fs, err := ctx.GetFsForStorageLocation(resource.StorageLocation)
	if err != nil {
		return nil, err
	}
	path, cleanup, err := localOrTempPath(fs, resource.GetCleanLocation())
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if reqCtx == nil {
		reqCtx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(reqCtx, mediaProbeTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(probeCtx, ctx.ffprobePath(),
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if probeCtx.Err() != nil {
			return nil, fmt.Errorf("probing the file timed out after %s", mediaProbeTimeout)
		}
		return nil, fmt.Errorf("could not probe the file: %w (ffprobe: %s)", err, truncateStderr(stderr.String(), 500))
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
	path, cleanup, err := localOrTempPath(fs, resource.GetCleanLocation())
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if reqCtx == nil {
		reqCtx = context.Background()
	}

	var frame []byte
	lockAcquired, runErr := ctx.locks.VideoThumbnailGenerationLock.RunWithLockTimeout(
		resource.ID,
		orDefault(ctx.Config.VideoThumbnailLockTimeout, defaultVideoLockTimeout),
		orDefault(ctx.Config.VideoThumbnailTimeout, defaultVideoRunTimeout),
		func() error {
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
			cmd := exec.CommandContext(reqCtx, ctx.Config.FfmpegPath, args...)
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
		},
	)
	if !lockAcquired {
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
func localOrTempPath(fs afero.Fs, location string) (string, func(), error) {
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
	if _, err := io.Copy(tmp, src); err != nil {
		cleanup()
		return "", noop, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", noop, err
	}
	return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, nil
}
