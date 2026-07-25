package actions

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/goodblaster/logos"
)

// Fetch concurrency is bounded PER HOST, not globally, for the same
// reason browsers cap connections per origin: throughput on a page whose
// resources span several CDNs, without hammering any single one. A flat
// global number gets this wrong in both directions — too slow when
// resources are spread across hosts, too aggressive when they all sit on
// one CDN (which is when an origin starts answering 429s, and a refused
// fetch means a MISSING IMAGE in the archive, not just a slower capture).
//
// perHostFetchConcurrency can be overridden with
// PSCRIPTION_FETCH_CONCURRENCY for a site that needs gentler or harder
// treatment.
const (
	perHostFetchConcurrency = 12
	maxFetchConcurrency     = 32 // global socket bound across all hosts
)

func perHostLimit() int {
	if v, err := strconv.Atoi(os.Getenv("PSCRIPTION_FETCH_CONCURRENCY")); err == nil && v > 0 {
		return v
	}
	return perHostFetchConcurrency
}

// fetchTransport is a tuned clone of http.DefaultTransport. The default
// keeps only TWO idle connections per host (MaxIdleConnsPerHost), so a
// parallel batch aimed at one CDN re-runs the TCP+TLS handshake for
// almost every image — a cost no amount of extra workers can recover.
// Sizing the idle pool to the per-host limit lets the batch reuse warm
// connections instead.
var fetchTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = perHostLimit()
	t.MaxIdleConns = maxFetchConcurrency * 2
	t.IdleConnTimeout = 60 * time.Second
	return t
}()

// fetchClient is shared so connections stay warm ACROSS the fetch steps
// (images, then backgrounds, typically from the same CDNs).
var fetchClient = &http.Client{Timeout: 30 * time.Second, Transport: fetchTransport}

// hostGate hands out per-host semaphores, so each origin is limited
// independently while a global semaphore bounds total sockets.
type hostGate struct {
	mu      sync.Mutex
	limit   int
	perHost map[string]chan struct{}
	global  chan struct{}
}

func newHostGate(perHost, global int) *hostGate {
	return &hostGate{limit: perHost, perHost: map[string]chan struct{}{}, global: make(chan struct{}, global)}
}

// acquire blocks until the URL's host and the global pool both have room,
// returning a release func. Per-host is taken FIRST so a goroutine
// queued behind a busy origin never sits on a global slot that another
// host could use. Returns false if ctx ends while waiting.
func (g *hostGate) acquire(ctx context.Context, rawURL string) (func(), bool) {
	host := rawURL
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		host = u.Host
	}
	g.mu.Lock()
	sem, ok := g.perHost[host]
	if !ok {
		sem = make(chan struct{}, g.limit)
		g.perHost[host] = sem
	}
	g.mu.Unlock()

	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return nil, false
	}
	select {
	case g.global <- struct{}{}:
	case <-ctx.Done():
		<-sem
		return nil, false
	}
	return func() { <-g.global; <-sem }, true
}

// fetchAllAsDataURLs fetches urls in parallel — deduplicated, limited per
// host, aborted by ctx — and returns url → data URL for the successes
// plus fetched/failed counts. Per-URL failures are logged and counted,
// never fatal: a missing image beats a failed capture. what names the
// resource kind in logs.
func fetchAllAsDataURLs(ctx context.Context, urls []string, what string) (map[string]string, int, int) {
	gate := newHostGate(perHostLimit(), maxFetchConcurrency)
	out := make(map[string]string, len(urls))
	seen := make(map[string]bool, len(urls))
	var (
		mu              sync.Mutex
		wg              sync.WaitGroup
		fetched, failed int
	)

	for _, u := range urls {
		if seen[u] {
			continue
		}
		seen[u] = true
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			release, ok := gate.acquire(ctx, u)
			if !ok {
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			defer release()

			dataURL, err := fetchAsDataURL(ctx, fetchClient, u)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				logos.With("url", u).With("error", err).Debug("failed to fetch " + what)
				failed++
				return
			}
			out[u] = dataURL
			fetched++
		}(u)
	}
	wg.Wait()
	return out, fetched, failed
}
