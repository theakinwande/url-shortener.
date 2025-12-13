// ===========================================
// Package service - Business Logic Layer
// ===========================================
// Services contain the business logic of your application.
// They orchestrate repositories, caches, and external services.
//
// WHY SERVICES?
// - Handlers are thin (HTTP in/out only)
// - Repositories are thin (DB in/out only)
// - Services contain the "brain" of the application
//
// SINGLE RESPONSIBILITY:
// Each service handles one domain area.
// URLService handles URLs, AuthService handles auth, etc.
// ===========================================

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/user/urlshortener/internal/config"
	"github.com/user/urlshortener/internal/database"
	"github.com/user/urlshortener/internal/models"
	"github.com/user/urlshortener/internal/repository"
)

// Service errors
var (
	ErrURLNotFound = errors.New("URL not found")
	ErrURLExpired  = errors.New("URL has expired")
	ErrCodeTaken   = errors.New("short code already taken")
	ErrInvalidCode = errors.New("invalid short code format")
	ErrInvalidURL  = errors.New("invalid URL format")
)

// URLService handles URL shortening business logic.
type URLService struct {
	repo    *repository.URLRepository
	cache   *database.RedisDB
	config  config.ShortenerConfig
	baseURL string
}

// NewURLService creates a new URL service.
func NewURLService(
	repo *repository.URLRepository,
	cache *database.RedisDB,
	cfg config.ShortenerConfig,
) *URLService {
	return &URLService{
		repo:    repo,
		cache:   cache,
		config:  cfg,
		baseURL: cfg.BaseURL,
	}
}

// ===========================================
// Core Business Operations
// ===========================================

// Create generates a new short URL.
// This is the main "shorten" endpoint logic.
//
// FLOW:
// 1. Validate input URL
// 2. Generate or validate short code
// 3. Check if code is available
// 4. Store in database
// 5. Cache the result
// 6. Return response
func (s *URLService) Create(ctx context.Context, req models.CreateURLRequest) (*models.CreateURLResponse, error) {
	// Step 1: Validate URL format
	if !isValidURL(req.URL) {
		return nil, ErrInvalidURL
	}

	// Step 2: Determine short code
	var shortCode string
	if req.CustomAlias != "" {
		// User provided custom alias
		shortCode = strings.ToLower(req.CustomAlias)
		if !isValidShortCode(shortCode, s.config.MaxCustomLength) {
			return nil, ErrInvalidCode
		}
	} else {
		// Generate random code
		var err error
		shortCode, err = s.generateUniqueCode(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to generate code: %w", err)
		}
	}

	// Step 3: Check availability
	exists, err := s.repo.Exists(ctx, shortCode)
	if err != nil {
		return nil, fmt.Errorf("failed to check code availability: %w", err)
	}
	if exists {
		return nil, ErrCodeTaken
	}

	// Step 4: Calculate expiration
	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	// Step 5: Create URL model
	url := &models.URL{
		ShortCode:   shortCode,
		OriginalURL: req.URL,
		Clicks:      0,
		ExpiresAt:   expiresAt,
	}

	// Step 6: Store in database
	if err := s.repo.Create(ctx, url); err != nil {
		if errors.Is(err, repository.ErrAlreadyExists) {
			return nil, ErrCodeTaken
		}
		return nil, fmt.Errorf("failed to store URL: %w", err)
	}

	// Step 7: Cache the result (async-safe, non-blocking)
	// We don't fail if cache fails - it's just an optimization
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.cacheURL(cacheCtx, url)
	}()

	// Step 8: Build response
	return &models.CreateURLResponse{
		ShortCode: shortCode,
		ShortURL:  fmt.Sprintf("%s/%s", s.baseURL, shortCode),
		ExpiresAt: expiresAt,
	}, nil
}

// Resolve looks up a short code and returns the original URL.
// This is called on every redirect - MUST be fast!
//
// PERFORMANCE CRITICAL:
// 1. Check cache first (sub-ms)
// 2. Only hit DB on cache miss
// 3. Increment clicks asynchronously
func (s *URLService) Resolve(ctx context.Context, shortCode string) (string, error) {
	// Step 1: Check cache
	url, err := s.getFromCache(ctx, shortCode)
	if err != nil {
		// Log error but continue - cache is optional
	}

	// Step 2: Cache miss - check database
	if url == nil {
		url, err = s.repo.GetByShortCode(ctx, shortCode)
		if errors.Is(err, repository.ErrNotFound) {
			return "", ErrURLNotFound
		}
		if err != nil {
			return "", fmt.Errorf("failed to resolve URL: %w", err)
		}

		// Cache for next time (async)
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = s.cacheURL(cacheCtx, url)
		}()
	}

	// Step 3: Check expiration
	if url.IsExpired() {
		return "", ErrURLExpired
	}

	// Step 4: Increment clicks asynchronously
	// We don't wait for this - it would slow down redirects
	go func() {
		incCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.repo.IncrementClicks(incCtx, shortCode)
	}()

	return url.OriginalURL, nil
}

// GetStats retrieves analytics for a short URL.
func (s *URLService) GetStats(ctx context.Context, shortCode string) (*models.URLStatsResponse, error) {
	url, err := s.repo.GetByShortCode(ctx, shortCode)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrURLNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return &models.URLStatsResponse{
		ShortCode:   url.ShortCode,
		OriginalURL: url.OriginalURL,
		Clicks:      url.Clicks,
		CreatedAt:   url.CreatedAt,
		ExpiresAt:   url.ExpiresAt,
	}, nil
}

// Delete removes a short URL.
func (s *URLService) Delete(ctx context.Context, shortCode string) error {
	// Delete from database
	err := s.repo.Delete(ctx, shortCode)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrURLNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to delete URL: %w", err)
	}

	// Invalidate cache
	cacheKey := database.CacheKey(shortCode)
	if err := s.cache.Delete(ctx, cacheKey); err != nil {
		// Log but don't fail - cache will expire eventually
	}

	return nil
}

// ===========================================
// Short Code Generation
// ===========================================
// We use base62 encoding (a-z, A-Z, 0-9) for short codes.
// This gives us 62^8 = 218 trillion possible codes with 8 chars.
//
// WHY BASE62?
// - URL-safe (no special characters)
// - Case-sensitive (abc ≠ ABC = more combinations)
// - Human-readable
//
// GENERATION STRATEGY:
// 1. Generate cryptographically random bytes
// 2. Convert to base62 string
// 3. Check if unique in database
// 4. Retry if collision (rare with 218T combinations)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// generateUniqueCode creates a unique short code.
// Uses cryptographic randomness for unpredictability.
func (s *URLService) generateUniqueCode(ctx context.Context) (string, error) {
	const maxRetries = 5

	for i := 0; i < maxRetries; i++ {
		code, err := generateRandomCode(s.config.DefaultCodeLength)
		if err != nil {
			return "", err
		}

		// Check uniqueness
		exists, err := s.repo.Exists(ctx, code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
		// Collision - retry with new code
	}

	return "", errors.New("failed to generate unique code after retries")
}

// generateRandomCode creates a random base62 string.
// Uses crypto/rand for secure randomness.
//
// SECURITY NOTE:
// math/rand is NOT suitable for this!
// It's predictable if you know the seed.
// crypto/rand uses OS entropy (truly random).
func generateRandomCode(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	for i := range bytes {
		// Map each random byte to a base62 character
		// bytes[i] is 0-255, we need 0-61
		bytes[i] = base62Chars[bytes[i]%62]
	}

	return string(bytes), nil
}

// ===========================================
// Caching Operations
// ===========================================

// cacheURL stores a URL in Redis.
func (s *URLService) cacheURL(ctx context.Context, url *models.URL) error {
	key := database.CacheKey(url.ShortCode)

	// Calculate TTL based on expiration
	ttl := s.cache.CacheTTL
	if url.ExpiresAt != nil {
		remaining := time.Until(*url.ExpiresAt)
		if remaining < ttl {
			ttl = remaining
		}
	}

	return s.cache.SetJSON(ctx, key, url, ttl)
}

// getFromCache retrieves a URL from Redis.
func (s *URLService) getFromCache(ctx context.Context, shortCode string) (*models.URL, error) {
	key := database.CacheKey(shortCode)

	var url models.URL
	found, err := s.cache.GetJSON(ctx, key, &url)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	return &url, nil
}

// ===========================================
// Validation Helpers
// ===========================================

// isValidURL checks if a string is a valid HTTP(S) URL.
// SECURITY: Only allow http:// and https:// schemes.
// Blocks javascript:, data:, file:, etc.
func isValidURL(rawURL string) bool {
	// Must start with http:// or https://
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return false
	}

	// Basic length check
	if len(rawURL) < 10 || len(rawURL) > 2083 {
		return false
	}

	return true
}

// isValidShortCode checks if a custom alias meets requirements.
func isValidShortCode(code string, maxLength int) bool {
	if len(code) < 3 || len(code) > maxLength {
		return false
	}

	// Only allow alphanumeric characters
	for _, c := range code {
		isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isNumber := c >= '0' && c <= '9'
		if !isLetter && !isNumber {
			return false
		}
	}

	return true
}

// ===========================================
// API Key Service
// ===========================================

// APIKeyService handles API key operations.
type APIKeyService struct {
	repo *repository.APIKeyRepository
}

// NewAPIKeyService creates a new API key service.
func NewAPIKeyService(repo *repository.APIKeyRepository) *APIKeyService {
	return &APIKeyService{repo: repo}
}

// ValidateKey checks if an API key is valid.
// Returns the key info if valid, nil otherwise.
//
// SECURITY FLOW:
// 1. Hash the provided key (we never store plain keys)
// 2. Look up hash in database
// 3. Return key info (for rate limit, etc.)
func (s *APIKeyService) ValidateKey(ctx context.Context, rawKey string) (*models.APIKey, error) {
	// Hash the key
	hash := hashAPIKey(rawKey)

	// Look up in database
	key, err := s.repo.GetByKeyHash(ctx, hash)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil // Invalid key, but not an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to validate key: %w", err)
	}

	// Update last used timestamp (async)
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.repo.UpdateLastUsed(updateCtx, key.ID)
	}()

	return key, nil
}

// GenerateKey creates a new API key.
// Returns the raw key (shown once) and saves the hash.
//
// IMPORTANT: The raw key is only returned ONCE.
// After this, only the hash exists. User must save the key!
func (s *APIKeyService) GenerateKey(ctx context.Context, name string, rateLimit int) (string, *models.APIKey, error) {
	// Generate random key
	rawKey, err := generateAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Hash for storage
	hash := hashAPIKey(rawKey)

	// Create key record
	key := &models.APIKey{
		KeyHash:   hash,
		Name:      name,
		RateLimit: rateLimit,
		IsActive:  true,
	}

	if err := s.repo.Create(ctx, key); err != nil {
		return "", nil, fmt.Errorf("failed to save key: %w", err)
	}

	return rawKey, key, nil
}

// generateAPIKey creates a random API key.
// Format: "sk_live_" + 32 random hex chars = 40 chars total
//
// CONVENTION:
// - sk_ = secret key
// - live_ / test_ = environment indicator
// This makes keys self-documenting!
func generateAPIKey() (string, error) {
	bytes := make([]byte, 16) // 16 bytes = 32 hex chars
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "sk_live_" + hex.EncodeToString(bytes), nil
}

// hashAPIKey creates a SHA-256 hash of an API key.
// SHA-256 is sufficient for high-entropy inputs like API keys.
func hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// ===========================================
// LEARNING NOTES - Goroutines for Async Work:
// ===========================================
//
// We use goroutines for "fire and forget" operations that
// shouldn't slow down the main request:
//
//   go func() {
//       ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
//       defer cancel()
//       _ = doSomething(ctx)
//   }()
//
// WHY context.Background()?
// The parent context (HTTP request) might be cancelled.
// Using Background() gives the goroutine its own lifecycle.
//
// WHY IGNORE ERRORS?
// These are non-critical operations (caching, metrics).
// If they fail, the main operation still succeeded.
// In production, log these errors!
//
// ===========================================
