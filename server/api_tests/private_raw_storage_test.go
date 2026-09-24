package api_tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/server"
)

func TestRawFileMountsHidePrivateStorageFromAdminsAndEditors(t *testing.T) {
	tc := setupAuthEnv(t)

	adminCookie, _ := loginSummaryExportSession(t, tc, "admin", "adminpw1")
	editor, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "raw-private-editor", Password: "password1", Role: models.RoleEditor,
	})
	if err != nil {
		t.Fatalf("create editor: %v", err)
	}
	editorCookie, _ := loginSummaryExportSession(t, tc, editor.Username, "password1")

	fileSaveBase := t.TempDir()
	fileRoot := filepath.Join(fileSaveBase, "private-café")
	fileRootNormalizationAlias := filepath.Join(fileSaveBase, "private-cafe\u0301")
	stagingRoot := filepath.Join(fileRoot, "custom-staging")
	stagingAliasRoot := filepath.Join(fileRoot, "CUSTOM-STAGING")
	dataRoot := t.TempDir()
	exportsRoot := filepath.Join(fileRoot, "_exports")
	write := func(root, name string) {
		t.Helper()
		fullPath := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("create %s: %v", fullPath, err)
		}
		if err := os.WriteFile(fullPath, []byte("private fixture"), 0o600); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}
	write(fileRoot, jobs.JobReplayKeyFileName)
	write(fileRoot, "."+jobs.JobReplayKeyFileName+"-synthetic")
	write(fileRoot, "_JOB_REPLAY_KEY")
	write(fileRoot, "._JOB_REPLAY_KEY-SYNTHETIC")
	write(fileRoot, "_exports/.part")
	write(fileRoot, "_exports/alternate.tar")
	write(fileRoot, "_exports/ALTERNATE.TAR")
	write(fileRoot, "_EXPORTS/ALTERNATE.TAR")
	write(fileRoot, "_imports/.part")
	write(fileRoot, "_plugin_commands/run.json")
	write(fileRoot, "custom-staging/run.json")
	write(fileRoot, "CUSTOM-STAGING/RUN.JSON")
	write(fileRoot, ".part-summary.json")
	write(fileRoot, "public.txt")
	if _, err := os.Stat(fileRootNormalizationAlias); os.IsNotExist(err) {
		if err := os.Symlink(fileRoot, fileRootNormalizationAlias); err != nil {
			t.Fatalf("create normalization alias for private file root: %v", err)
		}
	}
	write(fileRootNormalizationAlias, jobs.JobReplayKeyFileName)
	write(fileRootNormalizationAlias, "."+jobs.JobReplayKeyFileName+"-synthetic")
	write(stagingAliasRoot, "RUN.JSON")
	write(dataRoot, jobs.JobReplayKeyFileName)
	write(dataRoot, "public.txt")

	tc.AppCtx.Config.FileSavePath = fileRoot
	tc.AppCtx.Config.PluginCommandStagingPath = stagingRoot
	defaultStagingRoot := filepath.Join(fileRoot, "_plugin_commands")
	diskFS := afero.NewBasePathFs(afero.NewOsFs(), fileRoot)
	tc.Router = server.CreateServer(tc.AppCtx, diskFS, map[string]string{
		"commands":         stagingRoot,
		"staging-alias":    stagingAliasRoot,
		"default-commands": defaultStagingRoot,
		"data":             dataRoot,
		"exports":          exportsRoot,
		"parent":           filepath.Dir(fileRoot),
	}).Handler

	privatePaths := []string{
		"/files/" + jobs.JobReplayKeyFileName,
		"/files/." + jobs.JobReplayKeyFileName + "-synthetic",
		"/files/public/%2e%2e/" + jobs.JobReplayKeyFileName,
		"/files/_exports/.part",
		"/files/_imports/.part",
		"/files/_plugin_commands/run.json",
		"/files/.part-summary.json",
		"/commands/run.json",
		"/default-commands/run.json",
		"/data/_job_replay_key",
		"/exports/alternate.tar",
		"/parent/" + filepath.Base(fileRoot) + "/" + jobs.JobReplayKeyFileName,
		"/parent/" + filepath.Base(fileRoot) + "/_JOB_REPLAY_KEY",
		"/parent/" + filepath.Base(fileRoot) + "/." + jobs.JobReplayKeyFileName + "-synthetic",
		"/parent/" + filepath.Base(fileRoot) + "/._JOB_REPLAY_KEY-SYNTHETIC",
		"/parent/" + filepath.Base(fileRoot) + "/_EXPORTS/ALTERNATE.TAR",
		"/parent/" + filepath.Base(fileRootNormalizationAlias) + "/" + jobs.JobReplayKeyFileName,
		"/parent/" + filepath.Base(fileRootNormalizationAlias) + "/." + jobs.JobReplayKeyFileName + "-synthetic",
		"/files/CUSTOM-STAGING/RUN.JSON",
		"/staging-alias/RUN.JSON",
		"/exports/ALTERNATE.TAR",
	}
	for _, principal := range []struct {
		name   string
		cookie *http.Cookie
	}{{"administrator", adminCookie}, {"editor", editorCookie}} {
		for _, path := range privatePaths {
			response := doReq(tc, http.MethodGet, path, nil, []*http.Cookie{principal.cookie}, nil)
			if response.Code == http.StatusMovedPermanently || response.Code == http.StatusFound || response.Code == http.StatusTemporaryRedirect || response.Code == http.StatusPermanentRedirect {
				location := response.Header().Get("Location")
				if location == "" {
					t.Errorf("%s raw request %q redirected without a Location header", principal.name, path)
					continue
				}
				response = doReq(tc, http.MethodGet, location, nil, []*http.Cookie{principal.cookie}, nil)
			}
			if response.Code != http.StatusNotFound {
				t.Errorf("%s raw request %q: status=%d body=%q; want 404", principal.name, path, response.Code, response.Body.String())
			}
		}
		for _, path := range []string{"/files/public.txt", "/data/public.txt"} {
			response := doReq(tc, http.MethodGet, path, nil, []*http.Cookie{principal.cookie}, nil)
			if response.Code != http.StatusOK || response.Body.String() != "private fixture" {
				t.Errorf("%s public raw request %q: status=%d body=%q; want the public fixture", principal.name, path, response.Code, response.Body.String())
			}
		}
	}
}
