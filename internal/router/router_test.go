package router

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"xgate/internal/config"
)

func TestNew_InvalidTargetReturnsError(t *testing.T) {
	_, err := New([]config.Route{{Host: "a", Target: "://bad"}})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestNew_Valid(t *testing.T) {
	r, err := New([]config.Route{{Host: "a", Target: "http://127.0.0.1:9"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if r == nil {
		t.Fatal("nil router")
	}
	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1", r.Len())
	}
}

func TestHandler_SwapIsAtomic(t *testing.T) {
	// Two upstreams so we can tell which router was serving after the swap.
	var aHits, bHits atomic.Int64
	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aHits.Add(1)
		fmt.Fprintln(w, "A")
	}))
	defer upstreamA.Close()
	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bHits.Add(1)
		fmt.Fprintln(w, "B")
	}))
	defer upstreamB.Close()

	routerA, err := New([]config.Route{{Host: "h.localhost", Target: upstreamA.URL}})
	if err != nil {
		t.Fatal(err)
	}
	routerB, err := New([]config.Route{{Host: "h.localhost", Target: upstreamB.URL}})
	if err != nil {
		t.Fatal(err)
	}

	h := NewHandler(routerA)
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Drive traffic concurrently while we swap routers. The assertion is:
	// no panic, no goroutine read unbalance, some A hits, some B hits.
	var wg sync.WaitGroup
	stop := make(chan struct{})
	// started tracks how many goroutines have completed their first request.
	var started sync.WaitGroup
	started.Add(8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			first := true
			for {
				select {
				case <-stop:
					return
				default:
				}
				req, _ := http.NewRequest("GET", srv.URL, nil)
				req.Host = "h.localhost"
				resp, err := srv.Client().Do(req)
				if err != nil {
					t.Errorf("request: %v", err)
					return
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if first {
					first = false
					started.Done()
				}
			}
		}()
	}

	// Wait for all goroutines to complete at least one request before swapping.
	started.Wait()

	// Let A serve for a bit, swap to B, let B serve for a bit.
	for i := 0; i < 100; i++ {
		h.Store(routerA)
	}
	h.Store(routerB)
	for i := 0; i < 100; i++ {
		h.Store(routerB)
	}
	close(stop)
	wg.Wait()

	if aHits.Load() == 0 && bHits.Load() == 0 {
		t.Fatal("no requests were served")
	}
	// After the final store, B should have received at least one hit.
	if bHits.Load() == 0 {
		t.Fatal("router B never served a request after swap")
	}
}
