package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// maxMindmapSourceRunes bounds the answer sent to the model so one long reply
// cannot blow the context window.
const maxMindmapSourceRunes = 20000

// citationTagRE matches the <kb .../> and <web .../> citation tags that can
// remain in the answer text. They carry file names and chunk ids, which must
// never reach a mind map.
var citationTagRE = regexp.MustCompile(`(?i)</?(?:kb|web)\b[^>]*>`)

const (
	maxMindmapNodes  = 120
	maxMindmapDepth  = 5
	maxMindmapLabelR = 120
)

// mindmapSystemPrompt. The example mirrors the structure map the team uses as
// its reference: numbered sections, then "label：detail" points that keep the
// concrete facts (dates, counts, lists, article names). Copy the style and
// granularity, not the content, unless the answer is about this regulation.
const mindmapSystemPrompt = `You turn a Markdown answer into a structured mind map.
Output ONLY a JSON object, no code fence, no explanation, in this shape:
{"title": "topic", "children": [{"title": "section", "children": [{"title": "label：detail"}]}]}

Example of the expected granularity (a structure map of a regulation):
{"title": "行政事业单位内部控制规范（试行）结构", "children": [
 {"title": "一、文件概况", "children": [
   {"title": "发文依据：财政部以文件附件形式印发，全文六章六十五条"},
   {"title": "施行时间：自2014年1月1日起施行"}]},
 {"title": "二、第一章 总则", "children": [
   {"title": "目的依据：提高内部管理水平、规范内部控制、加强廉政风险防控机制建设；依据《会计法》《预算法》等"},
   {"title": "适用范围：党的机关、人大、行政、政协、审判、检察机关，各民主党派机关、人民团体和事业单位的经济活动"},
   {"title": "基本原则：全面性、重要性、制衡性、适应性"}]},
 {"title": "三、第二章 风险评估和控制方法", "children": [
   {"title": "定期评估机制：至少每年一次；外部环境、经济活动或管理要求重大变化时及时重评"},
   {"title": "八种控制方法：不相容岗位分离、内部授权审批、归口管理、预算控制、财产保护、会计控制、单据控制、信息内部公开"}]}
]}

Rules:
- "title" of the root is the topic in at most 20 characters.
- Top-level children are the main sections of the answer, in its order, numbered (一、二、三…) when the answer is ordered. Use 4 to 8 of them.
- Leaves are "label：detail" points of up to 70 characters, each a complete fact taken from the answer. Keep names, numbers, dates and lists exactly as written.
- Go at most %d levels deep and %d nodes in total. Leaves may omit "children".
- Never include document names, file names, chunk ids, citation tags or source references. Write only the knowledge itself.
- Do not invent facts. Write all titles in %s.`

// GenerateMindmapRequest carries the answer to map. The client sends the
// rendered answer text, so the server does not have to re-read the message.
type GenerateMindmapRequest struct {
	Content string `json:"content" binding:"required"`
	Title   string `json:"title"`
}

// MindmapNode is one node of the generated mind map tree.
type MindmapNode struct {
	Title    string        `json:"title"`
	Children []MindmapNode `json:"children,omitempty"`
}

// GenerateMindmap asks the session's KnowledgeQA chat model to summarise an
// answer into a mind map tree. The response must parse as a tree within the
// size limits; anything else is an error so the client can fall back.
func (h *Handler) GenerateMindmap(c *gin.Context) {
	ctx := c.Request.Context()

	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.Error(errors.NewBadRequestError(errors.ErrInvalidSessionID.Error()))
		return
	}

	var req GenerateMindmapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	session, err := h.sessionService.GetSession(ctx, sessionID)
	if err != nil {
		c.Error(errors.NewNotFoundError(err.Error()))
		return
	}

	source := []rune(citationTagRE.ReplaceAllString(req.Content, ""))
	if len(source) > maxMindmapSourceRunes {
		source = source[:maxMindmapSourceRunes]
	}
	system := fmtMindmapPrompt(types.LanguageNameFromContext(ctx))
	user := "Topic: " + strings.TrimSpace(req.Title) + "\n\nAnswer:\n" + string(source)

	// The model picked in this session's input bar goes first.
	sessionModelID := ""
	if session.LastRequestState != nil {
		sessionModelID = session.LastRequestState.ModelID
	}

	var tree *MindmapNode
	for _, modelID := range h.mindmapModelCandidates(ctx, sessionID, sessionModelID) {
		if tree = h.mindmapFromModel(ctx, modelID, system, user); tree != nil {
			break
		}
	}
	if tree == nil {
		c.Error(errors.NewInternalServerError("no model returned a valid mindmap"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": tree})
}

// mindmapFromModel asks one chat model for the mind map and returns nil when
// the call fails or the output is not a usable tree.
func (h *Handler) mindmapFromModel(parent context.Context, modelID, system, user string) *MindmapNode {
	// Bounded per attempt so the three candidates fit inside the client's
	// request timeout (see generateSessionMindmap in the frontend API wrapper).
	ctx, cancel := context.WithTimeout(parent, maxMindmapAttemptTimeout)
	defer cancel()
	chatModel, err := h.modelService.GetChatModel(ctx, modelID)
	if err != nil {
		logger.Warnf(ctx, "Mindmap model unavailable, model=%s: %v", modelID, err)
		return nil
	}
	thinking := false
	response, err := chatModel.Chat(ctx, []chat.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, &chat.ChatOptions{Temperature: 0.2, Thinking: &thinking, MaxCompletionTokens: 4000})
	if err != nil {
		logger.Warnf(ctx, "Mindmap generation failed, model=%s: %v", modelID, err)
		return nil
	}
	tree, ok := parseMindmapTree(response.Content)
	if !ok {
		logger.Warnf(ctx, "Model returned no usable mindmap, model=%s", modelID)
		return nil
	}
	return tree
}

// maxMindmapAttempts bounds how many models are tried, so a dead endpoint
// cannot stall the request for every candidate in turn.
const maxMindmapAttempts = 3

// maxMindmapAttemptTimeout caps one model call. Three attempts at 45s stay
// under the 150s client timeout.
const maxMindmapAttemptTimeout = 45 * time.Second

// mindmapModelCandidates lists models to try in order: the model selected in
// the session, then the model that wrote the session's latest answer, then the
// KnowledgeQA models.
func (h *Handler) mindmapModelCandidates(ctx context.Context, sessionID, sessionModelID string) []string {
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] && len(ids) < maxMindmapAttempts {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	add(sessionModelID)

	if h.messageService != nil {
		if msgs, err := h.messageService.GetRecentMessagesBySession(ctx, sessionID, 50); err == nil {
			var latest *types.Message
			for _, m := range msgs {
				if m != nil && m.Role == "assistant" && m.ModelID != "" &&
					(latest == nil || m.CreatedAt.After(latest.CreatedAt)) {
					latest = m
				}
			}
			if latest != nil {
				add(latest.ModelID)
			}
		}
	}

	if models, err := h.modelService.ListModels(ctx); err == nil {
		for _, m := range models {
			if m != nil && m.Type == types.ModelTypeKnowledgeQA {
				add(m.ID)
			}
		}
	}
	return ids
}

func fmtMindmapPrompt(language string) string {
	return fmt.Sprintf(mindmapSystemPrompt, maxMindmapDepth, maxMindmapNodes, language)
}

// parseMindmapTree extracts the JSON object from the model output and checks
// it is a usable tree: a non-empty root, bounded depth and node count.
func parseMindmapTree(raw string) (*MindmapNode, bool) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, false
	}
	var root MindmapNode
	if err := json.Unmarshal([]byte(raw[start:end+1]), &root); err != nil {
		return nil, false
	}
	count := 0
	if !normalizeMindmapNode(&root, 0, &count) || count < 2 {
		return nil, false
	}
	return &root, true
}

func normalizeMindmapNode(n *MindmapNode, depth int, count *int) bool {
	*count++
	if *count > maxMindmapNodes || depth > maxMindmapDepth {
		return false
	}
	n.Title = strings.TrimSpace(citationTagRE.ReplaceAllString(n.Title, ""))
	if n.Title == "" {
		return false
	}
	if utf8.RuneCountInString(n.Title) > maxMindmapLabelR {
		n.Title = string([]rune(n.Title)[:maxMindmapLabelR])
	}
	for i := range n.Children {
		if !normalizeMindmapNode(&n.Children[i], depth+1, count) {
			return false
		}
	}
	return true
}
