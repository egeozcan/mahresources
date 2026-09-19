package plugin_commands

import (
	"strings"
	"sync"
	"testing"
)

func TestLeaseBlocksSweepAndSweepClosesCheckToAcquireGap(t *testing.T) {
	leases := NewLeaseManager()
	release, err := leases.Acquire("run")
	if err != nil {
		t.Fatal(err)
	}
	if end, ok := leases.BeginSweep("run"); ok {
		end()
		t.Fatal("sweep began while a lease was active")
	}
	release()

	endSweep, ok := leases.BeginSweep("run")
	if !ok {
		t.Fatal("sweep did not begin after lease release")
	}
	if _, err := leases.Acquire("run"); err == nil || !strings.Contains(err.Error(), "run swept") {
		t.Fatalf("acquire during sweep error = %v", err)
	}
	endSweep()
	secondRelease, err := leases.Acquire("run")
	if err != nil {
		t.Fatalf("acquire after sweep decision completed: %v", err)
	}
	secondRelease()
}

func TestLeaseSweepInterleavingHasNoCheckToAcquireWindow(t *testing.T) {
	leases := NewLeaseManager()
	checked := make(chan struct{})
	allowDelete := make(chan struct{})
	deleted := make(chan struct{})

	go func() {
		end, ok := leases.BeginSweep("expired")
		if !ok {
			close(deleted)
			return
		}
		defer end()
		close(checked)
		<-allowDelete
		close(deleted)
	}()
	<-checked

	result := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := leases.Acquire("expired")
		result <- err
	}()
	acquireErr := <-result
	if acquireErr == nil || !strings.Contains(acquireErr.Error(), "run swept") {
		t.Fatalf("operation slipped into sweep decision: %v", acquireErr)
	}
	close(allowDelete)
	<-deleted
	wg.Wait()
}
