-- Migration: Add refresh_token and expires_at fields to provider_tokens table
-- Date: 2026-07-27
-- Purpose: Support automatic token refresh for OAuth-based providers

-- Add refresh_token column
ALTER TABLE provider_tokens ADD COLUMN refresh_token VARCHAR(512) DEFAULT '';

-- Add expires_at column with index for efficient expiration queries
ALTER TABLE provider_tokens ADD COLUMN expires_at BIGINT DEFAULT 0;
CREATE INDEX idx_provider_tokens_expires_at ON provider_tokens(expires_at);

-- Update comment
COMMENT ON COLUMN provider_tokens.refresh_token IS 'OAuth refresh token for automatic token renewal';
COMMENT ON COLUMN provider_tokens.expires_at IS 'Unix timestamp when the access token expires (0 = no expiration)';
