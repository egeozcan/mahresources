package application_context

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/hls"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// A download still running when its submitter's account is deleted creates no
// resource: nothing it made could be stamped with an account that no longer
// exists. Its Job ends without success, and keeps the trace of the account.
func TestADownloadWhoseSubmitterWasDeletedMidTransferCreatesNothing(t *testing.T) {
	ctx := newDownloadJobContext(t)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	head := strings.Repeat("h", hls.SniffLen()+1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(head))
		w.(http.Flusher).Flush()
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		_, _ = w.Write([]byte("bytes that arrive after the account was deleted"))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	makeAdmin(t, ctx, "keeper")
	owner := createGroupNamed(t, ctx, "deleted-mid-transfer", nil)
	user, err := ctx.CreateUser(&UserInput{Username: "deleted-mid-transfer", Password: "password1", Role: models.RoleUser})
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
	time.Sleep(300 * time.Millisecond)

	if err := ctx.DeleteUser(user.ID); err != nil {
		t.Fatalf("delete the submitter: %v", err)
	}
	close(release)

	finished := waitForSnapshot(t, ctx, submissions[0].CanonicalJobID, "the download to finish",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if finished.State == jobs.StateSucceeded {
		t.Fatalf("a download completed for an account deleted before its resource was created")
	}
	var count int64
	if err := ctx.db.Model(&models.Resource{}).Where("created_by_user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count resources: %v", err)
	}
	if count != 0 {
		t.Fatalf("%d resources name the deleted account as their creator", count)
	}
	if finished.Failure == nil || strings.TrimSpace(finished.Failure.Message) == "" {
		t.Fatalf("the download ended %s with %+v, and says nothing a person can read", finished.State, finished.Failure)
	}
	if !finished.ActorDeleted || !finished.OwnerDeleted {
		t.Fatalf("the Job does not say its account was deleted: %+v", finished)
	}
}
