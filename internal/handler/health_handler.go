// ===========================================
// Package handler - Health Check Handler
// ===========================================
// Health checks are essential for production deployments.
//
// USAGE:
// - Kubernetes readiness/liveness probes
// - Load balancer health checks
// - Monitoring systems (Prometheus, DataDog)
//
// TYPES OF HEALTH CHECKS:
// 1. Liveness: "Is the process alive?" - Basic, fast
// 2. Readiness: "Can the process handle requests?" - Checks dependencies
//
// We implement readiness (more comprehensive).
// ===========================================

package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/urlshortener/internal/database"
	"github.com/user/urlshortener/internal/models"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	postgres *database.PostgresDB
	redis    *database.RedisDB
	version  string
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(pg *database.PostgresDB, redis *database.RedisDB, version string) *HealthHandler {
	return &HealthHandler{
		postgres: pg,
		redis:    redis,
		version:  version,
	}
}

// ===========================================
// GET /health
// ===========================================
// Returns the health status of the service and its dependencies.
//
// Response (200 - healthy):
//
//	{
//	  "status": "healthy",
//	  "version": "1.0.0",
//	  "services": {
//	    "postgres": "ok",
//	    "redis": "ok"
//	  }
//	}
//
// Response (503 - unhealthy):
//
//	{
//	  "status": "unhealthy",
//	  "version": "1.0.0",
//	  "services": {
//	    "postgres": "ok",
//	    "redis": "error: connection refused"
//	  }
//	}
func (h *HealthHandler) Health(c *gin.Context) {
	// Use short timeout - health checks should be fast
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	services := make(map[string]string)
	healthy := true

	// Check PostgreSQL
	if err := h.postgres.Health(ctx); err != nil {
		services["postgres"] = "error: " + err.Error()
		healthy = false
	} else {
		services["postgres"] = "ok"
	}

	// Check Redis
	if err := h.redis.Health(ctx); err != nil {
		services["redis"] = "error: " + err.Error()
		healthy = false
	} else {
		services["redis"] = "ok"
	}

	// Build response
	response := models.HealthResponse{
		Version:  h.version,
		Services: services,
	}

	if healthy {
		response.Status = "healthy"
		c.JSON(http.StatusOK, response)
	} else {
		response.Status = "unhealthy"
		// 503 tells load balancers to route traffic elsewhere
		c.JSON(http.StatusServiceUnavailable, response)
	}
}

// ===========================================
// GET /ready
// ===========================================
// Simpler readiness check - just returns 200 or 503.
// Kubernetes prefers simple responses for probes.
func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	// Quick check of dependencies
	if err := h.postgres.Health(ctx); err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if err := h.redis.Health(ctx); err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	c.Status(http.StatusOK)
}

// ===========================================
// GET /live
// ===========================================
// Liveness check - just confirms the process is running.
// Does NOT check dependencies (that's for readiness).
func (h *HealthHandler) Live(c *gin.Context) {
	c.Status(http.StatusOK)
}

// ===========================================
// LEARNING NOTES - Kubernetes Probes:
// ===========================================
//
// LIVENESS PROBE:
// - Question: "Is the container still running?"
// - If fail: Kubernetes restarts the container
// - Keep it SIMPLE - no dependency checks!
// - Example: /live returns 200 always
//
// READINESS PROBE:
// - Question: "Can the container handle traffic?"
// - If fail: Kubernetes stops sending traffic
// - Check dependencies (DB, Redis, etc.)
// - Example: /ready returns 200 if DB is up
//
// STARTUP PROBE (K8s 1.16+):
// - Question: "Has the container finished starting?"
// - Runs before liveness/readiness
// - Good for slow-starting apps
//
// KUBERNETES CONFIG EXAMPLE:
//
// livenessProbe:
//   httpGet:
//     path: /live
//     port: 8080
//   periodSeconds: 10
//   failureThreshold: 3
//
// readinessProbe:
//   httpGet:
//     path: /ready
//     port: 8080
//   periodSeconds: 5
//   failureThreshold: 2
//
// ===========================================
