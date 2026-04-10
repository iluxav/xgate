package main

import (
	"strings"
	"testing"
)

func TestFormatRoutes(t *testing.T) {
	out := formatRoutes([]Route{
		{Host: "gateway.localhost", Target: "http://localhost:8081"},
		{Host: "*.app.localhost", Target: "http://localhost:5173"},
	})
	if !strings.Contains(out, "HOST") || !strings.Contains(out, "TARGET") {
		t.Fatalf("missing header: %q", out)
	}
	if !strings.Contains(out, "gateway.localhost") || !strings.Contains(out, "http://localhost:8081") {
		t.Fatalf("missing row 1: %q", out)
	}
	if !strings.Contains(out, "*.app.localhost") {
		t.Fatalf("missing row 2: %q", out)
	}
}

func TestFormatRoutes_Empty(t *testing.T) {
	out := formatRoutes(nil)
	if !strings.Contains(out, "HOST") {
		t.Fatalf("missing header on empty list: %q", out)
	}
}
