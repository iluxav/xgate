package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteConfig_AtomicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{
		Listen:      ":80",
		ManageHosts: true,
		Routes: []Route{
			{Host: "gateway.localhost", Target: "http://localhost:8081"},
			{Host: "*.app.localhost", Target: "http://localhost:5173"},
		},
	}

	if err := writeConfig(path, cfg); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0644 {
		t.Fatalf("mode = %v, want 0644", mode)
	}

	got, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got.Listen != cfg.Listen || got.ManageHosts != cfg.ManageHosts {
		t.Fatalf("top-level mismatch: %+v", got)
	}
	if len(got.Routes) != 2 || got.Routes[0].Host != "gateway.localhost" {
		t.Fatalf("routes mismatch: %+v", got.Routes)
	}
}

func TestWriteConfig_LeavesNoTempfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &Config{Listen: ":80"}

	if err := writeConfig(path, cfg); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yaml" {
		t.Fatalf("unexpected directory contents: %v", entries)
	}
}

func TestEnsureConfig_CreatesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.yaml")

	if err := ensureConfig(path); err != nil {
		t.Fatalf("ensureConfig: %v", err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Listen != ":80" {
		t.Fatalf("Listen = %q, want :80", cfg.Listen)
	}
	if !cfg.ManageHosts {
		t.Fatalf("ManageHosts = false, want true")
	}
	if len(cfg.Routes) != 0 {
		t.Fatalf("Routes = %v, want empty", cfg.Routes)
	}
}

func TestEnsureConfig_LeavesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := []byte("listen: \":9000\"\nmanage_hosts: false\nroutes: []\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := ensureConfig(path); err != nil {
		t.Fatalf("ensureConfig: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readfile: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("file mutated:\nwant: %s\ngot:  %s", original, got)
	}
}
