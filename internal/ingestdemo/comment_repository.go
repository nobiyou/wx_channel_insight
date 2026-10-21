package ingestdemo

import (
	"context"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

type CommentRepository struct {
	mu       sync.RWMutex
	comments map[string]map[string]ingest.RawCommentRecord
	syncs    map[string]ingest.VideoCommentSyncState
}

var _ ingest.RawCommentRepository = (*CommentRepository)(nil)
var _ ingest.VideoCommentSyncRepository = (*CommentRepository)(nil)

func NewCommentRepository() *CommentRepository {
	return &CommentRepository{
		comments: map[string]map[string]ingest.RawCommentRecord{},
		syncs:    map[string]ingest.VideoCommentSyncState{},
	}
}

func (r *CommentRepository) Upsert(_ context.Context, hubUserID string, items []ingest.RawCommentRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.comments[hubUserID] == nil {
		r.comments[hubUserID] = map[string]ingest.RawCommentRecord{}
	}
	for _, item := range items {
		if item.VideoObjectID == "" || item.CommentID == "" {
			continue
		}
		item.HubUserID = hubUserID
		r.comments[hubUserID][item.VideoObjectID+"\x00"+item.CommentID] = ingest.CloneRawCommentRecord(item)
	}
	return nil
}

func (r *CommentRepository) List(_ context.Context, hubUserID string, query ingest.RawCommentQuery) (ingest.RawCommentPage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]ingest.RawCommentRecord, 0, len(r.comments[hubUserID]))
	for _, item := range r.comments[hubUserID] {
		items = append(items, ingest.CloneRawCommentRecord(item))
	}
	return ingest.PageRawComments(items, query), nil
}

func (r *CommentRepository) GetState(_ context.Context, hubUserID, accountID, videoObjectID string) (ingest.VideoCommentSyncState, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.syncs[syncKey(hubUserID, accountID, videoObjectID)]
	return item, ok, nil
}

func (r *CommentRepository) UpsertState(_ context.Context, state ingest.VideoCommentSyncState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.syncs[syncKey(state.HubUserID, state.AccountID, state.VideoObjectID)] = state
	return nil
}

func syncKey(hubUserID, accountID, videoObjectID string) string {
	return hubUserID + "\x00" + accountID + "\x00" + videoObjectID
}
