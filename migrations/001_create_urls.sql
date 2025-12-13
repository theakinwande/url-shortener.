-- ===========================================
-- Migration 001: Create URLs Table
-- ===========================================
-- This is the core table for the URL shortener.
-- 
-- WHY UUID FOR ID?
-- - Globally unique without coordination
-- - Can generate IDs in application code (no DB roundtrip)
-- - Harder to guess/enumerate than sequential IDs (security!)
--
-- WHY VARCHAR(16) FOR SHORT_CODE?
-- - Base62 with 8 chars = 62^8 = 218 trillion combinations
-- - 16 chars is generous, allows custom aliases
-- - VARCHAR vs TEXT: enforces max length at DB level
-- ===========================================

CREATE TABLE IF NOT EXISTS urls (
    -- Primary key using PostgreSQL's native UUID generation
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- The short code (e.g., "abc123" in example.com/abc123)
    -- UNIQUE constraint creates an index automatically
    short_code VARCHAR(16) UNIQUE NOT NULL,
    
    -- The destination URL
    -- TEXT has no length limit, perfect for long URLs
    original_url TEXT NOT NULL,
    
    -- Click counter for analytics
    -- DEFAULT 0 means we don't need to specify on INSERT
    clicks INTEGER DEFAULT 0,
    
    -- Optional: which API key created this URL
    -- NULL means it was created anonymously (if allowed)
    api_key_id UUID,
    
    -- Optional expiration timestamp
    -- NULL means never expires
    -- TIMESTAMP WITH TIME ZONE stores in UTC (always use this!)
    expires_at TIMESTAMP WITH TIME ZONE,
    
    -- Automatic creation timestamp
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- ===========================================
-- Indexes for Performance
-- ===========================================
-- 
-- WHY INDEXES?
-- Without indexes, PostgreSQL scans every row (slow!).
-- With indexes, it jumps directly to matching rows.
--
-- Rule of thumb: Index columns used in WHERE clauses.
-- ===========================================

-- Index on short_code (already created by UNIQUE constraint)
-- This is the most critical index - used on every redirect

-- Index for checking expired URLs
-- Used by cleanup job that deletes expired entries
CREATE INDEX IF NOT EXISTS idx_urls_expires_at 
    ON urls(expires_at) 
    WHERE expires_at IS NOT NULL;

-- Index for finding URLs by API key
-- Used in dashboard to show "your URLs"
CREATE INDEX IF NOT EXISTS idx_urls_api_key_id 
    ON urls(api_key_id) 
    WHERE api_key_id IS NOT NULL;

-- ===========================================
-- LEARNING NOTES:
-- ===========================================
-- 
-- 1. Partial Indexes (WHERE clause in CREATE INDEX):
--    Only indexes rows matching the condition.
--    Smaller index = faster queries = less disk space.
--
-- 2. IF NOT EXISTS:
--    Makes migrations idempotent (can run multiple times safely).
--
-- 3. TIMESTAMP WITH TIME ZONE:
--    Always use this over plain TIMESTAMP.
--    Stores in UTC, converts to local time on read.
--    Prevents timezone bugs!
--
-- 4. DEFAULT values:
--    Reduce application code complexity.
--    DB handles defaults, app doesn't need to worry.
-- ===========================================
