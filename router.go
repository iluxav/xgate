package main

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
)

// Router matches incoming Host header to configured routes. Immutable after
// construction; swap a whole new one via RouterHandler to change routes.
type Router struct {
	entries []routeEntry
}

type routeEntry struct {
	pattern    string
	isWildcard bool
	suffix     string // for wildcard: e.g. ".proxy.localhost"
	proxy      *httputil.ReverseProxy
}

// NewRouter builds a Router from a list of routes. Returns an error if any
// target URL fails to parse — callers that need a non-fatal check (hot
// reload) must handle this.
func NewRouter(routes []Route) (*Router, error) {
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

// RouterHandler is a thin http.Handler that delegates to a *Router held in
// an atomic pointer, so the routing table can be swapped live without
// restarting the HTTP server.
type RouterHandler struct {
	ptr atomic.Pointer[Router]
}

func NewRouterHandler(initial *Router) *RouterHandler {
	h := &RouterHandler{}
	h.ptr.Store(initial)
	return h
}

func (h *RouterHandler) Store(r *Router) {
	h.ptr.Store(r)
}

func (h *RouterHandler) Load() *Router {
	return h.ptr.Load()
}

func (h *RouterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.ptr.Load().ServeHTTP(w, r)
}
