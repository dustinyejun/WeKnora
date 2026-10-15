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
