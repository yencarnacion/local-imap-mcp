package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"local-imap-mcp/internal/config"
	imapclientpkg "local-imap-mcp/internal/imapclient"
	"local-imap-mcp/internal/logging"
	"local-imap-mcp/internal/mcp"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	logging.Startup(cfg)

	imapClient := imapclientpkg.New(cfg)
	runner := mcp.NewToolRunner(imapClient)
	mcpServer := mcp.NewServer(runner)

	root := mcpServer.Handler()
	mux := http.NewServeMux()
	mux.Handle("/", root)
	mux.HandleFunc(cfg.Server.MCPPath, mcpServer.MCPHandler)

	server := &http.Server{
		Addr:              cfg.HTTPAddr(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on http://%s%s", cfg.HTTPAddr(), cfg.Server.MCPPath)
		errCh <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("shutdown error: %v", err)
		}
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}
}
