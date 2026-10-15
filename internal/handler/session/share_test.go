package session

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newShareTestHandler returns a handler whose share tables live in a fresh
// SQLite file, with one share holding the given files at positions 0..n-1.
func newShareTestHandler(t *testing.T, fileNames ...string) *Handler {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "share.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureConversationShareSchema(db); err != nil {
		t.Fatal(err)
	}
	share := conversationShare{
		ID: "share-1", Token: "tok", TenantID: 1, SessionID: "sess", Items: "[]", CreatedAt: time.Now(),
	}
	if err := db.Create(&share).Error; err != nil {
		t.Fatal(err)
	}
	for i, name := range fileNames {
		f := conversationShareFile{
			ID: "file-" + name, ShareID: share.ID, MessageID: "m1", Position: i,
			FileName: name, FileSize: 3, Content: []byte("abc"), CreatedAt: time.Now(),
		}
		if err := db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
	}
	return &Handler{db: db}
}

func TestDownloadPublicShareFileInlinePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newShareTestHandler(t, "evil.svg", "page.html", "notes.txt", "photo.png", "doc.pdf", "feed.xml")
	r := gin.New()
	r.GET("/api/v1/public/shares/:token/files/:message_id/:position", h.DownloadPublicShareFile)

	cases := []struct {
		name            string
		position        int
		query           string
		wantDisposition string
		wantType        string
	}{
		// Active content never renders inline on the app origin, even when asked.
		{"svg inline", 0, "?inline=1", "attachment", "application/octet-stream"},
		{"html inline", 1, "?inline=1", "attachment", "application/octet-stream"},
		{"xml inline", 5, "?inline=1", "attachment", "application/octet-stream"},
		// Harmless types may be previewed; text is served as plain text.
		{"txt inline", 2, "?inline=1", "inline", "text/plain; charset=utf-8"},
		{"png inline", 3, "?inline=1", "inline", "image/png"},
		{"pdf inline", 4, "?inline=1", "inline", "application/pdf"},
		// Without inline=1 everything is a download.
		{"svg download", 0, "", "attachment", "image/svg+xml"},
		{"png download", 3, "", "attachment", "image/png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			path := "/api/v1/public/shares/tok/files/m1/" + string(rune('0'+tc.position)) + tc.query
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Content-Disposition"); !strings.HasPrefix(got, tc.wantDisposition) {
				t.Errorf("Content-Disposition = %q, want prefix %q", got, tc.wantDisposition)
			}
			if got := w.Header().Get("Content-Type"); got != tc.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantType)
			}
			if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}
}

func TestShareInlineContentType(t *testing.T) {
	const docx = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	cases := []struct {
		name, contentType string
		wantType          string
		wantInline        bool
	}{
		{"a.svg", "image/svg+xml", "", false},
		{"a.xhtml", "application/xhtml+xml", "", false},
		{"a.htm", "text/html; charset=utf-8", "", false},
		{"a.js", "text/javascript", "", false},
		{"a.md", "text/markdown; charset=utf-8", "text/plain; charset=utf-8", true},
		{"a.txt", "application/octet-stream", "text/plain; charset=utf-8", true},
		{"a.docx", docx, docx, true},
		{"a.jpg", "image/jpeg", "image/jpeg", true},
	}
	for _, tc := range cases {
		gotType, gotInline := shareInlineContentType(tc.name, tc.contentType)
		if gotInline != tc.wantInline || gotType != tc.wantType {
			t.Errorf("%s (%s): got (%q, %v), want (%q, %v)",
				tc.name, tc.contentType, gotType, gotInline, tc.wantType, tc.wantInline)
		}
	}
}
