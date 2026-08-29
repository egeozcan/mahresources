package application_context

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"mahresources/models"
	"mahresources/models/query_models"
)

// mah.media against a real PluginManager, a real database and real ffmpeg. The
// interesting property is not that ffprobe works -- it is that a plugin reaches
// these files only through the handle its caller's principal is bound to, so a
// confined caller cannot probe or cut what it cannot see.

func mediaTestFfmpeg(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed; this test probes a real video")
	}
	return p
}

// tinyVideo returns two seconds of encoded MP4.
func tinyVideo(t *testing.T, ffmpeg string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "v.mp4")
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-movflags", "+faststart", "-y", out,
	)
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the test video: %v\n%s", err, o)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// putVideo writes real video bytes into the context's filesystem and records a
// resource pointing at them.
func putVideo(t *testing.T, ctx *MahresourcesContext, name string, owner *models.Group, data []byte) *models.Resource {
	t.Helper()
	location := name + ".mp4"
	if err := afero.WriteFile(ctx.fs, location, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", location, err)
	}
	res := &models.Resource{
		Name:        name,
		Hash:        "hash-" + name,
		Location:    location,
		ContentType: "video/mp4",
		FileSize:    int64(len(data)),
	}
	if owner != nil {
		res.OwnerId = &owner.ID
	}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource %s: %v", name, err)
	}
	return res
}

const mediaProbePlugin = `plugin = { name = "media", version = "1.0", api_version = 1,
           capabilities = { "db:read", "media", "hooks", "inject" } }
local report = ""
function init()
    mah.on("after_note_create", function(data)
        local target = tonumber(data.description) or 0
        local info, err = mah.media.probe(target)
        if not info then
            report = "error:" .. tostring(err)
        else
            report = "ok:" .. tostring(info.format and info.format.format_name or "?")
                     .. ":streams=" .. tostring(#(info.streams or {}))
        end
        return data
    end)
    mah.inject("page_top", function(c) return report end)
end
`

func TestPluginMediaProbeReadsTheFile(t *testing.T) {
	ffmpeg := mediaTestFfmpeg(t)
	ctx := newTwoPluginContext(t, map[string]string{"media": mediaProbePlugin})
	ctx.Config.FfmpegPath = ffmpeg

	res := putVideo(t, ctx, "clip", nil, tinyVideo(t, ffmpeg))

	if _, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "trigger", Description: itoa(res.ID)},
	}); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	out := runSlot(ctx, "page_top")
	if !strings.HasPrefix(out, "ok:") {
		t.Fatalf("probe reported %q, want the ffprobe document", out)
	}
	if !strings.Contains(out, "mp4") {
		t.Errorf("probe reported %q, want a format naming mp4", out)
	}
	// The nesting is what makes probe worth having: `streams` is a list, and a
	// flattened summary would have to choose which stream's codec to report.
	if strings.Contains(out, "streams=0") {
		t.Errorf("probe reported %q, want the stream list", out)
	}
}

// TestPluginMediaIsBoundToTheCallersScope is the property the whole design
// rests on. mah.media reaches the file through the same handle mah.db does, so
// a caller confined to a subtree cannot probe a resource outside it -- and that
// follows from the binding rather than from a check in the media code.
func TestPluginMediaIsBoundToTheCallersScope(t *testing.T) {
	ffmpeg := mediaTestFfmpeg(t)
	ctx := newTwoPluginContext(t, map[string]string{"media": mediaProbePlugin})
	ctx.Config.FfmpegPath = ffmpeg

	principal, inside := scopeProbeFixture(t, ctx)
	video := tinyVideo(t, ffmpeg)
	outsideGroup := &models.Group{Name: "far-away"}
	if err := ctx.db.Create(outsideGroup).Error; err != nil {
		t.Fatal(err)
	}
	insideRes := putVideo(t, ctx, "in-subtree", inside, video)
	outsideRes := putVideo(t, ctx, "out-of-subtree", outsideGroup, video)

	scoped := ctx.WithPrincipal(principal)

	probeAs := func(target uint) string {
		t.Helper()
		if _, err := scoped.CreateOrUpdateNote(&query_models.NoteEditor{
			NoteCreator: query_models.NoteCreator{
				Name: "trigger", OwnerId: inside.ID, Description: itoa(target),
			},
		}); err != nil {
			t.Fatalf("confined user could not create a note in its own subtree: %v", err)
		}
		return runSlot(ctx, "page_top")
	}

	// The control: inside the subtree the same call succeeds, so a refusal
	// below is about scope rather than about the plugin never working.
	if got := probeAs(insideRes.ID); !strings.HasPrefix(got, "ok:") {
		t.Fatalf("probing a resource inside the caller's own subtree reported %q", got)
	}

	if got := probeAs(outsideRes.ID); strings.HasPrefix(got, "ok:") {
		t.Fatalf("a caller confined to %q probed a resource outside it: %q", inside.Name, got)
	}
}

func TestPluginMediaExtractsAFrameAsADataURI(t *testing.T) {
	ffmpeg := mediaTestFfmpeg(t)
	ctx := newTwoPluginContext(t, map[string]string{"media": `
plugin = { name = "media", version = "1.0", api_version = 1,
           capabilities = { "db:read", "media", "hooks", "inject" } }
local report = ""
function init()
    mah.on("after_note_create", function(data)
        local uri, err = mah.media.extract_frame(tonumber(data.description) or 0, 1.0, 64)
        if not uri then
            report = "error:" .. tostring(err)
        else
            report = "len=" .. tostring(#uri) .. " prefix=" .. string.sub(uri, 1, 22)
        end
        return data
    end)
    mah.inject("page_top", function(c) return report end)
end
`})
	ctx.Config.FfmpegPath = ffmpeg

	res := putVideo(t, ctx, "clip", nil, tinyVideo(t, ffmpeg))
	if _, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "trigger", Description: itoa(res.ID)},
	}); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	out := runSlot(ctx, "page_top")
	// The shape mah.image already speaks, so a frame composes with it directly.
	if !strings.Contains(out, "prefix=data:image/jpeg;base64") {
		t.Fatalf("extract_frame reported %q, want a JPEG data URI", out)
	}
	if strings.Contains(out, "len=0") {
		t.Error("extract_frame returned an empty URI")
	}
}

// TestPluginMediaTrimIsRefusedInsideATransaction. A trim runs ffmpeg over the
// whole clip and writes a version; inside a transaction that is the database's
// write lock held for the length of a transcode.
func TestPluginMediaTrimIsRefusedInsideATransaction(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{"media": `
plugin = { name = "media", version = "1.0", api_version = 1,
           capabilities = { "db:write", "media", "hooks", "inject" } }
local report = "not-run"
function init()
    mah.on("after_note_create", function(data)
        mah.db.transaction(function()
            local ok, err = mah.media.trim(tonumber(data.description) or 0, "0", "1", "clip")
            if ok then report = "trimmed" else report = tostring(err) end
        end)
        return data
    end)
    mah.inject("page_top", function(c) return report end)
end
`})

	if _, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "trigger", Description: "1"},
	}); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	out := runSlot(ctx, "page_top")
	if out == "trimmed" || out == "not-run" {
		t.Fatalf("trim inside a transaction reported %q, want a refusal", out)
	}
	if !strings.Contains(out, "transaction") {
		t.Errorf("the refusal %q does not say what is wrong", out)
	}
}

func itoa(v uint) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}
