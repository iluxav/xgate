package admin

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"xgate/internal/config"
	"xgate/internal/hosts"
	"xgate/internal/router"
)

// Server owns the live config + router state and serializes mutations.
// All exported methods are safe for concurrent use. Mutating methods hold
// the mutex for the full duration of the operation (validate + rewrite
// config.yaml + rewrite /etc/hosts + swap router pointer).
type Server struct {
	mu         sync.Mutex
	configPath string
	cfg        *config.Config
	handler    *router.Handler
}

// NewServer constructs an admin server. The caller owns cfg and handler
// and must not mutate them after passing them in.
func NewServer(configPath string, cfg *config.Config, handler *router.Handler) *Server {
	return &Server{
		configPath: configPath,
		cfg:        cfg,
		handler:    handler,
	}
}

// Handler returns the underlying RouterHandler. Used by tests that want to
// verify the live router was swapped after a mutation.
func (a *Server) Handler() *router.Handler {
	return a.handler
}

// Add appends a new route, persists config, updates /etc/hosts (if
// manage_hosts), and swaps the live router. Returns the full routes slice
// after the mutation.
func (a *Server) Add(host, target string) ([]config.Route, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	host, err := validateHost(host)
	if err != nil {
		return nil, err
	}
	target, err = validateTarget(target)
	if err != nil {
		return nil, err
	}
	for _, r := range a.cfg.Routes {
		if r.Host == host {
			return nil, fmt.Errorf("host already exists: %s", host)
		}
	}

	newRoutes := append([]config.Route{}, a.cfg.Routes...)
	newRoutes = append(newRoutes, config.Route{Host: host, Target: target})

	return a.commit(newRoutes)
}

// Remove deletes the route with the given host. Returns the full routes
// slice after the mutation.
func (a *Server) Remove(host string) ([]config.Route, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	idx := -1
	for i, r := range a.cfg.Routes {
		if r.Host == host {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, fmt.Errorf("no such route: %s", host)
	}

	newRoutes := make([]config.Route, 0, len(a.cfg.Routes)-1)
	newRoutes = append(newRoutes, a.cfg.Routes[:idx]...)
	newRoutes = append(newRoutes, a.cfg.Routes[idx+1:]...)

	return a.commit(newRoutes)
}

// List returns a copy of the current routes.
func (a *Server) List() []config.Route {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]config.Route, len(a.cfg.Routes))
	copy(out, a.cfg.Routes)
	return out
}

// Reload re-reads config.yaml from disk and rebuilds the router. Useful
// after hand-editing the file.
func (a *Server) Reload() ([]config.Route, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	loaded, err := config.Load(a.configPath)
	if err != nil {
		return nil, fmt.Errorf("reload: %w", err)
	}

	newRouter, err := router.New(loaded.Routes)
	if err != nil {
		return nil, fmt.Errorf("reload: %w", err)
	}
	if loaded.ManageHosts {
		if err := hosts.Add(loaded.Routes); err != nil {
			return nil, fmt.Errorf("reload: %w", err)
		}
	}
	a.handler.Store(newRouter)
	a.cfg = loaded

	out := make([]config.Route, len(loaded.Routes))
	copy(out, loaded.Routes)
	return out, nil
}

// commit is the shared tail of Add and Remove. Assumes the mutex is held
// and newRoutes has already been validated. It builds the new router in
// memory, writes config.yaml, rewrites /etc/hosts, then swaps the router
// pointer. On any error, in-memory state is untouched.
func (a *Server) commit(newRoutes []config.Route) ([]config.Route, error) {
	newRouter, err := router.New(newRoutes)
	if err != nil {
		return nil, err
	}

	newCfg := &config.Config{
		Listen:      a.cfg.Listen,
		ManageHosts: a.cfg.ManageHosts,
		Routes:      newRoutes,
	}

	if err := config.Write(a.configPath, newCfg); err != nil {
		return nil, err
	}

	if a.cfg.ManageHosts {
		if err := hosts.Add(newRoutes); err != nil {
			return nil, fmt.Errorf("update /etc/hosts: %w", err)
		}
	}

	a.handler.Store(newRouter)
	a.cfg = newCfg

	out := make([]config.Route, len(newRoutes))
	copy(out, newRoutes)
	return out, nil
}

func validateHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("host cannot be empty")
	}
	return host, nil
}

func validateTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("target cannot be empty")
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("invalid target URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid target URL: %s (need scheme://host)", target)
	}
	return target, nil
}
