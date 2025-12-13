// ===========================================
// URL Shortener - Main Entry Point
// ===========================================
// This is where everything comes together.
//
// RESPONSIBILITY:
// 1. Load configuration
// 2. Initialize dependencies (DB, Redis)
// 3. Set up HTTP server with middleware
// 4. Start background jobs
// 5. Handle graceful shutdown
//
// DESIGN PRINCIPLE: "Fail Fast at Startup"
// If any critical dependency fails, crash immediately.
// Better to fail during deployment than serve broken requests.
// ===========================================

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/user/urlshortener/internal/config"
	"github.com/user/urlshortener/internal/database"
	"github.com/user/urlshortener/internal/handler"
	"github.com/user/urlshortener/internal/middleware"
	"github.com/user/urlshortener/internal/repository"
	"github.com/user/urlshortener/internal/service"
)

// Version is set at build time using ldflags.
// go build -ldflags "-X main.Version=1.0.0"
var Version = "dev"

func main() {
	// ===========================================
	// Step 0: Load .env File
	// ===========================================
	// Load environment variables from .env file if it exists.
	// This is silently ignored if .env doesn't exist (production).
	_ = godotenv.Load()

	// ===========================================
	// Step 1: Load Configuration
	// ===========================================
	// Configuration is loaded from environment variables.
	// Defaults are provided for local development.
	cfg := config.Load()
	log.Printf("Starting URL Shortener v%s on port %s", Version, cfg.Server.Port)

	// ===========================================
	// Step 2: Initialize PostgreSQL
	// ===========================================
	// Create a context with timeout for startup operations.
	// If we can't connect within 30 seconds, something is wrong.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("Connecting to PostgreSQL...")
	postgres, err := database.NewPostgresDB(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer postgres.Close()
	log.Println("PostgreSQL connected ✓")

	// ===========================================
	// Step 3: Initialize Redis
	// ===========================================
	log.Println("Connecting to Redis...")
	redis, err := database.NewRedisDB(ctx, cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redis.Close()
	log.Println("Redis connected ✓")

	// ===========================================
	// Step 4: Initialize Repositories
	// ===========================================
	// Repositories abstract database access.
	// They depend only on the database pool.
	urlRepo := repository.NewURLRepository(postgres.Pool)
	apiKeyRepo := repository.NewAPIKeyRepository(postgres.Pool)

	// ===========================================
	// Step 5: Initialize Services
	// ===========================================
	// Services contain business logic.
	// They depend on repositories and cache.
	urlService := service.NewURLService(urlRepo, redis, cfg.Shortener)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo)

	// ===========================================
	// Step 6: Initialize Handlers
	// ===========================================
	// Handlers process HTTP requests.
	// They depend on services.
	urlHandler := handler.NewURLHandler(urlService)
	healthHandler := handler.NewHealthHandler(postgres, redis, Version)

	// ===========================================
	// Step 7: Initialize Middleware
	// ===========================================
	rateLimiter := middleware.NewRateLimiter(redis, cfg.RateLimit.RequestsPerMinute)
	apiKeyAuth := middleware.NewAPIKeyAuth(apiKeyService)

	// ===========================================
	// Step 8: Set Up Gin Router
	// ===========================================
	// Gin is a high-performance HTTP framework.
	// In production, set GIN_MODE=release for better performance.
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.DebugMode)
	}

	router := gin.New() // Use New() instead of Default() for full control

	// ===========================================
	// Global Middleware (applied to ALL routes)
	// ===========================================
	// Order matters! Middleware runs in order of addition.

	// 1. Recovery middleware - catches panics, returns 500
	router.Use(gin.Recovery())

	// 2. Logger middleware - logs requests
	router.Use(gin.Logger())

	// 3. Security headers - XSS protection, etc.
	router.Use(middleware.SecurityHeaders())

	// 4. CORS - allow cross-origin requests
	router.Use(middleware.CORS(middleware.DefaultCORSConfig()))

	// ===========================================
	// Health Check Routes (no auth required)
	// ===========================================
	router.GET("/health", healthHandler.Health)
	router.GET("/ready", healthHandler.Ready)
	router.GET("/live", healthHandler.Live)

	// ===========================================
	// Redirect Route (no auth, rate limited)
	// ===========================================
	// This is the main feature - must be fast!
	// Rate limiting protects against abuse
	router.GET("/:shortCode", rateLimiter.Middleware(), urlHandler.Redirect)

	// ===========================================
	// API Routes (auth required, rate limited)
	// ===========================================
	api := router.Group("/api")
	api.Use(rateLimiter.Middleware())
	api.Use(apiKeyAuth.RequireKey())
	{
		// Create short URL
		api.POST("/shorten", urlHandler.Shorten)

		// Get stats for a URL
		api.GET("/stats/:shortCode", urlHandler.GetStats)

		// Delete a URL
		api.DELETE("/:shortCode", urlHandler.Delete)
	}

	// ===========================================
	// Step 9: Create HTTP Server
	// ===========================================
	// Using http.Server gives us control over timeouts.
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// ===========================================
	// Step 10: Start Background Jobs
	// ===========================================
	// Use a separate context for background jobs
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	// Expired URL cleanup job
	go runCleanupJob(bgCtx, urlRepo, 10*time.Minute)

	// ===========================================
	// Step 11: Start Server (non-blocking)
	// ===========================================
	// Start in a goroutine so we can handle shutdown signals
	go func() {
		log.Printf("Server listening on http://localhost:%s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// ===========================================
	// Step 12: Wait for Shutdown Signal
	// ===========================================
	// Graceful shutdown ensures in-flight requests complete.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Give outstanding requests 30 seconds to complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop accepting new requests
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Stop background jobs
	bgCancel()

	log.Println("Server stopped ✓")
}

// ===========================================
// Background Jobs
// ===========================================

// runCleanupJob periodically removes expired URLs.
// Runs until context is cancelled.
func runCleanupJob(ctx context.Context, repo *repository.URLRepository, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Cleanup job stopped")
			return
		case <-ticker.C:
			// Use a short timeout for cleanup operations
			cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			count, err := repo.DeleteExpired(cleanupCtx)
			cancel()

			if err != nil {
				log.Printf("Cleanup error: %v", err)
			} else if count > 0 {
				log.Printf("Cleaned up %d expired URLs", count)
			}
		}
	}
}

// ===========================================
// LEARNING NOTES - Graceful Shutdown:
// ===========================================
//
// WITHOUT GRACEFUL SHUTDOWN:
// - Kill signal → process dies immediately
// - In-flight requests get connection reset
// - Database connections leak
// - Bad user experience
//
// WITH GRACEFUL SHUTDOWN:
// 1. Receive SIGINT/SIGTERM
// 2. Stop accepting NEW requests
// 3. Wait for EXISTING requests to complete
// 4. Close database connections
// 5. Exit cleanly
//
// KUBERNETES:
// K8s sends SIGTERM, waits terminationGracePeriodSeconds
// (default 30s), then sends SIGKILL.
// Make sure your shutdown is faster than the grace period!
//
// ===========================================
// LEARNING NOTES - Dependency Injection:
// ===========================================
//
// Notice how we wire everything together in main():
// 1. Create database connections
// 2. Create repositories (depend on DB)
// 3. Create services (depend on repos)
// 4. Create handlers (depend on services)
//
// This is MANUAL dependency injection.
// Each layer only knows about one level below.
// Benefits:
// - Easy to test (swap real DB for mock)
// - Easy to understand the graph
// - No magic (unlike frameworks like Wire, Dig)
//
// For larger apps, consider DI frameworks,
// but manual DI is clearer for learning.
//
// ===========================================
