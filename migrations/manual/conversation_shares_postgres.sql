-- Generated from internal/database/conversation_share_schema.go; do not edit by hand.
-- Regenerate: UPDATE_SHARE_SQL=1 go test -run TestConversationShareManualSQLInSync ./internal/database/
-- Conversation share tables for postgres. Every statement is idempotent.
-- Run with an account that may create tables when the application account cannot.

CREATE TABLE IF NOT EXISTS conversation_shares (
    id VARCHAR(36) PRIMARY KEY,
    token VARCHAR(64) NOT NULL,
    tenant_id BIGINT NOT NULL,
    user_id VARCHAR(36) NOT NULL DEFAULT '',
    session_id VARCHAR(36) NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    agent_name TEXT NOT NULL DEFAULT '',
    items TEXT NOT NULL DEFAULT '[]',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_shares_token ON conversation_shares (token);

CREATE TABLE IF NOT EXISTS conversation_share_files (
    id VARCHAR(36) PRIMARY KEY,
    share_id VARCHAR(36) NOT NULL,
    message_id VARCHAR(36) NOT NULL,
    position INTEGER NOT NULL,
    file_name TEXT NOT NULL DEFAULT '',
    file_size BIGINT NOT NULL DEFAULT 0,
    content BYTEA NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_share_files_key
    ON conversation_share_files (share_id, message_id, position);

