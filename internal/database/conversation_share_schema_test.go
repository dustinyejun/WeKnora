package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openShareSchemaSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "share.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func sqliteTableCount(t *testing.T, db *gorm.DB, table string) int {
	t.Helper()
	var n int
	const q = "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?"
	if err := db.Raw(q, table).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func sqliteCreatedAtType(t *testing.T, db *gorm.DB, table string) string {
	t.Helper()
	var decl string
	q := fmt.Sprintf("SELECT type FROM pragma_table_info('%s') WHERE name = 'created_at'", table)
	if err := db.Raw(q).Scan(&decl).Error; err != nil {
		t.Fatal(err)
	}
	return decl
}

func TestEnsureConversationShareSchemaIsIdempotent(t *testing.T) {
	db := openShareSchemaSQLite(t)
	for i := 0; i < 2; i++ {
		if err := EnsureConversationShareSchema(db); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	for _, table := range []string{"conversation_shares", "conversation_share_files"} {
		if n := sqliteTableCount(t, db, table); n != 1 {
			t.Errorf("table %s: found %d, want 1", table, n)
		}
	}
	// A freshly created schema is never rebuilt.
	if n, err := repairSQLiteShareSchema(db); err != nil || n != 0 {
		t.Errorf("repair on a correct schema: rebuilt=%d err=%v, want 0 and nil", n, err)
	}
}

// Mirrors the handler's row types: created_at must scan into time.Time.
type shareSchemaRow struct {
	ID        string `gorm:"primaryKey"`
	Token     string
	TenantID  uint64
	SessionID string
	CreatedAt time.Time
}

func (shareSchemaRow) TableName() string { return "conversation_shares" }

type shareSchemaFileRow struct {
	ID        string `gorm:"primaryKey"`
	ShareID   string
	MessageID string
	Position  int
	Content   []byte
	CreatedAt time.Time
}

func (shareSchemaFileRow) TableName() string { return "conversation_share_files" }

func TestConversationShareSchemaSQLiteRoundTrip(t *testing.T) {
	db := openShareSchemaSQLite(t)
	if err := EnsureConversationShareSchema(db); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	share := shareSchemaRow{ID: "s1", Token: "tok", TenantID: 1, SessionID: "sess", CreatedAt: now}
	if err := db.Create(&share).Error; err != nil {
		t.Fatalf("insert share: %v", err)
	}
	file := shareSchemaFileRow{
		ID: "f1", ShareID: "s1", MessageID: "m1", Position: 0, Content: []byte{0, 1, 2, 255}, CreatedAt: now,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("insert file: %v", err)
	}

	var gotShare shareSchemaRow
	if err := db.Where("token = ?", "tok").Take(&gotShare).Error; err != nil {
		t.Fatalf("read share back: %v", err)
	}
	if d := gotShare.CreatedAt.Sub(now); d < -time.Second || d > time.Second {
		t.Errorf("share created_at round trip: got %v want %v", gotShare.CreatedAt, now)
	}

	var gotFile shareSchemaFileRow
	err := db.Where("share_id = ? AND message_id = ? AND position = ?", "s1", "m1", 0).Take(&gotFile).Error
	if err != nil {
		t.Fatalf("read file back: %v", err)
	}
	if string(gotFile.Content) != string(file.Content) {
		t.Errorf("file content round trip: got %v want %v", gotFile.Content, file.Content)
	}
	if gotFile.CreatedAt.IsZero() {
		t.Error("file created_at was not scanned")
	}
}

func TestConversationShareSchemaStatementsByDialect(t *testing.T) {
	sqliteDDL := strings.Join(ConversationShareSchemaStatements("sqlite"), "\n")
	for _, want := range []string{"created_at DATETIME", "content BLOB"} {
		if !strings.Contains(sqliteDDL, want) {
			t.Errorf("sqlite DDL missing %q", want)
		}
	}
	for _, unwanted := range []string{"WITH TIME ZONE", "BYTEA", "{{"} {
		if strings.Contains(sqliteDDL, unwanted) {
			t.Errorf("sqlite DDL still contains %q", unwanted)
		}
	}
	pgDDL := strings.Join(ConversationShareSchemaStatements("postgres"), "\n")
	for _, want := range []string{"created_at TIMESTAMP WITH TIME ZONE", "content BYTEA"} {
		if !strings.Contains(pgDDL, want) {
			t.Errorf("postgres DDL missing %q", want)
		}
	}
	if strings.Contains(pgDDL, "{{") {
		t.Error("postgres DDL has an unfilled placeholder")
	}
}

func TestSQLiteDeclaresTime(t *testing.T) {
	cases := map[string]bool{
		"DATETIME": true, "datetime": true, " Timestamp ": true, "DATE": true,
		"TIMESTAMP WITH TIME ZONE": false, "TIMESTAMPTZ": false, "TEXT": false, "": false,
	}
	for decl, want := range cases {
		if got := sqliteDeclaresTime(decl); got != want {
			t.Errorf("sqliteDeclaresTime(%q) = %v, want %v", decl, got, want)
		}
	}
}

// The first idempotent version ran the PostgreSQL DDL on SQLite too, so
// existing SQLite share tables declare created_at as TIMESTAMP WITH TIME ZONE
// and content as BYTEA. Ensure must rebuild them in place without losing rows
// and then do nothing on the next run.
func TestEnsureConversationShareSchemaRebuildsLegacySQLiteTables(t *testing.T) {
	db := openShareSchemaSQLite(t)
	for _, stmt := range ConversationShareSchemaStatements("postgres") {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("legacy DDL: %v", err)
		}
	}
	created := time.Date(2026, 10, 15, 9, 30, 0, 0, time.Local)
	share := shareSchemaRow{ID: "s1", Token: "tok", TenantID: 7, SessionID: "sess", CreatedAt: created}
	if err := db.Create(&share).Error; err != nil {
		t.Fatalf("insert legacy share: %v", err)
	}
	content := []byte{0, 1, 2, 255, 'x'}
	file := shareSchemaFileRow{
		ID: "f1", ShareID: "s1", MessageID: "m1", Position: 0, Content: content, CreatedAt: created,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("insert legacy file: %v", err)
	}
	// Reproduce the defect: the legacy declaration cannot be scanned.
	var broken shareSchemaRow
	if err := db.Where("token = ?", "tok").Take(&broken).Error; err == nil {
		t.Fatal("expected the legacy table to fail scanning created_at")
	}

	if err := EnsureConversationShareSchema(db); err != nil {
		t.Fatalf("ensure on legacy schema: %v", err)
	}
	if n, err := repairSQLiteShareSchema(db); err != nil || n != 0 {
		t.Errorf("second repair: rebuilt=%d err=%v, want 0 and nil", n, err)
	}
	if err := EnsureConversationShareSchema(db); err != nil {
		t.Fatalf("ensure again: %v", err)
	}

	for _, table := range []string{"conversation_shares", "conversation_share_files"} {
		if got := sqliteCreatedAtType(t, db, table); got != "DATETIME" {
			t.Errorf("%s.created_at declared %q, want DATETIME", table, got)
		}
		if n := sqliteTableCount(t, db, table); n != 1 {
			t.Errorf("table %s: found %d, want 1", table, n)
		}
		if n := sqliteTableCount(t, db, table+"__rebuild"); n != 0 {
			t.Errorf("temporary table %s__rebuild left behind", table)
		}
	}

	var gotShare shareSchemaRow
	if err := db.Where("token = ?", "tok").Take(&gotShare).Error; err != nil {
		t.Fatalf("read rebuilt share: %v", err)
	}
	if gotShare.ID != "s1" || gotShare.TenantID != 7 || gotShare.SessionID != "sess" {
		t.Errorf("share columns changed: %+v", gotShare)
	}
	if d := gotShare.CreatedAt.Sub(created); d < -time.Second || d > time.Second {
		t.Errorf("share created_at: got %v want %v", gotShare.CreatedAt, created)
	}
	var gotFile shareSchemaFileRow
	err := db.Where("share_id = ? AND message_id = ? AND position = ?", "s1", "m1", 0).Take(&gotFile).Error
	if err != nil {
		t.Fatalf("read rebuilt file: %v", err)
	}
	if string(gotFile.Content) != string(content) {
		t.Errorf("file content: got %v want %v", gotFile.Content, content)
	}
	if d := gotFile.CreatedAt.Sub(created); d < -time.Second || d > time.Second {
		t.Errorf("file created_at: got %v want %v", gotFile.CreatedAt, created)
	}

	// The unique indexes survive the rebuild.
	dup := shareSchemaRow{ID: "s2", Token: "tok", TenantID: 7, SessionID: "sess", CreatedAt: created}
	if err := db.Create(&dup).Error; err == nil {
		t.Error("duplicate token accepted: unique index idx_conversation_shares_token missing after rebuild")
	}
	dupFile := shareSchemaFileRow{
		ID: "f2", ShareID: "s1", MessageID: "m1", Position: 0, Content: content, CreatedAt: created,
	}
	if err := db.Create(&dupFile).Error; err == nil {
		t.Error("duplicate (share_id, message_id, position) accepted: unique index missing after rebuild")
	}
}

// migrations/manual/ must equal the Go table definitions. Set UPDATE_SHARE_SQL
// to rewrite the files after changing conversationShareTables.
func TestConversationShareManualSQLInSync(t *testing.T) {
	root := sqliteRepoRoot(t)
	files := map[string]string{
		"postgres": "conversation_shares_postgres.sql",
		"sqlite":   "conversation_shares_sqlite.sql",
	}
	for dialect, name := range files {
		path := filepath.Join(root, "migrations", "manual", name)
		want := ConversationShareManualSQL(dialect)
		if os.Getenv("UPDATE_SHARE_SQL") != "" {
			if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(got) != want {
			t.Errorf("%s is out of date; run UPDATE_SHARE_SQL=1 go test"+
				" -run TestConversationShareManualSQLInSync ./internal/database/", name)
		}
	}
}
