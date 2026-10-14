package database

import (
	"fmt"

	"gorm.io/gorm"
)

// conversationShareSchema creates the public conversation share tables.
//
// These are deliberately not a numbered migration. golang-migrate records a
// single current version, so a numbered share migration collides with the next
// upstream number and is either rejected at startup or silently skipped. Every
// statement here is idempotent and runs after the versioned migrations on each
// start.
var conversationShareSchema = []string{
	`CREATE TABLE IF NOT EXISTS conversation_shares (
    id VARCHAR(36) PRIMARY KEY,
    token VARCHAR(64) NOT NULL,
    tenant_id BIGINT NOT NULL,
    user_id VARCHAR(36) NOT NULL DEFAULT '',
    session_id VARCHAR(36) NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    agent_name TEXT NOT NULL DEFAULT '',
    items TEXT NOT NULL DEFAULT '[]',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_shares_token ON conversation_shares (token)`,
	`CREATE TABLE IF NOT EXISTS conversation_share_files (
    id VARCHAR(36) PRIMARY KEY,
    share_id VARCHAR(36) NOT NULL,
    message_id VARCHAR(36) NOT NULL,
    position INTEGER NOT NULL,
    file_name TEXT NOT NULL DEFAULT '',
    file_size BIGINT NOT NULL DEFAULT 0,
    content BYTEA NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_share_files_key
    ON conversation_share_files (share_id, message_id, position)`,
}

// EnsureConversationShareSchema creates the share tables if they are missing.
func EnsureConversationShareSchema(db *gorm.DB) error {
	for _, stmt := range conversationShareSchema {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("ensure conversation share schema: %w", err)
		}
	}
	return nil
}
