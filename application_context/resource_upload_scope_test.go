package application_context

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Content-hash deduplication decides what a new upload means by the resource
// that already holds its bytes. For a group-limited caller that resource has to
// be one it can see: attaching the caller's group to a resource outside its
// subtree is a write it has no right to make, and answering "that already
// exists" tells it what the library holds where it cannot look. Content held
// only outside the subtree is therefore new content to that caller, stored as
// its own resource over the same file.

func uploadAs(t *testing.T, ctx *MahresourcesContext, body, name string, owner uint) *models.Resource {
	t.Helper()
	created, err := ctx.AddResource(io.NopCloser(strings.NewReader(body)), name,
		&query_models.ResourceCreator{ResourceQueryBase: query_models.ResourceQueryBase{OwnerId: owner}})
	if err != nil {
		t.Fatalf("upload %s: %v", name, err)
	}
	return created
}

func relatedGroupIDs(t *testing.T, ctx *MahresourcesContext, resourceID uint) []uint {
	t.Helper()
	var loaded models.Resource
	if err := ctx.db.Preload("Groups").First(&loaded, resourceID).Error; err != nil {
		t.Fatalf("load resource %d: %v", resourceID, err)
	}
	ids := make([]uint, 0, len(loaded.Groups))
	for _, group := range loaded.Groups {
		ids = append(ids, group.ID)
	}
	return ids
}

func containsID(ids []uint, id uint) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func createGroupNamed(t *testing.T, ctx *MahresourcesContext, name string, owner *uint) *models.Group {
	t.Helper()
	group := &models.Group{Name: name, OwnerId: owner}
	if err := ctx.db.Create(group).Error; err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	return group
}

func downloadToTerminal(t *testing.T, ctx *MahresourcesContext, creator *query_models.ResourceFromRemoteCreator, owner *uint) jobs.Snapshot {
	t.Helper()
	submissions := ctx.SubmitRemoteDownloads(creator, owner, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil {
		t.Fatalf("submit: %+v", submissions)
	}
	return waitForSnapshot(t, ctx, submissions[0].CanonicalJobID, "the download to finish",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
}

func TestAScopedDownloadOfBytesHeldOutsideItsScopeCreatesItsOwnResource(t *testing.T) {
	ctx := newDownloadJobContext(t)
	const body = "bytes the library already holds outside this subtree"
	server := plainContentServer(t, body)
	inside := createGroupNamed(t, ctx, "scoped-download-inside", nil)
	outside := createGroupNamed(t, ctx, "scoped-download-outside", nil)
	existing := uploadAs(t, ctx, body, "held-elsewhere.txt", outside.ID)

	scopedUser, err := ctx.CreateUser(&UserInput{
		Username: "scoped-downloader", Password: "password1", Role: models.RoleUser, ScopeGroupId: &inside.ID,
	})
	if err != nil {
		t.Fatalf("create scoped user: %v", err)
	}

	finished := downloadToTerminal(t, ctx, &query_models.ResourceFromRemoteCreator{
		URL:               server.URL + "/fetched.txt",
		ResourceQueryBase: query_models.ResourceQueryBase{OwnerId: inside.ID},
	}, &scopedUser.ID)
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the scoped download ended %s (%+v), want a resource of its own", finished.State, finished.Failure)
	}
	outputs, err := ctx.GetJobOutputs(finished.ID)
	if err != nil {
		t.Fatalf("outputs: %v", err)
	}
	createdID := outputResourceID(t, outputs, jobDownloadResourceOutput)
	if createdID == 0 || createdID == existing.ID {
		t.Fatalf("the download reports resource %d, want a new resource rather than %d outside the subtree",
			createdID, existing.ID)
	}

	var created models.Resource
	if err := ctx.db.First(&created, createdID).Error; err != nil {
		t.Fatalf("load the created resource: %v", err)
	}
	if created.OwnerId == nil || *created.OwnerId != inside.ID {
		t.Fatalf("the created resource is owned by %v, want the scoped target %d", created.OwnerId, inside.ID)
	}
	if created.Hash != existing.Hash || created.Location != existing.Location {
		t.Fatalf("the created resource stores %s at %s, want the existing file %s at %s",
			created.Hash, created.Location, existing.Hash, existing.Location)
	}
	if !ctx.WithPrincipal(auth.FromUser(scopedUser)).ResourceVisible(createdID) {
		t.Fatalf("the scoped user cannot see the resource its own download created")
	}
	if containsID(relatedGroupIDs(t, ctx, existing.ID), inside.ID) {
		t.Fatalf("the scoped download attached its group to resource %d outside its subtree", existing.ID)
	}
}

func TestAnUnscopedDownloadStillFilesContentHeldByAnotherOwner(t *testing.T) {
	ctx := newDownloadJobContext(t)
	const body = "bytes an unscoped user downloads again under another owner"
	server := plainContentServer(t, body)
	first := createGroupNamed(t, ctx, "unscoped-download-first", nil)
	second := createGroupNamed(t, ctx, "unscoped-download-second", nil)
	existing := uploadAs(t, ctx, body, "already-here.txt", first.ID)

	user, err := ctx.CreateUser(&UserInput{Username: "unscoped-downloader", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	finished := downloadToTerminal(t, ctx, &query_models.ResourceFromRemoteCreator{
		URL:               server.URL + "/again.txt",
		ResourceQueryBase: query_models.ResourceQueryBase{OwnerId: second.ID},
	}, &user.ID)
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the download ended %s (%+v)", finished.State, finished.Failure)
	}
	outputs, err := ctx.GetJobOutputs(finished.ID)
	if err != nil {
		t.Fatalf("outputs: %v", err)
	}
	if got := outputResourceID(t, outputs, jobDownloadResourceOutput); got != existing.ID {
		t.Fatalf("the download reports resource %d, want the existing %d it was filed onto", got, existing.ID)
	}
	if !containsID(relatedGroupIDs(t, ctx, existing.ID), second.ID) {
		t.Fatalf("the requested owner %d was not attached to the resource holding the bytes", second.ID)
	}
}

func setupSharedFileTestCtx(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := createTestContext(t)
	if err := ctx.db.AutoMigrate(&models.ResourceVersion{}); err != nil {
		t.Fatalf("migrate versions: %v", err)
	}
	return ctx
}

func scopedPrincipalFor(group *models.Group) *auth.Principal {
	return &auth.Principal{UserID: 9001, Role: models.RoleUser, ScopeGroupID: &group.ID}
}

func TestAScopedUploadOfBytesHeldOutsideItsScopeSharesTheFile(t *testing.T) {
	ctx := setupSharedFileTestCtx(t)
	const body = "a scoped upload of content held outside its subtree"
	inside := createGroupNamed(t, ctx, "scoped-upload-inside", nil)
	outside := createGroupNamed(t, ctx, "scoped-upload-outside", nil)
	existing := uploadAs(t, ctx, body, "outside.txt", outside.ID)

	scoped := ctx.WithPrincipal(scopedPrincipalFor(inside))
	created := uploadAs(t, scoped, body, "inside.txt", inside.ID)

	if created.ID == existing.ID {
		t.Fatalf("the scoped upload was answered with resource %d outside its subtree", existing.ID)
	}
	if created.Location != existing.Location {
		t.Fatalf("the scoped copy is stored at %s, want the existing file %s", created.Location, existing.Location)
	}
	if containsID(relatedGroupIDs(t, ctx, existing.ID), inside.ID) {
		t.Fatalf("the scoped upload attached its group to resource %d outside its subtree", existing.ID)
	}
}

// Two rows over one file is only safe if deleting either leaves the file for the
// other. Whether the file may go is a question about every row on that
// filesystem, whoever is deleting: a group-limited caller's count must include
// rows it cannot see.
func TestDeletingEitherResourceThatSharesAFileKeepsTheOthersFile(t *testing.T) {
	runSharedFileDeletionCases(t, setupSharedFileTestCtx)
}

func runSharedFileDeletionCases(t *testing.T, newContext func(*testing.T) *MahresourcesContext) {
	const body = "one file behind two resources in different subtrees"

	arrange := func(t *testing.T) (*MahresourcesContext, *MahresourcesContext, *models.Resource, *models.Resource) {
		ctx := newContext(t)
		inside := createGroupNamed(t, ctx, "shared-file-inside", nil)
		outside := createGroupNamed(t, ctx, "shared-file-outside", nil)
		existing := uploadAs(t, ctx, body, "outside.txt", outside.ID)
		scoped := ctx.WithPrincipal(scopedPrincipalFor(inside))
		copyInScope := uploadAs(t, scoped, body, "inside.txt", inside.ID)
		if copyInScope.Location != existing.Location {
			t.Fatalf("the two resources do not share a file: %s and %s", copyInScope.Location, existing.Location)
		}
		return ctx, scoped, existing, copyInScope
	}
	fileExists := func(t *testing.T, ctx *MahresourcesContext, resource *models.Resource) bool {
		t.Helper()
		exists, err := afero.Exists(ctx.fs, resource.GetCleanLocation())
		if err != nil {
			t.Fatalf("stat %s: %v", resource.Location, err)
		}
		return exists
	}

	t.Run("the scoped caller deletes its copy of a resource that predates versions", func(t *testing.T) {
		ctx, scoped, existing, copyInScope := arrange(t)
		// A resource from before versioning, or kept by -skip-version-migration,
		// has no version rows: its resource row is its only reference to the file.
		if err := ctx.db.Model(&models.Resource{}).Where("id = ?", existing.ID).
			Update("current_version_id", nil).Error; err != nil {
			t.Fatalf("clear current version: %v", err)
		}
		if err := ctx.db.Where("resource_id = ?", existing.ID).Delete(&models.ResourceVersion{}).Error; err != nil {
			t.Fatalf("drop version rows: %v", err)
		}
		if err := scoped.DeleteResource(copyInScope.ID); err != nil {
			t.Fatalf("scoped delete: %v", err)
		}
		if !fileExists(t, ctx, existing) {
			t.Fatalf("deleting the scoped copy removed the file resource %d still points at", existing.ID)
		}
	})

	t.Run("the scoped caller bulk-deletes its copy", func(t *testing.T) {
		ctx, scoped, existing, copyInScope := arrange(t)
		if err := ctx.db.Model(&models.Resource{}).Where("id = ?", existing.ID).
			Update("current_version_id", nil).Error; err != nil {
			t.Fatalf("clear current version: %v", err)
		}
		if err := ctx.db.Where("resource_id = ?", existing.ID).Delete(&models.ResourceVersion{}).Error; err != nil {
			t.Fatalf("drop version rows: %v", err)
		}
		if err := scoped.BulkDeleteResources(&query_models.BulkQuery{ID: []uint{copyInScope.ID}}); err != nil {
			t.Fatalf("scoped bulk delete: %v", err)
		}
		if !fileExists(t, ctx, existing) {
			t.Fatalf("bulk-deleting the scoped copy removed the file resource %d still points at", existing.ID)
		}
	})

	t.Run("the scoped caller deletes its copy", func(t *testing.T) {
		ctx, scoped, existing, copyInScope := arrange(t)
		if err := scoped.DeleteResource(copyInScope.ID); err != nil {
			t.Fatalf("scoped delete: %v", err)
		}
		if !fileExists(t, ctx, existing) {
			t.Fatalf("deleting the scoped copy removed the file resource %d still points at", existing.ID)
		}
	})

	t.Run("an administrator deletes the original", func(t *testing.T) {
		ctx, _, existing, copyInScope := arrange(t)
		if err := ctx.DeleteResource(existing.ID); err != nil {
			t.Fatalf("delete the original: %v", err)
		}
		if !fileExists(t, ctx, copyInScope) {
			t.Fatalf("deleting the original removed the file the scoped copy %d still points at", copyInScope.ID)
		}
	})

	t.Run("the last reference removes the file", func(t *testing.T) {
		ctx, scoped, existing, copyInScope := arrange(t)
		if err := ctx.DeleteResource(existing.ID); err != nil {
			t.Fatalf("delete the original: %v", err)
		}
		if err := scoped.DeleteResource(copyInScope.ID); err != nil {
			t.Fatalf("scoped delete: %v", err)
		}
		if fileExists(t, ctx, copyInScope) {
			t.Fatalf("the file outlived the last resource that referenced it")
		}
	})
}

// The scope a download is created under is the submitter's account when the
// transfer ends. One that was disabled while its bytes were in flight is not
// unscoped by that: it creates nothing.
func TestADownloadWhoseSubmitterWasDisabledMidTransferCreatesNothing(t *testing.T) {
	ctx := newDownloadJobContext(t)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		_, _ = w.Write([]byte("bytes that arrive after the account was disabled"))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	owner := createGroupNamed(t, ctx, "disabled-mid-transfer", nil)
	user, err := ctx.CreateUser(&UserInput{Username: "disabled-mid-transfer", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{
		URL:               server.URL + "/late.txt",
		ResourceQueryBase: query_models.ResourceQueryBase{OwnerId: owner.ID},
	}, &user.ID, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil {
		t.Fatalf("submit: %+v", submissions)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the transfer never started")
	}

	if _, err := ctx.UpdateUser(user.ID, &UserUpdate{Disabled: UserField[bool]{Set: true, Value: true}}); err != nil {
		t.Fatalf("disable the submitter: %v", err)
	}
	close(release)

	finished := waitForSnapshot(t, ctx, submissions[0].CanonicalJobID, "the download to finish",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if finished.State == jobs.StateSucceeded {
		t.Fatalf("a download completed for an account disabled before its resource was created")
	}
	var count int64
	if err := ctx.db.Model(&models.Resource{}).Where("created_by_user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count resources: %v", err)
	}
	if count != 0 {
		t.Fatalf("%d resources were created for a disabled account", count)
	}
}
