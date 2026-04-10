package router

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"

	"xgate/internal/config"
)

// Router matches incoming Host header to configured routes. Immutable after
// construction; swap a whole new one via Handler to change routes.
type Router struct {
	entries []routeEntry
}

type routeEntry struct {
	pattern    string
	isWildcard bool
	suffix     string // for wildcard: e.g. ".proxy.localhost"
	proxy      *httputil.ReverseProxy
}

// New builds a Router from a list of routes. Returns an error if any
// target URL fails to parse — callers that need a non-fatal check (hot
// reload) must handle this.
func New(routes []config.Route) (*Router, error) {
	r := &Router{}
	for _, route := range routes {
		target, err := url.Parse(route.Target)
		if err != nil {
			return nil, fmt.Errorf("invalid target URL %q: %w", route.Target, err)
		}
		if target.Scheme == "" || target.Host == "" {
			return nil, fmt.Errorf("invalid target URL %q: missing scheme or host", route.Target)
		}

		host := route.Host
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[%s] upstream error: %v", host, err)
			http.Error(w, fmt.Sprintf("upstream unavailable: %v", err), http.StatusBadGateway)
		}

		entry := routeEntry{
			pattern: route.Host,
			proxy:   proxy,
		}
		if strings.HasPrefix(route.Host, "*.") {
			entry.isWildcard = true
			entry.suffix = route.Host[1:] // ".proxy.localhost"
		}
		r.entries = append(r.entries, entry)
	}

	// Match exact hosts before wildcards, regardless of config order.
	sort.SliceStable(r.entries, func(i, j int) bool {
		return !r.entries[i].isWildcard && r.entries[j].isWildcard
	})
	return r, nil
}

// Len returns the number of routes in the router. Primarily for test
// assertions that want to verify a hot-reload swap took effect.
func (r *Router) Len() int {
	return len(r.entries)
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	for _, e := range rt.entries {
		if e.isWildcard {
			if strings.HasSuffix(host, e.suffix) && len(host) > len(e.suffix) {
				e.proxy.ServeHTTP(w, r)
				return
			}
		} else if host == e.pattern {
			e.proxy.ServeHTTP(w, r)
			return
		}
	}

	log.Printf("no route matched for host: %s", host)
	http.Error(w, fmt.Sprintf("no route for host: %s", host), http.StatusBadGateway)
}

// Handler is a thin http.Handler that delegates to a *Router held in
// an atomic pointer, so the routing table can be swapped live without
// restarting the HTTP server.
type Handler struct {
	ptr atomic.Pointer[Router]
}

// NewHandler wraps the initial router in a live-swappable handler.
func NewHandler(initial *Router) *Handler {
	h := &Handler{}
	h.ptr.Store(initial)
	return h
}

// Store atomically replaces the underlying router. In-flight requests
// continue using the old router; new requests see the new one.
func (h *Handler) Store(r *Router) {
	h.ptr.Store(r)
}

// Load returns the current underlying router without copying.
func (h *Handler) Load() *Router {
	return h.ptr.Load()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.ptr.Load().ServeHTTP(w, r)
}
