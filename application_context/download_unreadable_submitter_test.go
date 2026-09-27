package application_context

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/download_queue"
	"mahresources/hls"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// A download whose submitter's account cannot be read once its bytes are in
// saves nothing, as a refusal does, but the account refused nothing: the Job
// fails as the read's failure, a dependency, and keeps its Retry.
func TestADownloadWhoseSubmitterCannotBeReadAfterItsBytesFailsAsTheRead(t *testing.T) {
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
		_, _ = w.Write([]byte("bytes that arrive while the account cannot be read"))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	owner := createGroupNamed(t, ctx, "unreadable-submitter", nil)
	user, err := ctx.CreateUser(&UserInput{Username: "unreadable-submitter", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{
		URL:               server.URL + "/unreadable.txt",
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

	// The next read of the account, the one the resource writer makes once the
	// body is in, fails.
	var armed atomic.Bool
	armed.Store(true)
	const name = "test:fail-the-rebind-account-read"
	if err := ctx.db.Callback().Query().Before("gorm:query").Register(name, func(db *gorm.DB) {
		if db.Statement.Table == "users" && armed.CompareAndSwap(true, false) {
			_ = db.AddError(errors.New("injected read failure"))
		}
	}); err != nil {
		t.Fatalf("register the failing read: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name) })
	close(release)

	finished := waitForSnapshot(t, ctx, submissions[0].CanonicalJobID, "the download to finish",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if armed.Load() {
		t.Fatalf("the account was never read after the bytes were in")
	}
	if finished.State != jobs.StateFailed || finished.Failure == nil ||
		finished.Failure.Code != download_queue.FailureAccountCheckUnavailable || finished.Failure.Class != jobs.FailureClassDependency {
		t.Fatalf("the download ended %s with %+v, want failed %s/%s", finished.State, finished.Failure,
			download_queue.FailureAccountCheckUnavailable, jobs.FailureClassDependency)
	}
	var count int64
	if err := ctx.db.Model(&models.Resource{}).Where("created_by_user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count resources: %v", err)
	}
	if count != 0 {
		t.Fatalf("%d resources were saved although the account could not be checked", count)
	}
	commands, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(), jobs.Access{Administrator: true}, finished.ID)
	if err != nil {
		t.Fatalf("advertised commands: %v", err)
	}
	retry := false
	for _, command := range commands {
		retry = retry || command.Key == jobs.CommandRetry
	}
	if !retry {
		t.Fatalf("a download that failed on an unanswered read offers no Retry: %+v", commands)
	}
}
