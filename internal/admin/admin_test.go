package admin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xgate/internal/config"
	"xgate/internal/router"
)

func newTestAdmin(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		Listen:      ":80",
		ManageHosts: false, // keep /etc/hosts untouched
		Routes:      []config.Route{},
	}
	if err := config.Write(path, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	r, err := router.New(cfg.Routes)
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	handler := router.NewHandler(r)
	admin := NewServer(path, cfg, handler)
	return admin, path
}

func TestAdmin_Add(t *testing.T) {
	admin, path := newTestAdmin(t)
	routes, err := admin.Add("gateway.localhost", "http://localhost:8081")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(routes) != 1 || routes[0].Host != "gateway.localhost" {
		t.Fatalf("routes = %+v", routes)
	}
	onDisk, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(onDisk.Routes) != 1 {
		t.Fatalf("onDisk.Routes = %+v", onDisk.Routes)
	}
	// Verify the live router was swapped to reflect the new route count.
	if n := admin.Handler().Load().Len(); n != 1 {
		t.Fatalf("router Len = %d, want 1", n)
	}
}

func TestAdmin_Add_RejectsDuplicate(t *testing.T) {
	admin, _ := newTestAdmin(t)
	if _, err := admin.Add("a.localhost", "http://localhost:1"); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	_, err := admin.Add("a.localhost", "http://localhost:2")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestAdmin_Add_RejectsEmptyHost(t *testing.T) {
	admin, _ := newTestAdmin(t)
	if _, err := admin.Add("", "http://x"); err == nil {
		t.Fatal("expected error for empty host")
	}
}

func TestAdmin_Add_RejectsBadURL(t *testing.T) {
	admin, _ := newTestAdmin(t)
	if _, err := admin.Add("a.localhost", "not a url"); err == nil {
		t.Fatal("expected error for invalid target")
	}
}

func TestAdmin_Remove(t *testing.T) {
	admin, path := newTestAdmin(t)
	if _, err := admin.Add("a.localhost", "http://localhost:1"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Add("b.localhost", "http://localhost:2"); err != nil {
		t.Fatal(err)
	}
	routes, err := admin.Remove("a.localhost")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(routes) != 1 || routes[0].Host != "b.localhost" {
		t.Fatalf("routes = %+v", routes)
	}
	onDisk, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(onDisk.Routes) != 1 {
		t.Fatalf("onDisk.Routes = %+v", onDisk.Routes)
	}
}

func TestAdmin_Remove_MissingHost(t *testing.T) {
	admin, _ := newTestAdmin(t)
	_, err := admin.Remove("nope.localhost")
	if err == nil || !strings.Contains(err.Error(), "no such route") {
		t.Fatalf("expected no-such-route error, got %v", err)
	}
}

func TestAdmin_List(t *testing.T) {
	admin, _ := newTestAdmin(t)
	if _, err := admin.Add("a.localhost", "http://localhost:1"); err != nil {
		t.Fatal(err)
	}
	routes := admin.List()
	if len(routes) != 1 {
		t.Fatalf("List() = %+v", routes)
	}
}

func TestAdmin_Reload(t *testing.T) {
	admin, path := newTestAdmin(t)
	edited := &config.Config{
		Listen:      ":80",
		ManageHosts: false,
		Routes: []config.Route{
			{Host: "fresh.localhost", Target: "http://localhost:9"},
		},
	}
	if err := config.Write(path, edited); err != nil {
		t.Fatal(err)
	}
	if len(admin.List()) != 0 {
		t.Fatalf("pre-reload List = %+v", admin.List())
	}
	routes, err := admin.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(routes) != 1 || routes[0].Host != "fresh.localhost" {
		t.Fatalf("post-reload routes = %+v", routes)
	}
}

func TestAdmin_Add_TrimsWhitespace(t *testing.T) {
	admin, _ := newTestAdmin(t)
	routes, err := admin.Add("  spaced.localhost  ", "  http://localhost:9001  ")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("routes = %+v", routes)
	}
	if routes[0].Host != "spaced.localhost" {
		t.Fatalf("Host = %q, want %q", routes[0].Host, "spaced.localhost")
	}
	if routes[0].Target != "http://localhost:9001" {
		t.Fatalf("Target = %q, want %q", routes[0].Target, "http://localhost:9001")
	}
	// A second Add with the trimmed form must be detected as a duplicate.
	if _, err := admin.Add("spaced.localhost", "http://localhost:9002"); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestAdmin_Add_FailedValidationDoesNotWrite(t *testing.T) {
	admin, path := newTestAdmin(t)
	if _, err := admin.Add("good.localhost", "http://localhost:1"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if _, err := admin.Add("bad.localhost", "not a url"); err == nil {
		t.Fatal("expected error")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("config.yaml was modified on validation failure")
	}
}
