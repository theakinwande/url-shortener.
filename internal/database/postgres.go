// ===========================================
// Package database - PostgreSQL Connection
// ===========================================
// This package manages the PostgreSQL connection pool.
//
// WHY pgxpool (not database/sql)?
// - pgx is the fastest PostgreSQL driver for Go
// - Native support for PostgreSQL types (UUID, JSON, arrays)
// - Connection pooling built-in
// - Better error messages
// - Query tracing support
//
// CONNECTION POOLING:
// Instead of opening a new connection per request (slow!),
// we maintain a pool of reusable connections.
// This is CRITICAL for performance under load.
// ===========================================

package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/user/urlshortener/internal/config"
)

// PostgresDB wraps the connection pool with helper methods.
// Using a struct allows us to add methods and makes testing easier.
type PostgresDB struct {
	Pool *pgxpool.Pool
}

// NewPostgresDB creates a new PostgreSQL connection pool.
// It validates the connection before returning.
//
// PATTERN: "Fail fast at startup"
// If we can't connect to the database, crash immediately.
// Better to fail during deployment than serve broken requests.
func NewPostgresDB(ctx context.Context, cfg config.DatabaseConfig) (*PostgresDB, error) {
	// Parse the connection string into a config object
	// This validates the URL format early
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Configure the connection pool
	// These settings prevent resource exhaustion and connection leaks
	poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	poolConfig.MinConns = int32(cfg.MaxIdleConns)
	poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime

	// HealthCheckPeriod: How often to check if connections are alive
	// Dead connections are removed and replaced
	poolConfig.HealthCheckPeriod = 1 * time.Minute

	// ConnectTimeout: Max time to wait for a connection
	// Prevents hanging forever if DB is down
	poolConfig.ConnConfig.ConnectTimeout = 5 * time.Second

	// Create the connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify the connection works
	// Ping sends a simple query to validate connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresDB{Pool: pool}, nil
}

// Close gracefully shuts down the connection pool.
// Always call this in main() using defer.
//
// WHY GRACEFUL SHUTDOWN?
// - Finishes in-flight queries
// - Releases database connections properly
// - Prevents "too many connections" errors on restart
func (db *PostgresDB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}

// Health checks if the database is responsive.
// Used by the /health endpoint for monitoring.
func (db *PostgresDB) Health(ctx context.Context) error {
	// Use a short timeout for health checks
	// We want fast failure detection
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return db.Pool.Ping(ctx)
}

// Stats returns connection pool statistics.
// Useful for debugging and monitoring.
//
// LEARNING NOTE - Pool Stats:
// - AcquireCount: Total connections acquired
// - AcquiredConns: Currently in-use connections
// - IdleConns: Available connections
// - MaxConns: Pool limit
//
// If AcquiredConns == MaxConns frequently, increase pool size!
func (db *PostgresDB) Stats() *pgxpool.Stat {
	return db.Pool.Stat()
}

// ===========================================
// TRANSACTION HELPER
// ===========================================
// Transactions group multiple queries into an atomic unit.
// Either ALL succeed, or ALL are rolled back.
//
// Example use case:
// 1. Insert new URL
// 2. Log creation event
// If step 2 fails, step 1 is also undone.

// WithTransaction executes a function within a transaction.
// The transaction is committed if fn returns nil, rolled back otherwise.
//
// Usage:
//
//	err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
//	    // Do multiple queries with tx
//	    return nil // or return error to rollback
//	})
func (db *PostgresDB) WithTransaction(ctx context.Context, fn func(tx interface{}) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Use defer to ensure cleanup happens
	defer func() {
		if p := recover(); p != nil {
			// Panic occurred - rollback and re-panic
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	// Execute the function
	if err := fn(tx); err != nil {
		// Error occurred - rollback
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	// Success - commit
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ===========================================
// LEARNING NOTES - Context & Timeouts:
// ===========================================
//
// ctx context.Context is passed to every database operation.
// This allows:
// 1. Cancellation: If client disconnects, query is cancelled
// 2. Timeouts: Prevent slow queries from hanging forever
// 3. Tracing: Pass request ID through for logging
//
// ALWAYS use context.WithTimeout for DB operations:
//
//   ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
//   defer cancel()
//   result, err := db.Pool.Query(ctx, sql)
//
// ===========================================
