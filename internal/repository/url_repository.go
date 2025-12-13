// ===========================================
// Package repository - Data Access Layer
// ===========================================
// The repository pattern abstracts database operations.
// Handlers call services, services call repositories.
//
// WHY REPOSITORY PATTERN?
// 1. Testability: Mock repositories for unit tests
// 2. Flexibility: Switch databases without changing services
// 3. Single Responsibility: Repository = data access only
// 4. Consistency: All DB operations go through one place
//
// NAMING CONVENTION:
// - Methods named after what they do: Create, GetByID, Delete
// - Input: domain models or primitives
// - Output: domain models or errors
// ===========================================

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/user/urlshortener/internal/models"
)

// Common errors returned by repository methods.
// Using package-level errors allows callers to check with errors.Is().
var (
	ErrNotFound      = errors.New("resource not found")
	ErrAlreadyExists = errors.New("resource already exists")
)

// URLRepository handles all URL database operations.
type URLRepository struct {
	db *pgxpool.Pool
}

// NewURLRepository creates a new URL repository.
func NewURLRepository(db *pgxpool.Pool) *URLRepository {
	return &URLRepository{db: db}
}

// Create inserts a new URL into the database.
// Returns ErrAlreadyExists if the short_code is taken.
//
// SECURITY NOTE - SQL Injection Prevention:
// We use parameterized queries ($1, $2, etc.) instead of
// string concatenation. The driver handles escaping.
// NEVER do: fmt.Sprintf("... WHERE code='%s'", code)
func (r *URLRepository) Create(ctx context.Context, url *models.URL) error {
	query := `
		INSERT INTO urls (id, short_code, original_url, clicks, api_key_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	// Generate UUID if not provided
	if url.ID == uuid.Nil {
		url.ID = uuid.New()
	}
	if url.CreatedAt.IsZero() {
		url.CreatedAt = time.Now()
	}

	_, err := r.db.Exec(ctx, query,
		url.ID,
		url.ShortCode,
		url.OriginalURL,
		url.Clicks,
		url.APIKeyID,
		url.ExpiresAt,
		url.CreatedAt,
	)

	if err != nil {
		// Check for unique constraint violation
		// PostgreSQL error code 23505 = unique_violation
		if isDuplicateKeyError(err) {
			return ErrAlreadyExists
		}
		return fmt.Errorf("failed to create URL: %w", err)
	}

	return nil
}

// GetByShortCode retrieves a URL by its short code.
// Returns ErrNotFound if the URL doesn't exist.
//
// LEARNING NOTE - QueryRow vs Query:
// - QueryRow: Expect exactly one row (like here)
// - Query: Expect multiple rows (returns iterator)
func (r *URLRepository) GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error) {
	query := `
		SELECT id, short_code, original_url, clicks, api_key_id, expires_at, created_at
		FROM urls
		WHERE short_code = $1
	`

	url := &models.URL{}
	err := r.db.QueryRow(ctx, query, shortCode).Scan(
		&url.ID,
		&url.ShortCode,
		&url.OriginalURL,
		&url.Clicks,
		&url.APIKeyID,
		&url.ExpiresAt,
		&url.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get URL: %w", err)
	}

	return url, nil
}

// IncrementClicks atomically increments the click counter.
// Uses UPDATE ... SET clicks = clicks + 1 for atomicity.
//
// WHY ATOMIC INCREMENT?
// Multiple concurrent requests might increment simultaneously.
// clicks = clicks + 1 is atomic at database level.
// In Go code, clicks++ would have race conditions!
func (r *URLRepository) IncrementClicks(ctx context.Context, shortCode string) error {
	query := `
		UPDATE urls
		SET clicks = clicks + 1
		WHERE short_code = $1
	`

	result, err := r.db.Exec(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to increment clicks: %w", err)
	}

	// Check if any row was actually updated
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete removes a URL from the database.
// Returns ErrNotFound if the URL doesn't exist.
//
// HARD DELETE vs SOFT DELETE:
// - Hard delete: Row is gone forever (what we do here)
// - Soft delete: Set deleted_at timestamp, filter in queries
// For URLs, hard delete is fine. For users, prefer soft delete.
func (r *URLRepository) Delete(ctx context.Context, shortCode string) error {
	query := `DELETE FROM urls WHERE short_code = $1`

	result, err := r.db.Exec(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to delete URL: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteExpired removes all expired URLs.
// Returns the number of deleted rows.
//
// USAGE: Call this periodically from a background job.
// See cmd/server/main.go for the cleanup goroutine.
func (r *URLRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM urls
		WHERE expires_at IS NOT NULL
		  AND expires_at < $1
	`

	result, err := r.db.Exec(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired URLs: %w", err)
	}

	return result.RowsAffected(), nil
}

// Exists checks if a short code is already taken.
// More efficient than GetByShortCode when you only need existence check.
//
// PERFORMANCE NOTE:
// SELECT 1 is faster than SELECT * because:
// 1. No data transfer (just checking existence)
// 2. Database can use index-only scan
func (r *URLRepository) Exists(ctx context.Context, shortCode string) (bool, error) {
	query := `SELECT 1 FROM urls WHERE short_code = $1 LIMIT 1`

	var exists int
	err := r.db.QueryRow(ctx, query, shortCode).Scan(&exists)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return true, nil
}

// ===========================================
// API Key Repository
// ===========================================

// APIKeyRepository handles API key database operations.
type APIKeyRepository struct {
	db *pgxpool.Pool
}

// NewAPIKeyRepository creates a new API key repository.
func NewAPIKeyRepository(db *pgxpool.Pool) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

// GetByKeyHash retrieves an API key by its hash.
// Only returns active keys.
func (r *APIKeyRepository) GetByKeyHash(ctx context.Context, keyHash string) (*models.APIKey, error) {
	query := `
		SELECT id, key_hash, name, rate_limit, is_active, created_at, last_used_at
		FROM api_keys
		WHERE key_hash = $1 AND is_active = true
	`

	key := &models.APIKey{}
	err := r.db.QueryRow(ctx, query, keyHash).Scan(
		&key.ID,
		&key.KeyHash,
		&key.Name,
		&key.RateLimit,
		&key.IsActive,
		&key.CreatedAt,
		&key.LastUsedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	return key, nil
}

// UpdateLastUsed updates the last_used_at timestamp.
// Called on every authenticated request.
func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE api_keys SET last_used_at = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, time.Now(), id)
	return err
}

// Create inserts a new API key.
func (r *APIKeyRepository) Create(ctx context.Context, key *models.APIKey) error {
	query := `
		INSERT INTO api_keys (id, key_hash, name, rate_limit, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	if key.ID == uuid.Nil {
		key.ID = uuid.New()
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now()
	}

	_, err := r.db.Exec(ctx, query,
		key.ID,
		key.KeyHash,
		key.Name,
		key.RateLimit,
		key.IsActive,
		key.CreatedAt,
	)

	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrAlreadyExists
		}
		return fmt.Errorf("failed to create API key: %w", err)
	}

	return nil
}

// ===========================================
// Helper Functions
// ===========================================

// isDuplicateKeyError checks if the error is a unique constraint violation.
// PostgreSQL error code 23505 = unique_violation.
func isDuplicateKeyError(err error) bool {
	// pgx wraps errors, so we need to check the error string
	// In production, use a more robust check with pgconn.PgError
	return err != nil && (
	// Check for pgx error code
	contains(err.Error(), "23505") ||
		contains(err.Error(), "duplicate key"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ===========================================
// LEARNING NOTES - Context Propagation:
// ===========================================
//
// Every repository method takes context.Context as first param.
// This allows:
//
// 1. Cancellation propagation:
//    If HTTP request is cancelled, DB query is cancelled too.
//    Prevents wasted work on abandoned requests.
//
// 2. Timeouts:
//    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
//    defer cancel()
//    url, err := repo.GetByShortCode(ctx, code)
//
// 3. Request tracing:
//    ctx = context.WithValue(ctx, "requestID", "abc123")
//    // Later in logs: ctx.Value("requestID")
//
// ===========================================
