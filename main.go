package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultConfigPath = "/etc/xgate/config.yaml"
	defaultSocketPath = "/var/run/xgate.sock"
)

func main() {
	configPath, socketPath, rest := parseGlobalFlags(os.Args[1:])

	if len(rest) == 0 {
		fmt.Print(usageText)
		return
	}

	if rest[0] == "serve" {
		if err := runDaemon(configPath, socketPath); err != nil {
			log.Fatal(err)
		}
		return
	}

	os.Exit(runCLI(rest, configPath, socketPath))
}

// parseGlobalFlags extracts --config and --socket from anywhere in args.
// Returns resolved paths (flag > env > default) and the remaining args
// (subcommand and its own arguments).
func parseGlobalFlags(args []string) (configPath, socketPath string, rest []string) {
	configPath = os.Getenv("XGATE_CONFIG")
	if configPath == "" {
		configPath = defaultConfigPath
	}
	socketPath = os.Getenv("XGATE_SOCKET")
	if socketPath == "" {
		socketPath = defaultSocketPath
	}

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--config requires a value")
				os.Exit(2)
			}
			configPath = args[i+1]
			i += 2
		case "--socket":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--socket requires a value")
				os.Exit(2)
			}
			socketPath = args[i+1]
			i += 2
		default:
			rest = append(rest, args[i])
			i++
		}
	}
	return configPath, socketPath, rest
}

func runDaemon(configPath, socketPath string) error {
	if err := ensureConfig(configPath); err != nil {
		return fmt.Errorf("ensure config: %w", err)
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	router, err := NewRouter(cfg.Routes)
	if err != nil {
		return err
	}
	handler := NewRouterHandler(router)

	if cfg.ManageHosts {
		if err := addHostsEntries(cfg.Routes); err != nil {
			log.Printf("WARNING: could not update /etc/hosts: %v (are you root?)", err)
		}
		// Ensure the managed /etc/hosts block is removed on every exit path,
		// including unexpected HTTP server errors — not just clean signals.
		defer func() {
			if err := removeHostsEntries(); err != nil {
				log.Printf("WARNING: could not clean /etc/hosts: %v", err)
			}
		}()
	}

	admin := NewAdminServer(configPath, cfg, handler)

	server := &http.Server{
		Addr:         cfg.Listen,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	socketDone := make(chan error, 1)
	go func() {
		socketDone <- ServeSocket(ctx, socketPath, admin)
	}()

	httpDone := make(chan error, 1)
	go func() {
		log.Printf("xgate listening on %s", cfg.Listen)
		for _, r := range cfg.Routes {
			log.Printf("  %s -> %s", r.Host, r.Target)
		}
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			httpDone <- err
			return
		}
		httpDone <- nil
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-httpDone:
		if err != nil {
			cancel()
			<-socketDone
			return err
		}
	case <-stop:
		log.Println("Shutting down...")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown error: %v", err)
	}

	cancel()
	<-socketDone

	log.Println("Done.")
	return nil
}
