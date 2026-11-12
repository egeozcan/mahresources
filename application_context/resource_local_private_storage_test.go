package application_context

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The HTTP rule makes /v1/resource/local admin-only, but a caller below
// server/ never passes through it, so the operation answers the same question
// itself. An unbound handle (the offline importer) and the auth-off super-user
// keep working.
func TestAddLocalResource_RequiresAdminAtTheOperation(t *testing.T) {
	ctx := createTestContext(t)
	altFs := afero.NewMemMapFs()
	ctx.altFileSystems["testfs"] = altFs
	if err := afero.WriteFile(altFs, "/staged.txt", []byte("staged"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	creator := func() *query_models.ResourceFromLocalCreator {
		return &query_models.ResourceFromLocalCreator{LocalPath: "/staged.txt", PathName: "testfs"}
	}

	for _, role := range []models.Role{models.RoleEditor, models.RoleUser, models.RoleGuest} {
		bound := ctx.WithPrincipal(&auth.Principal{UserID: 1, Role: role})
		if _, err := bound.AddLocalResource("", creator()); !errors.Is(err, ErrRoleCapability) {
			t.Errorf("%s AddLocalResource err = %v, want ErrRoleCapability", role, err)
		}
	}
	if _, err := ctx.WithPrincipal(auth.SystemPrincipal()).AddLocalResource("", creator()); err != nil {
		t.Errorf("auth-off super-user AddLocalResource: %v", err)
	}
	if _, err := ctx.AddLocalResource("", creator()); err != nil {
		t.Errorf("unbound AddLocalResource: %v", err)
	}
}

// The physical half of the reservation: a path whose name looks ordinary but
// resolves into a configured private root, on the main mount through a
// symlink or on an alternate mount that sits over one, is refused as well.
func TestAddLocalResource_RefusesPathsResolvingIntoPrivateRoots(t *testing.T) {
	ctx := createTestContext(t)
	fileRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fileRoot, "_exports"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fileRoot, "_exports", "x.tar"), []byte("export"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fileRoot, jobs.JobReplayKeyFileName), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(fileRoot, "_exports"), filepath.Join(fileRoot, "exports-link")); err != nil {
		t.Fatal(err)
	}
	ctx.Config.FileSavePath = fileRoot
	ctx.fs = afero.NewBasePathFs(afero.NewOsFs(), fileRoot)
	ctx.Config.AltFileSystems = map[string]string{"exports": filepath.Join(fileRoot, "_exports")}
	ctx.altFileSystems["exports"] = afero.NewBasePathFs(afero.NewOsFs(), filepath.Join(fileRoot, "_exports"))

	for _, c := range []struct{ path, pathName string }{
		{"exports-link/x.tar", ""},
		{"x.tar", "exports"},
		{jobs.JobReplayKeyFileName, ""},
	} {
		_, err := ctx.AddLocalResource("", &query_models.ResourceFromLocalCreator{LocalPath: c.path, PathName: c.pathName})
		if !errors.Is(err, ErrPrivateStoragePath) {
			t.Errorf("AddLocalResource(%q on %q) err = %v, want ErrPrivateStoragePath", c.path, c.pathName, err)
		}
	}
}
