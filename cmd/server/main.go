// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	httpAdapter "github.com/kolapsis/shm/internal/adapters/http"
	"github.com/kolapsis/shm/internal/adapters/postgres"
	"github.com/kolapsis/shm/internal/config"
	"github.com/kolapsis/shm/internal/middleware"
	"github.com/kolapsis/shm/web"
)

func main() {
	// Setup structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load database URL
	dbURL := os.Getenv("SHM_DB_DSN")
	if dbURL == "" {
		dbURL = "postgres://user:password@localhost:5432/metrics?sslmode=disable"
	}

	// Connect to database
	logger.Info("connecting to PostgreSQL")
	store, err := postgres.NewStore(dbURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		log.Fatalf("database connection failed: %v", err)
	}
	defer store.Close()
	logger.Info("connected to PostgreSQL")

	// Setup rate limiter
	rlConfig := config.LoadRateLimitConfig()
	rl := middleware.NewRateLimiter(rlConfig)
	defer rl.Stop()

	if rlConfig.Enabled {
		logger.Info("rate limiting enabled")
	}

	// Get optional GitHub token for higher API rate limits
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken != "" {
		logger.Info("GitHub token configured (higher rate limits enabled)")
	}

	// Create router with all dependencies
	router := httpAdapter.NewRouter(httpAdapter.RouterConfig{
		Store:       store,
		RateLimiter: rl,
		GitHubToken: githubToken,
		Logger:      logger,
	})

	// Serve static web assets
	router.Handle("/", http.FileServer(http.FS(web.Assets)))

	// Get port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start server
	logger.Info("server starting",
		"port", port,
		"endpoints", []string{"/v1/register", "/v1/activate", "/v1/snapshot", "/api/v1/admin/*"},
	)

	log.Fatal(http.ListenAndServe(":"+port, router))
}
