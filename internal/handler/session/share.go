package session

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Shares are public, read-only snapshots of selected messages. Files are
// copied into the database at share time, so the public routes never touch
// tenant-scoped sessions or storage.
// ponytail: file bytes live in the DB; move them to object storage if shares
// of large files become common.
const (
	shareMaxMessages  = 200
	shareMaxFileBytes = 50 << 20
	shareMaxTotal     = 200 << 20
)

type conversationShare struct {
	ID        string `gorm:"primaryKey"`
	Token     string
	TenantID  uint64
	UserID    string
	SessionID string
	Title     string
	AgentName string
	Items     string
	CreatedAt time.Time
}

func (conversationShare) TableName() string { return "conversation_shares" }

type conversationShareFile struct {
	ID        string `gorm:"primaryKey"`
	ShareID   string
	MessageID string
	Position  int
	FileName  string
	FileSize  int64
	Content   []byte
	CreatedAt time.Time
}

func (conversationShareFile) TableName() string { return "conversation_share_files" }

type shareFile struct {
	Position int    `json:"position"`
	FileName string `json:"file_name"`
	FileType string `json:"file_type"`
	FileSize int64  `json:"file_size"`
}

type shareItem struct {
	MessageID string      `json:"message_id"`
	Role      string      `json:"role"`
	Content   string      `json:"content"`
	CreatedAt time.Time   `json:"created_at"`
	Files     []shareFile `json:"files,omitempty"`
}

type createShareRequest struct {
	MessageIDs []string `json:"message_ids"`
	Title      string   `json:"title"`
	AgentName  string   `json:"agent_name"`
}

func newShareToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateShare snapshots the selected messages of a session into a public link.
//
// @Router /sessions/{id}/shares [post]
func (h *Handler) CreateShare(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := secutils.SanitizeForLog(paramSessionID(c))
	var req createShareRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.MessageIDs) == 0 {
		c.Error(errors.NewBadRequestError("message_ids is required"))
		return
	}
	if len(req.MessageIDs) > shareMaxMessages {
		c.Error(errors.NewBadRequestError("too many messages selected"))
		return
	}
	if h.db == nil {
		c.Error(errors.NewInternalServerError("share storage unavailable"))
		return
	}
	session, err := h.sessionService.GetSession(ctx, sessionID)
	if err != nil {
		if stderrors.Is(err, errors.ErrSessionNotFound) {
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	shareID := uuid.NewString()
	var items []shareItem
	var files []conversationShareFile
	var total int64
	seen := map[string]bool{}
	for _, id := range req.MessageIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		msg, err := h.messageService.GetMessage(ctx, sessionID, id)
		if err != nil || msg == nil {
			c.Error(errors.NewNotFoundError("message not found: " + secutils.SanitizeForLog(id)))
			return
		}
		item := shareItem{MessageID: msg.ID, Role: msg.Role, Content: msg.Content, CreatedAt: msg.CreatedAt}
		for i, a := range msg.Artifacts {
			if a.Deleted() {
				continue
			}
			reader, artifact, err := h.openMessageArtifact(ctx, msg, i)
			if err != nil {
				// A missing blob must not silently shrink the share.
				c.Error(errors.NewBadRequestError("file not shareable: " + a.FileName + " (" + err.Error() + ")"))
				return
			}
			content, err := io.ReadAll(io.LimitReader(reader, shareMaxFileBytes+1))
			reader.Close()
			if err != nil {
				c.Error(errors.NewInternalServerError("read file failed: " + artifact.FileName))
				return
			}
			total += int64(len(content))
			if len(content) > shareMaxFileBytes || total > shareMaxTotal {
				c.Error(errors.NewBadRequestError("files too large to share: " + artifact.FileName))
				return
			}
			item.Files = append(item.Files, shareFile{
				Position: i, FileName: artifact.FileName, FileType: artifact.FileType, FileSize: int64(len(content)),
			})
			files = append(files, conversationShareFile{
				ID: uuid.NewString(), ShareID: shareID, MessageID: msg.ID, Position: i,
				FileName: artifact.FileName, FileSize: int64(len(content)), Content: content, CreatedAt: time.Now(),
			})
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	itemsJSON, _ := json.Marshal(items)

	token, err := newShareToken()
	if err != nil {
		c.Error(errors.NewInternalServerError("token generation failed"))
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = session.Title
	}
	userID, _ := types.UserIDFromContext(ctx)
	share := conversationShare{
		ID: shareID, Token: token, TenantID: types.MustTenantIDFromContext(ctx), UserID: userID,
		SessionID: sessionID, Title: title, AgentName: strings.TrimSpace(req.AgentName),
		Items: string(itemsJSON), CreatedAt: time.Now(),
	}
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&share).Error; err != nil {
			return err
		}
		for i := range files {
			if err := tx.Create(&files[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.Warnf(ctx, "create share failed: session=%s err=%v", sessionID, err)
		c.Error(errors.NewInternalServerError("create share failed"))
		return
	}

	path := "/s/" + token
	url := ""
	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("SHARE_PUBLIC_BASE_URL")), "/"); base != "" {
		url = base + path
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"token": token, "path": path, "url": url}})
}

func (h *Handler) findShare(c *gin.Context) (*conversationShare, bool) {
	token := c.Param("token")
	if h.db == nil || token == "" || len(token) > 64 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "share not found"})
		return nil, false
	}
	var share conversationShare
	if err := h.db.WithContext(c.Request.Context()).Where("token = ?", token).Take(&share).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "share not found"})
		return nil, false
	}
	return &share, true
}

// GetPublicShare returns a share snapshot. Public: the token is the credential.
func (h *Handler) GetPublicShare(c *gin.Context) {
	share, ok := h.findShare(c)
	if !ok {
		return
	}
	var items []shareItem
	_ = json.Unmarshal([]byte(share.Items), &items)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"title": share.Title, "agent_name": share.AgentName, "created_at": share.CreatedAt, "items": items,
	}})
}

// DownloadPublicShareFile serves a file copied into a share. Public.
func (h *Handler) DownloadPublicShareFile(c *gin.Context) {
	share, ok := h.findShare(c)
	if !ok {
		return
	}
	position, err := strconv.Atoi(c.Param("position"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "file not found"})
		return
	}
	var f conversationShareFile
	if err := h.db.WithContext(c.Request.Context()).
		Where("share_id = ? AND message_id = ? AND position = ?", share.ID, c.Param("message_id"), position).
		Take(&f).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "file not found"})
		return
	}
	disposition, contentType := buildAttachmentHeader(f.FileName), mimeTypeFor(f.FileName)
	// WeChat's in-app browser cannot save attachments (it shows a 0 B stub), so
	// the share page asks for inline delivery there and WeChat previews the file.
	if c.Query("inline") == "1" {
		disposition = "inline" + strings.TrimPrefix(disposition, "attachment")
		if strings.HasPrefix(contentType, "text/") {
			contentType = "text/plain; charset=utf-8"
		}
	}
	c.Header("Content-Disposition", disposition)
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, contentType, f.Content)
}
