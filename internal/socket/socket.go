package socket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"

	"xgate/internal/admin"
	"xgate/internal/config"
)

// Request is the wire format for one CLI → daemon call.
type Request struct {
	Cmd    string `json:"cmd"`
	Host   string `json:"host,omitempty"`
	Target string `json:"target,omitempty"`
}

// Response is the wire format for one daemon → CLI reply.
type Response struct {
	OK     bool           `json:"ok"`
	Error  string         `json:"error,omitempty"`
	Routes []config.Route `json:"routes,omitempty"`
}

// Serve listens on a Unix socket at path and handles one request per
// connection. It blocks until ctx is canceled, then closes the listener and
// removes the socket file.
func Serve(ctx context.Context, path string, adminSrv *admin.Server) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("listen %s: %w", path, err)
	}

	if err := os.Chmod(path, 0600); err != nil {
		ln.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	log.Printf("admin socket listening on %s", path)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				os.Remove(path)
				return nil
			}
			log.Printf("socket accept: %v", err)
			continue
		}
		go handleConn(conn, adminSrv)
	}
}

func handleConn(conn net.Conn, adminSrv *admin.Server) {
	defer conn.Close()

	var req Request
	dec := json.NewDecoder(conn)
	if err := dec.Decode(&req); err != nil {
		writeResp(conn, Response{OK: false, Error: fmt.Sprintf("decode request: %v", err)})
		return
	}

	resp := dispatchCommand(adminSrv, req)
	writeResp(conn, resp)
}

func dispatchCommand(adminSrv *admin.Server, req Request) Response {
	switch req.Cmd {
	case "add":
		routes, err := adminSrv.Add(req.Host, req.Target)
		if err != nil {
			return Response{OK: false, Error: err.Error()}
		}
		return Response{OK: true, Routes: routes}
	case "rm":
		routes, err := adminSrv.Remove(req.Host)
		if err != nil {
			return Response{OK: false, Error: err.Error()}
		}
		return Response{OK: true, Routes: routes}
	case "ls":
		return Response{OK: true, Routes: adminSrv.List()}
	case "reload":
		routes, err := adminSrv.Reload()
		if err != nil {
			return Response{OK: false, Error: err.Error()}
		}
		return Response{OK: true, Routes: routes}
	default:
		return Response{OK: false, Error: fmt.Sprintf("unknown command: %q", req.Cmd)}
	}
}

func writeResp(conn net.Conn, resp Response) {
	enc := json.NewEncoder(conn)
	if err := enc.Encode(resp); err != nil {
		log.Printf("socket write: %v", err)
	}
}
