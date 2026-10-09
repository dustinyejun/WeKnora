-- Mirrors versioned migration 000115_conversation_shares.
CREATE TABLE IF NOT EXISTS conversation_shares (
    id VARCHAR(36) PRIMARY KEY,
    token VARCHAR(64) NOT NULL,
    tenant_id INTEGER NOT NULL,
    user_id VARCHAR(36) NOT NULL DEFAULT '',
    session_id VARCHAR(36) NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    agent_name TEXT NOT NULL DEFAULT '',
    items TEXT NOT NULL DEFAULT '[]',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_shares_token ON conversation_shares (token);

CREATE TABLE IF NOT EXISTS conversation_share_files (
    id VARCHAR(36) PRIMARY KEY,
    share_id VARCHAR(36) NOT NULL,
    message_id VARCHAR(36) NOT NULL,
    position INTEGER NOT NULL,
    file_name TEXT NOT NULL DEFAULT '',
    file_size INTEGER NOT NULL DEFAULT 0,
    content BLOB NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_share_files_key
    ON conversation_share_files (share_id, message_id, position);
