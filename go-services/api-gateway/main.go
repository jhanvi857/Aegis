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

	"github.com/aegis/go-services/api-gateway/config"
	"github.com/aegis/go-services/api-gateway/routes"
	"github.com/aegis/go-services/api-gateway/storage"
	"github.com/aegis/go-services/api-gateway/telemetry"
)

func initStorage(cfg *config.Config) storage.Store {
	if cfg.DatabaseURL != "" {
		pgStore, err := storage.NewPostgresStore(cfg.DatabaseURL)
		if err == nil {
			telemetry.Logger.Info("storage", "Connected and migrated PostgreSQL storage", map[string]interface{}{
				"database_url_configured": true,
			})
			return pgStore
		}
		telemetry.Logger.Warn("storage", fmt.Sprintf("Could not connect to PostgreSQL (%v), falling back to in-memory store", err))
	} else {
		telemetry.Logger.Info("storage", "No DATABASE_URL or POSTGRES_HOST provided; using in-memory store")
	}

	return storage.NewMemoryStore()
}

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("[api-gateway] Configuration error: %v", err)
	}

	// Initialize structured logger
	telemetry.Logger.SetLevel(telemetry.ParseLogLevel(cfg.LogLevel))
	telemetry.Logger.Info("system", "Aegis Control Plane API Gateway initializing...", map[string]interface{}{
		"environment": cfg.Environment,
		"port":        cfg.ServerPort,
		"tls_enabled": cfg.TLSEnabled,
		"demo_mode":   cfg.DemoMode,
	})

	// Inject config into routes
	routes.SetConfig(cfg)

	// Initialize dynamic state from topology configuration
	routes.InitState()

	// Initialize Python ML gRPC client
	routes.InitGRPCClient(cfg.PythonGRPCAddr)

	// Initialize Storage & Bootstrap Admin
	store := initStorage(cfg)
	defer store.Close()
	routes.SetAuthStore(store)
	routes.InitBootstrapAdmin(context.Background())

	// Build router with auth and RBAC
	r := routes.SetupRouter(store)

	srv := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if cfg.TLSEnabled {
			telemetry.Logger.Info("server", fmt.Sprintf("Control plane listening on https://localhost:%s (TLS enabled)", cfg.ServerPort))
			if err := srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				telemetry.Logger.Error("server", "Fatal TLS server error", err)
				os.Exit(1)
			}
		} else {
			telemetry.Logger.Info("server", fmt.Sprintf("Control plane listening on http://localhost:%s", cfg.ServerPort))
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				telemetry.Logger.Error("server", "Fatal HTTP server error", err)
				os.Exit(1)
			}
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	telemetry.Logger.Info("system", fmt.Sprintf("Received shutdown signal: %v. Initiating graceful shutdown...", sig))

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		telemetry.Logger.Error("system", "Server graceful shutdown error", err)
	} else {
		telemetry.Logger.Info("system", "Server shut down cleanly")
	}
}
