package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func startTestSocketServer(t *testing.T) (string, *AdminServer, context.CancelFunc) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	sockPath := filepath.Join(dir, "xgate.sock")

	cfg := &Config{Listen: ":80", ManageHosts: false, Routes: []Route{}}
	if err := writeConfig(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	router, _ := NewRouter(cfg.Routes)
	handler := NewRouterHandler(router)
	admin := NewAdminServer(cfgPath, cfg, handler)

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan error, 1)
	go func() {
		ready <- ServeSocket(ctx, sockPath, admin)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("unix", sockPath); err == nil {
			c.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Cleanup(func() {
		cancel()
		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			t.Error("socket server did not shut down")
		}
	})
	return sockPath, admin, cancel
}

func sendSocketCmd(t *testing.T, sockPath string, req map[string]any) map[string]any {
	t.Helper()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("encode: %v", err)
	}
	var resp map[string]any
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func TestSocket_AddListRemove(t *testing.T) {
	sockPath, _, _ := startTestSocketServer(t)

	resp := sendSocketCmd(t, sockPath, map[string]any{
		"cmd":    "add",
		"host":   "gateway.localhost",
		"target": "http://localhost:8081",
	})
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("add failed: %+v", resp)
	}

	resp = sendSocketCmd(t, sockPath, map[string]any{"cmd": "ls"})
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("ls failed: %+v", resp)
	}
	routes, _ := resp["routes"].([]any)
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %+v", routes)
	}

	resp = sendSocketCmd(t, sockPath, map[string]any{
		"cmd":  "rm",
		"host": "gateway.localhost",
	})
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("rm failed: %+v", resp)
	}

	resp = sendSocketCmd(t, sockPath, map[string]any{"cmd": "ls"})
	routes, _ = resp["routes"].([]any)
	if len(routes) != 0 {
		t.Fatalf("expected 0 routes, got %+v", routes)
	}
}

func TestSocket_AddError(t *testing.T) {
	sockPath, _, _ := startTestSocketServer(t)
	resp := sendSocketCmd(t, sockPath, map[string]any{
		"cmd":    "add",
		"host":   "",
		"target": "http://localhost:1",
	})
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatalf("expected error, got %+v", resp)
	}
	if errStr, _ := resp["error"].(string); errStr == "" {
		t.Fatalf("missing error string: %+v", resp)
	}
}

func TestSocket_UnknownCommand(t *testing.T) {
	sockPath, _, _ := startTestSocketServer(t)
	resp := sendSocketCmd(t, sockPath, map[string]any{"cmd": "wat"})
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatalf("expected error, got %+v", resp)
	}
}
