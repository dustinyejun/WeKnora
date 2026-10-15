package database

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// The public conversation share tables are deliberately not a numbered
// migration. golang-migrate records a single current version, so a numbered
// share migration collides with the next upstream number and is either
// rejected at startup or silently skipped. Every statement here is idempotent
// and runs on each start, independent of AUTO_MIGRATE.
//
// Column types are filled in per dialect. go-sqlite3 only parses DATETIME,
// TIMESTAMP and DATE declarations into time.Time; with "TIMESTAMP WITH TIME
// ZONE" it hands back a string, scanning created_at fails and every share
// reads as not found. The first idempotent version shipped the PostgreSQL
// types to SQLite as well, so SQLite tables created by it are rebuilt in
// place (see repairSQLiteShareSchema).

type shareTable struct {
	name    string
	columns []string // "name TYPE ..." in table order; {{TIMESTAMP}}/{{BLOB}} filled per dialect
	index   string   // CREATE UNIQUE INDEX IF NOT EXISTS statement
}

var conversationShareTables = []shareTable{
	{
		name: "conversation_shares",
		columns: []string{
			"id VARCHAR(36) PRIMARY KEY",
			"token VARCHAR(64) NOT NULL",
			"tenant_id BIGINT NOT NULL",
			"user_id VARCHAR(36) NOT NULL DEFAULT ''",
			"session_id VARCHAR(36) NOT NULL",
			"title TEXT NOT NULL DEFAULT ''",
			"agent_name TEXT NOT NULL DEFAULT ''",
			"items TEXT NOT NULL DEFAULT '[]'",
			"created_at {{TIMESTAMP}} NOT NULL DEFAULT CURRENT_TIMESTAMP",
		},
		index: "CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_shares_token ON conversation_shares (token)",
	},
	{
		name: "conversation_share_files",
		columns: []string{
			"id VARCHAR(36) PRIMARY KEY",
			"share_id VARCHAR(36) NOT NULL",
			"message_id VARCHAR(36) NOT NULL",
			"position INTEGER NOT NULL",
			"file_name TEXT NOT NULL DEFAULT ''",
			"file_size BIGINT NOT NULL DEFAULT 0",
			"content {{BLOB}} NOT NULL",
			"created_at {{TIMESTAMP}} NOT NULL DEFAULT CURRENT_TIMESTAMP",
		},
		index: "CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_share_files_key\n" +
			"    ON conversation_share_files (share_id, message_id, position)",
	},
}

func shareTypeReplacer(dialect string) *strings.Replacer {
	timestamp, blob := "TIMESTAMP WITH TIME ZONE", "BYTEA"
	if dialect == "sqlite" {
		timestamp, blob = "DATETIME", "BLOB"
	}
	return strings.NewReplacer("{{TIMESTAMP}}", timestamp, "{{BLOB}}", blob)
}

// createStatement returns CREATE TABLE [IF NOT EXISTS] for the table under the
// given name, so one definition serves both the idempotent create and the
// rebuild target.
func (t shareTable) createStatement(dialect, name string, ifNotExists bool) string {
	r := shareTypeReplacer(dialect)
	cols := make([]string, len(t.columns))
	for i, c := range t.columns {
		cols[i] = "    " + r.Replace(c)
	}
	keyword := "CREATE TABLE "
	if ifNotExists {
		keyword = "CREATE TABLE IF NOT EXISTS "
	}
	return keyword + name + " (\n" + strings.Join(cols, ",\n") + "\n)"
}

func (t shareTable) columnNames() []string {
	names := make([]string, len(t.columns))
	for i, c := range t.columns {
		names[i] = strings.Fields(c)[0]
	}
	return names
}

// ConversationShareSchemaStatements returns the idempotent share DDL for a GORM
// dialect name ("postgres" or "sqlite"). Anything other than sqlite gets the
// PostgreSQL types.
func ConversationShareSchemaStatements(dialect string) []string {
	out := make([]string, 0, 2*len(conversationShareTables))
	for _, t := range conversationShareTables {
		out = append(out, t.createStatement(dialect, t.name, true), t.index)
	}
	return out
}

// EnsureConversationShareSchema creates the share tables if they are missing
// and, on SQLite, rebuilds tables whose created_at cannot be scanned.
func EnsureConversationShareSchema(db *gorm.DB) error {
	dialect := db.Name() // Dialector.Name(), promoted through the embedded gorm.Config
	if dialect == "sqlite" {
		if _, err := repairSQLiteShareSchema(db); err != nil {
			return fmt.Errorf("ensure conversation share schema: %w", err)
		}
	}
	for _, stmt := range ConversationShareSchemaStatements(dialect) {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("ensure conversation share schema: %w", err)
		}
	}
	return nil
}

// sqliteDeclaresTime reports whether go-sqlite3 parses a column with this
// declared type into time.Time. The driver compares the lower-cased declared
// type against exactly these three names; "timestamp with time zone",
// "timestamptz" or TEXT come back as strings.
func sqliteDeclaresTime(declType string) bool {
	switch strings.ToLower(strings.TrimSpace(declType)) {
	case "datetime", "timestamp", "date":
		return true
	}
	return false
}

// repairSQLiteShareSchema rebuilds any share table whose created_at was
// declared with a type go-sqlite3 cannot scan. SQLite cannot alter a column
// type, so the rows are copied into a correctly typed table inside one
// transaction: create new, INSERT ... SELECT every column, drop old, rename,
// recreate the unique index. Returns the number of tables rebuilt.
func repairSQLiteShareSchema(db *gorm.DB) (int, error) {
	rebuilt := 0
	for _, t := range conversationShareTables {
		var declType string
		q := fmt.Sprintf("SELECT type FROM pragma_table_info('%s') WHERE name = 'created_at'", t.name)
		if err := db.Raw(q).Scan(&declType).Error; err != nil {
			return rebuilt, fmt.Errorf("inspect %s: %w", t.name, err)
		}
		if declType == "" || sqliteDeclaresTime(declType) {
			continue // table missing (created by the caller) or already correct
		}
		tmp := t.name + "__rebuild"
		cols := strings.Join(t.columnNames(), ", ")
		err := db.Transaction(func(tx *gorm.DB) error {
			stmts := []string{
				"DROP TABLE IF EXISTS " + tmp,
				t.createStatement("sqlite", tmp, false),
				fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s", tmp, cols, cols, t.name),
				"DROP TABLE " + t.name,
				fmt.Sprintf("ALTER TABLE %s RENAME TO %s", tmp, t.name),
				t.index,
			}
			for _, s := range stmts {
				if err := tx.Exec(s).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return rebuilt, fmt.Errorf("rebuild %s (created_at declared %q): %w", t.name, declType, err)
		}
		rebuilt++
	}
	return rebuilt, nil
}

// ConversationShareManualSQL renders the share DDL as a plain SQL script for
// migrations/manual/. It exists for deployments that apply schema changes
// with a privileged account and run the application without DDL rights
// (AUTO_MIGRATE=false): make migrate-up never creates these tables, so the
// operator runs this file instead. TestConversationShareManualSQLInSync keeps
// the checked-in files equal to this output.
func ConversationShareManualSQL(dialect string) string {
	var b strings.Builder
	b.WriteString("-- Generated from internal/database/conversation_share_schema.go; do not edit by hand.\n")
	b.WriteString("-- Regenerate: UPDATE_SHARE_SQL=1 go test -run TestConversationShareManualSQLInSync" +
		" ./internal/database/\n")
	b.WriteString("-- Conversation share tables for " + dialect + ". Every statement is idempotent.\n")
	b.WriteString("-- Run with an account that may create tables when the application account cannot.\n\n")
	for _, stmt := range ConversationShareSchemaStatements(dialect) {
		b.WriteString(stmt)
		b.WriteString(";\n\n")
	}
	return b.String()
}
