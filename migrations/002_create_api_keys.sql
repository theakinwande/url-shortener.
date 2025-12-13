-- ===========================================
-- Migration 002: Create API Keys Table
-- ===========================================
-- API keys authenticate clients and enable rate limiting.
-- 
-- SECURITY DESIGN:
-- - We NEVER store plain-text API keys
-- - Only store SHA-256 hash of the key
-- - User sees key once on creation, then it's gone
-- - Even if DB is compromised, keys are useless
-- ===========================================

CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- SHA-256 hash of the actual API key
    -- 64 chars = 256 bits in hex
    -- UNIQUE so we can look up by hash
    key_hash VARCHAR(64) UNIQUE NOT NULL,
    
    -- Human-readable name (e.g., "Production Server", "Mobile App")
    name VARCHAR(100) NOT NULL,
    
    -- Per-key rate limit (requests per minute)
    -- Allows different tiers: free=60, pro=1000, etc.
    rate_limit INTEGER DEFAULT 60,
    
    -- Soft delete flag
    -- Instead of DELETE, we set is_active=false
    -- Preserves audit trail and prevents re-use
    is_active BOOLEAN DEFAULT true,
    
    -- Timestamps for auditing
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP WITH TIME ZONE
);

-- Index for fast API key lookup (the hot path!)
-- Every authenticated request hits this index
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash 
    ON api_keys(key_hash) 
    WHERE is_active = true;

-- ===========================================
-- SECURITY NOTES:
-- ===========================================
-- 
-- 1. Why hash API keys?
--    If attacker gets DB access, they can't use the keys.
--    Same principle as password hashing.
--
-- 2. Why SHA-256 instead of bcrypt?
--    API keys are already high-entropy random strings.
--    bcrypt is for low-entropy human passwords.
--    SHA-256 is faster and sufficient here.
--
-- 3. Why soft delete (is_active)?
--    - Preserves history for auditing
--    - Prevents accidental data loss
--    - Allows key rotation without losing metadata
--
-- 4. Rate limit per key:
--    Allows tiered pricing and fair use:
--    - Free tier: 60 req/min
--    - Pro tier: 600 req/min
--    - Enterprise: custom
-- ===========================================

-- ===========================================
-- Insert a default API key for development
-- ===========================================
-- Key: "dev-test-key-12345"
-- SHA-256 hash of above key (pre-computed)
-- In production, generate via API endpoint!
-- ===========================================
INSERT INTO api_keys (key_hash, name, rate_limit)
VALUES (
    'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2',
    'Development Key',
    1000  -- High limit for testing
) ON CONFLICT (key_hash) DO NOTHING;

-- NOTE: The hash above is a placeholder. 
-- The actual hash will be computed by the application.
-- See internal/service/apikey_service.go for hashing logic.
