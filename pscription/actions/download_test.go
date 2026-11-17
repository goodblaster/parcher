package actions

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestCachedDownloadsStartBeforeCollection(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	slowStarted, lateStarted, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			close(slowStarted)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		} else {
			close(lateStarted)
		}
		fmt.Fprint(w, "image")
	}))
	defer srv.Close()
	cache := NewPrefetchCache()
	cache.start(ctx, srv.URL+"/slow", "test")
	select {
	case <-slowStarted:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	done := make(chan int, 1)
	go func() {
		_, n, _, _ := collectCached(ctx, cache, []string{srv.URL + "/slow", srv.URL + "/late", srv.URL + "/late"}, "image", "test")
		done <- n
	}()
	select {
	case <-lateStarted:
	case <-ctx.Done():
		t.Error("late resource waited behind slow prefetch")
	}
	once.Do(func() { close(release) })
	if n := <-done; n != 2 {
		t.Errorf("downloaded %d resources", n)
	}
}

func TestFontDownloadsParallelDeduplicatedAndOrdered(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	calls := map[string]int{}
	started := make(chan string, 8)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.URL.Path]++
		mu.Unlock()
		if r.URL.Path == "/bad" {
			http.Error(w, "missing", 404)
			return
		}
		started <- r.URL.Path
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, r.URL.Path)
	}))
	defer srv.Close()
	faces := []fontFace{{URLs: []string{srv.URL + "/a.woff2", srv.URL + "/unused"}}, {URLs: []string{srv.URL + "/bad", srv.URL + "/b.woff2"}}, {URLs: []string{srv.URL + "/a.woff2"}}}
	done := make(chan []downloadedFont, 1)
	go func() { done <- downloadFonts(ctx, srv.Client(), faces, "test") }()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Error("independent fonts did not download concurrently")
		}
	}
	once.Do(func() { close(release) })
	got := <-done
	if got[0].dataURL == "" || got[1].dataURL == "" || got[0] != got[2] || got[0] == got[1] {
		t.Fatalf("wrong ordered results: %+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["/a.woff2"] != 1 || calls["/b.woff2"] != 1 || calls["/bad"] != 1 || calls["/unused"] != 0 {
		t.Fatalf("requests: %v", calls)
	}
}

func TestDownloadGateCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	gate := newHostGate(1, 1)
	release, ok := gate.acquire(ctx, "https://example.com/a")
	if !ok {
		t.Fatal("first acquire")
	}
	cancel()
	if _, ok := gate.acquire(ctx, "https://example.com/b"); ok {
		t.Fatal("cancelled waiter acquired slot")
	}
	release()
	release, ok = gate.acquire(t.Context(), "https://example.com/c")
	if !ok {
		t.Fatal("leaked slot")
	}
	release()
}

func TestPrefetchFailureAndNewSession(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		http.Error(w, "forbidden", 403)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cache := NewPrefetchCache()
	for i := 0; i < 2; i++ {
		_, _, failed, _ := collectCached(ctx, cache, []string{srv.URL}, "image", "test")
		if failed != 1 {
			t.Fatal("missing failure")
		}
	}
	cancel() // a capture retry owns a new browser context
	collectCached(t.Context(), cache, []string{srv.URL}, "image", "test")
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("got %d requests, want one per session", calls)
	}
}
