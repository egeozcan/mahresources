package commands

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLoginDoesNotReplayCredentialsCrossOrigin(t *testing.T) {
	for _, status := range []int{307, 308} {
		var hits int32
		b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
		}))
		target := strings.Replace(b.URL, "127.0.0.1", "localhost", 1)
		a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target+"/collect", status)
		}))
		_, err := loginAndMintToken(a.URL, "alice", "hunter2", "x")
		if err == nil {
			t.Fatalf("%d: want error", status)
		}
		if atomic.LoadInt32(&hits) != 0 {
			t.Fatalf("%d: foreign origin got a request", status)
		}
		a.Close()
		b.Close()
	}
}

func TestLoginLogsOutTemporarySession(t *testing.T) {
	for _, mintStatus := range []int{200, 500} {
		var mu sync.Mutex
		var calls []string
		var logout struct{ method, csrf, cookie string }
		var logoutN int
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "abc", Path: "/"})
		})
		mux.HandleFunc("/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"csrfToken":"csrf1"}`))
		})
		mux.HandleFunc("/v1/account/tokens", func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			calls = append(calls, "mint")
			mu.Unlock()
			if mintStatus != 200 {
				w.WriteHeader(mintStatus)
				return
			}
			w.Write([]byte(`{"token":"T"}`))
		})
		mux.HandleFunc("/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, "logout")
			logoutN++
			logout.method, logout.csrf = r.Method, r.Header.Get("X-CSRF-Token")
			if c, err := r.Cookie("sid"); err == nil {
				logout.cookie = c.Value
			}
		})
		s := httptest.NewServer(mux)
		tok, err := loginAndMintToken(s.URL, "alice", "pw", "x")
		s.Close()
		if mintStatus == 200 && (err != nil || tok != "T") {
			t.Fatalf("tok=%q err=%v", tok, err)
		}
		if mintStatus != 200 && err == nil {
			t.Fatal("want error")
		}
		if logoutN != 1 || logout.method != "POST" || logout.csrf != "csrf1" || logout.cookie != "abc" {
			t.Fatalf("status %d: logout = %d %+v", mintStatus, logoutN, logout)
		}
		if len(calls) != 2 || calls[0] != "mint" || calls[1] != "logout" {
			t.Fatalf("order = %v", calls)
		}
	}
}
