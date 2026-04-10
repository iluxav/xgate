package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"
)

const usageText = `xgate — local reverse proxy with live-reload CLI

Usage:
  xgate serve                     Run the daemon (used by the service unit).
  xgate add <host> <target>       Add a route and persist it.
  xgate rm  <host>                Remove a route.
  xgate ls                        List routes (falls back to config file if daemon is down).
  xgate reload                    Re-read config.yaml from disk.

Flags:
  --config <path>                 Config file (default /etc/xgate/config.yaml, env XGATE_CONFIG).
  --socket <path>                 Admin socket (default /var/run/xgate.sock, env XGATE_SOCKET).
`

// runCLI dispatches a CLI subcommand. Returns the process exit code.
func runCLI(args []string, configPath, socketPath string) int {
	if len(args) == 0 {
		fmt.Print(usageText)
		return 0
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Print(usageText)
		return 0
	case "add":
		return cliAdd(args[1:], socketPath)
	case "rm":
		return cliRm(args[1:], socketPath)
	case "ls":
		return cliLs(socketPath, configPath)
	case "reload":
		return cliReload(socketPath)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", args[0])
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
}

func cliAdd(args []string, socketPath string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: xgate add <host> <target>")
		return 2
	}
	host, target := args[0], args[1]
	resp, err := sendRequest(socketPath, Request{Cmd: "add", Host: host, Target: target})
	if err != nil {
		printDaemonDownError(err)
		return 1
	}
	if !resp.OK {
		fmt.Fprintln(os.Stderr, resp.Error)
		return 1
	}
	fmt.Printf("added %s -> %s (%d routes)\n", host, target, len(resp.Routes))
	return 0
}

func cliRm(args []string, socketPath string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: xgate rm <host>")
		return 2
	}
	host := args[0]
	resp, err := sendRequest(socketPath, Request{Cmd: "rm", Host: host})
	if err != nil {
		printDaemonDownError(err)
		return 1
	}
	if !resp.OK {
		fmt.Fprintln(os.Stderr, resp.Error)
		return 1
	}
	fmt.Printf("removed %s (%d routes)\n", host, len(resp.Routes))
	return 0
}

func cliLs(socketPath, configPath string) int {
	resp, err := sendRequest(socketPath, Request{Cmd: "ls"})
	if err != nil {
		cfg, loadErr := loadConfig(configPath)
		if loadErr != nil {
			fmt.Fprintln(os.Stderr, "xgate is not installed or has never been started")
			return 1
		}
		fmt.Println("# daemon not running — showing config file")
		fmt.Print(formatRoutes(cfg.Routes))
		return 0
	}
	if !resp.OK {
		fmt.Fprintln(os.Stderr, resp.Error)
		return 1
	}
	fmt.Print(formatRoutes(resp.Routes))
	return 0
}

func cliReload(socketPath string) int {
	resp, err := sendRequest(socketPath, Request{Cmd: "reload"})
	if err != nil {
		printDaemonDownError(err)
		return 1
	}
	if !resp.OK {
		fmt.Fprintln(os.Stderr, resp.Error)
		return 1
	}
	fmt.Printf("reloaded (%d routes)\n", len(resp.Routes))
	return 0
}

// sendRequest dials the admin socket, sends one JSON request, reads one
// JSON response, and returns it.
func sendRequest(socketPath string, req Request) (*Response, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	var resp Response
	dec := json.NewDecoder(bufio.NewReader(conn))
	if err := dec.Decode(&resp); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("daemon closed connection unexpectedly")
		}
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}

// formatRoutes renders a tab-aligned two-column table of routes.
func formatRoutes(routes []Route) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "HOST\tTARGET")
	for _, r := range routes {
		fmt.Fprintf(w, "%s\t%s\n", r.Host, r.Target)
	}
	w.Flush()
	return b.String()
}

// printDaemonDownError prints an OS-appropriate message when the CLI cannot
// reach the daemon.
func printDaemonDownError(cause error) {
	fmt.Fprintln(os.Stderr, "xgate daemon not running:", cause)
	switch runtime.GOOS {
	case "linux":
		fmt.Fprintln(os.Stderr, "start with: sudo systemctl start xgate")
	case "darwin":
		fmt.Fprintln(os.Stderr, "start with: sudo launchctl load /Library/LaunchDaemons/com.xgate.daemon.plist")
	}
}
