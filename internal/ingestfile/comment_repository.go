package ingestfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

type persistedComments struct {
	Comments map[string][]ingest.RawCommentRecord    `json:"comments"`
	Syncs    map[string]ingest.VideoCommentSyncState `json:"syncs"`
}

type CommentRepository struct {
	mu       sync.Mutex
	path     string
	comments map[string][]ingest.RawCommentRecord
	syncs    map[string]ingest.VideoCommentSyncState
}

var _ ingest.RawCommentRepository = (*CommentRepository)(nil)
var _ ingest.VideoCommentSyncRepository = (*CommentRepository)(nil)

func NewCommentRepository(path string) (*CommentRepository, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("comment repository path is required")
	}
	r := &CommentRepository{path: path, comments: map[string][]ingest.RawCommentRecord{}, syncs: map[string]ingest.VideoCommentSyncState{}}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *CommentRepository) Upsert(_ context.Context, hubUserID string, items []ingest.RawCommentRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	byID := map[string]ingest.RawCommentRecord{}
	for _, item := range r.comments[hubUserID] {
		byID[item.VideoObjectID+"\x00"+item.CommentID] = ingest.CloneRawCommentRecord(item)
	}
	for _, item := range items {
		if strings.TrimSpace(item.VideoObjectID) == "" || strings.TrimSpace(item.CommentID) == "" {
			continue
		}
		item.HubUserID = hubUserID
		byID[item.VideoObjectID+"\x00"+item.CommentID] = ingest.CloneRawCommentRecord(item)
	}
	merged := make([]ingest.RawCommentRecord, 0, len(byID))
	for _, item := range byID {
		merged = append(merged, item)
	}
	r.comments[hubUserID] = merged
	return r.persistLocked()
}

func (r *CommentRepository) List(_ context.Context, hubUserID string, query ingest.RawCommentQuery) (ingest.RawCommentPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return ingest.RawCommentPage{}, err
	}
	return ingest.PageRawComments(ingest.CloneRawCommentRecords(r.comments[hubUserID]), query), nil
}

func (r *CommentRepository) GetState(_ context.Context, hubUserID, accountID, videoObjectID string) (ingest.VideoCommentSyncState, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return ingest.VideoCommentSyncState{}, false, err
	}
	item, ok := r.syncs[syncKey(hubUserID, accountID, videoObjectID)]
	return item, ok, nil
}

func (r *CommentRepository) UpsertState(_ context.Context, state ingest.VideoCommentSyncState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	r.syncs[syncKey(state.HubUserID, state.AccountID, state.VideoObjectID)] = state
	return r.persistLocked()
}

func (r *CommentRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *CommentRepository) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r.persistLocked()
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return r.persistLocked()
	}
	var state persistedComments
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Comments != nil {
		r.comments = state.Comments
	}
	if state.Syncs != nil {
		r.syncs = state.Syncs
	}
	return nil
}

func (r *CommentRepository) persistLocked() error {
	data, err := json.MarshalIndent(persistedComments{Comments: r.comments, Syncs: r.syncs}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}

func syncKey(hubUserID, accountID, videoObjectID string) string {
	return hubUserID + "\x00" + accountID + "\x00" + videoObjectID
}
