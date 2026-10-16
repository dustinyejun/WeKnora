package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

type indexRetryKBStub struct {
	interfaces.KnowledgeBaseService
}

func (s *indexRetryKBStub) GetKnowledgeBaseByIDOnly(_ context.Context, id string) (*types.KnowledgeBase, error) {
	return &types.KnowledgeBase{
		ID:               id,
		SummaryModelID:   "model-1",
		IndexingStrategy: types.IndexingStrategy{WikiEnabled: true},
	}, nil
}

type failingChatModelService struct {
	interfaces.ModelService
}

func (s *failingChatModelService) GetChatModel(context.Context, string) (chat.Chat, error) {
	return nil, errors.New("429 rate limited")
}

type indexRetryPendingRepo struct {
	folderPrunePendingRepoStub
	failCount int
}

func (s *indexRetryPendingRepo) IncrFailCount(context.Context, int64) (int, error) {
	return s.failCount, nil
}

// indexRetryChatStub fails every LLM call but is otherwise a valid chat.Chat,
// modelling a provider outage after GetChatModel succeeded. The error is
// deliberately non-transient (401, not 429) so the call path fails on the
// first attempt without the retry backoff sleeps.
type indexRetryChatStub struct {
	calls int
}

func (c *indexRetryChatStub) Chat(context.Context, []chat.Message, *chat.ChatOptions) (*types.ChatResponse, error) {
	c.calls++
	return nil, errors.New("API request failed with status 401: invalid api key")
}

func (c *indexRetryChatStub) ChatStream(
	context.Context, []chat.Message, *chat.ChatOptions,
) (<-chan types.StreamResponse, error) {
	return nil, errors.New("stream unsupported")
}

func (c *indexRetryChatStub) GetModelID() string   { return "model-1" }
func (c *indexRetryChatStub) GetModelName() string { return "model-1" }

type succeedingModelServiceStub struct {
	interfaces.ModelService
	chat chat.Chat
}

func (s *succeedingModelServiceStub) GetChatModel(_ context.Context, _ string) (chat.Chat, error) {
	return s.chat, nil
}

// indexRetryWikiServiceStub serves the index page and records UpdatePage
// calls, so the test can tell a saved intro from a skipped rebuild.
type indexRetryWikiServiceStub struct {
	interfaces.WikiPageService
	indexPage   *types.WikiPage
	updateCalls int
}

func (s *indexRetryWikiServiceStub) GetIndex(_ context.Context, _ string) (*types.WikiPage, error) {
	return s.indexPage, nil
}

func (s *indexRetryWikiServiceStub) UpdatePage(_ context.Context, page *types.WikiPage) (*types.WikiPage, error) {
	s.updateCalls++
	return page, nil
}

func runRemovedDocFinalize(t *testing.T, failCount int) (*indexRetryPendingRepo, *folderPruneTaskStub) {
	t.Helper()
	rowPayload, err := json.Marshal(wikiFinalizeRow{Change: &wikiFinalizeChange{
		Action: wikiFinalizeRemoved, DocTitle: "修订说明.doc",
	}})
	require.NoError(t, err)
	taskPayload, err := json.Marshal(WikiIngestPayload{TenantID: 1, KnowledgeBaseID: "kb-1"})
	require.NoError(t, err)

	repo := &indexRetryPendingRepo{failCount: failCount}
	repo.rows = []*types.TaskPendingOp{{ID: 21, Op: wikiFinalizeOpChange, Payload: rowPayload}}
	taskQueue := &folderPruneTaskStub{}
	svc := &wikiIngestService{
		wikiService: &folderPruneWikiServiceStub{}, kbService: &indexRetryKBStub{},
		modelService: &failingChatModelService{}, pendingRepo: repo, task: taskQueue,
	}
	require.NoError(t, svc.ProcessWikiFinalize(context.Background(),
		asynq.NewTask(types.TypeWikiFinalize, taskPayload)))
	return repo, taskQueue
}

func TestProcessWikiFinalizeKeepsRemovedDocChangeWhenIndexRebuildFails(t *testing.T) {
	repo, taskQueue := runRemovedDocFinalize(t, 1)
	require.Empty(t, repo.deletedRowIDs, "change row must stay so the intro drops the deleted document later")
	require.Equal(t, []string{types.TypeWikiFinalize}, taskQueue.enqueuedTypes)
}

func TestProcessWikiFinalizeDropsChangeAfterMaxRetries(t *testing.T) {
	repo, _ := runRemovedDocFinalize(t, wikiMaxFailRetries+1)
	require.Equal(t, []int64{21}, repo.deletedRowIDs)
}

// Regression for the retry guard cc9288982 added: rebuildIndexPage used to
// swallow the LLM generation error, re-save the old intro and return nil, so
// indexRebuilt became true and the retry branch never ran for the most
// common failure — the Chat call itself failing while GetChatModel succeeds.
func TestProcessWikiFinalizeKeepsRemovedDocChangeWhenLLMCallFails(t *testing.T) {
	rowPayload, err := json.Marshal(wikiFinalizeRow{Change: &wikiFinalizeChange{
		Action: wikiFinalizeRemoved, DocTitle: "修订说明.doc",
	}})
	require.NoError(t, err)
	taskPayload, err := json.Marshal(WikiIngestPayload{TenantID: 1, KnowledgeBaseID: "kb-1"})
	require.NoError(t, err)

	repo := &indexRetryPendingRepo{failCount: 1}
	repo.rows = []*types.TaskPendingOp{{ID: 21, Op: wikiFinalizeOpChange, Payload: rowPayload}}
	taskQueue := &folderPruneTaskStub{}
	chatStub := &indexRetryChatStub{}
	wikiSvc := &indexRetryWikiServiceStub{indexPage: &types.WikiPage{Content: "旧介绍", Summary: "旧介绍"}}
	svc := &wikiIngestService{
		wikiService: wikiSvc, kbService: &indexRetryKBStub{},
		modelService: &succeedingModelServiceStub{chat: chatStub}, pendingRepo: repo, task: taskQueue,
	}
	require.NoError(t, svc.ProcessWikiFinalize(context.Background(),
		asynq.NewTask(types.TypeWikiFinalize, taskPayload)))

	require.NotZero(t, chatStub.calls, "the LLM call must have been attempted")
	require.Empty(t, repo.deletedRowIDs, "change row must stay so the intro drops the deleted document later")
	require.Zero(t, wikiSvc.updateCalls, "the stale intro must not be re-saved as if the rebuild succeeded")
	require.Equal(t, []string{types.TypeWikiFinalize}, taskQueue.enqueuedTypes)
}
